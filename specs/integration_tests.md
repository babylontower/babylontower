# Babylon Tower - Integration Test Plan

## Overview

This document describes the integration tests for Babylon Tower that require network connectivity or multiple nodes. These tests are separated from unit tests to ensure CI pipelines run reliably without network dependencies.

## Running Integration Tests

Integration tests are marked with the `integration` build tag and are NOT run during normal CI builds.

### Run Integration Tests

```bash
# Run all integration tests
go test -tags=integration ./pkg/ipfsnode/... -v

# Run with race detector
go test -tags=integration -race ./pkg/ipfsnode/... -v

# Run specific test
go test -tags=integration ./pkg/ipfsnode/... -run TestTwoNodePubSub -v

# Run with timeout
go test -tags=integration ./pkg/ipfsnode/... -timeout 5m
```

### Skip Integration Tests (Default CI Behavior)

```bash
# Normal test run (excludes integration tests)
go test ./...

# With short flag (integration tests should check testing.Short())
go test -short ./...
```

## Integration Tests

### 1. TwoNodePubSub

**Purpose:** Verify PubSub message delivery between two IPFS nodes.

**Requirements:**
- Two IPFS nodes on same machine
- mDNS or manual peer discovery
- Working PubSub subscription

**Test Flow:**
1. Create two nodes with separate repos
2. Start both nodes
3. Wait for peer discovery (mDNS, 15s timeout)
4. Node2 subscribes to topic
5. Node1 publishes message
6. Node2 receives message

**Expected Result:** Message delivered successfully

**Failure Modes:**
- mDNS not working in isolated environment
- PubSub mesh not formed
- Port conflicts

---

### 2. TwoNodeBidirectional

**Purpose:** Verify bidirectional communication between two nodes.

**Requirements:**
- Two IPFS nodes
- Working PubSub on both nodes

**Test Flow:**
1. Create and start two nodes
2. Both nodes subscribe to same topic
3. Node1 → Node2 message
4. Node2 → Node1 message
5. Verify both received

**Expected Result:** Both messages delivered

---

### 3. NodeConnectManual

**Purpose:** Test manual peer connection using multiaddr.

**Requirements:**
- Two IPFS nodes
- Network connectivity between nodes

**Test Flow:**
1. Start Node1, get its multiaddr
2. Node2 connects to Node1 using `ConnectToPeer()`
3. Verify connection established

**Expected Result:** Connection successful or graceful failure with error

---

### 4. MultipleNodesMesh

**Purpose:** Test PubSub mesh formation with 3+ nodes.

**Requirements:**
- 3+ IPFS nodes
- mDNS or manual peering
- Longer timeout for mesh formation

**Test Flow:**
1. Create 3 nodes
2. All subscribe to same topic
3. Node0 publishes
4. Nodes 1 and 2 should receive

**Expected Result:** Message broadcast to all nodes

**Note:** Skipped in `testing.Short()` mode

---

### 5. NodeRestartPersistence

**Purpose:** Verify node identity persists across restarts.

**Requirements:**
- Single IPFS node
- Persistent repo directory

**Test Flow:**
1. Start node, record PeerID
2. Stop node
3. Restart node (same repo)
4. Verify PeerID unchanged

**Expected Result:** PeerID preserved

---

### 6. ConcurrentPublishSubscribe

**Purpose:** Test concurrent publish operations.

**Requirements:**
- Single IPFS node
- Multiple subscriptions

**Test Flow:**
1. Create node with 5 subscriptions
2. Publish 10 messages to each topic concurrently
3. Verify no errors

**Expected Result:** All publishes succeed

---

### 7. LargeMessage

**Purpose:** Test large message handling (100KB).

**Requirements:**
- Single IPFS node
- PubSub working

**Test Flow:**
1. Subscribe to topic
2. Publish 100KB message
3. Verify received completely

**Expected Result:** Large message delivered

---

### 8. TempDirCleanup

**Purpose:** Verify temporary directories are managed correctly.

**Requirements:**
- Single IPFS node

**Test Flow:**
1. Create node in temp dir
2. Start and stop
3. Verify repo exists after stop
4. TempDir cleanup is automatic

**Expected Result:** Proper directory lifecycle

