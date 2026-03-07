package app

import (
	"babylontower/pkg/config"
)

// AppConfig holds application configuration.
type AppConfig struct {
	// DataDir is the directory for application data
	DataDir string
	// IdentityPath is the path to the identity file
	IdentityPath string
	// StorageDir is the directory for storage
	StorageDir string
	// IPFSDir is the directory for IPFS repo
	IPFSDir string
	// LogLevel is the logging level
	LogLevel string
	// LogFile is the path to the log file (optional)
	LogFile string
	// IPFSConfig is the IPFS node configuration
	IPFSConfig *config.IPFSConfig
}

// DefaultAppConfig returns an AppConfig with sensible defaults.
func DefaultAppConfig() *AppConfig {
	return &AppConfig{
		LogLevel:   "warn",
		IPFSConfig: config.DefaultIPFSConfig(),
	}
}

// Validate validates the application configuration.
func (c *AppConfig) Validate() error {
	if c.IPFSConfig != nil {
		return c.IPFSConfig.Validate()
	}
	return nil
}
