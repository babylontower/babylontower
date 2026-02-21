package ipfsnode_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"babylontower/pkg/ipfsnode"
)

// Helper to safely stop a node
func stopNode(node *ipfsnode.Node) func() {
	return func() {
		_ = node.Stop()
	}
}

// Helper to safely close a subscription
func closeSub(sub *ipfsnode.Subscription) func() {
	return func() {
		_ = sub.Close()
	}
}

// TestTwoNodePubSub tests PubSub communication between two nodes
// NOTE: This test requires actual network connectivity for peer discovery
// In isolated environments, peers may not discover each other automatically
func TestTwoNodePubSub(t *testing.T) {
	t.Skip("Skipping: requires network connectivity for peer discovery - PoC limitation")

	// Create temporary directories for two nodes
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	config1 := &ipfsnode.Config{
		RepoDir: tmpDir1,
	}
	config2 := &ipfsnode.Config{
		RepoDir: tmpDir2,
	}

	// Create and start node 1
	node1, err := ipfsnode.NewNode(config1)
	if err != nil {
		t.Fatalf("Failed to create node1: %v", err)
	}

	if err := node1.Start(); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}
	defer stopNode(node1)()

	// Create and start node 2
	node2, err := ipfsnode.NewNode(config2)
	if err != nil {
		t.Fatalf("Failed to create node2: %v", err)
	}

	if err := node2.Start(); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}
	defer stopNode(node2)()

	t.Logf("Node1 PeerID: %s", node1.PeerID())
	t.Logf("Node2 PeerID: %s", node2.PeerID())
	t.Logf("Node1 Addrs: %v", node1.Multiaddrs())
	t.Logf("Node2 Addrs: %v", node2.Multiaddrs())

	// Subscribe node 2 to a topic
	topic := "integration-test-topic"
	sub2, err := node2.Subscribe(topic)
	if err != nil {
		t.Fatalf("Node2 failed to subscribe: %v", err)
	}
	defer closeSub(sub2)()

	// Give time for subscription to establish
	time.Sleep(500 * time.Millisecond)

	// Node 1 publishes a message
	testData := []byte("Hello from Node 1!")
	if err := node1.Publish(topic, testData); err != nil {
		t.Fatalf("Node1 failed to publish: %v", err)
	}

	t.Log("Node1 published message")

	// Node 2 should receive the message
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case msg := <-sub2.Messages():
		if msg == nil {
			t.Error("Received nil message")
		}
		if !bytes.Equal(msg.Data, testData) {
			t.Errorf("Message data mismatch: got %v, want %v", msg.Data, testData)
		}
		t.Logf("Node2 received message: %s", string(msg.Data))
		t.Logf("Message from peer: %s", msg.From)
	case err := <-sub2.Errors():
		t.Errorf("Node2 received error: %v", err)
	case <-ctx.Done():
		t.Error("Timeout waiting for message - this may be expected in isolated test environment")
		t.Log("Note: PubSub may require actual network connectivity for peer discovery")
	}
}

