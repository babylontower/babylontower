package ipfsnode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	bterrors "babylontower/pkg/errors"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-record"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
)

// NewNode creates a new IPFS node with the given config
func NewNode(config *Config) (*Node, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	node := &Node{
		config:               config,
		ctx:                  ctx,
		cancel:               cancel,
		topicCache:           newTopicCache(),
		dhtMaintenanceDone:   make(chan struct{}),
		ipfsBootstrapDone:    make(chan struct{}),
		babylonBootstrapDone: make(chan struct{}),
		peerScores:           make(map[string]*PeerScore),
		healthCheckInterval:  2 * time.Minute,
		metrics:              NewMetricsCollector(),
		startTime:            time.Time{}, // Will be set on Start()
	}

	return node, nil
}

// Start initializes and starts the IPFS node with decoupled bootstrap architecture.
// The startup is split into phases to ensure CLI responsiveness:
//
// Phase 1: Initialize libp2p host, DHT, PubSub (NON-BLOCKING)
// Phase 2: Bootstrap IPFS DHT asynchronously (transport layer only)
// Phase 3: Advertise on DHT rendezvous namespace for Babylon peer discovery
// Phase 4: Bootstrap Babylon DHT asynchronously (protocol layer via rendezvous)
//
// This architecture allows the node to:
// - Be responsive immediately after Phase 1
// - Discover other Babylon nodes via DHT rendezvous (O(log N) scalable)
// - Trigger Babylon bootstrap lazily on first connection
func (n *Node) Start() error {
	if n.isStarted.Load() {
		return nil
	}

	logger.Info("starting IPFS node with decoupled bootstrap architecture...")

	// ========== PHASE 1: Initialize libp2p host, DHT, PubSub ==========
	// This phase is NON-BLOCKING and completes quickly
	if err := n.initializeHost(); err != nil {
		return fmt.Errorf("phase 1 (host initialization) failed: %w", err)
	}

	logger.Infow("phase 1 complete: libp2p host initialized",
		"peer_id", n.host.ID(),
		"listen_addrs", len(n.host.Addrs()))

	// ========== PHASE 2: Bootstrap IPFS DHT asynchronously ==========
	// IPFS DHT is for transport layer only (PubSub connectivity)
	// Don't block - run in background
	bterrors.SafeGo("ipfs-dht-bootstrap", func() {
		if err := n.bootstrapIPFSDHT(); err != nil {
			logger.Warnw("phase 2 (IPFS DHT bootstrap) failed", "error", err)
			// Continue anyway - node can still function with limited connectivity
		}
	})

	// ========== PHASE 3: Start rendezvous advertise after IPFS bootstrap ==========
	// Advertise on DHT rendezvous namespace so other Babylon nodes can find us
	bterrors.SafeGo("rendezvous-advertise", func() {
		// Wait for IPFS DHT to have some peers before advertising
		if err := n.WaitForIPFSBootstrap(15 * time.Second); err != nil {
			logger.Warnw("IPFS bootstrap not ready for rendezvous, advertising anyway", "error", err)
		}
		n.startRendezvousAdvertise()
	})

	// ========== PHASE 4: Bootstrap Babylon DHT asynchronously ==========
	// Babylon DHT is for protocol layer (messaging, groups)
	// Uses DHT rendezvous to discover other Babylon nodes
	bterrors.SafeGo("babylon-dht-bootstrap", func() {
		if err := n.bootstrapBabylonDHT(); err != nil {
			if errors.Is(err, ErrBabylonBootstrapDeferred) {
				logger.Info("phase 4: Babylon bootstrap deferred - waiting for connections")
			} else {
				logger.Warnw("phase 4 (Babylon DHT bootstrap) failed", "error", err)
			}
		}
	})

	// ========== PHASE 5: Periodic rendezvous discovery ==========
	// Continuously search for Babylon peers via DHT rendezvous.
	// Runs every 30s while Babylon bootstrap is deferred, then every 5m.
	n.startRendezvousDiscovery()

	// ========== BACKGROUND: Start peer discovery and maintenance ==========
	// Start mDNS for local network discovery
	mdnsServiceName := "babylontower"
	n.mdns = mdns.NewMdnsService(n.host, mdnsServiceName, n)
	logger.Debugw("mDNS discovery enabled", "service_name", mdnsServiceName)

	// Advertise self to DHT after bootstrap
	bterrors.SafeGo("ipfs-self-advertise", func() {
		time.Sleep(2 * time.Second)
		ctx, cancel := context.WithTimeout(n.ctx, 30*time.Second)
		defer cancel()
		if err := n.AdvertiseSelf(ctx); err != nil {
			logger.Debugw("initial self-advertisement failed", "error", err)
		}
	})

	// Start peer discovery via DHT
	n.startPeerDiscovery()

	// Subscribe to peer connection events for tracking
	n.subscribePeerEvents()

	// Start periodic DHT maintenance
	n.startDHTMaintenance()

	// ========== NODE STARTED ==========
	n.isStarted.Store(true)
	n.startTime = time.Now()

	logger.Infow("IPFS node started successfully (decoupled bootstrap)",
		"peer_id", n.host.ID(),
		"mDNS", "enabled",
		"DHT", "enabled",
		"HolePunching", "enabled",
		"AutoRelay", "enabled",
		"QUIC", "enabled",
		"bootstrap_architecture", "decoupled",
		"listen_addrs", n.Multiaddrs())

	return nil
}

