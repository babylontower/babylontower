package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"babylontower/pkg/cli"
	"babylontower/pkg/config"
	"babylontower/pkg/groups"
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

var (
	Version   = "0.1.0-unversioned-poc"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

const (
	DefaultDataDir     = ".babylontower"
	IdentityFileName   = "identity.json"
	StorageDirName     = "storage"
	IPFSDirName        = "ipfs"
	peerSuccessThreshold = 0.5
	peerMaxAge         = 7 * 24 * time.Hour
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "-h" || arg == "--help" {
			printHelp()
			os.Exit(0)
		}
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("Babylon Tower - Decentralized P2P Messenger")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  messenger [options]")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  -data-dir <path>              Data directory for this instance")
	fmt.Println("                                Default: ~/.babylontower")
	fmt.Println("  -log-level <level>            Log level: debug, info, warn, error")
	fmt.Println("                                Default: warn (env: BABYLONTOWER_LOG_LEVEL)")
	fmt.Println("  -log-file <path>              Write logs to file instead of stderr")
	fmt.Println("                                (env: BABYLONTOWER_LOG_FILE)")
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
}

func run() error {
	dataDirFlag := flag.String("data-dir", "", "Data directory (default: ~/.babylontower)")
	configFlag := flag.String("config", "", "Config file path (optional)")
	logLevelFlag := flag.String("log-level", "", "Log level: debug, info, warn, error (default: warn)")
	logFileFlag := flag.String("log-file", "", "Write logs to file instead of stderr")
	flag.Parse()

	if err := configureLogging(*logLevelFlag, *logFileFlag); err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}

	dataDir, err := getDataDir(*dataDirFlag)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	identityPath := filepath.Join(dataDir, IdentityFileName)
	storageDir := filepath.Join(dataDir, StorageDirName)
	ipfsDir := filepath.Join(dataDir, IPFSDirName)

	logger.Infow("starting Babylon Tower", "version", Version, "data_dir", dataDir)

	// Load unified YAML configuration
	appConfig, err := config.LoadAppConfig(dataDir, *configFlag)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply CLI flag overrides on top of config
	if *logLevelFlag != "" {
		appConfig.Logging.Level = *logLevelFlag
	}
	if *logFileFlag != "" {
		appConfig.Logging.File = *logFileFlag
	}

	if err := config.ValidateAppConfig(appConfig); err != nil {
		logger.Warnw("config validation failed, using defaults", "error", err)
		appConfig = config.DefaultAppConfig()
	}

	// Convert to IPFSConfig for backward compatibility with ipfsnode package
	ipfsConfig := appConfig.ToIPFSConfig()

	ident, err := loadOrCreateIdentity(identityPath)
	if err != nil {
		return fmt.Errorf("failed to load identity: %w", err)
	}

	logger.Infow("identity loaded", "public_key", ident.PublicKeyHex())

	store, err := storage.NewBadgerStorage(storage.Config{
		Path:     storageDir,
		InMemory: appConfig.Storage.InMemory,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer closeStorage(store)

	logger.Info("storage initialized")

	logger.Infow("config loaded",
		"bootstrap_peers", len(ipfsConfig.BootstrapPeers),
		"max_stored_peers", ipfsConfig.MaxStoredPeers,
		"min_peer_connections", ipfsConfig.MinPeerConnections)

	storedPeers, err := loadAndConnectStoredPeers(store, ipfsConfig)
	if err != nil {
		logger.Warnw("failed to load stored peers", "error", err)
	} else if len(storedPeers) > 0 {
		logger.Infow("will connect to stored peers", "count", len(storedPeers))
	}

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

	if err := ipfsNode.Start(); err != nil {
		return fmt.Errorf("failed to start IPFS node: %w", err)
	}
	defer stopIPFSNode(ipfsNode)

	waitForDHT(ipfsNode, ipfsConfig.DHTBootstrapTimeout)

	if err := storeConnectedPeers(store, ipfsNode); err != nil {
		logger.Warnw("failed to store connected peers", "error", err)
	}

	logger.Infow("IPFS node started",
		"peer_id", ipfsNode.PeerID(),
		"addresses", ipfsNode.Multiaddrs())

	if err := initializeAddrBook(dataDir, ipfsNode); err != nil {
		logger.Warnw("failed to initialize address book", "error", err)
	}

	msgService := createMessagingService(ident, store, ipfsNode)

	if err := msgService.Start(); err != nil {
		return fmt.Errorf("failed to start messaging service: %w", err)
	}
	defer stopMessagingService(msgService)

	logger.Info("messaging service started")

	groupsService := createGroupsService(ident, store)

	cliApp, err := createCLIApp(Version, dataDir, identityPath, ident, store, ipfsNode, msgService, groupsService)
	if err != nil {
		return fmt.Errorf("failed to create CLI: %w", err)
	}

	if err := cliApp.Start(); err != nil {
		return fmt.Errorf("CLI error: %w", err)
	}

	if err := cliApp.Stop(); err != nil {
		logger.Warnw("CLI stop error", "error", err)
	}

	logger.Info("Babylon Tower shutdown complete")
	return nil
}

// configureLogging sets up structured logging with the given level and optional file output.
// Priority: CLI flag > env var > default ("warn").
func configureLogging(flagLevel, flagFile string) error {
	// Determine log level: flag > env > default
	level := flagLevel
	if level == "" {
		level = os.Getenv("BABYLONTOWER_LOG_LEVEL")
	}
	if level == "" {
		level = "warn"
	}
	level = strings.ToLower(level)

	// Determine log file: flag > env > stderr
	logFile := flagFile
	if logFile == "" {
		logFile = os.Getenv("BABYLONTOWER_LOG_FILE")
	}

	// Configure log output via go-log/v2 SetupLogging
	logCfg := log.GetConfig()
	logCfg.Level = log.LevelError // base level, overridden per-subsystem below
	if logFile != "" {
		logCfg.File = logFile
	}
	log.SetupLogging(logCfg)

	// Set all babylontower subsystems to the requested level
	btSubsystems := []string{
		"babylontower",
		"babylontower/cli",
		"babylontower/storage",
		"babylontower/messaging",
		"babylontower/ipfsnode",
		"babylontower/identity",
		"babylontower/peerstore",
		"babylontower/rtc",
		"babylontower/multidevice",
		"babylontower/groups",
		"babylontower/mailbox",
		"babylontower/reputation",
		"babylontower/protocol",
		"babylontower/ratchet",
		"babylontower/recovery",
	}
	for _, sub := range btSubsystems {
		if err := log.SetLogLevel(sub, level); err != nil {
			return fmt.Errorf("failed to set log level for %s: %w", sub, err)
		}
	}

	// Quiet libp2p subsystems — set to "error" normally, "warn" when app is debug
	libp2pLevel := "error"
	if level == "debug" {
		libp2pLevel = "warn"
	}
	libp2pSubsystems := []string{
		"dht", "pubsub", "swarm2", "relay", "autonat",
		"connmgr", "basichost", "net/identify",
	}
	for _, sub := range libp2pSubsystems {
		_ = log.SetLogLevel(sub, libp2pLevel) // best-effort; subsystem may not exist yet
	}

	return nil
}

func getDataDir(flagDir string) (string, error) {
	if flagDir != "" {
		return flagDir, nil
	}

	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
	}
	return filepath.Join(homeDir, DefaultDataDir), nil
}

