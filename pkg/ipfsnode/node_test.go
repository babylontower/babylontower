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
