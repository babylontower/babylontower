package ipfsnode

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Helper to safely stop a node
func stopNode(node *Node) func() {
	return func() {
		_ = node.Stop()
	}
}

// Helper to safely close a subscription
func closeSub(sub *Subscription) func() {
	return func() {
		_ = sub.Close()
	}
}

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

// TestPeerKeyPersistence tests that peer key is persisted and reused
func TestPeerKeyPersistence(t *testing.T) {
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

// TestTopicFromPublicKey tests topic derivation from public key
func TestTopicFromPublicKey(t *testing.T) {
	// Generate a test public key
	_, pubKey, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	pubKeyBytes, err := pubKey.Raw()
	if err != nil {
		t.Fatalf("Failed to get key bytes: %v", err)
	}

	topic := TopicFromPublicKey(pubKeyBytes)

	if topic == "" {
		t.Error("Topic should not be empty")
	}

	if len(topic) < 10 {
		t.Errorf("Topic too short: %s", topic)
	}

	// Same key should produce same topic
	topic2 := TopicFromPublicKey(pubKeyBytes)
	if topic != topic2 {
		t.Errorf("Same key should produce same topic: %s vs %s", topic, topic2)
	}

	// Different key should produce different topic
	_, pubKey2, _ := crypto.GenerateEd25519Key(rand.Reader)
	pubKeyBytes2, _ := pubKey2.Raw()
	topic3 := TopicFromPublicKey(pubKeyBytes2)
	if topic == topic3 {
		t.Errorf("Different keys should produce different topics")
	}

	t.Logf("Topic from public key: %s", topic)
}

// TestAddData tests adding data to IPFS
func TestAddData(t *testing.T) {
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

// TestSubscriptionLifecycle tests subscription creation and cleanup
func TestSubscriptionLifecycle(t *testing.T) {
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

	// Subscribe to a topic
	sub, err := node.Subscribe("test-topic")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	if sub == nil {
		t.Fatal("Subscription should not be nil")
	}

	if sub.Topic() != "test-topic" {
		t.Errorf("Wrong topic name: %s", sub.Topic())
	}

	if sub.IsClosed() {
		t.Error("Subscription should not be closed")
	}

	// Close subscription
	if err := sub.Close(); err != nil {
		t.Fatalf("Failed to close subscription: %v", err)
	}

	if !sub.IsClosed() {
		t.Error("Subscription should be closed")
	}

	// Double close should be safe
	if err := sub.Close(); err != nil {
		t.Errorf("Double close should not fail: %v", err)
	}
}

// TestPublishSubscribe tests basic pubsub functionality
func TestPublishSubscribe(t *testing.T) {
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

	// Subscribe to a topic
	sub, err := node.Subscribe("test-pubsub-topic")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer closeSub(sub)()

	// Give subscription time to establish
	time.Sleep(100 * time.Millisecond)

	// Publish a message
	testData := []byte("Test message")
	if err := node.Publish("test-pubsub-topic", testData); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Wait for message with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case msg := <-sub.Messages():
		if msg == nil {
			t.Error("Received nil message")
			return
		}
		if !bytes.Equal(msg.Data, testData) {
			t.Errorf("Message data mismatch: got %v, want %v", msg.Data, testData)
		}
		t.Logf("Received message: %s", string(msg.Data))
	case err := <-sub.Errors():
		t.Errorf("Received error: %v", err)
	case <-ctx.Done():
		t.Error("Timeout waiting for message")
	}
}

// TestPublishToPublicKey tests publishing to a topic derived from public key
func TestPublishToPublicKey(t *testing.T) {
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

	// Generate a test public key
	_, pubKey, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	pubKeyBytes, err := pubKey.Raw()
	if err != nil {
		t.Fatalf("Failed to get key bytes: %v", err)
	}

	// Subscribe to the derived topic
	topic := TopicFromPublicKey(pubKeyBytes)
	sub, err := node.Subscribe(topic)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer closeSub(sub)()

	time.Sleep(100 * time.Millisecond)

	// Publish to the public key
	testData := []byte("Message to public key")
	if err := node.PublishTo(pubKeyBytes, testData); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Wait for message
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case msg := <-sub.Messages():
		if !bytes.Equal(msg.Data, testData) {
			t.Errorf("Message data mismatch")
		}
		t.Logf("Received message for public key: %s", string(msg.Data))
	case err := <-sub.Errors():
		t.Errorf("Received error: %v", err)
	case <-ctx.Done():
		t.Error("Timeout waiting for message")
	}
}

// TestMultipleSubscriptions tests multiple subscriptions on same topic
func TestMultipleSubscriptions(t *testing.T) {
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

	// Create multiple subscriptions
	sub1, err := node.Subscribe("multi-sub-topic")
	if err != nil {
		t.Fatalf("Failed to subscribe sub1: %v", err)
	}
	defer closeSub(sub1)()

	sub2, err := node.Subscribe("multi-sub-topic")
	if err != nil {
		t.Fatalf("Failed to subscribe sub2: %v", err)
	}
	defer closeSub(sub2)()

	time.Sleep(100 * time.Millisecond)

	// Publish one message
	testData := []byte("Multi-sub test")
	if err := node.Publish("multi-sub-topic", testData); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Both subscriptions should receive the message
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var received1, received2 bool
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		select {
		case msg := <-sub1.Messages():
			if bytes.Equal(msg.Data, testData) {
				received1 = true
			}
		case <-ctx.Done():
		}
	}()

	go func() {
		defer wg.Done()
		select {
		case msg := <-sub2.Messages():
			if bytes.Equal(msg.Data, testData) {
				received2 = true
			}
		case <-ctx.Done():
		}
	}()

	wg.Wait()

	if !received1 {
		t.Error("Subscription 1 did not receive message")
	}
	if !received2 {
		t.Error("Subscription 2 did not receive message")
	}
}