func closeStorage(store *storage.BadgerStorage) {
	if err := store.Close(); err != nil {
		logger.Warnw("failed to close storage", "error", err)
	}
}

func stopIPFSNode(node *ipfsnode.Node) {
	if err := node.Stop(); err != nil {
		logger.Warnw("failed to stop IPFS node", "error", err)
	}
}

func stopMessagingService(service *messaging.Service) {
	if err := service.Stop(); err != nil {
		logger.Warnw("failed to stop messaging service", "error", err)
	}
}

func waitForDHT(node *ipfsnode.Node, timeout time.Duration) {
	logger.Infow("waiting for DHT bootstrap", "timeout", timeout)
	if err := node.WaitForDHT(timeout); err != nil {
		logger.Warnw("DHT bootstrap incomplete", "error", err, "action", "continuing with mDNS discovery")
	} else {
		logger.Info("DHT bootstrap complete")
	}
}

func initializeAddrBook(dataDir string, node *ipfsnode.Node) error {
	addrBook, err := peerstore.NewAddrBook(dataDir)
	if err != nil {
		return err
	}

	logger.Infow("address book initialized", "peer_count", addrBook.Count())

	if addrBook.Count() == 0 {
		return nil
	}

	logger.Info("attempting to connect to known peers...")
	ctx := node.Context()
	if err := addrBook.ConnectToAll(ctx, node); err != nil {
		logger.Warnw("auto-connect failed", "error", err)
	} else {
		logger.Info("auto-connect completed")
	}
	return nil
}

func createMessagingService(ident *identity.Identity, store storage.Storage, node *ipfsnode.Node) *messaging.Service {
	msgConfig := &messaging.Config{
		OwnEd25519PrivKey: ident.Ed25519PrivKey,
		OwnEd25519PubKey:  ident.Ed25519PubKey,
		OwnX25519PrivKey:  ident.X25519PrivKey,
		OwnX25519PubKey:   ident.X25519PubKey,
	}
	return messaging.NewService(msgConfig, store, node)
}

func createGroupsService(ident *identity.Identity, store *storage.BadgerStorage) *groups.Service {
	return groups.NewService(store, ident.Ed25519PubKey, ident.Ed25519PrivKey)
}

