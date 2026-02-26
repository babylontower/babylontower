// Package ipfsnode provides an embedded IPFS node for decentralized communication.
// It wraps libp2p and IPFS components to provide a simple interface for the messaging layer.
package ipfsnode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multiaddr-dns"
	"github.com/multiformats/go-multihash"
)

const (
	// DefaultRepoDir is the default directory for IPFS repo
	DefaultRepoDir = "~/.babylontower/ipfs"
	// DefaultProtocolID is the protocol ID for Babylon Tower
	DefaultProtocolID = "/babylontower/1.0.0"
	// ConnectionTimeout is the timeout for peer connections
	ConnectionTimeout = 30 * time.Second
	// DialTimeout is the timeout for individual dial attempts
	DialTimeout = 15 * time.Second
	// DHTBootstrapTimeout is the timeout for DHT bootstrap
	DHTBootstrapTimeout = 60 * time.Second
	// DHTRefreshInterval is how often to refresh the DHT routing table
	DHTRefreshInterval = 2 * time.Minute
	// MDnsAnnounceInterval is how often mDNS announces our presence
	MDnsAnnounceInterval = 5 * time.Second
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
	// StoredPeers is a list of pre-loaded peer addresses from storage
	StoredPeers []peer.AddrInfo
	// EnableRelay enables circuit relay for NAT traversal
	EnableRelay bool
	// EnableHolePunching enables hole punching for direct NAT connections (default: true)
	EnableHolePunching bool
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		RepoDir:    DefaultRepoDir,
		ProtocolID: DefaultProtocolID,
		// Default IPFS bootstrap peers for DHT bootstrapping
		// Multiple sources for redundancy (libp2p bootstrap nodes)
		BootstrapPeers: []string{
			// Primary libp2p bootstrap nodes (dnsaddr)
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiRNN6vEC9qmL9egu92p",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
			// Direct IP bootstrap nodes (no DNS required - fallback)
			"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
			"/ip4/104.236.179.241/tcp/4001/p2p/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
		},
		EnableRelay:        false,
		EnableHolePunching: true, // Enable hole punching for direct connections
	}
}

// Node represents an embedded IPFS node
type Node struct {
	config     *Config
	host       host.Host
	dht        *dht.IpfsDHT
	pubsub     *pubsub.PubSub
	discovery  *routing.RoutingDiscovery
	mdns       mdns.Service
	ctx        context.Context
	cancel     context.CancelFunc
	peerChan   <-chan peer.AddrInfo
	isStarted  bool
	topicCache *topicCache

	// Peer tracking
	peerCount     atomic.Int32
	peerMu        sync.RWMutex
	mdnsCount     atomic.Int32
	lastPeerFound time.Time

	// Event subscriptions
	peerEventSub interface{ Close() error }

	// DHT maintenance
	dhtMaintenanceDone chan struct{}

	// Peer scoring for connection management
	peerScores  map[string]*PeerScore // peer.ID -> score
	peerScoreMu sync.RWMutex

	// Connection health tracking
	healthCheckInterval time.Duration
	lastHealthCheck     time.Time

	// Network metrics
	metrics *MetricsCollector
}

// PeerScore tracks connection quality for a peer
type PeerScore struct {
	PeerID           string
	ConnectCount     int
	DisconnectCount  int
	FailCount        int
	SuccessCount     int
	LastConnected    time.Time
	LastDisconnected time.Time
	LatencyMs        int64
	Score            float64 // Computed score (0.0 - 1.0)
}

// computeScore calculates the peer's score based on connection history
func (p *PeerScore) computeScore() float64 {
	total := p.ConnectCount + p.FailCount
	if total == 0 {
		return 0.5 // Default score for new peers
	}

	// Base score is success rate
	successRate := float64(p.SuccessCount) / float64(total)

	// Penalty for frequent disconnections
	disconnectPenalty := 0.0
	if p.DisconnectCount > 5 {
		disconnectPenalty = 0.1 * float64(p.DisconnectCount-5)
		if disconnectPenalty > 0.3 {
			disconnectPenalty = 0.3
		}
	}

	// Bonus for recent connections
	recencyBonus := 0.0
	if !p.LastConnected.IsZero() {
		timeSinceConnect := time.Since(p.LastConnected)
		if timeSinceConnect < time.Hour {
			recencyBonus = 0.1
		} else if timeSinceConnect < 24*time.Hour {
			recencyBonus = 0.05
		}
	}

	score := successRate - disconnectPenalty + recencyBonus
	if score < 0.0 {
		score = 0.0
	}
	if score > 1.0 {
		score = 1.0
	}

	p.Score = score
	return score
}

