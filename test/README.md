# Babylon Tower - Phase 6 Testing Guide

## Overview

This guide covers the automation tools and testing procedures for Phase 6: Integration & Testing. It includes tools for launching multiple instances, Docker-based testing, and automated integration tests.

---

## Quick Reference

```bash
# Launch Instance 1 (Alice)
./scripts/test/launch-instance1.sh

# Launch Instance 2 (Bob)
./scripts/test/launch-instance2.sh

# Clean test data
./scripts/test/clean-test-data.sh

# Run integration tests
make test-integration
```

---

## Quick Start

### Local Testing (Recommended for Development)

```bash
# Build the application
make build

# Open two terminals and run:
# Terminal 1:
make launch-instance1

# Terminal 2:
make launch-instance2

# Or run the scripts directly:
# Terminal 1:
./scripts/test/launch-instance1.sh

# Terminal 2:
./scripts/test/launch-instance2.sh
```

### Docker Testing (Isolated Environment)

```bash
# Build Docker image and launch two containers
make launch-test-docker

# Or use docker-compose directly
make docker-run
```

---

## Testing Tools

### 1. Launch Scripts

**Instance 1 (Alice):**
```bash
./scripts/test/launch-instance1.sh
# or
make launch-instance1
```

**Instance 2 (Bob):**
```bash
./scripts/test/launch-instance2.sh
# or
make launch-instance2
```

**Clean test data:**
```bash
./scripts/test/clean-test-data.sh
# or
make clean-test
```

**Features:**
- Automatic binary build if missing
- Separate data directories for each instance
- Clear instructions displayed on startup
- Can be run in any terminal emulator

### 2. Combined Launch Script (`scripts/test/launch-two-instances.sh`)

Automates launching two Babylon Tower instances for manual testing.

**Usage:**
```bash
./scripts/test/launch-two-instances.sh [mode]

Modes:
  local    - Launch two instances locally (default)
  docker   - Launch two instances in Docker containers
  clean    - Clean test data and exit
  help     - Show help message
```

**Features:**
- Automatic binary build if missing
- Separate data directories for each instance
- Terminal window management (gnome-terminal, tmux, or background)
- Clear testing instructions displayed after launch

### 2. Makefile Targets

| Target | Description |
|--------|-------------|
| `make launch-test` | Launch two local instances |
| `make launch-test-docker` | Launch two Docker instances |
| `make test-integration` | Run integration tests |
| `make test-e2e` | Run end-to-end tests |
| `make docker-build` | Build Docker test image |
| `make docker-run` | Start Docker containers |
| `make docker-stop` | Stop Docker containers |
| `make docker-clean` | Clean Docker resources |
| `make clean-test` | Clean test artifacts |

### 3. Integration Test Framework (`test/testutil.go`)

Go package for programmatic test environment setup.

**Example:**
```go
package main

import "babylontower/test/testutil"

func main() {
    env, alice, bob, err := testutil.CreateTwoInstanceSetup("./bin/messenger")
    if err != nil {
        panic(err)
    }
    defer env.Cleanup()
    
    fmt.Printf("Alice: %s\n", alice.PublicKey)
    fmt.Printf("Bob: %s\n", bob.PublicKey)
    
    // Test messaging between instances
    // ...
}
```

### 4. Integration Tests (`test/integration_test.go`)

Automated tests for end-to-end scenarios.

**Run tests:**
```bash
# Run all integration tests
make test-integration

# Run specific test
go test -v -tags=integration ./test/... -run TestTwoInstanceCommunication

# Run with coverage
go test -v -tags=integration -coverprofile=coverage.out ./test/...
```

---

## Manual Testing Procedure

### Step 1: Launch Two Instances

**Terminal 1:**
```bash
make launch-instance1
```

**Terminal 2:**
```bash
make launch-instance2
```

This starts two instances with separate data directories:
- **Instance 1 (Alice)** - Data: `test-data/instance1`
- **Instance 2 (Bob)** - Data: `test-data/instance2`

### Step 2: Exchange Public Keys

In **Alice's** terminal:
```
>>> /myid
```

Copy Alice's public key (hex or base58 format).

In **Bob's** terminal:
```
>>> /myid
```

Copy Bob's public key.

### Step 3: Add Contacts

In **Alice's** terminal:
```
>>> /add <Bob's_public_key> Bob
```

In **Bob's** terminal:
```
>>> /add <Alice's_public_key> Alice
```

Verify contacts were added:
```
>>> /list
```

### Step 4: Start Chat

In **Alice's** terminal:
```
>>> /chat 1
```

In **Bob's** terminal:
```
>>> /chat 1
```

### Step 5: Exchange Messages

Type a message in Alice's terminal:
```
Hello Bob! This is a test message.
```

Verify Bob receives the message (shown in real-time).

Type a reply in Bob's terminal:
```
Hi Alice! Received your message.
```

Verify Alice receives the reply.

### Step 6: Check Message History

Exit chat mode (press Enter on empty line), then:
```
>>> /history Bob
```

Verify messages are stored and displayed correctly.

