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
}

func run() error {
	dataDirFlag := flag.String("data-dir", "", "Data directory (default: ~/.babylontower)")
	configFlag := flag.String("config", "", "Config file path (optional)")
	logLevelFlag := flag.String("log-level", "", "Log level (default: warn)")
	logFileFlag := flag.String("log-file", "", "Write logs to file")
	darkModeFlag := flag.Bool("dark-mode", true, "Enable dark mode (default: true)")
	flag.Parse()

	dataDir, err := getDataDir(*dataDirFlag)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	identityPath := filepath.Join(dataDir, IdentityFileName)

	// ── Step 1: Identity ────────────────────────────────────────────────
	// If no identity file exists, launch the onboarding GUI.
	// The user creates or restores an identity before the app boots.

	if !app.IdentityFileExists(identityPath) {
		result := runOnboarding()
		if result == nil {
			// User closed the onboarding window — exit cleanly
			return nil
		}

		if err := app.SaveIdentityToFile(result.identity, identityPath); err != nil {
			return fmt.Errorf("failed to save identity: %w", err)
		}
		logger.Infow("identity saved", "path", identityPath)

		// Save initial config with profile from onboarding
		profile := config.ProfileConfig{
			DisplayName: result.displayName,
			DeviceName:  result.deviceName,
		}
		if err := config.SaveMinimalConfig(dataDir, profile); err != nil {
			logger.Warnw("failed to save initial config", "error", err)
		}
	}

	// ── Step 2: Load identity ───────────────────────────────────────────
	identResult, err := app.LoadIdentityFromFile(identityPath)
	if err != nil {
		return fmt.Errorf("failed to load identity: %w", err)
	}
	ident := identResult.Identity

	// ── Step 3: Config & logging ────────────────────────────────────────
	appConfig, rawCfg, err := loadAppConfig(dataDir, *configFlag, *logLevelFlag, *logFileFlag)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := configureLogging(appConfig.LogLevel, appConfig.LogFile); err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}

	logger.Infow("starting Babylon Tower UI", "version", getVersion(), "data_dir", dataDir)

	// ── Step 4: Create UI immediately (core initializes in background) ──
	gioUI, err := ui.New(&ui.Config{
		DarkMode:  *darkModeFlag,
		Title:     "Babylon Tower",
		DataDir:   dataDir,
		AppConfig: rawCfg,
	}, nil) // nil coreApp — will be set asynchronously
	if err != nil {
		return fmt.Errorf("failed to create UI: %w", err)
	}

	// ── Step 5: Initialize core services in background ──────────────────
	var application app.Application
	go func() {
		coreApp, initErr := app.NewApplication(appConfig, ident)
		if initErr != nil {
			logger.Errorw("failed to create application", "error", initErr)
			gioUI.SetCoreError(initErr)
			return
		}

		if startErr := coreApp.Start(); startErr != nil {
			logger.Errorw("failed to start application", "error", startErr)
			if stopErr := coreApp.Stop(); stopErr != nil {
				logger.Warnw("failed to stop application after start failure", "error", stopErr)
			}
			gioUI.SetCoreError(startErr)
			return
		}

		application = coreApp
		gioUI.SetCoreApp(coreApp)
		logger.Infow("core services ready")
	}()

	// ── Step 6: Run main UI (blocking) ──────────────────────────────────
	uiErr := gioUI.Start()

	// Cleanup: stop core if it was started
	if application != nil {
		if err := application.Stop(); err != nil {
			logger.Errorw("failed to stop application", "error", err)
		}
	}

	logger.Info("Babylon Tower UI shutdown complete")
	return uiErr
}

// loadAppConfig loads and merges configuration from file and CLI flags.
// Returns both the core app config and the raw config (for UI settings screen).
func loadAppConfig(dataDir, configFile, logLevel, logFile string) (*app.AppConfig, *config.AppConfig, error) {
	cfg, err := config.LoadAppConfig(dataDir, configFile)
	if err != nil {
		return nil, nil, err
	}

	if logLevel != "" {
		cfg.Logging.Level = logLevel
	}
	if logFile != "" {
		cfg.Logging.File = logFile
	}

	if err := config.ValidateAppConfig(cfg); err != nil {
		logger.Warnw("config validation failed, using defaults", "error", err)
		cfg = config.DefaultAppConfig()
	}

	return &app.AppConfig{
		DataDir:      dataDir,
		IdentityPath: filepath.Join(dataDir, IdentityFileName),
		StorageDir:   filepath.Join(dataDir, "storage"),
		IPFSDir:      filepath.Join(dataDir, "ipfs"),
		LogLevel:     cfg.Logging.Level,
		LogFile:      resolvePath(cfg.Logging.File, dataDir),
		DisplayName:  cfg.Profile.DisplayName,
		DeviceName:   cfg.Profile.DeviceName,
		NetworkConfig: cfg.ToIPFSConfig(),
	}, cfg, nil
}

func resolvePath(path, baseDir string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

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

func getVersion() string {
	version := os.Getenv("VERSION")
	if version == "" {
		version = "0.1.0-ui-alpha"
	}
	return version
}
