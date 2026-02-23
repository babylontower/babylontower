// Package config provides configuration management for Babylon Tower.
// It supports IPFS node configuration, bootstrap peers, and connection settings.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/multiformats/go-multiaddr"
)

const (
	// Default protocol ID for Babylon Tower
	DefaultProtocolID = "/babylontower/1.0.0"

	// Default timeouts
	DefaultBootstrapTimeout   = 60 * time.Second
	DefaultConnectionTimeout  = 30 * time.Second
	DefaultDialTimeout        = 15 * time.Second
	DefaultDHTBootstrapTimeout = 60 * time.Second

	// Default connection limits
	DefaultMaxConnections = 400
	DefaultLowWater       = 50
	DefaultHighWater      = 400

	// Default peer storage limits
	DefaultMaxStoredPeers     = 100
	DefaultMinPeerConnections = 10

	// Default listen addresses
	DefaultListenAddrTCP  = "/ip4/0.0.0.0/tcp/0"
	DefaultListenAddrWS   = "/ip4/0.0.0.0/tcp/0/ws"
	DefaultListenAddrTCPv6 = "/ip6/::/tcp/0"
)

// PeerSource indicates where a peer was discovered
type PeerSource string

const (
	SourceBootstrap   PeerSource = "bootstrap"
	SourceDHT         PeerSource = "dht"
	SourceMDNS        PeerSource = "mdns"
	SourcePeerExchange PeerSource = "peer_exchange"
)

// IPFSConfig holds configuration for the IPFS node
type IPFSConfig struct {
	// Bootstrap configuration
	BootstrapPeers      []string        `json:"bootstrap_peers"`
	BootstrapTimeout    time.Duration   `json:"bootstrap_timeout"`

	// Peer persistence
	MaxStoredPeers      int             `json:"max_stored_peers"`
	MinPeerConnections  int             `json:"min_peer_connections"`

	// Connection management
	ConnectionTimeout   time.Duration   `json:"connection_timeout"`
	DialTimeout         time.Duration   `json:"dial_timeout"`
	MaxConnections      int             `json:"max_connections"`
	LowWater            int             `json:"low_water"`
	HighWater           int             `json:"high_water"`

	// NAT traversal
	EnableRelay         bool            `json:"enable_relay"`
	EnableHolePunching  bool            `json:"enable_hole_punching"`
	EnableAutoNAT       bool            `json:"enable_autonat"`

	// DHT configuration
	DHTMode             string          `json:"dht_mode"`
	DHTBootstrapTimeout time.Duration   `json:"dht_bootstrap_timeout"`

	// Network configuration
	ListenAddrs         []string        `json:"listen_addrs"`
	ProtocolID          string          `json:"protocol_id"`
}

// DefaultIPFSConfig returns an IPFSConfig with sensible defaults
func DefaultIPFSConfig() *IPFSConfig {
	return &IPFSConfig{
		// Bootstrap peers - mix of DNS and IP addresses for redundancy
		BootstrapPeers: []string{
			// Primary libp2p bootstrap nodes (dnsaddr)
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiRNN6vEC9qmL9egu92p",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
			// Direct IP bootstrap nodes (fallback)
			"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
			"/ip4/104.236.179.241/tcp/4001/p2p/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
			"/ip4/128.199.219.111/tcp/4001/p2p/QmSoLV4Bbm51jM9C4gDYZQ9Cy3U6aXMJDAbzgu2fzaDs64",
			"/ip4/104.236.76.40/tcp/4001/p2p/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
			"/ip4/178.62.158.147/tcp/4001/p2p/QmSoLer265NRgSp2LA3dPaeykiS1J6DifTC88f5uVQKNAd",
			"/ip4/178.62.61.185/tcp/4001/p2p/QmSoLMeWqB7YGVLJN3pNLQpmmEk35v6wYtsMGLzSr5QBU3",
		},
		BootstrapTimeout:      DefaultBootstrapTimeout,

		// Peer storage
		MaxStoredPeers:      DefaultMaxStoredPeers,
		MinPeerConnections:  DefaultMinPeerConnections,

		// Connection management
		ConnectionTimeout:   DefaultConnectionTimeout,
		DialTimeout:         DefaultDialTimeout,
		MaxConnections:      DefaultMaxConnections,
		LowWater:            DefaultLowWater,
		HighWater:           DefaultHighWater,

		// NAT traversal
		EnableRelay:         false,
		EnableHolePunching:  true,
		EnableAutoNAT:       true,

		// DHT
		DHTMode:             "auto",
		DHTBootstrapTimeout: DefaultDHTBootstrapTimeout,

		// Network
		ListenAddrs: []string{
			DefaultListenAddrTCP,
			DefaultListenAddrWS,
			DefaultListenAddrTCPv6,
		},
		ProtocolID:          DefaultProtocolID,
	}
}