### Step 7: Test Persistence

Exit both instances:
```
>>> /exit
```

Restart instances:
```bash
make launch-test
```

Verify:
- Same public keys (identity persisted)
- Contacts still exist
- Message history preserved

---

## Docker Testing

### Setup

Docker provides an isolated environment for testing, ensuring reproducibility.

**Build image:**
```bash
make docker-build
```

### Running Containers

**Start two containers:**
```bash
make docker-run
```

This creates:
- `babylon-alice` - Ports 4001 (TCP), 4002 (WebSocket)
- `babylon-bob` - Ports 4011 (TCP), 4012 (WebSocket)

### Attaching to Containers

**Attach to Alice:**
```bash
docker exec -it babylon-alice /app/messenger
```

**Attach to Bob:**
```bash
docker exec -it babylon-bob /app/messenger
```

### Viewing Logs

```bash
# View Alice's logs
docker logs -f babylon-alice

# View Bob's logs
docker logs -f babylon-bob

# View both (in separate terminals)
docker logs -f babylon-alice &
docker logs -f babylon-bob &
```

### Cleanup

```bash
# Stop containers
make docker-stop

# Remove containers and test data
make docker-clean
```

---

## Test Scenarios

### Scenario 1: Basic Message Exchange

**Goal:** Verify two instances can exchange messages.

**Steps:**
1. Launch two instances
2. Exchange public keys
3. Add contacts
4. Enter chat mode
5. Send messages both directions
6. Verify message delivery

**Expected:** Messages encrypted, transmitted, and decrypted successfully.

### Scenario 2: Identity Persistence

**Goal:** Verify identity survives application restart.

**Steps:**
1. Launch instance, note public key
2. Exit instance
3. Restart instance
4. Verify same public key

**Expected:** Same public key, mnemonic not regenerated.

### Scenario 3: Contact Persistence

**Goal:** Verify contacts survive restart.

**Steps:**
1. Add contact
2. Exit instance
3. Restart instance
4. Run `/list`

**Expected:** Contact still in list.

### Scenario 4: Message History Persistence

**Goal:** Verify messages survive restart.

**Steps:**
1. Exchange messages
2. Exit instance
3. Restart instance
4. Run `/history <contact>`

**Expected:** Previous messages displayed.

### Scenario 5: Error Handling

**Goal:** Verify graceful error handling.

**Test Cases:**
- Invalid public key format
- Duplicate contact addition
- Invalid contact index
- Unknown command

**Expected:** User-friendly error messages, no crashes.

### Scenario 6: Graceful Shutdown

**Goal:** Verify clean shutdown.

**Test Cases:**
- `/exit` command
- Ctrl+C (SIGINT)
- SIGTERM signal

**Expected:** Resources cleaned up, no data loss.

---

## Troubleshooting

### Issue: Binary Not Found

**Error:** `Binary not found. Run 'make build' first`

**Solution:**
```bash
make build
```

### Issue: Port Already in Use (Docker)

**Error:** `Bind for 0.0.0.0:4001 failed: port is already allocated`

**Solution:**
```bash
# Stop existing containers
make docker-clean

# Or manually kill process using port
lsof -i :4001
kill <PID>
```

### Issue: Docker Compose Not Found

**Error:** `docker-compose not installed`

**Solution:**
```bash
# Install docker-compose
sudo apt-get install docker-compose

# Or use Docker Compose V2
docker compose version
```

### Issue: Terminal Not Supported

**Error:** Script cannot open terminal windows

**Solution:** Use the separate launch scripts in manual terminals:
```bash
# Terminal 1
./scripts/test/launch-instance1.sh

# Terminal 2
./scripts/test/launch-instance2.sh
```

### Issue: Integration Tests Fail

**Error:** Tests timeout or fail

**Solution:**
```bash
# Ensure binary is built
make build

# Run with verbose output
go test -v -tags=integration ./test/...

# Run specific test
go test -v -tags=integration ./test/... -run TestTwoInstanceCommunication
```

---

## Test Data Location

| Mode | Instance 1 | Instance 2 |
|------|------------|------------|
| Local | `test-data/instance1/` | `test-data/instance2/` |
| Docker | Volume mounted | Volume mounted |

**Clean test data:**
```bash
rm -rf test-data/
# or
make clean-test
```

---

## Performance Benchmarks

**Run benchmarks:**
```bash
go test -v -tags=integration -bench=. ./test/...
```

**Key metrics:**
- Instance startup time
- Message encryption/decryption latency
- PubSub message delivery time

---

## Next Steps

After completing manual testing:

1. **Document issues** found during testing
2. **Fix bugs** identified
3. **Update test coverage** if gaps found
4. **Prepare release notes** for PoC

---

## Reference

- [Roadmap - Phase 6](../specs/roadmap.md#phase-6-integration--testing)
- [PoC Testing Specification](../specs/PoCTesting.md)
- [Integration Test Code](./integration_test.go)
- [Test Utilities](./testutil.go)

---

*Last updated: February 21, 2026*
*Phase 6 Testing Tools v0.1.0*