// initializeHost creates the libp2p host, DHT, and PubSub subsystems
// This is Phase 1 of the startup process
func (n *Node) initializeHost() error {
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
			"/ip4/0.0.0.0/tcp/0",         // IPv4 TCP
			"/ip4/0.0.0.0/tcp/0/ws",      // IPv4 WebSocket (for browser clients)
			"/ip6/::/tcp/0",              // IPv6 TCP
			"/ip4/127.0.0.1/tcp/0",       // Explicit localhost for single-machine testing
			"/ip4/0.0.0.0/udp/0/quic-v1", // QUIC for better NAT traversal
		),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Muxer(yamux.ID, yamux.DefaultTransport),
		libp2p.NATPortMap(),
		libp2p.EnableHolePunching(),
		libp2p.EnableRelay(),
		libp2p.EnableAutoNATv2(),
		libp2p.EnableNATService(),
		libp2p.ConnectionManager(connMgr),
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return fmt.Errorf("failed to create libp2p host: %w", err)
	}

	n.host = h
	logger.Debugw("libp2p host created", "id", h.ID(), "addrs", h.Addrs())

	// ========== Create IPFS DHT (Transport Layer) ==========
	// This DHT is for general IPFS connectivity and PubSub routing
	// It bootstraps to public IPFS bootstrap nodes
	dhtMode := parseDHTMode(n.config.DHTMode)

	// Parse non-DNS bootstrap peers for DHT internal use,
	// falling back to well-known IPFS defaults if config is empty
	bootstrapAddrs := n.config.BootstrapPeers
	if len(bootstrapAddrs) == 0 {
		bootstrapAddrs = DefaultConfig().BootstrapPeers
	}
	var dhtBootstrapPeers []peer.AddrInfo
	for _, addrStr := range bootstrapAddrs {
		if strings.HasPrefix(addrStr, "/dnsaddr/") {
			continue // DNS addresses handled by connectToBootstrapPeersWithDNS
		}
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			continue
		}
		dhtBootstrapPeers = append(dhtBootstrapPeers, *pi)
	}

	dhtOpts := []dht.Option{
		dht.Mode(dhtMode),
		dht.BootstrapPeers(dhtBootstrapPeers...),
	}

	dhtNode, err := dht.New(n.ctx, n.host, dhtOpts...)
	if err != nil {
		return fmt.Errorf("failed to create IPFS DHT: %w", err)
	}

	n.dht = dhtNode
	logger.Info("created IPFS DHT (transport layer)")

	// ========== Create Babylon DHT (Protocol Layer) ==========
	// This DHT is specifically for Babylon protocol operations.
	// It stores identity documents, prekeys, and other protocol data.
	// We use ProtocolPrefix to create a separate DHT network isolated from public IPFS.
	// Custom validators are registered for Babylon namespaces (/bt/id/, etc.).

	// Create custom validator for Babylon DHT.
	// NamespacedValidator looks up validators by the first path segment of the key
	// (e.g. key "/bt/id/abc" → SplitKey extracts namespace "bt").
	// Keys MUST be bare namespace names without slashes.
	babylonValidator := make(record.NamespacedValidator)
	babylonValidator["pk"] = &record.PublicKeyValidator{}
	babylonValidator["bt"] = NewBabylonNamespaceValidator()

	babylonDHTOpts := []dht.Option{
		dht.Mode(dhtMode),
		// No default bootstrap peers - will bootstrap via DHT rendezvous discovery
		dht.BootstrapPeers(),
		// Use /babylon protocol prefix to isolate from public IPFS DHT network
		// This creates a separate DHT network that only communicates with other
		// Babylon nodes using the same protocol prefix
		dht.ProtocolPrefix("/babylon"),
		// Set custom validator for Babylon namespaces
		dht.Validator(babylonValidator),
	}

	babylonDHTNode, err := dht.New(n.ctx, n.host, babylonDHTOpts...)
	if err != nil {
		return fmt.Errorf("failed to create Babylon DHT: %w", err)
	}

	n.babylonDHT = babylonDHTNode
	logger.Info("created Babylon DHT (protocol layer) with custom validator")

	logger.Infow("Babylon DHT validators configured",
		"protocol_prefix", "/babylon",
		"namespaces", []string{"pk", "bt"},
		"validators", []string{"PublicKeyValidator", "BabylonNamespaceValidator"})

	// Create discovery service (must be before PubSub)
	// Use IPFS DHT for general discovery
	n.discovery = routing.NewRoutingDiscovery(dhtNode)

	// Create PubSub with discovery integration
	// Note: Topic names use "babylon-*" prefix which is application-level naming.
	// PubSub message signing and verification ensure only authenticated peers participate.
	ps, err := pubsub.NewGossipSub(n.ctx, n.host,
		pubsub.WithMessageSigning(true),
		pubsub.WithStrictSignatureVerification(true),
		pubsub.WithDiscovery(n.discovery),
		pubsub.WithPeerExchange(true),
	)
	if err != nil {
		return fmt.Errorf("failed to create PubSub: %w", err)
	}

	n.pubsub = ps

	return nil
}

