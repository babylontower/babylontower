// Package main provides the Babylon Tower CLI application entry point.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"babylontower/cmd/messenger/cli"
	"babylontower/cmd/messenger/ui"
	"babylontower/pkg/app"
	"babylontower/pkg/config"
	"babylontower/pkg/identity"

	"github.com/ipfs/go-log/v2"
)

var logger = log.Logger("babylontower")

var (
	Version   = "0.1.0-unversioned-poc"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

const (
	DefaultDataDir   = ".babylontower"
	IdentityFileName = "identity.json"
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
	fmt.Println("  -ui                           Use graphical UI instead of CLI")
	fmt.Println("")
	fmt.Println("Running Multiple Instances:")
	fmt.Println("  To run two nodes on the same machine, use different data directories:")
	fmt.Println("    ./messenger -data-dir ~/.babylontower/node1")
	fmt.Println("    ./messenger -data-dir ~/.babylontower/node2")
	fmt.Println("")
	fmt.Println("Modes:")
	fmt.Println("  CLI (default): Text-based interactive console")
	fmt.Println("  UI (-ui):      Graphical interface using Gio")
}

func run() error {
	dataDirFlag := flag.String("data-dir", "", "Data directory (default: ~/.babylontower)")
	configFlag := flag.String("config", "", "Config file path (optional)")
	logLevelFlag := flag.String("log-level", "", "Log level (default: warn)")
	logFileFlag := flag.String("log-file", "", "Write logs to file")
	uiFlag := flag.Bool("ui", false, "Use graphical UI instead of CLI")
	flag.Parse()

	dataDir, err := getDataDir(*dataDirFlag)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	identityPath := filepath.Join(dataDir, IdentityFileName)

	// Load configuration
	appConfig, err := loadAppConfig(dataDir, *configFlag, *logLevelFlag, *logFileFlag)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Configure logging
	if err := configureLogging(appConfig.LogLevel, appConfig.LogFile); err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}

	logger.Infow("starting Babylon Tower", "version", Version, "data_dir", dataDir)

	// Load or create identity
	ident, err := loadOrCreateIdentity(identityPath)
	if err != nil {
		return fmt.Errorf("failed to load identity: %w", err)
	}

	logger.Infow("identity loaded", "public_key", ident.PublicKeyHex())

	// Create application (core services)
	application, err := app.NewApplication(appConfig, ident)
	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}
	defer func() {
		if err := application.Stop(); err != nil {
			logger.Errorw("failed to stop application", "error", err)
		}
	}()

	if err := application.Start(); err != nil {
		return fmt.Errorf("failed to start application: %w", err)
	}

	// Show bootstrap status information
	network := application.Network()
	bootstrapStatus := network.GetBootstrapStatus()
	
	fmt.Println("")
	fmt.Println("╔════════════════════════════════════════════════════════╗")
	fmt.Println("║          Babylon Tower - Bootstrap Status              ║")
	fmt.Println("╠════════════════════════════════════════════════════════╣")
	
	if bootstrapStatus.IPFSBootstrapComplete {
		fmt.Println("║ ✓ IPFS DHT (Transport): Complete                       ║")
	} else {
		fmt.Println("║ ⏳ IPFS DHT (Transport): Bootstrapping...                ║")
	}
	
	if bootstrapStatus.BabylonBootstrapComplete {
		fmt.Println("║ ✓ Babylon DHT (Protocol): Complete                       ║")
	} else if bootstrapStatus.BabylonBootstrapDeferred {
		fmt.Println("║ ⏸ Babylon DHT (Protocol): Deferred (lazy bootstrap)      ║")
	} else {
		fmt.Println("║ ⏳ Babylon DHT (Protocol): Bootstrapping...              ║")
	}
	
	if bootstrapStatus.RendezvousActive {
		fmt.Println("║ ✓ Rendezvous: Active (discoverable)                      ║")
	} else {
		fmt.Println("║ ⏳ Rendezvous: Pending                                   ║")
	}
	
	fmt.Println("╠════════════════════════════════════════════════════════╣")
	fmt.Println("║ Note: Protocol operations (identity, mailbox) will     ║")
	fmt.Println("║       wait for Babylon DHT bootstrap to complete.      ║")
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println("")

	// Create and run CLI or UI based on flag
	if *uiFlag {
		return runUI(Version, dataDir, identityPath, ident, application)
	}
	return runCLI(Version, dataDir, identityPath, ident, application)
}

