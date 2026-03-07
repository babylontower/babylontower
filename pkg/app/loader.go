// Package app provides high-level application loading and lifecycle management.
// This file offers simplified APIs for loading, connecting, and bootstrapping.
package app

import (
	"fmt"
	"path/filepath"
	"time"

	"babylontower/pkg/config"
	"babylontower/pkg/identity"
	"babylontower/pkg/ipfsnode"

	"github.com/ipfs/go-log/v2"
)

var loadLogger = log.Logger("babylontower/app/loader")

// LoadResult contains the result of loading and bootstrapping the application
type LoadResult struct {
	// Application is the loaded application instance
	Application Application
	// Identity is the loaded or created identity
	Identity *identity.Identity
	// BootstrapResult contains bootstrap statistics
	BootstrapResult *ipfsnode.BootstrapResult
	// Duration is how long loading took
	Duration time.Duration
}

// Loader provides high-level methods for loading and managing the application
type Loader struct {
	dataDir      string
	configFile   string
	logLevel     string
	logFile      string
	bootstrapTimeout time.Duration
}

// NewLoader creates a new application loader
func NewLoader() *Loader {
	return &Loader{
		dataDir:        "",
		configFile:     "",
		logLevel:       "warn",
		logFile:        "",
		bootstrapTimeout: 60 * time.Second,
	}
}

// WithDataDir sets the data directory
func (l *Loader) WithDataDir(dataDir string) *Loader {
	l.dataDir = dataDir
	return l
}

// WithConfigFile sets the config file path
func (l *Loader) WithConfigFile(configFile string) *Loader {
	l.configFile = configFile
	return l
}

// WithLogLevel sets the log level
func (l *Loader) WithLogLevel(level string) *Loader {
	l.logLevel = level
	return l
}

// WithLogFile sets the log file path
func (l *Loader) WithLogFile(logFile string) *Loader {
	l.logFile = logFile
	return l
}

// WithBootstrapTimeout sets the bootstrap timeout
func (l *Loader) WithBootstrapTimeout(timeout time.Duration) *Loader {
	l.bootstrapTimeout = timeout
	return l
}

// LoadAndBootstrap loads the application and waits for bootstrap to complete.
// This is the main entry point for starting Babylon Tower.
//
// Usage:
//   loader := app.NewLoader().WithDataDir("~/.babylontower")
//   result, err := loader.LoadAndBootstrap()
//   if err != nil {
//       return err
//   }
//   defer result.Application.Stop()
//
//   // Application is ready to use
//   fmt.Println("Bootstrap complete:", result.BootstrapResult)
func (l *Loader) LoadAndBootstrap() (*LoadResult, error) {
	startTime := time.Now()
	loadLogger.Infow("loading Babylon Tower application",
		"data_dir", l.dataDir,
		"config_file", l.configFile)

	// Step 1: Load or create identity
	identityPath := filepath.Join(l.dataDir, "identity.json")
	ident, err := loadOrCreateIdentity(identityPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load identity: %w", err)
	}

	loadLogger.Infow("identity loaded",
		"public_key", ident.PublicKeyHex(),
		"peer_id_fingerprint", identity.DeriveIdentityDHTKey(ident.Ed25519PubKey))

	// Step 2: Load application configuration
	appConfig, err := l.loadAppConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Step 3: Create application (this starts IPFS node and messaging)
	application, err := NewApplication(appConfig, ident)
	if err != nil {
		return nil, fmt.Errorf("failed to create application: %w", err)
	}

	// Step 4: Wait for bootstrap to complete
	loadLogger.Infow("waiting for bootstrap to complete",
		"timeout", l.bootstrapTimeout)

	network := application.Network()
	bootstrapResult, err := network.(*networkNodeAdapter).node.BootstrapAndWait(l.bootstrapTimeout)
	if err != nil {
		loadLogger.Warnw("bootstrap incomplete",
			"error", err,
			"routing_table", bootstrapResult.RoutingTableSize,
			"total_connected", bootstrapResult.TotalConnected)
		// Continue anyway - application can still function
	} else {
		loadLogger.Infow("bootstrap complete",
			"routing_table", bootstrapResult.RoutingTableSize,
			"babylon_peers", bootstrapResult.BabylonPeersConnected,
			"duration", bootstrapResult.Duration)
	}

	loadDuration := time.Since(startTime)
	loadLogger.Infow("application loaded successfully",
		"duration", loadDuration,
		"peer_id", network.PeerID())

	return &LoadResult{
		Application:   application,
		Identity:      ident,
		BootstrapResult: bootstrapResult,
		Duration:      loadDuration,
	}, nil
}

// LoadWithoutBootstrap loads the application without waiting for bootstrap.
// Useful for scenarios where you want to start quickly and bootstrap asynchronously.
func (l *Loader) LoadWithoutBootstrap() (*LoadResult, error) {
	startTime := time.Now()

	// Load identity
	identityPath := filepath.Join(l.dataDir, "identity.json")
	ident, err := loadOrCreateIdentity(identityPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load identity: %w", err)
	}

	// Load config
	appConfig, err := l.loadAppConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Create application
	application, err := NewApplication(appConfig, ident)
	if err != nil {
		return nil, fmt.Errorf("failed to create application: %w", err)
	}

	loadDuration := time.Since(startTime)

	return &LoadResult{
		Application: application,
		Identity:    ident,
		Duration:    loadDuration,
	}, nil
}

