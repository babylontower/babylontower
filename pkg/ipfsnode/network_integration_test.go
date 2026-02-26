//go:build integration
// +build integration

package ipfsnode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// TestTwoNodePubSub tests publish/subscribe between two IPFS nodes
// Spec reference: specs/testing.md Section 2.11 - Two-Node PubSub
func TestTwoNodePubSub(t *testing.T) {
	t.Log("=== Two-Node PubSub Test ===")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create temporary directories for nodes
	tmpDir1, err := os.MkdirTemp("", "babylon-node1-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir 1: %v", err)
	}
	defer os.RemoveAll(tmpDir1)

	tmpDir2, err := os.MkdirTemp("", "babylon-node2-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir 2: %v", err)
	}
	defer os.RemoveAll(tmpDir2)

	// Create node 1 (Alice)
	config1 := DefaultConfig()
	config1.RepoDir = tmpDir1
	config1.BootstrapPeers = []string{} // No bootstrap for direct connection test

	node1, err := NewNode(config1)
	if err != nil {
		t.Fatalf("Failed to create node 1: %v", err)
	}
	defer stopNode(node1)()

	if err := node1.Start(); err != nil {
		t.Fatalf("Failed to start node 1: %v", err)
	}

	t.Logf("Node 1 (Alice) started: %s", node1.PeerID())

	// Create node 2 (Bob)
	config2 := DefaultConfig()
	config2.RepoDir = tmpDir2
	config2.BootstrapPeers = []string{}

	node2, err := NewNode(config2)
	if err != nil {
		t.Fatalf("Failed to create node 2: %v", err)
	}
	defer stopNode(node2)()

	if err := node2.Start(); err != nil {
		t.Fatalf("Failed to start node 2: %v", err)
	}

	t.Logf("Node 2 (Bob) started: %s", node2.PeerID())

	// Connect node 2 to node 1
	addr1 := node1.Multiaddrs()[0]
	t.Logf("Node 1 address: %s", addr1)

	err = node2.Connect(ctx, addr1)
	if err != nil {
		t.Fatalf("Failed to connect node 2 to node 1: %v", err)
	}

	t.Log("Nodes connected successfully")

	// Wait for connection to establish
	time.Sleep(500 * time.Millisecond)

	// Node 1 subscribes to topic
	topic := "test-topic"
	sub1, err := node1.Subscribe(topic)
	if err != nil {
		t.Fatalf("Node 1 subscribe failed: %v", err)
	}
	defer closeSub(sub1)()

	t.Logf("Node 1 subscribed to: %s", topic)

	// Wait for subscription to propagate
	time.Sleep(500 * time.Millisecond)

	// Node 2 publishes message
	message := "Hello from Node 2!"
	err = node2.Publish(ctx, topic, []byte(message))
	if err != nil {
		t.Fatalf("Node 2 publish failed: %v", err)
	}

	t.Logf("Node 2 published: %s", message)

	// Node 1 receives message
	select {
	case msg := <-sub1.Messages:
		t.Logf("Node 1 received: %s", string(msg.Data))
		if string(msg.Data) != message {
			t.Errorf("Message content mismatch: got %s, want %s", msg.Data, message)
		}
		if msg.From != node2.PeerID() {
			t.Errorf("Message sender mismatch: got %s, want %s", msg.From, node2.PeerID())
		}
		t.Log("✓ Message received correctly")

	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for message")
	}

	t.Log("\n=== Acceptance Criteria ===")
	t.Log("✓ Both nodes start successfully")
	t.Log("✓ Nodes connect (explicit or discovery)")
	t.Log("✓ Message published by Node 2 received by Node 1")
	t.Log("✓ Message content matches")
}

