# Manual Integration Testing Scripts - Review & Update Plan

**Review Date:** February 23, 2026  
**Status:** Scripts Functional - Minor Updates Recommended

---

## Executive Summary

The existing manual integration testing scripts in `scripts/test/` are **functional and well-structured** for basic two-instance testing. However, they need minor updates to:

1. Support new Phase 4-5 features (network metrics, contact status)
2. Add multi-node testing capabilities (5+ nodes for Phase 6)
3. Improve Docker support for isolated testing
4. Add automated connection verification

---

## Current Script Inventory

### ✅ Available Scripts

| Script | Purpose | Status |
|--------|---------|--------|
| `scripts/test/launch-instance1.sh` | Launch Alice instance | ✅ Functional |
| `scripts/test/launch-instance2.sh` | Launch Bob instance | ✅ Functional |
| `scripts/test/clean-test-data.sh` | Clean test data | ✅ Functional |
| `scripts/test/connect-instances.sh` | Connection helper | ⚠️ Needs update |
| `scripts/test/launch-two-instances.sh` | Combined launcher | ⚠️ Missing |
| `test/integration_test.go` | Go integration tests | ✅ Functional |
| `test/testutil.go` | Test utilities | ✅ Functional |
| `test/docker-compose.yml` | Docker testing | ⚠️ Needs update |

---

## Script Analysis

### 1. Launch Scripts (`launch-instance1.sh`, `launch-instance2.sh`)

**Strengths:**
- ✅ Platform detection (Linux/macOS/Windows)
- ✅ Automatic binary build if missing
- ✅ Separate data directories
- ✅ Clear instructions on startup
- ✅ Uses `HOME` env var for data isolation

**Recommended Updates:**
- ⚠️ Add network status display after startup
- ⚠️ Add wait for DHT bootstrap completion
- ⚠️ Show `/network` command availability

**Update Priority:** Medium

---

### 2. Clean Script (`clean-test-data.sh`)

**Strengths:**
- ✅ Confirmation prompt
- ✅ Clear feedback
- ✅ Safe deletion

**Status:** ✅ No updates needed

---

### 3. Connect Helper (`connect-instances.sh`)

**Current Limitations:**
- ⚠️ Only provides instructions, doesn't automate connection
- ⚠️ Doesn't verify connection success
- ⚠️ No support for auto-discovery via DHT

**Recommended Updates:**
- Add automated `/connect` command injection
- Add connection verification via `/peers`
- Add DHT discovery wait option

**Update Priority:** High

---

### 4. Missing: Multi-Node Launcher (`launch-two-instances.sh`)

**Referenced in README but missing:**
```bash
./scripts/test/launch-two-instances.sh [mode]
```

**Should support:**
- `local` mode: Launch two instances in tmux/screen
- `docker` mode: Launch via docker-compose
- `clean` mode: Clean test data

**Creation Priority:** High

---

## Recommended New Scripts

### 1. Multi-Node Test Launcher (`launch-multi-node.sh`)

For Phase 6 testing (5+ nodes):

```bash
#!/bin/bash
# Usage: ./scripts/test/launch-multi-node.sh <node_count>

NODE_COUNT=${1:-5}

for i in $(seq 1 $NODE_COUNT); do
    DATA_DIR="$PROJECT_ROOT/test-data/node-$i"
    mkdir -p "$DATA_DIR"
    export HOME="$DATA_DIR"
    
    # Launch in background or tmux pane
    $BINARY &
done
```

**Features:**
- Launch N nodes for scale testing
- Automatic data directory management
- Optional tmux integration for monitoring

---

### 2. Connection Verifier (`verify-connections.sh`)

Automated connection verification:

```bash
#!/bin/bash
# Checks if all nodes are connected

for i in $(seq 1 $NODE_COUNT); do
    # Send /peers command via tmux or FIFO
    # Parse output to count connections
    # Report connection matrix
done
```

**Features:**
- Connection matrix display
- DHT routing table size check
- Automated pass/fail reporting

---

### 3. Test Scenario Runner (`run-scenario.sh`)

Automated test scenario execution:

```bash
#!/bin/bash
# Usage: ./scripts/test/run-scenario.sh <scenario_name>

case "$1" in
    basic_messaging)
        # Launch 2 nodes
        # Exchange keys
        # Send messages
        # Verify delivery
        ;;
    persistence)
        # Launch node
        # Add contacts
        # Send messages
        # Restart
        # Verify persistence
        ;;
    scale_test)
        # Launch 20 nodes
        # Wait for network formation
        # Measure connection stats
        ;;
esac
```

---

## Docker Updates Needed

### Current `docker-compose.yml` Issues

1. ⚠️ Uses old port configuration
2. ⚠️ No support for >2 nodes
3. ⚠️ No health checks
4. ⚠️ No network metrics exposure

### Recommended `docker-compose.yml` Update

```yaml
version: '3.8'

services:
  babylon-node-1:
    build: .
    environment:
      - BABYLON_DATA_DIR=/data/node1
      - BABYLON_BOOTSTRAP_PEERS=/dns/babylon-bootstrap/tcp/4001/p2p/QmBootstrap
    ports:
      - "4001:4001"
      - "4002:4002"
    volumes:
      - ./test-data/node1:/data
    healthcheck:
      test: ["CMD", "/app/messenger", "-cmd", "/peers"]
      interval: 10s
      timeout: 5s
      retries: 3

  babylon-node-2:
    build: .
    environment:
      - BABYLON_DATA_DIR=/data/node2
    ports:
      - "4011:4001"
      - "4012:4002"
    volumes:
      - ./test-data/node2:/data

  # Add more nodes for scale testing
  babylon-node-3:
    # ...
```

