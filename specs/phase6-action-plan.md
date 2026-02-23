# Phase 6: Testing & Validation - Action Plan

**Created:** February 23, 2026  
**Target Completion:** February 27, 2026  
**Priority:** High

---

## Overview

Phase 6 focuses on comprehensive testing and validation of the IPFS infrastructure implemented in Phases 1-5. This plan outlines the remaining tests and benchmarks to be completed.

### Current Status: 50% Complete

**Completed Tests:**
- ✅ Config parsing tests
- ✅ PeerDB CRUD tests  
- ✅ Peer scoring tests
- ✅ Bootstrap logic tests
- ✅ Two-node bootstrap test
- ✅ DHT discovery test
- ✅ Peer persistence test

**Remaining Tests:** 9 tasks

---

## Action Items

### 1. Multi-Node Network Test (6.2.2) - P0

**Goal:** Verify that 5+ nodes can form a stable mesh network.

**Test Design:**
```go
// File: pkg/ipfsnode/multinode_test.go
func TestMultiNodeNetworkFormation(t *testing.T) {
    // Create 5 nodes with different configs
    nodes := make([]*Node, 5)
    for i := 0; i < 5; i++ {
        nodes[i] = createTestNode(t, fmt.Sprintf("node-%d", i))
    }
    
    // Start all nodes
    for _, node := range nodes {
        node.Start()
        defer node.Stop()
    }
    
    // Connect first node to bootstrap
    nodes[0].ConnectToPeer(bootstrapAddr)
    
    // Wait for network to form
    time.Sleep(10 * time.Second)
    
    // Verify each node has >2 connections
    for i, node := range nodes {
        info := node.GetNetworkInfo()
        if info.ConnectedPeerCount < 2 {
            t.Errorf("Node %d has only %d connections, expected >=2", i, info.ConnectedPeerCount)
        }
    }
    
    // Verify DHT routing tables are populated
    for i, node := range nodes {
        dhtInfo := node.GetDHTInfo()
        if dhtInfo.RoutingTableSize < 3 {
            t.Errorf("Node %d DHT routing table has only %d peers, expected >=3", i, dhtInfo.RoutingTableSize)
        }
    }
}
```

**Acceptance Criteria:**
- 5 nodes form mesh network within 30 seconds
- Each node has ≥2 peer connections
- Each node's DHT routing table has ≥3 peers
- No node crashes or panics

**Estimated Effort:** 2-3 hours

---

### 2. NAT Traversal Test (6.2.5) - P1

**Goal:** Verify hole punching works for NAT traversal.

**Test Design:**
```go
// File: pkg/ipfsnode/nat_test.go
func TestNATTraversal(t *testing.T) {
    // This test requires special network setup
    // Skip in CI, run manually in controlled environment
    
    if os.Getenv("BABYLON_NAT_TEST") == "" {
        t.Skip("NAT traversal test requires manual setup. Set BABYLON_NAT_TEST=1 to run.")
    }
    
    // Create two nodes behind NAT (simulated)
    nodeA := createTestNodeBehindNAT(t, "node-a")
    nodeB := createTestNodeBehindNAT(t, "node-b")
    
    nodeA.Start()
    nodeB.Start()
    defer nodeA.Stop()
    defer nodeB.Stop()
    
    // Attempt connection
    err := nodeA.ConnectToPeer(nodeB.Multiaddr())
    if err != nil {
        t.Fatalf("NAT traversal failed: %v", err)
    }
    
    // Verify connection established
    info := nodeA.GetNetworkInfo()
    if info.ConnectedPeerCount == 0 {
        t.Fatal("No connection established after NAT traversal")
    }
}
```

**Acceptance Criteria:**
- Two nodes behind NAT can establish connection
- Hole punching succeeds within 30 seconds
- Fallback to relay if direct connection fails

**Estimated Effort:** 3-4 hours (includes setup)

---

### 3. End-to-End Tests (6.3.x) - P0

#### 3.1 Fresh Install Test (6.3.1)

**Goal:** Verify new node bootstraps successfully.

