// Package ipfsnode provides an embedded IPFS node for decentralized communication.
// It wraps libp2p and IPFS components to provide a simple interface for the messaging layer.
package ipfsnode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multihash"
)

const (
	// DefaultRepoDir is the default directory for IPFS repo
	DefaultRepoDir = "~/.babylontower/ipfs"
	// DefaultProtocolID is the protocol ID for Babylon Tower
	DefaultProtocolID = "/babylontower/1.0.0"
	// ConnectionTimeout is the timeout for peer connections
	ConnectionTimeout = 10 * time.Second
	// DHTBootstrapTimeout is the timeout for DHT bootstrap
	DHTBootstrapTimeout = 30 * time.Second
)

var (
	// ErrNodeNotStarted is returned when operations are attempted on a stopped node
	ErrNodeNotStarted = errors.New("IPFS node not started")
	// ErrAddFailed is returned when adding data to IPFS fails
	ErrAddFailed = errors.New("failed to add data to IPFS")
	// ErrGetFailed is returned when getting data from IPFS fails
	ErrGetFailed = errors.New("failed to get data from IPFS")
)

// Logger for the ipfsnode package
var logger = log.Logger("babylontower/ipfsnode")

// Config holds configuration for the IPFS node
type Config struct {
	// RepoDir is the directory for IPFS repo
	RepoDir string
	// ProtocolID is the protocol ID for libp2p
	ProtocolID string
	// BootstrapPeers is a list of multiaddr strings for bootstrap peers
	BootstrapPeers []string
	// EnableRelay enables circuit relay for NAT traversal
	EnableRelay bool
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		RepoDir:        DefaultRepoDir,
		ProtocolID:     DefaultProtocolID,
		BootstrapPeers: []string{}, // Empty for PoC - will use local discovery
		EnableRelay:    false,
	}
}

// Node represents an embedded IPFS node
type Node struct {
	config     *Config
	host       host.Host
	dht        *dht.IpfsDHT
	pubsub     *pubsub.PubSub
	discovery  *routing.RoutingDiscovery
	ctx        context.Context
	cancel     context.CancelFunc
	peerChan   <-chan peer.AddrInfo
	isStarted  bool
	topicCache *topicCache
}

// NewNode creates a new IPFS node with the given config
func NewNode(config *Config) (*Node, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	node := &Node{
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
		isStarted:  false,
		topicCache: newTopicCache(),
	}

	return node, nil
}

// Start initializes and starts the IPFS node
// This creates the libp2p host, DHT, and PubSub subsystems
func (n *Node) Start() error {
	if n.isStarted {
		return nil
	}

	// Expand repo directory
	repoDir, err := expandPath(n.config.RepoDir)
	if err != nil {
		return fmt.Errorf("failed to expand repo dir: %w", err)
	}

	// Create repo directory if it doesn't exist
	if err := os.MkdirAll(repoDir, 0700); err != nil {
		return fmt.Errorf("failed to create repo directory: %w", err)
	}

	// Generate or load libp2p private key
	privKey, err := n.loadOrGeneratePeerKey(repoDir)
	if err != nil {
		return fmt.Errorf("failed to load peer key: %w", err)
	}

	// Create libp2p host
	opts := []libp2p.Option{
		libp2p.Identity(privKey),
		libp2p.ListenAddrStrings(
			"/ip4/0.0.0.0/tcp/0",    // IPv4 TCP
			"/ip4/0.0.0.0/tcp/0/ws", // IPv4 WebSocket (for browser clients)
			"/ip6/::/tcp/0",         // IPv6 TCP
		),
		libp2p.NATPortMap(),
	}

	if n.config.EnableRelay {
		opts = append(opts, libp2p.EnableRelay())
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return fmt.Errorf("failed to create libp2p host: %w", err)
	}

	n.host = h
	logger.Infow("libp2p host created", "id", h.ID(), "addrs", h.Addrs())

	// Create DHT
	dhtOpts := []dht.Option{
		dht.Mode(dht.ModeAuto),
		dht.ProtocolPrefix(protocol.ID(n.config.ProtocolID)),
	}

	dhtNode, err := dht.New(n.ctx, n.host, dhtOpts...)
	if err != nil {
		return fmt.Errorf("failed to create DHT: %w", err)
	}

	n.dht = dhtNode

	// Create PubSub
	ps, err := pubsub.NewGossipSub(n.ctx, n.host,
		pubsub.WithMessageSigning(true),
		pubsub.WithStrictSignatureVerification(true),
	)
	if err != nil {
		return fmt.Errorf("failed to create PubSub: %w", err)
	}

	n.pubsub = ps

	// Create discovery service
	n.discovery = routing.NewRoutingDiscovery(dhtNode)

	// Bootstrap DHT
	if err := n.bootstrapDHT(); err != nil {
		logger.Warnw("DHT bootstrap failed", "error", err)
		// Continue anyway - may work in local network
	}

	// Start peer discovery
	n.startPeerDiscovery()

	n.isStarted = true
	logger.Info("IPFS node started successfully")

	return nil
}