// TestMultiNodeNetworkFormation tests 5+ nodes forming a stable mesh network
// Spec reference: specs/testing.md Section 2.12 - Multi-Node Network Formation
func TestMultiNodeNetworkFormation(t *testing.T) {
	t.Log("=== Multi-Node Network Formation Test ===")

	const nodeCount = 5
	nodes := make([]*Node, nodeCount)
	cleanupFns := make([]func(), nodeCount)

	// Create temporary directories
	tmpDirs := make([]string, nodeCount)
	for i := 0; i < nodeCount; i++ {
		tmpDir, err := os.MkdirTemp("", fmt.Sprintf("babylon-node%d-*", i))
		if err != nil {
			t.Fatalf("Failed to create temp dir %d: %v", i, err)
		}
		tmpDirs[i] = tmpDir
		defer os.RemoveAll(tmpDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create and start all nodes
	for i := 0; i < nodeCount; i++ {
		config := DefaultConfig()
		config.RepoDir = tmpDirs[i]
		config.BootstrapPeers = []string{} // Manual connection for test

		node, err := NewNode(config)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v", i, err)
		}
		nodes[i] = node

		if err := node.Start(); err != nil {
			t.Fatalf("Failed to start node %d: %v", i, err)
		}

		cleanupFns[i] = stopNode(node)
		t.Logf("Node %d started: %s", i+1, node.PeerID())
	}

	defer func() {
		for _, cleanup := range cleanupFns {
			cleanup()
		}
	}()

	// Connect nodes in a ring topology for initial bootstrap
	// Node 1 → Node 2 → Node 3 → Node 4 → Node 5 → Node 1
	t.Log("Connecting nodes in ring topology...")
	for i := 0; i < nodeCount; i++ {
		nextIdx := (i + 1) % nodeCount
		addr := nodes[nextIdx].Multiaddrs()[0]
		
		err := nodes[i].Connect(ctx, addr)
		if err != nil {
			t.Logf("Warning: Failed to connect node %d to node %d: %v", i+1, nextIdx+1, err)
		}
	}

	t.Log("Waiting for network formation...")
	time.Sleep(5 * time.Second)

	// Verify network formation
	t.Log("\nVerifying network formation...")

	totalConnections := 0
	totalDHTPeers := 0
	nodesWithConnections := 0

	for i := 0; i < nodeCount; i++ {
		// Get connection count
		connections := nodes[i].ConnectionManager().ConnCount()
		
		// Get DHT routing table size
		routingTableSize := nodes[i].DHT().RoutingTable().Size()

		t.Logf("Node %d: %d connections, DHT: %d peers", i+1, connections, routingTableSize)

		if connections > 0 {
			nodesWithConnections++
		}
		totalConnections += connections
		totalDHTPeers += routingTableSize
	}

	avgConnections := float64(totalConnections) / float64(nodeCount)
	avgDHTPeers := float64(totalDHTPeers) / float64(nodeCount)

	t.Logf("\nSummary:")
	t.Logf("  Nodes: %d", nodeCount)
	t.Logf("  Nodes with connections: %d", nodesWithConnections)
	t.Logf("  Total connections: %d", totalConnections)
	t.Logf("  Total DHT peers: %d", totalDHTPeers)
	t.Logf("  Avg connections/node: %.2f", avgConnections)
	t.Logf("  Avg DHT peers/node: %.2f", avgDHTPeers)

	// Acceptance criteria
	t.Log("\n=== Acceptance Criteria ===")

	passed := 0
	total := 4

	// All nodes running
	if nodesWithConnections == nodeCount {
		t.Log("✓ All nodes running: PASS")
		passed++
	} else {
		t.Errorf("✗ All nodes running: FAIL (%d/%d)", nodesWithConnections, nodeCount)
	}

	// Average connections ≥ 2
	if avgConnections >= 2 {
		t.Log("✓ Avg connections ≥2: PASS")
		passed++
	} else {
		t.Errorf("✗ Avg connections ≥2: FAIL (%.2f)", avgConnections)
	}

	// Average DHT peers ≥ 3
	if avgDHTPeers >= 3 {
		t.Log("✓ Avg DHT peers ≥3: PASS")
		passed++
	} else {
		t.Errorf("✗ Avg DHT peers ≥3: FAIL (%.2f)", avgDHTPeers)
	}

	// Network formation
	if passed == total {
		t.Log("✓ Network formation: PASS")
		passed++
	} else {
		t.Logf("✗ Network formation: PARTIAL (%d/%d criteria)", passed, total)
	}

	if passed < total {
		t.Logf("\n⚠ Test needs attention: %d/%d criteria passed", passed, total)
	} else {
		t.Logf("\n✓ Multi-node network test PASSED!")
	}
}

// TestPubSubMessageDelivery tests reliable message delivery across network
// Spec reference: specs/testing.md - PubSub message delivery
func TestPubSubMessageDelivery(t *testing.T) {
	t.Log("=== PubSub Message Delivery Test ===")

	const nodeCount = 3
	nodes := make([]*Node, nodeCount)
	subs := make([]*Subscription, nodeCount)
	cleanupFns := make([]func(), nodeCount)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create nodes
	tmpDirs := make([]string, nodeCount)
	for i := 0; i < nodeCount; i++ {
		tmpDir, _ := os.MkdirTemp("", fmt.Sprintf("babylon-msg-node%d-*", i))
		tmpDirs[i] = tmpDir
		defer os.RemoveAll(tmpDir)

		config := DefaultConfig()
		config.RepoDir = tmpDirs[i]
		config.BootstrapPeers = []string{}

		node, err := NewNode(config)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v", i, err)
		}
		nodes[i] = node
		cleanupFns[i] = stopNode(node)

		if err := node.Start(); err != nil {
			t.Fatalf("Failed to start node %d: %v", i, err)
		}
	}

	defer func() {
		for i := range subs {
			if subs[i] != nil {
				closeSub(subs[i])()
			}
		}
		for _, cleanup := range cleanupFns {
			cleanup()
		}
	}()

	// Connect all nodes to first node (star topology)
	for i := 1; i < nodeCount; i++ {
		addr := nodes[0].Multiaddrs()[0]
		if err := nodes[i].Connect(ctx, addr); err != nil {
			t.Fatalf("Failed to connect node %d: %v", i, err)
		}
	}

	time.Sleep(1 * time.Second)

	// All nodes subscribe to same topic
	topic := "broadcast-topic"
	for i := 0; i < nodeCount; i++ {
		sub, err := nodes[i].Subscribe(topic)
		if err != nil {
			t.Fatalf("Node %d subscribe failed: %v", i, err)
		}
		subs[i] = sub
	}

	time.Sleep(1 * time.Second)

	// Node 0 publishes message
	message := "Broadcast message from Node 0"
	if err := nodes[0].Publish(ctx, topic, []byte(message)); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	t.Logf("Node 0 published: %s", message)

	// All nodes should receive message
	received := make(chan int, nodeCount)
	for i := 0; i < nodeCount; i++ {
		go func(idx int) {
			select {
			case msg := <-subs[idx].Messages:
				if string(msg.Data) == message {
					received <- idx
				}
			case <-time.After(5 * time.Second):
				t.Logf("Node %d timeout waiting for message", idx)
			}
		}(i)
	}

	// Wait for all nodes to receive (or timeout)
	receivedCount := 0
	timeout := time.After(6 * time.Second)
	for receivedCount < nodeCount {
		select {
		case idx := <-received:
			receivedCount++
			t.Logf("Node %d received message (%d/%d)", idx, receivedCount, nodeCount)
		case <-timeout:
			t.Logf("Timeout: only %d/%d nodes received message", receivedCount, nodeCount)
			goto done
		}
	}

done:
	t.Log("\n=== Acceptance Criteria ===")
	if receivedCount == nodeCount {
		t.Log("✓ All nodes received broadcast message")
	} else {
		t.Errorf("✗ Only %d/%d nodes received message", receivedCount, nodeCount)
	}
	t.Log("✓ Message delivery via GossipSub")
}