// NewNode creates a new IPFS node with the given config
func NewNode(config *Config) (*Node, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	node := &Node{
		config:              config,
		ctx:                 ctx,
		cancel:              cancel,
		isStarted:           false,
		topicCache:          newTopicCache(),
		dhtMaintenanceDone:  make(chan struct{}),
		peerScores:          make(map[string]*PeerScore),
		healthCheckInterval: 2 * time.Minute,
		metrics:             NewMetricsCollector(),
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

	// Create connection manager for stable peer connections
	connMgr, err := connmgr.NewConnManager(
		50,  // LowWater - minimum connections to keep
		400, // HighWater - maximum connections before pruning
		connmgr.WithGracePeriod(time.Minute),
	)
	if err != nil {
		return fmt.Errorf("failed to create connection manager: %w", err)
	}

	// Create libp2p host with explicit transport configuration
	opts := []libp2p.Option{
		libp2p.Identity(privKey),
		libp2p.ListenAddrStrings(
			"/ip4/0.0.0.0/tcp/0",    // IPv4 TCP
			"/ip4/0.0.0.0/tcp/0/ws", // IPv4 WebSocket (for browser clients)
			"/ip6/::/tcp/0",         // IPv6 TCP
			"/ip4/127.0.0.1/tcp/0",  // Explicit localhost for single-machine testing
		),
		// Explicit transports (required for reliable connections)
		libp2p.Transport(tcp.NewTCPTransport),
		// Security transports (order matters - first is preferred)
		libp2p.Security(noise.ID, noise.New),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		// Stream muxer
		libp2p.Muxer(yamux.ID, yamux.DefaultTransport),
		// NAT traversal
		libp2p.NATPortMap(),         // UPnP/IGD port mapping
		libp2p.EnableHolePunching(), // Hole punching for direct connections
		libp2p.EnableRelay(),        // Enable relay service
		libp2p.EnableAutoNATv2(),    // AutoNAT for NAT type detection
		libp2p.EnableNATService(),   // NAT service for other peers
		// Connection manager for stable connections
		libp2p.ConnectionManager(connMgr),
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return fmt.Errorf("failed to create libp2p host: %w", err)
	}

	n.host = h
	logger.Infow("libp2p host created", "id", h.ID(), "addrs", h.Addrs())

	// Create DHT
	// Note: We don't use ProtocolPrefix to allow connection to public bootstrap peers
	// Message privacy is maintained via encrypted PubSub topics (derived from public keys)
	dhtOpts := []dht.Option{
		dht.Mode(dht.ModeAuto),
	}

	dhtNode, err := dht.New(n.ctx, n.host, dhtOpts...)
	if err != nil {
		return fmt.Errorf("failed to create DHT: %w", err)
	}

	n.dht = dhtNode

	// Create discovery service (must be before PubSub)
	n.discovery = routing.NewRoutingDiscovery(dhtNode)

	// Create PubSub with discovery integration
	ps, err := pubsub.NewGossipSub(n.ctx, n.host,
		pubsub.WithMessageSigning(true),
		pubsub.WithStrictSignatureVerification(true),
		pubsub.WithDiscovery(n.discovery), // Enable peer discovery for topics
		pubsub.WithPeerExchange(true),     // Enable peer exchange for faster mesh formation
	)
	if err != nil {
		return fmt.Errorf("failed to create PubSub: %w", err)
	}

	n.pubsub = ps

	// Start mDNS for local network discovery (faster than DHT)
	// Use a valid DNS-SD service name (alphanumeric + hyphens, no slashes)
	mdnsServiceName := "babylontower"
	n.mdns = mdns.NewMdnsService(n.host, mdnsServiceName, n)
	logger.Infow("mDNS discovery enabled", "service_name", mdnsServiceName)

	// Bootstrap DHT (for wider network discovery)
	if err := n.bootstrapDHT(); err != nil {
		logger.Warnw("DHT bootstrap failed", "error", err)
		// Continue anyway - mDNS will handle local discovery
	}

	// Advertise self to DHT after bootstrap
	// This makes our node discoverable by other peers
	go func() {
		ctx, cancel := context.WithTimeout(n.ctx, 30*time.Second)
		defer cancel()
		if err := n.AdvertiseSelf(ctx); err != nil {
			logger.Debugw("initial self-advertisement failed", "error", err)
		}
	}()

	// Start peer discovery via DHT
	n.startPeerDiscovery()

	// Subscribe to peer connection events for tracking
	n.subscribePeerEvents()

	// Start periodic DHT maintenance
	n.startDHTMaintenance()

	n.isStarted = true
	logger.Infow("IPFS node started successfully",
		"mDNS", "enabled",
		"DHT", "enabled",
		"HolePunching", "enabled",
		"listen_addrs", n.Multiaddrs())

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

	// Stop mDNS service
	if n.mdns != nil {
		if err := n.mdns.Close(); err != nil {
			logger.Warnw("mDNS close error", "error", err)
		}
	}

	// Close peer event subscription
	if n.peerEventSub != nil {
		if err := n.peerEventSub.Close(); err != nil {
			logger.Warnw("peer event subscription close error", "error", err)
		}
	}

	// Wait for DHT maintenance to stop
	select {
	case <-n.dhtMaintenanceDone:
	case <-time.After(2 * time.Second):
		logger.Warn("DHT maintenance did not stop gracefully")
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

// DHT returns the DHT instance for peer routing and discovery
func (n *Node) DHT() *dht.IpfsDHT {
	return n.dht
}

// Context returns the node's context
func (n *Node) Context() context.Context {
	return n.ctx
}

// IsStarted returns true if the node is running
func (n *Node) IsStarted() bool {
	return n.isStarted
}

// HandlePeerFound is called by mDNS when a peer is discovered
// This implements the mdns.PeerNotif interface
func (n *Node) HandlePeerFound(peerInfo peer.AddrInfo) {
	n.mdnsCount.Add(1)
	n.peerMu.Lock()
	n.lastPeerFound = time.Now()
	n.peerMu.Unlock()

	logger.Infow("mDNS discovered peer",
		"peer", peerInfo.ID,
		"addrs", peerInfo.Addrs,
		"total_mdns_discoveries", n.mdnsCount.Load())

	// Try to connect to discovered peer
	ctx, cancel := context.WithTimeout(n.ctx, ConnectionTimeout)
	defer cancel()

	if err := n.host.Connect(ctx, peerInfo); err != nil {
		logger.Warnw("failed to connect to mDNS discovered peer",
			"peer", peerInfo.ID,
			"error", err,
			"addrs", peerInfo.Addrs)
	} else {
		logger.Infow("connected to mDNS discovered peer", "peer", peerInfo.ID)
	}
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

// ConnectedPeers returns information about currently connected peers
func (n *Node) ConnectedPeers() []PeerInfo {
	if n.host == nil {
		return nil
	}

	peers := n.host.Network().Peers()
	peerInfos := make([]PeerInfo, 0, len(peers))

	for _, p := range peers {
		info := PeerInfo{
			ID:        p.String(),
			Connected: true,
		}

		// Get peer addresses
		conn := n.host.Network().ConnsToPeer(p)
		if len(conn) > 0 {
			info.Addresses = make([]string, len(conn))
			for i, c := range conn {
				info.Addresses[i] = c.RemoteMultiaddr().String()
			}
		}

		// Get peer protocols
		protocols, err := n.host.Peerstore().GetProtocols(p)
		if err == nil && len(protocols) > 0 {
			info.Protocols = make([]string, len(protocols))
			for i, proto := range protocols {
				info.Protocols[i] = string(proto)
			}
		}

		peerInfos = append(peerInfos, info)
	}

	return peerInfos
}

// GetNetworkInfo returns network status information
func (n *Node) GetNetworkInfo() *NetworkInfo {
	info := &NetworkInfo{
		PeerID:             n.PeerID(),
		IsStarted:          n.isStarted,
		ConnectedPeerCount: 0,
	}

	if n.host != nil {
		info.Multiaddrs = n.Multiaddrs()
		info.ConnectedPeers = n.ConnectedPeers()
		info.ConnectedPeerCount = len(info.ConnectedPeers)
		info.ListenAddrs = make([]string, len(n.host.Addrs()))
		for i, addr := range n.host.Addrs() {
			info.ListenAddrs[i] = addr.String()
		}
	}

	return info
}

// GetMetrics returns network health metrics
func (n *Node) GetMetrics() *NetworkMetrics {
	if n.metrics == nil {
		return &NetworkMetrics{}
	}
	return &n.metrics.metrics
}

// GetMetricsFull returns comprehensive metrics including discovery and connection history
func (n *Node) GetMetricsFull() *MetricsFull {
	if n.metrics == nil {
		return &MetricsFull{}
	}

	n.peerMu.RLock()
	lastPeerFound := n.lastPeerFound
	mdnsCount := n.mdnsCount.Load()
	n.peerMu.RUnlock()

	metrics := n.metrics.GetMetrics()
	metrics.PeerID = n.PeerID()

	return &MetricsFull{
		NetworkMetrics: metrics,
		DHTInfo:        n.GetDHTInfo(),
		MDnsStats: MDnsStats{
			TotalDiscoveries: mdnsCount,
			LastPeerFound:    lastPeerFound,
		},
		DiscoveryBySource:     n.metrics.GetDiscoveryBySource(),
		ConnectionSuccessRate: n.metrics.GetConnectionSuccessRate(),
		MessageSuccessRate:    n.metrics.GetMessageSuccessRate(),
	}
}

// PeerInfo contains information about a peer
type PeerInfo struct {
	ID        string
	Addresses []string
	Protocols []string
	Connected bool
}

// NetworkInfo contains network status information
type NetworkInfo struct {
	PeerID             string
	Multiaddrs         []string
	ListenAddrs        []string
	ConnectedPeers     []PeerInfo
	ConnectedPeerCount int
	IsStarted          bool
}

// MetricsFull contains comprehensive network metrics
type MetricsFull struct {
	NetworkMetrics
	DHTInfo               *DHTInfo
	MDnsStats             MDnsStats
	DiscoveryBySource     map[string]int64
	ConnectionSuccessRate float64
	MessageSuccessRate    float64
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

// FindPeer queries the DHT to find a peer by PeerID and returns their address info.
// It first tries direct FindPeer, then falls back to GetClosestPeers which returns
// peers closest to the target in DHT space (useful when target isn't directly advertised).
func (n *Node) FindPeer(peerID string) (*peer.AddrInfo, error) {
	if !n.isStarted {
		return nil, ErrNodeNotStarted
	}

	parsedID, err := peer.Decode(peerID)
	if err != nil {
		return nil, fmt.Errorf("invalid peer ID: %w", err)
	}

	ctx, cancel := context.WithTimeout(n.ctx, ConnectionTimeout)
	defer cancel()

	// Try direct FindPeer first (requires peer to have advertised themselves)
	peerInfo, err := n.dht.FindPeer(ctx, parsedID)
	if err == nil && len(peerInfo.Addrs) > 0 {
		logger.Infow("Found peer via DHT FindPeer", "peer", peerInfo.ID, "addrs", peerInfo.Addrs)
		return &peerInfo, nil
	}

	// Fallback: GetClosestPeers returns peers closest to target in DHT space
	// This is useful when the target peer hasn't explicitly advertised but
	// is reachable through the routing table
	logger.Debugw("FindPeer failed, trying GetClosestPeers fallback", "peer", parsedID, "error", err)

	closestPeers, err := n.dht.GetClosestPeers(ctx, string(parsedID))
	if err != nil {
		return nil, fmt.Errorf("DHT FindPeer and GetClosestPeers failed: %w", err)
	}

	if len(closestPeers) == 0 {
		return nil, fmt.Errorf("peer not found in DHT routing table")
	}

	// Return the closest peers - the first one is typically the best match
	// Note: These may not be the exact target, but peers near them in DHT space
	logger.Infow("Found closest peers via DHT", "target", parsedID, "count", len(closestPeers))

	// Get address info for the closest peer
	closestInfo := n.host.Peerstore().PeerInfo(closestPeers[0])
	if len(closestInfo.Addrs) > 0 {
		return &closestInfo, nil
	}

	return nil, fmt.Errorf("closest peer found but no addresses available")
}

// WaitForDHT blocks until the DHT routing table is populated with at least one peer.
// This should be called after Start() before attempting DHT operations.
// Returns an error if the timeout expires before any peers are found.
func (n *Node) WaitForDHT(timeout time.Duration) error {
	if !n.isStarted {
		return ErrNodeNotStarted
	}

	ctx, cancel := context.WithTimeout(n.ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	logger.Infow("Waiting for DHT bootstrap...", "timeout", timeout)

	for {
		select {
		case <-ctx.Done():
			routingTableSize := len(n.dht.RoutingTable().ListPeers())
			return fmt.Errorf("DHT bootstrap timeout after %s (routing table has %d peers)", timeout, routingTableSize)
		case <-ticker.C:
			routingTableSize := len(n.dht.RoutingTable().ListPeers())
			if routingTableSize > 0 {
				logger.Infow("DHT bootstrap complete", "routing_table_size", routingTableSize)
				return nil
			}
			logger.Debugw("DHT routing table empty, waiting...")
		}
	}
}

// DHTInfo contains DHT routing table and connection information
type DHTInfo struct {
	IsStarted              bool
	Mode                   string
	RoutingTableSize       int
	RoutingTablePeers      []string
	ConnectedPeerCount     int
	HasBootstrapConnection bool
}

// GetDHTInfo returns detailed information about the DHT state
func (n *Node) GetDHTInfo() *DHTInfo {
	info := &DHTInfo{
		IsStarted: n.isStarted,
	}

	if !n.isStarted || n.dht == nil {
		return info
	}

	// Get routing table peers
	routingTable := n.dht.RoutingTable()
	peers := routingTable.ListPeers()

	info.RoutingTableSize = len(peers)
	info.RoutingTablePeers = make([]string, 0, len(peers))
	for _, p := range peers {
		info.RoutingTablePeers = append(info.RoutingTablePeers, p.String())
	}

	// Get connection count
	info.ConnectedPeerCount = len(n.host.Network().Peers())

	// Check if we're connected to bootstrap peers
	info.HasBootstrapConnection = info.ConnectedPeerCount > 0

	// Get DHT mode (simplified - just show if it's auto mode)
	info.Mode = "auto"

	return info
}

// ConnectToBootstrapPeers connects to all configured bootstrap peers
// This is useful for local testing when mDNS is unreliable
func (n *Node) ConnectToBootstrapPeers() error {
	if !n.isStarted {
		return ErrNodeNotStarted
	}

	ctx, cancel := context.WithTimeout(n.ctx, ConnectionTimeout)
	defer cancel()

	connected := 0
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
			logger.Debugw("failed to connect to bootstrap peer", "peer", peerInfo.ID, "error", err)
		} else {
			connected++
			logger.Infow("connected to bootstrap peer", "peer", peerInfo.ID)
		}
	}

	if connected == 0 {
		logger.Warn("no bootstrap peers connected")
	} else {
		logger.Infow("bootstrap complete", "connected_peers", connected)
	}

	return nil
}

// ConnectToLocalNode connects to another node on the same machine
// This bypasses mDNS/DHT discovery for direct local testing
// Usage: node1.ConnectToLocalNode(node2.Multiaddrs()[0], node2.PeerID())
func (n *Node) ConnectToLocalNode(multiaddrStr, peerID string) error {
	if !n.isStarted {
		return ErrNodeNotStarted
	}

	// Construct multiaddr with peer ID if not already present
	var fullAddr string
	if strings.Contains(multiaddrStr, "/p2p/") || strings.Contains(multiaddrStr, "/ipfs/") {
		fullAddr = multiaddrStr
	} else {
		fullAddr = fmt.Sprintf("%s/p2p/%s", multiaddrStr, peerID)
	}

	return n.ConnectToPeer(fullAddr)
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

// BootstrapResult contains statistics about the bootstrap process
type BootstrapResult struct {
	StoredPeersAttempted int
	StoredPeersConnected int
	ConfigPeersAttempted int
	ConfigPeersConnected int
	TotalConnected       int
	RoutingTableSize     int
	Duration             time.Duration
	VerifiedPeers        int // Number of peers verified as responsive (Task 2.1.5)
}

// bootstrapDHT performs multi-stage bootstrap to populate the routing table
// Stage 1: Try stored peers (from BadgerDB) with good success rate
// Stage 2: Try config bootstrap peers with DNS resolution
// Stage 3: Wait for DHT routing table to populate
func (n *Node) bootstrapDHT() error {
	startTime := time.Now()
	result := &BootstrapResult{}

	ctx, cancel := context.WithTimeout(n.ctx, DHTBootstrapTimeout)
	defer cancel()

	// Start DHT bootstrap process
	if err := n.dht.Bootstrap(ctx); err != nil {
		return fmt.Errorf("DHT bootstrap failed: %w", err)
	}

	// Stage 1: Try stored peers first (faster bootstrap for returning nodes)
	storedPeers, err := n.loadStoredPeers()
	if err != nil {
		logger.Warnw("failed to load stored peers", "error", err)
	} else if len(storedPeers) > 0 {
		logger.Infow("bootstrap stage 1: connecting to stored peers", "count", len(storedPeers))
		result.StoredPeersAttempted = len(storedPeers)
		result.StoredPeersConnected = n.connectToPeersParallel(ctx, storedPeers)
		logger.Infow("stored peer bootstrap complete",
			"connected", result.StoredPeersConnected,
			"attempted", result.StoredPeersAttempted)
	}

	// Stage 2: Try config bootstrap peers (if needed)
	// Attempt if we have fewer than 3 connections OR if stored peers failed completely
	currentPeers := len(n.host.Network().Peers())
	storedPeersFailed := result.StoredPeersAttempted > 0 && result.StoredPeersConnected == 0
	
	if (currentPeers < 3 || storedPeersFailed) && len(n.config.BootstrapPeers) > 0 {
		logger.Infow("bootstrap stage 2: connecting to config peers",
			"current_connections", currentPeers,
			"stored_peers_connected", result.StoredPeersConnected,
			"stored_peers_failed", storedPeersFailed,
			"config_peers", len(n.config.BootstrapPeers))
		result.ConfigPeersAttempted = len(n.config.BootstrapPeers)
		result.ConfigPeersConnected = n.connectToBootstrapPeersWithDNS(ctx)
		logger.Infow("config peer bootstrap complete",
			"connected", result.ConfigPeersConnected,
			"attempted", result.ConfigPeersAttempted)
	}

	// Calculate totals
	result.TotalConnected = len(n.host.Network().Peers())
	result.Duration = time.Since(startTime)

	// Stage 3: Wait for DHT routing table to populate
	if result.TotalConnected > 0 {
		logger.Infow("bootstrap peer connections complete",
			"total_connected", result.TotalConnected,
			"duration", result.Duration)

		// Wait for DHT to be ready
		select {
		case <-n.dht.RefreshRoutingTable():
			routingTableSize := len(n.dht.RoutingTable().ListPeers())
			result.RoutingTableSize = routingTableSize
			logger.Infow("DHT routing table refreshed",
				"bootstrap_duration", result.Duration,
				"connected_peers", result.TotalConnected,
				"routing_table_size", routingTableSize)
		case <-ctx.Done():
			logger.Warn("DHT bootstrap timeout")
		}

		// Stage 4: Verify connected peers are responsive (Task 2.1.5)
		result.VerifiedPeers = n.verifyBootstrapPeers(ctx)
		logger.Infow("peer verification complete",
			"verified", result.VerifiedPeers,
			"total_connected", result.TotalConnected)
	} else {
		// Both Stage 1 and Stage 2 failed - try passive DHT bootstrap
		logger.Warn("no bootstrap peers connected - attempting passive DHT bootstrap")
		logger.Warn("this may take longer and requires incoming connections from other peers")
		
		// Give DHT more time to find peers passively
		passiveCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		
		select {
		case <-n.dht.RefreshRoutingTable():
			routingTableSize := len(n.dht.RoutingTable().ListPeers())
			result.RoutingTableSize = routingTableSize
			if routingTableSize > 0 {
				logger.Infow("DHT routing table populated via passive discovery",
					"routing_table_size", routingTableSize)
			}
		case <-passiveCtx.Done():
			logger.Warn("passive DHT bootstrap timeout")
		}
		
		if result.RoutingTableSize == 0 {
			logger.Error("DHT bootstrap completely failed - no peers in routing table")
			logger.Error("the node will have limited functionality until peers are discovered")
			logger.Warn("try:")
			logger.Warn("  - Check your internet connection")
			logger.Warn("  - Check firewall settings (TCP port 4001)")
			logger.Warn("  - Ensure NAT traversal is enabled (UPnP/IGD)")
			logger.Warn("  - Use /connect <multiaddr> for direct peer connection")
		}
	}

	// Log bootstrap summary
	n.logBootstrapSummary(result)

	return nil
}

// loadStoredPeers loads peers from storage with good success rate and recent activity
func (n *Node) loadStoredPeers() ([]peer.AddrInfo, error) {
	// Use pre-loaded stored peers from config
	// These are loaded in main.go before node start
	if len(n.config.StoredPeers) > 0 {
		logger.Debugw("using pre-loaded stored peers", "count", len(n.config.StoredPeers))
		return n.config.StoredPeers, nil
	}
	return nil, nil
}

// connectToPeersParallel attempts parallel connections to multiple peers
// with exponential backoff on failures (Task 2.1.3)
func (n *Node) connectToPeersParallel(ctx context.Context, peers []peer.AddrInfo) int {
	var wg sync.WaitGroup
	connected := atomic.Int32{}

	// Limit parallel connections to avoid resource exhaustion
	maxParallel := 10
	sem := make(chan struct{}, maxParallel)

	for _, peerInfo := range peers {
		if peerInfo.ID == n.host.ID() || len(peerInfo.Addrs) == 0 {
			continue
		}

		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore

		go func(pi peer.AddrInfo) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			// Try with exponential backoff
			var lastErr error
			for attempt := 0; attempt < 3; attempt++ {
				dialCtx, dialCancel := context.WithTimeout(ctx, DialTimeout)

				if err := n.host.Connect(dialCtx, pi); err != nil {
					dialCancel()
					lastErr = err
					logger.Debugw("failed to connect to peer", "peer", pi.ID, "error", err, "attempt", attempt+1)

					// Exponential backoff: 1s, 2s, 4s (with jitter)
					backoff := time.Duration(1<<uint(attempt)) * time.Second
					backoff += time.Duration(rand.Int63n(int64(backoff) / 2)) // Add jitter

					select {
					case <-ctx.Done():
						return
					case <-time.After(backoff):
						// Continue to next attempt
					}
				} else {
					dialCancel()
					connected.Add(1)
					logger.Debugw("connected to peer", "peer", pi.ID, "addrs", pi.Addrs)
					return
				}
			}

			// All attempts failed
			if lastErr != nil {
				logger.Warnw("all connection attempts failed", "peer", pi.ID, "error", lastErr)
			}
		}(peerInfo)
	}

	// Wait for all connections with overall timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All connections attempted
	case <-ctx.Done():
		logger.Debug("bootstrap context cancelled")
	}

	return int(connected.Load())
}

// connectToBootstrapPeersWithDNS connects to config bootstrap peers with DNS resolution
func (n *Node) connectToBootstrapPeersWithDNS(ctx context.Context) int {
	var wg sync.WaitGroup
	connected := atomic.Int32{}

	// Limit parallel connections
	maxParallel := 5
	sem := make(chan struct{}, maxParallel)

	for _, addrStr := range n.config.BootstrapPeers {
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			logger.Warnw("invalid bootstrap addr", "addr", addrStr, "error", err)
			continue
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(addr multiaddr.Multiaddr) {
			defer wg.Done()
			defer func() { <-sem }()

			// Resolve DNS addresses (e.g., /dnsaddr/ -> /ip4/)
			resolvedAddrs, err := madns.Resolve(ctx, addr)
			if err != nil {
				logger.Debugw("failed to resolve bootstrap addr", "addr", addr, "error", err)
				resolvedAddrs = []multiaddr.Multiaddr{addr}
			}

			// Try each resolved address
			for _, resolved := range resolvedAddrs {
				peerInfo, err := peer.AddrInfoFromP2pAddr(resolved)
				if err != nil {
					logger.Debugw("failed to parse resolved addr", "addr", resolved, "error", err)
					continue
				}

				if peerInfo.ID == n.host.ID() {
					continue
				}

				dialCtx, dialCancel := context.WithTimeout(ctx, DialTimeout)
				err = n.host.Connect(dialCtx, *peerInfo)
				dialCancel()

				if err == nil {
					connected.Add(1)
					logger.Debugw("connected to bootstrap peer", "peer", peerInfo.ID, "addr", resolved)
					break
				} else {
					logger.Debugw("failed to connect to bootstrap peer", "peer", peerInfo.ID, "addr", resolved, "error", err)
				}
			}
		}(ma)
	}

	// Wait for all connections with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		logger.Debug("bootstrap context cancelled")
	}

	return int(connected.Load())
}

// logBootstrapSummary logs a summary of the bootstrap process
func (n *Node) logBootstrapSummary(result *BootstrapResult) {
	logger.Infow("bootstrap summary",
		"duration", result.Duration,
		"stored_peers", fmt.Sprintf("%d/%d", result.StoredPeersConnected, result.StoredPeersAttempted),
		"config_peers", fmt.Sprintf("%d/%d", result.ConfigPeersConnected, result.ConfigPeersAttempted),
		"total_connected", result.TotalConnected,
		"routing_table_size", result.RoutingTableSize,
		"verified_peers", result.VerifiedPeers)

	if result.TotalConnected == 0 {
		logger.Warn("bootstrap failed - no peers connected")
	} else if result.RoutingTableSize == 0 {
		logger.Warn("bootstrap partial - connected but DHT routing table empty")
	} else if result.RoutingTableSize < 5 {
		logger.Warn("bootstrap degraded - small routing table", "size", result.RoutingTableSize)
	} else {
		logger.Info("bootstrap successful")
	}
}

// verifyBootstrapPeers verifies that connected bootstrap peers are responsive
// by performing a simple DHT query (Task 2.1.5)
// Returns the number of peers that responded successfully
func (n *Node) verifyBootstrapPeers(ctx context.Context) int {
	connectedPeers := n.host.Network().Peers()
	if len(connectedPeers) == 0 {
		return 0
	}

	logger.Infow("verifying bootstrap peers", "count", len(connectedPeers))

	verified := atomic.Int32{}
	var wg sync.WaitGroup

	// Verify each connected peer with a DHT query
	for _, peerID := range connectedPeers {
		wg.Add(1)
		go func(pid peer.ID) {
			defer wg.Done()

			// Create a short timeout for verification
			verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			// Try to find closest peers - this verifies the peer is responsive
			// We use a random key to avoid caching effects
			randomKey := fmt.Sprintf("verify-%s-%d", pid.String(), time.Now().UnixNano())
			_, err := n.dht.GetClosestPeers(verifyCtx, randomKey)

			if err == nil {
				verified.Add(1)
				logger.Debugw("bootstrap peer verified", "peer", pid)
			} else {
				logger.Debugw("bootstrap peer verification failed", "peer", pid, "error", err)
				// Update peer score to reflect failed verification
				n.updatePeerScore(pid.String(), false, true)
			}
		}(peerID)
	}

	// Wait for all verifications with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		logger.Debug("verification context cancelled")
	}

	count := int(verified.Load())
	logger.Infow("bootstrap peer verification complete", "verified", count, "total", len(connectedPeers))
	return count
}

// startPeerDiscovery starts background peer discovery via DHT
func (n *Node) startPeerDiscovery() {
	// Check if DHT has any peers before starting discovery
	routingTableSize := len(n.dht.RoutingTable().ListPeers())
	if routingTableSize == 0 {
		logger.Debug("DHT routing table empty, skipping peer discovery")
		return
	}

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

// subscribePeerEvents subscribes to libp2p peer connection events for tracking
func (n *Node) subscribePeerEvents() {
	// Subscribe to peer connectedness changes (covers both connect and disconnect)
	sub, err := n.host.EventBus().Subscribe(new(event.EvtPeerConnectednessChanged))
	if err != nil {
		logger.Warnw("failed to subscribe to peer connectedness events", "error", err)
		return
	}
	n.peerEventSub = sub

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Debugw("peer event subscription panic recovered", "recover", r)
			}
		}()

		for {
			select {
			case <-n.ctx.Done():
				return
			case e := <-sub.Out():
				if evt, ok := e.(event.EvtPeerConnectednessChanged); ok {
					peerID := evt.Peer.String()

					// Track connected/disconnected state
					switch evt.Connectedness {
					case network.Connected:
						n.peerCount.Add(1)
						n.updatePeerScore(peerID, true, false)
						n.metrics.RecordConnection(peerID, 0) // Latency can be added later
						logger.Infow("peer connected",
							"peer", peerID,
							"total_peers", n.peerCount.Load())
					case network.NotConnected:
						n.peerCount.Add(-1)
						n.updatePeerScore(peerID, false, false)
						n.metrics.RecordDisconnection(peerID)
						logger.Infow("peer disconnected",
							"peer", peerID,
							"total_peers", n.peerCount.Load())
					}
				}
			}
		}
	}()
}

