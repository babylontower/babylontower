// Package config provides configuration management for Babylon Tower.
// It supports IPFS node configuration, bootstrap peers, and connection settings.
package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/multiformats/go-multiaddr"
)

const (
	// Default protocol ID for Babylon Tower
	DefaultProtocolID = "/babylontower/1.0.0"

	// Default timeouts
	DefaultBootstrapTimeout    = 60 * time.Second
	DefaultConnectionTimeout   = 30 * time.Second
	DefaultDialTimeout         = 15 * time.Second
	DefaultDHTBootstrapTimeout = 60 * time.Second

	// Default connection limits
	DefaultMaxConnections = 400
	DefaultLowWater       = 50
	DefaultHighWater      = 400

	// Default peer storage limits
	DefaultMaxStoredPeers     = 100
	DefaultMinPeerConnections = 10

	// Default listen addresses
	DefaultListenAddrTCP   = "/ip4/0.0.0.0/tcp/0"
	DefaultListenAddrWS    = "/ip4/0.0.0.0/tcp/0/ws"
	DefaultListenAddrTCPv6 = "/ip6/::/tcp/0"
)

// IPFSConfig holds configuration for the IPFS node (internal representation for ipfsnode package)
type IPFSConfig struct {
	// Bootstrap configuration
	BootstrapPeers   []string      `json:"bootstrap_peers"`
	BootstrapTimeout time.Duration `json:"bootstrap_timeout"`

	// Peer persistence
	MaxStoredPeers     int `json:"max_stored_peers"`
	MinPeerConnections int `json:"min_peer_connections"`

	// Connection management
	ConnectionTimeout time.Duration `json:"connection_timeout"`
	DialTimeout       time.Duration `json:"dial_timeout"`
	MaxConnections    int           `json:"max_connections"`
	LowWater          int           `json:"low_water"`
	HighWater         int           `json:"high_water"`

	// NAT traversal
	EnableRelay        bool `json:"enable_relay"`
	EnableHolePunching bool `json:"enable_hole_punching"`
	EnableAutoNAT      bool `json:"enable_autonat"`

	// DHT configuration
	DHTMode             string        `json:"dht_mode"`
	DHTBootstrapTimeout time.Duration `json:"dht_bootstrap_timeout"`

	// Network configuration
	ListenAddrs []string `json:"listen_addrs"`
	ProtocolID  string   `json:"protocol_id"`
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
		BootstrapTimeout: DefaultBootstrapTimeout,

		// Peer storage
		MaxStoredPeers:     DefaultMaxStoredPeers,
		MinPeerConnections: DefaultMinPeerConnections,

		// Connection management
		ConnectionTimeout: DefaultConnectionTimeout,
		DialTimeout:       DefaultDialTimeout,
		MaxConnections:    DefaultMaxConnections,
		LowWater:          DefaultLowWater,
		HighWater:         DefaultHighWater,

		// NAT traversal
		EnableRelay:        false,
		EnableHolePunching: true,
		EnableAutoNAT:      true,

		// DHT
		DHTMode:             "auto",
		DHTBootstrapTimeout: DefaultDHTBootstrapTimeout,

		// Network
		ListenAddrs: []string{
			DefaultListenAddrTCP,
			DefaultListenAddrWS,
			DefaultListenAddrTCPv6,
		},
		ProtocolID: DefaultProtocolID,
	}
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
		return errors.New("low_water must be non-negative")
	}
	if c.HighWater < c.LowWater {
		return errors.New("high_water must be >= low_water")
	}
	if c.MaxConnections < c.HighWater {
		return errors.New("max_connections must be >= high_water")
	}

	// Validate timeouts
	if c.BootstrapTimeout <= 0 {
		return errors.New("bootstrap_timeout must be positive")
	}
	if c.ConnectionTimeout <= 0 {
		return errors.New("connection_timeout must be positive")
	}
	if c.DialTimeout <= 0 {
		return errors.New("dial_timeout must be positive")
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
		return errors.New("max_stored_peers must be >= 10")
	}
	if c.MinPeerConnections < 1 {
		return errors.New("min_peer_connections must be >= 1")
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