// TestDHTBootstrap tests DHT bootstrap process
// Spec reference: specs/testing.md - DHT bootstrap
func TestDHTBootstrap(t *testing.T) {
	t.Log("=== DHT Bootstrap Test ===")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create bootstrap node (simulating public bootstrap node)
	bootstrapDir, _ := os.MkdirTemp("", "babylon-bootstrap-*")
	defer os.RemoveAll(bootstrapDir)

	bootstrapConfig := DefaultConfig()
	bootstrapConfig.RepoDir = bootstrapDir
	bootstrapConfig.BootstrapPeers = []string{}

	bootstrapNode, err := NewNode(bootstrapConfig)
	if err != nil {
		t.Fatalf("Failed to create bootstrap node: %v", err)
	}
	defer stopNode(bootstrapNode)()

	if err := bootstrapNode.Start(); err != nil {
		t.Fatalf("Failed to start bootstrap node: %v", err)
	}

	t.Logf("Bootstrap node: %s", bootstrapNode.PeerID())

	// Create client node
	clientDir, _ := os.MkdirTemp("", "babylon-client-*")
	defer os.RemoveAll(clientDir)

	clientConfig := DefaultConfig()
	clientConfig.RepoDir = clientDir
	clientConfig.BootstrapPeers = []string{
		fmt.Sprintf("%s/ipfs/%s", bootstrapNode.Multiaddrs()[0], bootstrapNode.PeerID()),
	}

	clientNode, err := NewNode(clientConfig)
	if err != nil {
		t.Fatalf("Failed to create client node: %v", err)
	}
	defer stopNode(clientNode)()

	if err := clientNode.Start(); err != nil {
		t.Fatalf("Failed to start client node: %v", err)
	}

	t.Logf("Client node: %s", clientNode.PeerID())

	// Wait for bootstrap
	t.Log("Waiting for DHT bootstrap...")
	time.Sleep(3 * time.Second)

	// Check DHT routing table
	routingTableSize := clientNode.DHT().RoutingTable().Size()
	t.Logf("DHT routing table size: %d", routingTableSize)

	// Check connections
	connCount := clientNode.ConnectionManager().ConnCount()
	t.Logf("Connection count: %d", connCount)

	t.Log("\n=== Acceptance Criteria ===")
	if connCount > 0 {
		t.Log("✓ Client connected to bootstrap node")
	} else {
		t.Error("✗ Client not connected to bootstrap node")
	}

	if routingTableSize > 0 {
		t.Log("✓ DHT routing table populated")
	} else {
		t.Error("✗ DHT routing table empty")
	}
}

