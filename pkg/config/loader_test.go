package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	bterrors "babylontower/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAppConfig_Defaults(t *testing.T) {
	// Loading from a non-existent dir should return defaults.
	cfg, err := LoadAppConfig(t.TempDir(), "")
	require.NoError(t, err)

	assert.Equal(t, "warn", cfg.Logging.Level)
	assert.Equal(t, 400, cfg.Network.MaxConnections)
	assert.Equal(t, "auto", cfg.Network.DHTMode)
	assert.Equal(t, 100, cfg.Messaging.ChannelBufferSize)
	assert.Equal(t, 25, cfg.RTC.MaxParticipants)
	assert.Equal(t, 5, cfg.Multidevice.MaxDevices)
	assert.True(t, cfg.Identity.DHTPublish)
}

func TestLoadAppConfig_YAMLFile(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
logging:
  level: debug
network:
  max_connections: 200
  dht_mode: client
rtc:
  enable_video: false
`
	err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yamlContent), 0600)
	require.NoError(t, err)

	cfg, err := LoadAppConfig(dir, "")
	require.NoError(t, err)

	assert.Equal(t, "debug", cfg.Logging.Level)
	assert.Equal(t, 200, cfg.Network.MaxConnections)
	assert.Equal(t, "client", cfg.Network.DHTMode)
	assert.False(t, cfg.RTC.EnableVideo)
	// Unset fields keep defaults
	assert.Equal(t, 100, cfg.Messaging.ChannelBufferSize)
}

func TestLoadAppConfig_ExplicitConfigPath(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "custom.yaml")
	err := os.WriteFile(cfgFile, []byte("logging:\n  level: error\n"), 0600)
	require.NoError(t, err)

	cfg, err := LoadAppConfig(dir, cfgFile)
	require.NoError(t, err)
	assert.Equal(t, "error", cfg.Logging.Level)
}

func TestLoadAppConfig_EnvOverrides(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("BABYLONTOWER_LOGGING_LEVEL", "info")
	t.Setenv("BABYLONTOWER_NETWORK_MAX_CONNECTIONS", "999")

	cfg, err := LoadAppConfig(dir, "")
	require.NoError(t, err)

	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, 999, cfg.Network.MaxConnections)
}

func TestValidateAppConfig_Valid(t *testing.T) {
	cfg := DefaultAppConfig()
	err := ValidateAppConfig(cfg)
	assert.NoError(t, err)
}

func TestValidateAppConfig_InvalidDHTMode(t *testing.T) {
	cfg := DefaultAppConfig()
	cfg.Network.DHTMode = "invalid"
	err := ValidateAppConfig(cfg)
	require.Error(t, err)
	assert.True(t, errors.Is(err, bterrors.ErrInvalidConfig))
}

func TestValidateAppConfig_InvalidConnLimits(t *testing.T) {
	cfg := DefaultAppConfig()
	cfg.Network.HighWater = cfg.Network.LowWater - 1
	err := ValidateAppConfig(cfg)
	require.Error(t, err)
	assert.True(t, errors.Is(err, bterrors.ErrInvalidConfig))
}

func TestValidateAppConfig_InvalidLogLevel(t *testing.T) {
	cfg := DefaultAppConfig()
	cfg.Logging.Level = "trace"
	err := ValidateAppConfig(cfg)
	require.Error(t, err)
	assert.True(t, errors.Is(err, bterrors.ErrInvalidConfig))
}

func TestValidateAppConfig_InvalidMaxParticipants(t *testing.T) {
	cfg := DefaultAppConfig()
	cfg.RTC.MaxParticipants = 1
	err := ValidateAppConfig(cfg)
	require.Error(t, err)
	assert.True(t, errors.Is(err, bterrors.ErrInvalidConfig))
}

func TestToIPFSConfig(t *testing.T) {
	app := DefaultAppConfig()
	ipfs := app.ToIPFSConfig()

	assert.Equal(t, app.Network.BootstrapPeers, ipfs.BootstrapPeers)
	assert.Equal(t, app.Network.DialTimeout, ipfs.DialTimeout)
	assert.Equal(t, app.Network.MaxConnections, ipfs.MaxConnections)
	assert.Equal(t, app.Network.DHTMode, ipfs.DHTMode)
}

func TestDefaultAppConfig_Values(t *testing.T) {
	d := DefaultAppConfig()

	// Spot-check a few important defaults
	assert.Equal(t, 15*time.Second, d.Network.DialTimeout)
	assert.Equal(t, uint32(500), d.Mailbox.MaxMessagesPerTarget)
	assert.Equal(t, 7*24*time.Hour, d.Mailbox.DefaultTTL)
	assert.Equal(t, 30*time.Second, d.Multidevice.SyncInterval)
	assert.Equal(t, 4*time.Hour, d.Identity.RefreshInterval)
	assert.Len(t, d.Network.BootstrapPeers, 11)
}
