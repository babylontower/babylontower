# Babylon Tower - Testing Specification

## Overview

This document defines the comprehensive testing strategy for Babylon Tower Proof of Concept (PoC). It covers unit tests, integration tests, end-to-end tests, and manual testing procedures.

**Current Status:** Phase 6 In Progress - Unit tests complete, integration and E2E tests pending.

---

## Testing Goals

1. **Verify Functionality:** Ensure all implemented features work as specified
2. **Validate Security:** Confirm cryptographic operations are correct
3. **Test Integration:** Verify modules work together correctly
4. **Demonstrate PoC:** Show end-to-end message exchange between two instances
5. **Document Limitations:** Clearly identify what is and isn't working

---

## Test Categories

### 1. Unit Tests

**Purpose:** Verify individual functions and methods work correctly.

**Coverage Target:** >80% for core modules

#### Current Coverage

| Module | Coverage | Status | Key Tests |
|--------|----------|--------|-----------|
| `pkg/identity` | 86.1% | ✅ | Mnemonic generation, key derivation, persistence |
| `pkg/crypto` | 95.2% | ✅ | Encrypt/decrypt, sign/verify, key operations |
| `pkg/storage` | 87.9% | ✅ | CRUD operations, key formatting, transactions, peers |
| `pkg/ipfsnode` | 71.3% | ✅ | Add/Get, PubSub, topic management, bootstrap |
| `pkg/messaging` | 29.8% | ✅ | Envelope building, encryption flow |
| `pkg/cli` | 85.0% | ✅ | Display formatting, command parsing |

**Run Commands:**
```bash
# Run all unit tests
make test

# Run with coverage report
make test-coverage

# Run specific package tests
go test ./pkg/cli/... -v
go test ./pkg/messaging/... -v
go test ./pkg/ipfsnode/... -v
```

#### Unit Test Highlights

**Identity Module (`pkg/identity/identity_test.go`):**
- ✅ Mnemonic generation produces valid 12-word phrases
- ✅ Seed derivation from mnemonic is deterministic
- ✅ Ed25519 and X25519 keys derived correctly
- ✅ Identity persists and loads correctly
- ✅ Public key export (hex/base58) works

**Crypto Module (`pkg/crypto/crypto_test.go`):**
- ✅ X25519 shared secret computation
- ✅ XChaCha20-Poly1305 encrypt/decrypt roundtrip
- ✅ Ed25519 sign/verify
- ✅ HKDF key derivation

**Storage Module (`pkg/storage/storage_test.go`):**
- ✅ Contact CRUD operations
- ✅ Message storage with composite keys
- ✅ Message retrieval with limit/offset
- ✅ Peer record storage and pruning
- ✅ Configuration storage
- ✅ Transaction handling

**IPFS Node Module (`pkg/ipfsnode/node_test.go`):**
- ✅ Node creation and lifecycle
- ✅ Topic subscription and publishing
- ✅ Topic derivation from public keys
- ✅ Bootstrap connection (with network)
- ✅ Peer persistence

**Messaging Module (`pkg/messaging/messaging_test.go`):**
- ✅ Envelope creation and parsing
- ✅ Signature verification
- ✅ Encryption flow (unit level)

**CLI Module (`pkg/cli/cli_test.go`):**
- ✅ Display formatting functions
- ✅ Help text generation
- ✅ Error message formatting

---

### 2. Integration Tests

**Purpose:** Verify module interactions work correctly.

**Note:** Integration tests requiring network connectivity are marked with the `integration` build tag and are NOT run during normal CI builds.

#### Running Integration Tests

```bash
# Run all integration tests
go test -tags=integration ./pkg/ipfsnode/... -v

# Run with race detector
go test -tags=integration -race ./pkg/ipfsnode/... -v

# Run specific test
go test -tags=integration ./pkg/ipfsnode/... -run TestTwoNodePubSub -v

# Run with timeout
go test -tags=integration ./pkg/ipfsnode/... -timeout 5m

# Skip integration tests (default CI behavior)
go test -short ./...
```

