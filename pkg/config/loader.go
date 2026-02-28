package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// LoadAppConfig loads the unified configuration with the following precedence:
//
//	CLI flags > env vars (BABYLONTOWER_ prefix) > config.yaml > defaults
//
// dataDir is the base data directory (e.g. ~/.babylontower).
// configPath overrides the config file location (empty = auto-detect in dataDir).
func LoadAppConfig(dataDir, configPath string) (*AppConfig, error) {
	v := viper.New()

	// 1. Set defaults from DefaultAppConfig()
	setViperDefaults(v)

	// 2. Load config file
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else if dataDir != "" {
		v.AddConfigPath(dataDir)
	}
	// Config file is optional — ignore "not found" errors
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Only return real errors; missing file is fine
			if !strings.Contains(err.Error(), "Not Found") {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
		}
	}

	// 3. Environment variable overrides
	v.SetEnvPrefix("BABYLONTOWER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 4. Unmarshal into AppConfig
	cfg := DefaultAppConfig()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}

// setViperDefaults registers all default values so that env vars can be
// resolved even when no config file is present.
func setViperDefaults(v *viper.Viper) {
	d := DefaultAppConfig()

	// Network
	v.SetDefault("network.bootstrap_peers", d.Network.BootstrapPeers)
	v.SetDefault("network.dial_timeout", d.Network.DialTimeout)
	v.SetDefault("network.connection_timeout", d.Network.ConnectionTimeout)
	v.SetDefault("network.bootstrap_timeout", d.Network.BootstrapTimeout)
	v.SetDefault("network.max_connections", d.Network.MaxConnections)
	v.SetDefault("network.low_water", d.Network.LowWater)
	v.SetDefault("network.high_water", d.Network.HighWater)
	v.SetDefault("network.enable_relay", d.Network.EnableRelay)
	v.SetDefault("network.enable_hole_punching", d.Network.EnableHolePunching)
	v.SetDefault("network.enable_autonat", d.Network.EnableAutoNAT)
	v.SetDefault("network.dht_mode", d.Network.DHTMode)
	v.SetDefault("network.dht_bootstrap_timeout", d.Network.DHTBootstrapTimeout)
	v.SetDefault("network.listen_addrs", d.Network.ListenAddrs)
	v.SetDefault("network.protocol_id", d.Network.ProtocolID)
	v.SetDefault("network.max_stored_peers", d.Network.MaxStoredPeers)
	v.SetDefault("network.min_peer_connections", d.Network.MinPeerConnections)

	// Storage
	v.SetDefault("storage.path", d.Storage.Path)
	v.SetDefault("storage.in_memory", d.Storage.InMemory)

	// Logging
	v.SetDefault("logging.level", d.Logging.Level)
	v.SetDefault("logging.file", d.Logging.File)

	// Messaging
	v.SetDefault("messaging.channel_buffer_size", d.Messaging.ChannelBufferSize)

	// Mailbox
	v.SetDefault("mailbox.max_messages_per_target", d.Mailbox.MaxMessagesPerTarget)
	v.SetDefault("mailbox.max_message_size", d.Mailbox.MaxMessageSize)
	v.SetDefault("mailbox.max_total_bytes_per_target", d.Mailbox.MaxTotalBytesPerTarget)
	v.SetDefault("mailbox.default_ttl", d.Mailbox.DefaultTTL)
	v.SetDefault("mailbox.rate_limit_per_minute", d.Mailbox.RateLimitPerMinute)

	// RTC
	v.SetDefault("rtc.call_timeout", d.RTC.CallTimeout)
	v.SetDefault("rtc.enable_video", d.RTC.EnableVideo)
	v.SetDefault("rtc.max_participants", d.RTC.MaxParticipants)

	// Multidevice
	v.SetDefault("multidevice.device_name", d.Multidevice.DeviceName)
	v.SetDefault("multidevice.max_devices", d.Multidevice.MaxDevices)
	v.SetDefault("multidevice.sync_interval", d.Multidevice.SyncInterval)

	// Identity
	v.SetDefault("identity.dht_publish", d.Identity.DHTPublish)
	v.SetDefault("identity.refresh_interval", d.Identity.RefreshInterval)
}