**Test Design:**
```go
// File: test/e2e/fresh_install_test.go
func TestFreshInstallBootstrap(t *testing.T) {
    // Create fresh directory (simulating new install)
    dir := t.TempDir()
    
    // Create node with default config
    node := createNodeWithDefaults(t, dir)
    
    // Start node
    node.Start()
    defer node.Stop()
    
    // Wait for bootstrap with timeout
    err := node.WaitForDHT(30 * time.Second)
    if err != nil {
        t.Fatalf("Bootstrap failed: %v", err)
    }
    
    // Verify bootstrap succeeded
    dhtInfo := node.GetDHTInfo()
    if dhtInfo.RoutingTableSize == 0 {
        t.Fatal("DHT routing table empty after bootstrap")
    }
    
    // Verify metrics
    metrics := node.GetMetrics()
    if metrics.BootstrapSuccesses == 0 {
        t.Error("No successful bootstrap recorded")
    }
}
```

**Acceptance Criteria:**
- New node bootstraps within 30 seconds
- DHT routing table populated (≥5 peers)
- At least one bootstrap connection successful

**Estimated Effort:** 1 hour

#### 3.2 Restart Test (6.3.2)

**Goal:** Verify node reconnects after restart.

**Test Design:**
```go
// File: test/e2e/restart_test.go
func TestNodeRestartReconnection(t *testing.T) {
    dir := t.TempDir()
    
    // First run
    node1 := createNodeWithDefaults(t, dir)
    node1.Start()
    
    // Wait for connections
    time.Sleep(5 * time.Second)
    initialConnections := node1.GetNetworkInfo().ConnectedPeerCount
    
    // Store peer count
    storedPeers, _ := node1.ListStoredPeers()
    
    node1.Stop()
    
    // Second run (restart)
    node2 := createNodeWithDefaults(t, dir)
    node2.Start()
    defer node2.Stop()
    
    // Wait for reconnection
    time.Sleep(5 * time.Second)
    
    // Verify faster bootstrap (warm start)
    metrics := node2.GetMetrics()
    if metrics.BootstrapAttempts > 3 {
        t.Errorf("Warm bootstrap took too many attempts: %d", metrics.BootstrapAttempts)
    }
    
    // Verify peers restored
    restoredPeers, _ := node2.ListStoredPeers()
    if len(restoredPeers) < len(storedPeers)/2 {
        t.Errorf("Only %d peers restored, expected >=%d", len(restoredPeers), len(storedPeers)/2)
    }
}
```

**Acceptance Criteria:**
- Node reconnects within 10 seconds on restart
- Stored peers are restored
- Warm bootstrap faster than cold bootstrap

**Estimated Effort:** 1-2 hours

#### 3.3 Contact Messaging Test (6.3.3)

**Goal:** Verify messages are delivered between contacts.

**Test Design:**
```go
// File: test/e2e/contact_messaging_test.go
func TestContactMessaging(t *testing.T) {
    // Create two nodes (Alice and Bob)
    aliceDir := t.TempDir()
    bobDir := t.TempDir()
    
    alice := createNodeWithDefaults(t, aliceDir)
    bob := createNodeWithDefaults(t, bobDir)
    
    alice.Start()
    bob.Start()
    defer alice.Stop()
    defer bob.Stop()
    
    // Connect nodes
    alice.ConnectToPeer(bob.Multiaddr())
    time.Sleep(2 * time.Second)
    
    // Create messaging services
    aliceMsg := createMessagingService(t, alice)
    bobMsg := createMessagingService(t, bob)
    
    // Add each other as contacts
    bobContact := &pb.Contact{
        PublicKey:       bob.Identity.PublicKey,
        X25519PublicKey: bob.Identity.X25519PubKey,
    }
    aliceMsg.Storage.AddContact(bobContact)
    
    // Send message
    result, err := aliceMsg.SendMessage("Hello Bob!", bob.Identity.PublicKey, bob.Identity.X25519PubKey)
    if err != nil {
        t.Fatalf("Failed to send message: %v", err)
    }
    
    // Wait for message delivery
    select {
    case msg := <-bobMsg.Messages():
        if msg.Message.Text != "Hello Bob!" {
            t.Errorf("Wrong message received: %s", msg.Message.Text)
        }
    case <-time.After(10 * time.Second):
        t.Fatal("Message not received within timeout")
    }
    
    // Verify message stored
    messages, _ := bobMsg.Storage.GetMessages(bob.Identity.PublicKey, 10, 0)
    if len(messages) == 0 {
        t.Error("Message not stored in database")
    }
}
```

**Acceptance Criteria:**
- Message sent from Alice to Bob
- Bob receives message within 5 seconds
- Message stored in Bob's database
- Message can be retrieved from history