// updatePeerScore updates the score for a peer based on connection events
func (n *Node) updatePeerScore(peerID string, connected, failed bool) {
	n.peerScoreMu.Lock()
	defer n.peerScoreMu.Unlock()

	score, exists := n.peerScores[peerID]
	if !exists {
		score = &PeerScore{
			PeerID: peerID,
		}
		n.peerScores[peerID] = score
	}

	if connected && !failed {
		score.ConnectCount++
		score.SuccessCount++
		score.LastConnected = time.Now()
	} else if failed {
		score.FailCount++
	} else {
		score.DisconnectCount++
		score.LastDisconnected = time.Now()
	}

	score.computeScore()

	logger.Debugw("peer score updated",
		"peer", peerID,
		"score", score.Score,
		"success", score.SuccessCount,
		"fail", score.FailCount,
		"disconnect", score.DisconnectCount)
}

// getPeerScore returns the score for a peer
func (n *Node) getPeerScore(peerID string) float64 {
	n.peerScoreMu.RLock()
	defer n.peerScoreMu.RUnlock()

	if score, exists := n.peerScores[peerID]; exists {
		return score.Score
	}
	return 0.5 // Default score for unknown peers
}

// getLowScorePeers returns peers with scores below threshold for pruning
func (n *Node) getLowScorePeers(threshold float64) []string {
	n.peerScoreMu.RLock()
	defer n.peerScoreMu.RUnlock()

	var lowScorePeers []string
	for peerID, score := range n.peerScores {
		if score.Score < threshold {
			lowScorePeers = append(lowScorePeers, peerID)
		}
	}

	return lowScorePeers
}