// Stop gracefully shuts down the IPFS node
func (n *Node) Stop() error {
	if !n.isStarted.Load() {
		return nil
	}

	logger.Info("Stopping IPFS node...")

	// Cancel context to stop all goroutines (including rendezvous advertise)
	n.cancel()
	n.rendezvousActive.Store(false)

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
		logger.Warnw("DHT maintenance did not stop gracefully", "timeout", "2s")
	}

	// Close DHTs
	if n.babylonDHT != nil {
		if err := n.babylonDHT.Close(); err != nil {
			logger.Warnw("Babylon DHT close error", "error", err)
		}
	}
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

	n.isStarted.Store(false)
	logger.Info("IPFS node stopped")

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

// parseDHTMode converts a string to dht.ModeOpt
func parseDHTMode(mode string) dht.ModeOpt {
	switch mode {
	case "server", "Server", "SERVER":
		return dht.ModeServer
	case "client", "Client", "CLIENT":
		return dht.ModeClient
	case "auto", "Auto", "AUTO", "":
		return dht.ModeAuto
	default:
		return dht.ModeAuto
	}
}

// expandPath expands ~ to home directory
func expandPath(path string) (string, error) {
	if len(path) == 0 {
		return "", errors.New("empty path")
	}

	if path[0] == '~' {
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

// GetStartTime returns the node start time for uptime calculation
func (n *Node) GetStartTime() time.Time {
	return n.startTime
}
