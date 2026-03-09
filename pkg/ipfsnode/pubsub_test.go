package ipfsnode

import (
	"bytes"
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// TestSubscriptionLifecycle tests subscription creation and cleanup
func TestSubscriptionLifecycle(t *testing.T) {
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

// TestListTopicsAndPeers tests listing topics and peers
func TestListTopicsAndPeers(t *testing.T) {
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

	// Initially should have no topics (rendezvous uses DHT, not PubSub topics)
	topics := node.ListTopics()
	if len(topics) != 0 {
		t.Errorf("Expected 0 topics initially, got: %v", topics)
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
		From:  "test-peer",
		Topic: "test-topic",
		SeqNo: []byte{1, 2, 3, 4, 5},
	}

	str := msg.String()
	if str == "" {
		t.Error("Message string should not be empty")
	}

	t.Logf("Message string: %s", str)
}
