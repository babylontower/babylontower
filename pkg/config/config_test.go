package config

import (
	"testing"
	"time"
)

func TestDefaultIPFSConfig(t *testing.T) {
	cfg := DefaultIPFSConfig()

	if cfg.BootstrapTimeout != DefaultBootstrapTimeout {
		t.Errorf("BootstrapTimeout = %v, want %v", cfg.BootstrapTimeout, DefaultBootstrapTimeout)
	}
	if cfg.MaxStoredPeers != DefaultMaxStoredPeers {
		t.Errorf("MaxStoredPeers = %d, want %d", cfg.MaxStoredPeers, DefaultMaxStoredPeers)
	}
	if cfg.EnableHolePunching != true {
		t.Error("EnableHolePunching should be true")
	}
	if len(cfg.BootstrapPeers) < 10 {
		t.Errorf("BootstrapPeers should have at least 10 peers, got %d", len(cfg.BootstrapPeers))
	}
}

func TestIPFSConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*IPFSConfig)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(c *IPFSConfig) {},
			wantErr: false,
		},
		{
			name: "invalid bootstrap peer",
			modify: func(c *IPFSConfig) {
				c.BootstrapPeers = []string{"invalid-multiaddr"}
			},
			wantErr: true,
		},
		{
			name: "invalid listen address",
			modify: func(c *IPFSConfig) {
				c.ListenAddrs = []string{"invalid"}
			},
			wantErr: true,
		},
		{
			name: "high_water less than low_water",
			modify: func(c *IPFSConfig) {
				c.LowWater = 100
				c.HighWater = 50
			},
			wantErr: true,
		},
		{
			name: "max_connections less than high_water",
			modify: func(c *IPFSConfig) {
				c.MaxConnections = 100
				c.HighWater = 200
			},
			wantErr: true,
		},
		{
			name: "invalid DHT mode",
			modify: func(c *IPFSConfig) {
				c.DHTMode = "invalid"
			},
			wantErr: true,
		},
		{
			name: "max_stored_peers too low",
			modify: func(c *IPFSConfig) {
				c.MaxStoredPeers = 5
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			modify: func(c *IPFSConfig) {
				c.BootstrapTimeout = -1 * time.Second
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultIPFSConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetBootstrapPeerInfos(t *testing.T) {
	cfg := DefaultIPFSConfig()

	infos, err := cfg.GetBootstrapPeerInfos()
	if err != nil {
		t.Fatalf("GetBootstrapPeerInfos failed: %v", err)
	}

	if len(infos) != len(cfg.BootstrapPeers) {
		t.Errorf("Got %d peer infos, want %d", len(infos), len(cfg.BootstrapPeers))
	}
}

func TestGetBootstrapPeerInfosInvalid(t *testing.T) {
	cfg := DefaultIPFSConfig()
	cfg.BootstrapPeers = append(cfg.BootstrapPeers, "invalid-multiaddr")

	_, err := cfg.GetBootstrapPeerInfos()
	if err == nil {
		t.Error("GetBootstrapPeerInfos should return error for invalid peer")
	}
}