**Estimated Effort:** 2-3 hours

#### 3.4 Network Partition Test (6.3.4) - P1

**Goal:** Verify network heals after partition.

**Test Design:**
```go
// File: test/e2e/network_partition_test.go
func TestNetworkPartitionRecovery(t *testing.T) {
    // Create 3 nodes
    nodes := make([]*Node, 3)
    for i := 0; i < 3; i++ {
        nodes[i] = createTestNode(t, fmt.Sprintf("node-%d", i))
        nodes[i].Start()
    }
    
    // Form network
    connectNodes(t, nodes)
    time.Sleep(5 * time.Second)
    
    // Simulate partition (stop middle node)
    nodes[1].Stop()
    time.Sleep(2 * time.Second)
    
    // Verify nodes 0 and 2 can still communicate
    // (they should reconnect via DHT)
    
    // Restart node 1
    nodes[1] = createTestNode(t, "node-1-restart")
    nodes[1].Start()
    defer nodes[1].Stop()
    
    // Wait for network to heal
    time.Sleep(10 * time.Second)
    
    // Verify all nodes reconnected
    for i, node := range nodes {
        info := node.GetNetworkInfo()
        if info.ConnectedPeerCount < 2 {
            t.Errorf("Node %d has only %d connections after partition heal", i, info.ConnectedPeerCount)
        }
    }
}
```

**Acceptance Criteria:**
- Network detects partition
- Remaining nodes maintain connectivity
- Network heals when partitioned node returns
- No data loss during partition

**Estimated Effort:** 2-3 hours

#### 3.5 Scale Test (6.3.5) - P1

**Goal:** Verify 20+ nodes maintain stable network.

**Test Design:**
```go
// File: test/e2e/scale_test.go
func TestLargeNetworkScale(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping scale test in short mode")
    }
    
    const nodeCount = 20
    
    nodes := make([]*Node, nodeCount)
    for i := 0; i < nodeCount; i++ {
        nodes[i] = createTestNode(t, fmt.Sprintf("scale-node-%d", i))
        nodes[i].Start()
        defer nodes[i].Stop()
    }
    
    // Connect first few nodes to bootstrap
    for i := 0; i < 5; i++ {
        nodes[i].ConnectToPeer(bootstrapAddr)
    }
    
    // Wait for network to form
    time.Sleep(30 * time.Second)
    
    // Collect metrics
    totalConnections := 0
    totalDHTPeers := 0
    
    for i, node := range nodes {
        info := node.GetNetworkInfo()
        totalConnections += info.ConnectedPeerCount
        
        dhtInfo := node.GetDHTInfo()
        totalDHTPeers += dhtInfo.RoutingTableSize
    }
    
    avgConnections := float64(totalConnections) / nodeCount
    avgDHTPeers := float64(totalDHTPeers) / nodeCount
    
    t.Logf("Average connections per node: %.2f", avgConnections)
    t.Logf("Average DHT peers per node: %.2f", avgDHTPeers)
    
    // Verify minimum connectivity
    if avgConnections < 3 {
        t.Errorf("Average connections too low: %.2f", avgConnections)
    }
    
    if avgDHTPeers < 5 {
        t.Errorf("Average DHT peers too low: %.2f", avgDHTPeers)
    }
}
```

**Acceptance Criteria:**
- 20 nodes form stable network
- Average connections per node ≥3
- Average DHT routing table size ≥5
- No node crashes under load
- Memory usage per node <500MB

**Estimated Effort:** 4-5 hours

---

### 4. Performance Benchmarks (6.4) - P1

**Goal:** Measure and document performance metrics.

