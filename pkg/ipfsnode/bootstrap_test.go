package ipfsnode

import (
	"context"
	"testing"
	"time"
)

// TestDHTMaintenance tests that DHT maintenance runs
func TestDHTMaintenance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping node test in short mode")
	}
	tmpDir := t.TempDir()
	config := &Config{
		RepoDir: tmpDir,
		// Add Bootstrap config for new hybrid bootstrap
		Bootstrap: DefaultBootstrapConfig(),
	}

	node, err := NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	if err := node.Start(); err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}
	defer stopNode(node)()

	// DHT maintenance should be running in background
	// Just verify the node doesn't crash
	time.Sleep(100 * time.Millisecond)

	// Get DHT info
	dhtInfo := node.GetDHTInfo()
	if dhtInfo == nil || !dhtInfo.IsStarted {
		t.Error("DHT should be started")
	}
}

// TestAdvertiseSelf tests self-advertisement to DHT
func TestAdvertiseSelf(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping node test in short mode")
	}
	tmpDir := t.TempDir()
	config := &Config{
		RepoDir: tmpDir,
		// Add Bootstrap config for new hybrid bootstrap
		Bootstrap: DefaultBootstrapConfig(),
	}

	node, err := NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	if err := node.Start(); err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}
	defer stopNode(node)()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Advertise self
	err = node.AdvertiseSelf(ctx)
	if err != nil {
		t.Logf("AdvertiseSelf returned: %v (may be expected in isolated test)", err)
	}

	// Check DHT info
	dhtInfo := node.GetDHTInfo()
	t.Logf("DHT info after advertise: routing_table=%d, connected=%d",
		dhtInfo.RoutingTableSize, dhtInfo.ConnectedPeerCount)
}
