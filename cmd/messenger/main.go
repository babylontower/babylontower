package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"babylontower/pkg/cli"
	"babylontower/pkg/config"
	"babylontower/pkg/identity"
	"babylontower/pkg/ipfsnode"
	"babylontower/pkg/messaging"
	"babylontower/pkg/peerstore"
	"babylontower/pkg/storage"
	"github.com/ipfs/go-log/v2"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

var logger = log.Logger("babylontower")

// Version information injected at build time
var (
	Version   = "0.1.0-poc"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

const (
	// DefaultDataDir is the default directory for application data
	DefaultDataDir = ".babylontower"
	// IdentityFileName is the name of the identity file
	IdentityFileName = "identity.json"
	// StorageDirName is the name of the storage directory
	StorageDirName = "storage"
	// IPFSDirName is the name of the IPFS repo directory
	IPFSDirName = "ipfs"
)

func main() {
	// Check for help flag first
	for _, arg := range os.Args[1:] {
		if arg == "-h" || arg == "--help" {
			fmt.Println("Babylon Tower - Decentralized P2P Messenger")
			fmt.Println("")
			fmt.Println("Usage:")
			fmt.Println("  messenger [options]")
			fmt.Println("")
			fmt.Println("Options:")
			fmt.Println("  -data-dir <path>    Data directory for this instance")
			fmt.Println("                      Default: ~/.babylontower")
			fmt.Println("")
			fmt.Println("Running Multiple Instances:")
			fmt.Println("  To run two nodes on the same machine, use different data directories:")
			fmt.Println("    ./messenger -data-dir ~/.babylontower/node1")
			fmt.Println("    ./messenger -data-dir ~/.babylontower/node2")
			fmt.Println("")
			fmt.Println("Each instance will have:")
			fmt.Println("  - Unique peer identity (PeerID)")
			fmt.Println("  - Separate storage and contacts")
			fmt.Println("  - Dynamic port assignment (no port conflicts)")
			os.Exit(0)
		}
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Parse command-line flags
	dataDirFlag := flag.String("data-dir", "", "Data directory (default: ~/.babylontower)")
	configFlag := flag.String("config", "", "Config file path (optional, uses stored config or defaults)")
	flag.Parse()

	// Setup logging - use debug level for development
	if err := log.SetLogLevel("babylontower", "debug"); err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}

	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Setup data directory
	var dataDir string
	if *dataDirFlag != "" {
		dataDir = *dataDirFlag
	} else {
		dataDir = filepath.Join(homeDir, DefaultDataDir)
	}

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Setup paths
	identityPath := filepath.Join(dataDir, IdentityFileName)
	storageDir := filepath.Join(dataDir, StorageDirName)
	ipfsDir := filepath.Join(dataDir, IPFSDirName)
	configPath := filepath.Join(dataDir, "config.json")

	logger.Infow("starting Babylon Tower", "version", Version, "data_dir", dataDir)

	// Load or create identity
	ident, err := loadOrCreateIdentity(identityPath)
	if err != nil {
		return fmt.Errorf("failed to load identity: %w", err)
	}

	logger.Infow("identity loaded", "public_key", ident.PublicKeyHex())

	// Initialize storage
	store, err := storage.NewBadgerStorage(storage.Config{
		Path:     storageDir,
		InMemory: false,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Warnw("failed to close storage", "error", err)
		}
	}()

	logger.Info("storage initialized")

	// Load IPFS configuration
	ipfsConfig := loadIPFSConfig(store, configPath, *configFlag)

	// Validate config
	if err := ipfsConfig.Validate(); err != nil {
		logger.Warnw("config validation failed, using defaults", "error", err)
		ipfsConfig = config.DefaultIPFSConfig()
	}

	// Save config to storage for persistence
	if err := saveIPFSConfig(store, ipfsConfig); err != nil {
		logger.Warnw("failed to save config to storage", "error", err)
	}

	// Save config file for user reference
	if err := ipfsConfig.SaveToFile(configPath); err != nil {
		logger.Warnw("failed to save config file", "error", err)
	}

	logger.Infow("IPFS config loaded",
		"bootstrap_peers", len(ipfsConfig.BootstrapPeers),
		"max_stored_peers", ipfsConfig.MaxStoredPeers,
		"min_peer_connections", ipfsConfig.MinPeerConnections)

	// Load stored peers for faster bootstrap
	storedPeers, err := loadAndConnectStoredPeers(store, ipfsConfig)
	if err != nil {
		logger.Warnw("failed to load stored peers", "error", err)
	} else if len(storedPeers) > 0 {
		logger.Infow("will connect to stored peers", "count", len(storedPeers))
	}

	// Initialize IPFS node with config
	ipfsNode, err := ipfsnode.NewNode(&ipfsnode.Config{
		RepoDir:          ipfsDir,
		ProtocolID:       ipfsConfig.ProtocolID,
		BootstrapPeers:   ipfsConfig.BootstrapPeers,
		StoredPeers:      storedPeers,
		EnableRelay:      ipfsConfig.EnableRelay,
		EnableHolePunching: ipfsConfig.EnableHolePunching,
	})
	if err != nil {
		return fmt.Errorf("failed to create IPFS node: %w", err)
	}

	// Start IPFS node
	if err := ipfsNode.Start(); err != nil {
		return fmt.Errorf("failed to start IPFS node: %w", err)
	}
	defer func() {
		if err := ipfsNode.Stop(); err != nil {
			logger.Warnw("failed to stop IPFS node", "error", err)
		}
	}()

	// Wait for DHT bootstrap with configured timeout
	logger.Infow("waiting for DHT bootstrap", "timeout", ipfsConfig.DHTBootstrapTimeout)
	if err := ipfsNode.WaitForDHT(ipfsConfig.DHTBootstrapTimeout); err != nil {
		logger.Warnw("DHT bootstrap incomplete", "error", err, "action", "continuing with mDNS discovery")
	} else {
		logger.Info("DHT bootstrap complete")
	}

	// Store successfully connected bootstrap peers
	if err := storeConnectedPeers(store, ipfsNode); err != nil {
		logger.Warnw("failed to store connected peers", "error", err)
	}

	logger.Infow("IPFS node started",
		"peer_id", ipfsNode.PeerID(),
		"addresses", ipfsNode.Multiaddrs())

	// Initialize peer address book
	addrBook, err := peerstore.NewAddrBook(dataDir)
	if err != nil {
		logger.Warnw("failed to initialize address book", "error", err)
		// Continue without address book
	} else {
		logger.Infow("address book initialized", "peer_count", addrBook.Count())

		// Auto-connect to known peers
		if addrBook.Count() > 0 {
			logger.Info("attempting to connect to known peers...")
			ctx := ipfsNode.Context()
			if err := addrBook.ConnectToAll(ctx, ipfsNode); err != nil {
				logger.Warnw("auto-connect failed", "error", err)
			} else {
				logger.Info("auto-connect completed")
			}
		}
	}

	// Initialize messaging service
	msgConfig := &messaging.Config{
		OwnEd25519PrivKey: ident.Ed25519PrivKey,
		OwnEd25519PubKey:  ident.Ed25519PubKey,
		OwnX25519PrivKey:  ident.X25519PrivKey,
		OwnX25519PubKey:   ident.X25519PubKey,
	}

	msgService := messaging.NewService(msgConfig, store, ipfsNode)

	// Start messaging service
	if err := msgService.Start(); err != nil {
		return fmt.Errorf("failed to start messaging service: %w", err)
	}
	defer func() {
		if err := msgService.Stop(); err != nil {
			logger.Warnw("failed to stop messaging service", "error", err)
		}
	}()

	logger.Info("messaging service started")

	// Create CLI
	cliConfig := &cli.Config{
		Version:      Version,
		DataDir:      dataDir,
		IdentityPath: identityPath,
	}

	// Convert identity to CLI identity
	cliIdentity := &cli.Identity{
		Ed25519PubKey:  ident.Ed25519PubKey,
		Ed25519PrivKey: ident.Ed25519PrivKey,
		X25519PubKey:   ident.X25519PubKey,
		X25519PrivKey:  ident.X25519PrivKey,
	}

	app, err := cli.New(cliConfig, cliIdentity, store, ipfsNode, msgService)
	if err != nil {
		return fmt.Errorf("failed to create CLI: %w", err)
	}

	// Start CLI main loop
	if err := app.Start(); err != nil {
		return fmt.Errorf("CLI error: %w", err)
	}

	// Graceful shutdown
	if err := app.Stop(); err != nil {
		logger.Warnw("CLI stop error", "error", err)
	}

	logger.Info("Babylon Tower shutdown complete")

	return nil
}

// loadOrCreateIdentity loads an existing identity or creates a new one
func loadOrCreateIdentity(identityPath string) (*identity.Identity, error) {
	if identity.IdentityExists(identityPath) {
		// Load existing identity
		ident, err := identity.LoadIdentity(identityPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load identity: %w", err)
		}
		return ident, nil
	}

	// Generate new identity
	logger.Info("Generating new identity...")
	ident, err := identity.GenerateIdentity()
	if err != nil {
		return nil, fmt.Errorf("failed to generate identity: %w", err)
	}

	// Save identity
	if err := identity.SaveIdentity(ident, identityPath); err != nil {
		return nil, fmt.Errorf("failed to save identity: %w", err)
	}

	logger.Info("New identity generated and saved")
	fmt.Printf("\n🎉 New identity generated!\n")
	fmt.Printf("Your mnemonic (write this down safely):\n")
	fmt.Printf("  %s\n\n", ident.Mnemonic)
	fmt.Printf("WARNING: If you lose this mnemonic, you lose your identity!\n")
	fmt.Printf("Store it in a secure location.\n\n")

	return ident, nil
}

// loadIPFSConfig loads IPFS configuration from file, storage, or uses defaults
func loadIPFSConfig(store *storage.BadgerStorage, configPath, configFlag string) *config.IPFSConfig {
	// Priority: 1. Config flag, 2. Config file, 3. Storage, 4. Defaults

	// Try config flag first
	if configFlag != "" {
		cfg, err := config.LoadFromFile(configFlag)
		if err != nil {
			logger.Warnw("failed to load config from flag", "path", configFlag, "error", err)
		} else {
			logger.Infow("loaded config from flag", "path", configFlag)
			return cfg
		}
	}

	// Try config file
	if _, err := os.Stat(configPath); err == nil {
		cfg, err := config.LoadFromFile(configPath)
		if err != nil {
			logger.Warnw("failed to load config file", "path", configPath, "error", err)
		} else {
			logger.Infow("loaded config from file", "path", configPath)
			return cfg
		}
	}

	// Try storage
	value, err := store.GetConfig("ipfs_config")
	if err == nil && value != "" {
		cfg := config.DefaultIPFSConfig()
		if err := cfg.FromMap(map[string]string{"ipfs_config": value}); err != nil {
			logger.Warnw("failed to parse config from storage", "error", err)
		} else {
			logger.Info("loaded config from storage")
			return cfg
		}
	}

	// Use defaults
	logger.Info("using default IPFS configuration")
	return config.DefaultIPFSConfig()
}

// saveIPFSConfig saves the configuration to storage
func saveIPFSConfig(store *storage.BadgerStorage, cfg *config.IPFSConfig) error {
	m, err := cfg.ToMap()
	if err != nil {
		return err
	}
	return store.SetConfig("ipfs_config", m["ipfs_config"])
}

// loadAndConnectStoredPeers loads peers from storage and returns them for the node to connect to
func loadAndConnectStoredPeers(store *storage.BadgerStorage, ipfsConfig *config.IPFSConfig) ([]libp2ppeer.AddrInfo, error) {
	// Load stored peers (limit to max_stored_peers)
	peers, err := store.ListPeers(ipfsConfig.MaxStoredPeers)
	if err != nil {
		return nil, err
	}

	if len(peers) == 0 {
		logger.Debug("no stored peers found")
		return nil, nil
	}

	logger.Infow("loaded stored peers", "count", len(peers))

	// Filter peers with good success rate (>50%) and recent (<7 days)
	maxAge := 7 * 24 * time.Hour
	var goodPeers []libp2ppeer.AddrInfo

	for _, peer := range peers {
		if peer.SuccessRate() < 0.5 {
			logger.Debugw("skipping peer with low success rate", "peer", peer.PeerID, "rate", peer.SuccessRate())
			continue
		}
		if peer.IsStale(maxAge) {
			logger.Debugw("skipping stale peer", "peer", peer.PeerID, "last_seen", peer.LastSeen)
			continue
		}

		// Convert to AddrInfo
		addrInfo, err := parsePeerRecord(peer)
		if err != nil {
			logger.Debugw("failed to parse peer record", "peer", peer.PeerID, "error", err)
			continue
		}
		goodPeers = append(goodPeers, addrInfo)
	}

	if len(goodPeers) == 0 {
		logger.Debug("no good stored peers found")
		return nil, nil
	}

	logger.Infow("filtered stored peers", "count", len(goodPeers))
	return goodPeers, nil
}

// parsePeerRecord converts a PeerRecord to peer.AddrInfo
func parsePeerRecord(peer *storage.PeerRecord) (libp2ppeer.AddrInfo, error) {
	addrInfo := libp2ppeer.AddrInfo{
		ID: libp2ppeer.ID(peer.PeerID),
	}

	for _, addrStr := range peer.Multiaddrs {
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			continue
		}
		addrInfo.Addrs = append(addrInfo.Addrs, ma)
	}

	if len(addrInfo.Addrs) == 0 {
		return addrInfo, fmt.Errorf("no valid addresses for peer %s", peer.PeerID)
	}

	return addrInfo, nil
}