// LoadFromFile loads configuration from a JSON file
func LoadFromFile(path string) (*IPFSConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := DefaultIPFSConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// SaveToFile saves configuration to a JSON file
func (c *IPFSConfig) SaveToFile(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate validates the configuration
func (c *IPFSConfig) Validate() error {
	// Validate bootstrap peers
	for i, peer := range c.BootstrapPeers {
		if err := validateMultiaddr(peer); err != nil {
			return fmt.Errorf("invalid bootstrap peer %d: %w", i, err)
		}
	}

	// Validate listen addresses
	for i, addr := range c.ListenAddrs {
		if err := validateMultiaddr(addr); err != nil {
			return fmt.Errorf("invalid listen address %d: %w", i, err)
		}
	}

	// Validate connection limits
	if c.LowWater < 0 {
		return fmt.Errorf("low_water must be non-negative")
	}
	if c.HighWater < c.LowWater {
		return fmt.Errorf("high_water must be >= low_water")
	}
	if c.MaxConnections < c.HighWater {
		return fmt.Errorf("max_connections must be >= high_water")
	}

	// Validate timeouts
	if c.BootstrapTimeout <= 0 {
		return fmt.Errorf("bootstrap_timeout must be positive")
	}
	if c.ConnectionTimeout <= 0 {
		return fmt.Errorf("connection_timeout must be positive")
	}
	if c.DialTimeout <= 0 {
		return fmt.Errorf("dial_timeout must be positive")
	}

	// Validate DHT mode
	switch c.DHTMode {
	case "auto", "client", "server":
		// Valid
	default:
		return fmt.Errorf("invalid dht_mode: %s (must be auto, client, or server)", c.DHTMode)
	}

	// Validate peer storage
	if c.MaxStoredPeers < 10 {
		return fmt.Errorf("max_stored_peers must be >= 10")
	}
	if c.MinPeerConnections < 1 {
		return fmt.Errorf("min_peer_connections must be >= 1")
	}

	return nil
}

// validateMultiaddr validates a multiaddr string
func validateMultiaddr(addr string) error {
	_, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return fmt.Errorf("invalid multiaddr %q: %w", addr, err)
	}
	return nil
}

// GetBootstrapPeerInfos parses bootstrap peers into AddrInfo format
func (c *IPFSConfig) GetBootstrapPeerInfos() ([]multiaddr.Multiaddr, error) {
	addrs := make([]multiaddr.Multiaddr, 0, len(c.BootstrapPeers))
	for _, peerStr := range c.BootstrapPeers {
		ma, err := multiaddr.NewMultiaddr(peerStr)
		if err != nil {
			return nil, fmt.Errorf("invalid bootstrap peer %q: %w", peerStr, err)
		}
		addrs = append(addrs, ma)
	}
	return addrs, nil
}

// ToMap converts the config to a map for storage
func (c *IPFSConfig) ToMap() (map[string]string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"ipfs_config": string(data),
	}, nil
}

// FromMap loads the config from a map
func (c *IPFSConfig) FromMap(m map[string]string) error {
	data, ok := m["ipfs_config"]
	if !ok {
		return fmt.Errorf("ipfs_config key not found")
	}

	return json.Unmarshal([]byte(data), c)
}

// PeerRecord represents a discovered peer for persistence
type PeerRecord struct {
	PeerID        string    `json:"peer_id"`
	Multiaddrs    []string  `json:"multiaddrs"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	LastConnected time.Time `json:"last_connected"`
	ConnectCount  int       `json:"connect_count"`
	FailCount     int       `json:"fail_count"`
	Source        PeerSource `json:"source"`
	Protocols     []string  `json:"protocols"`
	LatencyMs     int64     `json:"latency_ms"`
}

// SuccessRate returns the connection success rate (0.0 to 1.0)
func (p *PeerRecord) SuccessRate() float64 {
	total := p.ConnectCount + p.FailCount
	if total == 0 {
		return 0.0
	}
	return float64(p.ConnectCount) / float64(total)
}

// IsStale returns true if the peer hasn't been seen recently
func (p *PeerRecord) IsStale(maxAge time.Duration) bool {
	return time.Since(p.LastSeen) > maxAge
}

// ToMap converts the peer record to a map for storage
func (p *PeerRecord) ToMap() (map[string]string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		p.PeerID: string(data),
	}, nil
}

// FromMap loads a peer record from a map
func (p *PeerRecord) FromMap(m map[string]string, peerID string) error {
	data, ok := m[peerID]
	if !ok {
		return fmt.Errorf("peer %s not found", peerID)
	}

	return json.Unmarshal([]byte(data), p)
}