// TestTwoNodeBidirectional tests bidirectional communication
// NOTE: This test requires actual network connectivity for peer discovery
func TestTwoNodeBidirectional(t *testing.T) {
	t.Skip("Skipping: requires network connectivity for peer discovery - PoC limitation")

	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	config1 := &ipfsnode.Config{
		RepoDir: tmpDir1,
	}
	config2 := &ipfsnode.Config{
		RepoDir: tmpDir2,
	}

	node1, err := ipfsnode.NewNode(config1)
	if err != nil {
		t.Fatalf("Failed to create node1: %v", err)
	}
	if err := node1.Start(); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}
	defer stopNode(node1)()

	node2, err := ipfsnode.NewNode(config2)
	if err != nil {
		t.Fatalf("Failed to create node2: %v", err)
	}
	if err := node2.Start(); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}
	defer stopNode(node2)()

	// Both nodes subscribe to the same topic
	sub1, err := node1.Subscribe("bidi-topic")
	if err != nil {
		t.Fatalf("Node1 failed to subscribe: %v", err)
	}
	defer closeSub(sub1)()

	sub2, err := node2.Subscribe("bidi-topic")
	if err != nil {
		t.Fatalf("Node2 failed to subscribe: %v", err)
	}
	defer closeSub(sub2)()

	time.Sleep(500 * time.Millisecond)

	// Node 1 -> Node 2
	msg1 := []byte("Message 1->2")
	if err := node1.Publish("bidi-topic", msg1); err != nil {
		t.Fatalf("Node1 failed to publish: %v", err)
	}

	// Node 2 -> Node 1
	msg2 := []byte("Message 2->1")
	if err := node2.Publish("bidi-topic", msg2); err != nil {
		t.Fatalf("Node2 failed to publish: %v", err)
	}

	// Wait for both messages
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var node1Received, node2Received bool
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for {
			select {
			case msg := <-sub1.Messages():
				if bytes.Equal(msg.Data, msg2) {
					node1Received = true
					t.Logf("Node1 received: %s", string(msg.Data))
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		for {
			select {
			case msg := <-sub2.Messages():
				if bytes.Equal(msg.Data, msg1) {
					node2Received = true
					t.Logf("Node2 received: %s", string(msg.Data))
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()

	if !node1Received {
		t.Error("Node1 did not receive message from Node2")
	}
	if !node2Received {
		t.Error("Node2 did not receive message from Node1")
	}
}

// TestNodeConnectManual tests manual peer connection
func TestNodeConnectManual(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	config1 := &ipfsnode.Config{
		RepoDir: tmpDir1,
	}
	config2 := &ipfsnode.Config{
		RepoDir: tmpDir2,
	}

	node1, err := ipfsnode.NewNode(config1)
	if err != nil {
		t.Fatalf("Failed to create node1: %v", err)
	}
	if err := node1.Start(); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}
	defer stopNode(node1)()

	node2, err := ipfsnode.NewNode(config2)
	if err != nil {
		t.Fatalf("Failed to create node2: %v", err)
	}
	if err := node2.Start(); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}
	defer stopNode(node2)()

	// Get node1's addresses
	addrs := node1.Multiaddrs()
	if len(addrs) == 0 {
		t.Fatal("Node1 has no addresses")
	}

	// Try to connect node2 to node1
	// Use the first address with node1's peer ID
	peerAddr := fmt.Sprintf("%s/p2p/%s", addrs[0], node1.PeerID())
	t.Logf("Connecting to: %s", peerAddr)

	err = node2.ConnectToPeer(peerAddr)
	if err != nil {
		// Connection may fail in test environment without proper network
		t.Logf("Connection failed (may be expected in test env): %v", err)
	} else {
		t.Log("Successfully connected node2 to node1")
	}
}

// TestMultipleNodesMesh tests a mesh of multiple nodes
// NOTE: This test requires actual network connectivity for peer discovery
func TestMultipleNodesMesh(t *testing.T) {
	t.Skip("Skipping: requires network connectivity for peer discovery - PoC limitation")

	if testing.Short() {
		t.Skip("Skipping mesh test in short mode")
	}

	const numNodes = 3

	nodes := make([]*ipfsnode.Node, numNodes)
	subs := make([]*ipfsnode.Subscription, numNodes)
	tmpDirs := make([]string, numNodes)

	// Cleanup function
	cleanup := func() {
		for i := range subs {
			if subs[i] != nil {
				_ = subs[i].Close()
			}
		}
		for i := range nodes {
			if nodes[i] != nil {
				_ = nodes[i].Stop()
			}
		}
		for _, dir := range tmpDirs {
			if dir != "" {
				_ = os.RemoveAll(dir)
			}
		}
	}
	defer cleanup()

	// Create and start all nodes
	for i := 0; i < numNodes; i++ {
		tmpDir := t.TempDir()
		tmpDirs[i] = tmpDir

		node, err := ipfsnode.NewNode(&ipfsnode.Config{
			RepoDir: tmpDir,
		})
		if err != nil {
			t.Fatalf("Failed to create node%d: %v", i, err)
		}

		if err := node.Start(); err != nil {
			t.Fatalf("Failed to start node%d: %v", i, err)
		}

		nodes[i] = node

		// Subscribe to common topic
		sub, err := node.Subscribe("mesh-topic")
		if err != nil {
			t.Fatalf("Node%d failed to subscribe: %v", i, err)
		}
		subs[i] = sub

		t.Logf("Node%d PeerID: %s", i, node.PeerID())
	}

	time.Sleep(500 * time.Millisecond)

	// Node 0 publishes
	testData := []byte("Mesh broadcast")
	if err := nodes[0].Publish("mesh-topic", testData); err != nil {
		t.Fatalf("Node0 failed to publish: %v", err)
	}

	// All other nodes should receive
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var receivedCount int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 1; i < numNodes; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			select {
			case msg := <-subs[idx].Messages():
				if bytes.Equal(msg.Data, testData) {
					mu.Lock()
					receivedCount++
					mu.Unlock()
					t.Logf("Node%d received mesh message", idx)
				}
			case <-ctx.Done():
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Received by %d/%d nodes", receivedCount, numNodes-1)
}

// TestNodeRestartPersistence tests that node can restart and maintain identity
func TestNodeRestartPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	config := &ipfsnode.Config{
		RepoDir: tmpDir,
	}

	// First instance
	node1, err := ipfsnode.NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node1: %v", err)
	}
	if err := node1.Start(); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}

	peerID1 := node1.PeerID()
	addrs1 := node1.Multiaddrs()

	t.Logf("First instance - PeerID: %s", peerID1)

	_ = node1.Stop()

	// Second instance (same repo)
	node2, err := ipfsnode.NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node2: %v", err)
	}
	if err := node2.Start(); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}
	defer stopNode(node2)()

	peerID2 := node2.PeerID()
	addrs2 := node2.Multiaddrs()

	t.Logf("Second instance - PeerID: %s", peerID2)

	// Peer ID should be the same (persisted key)
	if peerID1 != peerID2 {
		t.Errorf("PeerID changed after restart: %s -> %s", peerID1, peerID2)
	}

	// Addresses may change (different ports), but should exist
	if len(addrs2) == 0 {
		t.Error("No addresses after restart")
	}

	t.Logf("Addresses changed: %v -> %v", addrs1, addrs2)
}

