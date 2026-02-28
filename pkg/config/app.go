package config

import "time"

// AppConfig is the unified application configuration.
// It maps directly to config.yaml sections.
type AppConfig struct {
	Network     NetworkConfig     `mapstructure:"network"`
	Storage     StorageConfig     `mapstructure:"storage"`
	Logging     LoggingConfig     `mapstructure:"logging"`
	Messaging   MessagingConfig   `mapstructure:"messaging"`
	Mailbox     MailboxConfig     `mapstructure:"mailbox"`
	RTC         RTCConfig         `mapstructure:"rtc"`
	Multidevice MultideviceConfig `mapstructure:"multidevice"`
	Identity    IdentityConfig    `mapstructure:"identity"`
}

// NetworkConfig holds network and libp2p settings.
type NetworkConfig struct {
	BootstrapPeers    []string      `mapstructure:"bootstrap_peers"`
	DialTimeout       time.Duration `mapstructure:"dial_timeout"`
	ConnectionTimeout time.Duration `mapstructure:"connection_timeout"`
	BootstrapTimeout  time.Duration `mapstructure:"bootstrap_timeout"`
	MaxConnections    int           `mapstructure:"max_connections"`
	LowWater          int           `mapstructure:"low_water"`
	HighWater          int           `mapstructure:"high_water"`
	EnableRelay       bool          `mapstructure:"enable_relay"`
	EnableHolePunching bool         `mapstructure:"enable_hole_punching"`
	EnableAutoNAT     bool          `mapstructure:"enable_autonat"`
	DHTMode           string        `mapstructure:"dht_mode"`
	DHTBootstrapTimeout time.Duration `mapstructure:"dht_bootstrap_timeout"`
	ListenAddrs       []string      `mapstructure:"listen_addrs"`
	ProtocolID        string        `mapstructure:"protocol_id"`
	MaxStoredPeers    int           `mapstructure:"max_stored_peers"`
	MinPeerConnections int          `mapstructure:"min_peer_connections"`
}

// StorageConfig holds storage settings.
type StorageConfig struct {
	Path     string `mapstructure:"path"`
	InMemory bool   `mapstructure:"in_memory"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level string `mapstructure:"level"`
	File  string `mapstructure:"file"`
}

// MessagingConfig holds messaging settings.
type MessagingConfig struct {
	ChannelBufferSize int `mapstructure:"channel_buffer_size"`
}

// MailboxConfig holds mailbox settings.
type MailboxConfig struct {
	MaxMessagesPerTarget   uint32        `mapstructure:"max_messages_per_target"`
	MaxMessageSize         uint32        `mapstructure:"max_message_size"`
	MaxTotalBytesPerTarget uint64        `mapstructure:"max_total_bytes_per_target"`
	DefaultTTL             time.Duration `mapstructure:"default_ttl"`
	RateLimitPerMinute     int           `mapstructure:"rate_limit_per_minute"`
}

// RTCConfig holds real-time call settings.
type RTCConfig struct {
	CallTimeout     time.Duration `mapstructure:"call_timeout"`
	EnableVideo     bool          `mapstructure:"enable_video"`
	MaxParticipants int           `mapstructure:"max_participants"`
}

// MultideviceConfig holds multi-device settings.
type MultideviceConfig struct {
	DeviceName   string        `mapstructure:"device_name"`
	MaxDevices   int           `mapstructure:"max_devices"`
	SyncInterval time.Duration `mapstructure:"sync_interval"`
}

// IdentityConfig holds identity settings.
type IdentityConfig struct {
	DHTPublish      bool          `mapstructure:"dht_publish"`
	RefreshInterval time.Duration `mapstructure:"refresh_interval"`
}

// ToIPFSConfig converts the unified AppConfig into the IPFSConfig consumed by
// the ipfsnode package, keeping full backward compatibility.
func (c *AppConfig) ToIPFSConfig() *IPFSConfig {
	return &IPFSConfig{
		BootstrapPeers:     c.Network.BootstrapPeers,
		BootstrapTimeout:   c.Network.BootstrapTimeout,
		MaxStoredPeers:     c.Network.MaxStoredPeers,
		MinPeerConnections: c.Network.MinPeerConnections,
		ConnectionTimeout:  c.Network.ConnectionTimeout,
		DialTimeout:        c.Network.DialTimeout,
		MaxConnections:     c.Network.MaxConnections,
		LowWater:           c.Network.LowWater,
		HighWater:          c.Network.HighWater,
		EnableRelay:        c.Network.EnableRelay,
		EnableHolePunching: c.Network.EnableHolePunching,
		EnableAutoNAT:      c.Network.EnableAutoNAT,
		DHTMode:            c.Network.DHTMode,
		DHTBootstrapTimeout: c.Network.DHTBootstrapTimeout,
		ListenAddrs:        c.Network.ListenAddrs,
		ProtocolID:         c.Network.ProtocolID,
	}
}
