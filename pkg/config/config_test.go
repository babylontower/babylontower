package config

import (
	"os"
	"path/filepath"
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

func TestLoadFromFile(t *testing.T) {
	// Create temp config file
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	// Write test config - use numeric values for time.Duration (nanoseconds)
	testConfig := `{
		"bootstrap_timeout": 30000000000,
		"max_stored_peers": 50,
		"enable_hole_punching": false
	}`

	if err := os.WriteFile(configPath, []byte(testConfig), 0600); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load config
	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Verify loaded values
	if cfg.BootstrapTimeout != 30*time.Second {
		t.Errorf("BootstrapTimeout = %v, want 30s", cfg.BootstrapTimeout)
	}
	if cfg.MaxStoredPeers != 50 {
		t.Errorf("MaxStoredPeers = %d, want 50", cfg.MaxStoredPeers)
	}
	if cfg.EnableHolePunching != false {
		t.Error("EnableHolePunching should be false")
	}

	// Verify defaults are preserved
	if cfg.ProtocolID != DefaultProtocolID {
		t.Errorf("ProtocolID = %q, want %q", cfg.ProtocolID, DefaultProtocolID)
	}
}

func TestLoadFromFileNotFound(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path/config.json")
	if err == nil {
		t.Error("LoadFromFile should return error for non-existent file")
	}
}

func TestSaveToFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	cfg := DefaultIPFSConfig()
	cfg.MaxStoredPeers = 75

	err := cfg.SaveToFile(configPath)
	if err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// Load and verify
	loaded, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if loaded.MaxStoredPeers != 75 {
		t.Errorf("MaxStoredPeers = %d, want 75", loaded.MaxStoredPeers)
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

func TestIPFSConfigToMap(t *testing.T) {
	cfg := DefaultIPFSConfig()
	cfg.MaxStoredPeers = 99

	m, err := cfg.ToMap()
	if err != nil {
		t.Fatalf("ToMap failed: %v", err)
	}

	_, ok := m["ipfs_config"]
	if !ok {
		t.Error("ToMap should return map with 'ipfs_config' key")
	}

	// Verify it's valid JSON by converting back
	cfg2 := &IPFSConfig{}
	if err := cfg2.FromMap(m); err != nil {
		t.Errorf("FromMap failed on ToMap output: %v", err)
	}
}

func TestIPFSConfigFromMap(t *testing.T) {
	cfg := DefaultIPFSConfig()

	testMap := map[string]string{
		"ipfs_config": `{"max_stored_peers": 123, "enable_relay": true}`,
	}

	err := cfg.FromMap(testMap)
	if err != nil {
		t.Fatalf("FromMap failed: %v", err)
	}

	if cfg.MaxStoredPeers != 123 {
		t.Errorf("MaxStoredPeers = %d, want 123", cfg.MaxStoredPeers)
	}
	if cfg.EnableRelay != true {
		t.Error("EnableRelay should be true")
	}
}

func TestIPFSConfigFromMapNotFound(t *testing.T) {
	cfg := DefaultIPFSConfig()

	err := cfg.FromMap(map[string]string{})
	if err == nil {
		t.Error("FromMap should return error for missing key")
	}
}

func TestIPFSConfigRoundTrip(t *testing.T) {
	cfg1 := DefaultIPFSConfig()
	cfg1.MaxStoredPeers = 88
	cfg1.EnableRelay = true
	cfg1.DHTMode = "client"

	// Convert to map
	m, err := cfg1.ToMap()
	if err != nil {
		t.Fatalf("ToMap failed: %v", err)
	}

	// Convert back
	cfg2 := &IPFSConfig{}
	err = cfg2.FromMap(m)
	if err != nil {
		t.Fatalf("FromMap failed: %v", err)
	}

	// Compare
	if cfg2.MaxStoredPeers != cfg1.MaxStoredPeers {
		t.Errorf("MaxStoredPeers = %d, want %d", cfg2.MaxStoredPeers, cfg1.MaxStoredPeers)
	}
	if cfg2.EnableRelay != cfg1.EnableRelay {
		t.Errorf("EnableRelay = %v, want %v", cfg2.EnableRelay, cfg1.EnableRelay)
	}
	if cfg2.DHTMode != cfg1.DHTMode {
		t.Errorf("DHTMode = %q, want %q", cfg2.DHTMode, cfg1.DHTMode)
	}
}

func TestPeerRecordSuccessRate(t *testing.T) {
	tests := []struct {
		name         string
		connectCount int
		failCount    int
		want         float64
	}{
		{"no attempts", 0, 0, 0.0},
		{"all success", 10, 0, 1.0},
		{"all fail", 0, 10, 0.0},
		{"50-50", 5, 5, 0.5},
		{"75% success", 75, 25, 0.75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer := &PeerRecord{
				ConnectCount: tt.connectCount,
				FailCount:    tt.failCount,
			}
			got := peer.SuccessRate()
			if got != tt.want {
				t.Errorf("SuccessRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPeerRecordIsStale(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		lastSeen time.Time
		maxAge   time.Duration
		want     bool
	}{
		{"recent", now, 24 * time.Hour, false},
		{"old", now.Add(-48 * time.Hour), 24 * time.Hour, true},
		{"well within", now.Add(-12 * time.Hour), 24 * time.Hour, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer := &PeerRecord{LastSeen: tt.lastSeen}
			got := peer.IsStale(tt.maxAge)
			if got != tt.want {
				t.Errorf("IsStale() = %v, want %v", got, tt.want)
			}
		})
	}
}
