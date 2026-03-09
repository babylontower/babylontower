package ipfsnode

import (
	"testing"
)

// TestGetNetworkInfo tests network info retrieval
func TestGetNetworkInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping node test in short mode")
	}
	tmpDir := t.TempDir()
	config := &Config{
		RepoDir: tmpDir,
	}

	node, err := NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	if err := node.Start(); err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}
	defer stopNode(node)()

	info := node.GetNetworkInfo()
	if info == nil {
		t.Fatal("Network info should not be nil")
	}
	if info.PeerID == "" {
		t.Error("PeerID should not be empty")
	}
	if !info.IsStarted {
		t.Error("IsStarted should be true")
	}

	t.Logf("Network info: PeerID=%s, ConnectedPeers=%d",
		info.PeerID, info.ConnectedPeerCount)
}

// TestMDnsStats tests mDNS statistics
func TestMDnsStats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping node test in short mode")
	}
	tmpDir := t.TempDir()
	config := &Config{
		RepoDir: tmpDir,
	}

	node, err := NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	if err := node.Start(); err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}
	defer stopNode(node)()

	stats := node.GetMDnsStats()

	// Freshly started node should have zero discoveries
	if stats.TotalDiscoveries != 0 {
		t.Errorf("Expected 0 discoveries for freshly started node, got %d", stats.TotalDiscoveries)
	}
}