---

## CI Integration

### GitHub Actions Configuration

Add to `.github/workflows/ci.yml`:

```yaml
# Unit tests (runs on every PR)
- name: Run unit tests
  run: go test -v ./...

# Integration tests (runs on main branch or nightly)
- name: Run integration tests
  if: github.ref == 'refs/heads/main' || github.event_name == 'schedule'
  run: go test -tags=integration -v ./pkg/ipfsnode/...
  timeout-minutes: 10
```

### Makefile Targets

```makefile
# Run unit tests only
test:
	go test -short ./...

# Run integration tests
test-integration:
	go test -tags=integration ./pkg/ipfsnode/... -v -timeout 5m

# Run all tests (unit + integration)
test-all:
	go test ./...
	go test -tags=integration ./pkg/ipfsnode/... -timeout 5m
```

---

## Environment Requirements

### For Local Testing

1. **Network Access:**
   - No firewall blocking localhost connections
   - mDNS allowed (UDP 5353)

2. **Ports:**
   - Dynamic port allocation (no fixed ports)
   - Multiple ports available for TCP/WebSocket

3. **System Resources:**
   - 500MB+ free RAM for multi-node tests
   - 1GB+ free disk for IPFS repos

### For CI Testing

1. **Docker Environment:**
   ```yaml
   services:
     - docker:dind  # If using Docker-in-Docker
   ```

2. **Network Mode:**
   - Use `host` network mode for mDNS
   - Or configure explicit peer connections

3. **Timeouts:**
   - Increase test timeout to 5-10 minutes
   - mDNS discovery may take 15-30 seconds

---

## Troubleshooting

### mDNS Not Working

**Symptom:** Tests timeout waiting for peer discovery

**Solutions:**
1. Use manual connection: `node2.ConnectToPeer(node1.Multiaddrs()[0])`
2. Configure bootstrap peers explicitly
3. Run with host network in CI

### Port Conflicts

**Symptom:** "address already in use"

**Solutions:**
1. Use `/ip4/0.0.0.0/tcp/0` for dynamic ports
2. Add retry logic with different ports
3. Clean up stopped nodes properly

### PubSub Not Delivering Messages

**Symptom:** Messages published but not received

**Solutions:**
1. Wait longer for mesh formation (2-5 seconds)
2. Verify peers are connected before publishing
3. Check topic names match exactly

---

## Test Coverage Goals

| Test | Unit | Integration | E2E |
|------|------|-------------|-----|
| Node creation/start/stop | ✅ | | |
| Topic subscribe/publish | ✅ | ✅ | |
| Peer discovery (mDNS) | | ✅ | |
| Peer discovery (DHT) | | ✅ | |
| Manual peer connection | | ✅ | |
| PubSub message delivery | | ✅ | ✅ |
| Multi-node mesh | | ✅ | |
| Identity persistence | ✅ | ✅ | |
| Concurrent operations | ✅ | ✅ | |
| Large messages | | ✅ | |
| Network partitions | | | ✅ |
| Bootstrap from stored peers | | | ✅ |

---

## Future Integration Tests

### Planned Tests

1. **BootstrapFromStoredPeers**
   - Store peers in database
   - Restart node
   - Verify auto-connect to stored peers

2. **DHTBootstrap**
   - Connect to public bootstrap nodes
   - Verify DHT routing table populated
   - Test FindPeer queries

3. **NATTraversal**
   - Test hole punching between nodes
   - Verify relay fallback

4. **ContactDiscovery**
   - Add contact with peer ID
   - Verify DHT lookup finds contact
   - Test message delivery to contact

5. **NetworkPartition**
   - Split network into two groups
   - Verify messages don't cross partition
   - Heal partition, verify sync

---

## Maintenance

### When to Update

- Adding new network features
- Changing peer discovery logic
- Modifying PubSub configuration
- Updating libp2p dependencies

### Review Checklist

- [ ] Tests pass locally
- [ ] Tests pass in CI environment
- [ ] Timeout values are appropriate
- [ ] Cleanup is proper (no resource leaks)
- [ ] Error messages are clear
- [ ] Documentation updated

---

*Last updated: February 22, 2026*
*Version: 1.0*
