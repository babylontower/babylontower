// Package protocol implements the Babylon Tower Protocol v1 specification.
package protocol

import (
	"errors"
	"fmt"
	"time"
)

// ProtocolConfig holds all configuration for the Babylon Tower protocol.
// It provides settings for identity management, session handling, DHT operations,
// and peer discovery. All configuration values have sensible defaults.
type ProtocolConfig struct {
	// ========================================================================
	// Identity Configuration
	// ========================================================================

	// IdentityFilePath is the path to the local identity file
	IdentityFilePath string `json:"identity_file_path" yaml:"identity_file_path"`
	// DeviceName is the human-readable name for this device
	DeviceName string `json:"device_name" yaml:"device_name"`
	// DeviceExpiresAt is when the device certificate expires (0 = no expiry)
	DeviceExpiresAt uint64 `json:"device_expires_at" yaml:"device_expires_at"`

	// ========================================================================
	// Prekey Configuration
	// ========================================================================

	// PrekeyTargetCount is the target number of one-time prekeys to maintain
	PrekeyTargetCount int `json:"prekey_target_count" yaml:"prekey_target_count"`
	// PrekeyReplenishThreshold is the threshold for prekey replenishment
	PrekeyReplenishThreshold int `json:"prekey_replenish_threshold" yaml:"prekey_replenish_threshold"`
	// PrekeyBatchSize is the number of prekeys to generate in a batch
	PrekeyBatchSize int `json:"prekey_batch_size" yaml:"prekey_batch_size"`
	// SignedPrekeyRotationInterval is how often signed prekeys are rotated
	SignedPrekeyRotationInterval time.Duration `json:"signed_prekey_rotation_interval" yaml:"signed_prekey_rotation_interval"`
	// SignedPrekeyOverlapPeriod is the overlap period for signed prekey rotation
	SignedPrekeyOverlapPeriod time.Duration `json:"signed_prekey_overlap_period" yaml:"signed_prekey_overlap_period"`
	// SignedPrekeyMaxAge is the maximum age before rejection
	SignedPrekeyMaxAge time.Duration `json:"signed_prekey_max_age" yaml:"signed_prekey_max_age"`

	// ========================================================================
	// Session Configuration
	// ========================================================================

	// MaxStoredSessions is the maximum number of sessions to store
	MaxStoredSessions int `json:"max_stored_sessions" yaml:"max_stored_sessions"`
	// MaxSessionAge is the maximum age for idle sessions before cleanup
	MaxSessionAge time.Duration `json:"max_session_age" yaml:"max_session_age"`
	// SessionCleanupInterval is how often session cleanup runs
	SessionCleanupInterval time.Duration `json:"session_cleanup_interval" yaml:"session_cleanup_interval"`
	// MaxSkippedKeys is the maximum number of skipped message keys to cache
	MaxSkippedKeys int `json:"max_skipped_keys" yaml:"max_skipped_keys"`

	// ========================================================================
	// DHT Configuration
	// ========================================================================

	// DHTMode is the DHT mode: "auto", "client", or "server"
	DHTMode string `json:"dht_mode" yaml:"dht_mode"`
	// DHTBootstrapTimeout is the timeout for DHT bootstrap operations
	DHTBootstrapTimeout time.Duration `json:"dht_bootstrap_timeout" yaml:"dht_bootstrap_timeout"`
	// DHTOperationTimeout is the timeout for individual DHT operations
	DHTOperationTimeout time.Duration `json:"dht_operation_timeout" yaml:"dht_operation_timeout"`
	// IdentityRepublishInterval is how often identity documents are republished
	IdentityRepublishInterval time.Duration `json:"identity_republish_interval" yaml:"identity_republish_interval"`
	// IdentityRecordTTL is the TTL for identity DHT records
	IdentityRecordTTL time.Duration `json:"identity_record_ttl" yaml:"identity_record_ttl"`

	// ========================================================================
	// Bootstrap Configuration
	// ========================================================================

	// BootstrapTimeout is the timeout for bootstrap operations
	BootstrapTimeout time.Duration `json:"bootstrap_timeout" yaml:"bootstrap_timeout"`
	// MinBabylonPeersRequired is the minimum number of Babylon peers needed
	MinBabylonPeersRequired int `json:"min_babylon_peers_required" yaml:"min_babylon_peers_required"`
	// RendezvousNamespace is the DHT rendezvous namespace for discovering Babylon nodes
	RendezvousNamespace string `json:"rendezvous_namespace" yaml:"rendezvous_namespace"`

	// ========================================================================
	// Discovery Configuration
	// ========================================================================

	// DiscoveryInterval is how often to run peer discovery
	DiscoveryInterval time.Duration `json:"discovery_interval" yaml:"discovery_interval"`
	// MaxDiscoveredPeers is the maximum number of peers to discover per round
	MaxDiscoveredPeers int `json:"max_discovered_peers" yaml:"max_discovered_peers"`
	// PeerConnectTimeout is the timeout for peer connections
	PeerConnectTimeout time.Duration `json:"peer_connect_timeout" yaml:"peer_connect_timeout"`

	// ========================================================================
	// Contact Configuration
	// ========================================================================

	// MaxContacts is the maximum number of contacts to track
	MaxContacts int `json:"max_contacts" yaml:"max_contacts"`
	// ContactDiscoveryTimeout is the timeout for contact discovery
	ContactDiscoveryTimeout time.Duration `json:"contact_discovery_timeout" yaml:"contact_discovery_timeout"`

	// ========================================================================
	// Logging Configuration
	// ========================================================================

	// LogLevel is the logging level: "debug", "info", "warn", "error"
	LogLevel string `json:"log_level" yaml:"log_level"`
	// EnableMetrics enables protocol metrics collection
	EnableMetrics bool `json:"enable_metrics" yaml:"enable_metrics"`
}