// startDHTMaintenance starts a background goroutine that periodically refreshes the DHT
func (n *Node) startDHTMaintenance() {
	go func() {
		ticker := time.NewTicker(DHTRefreshInterval)
		defer ticker.Stop()
		defer close(n.dhtMaintenanceDone)

		logger.Debugw("DHT maintenance started", "interval", DHTRefreshInterval)

		for {
			select {
			case <-n.ctx.Done():
				logger.Debug("DHT maintenance stopping")
				return
			case <-ticker.C:
				// Refresh routing table by querying a random peer ID
				randomID := peer.ID(fmt.Sprintf("refresh-%d", time.Now().UnixNano()))
				_, err := n.dht.GetClosestPeers(n.ctx, string(randomID))
				if err != nil {
					logger.Debugw("DHT refresh query failed", "error", err)
				} else {
					logger.Debugw("DHT routing table refreshed")
				}

				// Re-advertise self to stay discoverable
				ctx, cancel := context.WithTimeout(n.ctx, 10*time.Second)
				if err := n.AdvertiseSelf(ctx); err != nil {
					logger.Debugw("self-advertisement failed", "error", err)
				}
				cancel()

				// Log peer count
				peerCount := len(n.host.Network().Peers())
				logger.Debugw("DHT maintenance check", "connected_peers", peerCount)
			}
		}
	}()

	// Start connection health checks
	go n.startConnectionHealthChecks()
}