// storeConnectedPeers stores currently connected peers to storage
func storeConnectedPeers(store *storage.BadgerStorage, node *ipfsnode.Node) error {
	info := node.GetNetworkInfo()
	if info.ConnectedPeerCount == 0 {
		logger.Debug("no connected peers to store")
		return nil
	}

	stored := 0
	for _, peer := range info.ConnectedPeers {
		// Check if peer already exists
		existing, err := store.GetPeer(peer.ID)
		if err != nil {
			logger.Warnw("failed to get peer", "peer", peer.ID, "error", err)
			continue
		}

		now := time.Now()
		var peerRecord *storage.PeerRecord

		if existing != nil {
			// Update existing peer
			peerRecord = existing
			peerRecord.LastSeen = now
			peerRecord.LastConnected = now
			peerRecord.ConnectCount++
			peerRecord.Multiaddrs = peer.Addresses
			peerRecord.Protocols = peer.Protocols
		} else {
			// Create new peer record
			peerRecord = &storage.PeerRecord{
				PeerID:        peer.ID,
				Multiaddrs:    peer.Addresses,
				Protocols:     peer.Protocols,
				FirstSeen:     now,
				LastSeen:      now,
				LastConnected: now,
				ConnectCount:  1,
				FailCount:     0,
				Source:        storage.SourceBootstrap, // Assume bootstrap for initial connections
			}
		}

		if err := store.AddPeer(peerRecord); err != nil {
			logger.Warnw("failed to store peer", "peer", peer.ID, "error", err)
			continue
		}
		stored++
	}

	logger.Infow("stored connected peers", "count", stored, "total_connected", info.ConnectedPeerCount)
	return nil
}
