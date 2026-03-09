package config

import "time"

// AppConfig is the unified application configuration.
// It maps directly to config.yaml sections.
type AppConfig struct {
	// Profile holds the user's display name and device name.
	Profile     ProfileConfig     `yaml:"profile" mapstructure:"profile"`
	Network     NetworkConfig     `yaml:"network" mapstructure:"network"`
	Storage     StorageConfig     `yaml:"storage" mapstructure:"storage"`
	Logging     LoggingConfig     `yaml:"logging" mapstructure:"logging"`
	Messaging   MessagingConfig   `yaml:"messaging" mapstructure:"messaging"`
	Mailbox     MailboxConfig     `yaml:"mailbox" mapstructure:"mailbox"`
	RTC         RTCConfig         `yaml:"rtc" mapstructure:"rtc"`
	Multidevice MultideviceConfig `yaml:"multidevice" mapstructure:"multidevice"`
	Identity    IdentityConfig    `yaml:"identity" mapstructure:"identity"`
	Appearance  AppearanceConfig  `yaml:"appearance" mapstructure:"appearance"`
	Privacy     PrivacyConfig     `yaml:"privacy" mapstructure:"privacy"`
	Bootstrap   BootstrapConfig   `yaml:"bootstrap" mapstructure:"bootstrap"`
}

// ProfileConfig holds user profile settings.
type ProfileConfig struct {
	DisplayName string `yaml:"display_name" mapstructure:"display_name"`
	DeviceName  string `yaml:"device_name" mapstructure:"device_name"`
}

// NetworkConfig holds network and libp2p settings.
type NetworkConfig struct {
	BootstrapPeers      []string      `yaml:"bootstrap_peers" mapstructure:"bootstrap_peers"`
	DialTimeout         time.Duration `yaml:"dial_timeout" mapstructure:"dial_timeout"`
	ConnectionTimeout   time.Duration `yaml:"connection_timeout" mapstructure:"connection_timeout"`
	BootstrapTimeout    time.Duration `yaml:"bootstrap_timeout" mapstructure:"bootstrap_timeout"`
	MaxConnections      int           `yaml:"max_connections" mapstructure:"max_connections"`
	LowWater            int           `yaml:"low_water" mapstructure:"low_water"`
	HighWater           int           `yaml:"high_water" mapstructure:"high_water"`
	EnableRelay         bool          `yaml:"enable_relay" mapstructure:"enable_relay"`
	EnableHolePunching  bool          `yaml:"enable_hole_punching" mapstructure:"enable_hole_punching"`
	EnableAutoNAT       bool          `yaml:"enable_autonat" mapstructure:"enable_autonat"`
	DHTMode             string        `yaml:"dht_mode" mapstructure:"dht_mode"`
	DHTBootstrapTimeout time.Duration `yaml:"dht_bootstrap_timeout" mapstructure:"dht_bootstrap_timeout"`
	ListenAddrs         []string      `yaml:"listen_addrs" mapstructure:"listen_addrs"`
	ProtocolID          string        `yaml:"protocol_id" mapstructure:"protocol_id"`
	MaxStoredPeers      int           `yaml:"max_stored_peers" mapstructure:"max_stored_peers"`
	MinPeerConnections  int           `yaml:"min_peer_connections" mapstructure:"min_peer_connections"`
}