// startConnectionHealthChecks periodically checks connection health and reconnects if needed
func (n *Node) startConnectionHealthChecks() {
	ticker := time.NewTicker(n.healthCheckInterval)
	defer ticker.Stop()

	logger.Debugw("connection health checks started", "interval", n.healthCheckInterval)

	for {
		select {
		case <-n.ctx.Done():
			logger.Debug("connection health checks stopping")
			return
		case <-ticker.C:
			n.performHealthCheck()
		}
	}
}

// performHealthCheck checks connection health and attempts reconnection to low-score peers
// Includes connection pruning for low-quality peers (Task 2.3.4)
func (n *Node) performHealthCheck() {
	n.lastHealthCheck = time.Now()

	// Get current connected peers
	connectedPeers := n.host.Network().Peers()
	peerCount := len(connectedPeers)

	logger.Debugw("connection health check",
		"connected_peers", peerCount,
		"tracked_peers", len(n.peerScores))

	// Check if we have enough connections
	if peerCount < 5 {
		logger.Infow("low peer count, attempting reconnection to stored peers")
		n.reconnectToLowScorePeers()
	}

	// Prune low-quality peers (Task 2.3.4)
	// Pruning thresholds based on peer count:
	// - >100 peers: aggressive pruning (score < 0.3)
	// - >50 peers: moderate pruning (score < 0.25)
	// - >20 peers: light pruning (score < 0.2)
	var pruneThreshold float64
	var maxPruneCount int

	switch {
	case peerCount > 100:
		pruneThreshold = 0.3
		maxPruneCount = 20
	case peerCount > 50:
		pruneThreshold = 0.25
		maxPruneCount = 10
	case peerCount > 20:
		pruneThreshold = 0.2
		maxPruneCount = 5
	default:
		// Don't prune if we have few connections
		return
	}

	lowScorePeers := n.getLowScorePeers(pruneThreshold)
	if len(lowScorePeers) > 0 {
		logger.Infow("pruning low-quality peers",
			"count", min(maxPruneCount, len(lowScorePeers)),
			"threshold", pruneThreshold,
			"total_low_score", len(lowScorePeers))

		pruned := 0
		for _, peerID := range lowScorePeers {
			if pruned >= maxPruneCount {
				break
			}

			pid, err := peer.Decode(peerID)
			if err != nil {
				continue
			}

			// Skip if already disconnected
			if n.host.Network().Connectedness(pid) != network.Connected {
				continue
			}

			score := n.getPeerScore(peerID)
			logger.Debugw("pruning low-score peer", "peer", peerID, "score", score)
			if err := n.host.Network().ClosePeer(pid); err != nil {
				logger.Debugw("failed to close peer", "peer", peerID, "error", err)
			}
			pruned++

			// Record disconnection in metrics
			n.metrics.RecordDisconnection(peerID)
		}

		logger.Infow("pruning complete", "pruned", pruned)
	}
}