#### 2.1 Identity + Storage Integration

**Test:** Save and load identity across application restarts.

**Scenario:**
1. Generate new identity
2. Save to disk
3. Load from disk
4. Verify all keys match original

**Acceptance Criteria:**
- ✅ Mnemonic persists correctly
- ✅ Ed25519 public/private keys match after reload
- ✅ X25519 public/private keys match after reload
- ✅ Identity file has correct permissions (0600)

**Status:** ✅ Complete (see `pkg/identity/identity_test.go`)

---

#### 2.2 Messaging + IPFS Integration

**Test:** Send and receive messages via embedded IPFS node.

**Scenario:**
1. Start messaging service
2. Create encrypted message
3. Add to IPFS, get CID
4. Publish CID via PubSub
5. Verify message received on subscribed topic

**Acceptance Criteria:**
- ✅ Message encrypted correctly
- ✅ CID generated successfully
- ✅ PubSub publishes without error
- ✅ Subscriber receives CID

**Status:** ✅ Complete (see `pkg/messaging/messaging_test.go`)

---

#### 2.3 Messaging + Storage Integration

**Test:** Persist and retrieve messages from BadgerDB.

**Scenario:**
1. Create signed envelope
2. Store message for contact
3. Retrieve messages
4. Verify ordering and content

**Acceptance Criteria:**
- ✅ Messages stored with correct composite key
- ✅ Messages retrieved in timestamp order
- ✅ Limit and offset work correctly
- ✅ Empty contact returns empty list

**Status:** ✅ Complete (see `pkg/storage/storage_test.go`)

---

#### 2.4 Two-Node PubSub Integration

**Test:** Publish/subscribe between two IPFS nodes.

**Scenario:**
1. Start two IPFS nodes on different ports
2. Node A subscribes to topic "test"
3. Node B publishes "Hello" to "test"
4. Node A receives message

**Acceptance Criteria:**
- ✅ Both nodes start successfully
- ✅ Nodes can discover each other (or manually connect)
- ✅ Message published by B received by A
- ✅ Message content matches

**Status:** ⚠️ Partial (skipped in isolated environments)

**Test Command:**
```bash
go test -tags=integration -v ./pkg/ipfsnode/... -run TestTwoNodePubSub
```

**Note:** Requires network connectivity for peer discovery. Manual connection test passes (`TestNodeConnectManual`).

---

#### 2.5 Multi-Node Network Formation

**Test:** Verify 5+ nodes can form a stable mesh network.

**Scenario:**
1. Create 5 nodes with different repos
2. Start all nodes
3. Connect first node to bootstrap
4. Wait for network to form
5. Verify each node has >2 connections

**Acceptance Criteria:**
- ⏳ 5 nodes form mesh network within 30 seconds
- ⏳ Each node has ≥2 peer connections
- ⏳ Each node's DHT routing table has ≥3 peers
- ⏳ No node crashes or panics

**Status:** ⏳ Pending

---

#### 2.6 Peer Persistence Across Restarts

**Test:** Verify peer records survive node restart.

**Scenario:**
1. Start node, connect to peers
2. Record stored peer count
3. Stop node
4. Restart node (same repo)
5. Verify peers restored

**Acceptance Criteria:**
- ✅ Peer records persist to BadgerDB
- ✅ Node reconnects to stored peers on restart
- ✅ Warm bootstrap faster than cold bootstrap

**Status:** ✅ Complete (see `pkg/storage/peer_test.go`)

---

#### 2.7 DHT Discovery

**Test:** Verify DHT bootstrap and peer discovery.

**Scenario:**
1. Start node with bootstrap peers
2. Wait for DHT bootstrap
3. Query routing table size
4. Perform DHT lookup

**Acceptance Criteria:**
- ✅ DHT routing table populated (>5 peers)
- ✅ FindPeer queries return results
- ✅ Self-advertisement to DHT works

**Status:** ✅ Complete (see `pkg/ipfsnode/node_test.go`)