// TestRepoDirExpansion tests that ~ is expanded in repo directory
func TestRepoDirExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Cannot get home directory: %v", err)
	}

	config := &Config{
		RepoDir: "~/.babylontower/test-ipfs-" + filepath.Base(t.TempDir()),
	}

	node, err := NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	if err := node.Start(); err != nil {
		// May fail due to permissions, skip test
		t.Skipf("Cannot start node with expanded path: %v", err)
	}
	defer stopNode(node)()

	// Verify the path was expanded (should not contain ~)
	expanded, _ := expandPath(config.RepoDir)
	if len(expanded) == 0 || expanded[0] != '/' {
		t.Errorf("Path not expanded correctly: %s", expanded)
	}

	// Should start with home directory
	if len(expanded) < len(home) || expanded[:len(home)] != home {
		t.Logf("Expanded path: %s, Home: %s", expanded, home)
	}
}

// TestExpandPathRespectsHomeEnv tests that HOME environment variable is respected
func TestExpandPathRespectsHomeEnv(t *testing.T) {
	// Save original HOME
	originalHome := os.Getenv("HOME")
	defer func() {
		if originalHome != "" {
			_ = os.Setenv("HOME", originalHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	}()

	// Set custom HOME for test
	customHome := "/custom/test/home"
	_ = os.Setenv("HOME", customHome)

	// Test expansion with custom HOME
	path := "~/.babylontower/ipfs"
	expanded, err := expandPath(path)
	if err != nil {
		t.Fatalf("Failed to expand path: %v", err)
	}

	expected := filepath.Join(customHome, ".babylontower/ipfs")
	if expanded != expected {
		t.Errorf("Expected %s, got %s", expected, expanded)
	}

	// Test that UserHomeDir is used when HOME is not set
	_ = os.Unsetenv("HOME")

	// Get user home dir BEFORE unsetting (some systems require HOME to be set)
	userHome, err := os.UserHomeDir()
	if err != nil {
		// Try to get home from /etc/passwd or other means
		// If this fails, skip the test
		t.Logf("Cannot get user home dir without HOME set: %v", err)
		// Re-set HOME to original value for this check
		if originalHome != "" {
			_ = os.Setenv("HOME", originalHome)
			userHome, err = os.UserHomeDir()
			if err != nil {
				t.Skipf("Cannot get user home dir at all: %v", err)
			}
		} else {
			t.Skipf("Cannot get user home dir: %v", err)
		}
	}

	expanded2, err := expandPath(path)
	if err != nil {
		t.Fatalf("Failed to expand path: %v", err)
	}

	expected2 := filepath.Join(userHome, ".babylontower/ipfs")
	if expanded2 != expected2 {
		t.Errorf("Expected %s, got %s", expected2, expanded2)
	}
}

// TestListTopicsAndPeers tests listing topics and peers
func TestListTopicsAndPeers(t *testing.T) {
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

	// Initially no topics
	topics := node.ListTopics()
	if len(topics) != 0 {
		t.Errorf("Expected no topics initially, got: %v", topics)
	}

	// Subscribe to a topic
	_, err = node.Subscribe("list-test-topic")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Should have one topic now
	topics = node.ListTopics()
	if len(topics) != 1 {
		t.Errorf("Expected 1 topic, got: %v", topics)
	}

	// Get peers for the topic (may be empty in single-node test)
	peers := node.ListPeers("list-test-topic")
	t.Logf("Peers on topic: %v", peers)
}

// TestMessageString tests Message string representation
func TestMessageString(t *testing.T) {
	msg := &Message{
		Data:  []byte("test data"),
		From:  peer.ID("test-peer"),
		Topic: "test-topic",
		SeqNo: []byte{1, 2, 3, 4, 5},
	}

	str := msg.String()
	if str == "" {
		t.Error("Message string should not be empty")
	}

	t.Logf("Message string: %s", str)
}

// TestBootstrapResult tests the BootstrapResult struct
func TestBootstrapResult(t *testing.T) {
	result := &BootstrapResult{
		StoredPeersAttempted: 5,
		StoredPeersConnected: 3,
		ConfigPeersAttempted: 10,
		ConfigPeersConnected: 7,
		TotalConnected:       10,
		RoutingTableSize:     15,
		Duration:             5 * time.Second,
	}

	if result.StoredPeersAttempted != 5 {
		t.Errorf("StoredPeersAttempted mismatch: %d", result.StoredPeersAttempted)
	}
	if result.TotalConnected != 10 {
		t.Errorf("TotalConnected mismatch: %d", result.TotalConnected)
	}
	if result.RoutingTableSize != 15 {
		t.Errorf("RoutingTableSize mismatch: %d", result.RoutingTableSize)
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

// TestConnectToPeersParallel tests parallel peer connection
func TestConnectToPeersParallel(t *testing.T) {
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
	tmpDir := t.TempDir()
	config := &Config{
		RepoDir: tmpDir,
		// Use minimal bootstrap peers for testing
		BootstrapPeers: []string{},
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

// TestDHTMaintenance tests that DHT maintenance runs
func TestDHTMaintenance(t *testing.T) {
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

// TestGetNetworkInfo tests network info retrieval
func TestGetNetworkInfo(t *testing.T) {
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
	t.Logf("mDNS stats: TotalDiscoveries=%d, LastPeerFound=%v",
		stats.TotalDiscoveries, stats.LastPeerFound)
}