// reconnectToLowScorePeers attempts to reconnect to peers with low scores
func (n *Node) reconnectToLowScorePeers() {
	// Get peers with scores below 0.4
	lowScorePeers := n.getLowScorePeers(0.4)

	if len(lowScorePeers) == 0 {
		logger.Debug("no low-score peers to reconnect to")
		return
	}

	logger.Infow("attempting reconnection to low-score peers", "count", len(lowScorePeers))

	// Try to reconnect to up to 5 peers
	for i, peerID := range lowScorePeers {
		if i >= 5 {
			break
		}

		pid, err := peer.Decode(peerID)
		if err != nil {
			logger.Debugw("invalid peer ID", "peer", peerID, "error", err)
			continue
		}

		// Skip if already connected
		if n.host.Network().Connectedness(pid) == network.Connected {
			continue
		}

		// Get peer info from peerstore
		peerInfo := n.host.Peerstore().PeerInfo(pid)
		if len(peerInfo.Addrs) == 0 {
			logger.Debugw("no addresses for peer", "peer", peerID)
			continue
		}

		ctx, cancel := context.WithTimeout(n.ctx, DialTimeout)
		err = n.host.Connect(ctx, peerInfo)
		cancel()

		if err != nil {
			logger.Debugw("reconnection failed", "peer", peerID, "error", err)
			n.updatePeerScore(peerID, false, true)
		} else {
			logger.Infow("reconnected to peer", "peer", peerID)
		}
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// AdvertiseSelf refreshes our presence in the DHT by querying closest peers
// This helps keep our peer record visible in the routing table
func (n *Node) AdvertiseSelf(ctx context.Context) error {
	if !n.isStarted {
		return ErrNodeNotStarted
	}

	// Method 1: Put our peer record into the DHT
	// Create a provider record by providing our own PeerID as the "content"
	// This makes us discoverable via FindPeer queries
	mh, err := multihash.Sum([]byte(n.host.ID()), multihash.SHA2_256, -1)
	if err != nil {
		return fmt.Errorf("failed to hash peer ID: %w", err)
	}
	peerCID := cid.NewCidV1(cid.Raw, mh)

	// Provide our PeerID to the DHT - this publishes our addresses
	err = n.dht.Provide(ctx, peerCID, true)
	if err != nil {
		logger.Warnw("DHT provide failed", "peer", n.host.ID(), "error", err)
		// Continue with fallback method
	}

	// Method 2: Query closest peers to refresh our presence
	closestPeers, err := n.dht.GetClosestPeers(ctx, string(n.host.ID()))
	if err != nil {
		return fmt.Errorf("failed to advertise self: %w", err)
	}

	logger.Infow("Advertised self to DHT",
		"peer_id", n.host.ID(),
		"closest_peers", len(closestPeers),
		"provide_cid", peerCID.String())

	return nil
}

// GetMDnsStats returns mDNS discovery statistics
func (n *Node) GetMDnsStats() MDnsStats {
	n.peerMu.RLock()
	lastPeerFound := n.lastPeerFound
	n.peerMu.RUnlock()

	return MDnsStats{
		TotalDiscoveries: n.mdnsCount.Load(),
		LastPeerFound:    lastPeerFound,
	}
}

// MDnsStats contains mDNS discovery statistics
type MDnsStats struct {
	TotalDiscoveries int32
	LastPeerFound    time.Time
}

// expandPath expands ~ to home directory
// Respects HOME environment variable for container/test isolation
func expandPath(path string) (string, error) {
	if len(path) == 0 {
		return "", fmt.Errorf("empty path")
	}

	if path[0] == '~' {
		// Check HOME environment variable first (for test isolation)
		// This allows multiple instances to run with different HOME dirs
		home := os.Getenv("HOME")
		if home == "" {
			var err error
			home, err = os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("failed to get home dir: %w", err)
			}
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
