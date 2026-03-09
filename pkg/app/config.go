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
	// DisplayName is the user's chosen display name
	DisplayName string
	// DeviceName is the name of this device
	DeviceName string
	// NetworkConfig is the network/IPFS node configuration
	NetworkConfig *config.NetworkConfig
}

// DefaultAppConfig returns an AppConfig with sensible defaults.
func DefaultAppConfig() *AppConfig {
	return &AppConfig{
		LogLevel:   "warn",
		NetworkConfig: config.DefaultIPFSConfig(),
	}
}

// Validate validates the application configuration.
func (c *AppConfig) Validate() error {
	if c.NetworkConfig != nil {
		return c.NetworkConfig.Validate()
	}
	return nil
}