// loadAppConfig loads and validates the application configuration
func (l *Loader) loadAppConfig() (*AppConfig, error) {
	cfg, err := config.LoadAppConfig(l.dataDir, l.configFile)
	if err != nil {
		return nil, err
	}

	// Apply overrides
	if l.logLevel != "" {
		cfg.Logging.Level = l.logLevel
	}
	if l.logFile != "" {
		cfg.Logging.File = l.logFile
	}

	// Validate
	if err := config.ValidateAppConfig(cfg); err != nil {
		loadLogger.Warnw("config validation failed, using defaults", "error", err)
		cfg = config.DefaultAppConfig()
	}

	// Convert to app.AppConfig
	return &AppConfig{
		DataDir:      l.dataDir,
		IdentityPath: filepath.Join(l.dataDir, "identity.json"),
		StorageDir:   filepath.Join(l.dataDir, "storage"),
		IPFSDir:      filepath.Join(l.dataDir, "ipfs"),
		LogLevel:     cfg.Logging.Level,
		LogFile:      resolvePath(cfg.Logging.File, l.dataDir),
		IPFSConfig:   cfg.ToIPFSConfig(),
	}, nil
}

// loadOrCreateIdentity loads existing identity or creates new one
func loadOrCreateIdentity(identityPath string) (*identity.Identity, error) {
	if identity.IdentityExists(identityPath) {
		loadLogger.Infow("loading existing identity", "path", identityPath)
		return identity.LoadIdentity(identityPath)
	}

	loadLogger.Infow("generating new identity", "path", identityPath)
	ident, err := identity.GenerateIdentity()
	if err != nil {
		return nil, fmt.Errorf("failed to generate identity: %w", err)
	}

	if err := identity.SaveIdentity(ident, identityPath); err != nil {
		return nil, fmt.Errorf("failed to save identity: %w", err)
	}

	loadLogger.Info("new identity generated and saved")
	return ident, nil
}

// resolvePath resolves a relative path against a base directory
func resolvePath(path, baseDir string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

// ==================== Convenience Functions ====================

// QuickStart is a convenience function that loads and bootstraps with minimal configuration.
// Perfect for getting started quickly with sensible defaults.
//
// Usage:
//   result, cleanup, err := app.QuickStart("~/.babylontower")
//   if err != nil {
//       return err
//   }
//   defer cleanup()
//
//   // Use result.Application
func QuickStart(dataDir string) (*LoadResult, func(), error) {
	loader := NewLoader().
		WithDataDir(dataDir).
		WithBootstrapTimeout(60 * time.Second)

	result, err := loader.LoadAndBootstrap()
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		if result.Application != nil {
			if err := result.Application.Stop(); err != nil {
				loadLogger.Warnw("cleanup stop error", "error", err)
			}
		}
	}

	return result, cleanup, nil
}

// QuickStartWithConfig is like QuickStart but accepts a config file path
func QuickStartWithConfig(dataDir, configFile string) (*LoadResult, func(), error) {
	loader := NewLoader().
		WithDataDir(dataDir).
		WithConfigFile(configFile).
		WithBootstrapTimeout(60 * time.Second)

	result, err := loader.LoadAndBootstrap()
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		if result.Application != nil {
			if err := result.Application.Stop(); err != nil {
				loadLogger.Warnw("cleanup stop error", "error", err)
			}
		}
	}

	return result, cleanup, nil
}

// WaitForBootstrap waits for bootstrap to complete with timeout
func WaitForBootstrap(app Application, timeout time.Duration) (*ipfsnode.BootstrapResult, error) {
	network := app.Network()
	adapter, ok := network.(*networkNodeAdapter)
	if !ok {
		return nil, fmt.Errorf("network node is not the expected type")
	}

	return adapter.node.BootstrapAndWait(timeout)
}

// GetBootstrapStatus gets the current bootstrap status
func GetBootstrapStatus(app Application) *BootstrapStatus {
	network := app.Network()
	return network.GetBootstrapStatus()
}

// IsReady checks if the application is ready for protocol operations
func IsReady(app Application) bool {
	status := GetBootstrapStatus(app)
	// Ready if IPFS bootstrap is complete (transport layer)
	// Babylon bootstrap can be deferred (lazy bootstrap)
	return status.IPFSBootstrapComplete
}

// GetConnectionInfo returns comprehensive connection information
func GetConnectionInfo(app Application) *ConnectionInfo {
	network := app.Network()
	status := GetBootstrapStatus(app)
	networkInfo := network.GetNetworkInfo()
	dhtInfo := network.GetDHTInfo()
	babylonInfo := network.GetBabylonDHTInfo()
	peerCounts := network.GetPeerCountsBySource()

	return &ConnectionInfo{
		PeerID:              networkInfo.PeerID,
		ListenAddresses:     networkInfo.Multiaddrs,
		ConnectedPeers:      networkInfo.ConnectedPeerCount,
		IPFSRoutingTable:    dhtInfo.RoutingTableSize,
		BabylonPeers:        babylonInfo.ConnectedBabylonPeers,
		PeerCountsBySource:  peerCounts,
		BootstrapStatus:     status,
		IsReady:             IsReady(app),
	}
}

// ConnectionInfo contains comprehensive connection information
type ConnectionInfo struct {
	PeerID             string
	ListenAddresses    []string
	ConnectedPeers     int
	IPFSRoutingTable   int
	BabylonPeers       int
	PeerCountsBySource *PeerCountsBySource
	BootstrapStatus    *BootstrapStatus
	IsReady            bool
}

// String returns a human-readable summary
func (ci *ConnectionInfo) String() string {
	if ci == nil {
		return "ConnectionInfo(nil)"
	}

	return fmt.Sprintf("ConnectionInfo{PeerID=%s, Connected=%d, IPFS_RT=%d, Babylon=%d, Ready=%v}",
		ci.PeerID,
		ci.ConnectedPeers,
		ci.IPFSRoutingTable,
		ci.BabylonPeers,
		ci.IsReady,
	)
}
