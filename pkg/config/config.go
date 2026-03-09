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

// IPFSConfig is an alias for NetworkConfig (defined in app.go).
// Kept for backward compatibility — prefer using NetworkConfig directly.
type IPFSConfig = NetworkConfig

// DefaultIPFSConfig returns a NetworkConfig with sensible defaults.
// Delegates to DefaultAppConfig().Network to avoid duplicate default lists.
func DefaultIPFSConfig() *NetworkConfig {
	defaults := DefaultAppConfig()
	return &defaults.Network
}

// Validate validates the network configuration.
func (c *NetworkConfig) Validate() error {
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

// GetBootstrapPeerInfos parses bootstrap peers into multiaddr format.
func (c *NetworkConfig) GetBootstrapPeerInfos() ([]multiaddr.Multiaddr, error) {
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