// StorageConfig holds storage settings.
type StorageConfig struct {
	Path     string `yaml:"path" mapstructure:"path"`
	InMemory bool   `yaml:"in_memory" mapstructure:"in_memory"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level string `yaml:"level" mapstructure:"level"`
	File  string `yaml:"file" mapstructure:"file"`
}

// MessagingConfig holds messaging settings.
type MessagingConfig struct {
	ChannelBufferSize int `yaml:"channel_buffer_size" mapstructure:"channel_buffer_size"`
}

// MailboxConfig holds mailbox settings.
type MailboxConfig struct {
	MaxMessagesPerTarget   uint32        `yaml:"max_messages_per_target" mapstructure:"max_messages_per_target"`
	MaxMessageSize         uint32        `yaml:"max_message_size" mapstructure:"max_message_size"`
	MaxTotalBytesPerTarget uint64        `yaml:"max_total_bytes_per_target" mapstructure:"max_total_bytes_per_target"`
	DefaultTTL             time.Duration `yaml:"default_ttl" mapstructure:"default_ttl"`
	RateLimitPerMinute     int           `yaml:"rate_limit_per_minute" mapstructure:"rate_limit_per_minute"`
}

// RTCConfig holds real-time call settings.
type RTCConfig struct {
	CallTimeout     time.Duration `yaml:"call_timeout" mapstructure:"call_timeout"`
	EnableVideo     bool          `yaml:"enable_video" mapstructure:"enable_video"`
	MaxParticipants int           `yaml:"max_participants" mapstructure:"max_participants"`
}

// MultideviceConfig holds multi-device settings.
type MultideviceConfig struct {
	DeviceName   string        `yaml:"device_name" mapstructure:"device_name"`
	MaxDevices   int           `yaml:"max_devices" mapstructure:"max_devices"`
	SyncInterval time.Duration `yaml:"sync_interval" mapstructure:"sync_interval"`
}

// IdentityConfig holds identity settings.
type IdentityConfig struct {
	DHTPublish      bool          `yaml:"dht_publish" mapstructure:"dht_publish"`
	RefreshInterval time.Duration `yaml:"refresh_interval" mapstructure:"refresh_interval"`
}

// AppearanceConfig holds UI appearance settings.
type AppearanceConfig struct {
	DarkMode     bool `yaml:"dark_mode" mapstructure:"dark_mode"`
	FontSize     int  `yaml:"font_size" mapstructure:"font_size"`
	WindowWidth  int  `yaml:"window_width" mapstructure:"window_width"`
	WindowHeight int  `yaml:"window_height" mapstructure:"window_height"`
}

// PrivacyConfig holds messaging privacy settings.
type PrivacyConfig struct {
	SendReadReceipts    bool `yaml:"send_read_receipts" mapstructure:"send_read_receipts"`
	SendTypingIndicators bool `yaml:"send_typing_indicators" mapstructure:"send_typing_indicators"`
}

// BootstrapConfig holds configuration for the hybrid bootstrap mechanism
type BootstrapConfig struct {
	// PubSubTopic is the topic name for bootstrap discovery
	PubSubTopic string `yaml:"pubsub_topic" mapstructure:"pubsub_topic"`
	// ResponseProbability is the probability of responding to a bootstrap request (0.0-1.0)
	ResponseProbability float64 `yaml:"response_probability" mapstructure:"response_probability"`
	// MaxResponsesPerMinute is the maximum number of responses allowed per minute
	MaxResponsesPerMinute int `yaml:"max_responses_per_minute" mapstructure:"max_responses_per_minute"`
	// RequestDedupWindowSecs is the time window for request deduplication in seconds
	RequestDedupWindowSecs int `yaml:"request_dedup_window_seconds" mapstructure:"request_dedup_window_seconds"`
	// MinUptimeSecs is the minimum uptime required to qualify as a helper node
	MinUptimeSecs int `yaml:"min_uptime_secs" mapstructure:"min_uptime_secs"`
	// MinPeerCount is the minimum number of connected peers required to qualify as a helper
	MinPeerCount int `yaml:"min_peer_count" mapstructure:"min_peer_count"`
	// MinRoutingTableSize is the minimum DHT routing table size required to qualify as a helper
	MinRoutingTableSize int `yaml:"min_routing_table_size" mapstructure:"min_routing_table_size"`
	// StoredPeerTimeoutSecs is the maximum age of stored peers before they're considered stale
	StoredPeerTimeoutSecs int `yaml:"stored_peer_timeout_seconds" mapstructure:"stored_peer_timeout_seconds"`
	// PubSubListenSecs is how long to listen for responses during bootstrap
	PubSubListenSecs int `yaml:"pubsub_listen_seconds" mapstructure:"pubsub_listen_seconds"`
	// MinBabylonPeersRequired is the minimum number of Babylon peers needed
	MinBabylonPeersRequired int `yaml:"min_babylon_peers_required" mapstructure:"min_babylon_peers_required"`
}

// ToIPFSConfig returns a pointer to the Network section, since IPFSConfig is
// now a type alias for NetworkConfig.
func (c *AppConfig) ToIPFSConfig() *NetworkConfig {
	return &c.Network
}