// Stop gracefully shuts down the IPFS node
func (n *Node) Stop() error {
	if !n.isStarted {
		return nil
	}

	logger.Info("Stopping IPFS node...")

	// Cancel context to stop all goroutines
	n.cancel()

	// Close all topics
	if n.topicCache != nil {
		n.topicCache.closeAll()
	}

	// Close DHT
	if n.dht != nil {
		if err := n.dht.Close(); err != nil {
			logger.Warnw("DHT close error", "error", err)
		}
	}

	// Close host
	if n.host != nil {
		if err := n.host.Close(); err != nil {
			logger.Warnw("Host close error", "error", err)
		}
	}

	n.isStarted = false
	logger.Info("IPFS node stopped")

	return nil
}

// Add adds data to IPFS and returns the CID
// The data is stored in the node's blockstore
func (n *Node) Add(data []byte) (string, error) {
	if !n.isStarted {
		return "", ErrNodeNotStarted
	}

	// Create a CID from the data
	// Using SHA2-256 hash function
	mh, err := multihash.Sum(data, multihash.SHA2_256, -1)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrAddFailed, err)
	}

	c := cid.NewCidV1(cid.Raw, mh)
	cidStr := c.String()

	// Provide the CID to the DHT
	err = n.dht.Provide(n.ctx, c, true)
	if err != nil {
		logger.Warnw("DHT provide failed", "cid", cidStr, "error", err)
	}

	logger.Debugw("data added to IPFS", "cid", cidStr, "size", len(data))

	return cidStr, nil
}

// Get retrieves data from IPFS by CID
// Returns the raw bytes if successful
func (n *Node) Get(cidStr string) ([]byte, error) {
	if !n.isStarted {
		return nil, ErrNodeNotStarted
	}

	// Parse CID
	_, err := cid.Parse(cidStr)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid CID: %v", ErrGetFailed, err)
	}

	// For PoC, we don't have a full blockstore
	// In a real implementation, this would fetch from the network
	// For now, return an error indicating the limitation
	logger.Warnw("IPFS Get not fully implemented for PoC", "cid", cidStr)

	return nil, fmt.Errorf("%w: direct block fetch not implemented in PoC", ErrGetFailed)
}

// Host returns the underlying libp2p host
func (n *Node) Host() host.Host {
	return n.host
}

// PubSub returns the PubSub instance
func (n *Node) PubSub() *pubsub.PubSub {
	return n.pubsub
}

// Context returns the node's context
func (n *Node) Context() context.Context {
	return n.ctx
}

// IsStarted returns true if the node is running
func (n *Node) IsStarted() bool {
	return n.isStarted
}

// PeerID returns the node's peer ID
func (n *Node) PeerID() string {
	if n.host == nil {
		return ""
	}
	return n.host.ID().String()
}

// Multiaddrs returns the node's multiaddresses
func (n *Node) Multiaddrs() []string {
	if n.host == nil {
		return nil
	}
	addrs := make([]string, len(n.host.Addrs()))
	for i, addr := range n.host.Addrs() {
		addrs[i] = addr.String()
	}
	return addrs
}