// DefaultProtocolConfig returns a ProtocolConfig with sensible defaults.
// These defaults are based on the protocol-v2.md specification and
// production deployment experience.
func DefaultProtocolConfig() *ProtocolConfig {
	return &ProtocolConfig{
		// Identity
		IdentityFilePath: "~/.babylontower/identity.json",
		DeviceName:       "Babylon Tower Device",
		DeviceExpiresAt:  0, // No expiry by default

		// Prekeys
		PrekeyTargetCount:          DefaultPrekeyTargetCount,
		PrekeyReplenishThreshold:   MinPrekeyThreshold,
		PrekeyBatchSize:            80,
		SignedPrekeyRotationInterval: SignedPrekeyRotationInterval,
		SignedPrekeyOverlapPeriod:  SignedPrekeyOverlapPeriod,
		SignedPrekeyMaxAge:         SignedPrekeyMaxAge,

		// Sessions
		MaxStoredSessions:      MaxStoredSessions,
		MaxSessionAge:          MaxSessionAge,
		SessionCleanupInterval: SessionCleanupInterval,
		MaxSkippedKeys:         MaxSkippedKeys,

		// DHT
		DHTMode:                   "auto",
		DHTBootstrapTimeout:       DefaultBootstrapTimeout,
		DHTOperationTimeout:       DefaultDHTTimeout,
		IdentityRepublishInterval: IdentityRepublishInterval,
		IdentityRecordTTL:         IdentityRecordTTL,

		// Bootstrap
		BootstrapTimeout:             DefaultBootstrapTimeout,
		MinBabylonPeersRequired:      3,
		RendezvousNamespace:          "babylon/rendezvous/v1",

		// Discovery
		DiscoveryInterval:     30 * time.Second,
		MaxDiscoveredPeers:    20,
		PeerConnectTimeout:    DefaultPeerConnectTimeout,

		// Contacts
		MaxContacts:             MaxContacts,
		ContactDiscoveryTimeout: DefaultDHTTimeout,

		// Logging
		LogLevel:      "info",
		EnableMetrics: true,
	}
}

