package config

import (
	"fmt"

	bterrors "babylontower/pkg/errors"
)

// ValidateAppConfig validates the unified application configuration.
// Returns a wrapped ErrInvalidConfig on failure.
func ValidateAppConfig(cfg *AppConfig) error {
	// Network validation
	if cfg.Network.LowWater < 0 {
		return fmt.Errorf("%w: low_water must be non-negative", bterrors.ErrInvalidConfig)
	}
	if cfg.Network.HighWater < cfg.Network.LowWater {
		return fmt.Errorf("%w: high_water must be >= low_water", bterrors.ErrInvalidConfig)
	}
	if cfg.Network.MaxConnections < cfg.Network.HighWater {
		return fmt.Errorf("%w: max_connections must be >= high_water", bterrors.ErrInvalidConfig)
	}
	if cfg.Network.DialTimeout <= 0 {
		return fmt.Errorf("%w: dial_timeout must be positive", bterrors.ErrInvalidConfig)
	}
	if cfg.Network.ConnectionTimeout <= 0 {
		return fmt.Errorf("%w: connection_timeout must be positive", bterrors.ErrInvalidConfig)
	}
	if cfg.Network.BootstrapTimeout <= 0 {
		return fmt.Errorf("%w: bootstrap_timeout must be positive", bterrors.ErrInvalidConfig)
	}
	switch cfg.Network.DHTMode {
	case "auto", "client", "server":
		// valid
	default:
		return fmt.Errorf("%w: invalid dht_mode %q (must be auto, client, or server)", bterrors.ErrInvalidConfig, cfg.Network.DHTMode)
	}
	if cfg.Network.MaxStoredPeers < 10 {
		return fmt.Errorf("%w: max_stored_peers must be >= 10", bterrors.ErrInvalidConfig)
	}
	if cfg.Network.MinPeerConnections < 1 {
		return fmt.Errorf("%w: min_peer_connections must be >= 1", bterrors.ErrInvalidConfig)
	}

	// Logging validation
	switch cfg.Logging.Level {
	case "debug", "info", "warn", "error", "":
		// valid (empty string will use default)
	default:
		return fmt.Errorf("%w: invalid log level %q", bterrors.ErrInvalidConfig, cfg.Logging.Level)
	}

	// Messaging validation
	if cfg.Messaging.ChannelBufferSize < 1 {
		return fmt.Errorf("%w: channel_buffer_size must be >= 1", bterrors.ErrInvalidConfig)
	}

	// Mailbox validation
	if cfg.Mailbox.MaxMessageSize == 0 {
		return fmt.Errorf("%w: max_message_size must be > 0", bterrors.ErrInvalidConfig)
	}

	// RTC validation
	if cfg.RTC.MaxParticipants < 2 {
		return fmt.Errorf("%w: max_participants must be >= 2", bterrors.ErrInvalidConfig)
	}

	// Multidevice validation
	if cfg.Multidevice.MaxDevices < 1 {
		return fmt.Errorf("%w: max_devices must be >= 1", bterrors.ErrInvalidConfig)
	}

	return nil
}