---

## Updated Test Procedures

### Phase 6 Test Scenarios

#### Scenario 1: Multi-Node Network Formation (6.2.2)

```bash
# 1. Launch 5 nodes
./scripts/test/launch-multi-node.sh 5

# 2. Wait for network formation
sleep 30

# 3. Verify connections
./scripts/test/verify-connections.sh

# 4. Check DHT routing tables
for i in $(seq 1 5); do
    echo "Node $i DHT status:"
    # Send /dhtinfo to node $i
done

# 5. Cleanup
./scripts/test/clean-test-data.sh
```

**Expected Results:**
- All 5 nodes connected
- Each node has ≥2 peer connections
- DHT routing table has ≥3 peers per node

---

#### Scenario 2: Contact Messaging E2E (6.3.3)

```bash
# 1. Launch two instances
./scripts/test/launch-instance1.sh &
./scripts/test/launch-instance2.sh &

# 2. Wait for startup
sleep 5

# 3. Run automated test
./scripts/test/run-scenario.sh contact_messaging

# Expected output:
# ✓ Instance 1 started
# ✓ Instance 2 started
# ✓ Keys exchanged
# ✓ Contacts added
# ✓ Message sent: "Hello Bob!"
# ✓ Message received by Bob
# ✓ Message stored in database
# ✓ Test PASSED
```

---

#### Scenario 3: Scale Test (6.3.5)

```bash
# 1. Launch 20 nodes
./scripts/test/launch-multi-node.sh 20

# 2. Wait for network formation
sleep 60

# 3. Collect metrics
./scripts/test/collect-metrics.sh > metrics.json

# 4. Analyze results
# - Average connections per node
# - Average DHT routing table size
# - Memory usage per node
# - Message delivery latency

# 5. Cleanup
./scripts/test/clean-test-data.sh
```

---

## Implementation Plan

### Week 1: Script Updates

| Day | Task | Priority |
|-----|------|----------|
| 1 | Update `connect-instances.sh` with automation | High |
| 2 | Create `launch-two-instances.sh` | High |
| 3 | Create `launch-multi-node.sh` | High |
| 4 | Create `verify-connections.sh` | Medium |
| 5 | Update `docker-compose.yml` | Medium |

### Week 2: Test Scenarios

| Day | Task | Priority |
|-----|------|----------|
| 1 | Create `run-scenario.sh` framework | High |
| 2 | Implement basic_messaging scenario | High |
| 3 | Implement persistence scenario | Medium |
| 4 | Implement scale_test scenario | Medium |
| 5 | Test all scenarios, fix bugs | High |

---

## Quick Wins (Immediate Updates)

### 1. Update Launch Scripts with Network Info

Add to end of `launch-instance1.sh` and `launch-instance2.sh`:

```bash
# After instance starts, show helpful commands
cat << 'EOF'

═══════════════════════════════════════════════════
Testing Commands:
  /myid          - Get your public keys
  /network       - View network health metrics
  /peers         - List connected peers
  /dhtinfo       - View DHT routing table
  /contactstatus - View contact online status
═══════════════════════════════════════════════════

EOF
```

### 2. Add Connection Helper Script

Create `scripts/test/auto-connect.sh`:

```bash
#!/bin/bash
# Automatically connect two running instances

# Get multiaddrs from running instances
ADDR1=$(docker exec babylon-alice /app/messenger -cmd "/myaddr" | grep -oP '/ip4.*')
ADDR2=$(docker exec babylon-bob /app/messenger -cmd "/myaddr" | grep -oP '/ip4.*')

# Send connect commands
docker exec babylon-alice /app/messenger -cmd "/connect $ADDR2"
docker exec babylon-bob /app/messenger -cmd "/connect $ADDR1"

# Verify
sleep 2
echo "=== Connection Status ==="
docker exec babylon-alice /app/messenger -cmd "/peers"
docker exec babylon-bob /app/messenger -cmd "/peers"
```

---

## Makefile Updates

Add new targets to `Makefile`:

```makefile
## launch-multi-node: Launch N nodes for scale testing
launch-multi-node: build-all
	@$(TEST_SCRIPTS_DIR)/launch-multi-node.sh $(or $(NODES),5)

## verify-connections: Verify node connections
verify-connections:
	@$(TEST_SCRIPTS_DIR)/verify-connections.sh

## run-scenario: Run test scenario
run-scenario:
	@$(TEST_SCRIPTS_DIR)/run-scenario.sh $(SCENARIO)

## test-scale: Run scale test (20 nodes)
test-scale: build-all
	@$(TEST_SCRIPTS_DIR)/launch-multi-node.sh 20
	@sleep 60
	@$(TEST_SCRIPTS_DIR)/verify-connections.sh
	@$(TEST_SCRIPTS_DIR)/collect-metrics.sh
	@$(TEST_SCRIPTS_DIR)/clean-test-data.sh
```

---

## Conclusion

**Current Status:**
- ✅ Basic two-instance testing scripts are functional
- ⚠️ Missing multi-node testing capabilities
- ⚠️ Connection verification is manual
- ⚠️ Docker support needs updates

**Recommended Actions:**
1. **Immediate:** Update launch scripts with network info display
2. **High Priority:** Create multi-node launcher for Phase 6 testing
3. **High Priority:** Automate connection verification
4. **Medium Priority:** Update Docker configuration
5. **Medium Priority:** Create automated test scenario framework

**Estimated Effort:**
- Script updates: 2-3 days
- New scripts: 3-4 days
- Testing & debugging: 2 days
- **Total: 7-9 days**

---

*Review completed: February 23, 2026*
