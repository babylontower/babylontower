// Package main provides the Babylon Tower Gio UI application entry point.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"babylontower/cmd/messenger/ui"
	"babylontower/pkg/app"
	"babylontower/pkg/config"
	"babylontower/pkg/identity"

	"github.com/ipfs/go-log/v2"
)

var logger = log.Logger("babylontower")

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
	fmt.Println("Babylon Tower - Decentralized P2P Messenger (Gio UI)")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  babylon-ui [options]")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  -data-dir <path>              Data directory for this instance")
	fmt.Println("                                Default: ~/.babylontower")
	fmt.Println("  -log-level <level>            Log level: debug, info, warn, error")
	fmt.Println("                                Default: warn (env: BABYLONTOWER_LOG_LEVEL)")
	fmt.Println("  -log-file <path>              Write logs to file instead of stderr")
	fmt.Println("                                (env: BABYLONTOWER_LOG_FILE)")
	fmt.Println("  -dark-mode                    Enable dark mode")
	fmt.Println("")
	fmt.Println("Running Multiple Instances:")
	fmt.Println("  To run two nodes on the same machine, use different data directories:")
	fmt.Println("    ./babylon-ui -data-dir ~/.babylontower/node1")
	fmt.Println("    ./babylon-ui -data-dir ~/.babylontower/node2")
}

func run() error {
	dataDirFlag := flag.String("data-dir", "", "Data directory (default: ~/.babylontower)")
	configFlag := flag.String("config", "", "Config file path (optional)")
	logLevelFlag := flag.String("log-level", "", "Log level (default: warn)")
	logFileFlag := flag.String("log-file", "", "Write logs to file")
	darkModeFlag := flag.Bool("dark-mode", false, "Enable dark mode")
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

	logger.Infow("starting Babylon Tower UI", "version", getVersion(), "data_dir", dataDir)

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

	// Create and run Gio UI
	gioUI, err := ui.New(&ui.Config{
		DarkMode: *darkModeFlag,
		Title:    "Babylon Tower",
	}, application)
	if err != nil {
		return fmt.Errorf("failed to create UI: %w", err)
	}

	// Run UI event loop
	if err := gioUI.Start(); err != nil {
		return fmt.Errorf("UI error: %w", err)
	}

	logger.Info("Babylon Tower UI shutdown complete")
	return nil
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
		IPFSConfig:   cfg.ToIPFSConfig(),
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
		"babylontower", "babylontower/ui", "babylontower/storage",
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

func getVersion() string {
	version := os.Getenv("VERSION")
	if version == "" {
		version = "0.1.0-ui-alpha"
	}
	return version
}