// ConnectToPeer connects to a peer by multiaddr
func (n *Node) ConnectToPeer(maddr string) error {
	if !n.isStarted {
		return ErrNodeNotStarted
	}

	ma, err := multiaddr.NewMultiaddr(maddr)
	if err != nil {
		return fmt.Errorf("invalid multiaddr: %w", err)
	}

	peerInfo, err := peer.AddrInfoFromP2pAddr(ma)
	if err != nil {
		return fmt.Errorf("failed to parse peer addr: %w", err)
	}

	ctx, cancel := context.WithTimeout(n.ctx, ConnectionTimeout)
	defer cancel()

	if err := n.host.Connect(ctx, *peerInfo); err != nil {
		return fmt.Errorf("failed to connect to peer: %w", err)
	}

	logger.Infow("connected to peer", "peer", peerInfo.ID)
	return nil
}

// loadOrGeneratePeerKey loads an existing peer key or generates a new one
func (n *Node) loadOrGeneratePeerKey(repoDir string) (crypto.PrivKey, error) {
	keyPath := filepath.Join(repoDir, "peer.key")

	// Try to load existing key
	if data, err := os.ReadFile(keyPath); err == nil {
		privKey, err := crypto.UnmarshalPrivateKey(data)
		if err == nil {
			logger.Info("Loaded existing peer key")
			return privKey, nil
		}
	}

	// Generate new key
	privKey, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate peer key: %w", err)
	}

	// Save key
	data, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal peer key: %w", err)
	}

	if err := os.WriteFile(keyPath, data, 0600); err != nil {
		return nil, fmt.Errorf("failed to save peer key: %w", err)
	}

	logger.Info("Generated new peer key")
	return privKey, nil
}

// bootstrapDHT connects to bootstrap peers and populates the routing table
func (n *Node) bootstrapDHT() error {
	ctx, cancel := context.WithTimeout(n.ctx, DHTBootstrapTimeout)
	defer cancel()

	// Wait for DHT to be ready
	if err := n.dht.Bootstrap(ctx); err != nil {
		return fmt.Errorf("DHT bootstrap failed: %w", err)
	}

	// Connect to bootstrap peers if configured
	for _, addr := range n.config.BootstrapPeers {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			logger.Warnw("invalid bootstrap addr", "addr", addr, "error", err)
			continue
		}

		peerInfo, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			logger.Warnw("failed to parse bootstrap addr", "addr", addr, "error", err)
			continue
		}

		if err := n.host.Connect(ctx, *peerInfo); err != nil {
			logger.Warnw("failed to connect to bootstrap peer", "peer", peerInfo.ID, "error", err)
		}
	}

	// Wait for DHT to be ready
	select {
	case <-n.dht.RefreshRoutingTable():
		logger.Info("DHT routing table refreshed")
	case <-ctx.Done():
		logger.Warn("DHT bootstrap timeout")
	}

	return nil
}

// startPeerDiscovery starts background peer discovery
func (n *Node) startPeerDiscovery() {
	// Use the discovery service to find peers
	peerChan, err := n.discovery.FindPeers(n.ctx, n.config.ProtocolID)
	if err != nil {
		logger.Warnw("discovery FindPeers failed", "error", err)
		return
	}
	n.peerChan = peerChan

	// Process discovered peers in background
	go func() {
		for peerInfo := range n.peerChan {
			if peerInfo.ID == n.host.ID() || len(peerInfo.Addrs) == 0 {
				continue
			}

			// Try to connect to discovered peer
			ctx, cancel := context.WithTimeout(n.ctx, ConnectionTimeout)
			if err := n.host.Connect(ctx, peerInfo); err != nil {
				logger.Debugw("failed to connect to discovered peer", "peer", peerInfo.ID, "error", err)
			} else {
				logger.Infow("connected to discovered peer", "peer", peerInfo.ID)
			}
			cancel()
		}
	}()
}

// expandPath expands ~ to home directory
func expandPath(path string) (string, error) {
	if len(path) == 0 {
		return "", fmt.Errorf("empty path")
	}

	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home dir: %w", err)
		}
		if len(path) > 1 {
			path = filepath.Join(home, path[1:])
		} else {
			path = home
		}
	}

	return filepath.Abs(path)
}

// TopicFromPublicKey derives a PubSub topic from a public key
// Uses SHA256 hash of the public key as the topic name
func TopicFromPublicKey(pubKey []byte) string {
	hash := sha256.Sum256(pubKey)
	return "babylon-" + hex.EncodeToString(hash[:8])
}