func createCLIApp(version, dataDir, identityPath string, ident *identity.Identity, store storage.Storage, node *ipfsnode.Node, msgService *messaging.Service, groupsService *groups.Service) (*cli.CLI, error) {
	cliConfig := &cli.Config{
		Version:      version,
		DataDir:      dataDir,
		IdentityPath: identityPath,
	}

	cliIdentity := &cli.Identity{
		Ed25519PubKey:  ident.Ed25519PubKey,
		Ed25519PrivKey: ident.Ed25519PrivKey,
		X25519PubKey:   ident.X25519PubKey,
		X25519PrivKey:  ident.X25519PrivKey,
	}

	return cli.New(cliConfig, cliIdentity, store, node, msgService, groupsService)
}

func loadOrCreateIdentity(identityPath string) (*identity.Identity, error) {
	if identity.IdentityExists(identityPath) {
		return identity.LoadIdentity(identityPath)
	}

	logger.Info("Generating new identity...")
	ident, err := identity.GenerateIdentity()
	if err != nil {
		return nil, fmt.Errorf("failed to generate identity: %w", err)
	}

	if err := identity.SaveIdentity(ident, identityPath); err != nil {
		return nil, fmt.Errorf("failed to save identity: %w", err)
	}

	logger.Info("New identity generated and saved")
	printNewIdentityInfo(ident)

	return ident, nil
}

func printNewIdentityInfo(ident *identity.Identity) {
	fmt.Printf("\n🎉 New identity generated!\n")
	fmt.Printf("Your mnemonic (write this down safely):\n")
	fmt.Printf("  %s\n\n", ident.Mnemonic)
	fmt.Printf("WARNING: If you lose this mnemonic, you lose your identity!\n")
	fmt.Printf("Store it in a secure location.\n\n")
}


func loadAndConnectStoredPeers(store *storage.BadgerStorage, ipfsConfig *config.IPFSConfig) ([]libp2ppeer.AddrInfo, error) {
	peers, err := store.ListPeers(ipfsConfig.MaxStoredPeers)
	if err != nil {
		return nil, err
	}

	if len(peers) == 0 {
		logger.Debug("no stored peers found")
		return nil, nil
	}

	logger.Infow("loaded stored peers", "count", len(peers))

	goodPeers := filterGoodPeers(peers)
	if len(goodPeers) == 0 {
		logger.Debug("no good stored peers found")
		return nil, nil
	}

	logger.Infow("filtered stored peers", "count", len(goodPeers))
	return goodPeers, nil
}

func filterGoodPeers(peers []*storage.PeerRecord) []libp2ppeer.AddrInfo {
	var goodPeers []libp2ppeer.AddrInfo

	for _, peer := range peers {
		if peer.SuccessRate() < peerSuccessThreshold {
			logger.Debugw("skipping peer with low success rate", "peer", peer.PeerID, "rate", peer.SuccessRate())
			continue
		}
		if peer.IsStale(peerMaxAge) {
			logger.Debugw("skipping stale peer", "peer", peer.PeerID, "last_seen", peer.LastSeen)
			continue
		}

		addrInfo, err := parsePeerRecord(peer)
		if err != nil {
			logger.Debugw("failed to parse peer record", "peer", peer.PeerID, "error", err)
			continue
		}
		goodPeers = append(goodPeers, addrInfo)
	}

	return goodPeers
}

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

func storeConnectedPeers(store *storage.BadgerStorage, node *ipfsnode.Node) error {
	info := node.GetNetworkInfo()
	if info.ConnectedPeerCount == 0 {
		logger.Debug("no connected peers to store")
		return nil
	}

	stored := 0
	for _, peer := range info.ConnectedPeers {
		peerRecord, err := getOrCreatePeerRecord(store, peer)
		if err != nil {
			logger.Warnw("failed to get peer", "peer", peer.ID, "error", err)
			continue
		}

		updatePeerRecord(peerRecord, peer)

		if err := store.AddPeer(peerRecord); err != nil {
			logger.Warnw("failed to store peer", "peer", peer.ID, "error", err)
			continue
		}
		stored++
	}

	logger.Debugw("stored connected peers", "count", stored, "total_connected", info.ConnectedPeerCount)
	return nil
}

func getOrCreatePeerRecord(store *storage.BadgerStorage, peer ipfsnode.PeerInfo) (*storage.PeerRecord, error) {
	existing, err := store.GetPeer(peer.ID)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		return existing, nil
	}

	now := time.Now()
	return &storage.PeerRecord{
		PeerID:        peer.ID,
		FirstSeen:     now,
		LastSeen:      now,
		LastConnected: now,
		ConnectCount:  1,
		Source:        storage.SourceBootstrap,
	}, nil
}

func updatePeerRecord(record *storage.PeerRecord, peer ipfsnode.PeerInfo) {
	now := time.Now()
	record.LastSeen = now
	record.LastConnected = now
	record.ConnectCount++
	record.Multiaddrs = peer.Addresses
	record.Protocols = peer.Protocols
}