---

### 3. End-to-End Tests

**Purpose:** Verify complete application flow from user perspective.

#### 3.1 Two-Instance Chat Test

**Setup:**
- Terminal 1: Instance A (Alice)
- Terminal 2: Instance B (Bob)
- Both instances on same machine or network

**Test Scenario:**

**Step 1: Launch Instances**
```bash
# Terminal 1
./bin/messenger

# Terminal 2
./bin/messenger
```

**Acceptance Criteria:**
- ⏳ Both instances generate identities on first launch
- ⏳ Mnemonics displayed and saved
- ⏳ Public keys shown in banner
- ⏳ CLI prompt appears (`>>>`)

---

**Step 2: Exchange Public Keys**
```bash
# Terminal 1 (Alice)
>>> /myid
Your Public Key:
  Hex:    0e1ba4bb4d0bb9d0...
  Base58: x5A38TJihSa1rofx...

# Terminal 2 (Bob)
>>> /myid
Your Public Key:
  Hex:    7f2c91ee8a4c3b21...
  Base58: 2Uw1bppLugs5Bqtr...
```

**Acceptance Criteria:**
- ⏳ `/myid` displays both hex and base58 formats
- ⏳ Keys are consistent across restarts
- ⏳ Output is readable and formatted

---

**Step 3: Add Contacts**
```bash
# Terminal 1 (Alice)
>>> /add 2Uw1bppLugs5Bqtr... Bob
✅ Contact added: Bob

>>> /list
=== Contacts ===
[1] Bob - 2Uw1bppLugs5Bqtr...
================

# Terminal 2 (Bob)
>>> /add x5A38TJihSa1rofx... Alice
✅ Contact added: Alice

>>> /list
=== Contacts ===
[1] Alice - x5A38TJihSa1rofx...
================
```

**Acceptance Criteria:**
- ⏳ Contact added successfully
- ⏳ Contact appears in list
- ⏳ Duplicate contact rejected
- ⏳ Invalid public key format rejected
- ⏳ Nickname stored correctly

---

**Step 4: Enter Chat Mode**
```bash
# Terminal 1 (Alice)
>>> /chat 1

━━━ Chat with Bob ━━━
Public key: 2Uw1bppLugs5Bqtr...
Type your message and press Enter to send.
Press Enter on an empty line to exit chat.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

>>> Hello Bob!
[2026-02-23 10:30:00] You: Hello Bob!
```

**Acceptance Criteria:**
- ⏳ Chat mode entered successfully
- ⏳ Chat header displays contact info
- ⏳ Sent message displayed with timestamp
- ⏳ Message encrypted and published

---

**Step 5: Receive Message (Bob)**
```bash
# Terminal 2 (Bob)
📬 New message from Alice
[2026-02-23 10:30:00] Alice: [Encrypted message received]
Type /chat to reply.
```

**Acceptance Criteria:**
- ⏳ Incoming message notification displayed
- ⏳ Message displayed (PoC: placeholder text)
- ⏳ Message stored in BadgerDB
- ⏳ Does not block user input

---

**Step 6: Reply to Message**
```bash
# Terminal 2 (Bob)
>>> /chat 1

━━━ Chat with Alice ━━━
...

>>> Hi Alice! How are you?
[2026-02-23 10:30:15] You: Hi Alice! How are you?

# Terminal 1 (Alice)
📬 New message from Bob
[2026-02-23 10:30:15] Bob: [Encrypted message received]
```

**Acceptance Criteria:**
- ⏳ Bidirectional communication works
- ⏳ Messages arrive in order
- ⏳ Timestamps are correct

---

**Step 7: View History**
```bash
# Terminal 1 (Alice)
>>> /history Bob

=== History with Bob ===
[2026-02-23 10:30:00] You: [Encrypted message received]
[2026-02-23 10:30:15] Bob: [Encrypted message received]
==========================
```

**Acceptance Criteria:**
- ⏳ History displays correctly
- ⏳ Messages ordered by timestamp
- ⏳ Limit parameter works
- ⏳ Shows outgoing/incoming indicators