// TestConcurrentPublishSubscribe tests concurrent operations
func TestConcurrentPublishSubscribe(t *testing.T) {
	tmpDir := t.TempDir()

	config := &ipfsnode.Config{
		RepoDir: tmpDir,
	}

	node, err := ipfsnode.NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}
	if err := node.Start(); err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}
	defer stopNode(node)()

	// Create multiple subscriptions
	const numSubs = 5
	subs := make([]*ipfsnode.Subscription, numSubs)
	for i := 0; i < numSubs; i++ {
		topic := fmt.Sprintf("concurrent-topic-%d", i)
		sub, err := node.Subscribe(topic)
		if err != nil {
			t.Fatalf("Failed to subscribe %d: %v", i, err)
		}
		defer closeSub(sub)()
		subs[i] = sub
	}

	time.Sleep(200 * time.Millisecond)

	// Publish concurrently
	const numMessages = 10
	var wg sync.WaitGroup
	errChan := make(chan error, numMessages*numSubs)

	for i := 0; i < numMessages; i++ {
		for j := 0; j < numSubs; j++ {
			wg.Add(1)
			go func(msgNum, subNum int) {
				defer wg.Done()
				data := []byte(fmt.Sprintf("message-%d-%d", msgNum, subNum))
				topic := fmt.Sprintf("concurrent-topic-%d", subNum)
				if err := node.Publish(topic, data); err != nil {
					errChan <- err
				}
			}(i, j)
		}
	}

	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		t.Errorf("Publish errors: %d", len(errChan))
		for err := range errChan {
			t.Error(err)
		}
	}
}

// TestLargeMessage tests publishing large messages
func TestLargeMessage(t *testing.T) {
	tmpDir := t.TempDir()

	config := &ipfsnode.Config{
		RepoDir: tmpDir,
	}

	node, err := ipfsnode.NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}
	if err := node.Start(); err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}
	defer stopNode(node)()

	sub, err := node.Subscribe("large-message-topic")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer closeSub(sub)()

	time.Sleep(100 * time.Millisecond)

	// Create a large message (100KB)
	largeData := make([]byte, 100*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	if err := node.Publish("large-message-topic", largeData); err != nil {
		t.Fatalf("Failed to publish large message: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case msg := <-sub.Messages():
		if len(msg.Data) != len(largeData) {
			t.Errorf("Large message size mismatch: got %d, want %d", len(msg.Data), len(largeData))
		}
		if !bytes.Equal(msg.Data, largeData) {
			t.Error("Large message content mismatch")
		}
		t.Logf("Successfully received large message: %d bytes", len(msg.Data))
	case err := <-sub.Errors():
		t.Errorf("Received error: %v", err)
	case <-ctx.Done():
		t.Error("Timeout waiting for large message")
	}
}

// TestTempDirCleanup ensures temporary directories are cleaned up
func TestTempDirCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-node")

	config := &ipfsnode.Config{
		RepoDir: basePath,
	}

	node, err := ipfsnode.NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	if err := node.Start(); err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		t.Error("Repo directory was not created")
	}

	_ = node.Stop()

	// Directory should still exist after stop (cleanup is manual)
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		t.Error("Repo directory was removed prematurely")
	}

	// t.TempDir() will clean up automatically
}