// BenchmarkNodeStartup benchmarks node startup time
func BenchmarkNodeStartup(b *testing.B) {
	for i := 0; i < b.N; i++ {
		tmpDir, _ := os.MkdirTemp("", "babylon-bench-*")
		
		config := DefaultConfig()
		config.RepoDir = tmpDir
		config.BootstrapPeers = []string{}

		node, err := NewNode(config)
		if err != nil {
			b.Fatalf("Failed to create node: %v", err)
		}

		if err := node.Start(); err != nil {
			b.Fatalf("Failed to start node: %v", err)
		}

		stopNode(node)()
		os.RemoveAll(tmpDir)
	}
}

// BenchmarkPubSubThroughput benchmarks PubSub message throughput
func BenchmarkPubSubThroughput(b *testing.B) {
	// Create two nodes
	tmpDir1, _ := os.MkdirTemp("", "babylon-bench1-*")
	tmpDir2, _ := os.MkdirTemp("", "babylon-bench2-*")
	defer os.RemoveAll(tmpDir1)
	defer os.RemoveAll(tmpDir2)

	ctx := context.Background()

	config1 := DefaultConfig()
	config1.RepoDir = tmpDir1
	config1.BootstrapPeers = []string{}

	node1, _ := NewNode(config1)
	node1.Start()
	defer stopNode(node1)()

	config2 := DefaultConfig()
	config2.RepoDir = tmpDir2
	config2.BootstrapPeers = []string{}

	node2, _ := NewNode(config2)
	node2.Start()
	defer stopNode(node2)()

	// Connect nodes
	node2.Connect(ctx, node1.Multiaddrs()[0])
	time.Sleep(500 * time.Millisecond)

	// Subscribe
	sub, _ := node2.Subscribe("bench-topic")
	defer closeSub(sub)()
	time.Sleep(500 * time.Millisecond)

	message := []byte("Benchmark message for throughput testing")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node1.Publish(ctx, "bench-topic", message)
		<-sub.Messages
	}
}