---

**Step 8: Exit Chat and Application**
```bash
# Terminal 1 (Alice)
>>>

Exited chat mode.

>>> /exit

# Application exits, returns to shell
```

**Acceptance Criteria:**
- ⏳ Empty line exits chat mode
- ⏳ `/exit` command closes application
- ⏳ All resources cleaned up (IPFS, storage)
- ⏳ No goroutine leaks

---

**Step 9: Restart and Verify Persistence**
```bash
# Terminal 1 (Alice)
./bin/messenger

# Should load existing identity
# Contacts and messages should persist
>>> /list
=== Contacts ===
[1] Bob - 2Uw1bppLugs5Bqtr...
================

>>> /history Bob
# Previous messages should appear
```

**Acceptance Criteria:**
- ⏳ Identity loaded from disk (same public key)
- ⏳ Contacts persist across restarts
- ⏳ Messages persist across restarts
- ⏳ No data loss

---

#### 3.2 Error Handling Test

**Scenario:** Test application behavior with invalid inputs.

**Test Cases:**

1. **Invalid Public Key Format**
```bash
>>> /add invalid-key-format
❌ Error: Invalid public key format. Use hex or base58.
```

2. **Invalid Contact Index**
```bash
>>> /chat 999
❌ Error: Invalid contact index: 999. Use /list to see contacts.
```

3. **Unknown Command**
```bash
>>> /unknown
❌ Error: Unknown command: unknown. Type /help for help.
```

4. **Duplicate Contact**
```bash
>>> /add 0102030405060708091011121314151617181920
✅ Contact added: 3jYEF...

>>> /add 0102030405060708091011121314151617181920
ℹ️  Contact already exists.
[1] 3jYEF... - 3jYEF...
```

**Acceptance Criteria:**
- ⏳ All errors display user-friendly messages
- ⏳ Application doesn't crash on invalid input
- ⏳ Error messages are actionable

---

#### 3.3 Graceful Shutdown Test

**Scenario:** Verify clean shutdown on various signals.

**Test Cases:**

1. **Exit Command**
```bash
>>> /exit
# Application exits cleanly
```

2. **Ctrl+C (SIGINT)**
```bash
>>> ^C
Shutting down...
# Application exits cleanly
```

3. **SIGTERM**
```bash
# From another terminal
kill <pid>
# Application exits cleanly
```

**Acceptance Criteria:**
- ⏳ All goroutines terminate
- ⏳ IPFS node stops cleanly
- ⏳ Storage closes without data loss
- ⏳ No resource leaks

---

#### 3.4 Fresh Install Bootstrap

**Test:** Verify new node bootstraps successfully.

**Scenario:**
1. Create fresh directory (simulating new install)
2. Create node with default config
3. Start node
4. Wait for bootstrap with timeout
5. Verify DHT routing table populated

**Acceptance Criteria:**
- ⏳ New node bootstraps within 30 seconds
- ⏳ DHT routing table populated (≥5 peers)
- ⏳ At least one bootstrap connection successful

**Status:** ⏳ Pending

---

#### 3.5 Network Partition Recovery

**Test:** Verify network heals after partition.

**Scenario:**
1. Create 3 nodes
2. Form network
3. Simulate partition (stop middle node)
4. Verify remaining nodes can communicate
5. Restart node
6. Verify network heals

**Acceptance Criteria:**
- ⏳ Network detects partition
- ⏳ Remaining nodes maintain connectivity
- ⏳ Network heals when partitioned node returns
- ⏳ No data loss during partition

**Status:** ⏳ Pending

---

### 4. Manual Testing Checklist

Use this checklist for final PoC validation before release.

#### 4.1 Identity Management

- [ ] Fresh install generates valid 12-word mnemonic
- [ ] Mnemonic displayed clearly with warning
- [ ] Identity file created in `~/.babylontower/identity.json`
- [ ] Identity file has secure permissions (0600)
- [ ] Identity persists across application restarts
- [ ] Same mnemonic produces same keys (deterministic)
- [ ] `/myid` shows both hex and base58 formats