// Validate validates the protocol configuration.
// It returns an error if any configuration values are invalid.
func (c *ProtocolConfig) Validate() error {
	var errs []error

	// Validate identity configuration
	if c.IdentityFilePath == "" {
		errs = append(errs, errors.New("identity_file_path is required"))
	}
	if c.DeviceName == "" {
		errs = append(errs, errors.New("device_name is required"))
	}

	// Validate prekey configuration
	if c.PrekeyTargetCount < 10 {
		errs = append(errs, fmt.Errorf("prekey_target_count must be >= 10, got %d", c.PrekeyTargetCount))
	}
	if c.PrekeyReplenishThreshold < 5 {
		errs = append(errs, fmt.Errorf("prekey_replenish_threshold must be >= 5, got %d", c.PrekeyReplenishThreshold))
	}
	if c.PrekeyReplenishThreshold >= c.PrekeyTargetCount {
		errs = append(errs, fmt.Errorf("prekey_replenish_threshold must be < prekey_target_count"))
	}
	if c.PrekeyBatchSize < 10 {
		errs = append(errs, fmt.Errorf("prekey_batch_size must be >= 10, got %d", c.PrekeyBatchSize))
	}
	if c.SignedPrekeyRotationInterval < 1*time.Hour {
		errs = append(errs, fmt.Errorf("signed_prekey_rotation_interval must be >= 1h"))
	}
	if c.SignedPrekeyOverlapPeriod < 1*time.Minute {
		errs = append(errs, fmt.Errorf("signed_prekey_overlap_period must be >= 1m"))
	}
	if c.SignedPrekeyMaxAge < c.SignedPrekeyRotationInterval {
		errs = append(errs, fmt.Errorf("signed_prekey_max_age must be >= signed_prekey_rotation_interval"))
	}

	// Validate session configuration
	if c.MaxStoredSessions < 10 {
		errs = append(errs, fmt.Errorf("max_stored_sessions must be >= 10, got %d", c.MaxStoredSessions))
	}
	if c.MaxSessionAge < 1*time.Hour {
		errs = append(errs, fmt.Errorf("max_session_age must be >= 1h"))
	}
	if c.SessionCleanupInterval < 1*time.Minute {
		errs = append(errs, fmt.Errorf("session_cleanup_interval must be >= 1m"))
	}
	if c.MaxSkippedKeys < 10 {
		errs = append(errs, fmt.Errorf("max_skipped_keys must be >= 10, got %d", c.MaxSkippedKeys))
	}
	if c.MaxSkippedKeys > 1000 {
		errs = append(errs, fmt.Errorf("max_skipped_keys must be <= 1000, got %d", c.MaxSkippedKeys))
	}

	// Validate DHT configuration
	switch c.DHTMode {
	case "auto", "client", "server":
		// Valid
	default:
		errs = append(errs, fmt.Errorf("invalid dht_mode: %s (must be auto, client, or server)", c.DHTMode))
	}
	if c.DHTBootstrapTimeout < 10*time.Second {
		errs = append(errs, fmt.Errorf("dht_bootstrap_timeout must be >= 10s"))
	}
	if c.DHTOperationTimeout < 5*time.Second {
		errs = append(errs, fmt.Errorf("dht_operation_timeout must be >= 5s"))
	}
	if c.IdentityRepublishInterval < 1*time.Hour {
		errs = append(errs, fmt.Errorf("identity_republish_interval must be >= 1h"))
	}
	if c.IdentityRecordTTL < 1*time.Hour {
		errs = append(errs, fmt.Errorf("identity_record_ttl must be >= 1h"))
	}

	// Validate bootstrap configuration
	if c.BootstrapTimeout < 10*time.Second {
		errs = append(errs, fmt.Errorf("bootstrap_timeout must be >= 10s"))
	}
	if c.MinBabylonPeersRequired < 1 {
		errs = append(errs, fmt.Errorf("min_babylon_peers_required must be >= 1"))
	}
	// Validate discovery configuration
	if c.DiscoveryInterval < 5*time.Second {
		errs = append(errs, fmt.Errorf("discovery_interval must be >= 5s"))
	}
	if c.MaxDiscoveredPeers < 1 {
		errs = append(errs, fmt.Errorf("max_discovered_peers must be >= 1"))
	}
	if c.PeerConnectTimeout < 5*time.Second {
		errs = append(errs, fmt.Errorf("peer_connect_timeout must be >= 5s"))
	}

	// Validate contact configuration
	if c.MaxContacts < 10 {
		errs = append(errs, fmt.Errorf("max_contacts must be >= 10, got %d", c.MaxContacts))
	}
	if c.ContactDiscoveryTimeout < 5*time.Second {
		errs = append(errs, fmt.Errorf("contact_discovery_timeout must be >= 5s"))
	}

	// Validate logging configuration
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
		// Valid
	default:
		errs = append(errs, fmt.Errorf("invalid log_level: %s (must be debug, info, warn, or error)", c.LogLevel))
	}

	if len(errs) > 0 {
		return fmt.Errorf("protocol config validation failed: %v", errs)
	}
	return nil
}

// WithIdentityFilePath sets the identity file path.
func (c *ProtocolConfig) WithIdentityFilePath(path string) *ProtocolConfig {
	c.IdentityFilePath = path
	return c
}

// WithDeviceName sets the device name.
func (c *ProtocolConfig) WithDeviceName(name string) *ProtocolConfig {
	c.DeviceName = name
	return c
}

// WithDHTMode sets the DHT mode.
func (c *ProtocolConfig) WithDHTMode(mode string) *ProtocolConfig {
	c.DHTMode = mode
	return c
}

// WithLogLevel sets the logging level.
func (c *ProtocolConfig) WithLogLevel(level string) *ProtocolConfig {
	c.LogLevel = level
	return c
}

// WithBootstrapTimeout sets the bootstrap timeout.
func (c *ProtocolConfig) WithBootstrapTimeout(timeout time.Duration) *ProtocolConfig {
	c.BootstrapTimeout = timeout
	return c
}

// WithMinBabylonPeers sets the minimum Babylon peers required.
func (c *ProtocolConfig) WithMinBabylonPeers(count int) *ProtocolConfig {
	c.MinBabylonPeersRequired = count
	return c
}

// WithMaxSessions sets the maximum stored sessions.
func (c *ProtocolConfig) WithMaxSessions(count int) *ProtocolConfig {
	c.MaxStoredSessions = count
	return c
}

// WithPrekeyTarget sets the target prekey count.
func (c *ProtocolConfig) WithPrekeyTarget(count int) *ProtocolConfig {
	c.PrekeyTargetCount = count
	return c
}
