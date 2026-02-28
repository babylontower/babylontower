# Babylon Tower - Manual Integration Testing Scripts

**Last Updated:** February 23, 2026  
**Phase:** 6 - Testing & Validation

---

## Quick Start

### Two-Instance Testing (Basic)

```bash
# Terminal 1
make launch-instance1

# Terminal 2
make launch-instance2

# Then follow on-screen instructions to connect instances
```

### Multi-Node Testing (Scale/Phase 6)

```bash
# Launch 5 nodes (default)
make launch-multi-node

# Launch 20 nodes (scale test)
make launch-multi-node NODES=20

# Verify connections
make verify-connections

# Stop all nodes
make stop-multi-node

# Clean test data
make clean-test
```

---

## Script Inventory

| Script | Purpose | Make Target |
|--------|---------|-------------|
| `launch-instance1.sh` | Launch Alice instance | `make launch-instance1` |
| `launch-instance2.sh` | Launch Bob instance | `make launch-instance2` |
| `launch-multi-node.sh` | Launch N nodes | `make launch-multi-node` |
| `stop-multi-node.sh` | Stop all nodes | `make stop-multi-node` |
| `verify-connections.sh` | Verify connections | `make verify-connections` |
| `connect-instances.sh` | Connection helper | - |
| `clean-test-data.sh` | Clean test data | `make clean-test` |

---

## Detailed Usage

### Two-Instance Testing

**Step 1: Launch Instances**

```bash
# Terminal 1
make launch-instance1

# Terminal 2  
make launch-instance2
```

**Step 2: Get Public Keys**

In each terminal:
```
>>> /myid
```

Copy the Ed25519 public key (hex or base58).

**Step 3: Add Contacts**

In Alice's terminal:
```
>>> /add <Bob's_public_key> Bob
```

In Bob's terminal:
```
>>> /add <Alice's_public_key> Alice
```

**Step 4: Start Chat**

In both terminals:
```
>>> /chat 1
```

**Step 5: Exchange Messages**

Type messages in either terminal. Messages should appear in both.

**Step 6: Verify Persistence**

Exit both instances (`/exit`), then restart:
```bash
make launch-instance1
make launch-instance2
```

Verify contacts and message history persist.

---

### Multi-Node Testing (Phase 6)

**Launch N Nodes:**

```bash
# Launch 5 nodes in background
make launch-multi-node NODES=5

# Launch 20 nodes for scale test
make launch-multi-node NODES=20 MODE=background
```

**Wait for Network Formation:**

```bash
# Wait 30-60 seconds for DHT bootstrap and peer discovery
sleep 60
```

**Verify Connections:**

```bash
make verify-connections NODES=20
```

**Expected Output:**
```
╔═══════════════════════════════════════════════════════════╗
║     Babylon Tower - Connection Verifier                   ║
╚═══════════════════════════════════════════════════════════╝

Node Status:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✓ Node 1: Connected to 3 peers, DHT: 5 peers ✓
✓ Node 2: Connected to 2 peers, DHT: 4 peers ✓
✓ Node 3: Connected to 4 peers, DHT: 6 peers ✓
...

Summary:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Nodes checked:        20
  Nodes connected:      20
  Total connections:    58
  Total DHT peers:      95
  Avg connections/node: 2.9
  Avg DHT peers/node:   4.75

Phase 6 Acceptance Criteria:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✓ All nodes running: PASS
✓ Avg connections ≥2: PASS (2.9)
✓ Avg DHT peers ≥3: PASS (4.75)
✓ Network formation: PASS

Overall: 4/4 criteria passed
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✓ Multi-node network test PASSED!
```

**Stop Nodes:**

```bash
make stop-multi-node
```

**Clean Test Data:**

```bash
make clean-test
```

---

## Test Scenarios

### Scenario 1: Basic Message Exchange

**Goal:** Verify two instances can exchange messages.

```bash
# Launch instances
make launch-instance1  # Terminal 1
make launch-instance2  # Terminal 2

# In Terminal 1:
>>> /myid              # Copy Alice's key
>>> /add <Bob's_key> Bob
>>> /chat 1
>>> Hello Bob!

# In Terminal 2:
>>> /myid              # Copy Bob's key
>>> /add <Alice's_key> Alice
>>> /chat 1
# Verify message appears

# Verify bidirectional
>>> Hi Alice! Received your message.

# In Terminal 1:
# Verify reply appears

# Exit and verify persistence
>>> /exit
```

**Expected:** Messages encrypted, transmitted, decrypted, and stored.

---

### Scenario 2: Identity Persistence

**Goal:** Verify identity survives restart.

```bash
# Launch instance
make launch-instance1

# Get public key
>>> /myid
# Note: Alice's public key = 0x1234...

# Exit
>>> /exit

# Restart
make launch-instance1

# Verify same key
>>> /myid
# Should show same public key
```

**Expected:** Same public key, mnemonic not regenerated.

---

### Scenario 3: Network Health Metrics

**Goal:** Verify network monitoring features.

