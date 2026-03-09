package ipfsnode

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

// TestNodeCreation tests that a node can be created with default config
func TestNodeCreation(t *testing.T) {
	config := DefaultConfig()
	node, err := NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}
	defer stopNode(node)()

	if node == nil {
		t.Fatal("Node is nil")
	}

	if node.IsStarted() {
		t.Error("Node should not be started yet")
	}

	if node.config != config {
		t.Error("Config not set correctly")
	}
}

// TestNodeStartStop tests starting and stopping a node
func TestNodeStartStop(t *testing.T) {
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

	// Start the node
	if err := node.Start(); err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}

	if !node.IsStarted() {
		t.Error("Node should be started")
	}

	// Get node info
	peerID := node.PeerID()
	if peerID == "" {
		t.Error("PeerID should not be empty")
	}

	addrs := node.Multiaddrs()
	if len(addrs) == 0 {
		t.Error("Node should have addresses")
	}

	t.Logf("Node started with PeerID: %s", peerID)
	t.Logf("Node addresses: %v", addrs)

	// Stop the node
	if err := node.Stop(); err != nil {
		t.Fatalf("Failed to stop node: %v", err)
	}

	if node.IsStarted() {
		t.Error("Node should not be started after stop")
	}
}

// TestNodeDoubleStartStop tests that starting/stopping twice is safe
func TestNodeDoubleStartStop(t *testing.T) {
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

	// Start twice (should be idempotent)
	if err := node.Start(); err != nil {
		t.Fatalf("First start failed: %v", err)
	}
	if err := node.Start(); err != nil {
		t.Fatalf("Second start failed: %v", err)
	}

	// Stop twice (should be idempotent)
	if err := node.Stop(); err != nil {
		t.Fatalf("First stop failed: %v", err)
	}
	if err := node.Stop(); err != nil {
		t.Fatalf("Second stop failed: %v", err)
	}
}

// TestPeerKeyPersistence tests that peer key is persisted and reused
func TestPeerKeyPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping node test in short mode")
	}
	tmpDir := t.TempDir()
	config := &Config{
		RepoDir: tmpDir,
	}

	// Create and start first node
	node1, err := NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node1: %v", err)
	}

	if err := node1.Start(); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}

	peerID1 := node1.PeerID()
	_ = node1.Stop()

	// Create and start second node with same repo
	node2, err := NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node2: %v", err)
	}

	if err := node2.Start(); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}

	peerID2 := node2.PeerID()
	_ = node2.Stop()

	// Peer IDs should be the same (same key)
	if peerID1 != peerID2 {
		t.Errorf("Peer IDs should match after restart: %s vs %s", peerID1, peerID2)
	}
}

// TestConfigWithStoredPeers tests node config accepts stored peers
func TestConfigWithStoredPeers(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test stored peers
	storedPeers := []peer.AddrInfo{
		{
			ID:    peer.ID("QmTestPeer1"),
			Addrs: nil,
		},
		{
			ID:    peer.ID("QmTestPeer2"),
			Addrs: nil,
		},
	}

	config := &Config{
		RepoDir:     tmpDir,
		StoredPeers: storedPeers,
	}

	node, err := NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}
	defer stopNode(node)()

	if len(node.config.StoredPeers) != 2 {
		t.Errorf("StoredPeers not set correctly: %d", len(node.config.StoredPeers))
	}
}

// TestLoadStoredPeersEmpty tests loadStoredPeers with no stored peers
func TestLoadStoredPeersEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	config := &Config{
		RepoDir:     tmpDir,
		StoredPeers: nil,
	}

	node, err := NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}
	defer stopNode(node)()

	peers, err := node.loadStoredPeers()
	if err != nil {
		t.Errorf("loadStoredPeers should not fail with empty stored peers: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("Expected 0 peers, got: %d", len(peers))
	}
}

// TestLoadStoredPeers tests loadStoredPeers with stored peers
func TestLoadStoredPeers(t *testing.T) {
	tmpDir := t.TempDir()

	storedPeers := []peer.AddrInfo{
		{
			ID: peer.ID("QmStoredPeer1"),
		},
		{
			ID: peer.ID("QmStoredPeer2"),
		},
	}

	config := &Config{
		RepoDir:     tmpDir,
		StoredPeers: storedPeers,
	}

	node, err := NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}
	defer stopNode(node)()

	peers, err := node.loadStoredPeers()
	if err != nil {
		t.Errorf("loadStoredPeers failed: %v", err)
	}
	if len(peers) != 2 {
		t.Errorf("Expected 2 stored peers, got: %d", len(peers))
	}
}