// loadAppConfig loads and merges configuration from file and CLI flags
func loadAppConfig(dataDir, configFile, logLevel, logFile string) (*app.AppConfig, error) {
	cfg, err := config.LoadAppConfig(dataDir, configFile)
	if err != nil {
		return nil, err
	}

	// Apply CLI flag overrides
	if logLevel != "" {
		cfg.Logging.Level = logLevel
	}
	if logFile != "" {
		cfg.Logging.File = logFile
	}

	// Validate config
	if err := config.ValidateAppConfig(cfg); err != nil {
		logger.Warnw("config validation failed, using defaults", "error", err)
		cfg = config.DefaultAppConfig()
	}

	// Convert to app.AppConfig
	return &app.AppConfig{
		DataDir:      dataDir,
		IdentityPath: filepath.Join(dataDir, IdentityFileName),
		StorageDir:   filepath.Join(dataDir, "storage"),
		IPFSDir:      filepath.Join(dataDir, "ipfs"),
		LogLevel:     cfg.Logging.Level,
		LogFile:      resolvePath(cfg.Logging.File, dataDir),
		NetworkConfig: cfg.ToIPFSConfig(),
	}, nil
}

// resolvePath resolves a relative path against a base directory
func resolvePath(path, baseDir string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

// configureLogging sets up structured logging
func configureLogging(level, logFile string) error {
	if level == "" {
		level = os.Getenv("BABYLONTOWER_LOG_LEVEL")
	}
	if level == "" {
		level = "warn"
	}
	level = strings.ToLower(level)

	if logFile == "" {
		logFile = os.Getenv("BABYLONTOWER_LOG_FILE")
	}

	logCfg := log.GetConfig()
	logCfg.Level = log.LevelError
	if logFile != "" {
		logCfg.File = logFile
	} else {
		logCfg.File = ""
	}
	log.SetupLogging(logCfg)

	// Set babylontower subsystems
	btSubsystems := []string{
		"babylontower", "babylontower/cli", "babylontower/storage",
		"babylontower/messaging", "babylontower/ipfsnode", "babylontower/identity",
		"babylontower/peerstore", "babylontower/rtc", "babylontower/multidevice",
		"babylontower/groups", "babylontower/mailbox", "babylontower/reputation",
		"babylontower/protocol", "babylontower/ratchet", "babylontower/recovery",
		"babylontower/app",
	}
	for _, sub := range btSubsystems {
		_ = log.SetLogLevel(sub, level)
	}

	// Quiet libp2p subsystems
	libp2pLevel := "error"
	if level == "debug" {
		libp2pLevel = "warn"
	}
	libp2pSubsystems := []string{
		"dht", "pubsub", "swarm2", "relay", "autonat", "connmgr",
		"basichost", "net/identify", "p2p-discovery", "p2p-swarm",
		"p2p-net", "ipns", "bitswap", "blockservice", "flatfs", "pebble", "badger",
	}
	for _, sub := range libp2pSubsystems {
		_ = log.SetLogLevel(sub, libp2pLevel)
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

func createCLIApp(version, dataDir, identityPath string, ident *identity.Identity, application app.Application) (*cli.CLI, error) {
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
		Mnemonic:       ident.Mnemonic,
	}

	return cli.New(cliConfig, cliIdentity, application.Storage(), application.Network(), application.Messenger(), application.Groups())
}

func createUIApp(version, dataDir string, ident *identity.Identity, application app.Application) (*ui.UI, error) {
	uiConfig := &ui.Config{
		DarkMode: false, // Could be made configurable via flags
		Title:    "Babylon Tower - " + version,
	}

	return ui.New(uiConfig, application)
}

// runCLI creates and runs the CLI interface
func runCLI(version, dataDir, identityPath string, ident *identity.Identity, application app.Application) error {
	cliApp, err := createCLIApp(version, dataDir, identityPath, ident, application)
	if err != nil {
		return fmt.Errorf("failed to create CLI: %w", err)
	}

	if err := cliApp.Start(); err != nil {
		return fmt.Errorf("CLI error: %w", err)
	}

	logger.Info("Babylon Tower shutdown complete")
	return nil
}

// runUI creates and runs the graphical UI interface
func runUI(version, dataDir, identityPath string, ident *identity.Identity, application app.Application) error {
	uiApp, err := createUIApp(version, dataDir, ident, application)
	if err != nil {
		return fmt.Errorf("failed to create UI: %w", err)
	}

	logger.Info("starting graphical UI")
	if err := uiApp.Start(); err != nil {
		return fmt.Errorf("UI error: %w", err)
	}

	logger.Info("Babylon Tower shutdown complete")
	return nil
}