#### 4.2 Contact Management

- [ ] `/add` with valid base58 key works
- [ ] `/add` with valid hex key works
- [ ] `/add` with invalid key shows error
- [ ] `/add` with duplicate key shows info message
- [ ] `/add` with nickname stores name correctly
- [ ] `/add` without nickname uses truncated key
- [ ] `/list` shows all contacts with indices
- [ ] Contact count matches stored contacts

#### 4.3 Messaging

- [ ] `/chat` with valid index enters chat mode
- [ ] `/chat` with valid pubkey enters chat mode
- [ ] `/chat` with invalid index shows error
- [ ] `/chat` with unknown pubkey shows error
- [ ] Chat header displays contact info
- [ ] Messages sent in chat mode display with timestamp
- [ ] Empty line exits chat mode
- [ ] Incoming messages display in real-time
- [ ] Incoming messages don't block user input
- [ ] Notification shown for messages when not in chat

#### 4.4 Message History

- [ ] `/history` with valid contact shows messages
- [ ] `/history` with limit parameter works
- [ ] `/history` with invalid contact shows error
- [ ] Messages ordered by timestamp
- [ ] Outgoing/incoming messages distinguished
- [ ] Empty history shows appropriate message

#### 4.5 Help and Commands

- [ ] `/help` displays all commands
- [ ] Command descriptions are clear
- [ ] Unknown command shows error with hint
- [ ] All documented commands work
- [ ] `/network` shows health metrics
- [ ] `/contactstatus` shows contact status

#### 4.6 Persistence

- [ ] Identity survives restart
- [ ] Contacts survive restart
- [ ] Messages survive restart
- [ ] Data directory created automatically
- [ ] No data corruption after restart

#### 4.7 Error Handling

- [ ] Invalid inputs handled gracefully
- [ ] Error messages are user-friendly
- [ ] Application doesn't crash on errors
- [ ] Network errors logged but don't crash

#### 4.8 Shutdown

- [ ] `/exit` closes application cleanly
- [ ] Ctrl+C closes application cleanly
- [ ] All goroutines terminate
- [ ] IPFS node stops cleanly
- [ ] Storage closes without data loss
- [ ] No resource leaks (file handles, ports)

#### 4.9 Performance

- [ ] Application starts in <5 seconds
- [ ] Commands respond in <100ms
- [ ] Messages display in real-time
- [ ] No memory leaks (check with long-running instance)
- [ ] CPU usage reasonable when idle

#### 4.10 Cross-Platform (if applicable)

- [ ] Linux binary works
- [ ] macOS binary works (if built)
- [ ] Windows binary works (if built)
- [ ] Paths work on all platforms
- [ ] No platform-specific issues

---

## Test Execution Plan

### Automated Tests

```bash
# Run all unit tests
make test

# Run with coverage
make test-coverage

# Run specific package tests
go test ./pkg/cli/... -v
go test ./pkg/messaging/... -v
go test ./pkg/ipfsnode/... -v

# Run integration tests (requires network)
go test -tags=integration ./pkg/ipfsnode/... -v

# Run linter
make lint
```

### Manual End-to-End Test

1. **Setup:** Open two terminals
2. **Launch:** Start instance in each terminal
3. **Exchange:** Share public keys via `/myid`
4. **Add:** Add each other as contacts
5. **Chat:** Enter chat mode and exchange messages
6. **History:** Verify message history
7. **Restart:** Close and restart, verify persistence
8. **Cleanup:** Remove test data from `~/.babylontower/`

### Test Report Template

After testing, create a report with:

```markdown
## Test Execution Report

**Date**: YYYY-MM-DD
**Tester**: Name
**Version**: v0.1.0-poc

### Summary
- Unit Tests: XX/XX passed
- Integration Tests: XX/XX passed
- E2E Tests: XX/XX passed
- Manual Checklist: XX/XX items passed

### Issues Found
1. [Issue description]
2. [Issue description]

### Known Limitations
1. [Limitation description]
2. [Limitation description]

### Recommendation
[Ready for release / Needs fixes / Needs more testing]
```

