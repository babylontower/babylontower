package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
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

// SaveMinimalConfig writes a minimal config.yaml with the profile section to
// the given data directory. It only writes fields explicitly set, avoiding
// dumping all defaults.
func SaveMinimalConfig(dataDir string, profile ProfileConfig) error {
	type minimalConfig struct {
		Profile ProfileConfig `yaml:"profile"`
	}
	cfg := minimalConfig{Profile: profile}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := filepath.Join(dataDir, "config.yaml")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

// SaveAppConfig writes the full AppConfig to config.yaml in the given data directory.
func SaveAppConfig(dataDir string, cfg *AppConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := filepath.Join(dataDir, "config.yaml")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

// setViperDefaults registers all default values so that env vars can be
// resolved even when no config file is present.
func setViperDefaults(v *viper.Viper) {
	d := DefaultAppConfig()

	// Profile
	v.SetDefault("profile.display_name", d.Profile.DisplayName)
	v.SetDefault("profile.device_name", d.Profile.DeviceName)

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

	// Appearance
	v.SetDefault("appearance.dark_mode", d.Appearance.DarkMode)
	v.SetDefault("appearance.font_size", d.Appearance.FontSize)
	v.SetDefault("appearance.window_width", d.Appearance.WindowWidth)
	v.SetDefault("appearance.window_height", d.Appearance.WindowHeight)

	// Privacy
	v.SetDefault("privacy.send_read_receipts", d.Privacy.SendReadReceipts)
	v.SetDefault("privacy.send_typing_indicators", d.Privacy.SendTypingIndicators)

	// Identity
	v.SetDefault("identity.dht_publish", d.Identity.DHTPublish)
	v.SetDefault("identity.refresh_interval", d.Identity.RefreshInterval)
}