**Benchmark Design:**
```go
// File: test/benchmark/performance_test.go

func BenchmarkBootstrapTime(b *testing.B) {
    // Measure cold bootstrap time
    for i := 0; i < b.N; i++ {
        b.StopTimer()
        dir := b.TempDir()
        node := createNodeWithDefaults(b, dir)
        b.StartTimer()
        
        node.Start()
        start := time.Now()
        node.WaitForDHT(60 * time.Second)
        elapsed := time.Since(start)
        
        b.ReportMetric(float64(elapsed.Milliseconds()), "ms/bootstrap")
        node.Stop()
    }
}

func BenchmarkMessageDeliveryLatency(b *testing.B) {
    // Measure P95 message delivery latency
    latencies := make([]time.Duration, b.N)
    
    alice := createTestNode(b, "alice")
    bob := createTestNode(b, "bob")
    alice.Start()
    bob.Start()
    defer alice.Stop()
    defer bob.Stop()
    
    alice.ConnectToPeer(bob.Multiaddr())
    
    for i := 0; i < b.N; i++ {
        start := time.Now()
        // Send message
        // Wait for receipt
        latencies[i] = time.Since(start)
    }
    
    // Calculate P95
    sort.Slice(latencies, func(i, j int) bool {
        return latencies[i] < latencies[j]
    })
    p95 := latencies[int(float64(len(latencies))*0.95)]
    
    b.ReportMetric(float64(p95.Milliseconds()), "ms/p95")
}

func BenchmarkPeerDBSize(b *testing.B) {
    // Measure peer DB growth over time
    dir := b.TempDir()
    node := createNodeWithDefaults(b, dir)
    node.Start()
    defer node.Stop()
    
    // Run for 1 hour (simulated)
    time.Sleep(1 * time.Hour)
    
    peers, _ := node.ListStoredPeers()
    b.ReportMetric(float64(len(peers)), "peers")
}
```

**Target Metrics:**

| Metric | Target | Measurement |
|--------|--------|-------------|
| Bootstrap time (cold) | <30 seconds | First connection |
| Bootstrap time (warm) | <10 seconds | With stored peers |
| DHT routing table size | >10 peers | After bootstrap |
| Connection success rate | >70% | Successful / attempted |
| Message delivery latency (P95) | <5 seconds | P95 latency |
| Peer DB size | ≤100 peers | After 1 week |
| Memory usage | <500MB | Per node |
| CPU usage | <10% | Average |

**Estimated Effort:** 3-4 hours

---

### 5. Test Report Document (6.4) - P1

**Goal:** Document test results and recommendations.

**Template:**
```markdown
# Phase 6 Test Report

## Executive Summary
- Tests run: X
- Tests passed: Y
- Tests failed: Z
- Overall status: PASS/FAIL

## Test Results

### Unit Tests
- Config parsing: PASS
- PeerDB CRUD: PASS
- ...

### Integration Tests
- Two-node bootstrap: PASS
- Multi-node network: PASS/FAIL
- ...

### End-to-End Tests
- Fresh install: PASS
- Restart: PASS
- Contact messaging: PASS
- ...

### Performance Benchmarks
| Metric | Target | Actual | Status |
|--------|--------|--------|--------|
| Bootstrap time (cold) | <30s | 25s | PASS |
| ... | ... | ... | ... |

## Issues Found
1. [Issue description]
   - Severity: High/Medium/Low
   - Status: Open/Resolved
   - Recommendation: ...

## Recommendations
1. ...

## Conclusion
[Summary of test outcomes and readiness for production]
```

**Estimated Effort:** 2-3 hours

---

## Timeline

| Day | Tasks | Estimated Hours |
|-----|-------|-----------------|
| Day 1 | Multi-node test (6.2.2), Fresh install test (6.3.1) | 4-5 hours |
| Day 2 | Restart test (6.3.2), Contact messaging test (6.3.3) | 4-5 hours |
| Day 3 | Network partition test (6.3.4), Scale test (6.3.5) | 6-8 hours |
| Day 4 | NAT traversal test (6.2.5), Performance benchmarks (6.4) | 7-8 hours |
| Day 5 | Test report document, Bug fixes, Retries | 5-6 hours |

**Total Estimated Effort:** 26-32 hours

---

## Success Criteria

Phase 6 is complete when:

- ✅ All P0 tests implemented and passing
- ✅ At least 80% of P1 tests implemented and passing
- ✅ Performance benchmarks meet targets
- ✅ Test report document completed
- ✅ No critical bugs found (or all resolved)

---

## Dependencies

- Phases 1-5: Complete ✅
- Test environment: Available ✅
- CI/CD pipeline: Configured (for automated test runs)
- Documentation: Updated after test completion

---

## Risk Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| NAT test environment unavailable | Medium | Skip in CI, document manual test procedure |
| Scale test too resource-intensive | Medium | Reduce node count to 10, document limitations |
| Flaky tests | Low | Add retry logic, improve test stability |
| Performance targets not met | High | Profile code, optimize bottlenecks, adjust targets if needed |

---

*Created: February 23, 2026*  
*Last Updated: February 23, 2026*