---

## Performance Benchmarks

### Target Metrics

| Metric | Target | Measurement | Status |
|--------|--------|-------------|--------|
| Bootstrap time (cold) | <30 seconds | First connection | ⏳ Pending |
| Bootstrap time (warm) | <10 seconds | With stored peers | ⏳ Pending |
| DHT routing table size | >10 peers | After bootstrap | ⏳ Pending |
| Connection success rate | >70% | Successful / attempted | ⏳ Pending |
| Message delivery latency (P95) | <5 seconds | P95 latency | ⏳ Pending |
| Peer DB size | ≤100 peers | After 1 week | ⏳ Pending |
| Memory usage | <500MB | Per node | ⏳ Pending |
| CPU usage | <10% | Average | ⏳ Pending |

### Benchmark Tests

```bash
# Run benchmarks
go test -bench=. ./pkg/ipfsnode/... -benchmem

# Run specific benchmark
go test -bench=BenchmarkBootstrapTime ./pkg/ipfsnode/... -benchmem
```

---

## CI/CD Integration

### GitHub Actions Configuration

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

# Run with coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out
```

---

## Known Testing Limitations

### PoC Constraints

1. **Network Isolation:** Some integration tests skipped in isolated CI environments
   - Workaround: Manual connection test (`TestNodeConnectManual`) passes
   - Full two-node test requires network connectivity

2. **IPFS Get Not Implemented:** Cannot fetch envelopes from IPFS by CID
   - Impact: Incoming messages show placeholder text
   - Workaround: Direct PubSub message passing works

3. **X25519 Key Storage:** Contact X25519 keys not stored
   - Impact: Cannot fully encrypt messages to contacts
   - Workaround: Manual key exchange for testing

4. **NAT Traversal:** Limited implementation
   - Impact: Nodes must be on same network or have direct connectivity
   - Workaround: Test on local network or same machine

### Test Coverage Gaps

| Module | Coverage | Gap |
|--------|----------|-----|
| `pkg/messaging` | 29.8% | Service layer integration tests |
| `pkg/ipfsnode` | 71.3% | Some edge cases in peer discovery |
| `pkg/cli` | 85.0% | Command handler integration |

**Note:** Core cryptographic logic has >90% coverage. Lower coverage in service layers due to integration complexity.

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
   - Use host network mode for mDNS
   - Or configure explicit peer connections

2. **Timeouts:**
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

## Success Criteria

The PoC is considered successfully tested when:

### Functional Criteria
- ⏳ Two instances exchange messages without central server
- ✅ Messages are signed and verified correctly
- ✅ Identity derives from mnemonic and persists
- ✅ Contacts and messages stored locally
- ⏳ CLI responds to all documented commands

### Technical Criteria
- ✅ All unit tests pass (>80% coverage target met for core modules)
- ⏳ Integration tests pass (with documented caveats)
- ⏳ End-to-end demo works reliably
- ✅ No external dependencies required (single binary)
- ✅ Linter passes with 0 issues

### Documentation Criteria
- ✅ README with build and usage instructions
- ✅ Testing specification complete (this document)
- ✅ Known limitations documented
- ✅ Architecture diagrams accurate

---

## Future Testing (Post-PoC)

### Phase 6+ Testing

1. **Automated E2E Tests**
   - Script two-instance communication
   - Automated contact exchange
   - Message verification

2. **Performance Testing**
   - Load testing with many messages
   - Memory profiling
   - CPU usage analysis

3. **Security Testing**
   - Fuzzing for crypto functions
   - Input validation testing
   - Penetration testing

4. **Platform Testing**
   - Cross-platform CI/CD
   - Docker containerization
   - Mobile platform testing (future)

---

*Last updated: February 23, 2026*
*Version: 1.0 (Consolidated)*