```bash
make launch-instance1

# Check network status
>>> /network

# Expected output:
╔════════════════════════════════════════════════════════╗
║        Babylon Tower - Network Health Metrics         ║
╚════════════════════════════════════════════════════════╝

┌─ Node Information ───────────────────────────────────┐
│ Peer ID:      12D3KooW...
│ Uptime:       1h 23m
│ Started:      2026-02-23 10:15:30
└────────────────────────────────────────────────────────┘

┌─ Connection Metrics ─────────────────────────────────┐
│ Current Connections:    12
│ Total Connections:      45
│ Connection Success Rate: 78.3%
└────────────────────────────────────────────────────────┘

# Check DHT status
>>> /dhtinfo

# Check contact status
>>> /contactstatus
```

---

### Scenario 4: Scale Test (Phase 6.3.5)

**Goal:** Verify 20+ nodes maintain stable network.

```bash
# Full scale test
make test-scale

# Or manual steps:
make launch-multi-node NODES=20
sleep 60
make verify-connections NODES=20
make stop-multi-node
make clean-test
```

**Acceptance Criteria:**
- 20 nodes form stable network
- Average connections per node ≥3
- Average DHT routing table size ≥5
- No node crashes
- Memory usage per node <500MB

---

## Advanced Features

### Connection Helper (Auto Mode)

For tmux users, auto-connect instances:

```bash
# Launch instances in tmux
./scripts/test/launch-instance1.sh  # In tmux session
./scripts/test/launch-instance2.sh  # In tmux session

# Get multiaddrs
# In instance 1: /myaddr
# In instance 2: /myaddr

# Auto-connect
./scripts/test/connect-instances.sh auto \
  "/ip4/127.0.0.1/tcp/4001/p2p/QmAlice" \
  "/ip4/127.0.0.1/tcp/4002/p2p/QmBob"
```

---

### Launch Modes

**Background Mode (default):**
```bash
./scripts/test/launch-multi-node.sh 5 background
# Nodes run in background, output to log files
```

**Tmux Mode:**
```bash
./scripts/test/launch-multi-node.sh 5 tmux
# Nodes in tmux panes for interactive monitoring
```

---

## Troubleshooting

### Issue: Failed to Negotiate Security Protocol

**Error:**
```
❌ Error: Failed to connect: failed to negotiate security protocol: 
   wsarecv: An existing connection was forcibly closed by the remote host.
```

**Cause:** Both instances are using the same PeerID (sharing the same peer.key file).

**Solution:** This was a known issue fixed in the latest version. The scripts now properly
set the `HOME` environment variable to isolate instances. If you still see this error:

1. Clean test data: `make clean-test`
2. Ensure you're using the latest build: `make build`
3. Check that instances have different PeerIDs by running `/myid` in each

See `FIX-MULTI-INSTANCE.md` for technical details.

### Issue: Binary Not Found

```bash
make build
```

### Issue: Port Already in Use

```bash
# Stop any running nodes
make stop-multi-node

# Kill any remaining processes
pkill -f messenger

# Clean and retry
make clean-test
make launch-multi-node
```

### Issue: Connections Not Forming

**Check bootstrap connectivity:**
```bash
# In each node, run:
>>> /bootstrap

# Check DHT status:
>>> /dhtinfo

# Wait longer for network formation:
sleep 30
make verify-connections
```

### Issue: Verification Fails

**Check node logs:**
```bash
# View node 1 output
tail -f test-data/node-1/output.log

# Look for errors:
grep -i "error\|failed" test-data/node-*/output.log
```

---

## Test Data Location

| Mode | Data Directory |
|------|----------------|
| Two-instance | `test-data/instance1/`, `test-data/instance2/` |
| Multi-node | `test-data/node-1/` through `test-data/node-N/` |

**Clean test data:**
```bash
make clean-test
# or
./scripts/test/clean-test-data.sh
```

---

## Performance Benchmarks

**Run benchmarks:**
```bash
# Instance startup time
time make launch-instance1

# Network formation time
time (make launch-multi-node NODES=10 && sleep 30 && make verify-connections)

# Message delivery latency
# (Manual measurement in chat mode)
```

**Target Metrics:**

| Metric | Target |
|--------|--------|
| Bootstrap time (cold) | <30 seconds |
| Bootstrap time (warm) | <10 seconds |
| DHT routing table size | >10 peers |
| Connection success rate | >70% |
| Message delivery latency (P95) | <5 seconds |

---

## Integration with Automated Tests

**Run Go integration tests:**
```bash
make test-integration

# Or with verbose output
go test -v -tags=integration ./pkg/ipfsnode/... -timeout 5m
```

**Run specific test:**
```bash
go test -v -tags=integration ./test/... -run TestTwoInstanceCommunication
```

---

## Reference

- [Roadmap](../specs/roadmap.md)
- [Testing Specification](../specs/testing.md)
- [Technical Specification (PoC)](../specs/poc.md)
- [Integration Test Code](../test/integration_test.go)
- [Test Utilities](../test/testutil.go)

---

*Last updated: February 23, 2026*
*Version: 2.0 - Consolidated Specs*
