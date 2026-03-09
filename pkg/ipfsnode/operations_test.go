package ipfsnode

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// TestNodeOperationsBeforeStart tests that operations fail before start
func TestNodeOperationsBeforeStart(t *testing.T) {
	config := DefaultConfig()
	node, err := NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Add should fail
	_, err = node.Add([]byte("test"))
	if err != ErrNodeNotStarted {
		t.Errorf("Add should fail with ErrNodeNotStarted, got: %v", err)
	}

	// Get should fail
	_, err = node.Get("QmTest")
	if err != ErrNodeNotStarted {
		t.Errorf("Get should fail with ErrNodeNotStarted, got: %v", err)
	}

	// Publish should fail
	err = node.Publish("test-topic", []byte("test"))
	if err != ErrNodeNotStarted {
		t.Errorf("Publish should fail with ErrNodeNotStarted, got: %v", err)
	}

	// Subscribe should fail
	_, err = node.Subscribe("test-topic")
	if err != ErrNodeNotStarted {
		t.Errorf("Subscribe should fail with ErrNodeNotStarted, got: %v", err)
	}
}

// TestAddData tests adding data to IPFS
func TestAddData(t *testing.T) {
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

	// Add data
	testData := []byte("Hello, Babylon Tower!")
	cid, err := node.Add(testData)
	if err != nil {
		t.Fatalf("Failed to add data: %v", err)
	}

	if cid == "" {
		t.Error("CID should not be empty")
	}

	t.Logf("Added data with CID: %s", cid)

	// Note: Get is not fully implemented in PoC, so we just verify Add returns a valid CID format
	if len(cid) < 10 {
		t.Errorf("CID seems too short: %s", cid)
	}
}

// TestConnectToPeersParallel tests parallel peer connection
func TestConnectToPeersParallel(t *testing.T) {
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

	// Create empty peer list (no actual connections)
	peers := []peer.AddrInfo{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connected := node.connectToPeersParallel(ctx, peers)
	if connected != 0 {
		t.Errorf("Expected 0 connections with empty peer list, got: %d", connected)
	}
}

// TestConnectToBootstrapPeersWithDNS tests DNS resolution for bootstrap peers
func TestConnectToBootstrapPeersWithDNS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping node test in short mode")
	}
	tmpDir := t.TempDir()
	config := &Config{
		RepoDir: tmpDir,
		// Use minimal bootstrap peers for testing
		BootstrapPeers: []string{},
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

	// Test with empty bootstrap list
	connected := node.connectToBootstrapPeersWithDNS(ctx)
	if connected != 0 {
		t.Errorf("Expected 0 connections with empty bootstrap list, got: %d", connected)
	}
}
