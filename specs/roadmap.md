# Babylon Tower - Implementation Roadmap

## Project Overview

**Babylon Tower** is a decentralized, secure peer-to-peer messenger that operates without central servers. This roadmap details the implementation plan and tracks progress through all phases.

**Current Status:** Phase 10 Complete - Protocol v1 Wire Format implemented with BabylonEnvelope, all message types, and topic routing. Ready for Multi-Device implementation.

---

## Versioning Strategy

### Current PoC (Unversioned)

The existing proof-of-concept implementation remains **unversioned** — it was never formally specified or shipped. It serves as a learning implementation and testbed.

**Characteristics:**
- Static X25519 ECDH (no forward secrecy)
- Single-device only
- No group messaging
- No offline delivery
- Basic PubSub routing

### Protocol v1 (Target)

The protocol specified in `protocol-v2.md` is now designated as **Protocol v1.0** — the first official versioned protocol. This is the production target.

**Characteristics:**
- X3DH + Double Ratchet (forward secrecy + post-compromise security)
- Multi-device support
- Private/public groups and channels
- Offline delivery (mailbox)
- Voice/video calls
- Reputation system

---

## Decision: Skip Remaining PoC Phases

Phases 6 and 7 (PoC Integration & Testing, Release Preparation) are **cancelled**. The PoC is functional enough to validate the architecture but will not be released as a standalone version. All development effort shifts directly to Protocol v1 implementation.

---

## System Architecture

### v1 (PoC)

```
┌─────────────────┐       ┌──────────────────┐
│      CLI        │<─────>│    Messaging     │
│  (user input)   │       │  (business logic)│
└─────────────────┘       └────────┬─────────┘
                                   │
                          ┌────────▼────────┐
                          │   IPFS Node     │
                          │ (embedded +     │
                          │   PubSub)       │
                          └────────┬─────────┘
                                   │
                          ┌────────▼────────┐
                          │   Storage       │
                          │  (BadgerDB)     │
                          └─────────────────┘
```

### v1 (Target - Formerly v2)

```
┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐
│   CLI    │  │  Groups  │  │   RTC    │  │  Mailbox │
│  (REPL)  │  │ Channels │  │Voice/Vid │  │  Relay   │
└────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘
     │             │              │              │
┌────▼─────────────▼──────────────▼──────────────▼─────┐
│                   Messaging Service                    │
│  (X3DH · Double Ratchet · Sender Keys · Multi-Device) │
└──────────────────────────┬───────────────────────────┘
                           │
┌──────────────────────────▼───────────────────────────┐
│                    Identity v1                         │
│  (Master Key · Device Keys · IdentityDocument · DHT)  │
└──────────────────────────┬───────────────────────────┘
                           │
              ┌────────────▼────────────┐
              │      libp2p Node        │
              │  GossipSub · DHT · Relay│
              └────────────┬────────────┘
                           │
              ┌────────────▼────────────┐
              │     Storage (BadgerDB)   │
              │  Sessions · Groups · Rep │
              └─────────────────────────┘
```

---

## Project Structure

```
babylontower/
├── cmd/
│   └── messenger/
│       └── main.go              # Application entry point
├── pkg/
│   ├── identity/
│   │   ├── identity.go          # Master seed generation, key derivation
│   │   └── keys.go              # Ed25519, X25519 key operations
│   ├── crypto/
│   │   ├── crypto.go            # Encrypt/decrypt (XChaCha20-Poly1305)
│   │   └── sign.go              # Sign/verify (Ed25519)
│   ├── storage/
│   │   ├── badger.go            # BadgerDB implementation
│   │   ├── contacts.go          # Contact CRUD
│   │   └── messages.go          # Message CRUD
│   ├── ipfsnode/
│   │   ├── node.go              # Embedded IPFS node wrapper
│   │   ├── pubsub.go            # PubSub subscribe/publish
│   │   ├── metrics.go           # Network health metrics
│   │   └── config.go            # IPFS configuration
│   ├── messaging/
│   │   ├── protocol.go          # Core protocol logic
│   │   ├── envelope.go          # Envelope building/parsing
│   │   ├── outgoing.go          # Send message flow
│   │   └── incoming.go          # Receive message flow
│   ├── cli/
│   │   ├── cli.go               # REPL implementation
│   │   ├── commands.go          # Command handlers
│   │   └── display.go           # UI formatting
│   └── proto/
│       └── message.pb.go        # Generated protobuf code
├── proto/
│   └── message.proto            # Protobuf definitions
├── configs/
│   └── config.go                # Configuration management
├── internal/
│   └── testutil/
│       └── helpers.go           # Test utilities
├── go.mod
├── go.sum
├── Makefile
├── README.md
└── specs/
    ├── poc.md                   # Technical specification
    ├── roadmap.md               # This document
    └── testing.md               # Testing strategy and plans
```

---

## Implementation Phases Summary

### v1 (PoC - Unversioned)

| Phase | Status | Description |
|-------|--------|-------------|
| 0 | ✅ Complete | Project Setup |
| 1 | ✅ Complete | Identity & Cryptography |
| 2 | ✅ Complete | Storage Layer |
| 3 | ✅ Complete | IPFS Node Integration |
| 4 | ✅ Complete | Messaging Protocol |
| 5 | ✅ Complete | CLI Interface |
| 6 | ❌ Cancelled | Integration & Testing (skipped) |
| 7 | ❌ Cancelled | Release Preparation (skipped) |

**Note:** The PoC is functional but will not be released. Development has shifted to Protocol v1.

### v1 (Protocol - Target)

| Phase | Status | Description | Dependencies |
|-------|--------|-------------|-------------|
| 8 | ✅ Complete | Identity v1 (devices, identity docs, prekeys) | — |
| 9 | ✅ Complete | X3DH & Double Ratchet | Phase 8 |
| 10 | ✅ Complete | Protocol v1 Wire Format | Phase 9 |
| 11 | ⏹️ Pending | Multi-Device | Phase 10 |
| 12 | ⏹️ Pending | Private Groups (Sender Keys) | Phase 10 |
| 13 | ⏹️ Pending | Public Groups & Channels | Phase 12 |
| 14 | ⏹️ Pending | Offline Delivery (Mailbox) | Phase 10 |
| 15 | ⏹️ Pending | Voice & Video Calls | Phase 10 |
| 16 | ⏹️ Pending | Group Calls (Mesh + SFU) | Phase 12, 15 |
| 17 | ⏹️ Pending | Reputation System | Phase 14 |
| 18 | ⏹️ Pending | Integration & Hardening | All above |

---

## Phase 0: Project Setup ✅

**Goal:** Establish project foundation and tooling.

### Completed Tasks

| Task | Description | Status |
|------|-------------|--------|
| 0.1 | Initialize Go module (`go mod init babylontower`) | ✅ |
| 0.2 | Create directory structure | ✅ |
| 0.3 | Add dependencies to `go.mod` | ✅ |
| 0.4 | Create Makefile with common targets | ✅ |
| 0.5 | Set up CI configuration (GitHub Actions) | ✅ |
| 0.6 | Create initial README.md | ✅ |

**Deliverables:**
- ✅ Working Go module with all dependencies
- ✅ Makefile with build/test/lint targets
- ✅ CI pipeline passing
- ✅ Basic README

---

## Phase 1: Identity & Cryptography ✅

**Goal:** Implement secure identity management and cryptographic operations.

### 1.1 Identity Module (`pkg/identity`)

| Task | Description | Status |
|------|-------------|--------|
| 1.1.1 | BIP39 mnemonic generation | ✅ |
| 1.1.2 | Seed derivation from mnemonic | ✅ |
| 1.1.3 | Ed25519 key pair derivation | ✅ |
| 1.1.4 | X25519 key pair derivation | ✅ |
| 1.1.5 | Identity persistence (load/save) | ✅ |
| 1.1.6 | Public key export (hex/base58) | ✅ |

### 1.2 Crypto Module (`pkg/crypto`)

| Task | Description | Status |
|------|-------------|--------|
| 1.2.1 | X25519 shared secret computation | ✅ |
| 1.2.2 | XChaCha20-Poly1305 encryption | ✅ |
| 1.2.3 | XChaCha20-Poly1305 decryption | ✅ |
| 1.2.4 | Ed25519 signing | ✅ |
| 1.2.5 | Ed25519 verification | ✅ |
| 1.2.6 | HKDF key derivation | ✅ |

**Deliverables:**
- ✅ Identity generation and persistence working
- ✅ All crypto functions implemented and tested
- ✅ Unit tests with >80% coverage (achieved: 86.1% identity, 95.2% crypto)
- ✅ No hardcoded secrets in tests

---

## Phase 2: Storage Layer ✅

**Goal:** Implement embedded key-value storage with BadgerDB.

### 2.1 Protobuf Definitions (`proto/`)

| Task | Description | Status |
|------|-------------|--------|
| 2.1.1 | Create `message.proto` | ✅ |
| 2.1.2 | Generate Go code | ✅ |
| 2.1.3 | Add Makefile proto target | ✅ |

### 2.2 Storage Implementation (`pkg/storage`)

| Task | Description | Status |
|------|-------------|--------|
| 2.2.1 | BadgerDB initialization | ✅ |
| 2.2.2 | Contact storage interface | ✅ |
| 2.2.3 | Message storage interface | ✅ |
| 2.2.4 | Composite key for messages | ✅ |
| 2.2.5 | Transaction handling | ✅ |
| 2.2.6 | Graceful shutdown | ✅ |
| 2.2.7 | Peer storage (extended) | ✅ |
| 2.2.8 | Configuration storage | ✅ |

**Unified BadgerDB Schema:**
```
Key Prefixes:
├── "c:" + <pubkey>          → Contact records (protobuf)
├── "m:" + <pubkey> + <ts> + <nonce>  → Message envelopes (protobuf)
├── "p:" + <peer_id>         → Peer records (JSON)
└── "cfg:" + <config_key>    → Configuration values (JSON)
```

**Deliverables:**
- ✅ Protobuf definitions compiled
- ✅ Storage interface fully implemented
- ✅ Unit tests with in-memory or temp DB
- ✅ CRUD operations verified
- ✅ Test coverage: 87.9%

---

## Phase 3: IPFS Node Integration ✅

**Goal:** Embed IPFS node with PubSub functionality.

### 3.1 Core Node (`pkg/ipfsnode`)

| Task | Description | Status |
|------|-------------|--------|
| 3.1.1 | IPFS node initialization | ✅ |
| 3.1.2 | Repository configuration | ✅ |
| 3.1.3 | Graceful shutdown | ✅ |
| 3.1.4 | Add data to IPFS | ✅ |
| 3.1.5 | Get data from IPFS | ✅ (PoC limitation) |
| 3.1.6 | Error handling | ✅ |

### 3.2 PubSub (`pkg/ipfsnode/pubsub.go`)

| Task | Description | Status |
|------|-------------|--------|
| 3.2.1 | Topic derivation | ✅ |
| 3.2.2 | Subscribe to topic | ✅ |
| 3.2.3 | Publish to topic | ✅ |
| 3.2.4 | Message channel handling | ✅ |
| 3.2.5 | Subscription lifecycle | ✅ |

### 3.3 Configuration & Bootstrap (`pkg/ipfsnode/config.go`)

| Task | Description | Status |
|------|-------------|--------|
| 3.3.1 | Configurable bootstrap peers | ✅ |
| 3.3.2 | Multi-stage bootstrap strategy | ✅ |
| 3.3.3 | DNS address resolution | ✅ |
| 3.3.4 | Connection manager setup | ✅ |
| 3.3.5 | NAT traversal (AutoNAT + hole punching) | ✅ |

### 3.4 Metrics & Monitoring (`pkg/ipfsnode/metrics.go`)

| Task | Description | Status |
|------|-------------|--------|
| 3.4.1 | Connection metrics tracking | ✅ |
| 3.4.2 | Discovery source tracking | ✅ |
| 3.4.3 | Bootstrap timing metrics | ✅ |
| 3.4.4 | Network health API | ✅ |

### 3.5 Integration Testing

| Task | Description | Status |
|------|-------------|--------|
| 3.5.1 | Two-node test setup | ✅ |
| 3.5.2 | Add/Get test | ✅ |
| 3.5.3 | PubSub test | ✅ (requires network) |
| 3.5.4 | Connection test | ✅ (manual works) |

**Deliverables:**
- ✅ Embedded IPFS node working
- ✅ PubSub subscribe/publish functional
- ✅ Multi-stage bootstrap with fallback
- ✅ Peer persistence across restarts
- ✅ Network health metrics available
- ✅ Test coverage: 71.3%

---

## Phase 4: Messaging Protocol ✅

**Goal:** Implement end-to-end encrypted messaging protocol.

### 4.1 Protocol Core (`pkg/messaging`)

| Task | Description | Status |
|------|-------------|--------|
| 4.1.1 | Message protobuf builder | ✅ |
| 4.1.2 | Envelope creation | ✅ |
| 4.1.3 | Signed envelope creation | ✅ |
| 4.1.4 | Envelope parsing | ✅ |
| 4.1.5 | Signature verification | ✅ |

### 4.2 Outgoing Messages

| Task | Description | Status |
|------|-------------|--------|
| 4.2.1 | Full encryption flow | ✅ |
| 4.2.2 | IPFS add integration | ✅ |
| 4.2.3 | PubSub publish | ✅ |
| 4.2.4 | Local message storage | ✅ |
| 4.2.5 | Error handling | ✅ |

### 4.3 Incoming Messages

| Task | Description | Status |
|------|-------------|--------|
| 4.3.1 | PubSub message handler | ✅ |
| 4.3.2 | IPFS fetch | ✅ (PoC limitation) |
| 4.3.3 | Signature verification | ✅ |
| 4.3.4 | Decryption | ✅ |
| 4.3.5 | Message storage | ✅ |
| 4.3.6 | Callback/notification | ✅ |

### 4.4 Messaging Service Enhancements

| Task | Description | Status |
|------|-------------|--------|
| 4.4.1 | Service initialization | ✅ |
| 4.4.2 | Background goroutines | ✅ |
| 4.4.3 | Contact peer tracking | ✅ |
| 4.4.4 | Connection pooling | ✅ |
| 4.4.5 | Message retry logic | ✅ |
| 4.4.6 | PubSub mesh optimization | ✅ |
| 4.4.7 | Contact status API | ✅ |

**Deliverables:**
- ✅ Full message encryption/decryption working
- ✅ End-to-end message delivery verified
- ✅ Messages persisted and retrievable
- ✅ Contact-aware routing implemented
- ✅ Connection pooling for active contacts
- ✅ Message retry with exponential backoff
- ✅ Unit and integration tests passing
- ✅ Test coverage: 29.8% (core crypto tested)

---

## Phase 5: CLI Interface ✅

**Goal:** Build interactive command-line interface.

### 5.1 REPL Engine (`pkg/cli`)

| Task | Description | Status |
|------|-------------|--------|
| 5.1.1 | Read-eval-print loop | ✅ |
| 5.1.2 | Command parsing | ✅ |
| 5.1.3 | Concurrent input/events | ✅ |
| 5.1.4 | Graceful exit | ✅ |
| 5.1.5 | Signal handling (Ctrl+C) | ✅ |

### 5.2 Commands (`pkg/cli/commands.go`)

| Task | Description | Status |
|------|-------------|--------|
| 5.2.1 | `/help` | ✅ |
| 5.2.2 | `/myid` | ✅ |
| 5.2.3 | `/add <pubkey> [nickname]` | ✅ |
| 5.2.4 | `/list` | ✅ |
| 5.2.5 | `/chat <contact>` | ✅ |
| 5.2.6 | `/history <contact> [limit]` | ✅ |
| 5.2.7 | `/exit` | ✅ |
| 5.2.8 | `/network` (health metrics) | ✅ |
| 5.2.9 | `/contactstatus` | ✅ |

### 5.3 Chat Mode

| Task | Description | Status |
|------|-------------|--------|
| 5.3.1 | Message input loop | ✅ |
| 5.3.2 | Empty line exits chat | ✅ |
| 5.3.3 | Real-time message display | ✅ |
| 5.3.4 | Message formatting | ✅ |

### 5.4 Display (`pkg/cli/display.go`)

| Task | Description | Status |
|------|-------------|--------|
| 5.4.1 | Contact list formatting | ✅ |
| 5.4.2 | Message formatting | ✅ |
| 5.4.3 | Error display | ✅ |
| 5.4.4 | Help formatting | ✅ |

**Deliverables:**
- ✅ All commands implemented
- ✅ Chat mode with real-time updates
- ✅ Clean, readable UI
- ✅ Network health monitoring
- ✅ Contact status tracking
- ✅ Unit tests passing (12 tests, 85% coverage)

---

## Phase 6: Integration & Testing ❌

**Status:** **SKIPPED** - This phase was cancelled as part of the decision to skip remaining PoC phases and move directly to Protocol v1 implementation.

**Goal:** ~~End-to-end testing and validation.~~

### 6.1 Unit Tests (Completed)

| Module | Coverage | Status |
|--------|----------|--------|
| `pkg/identity` | 86.1% | ✅ |
| `pkg/crypto` | 95.2% | ✅ |
| `pkg/storage` | 87.9% | ✅ |
| `pkg/ipfsnode` | 71.3% | ✅ |
| `pkg/messaging` | 29.8% | ✅ |
| `pkg/cli` | 85.0% | ✅ |

### 6.2 Integration Tests (Skipped)

| Test | Description | Status |
|------|-------------|--------|
| 6.2.1 | Two-node bootstrap | ✅ |
| 6.2.2 | Multi-node network (5+ nodes) | ❌ Skipped |
| 6.2.3 | DHT discovery | ✅ |
| 6.2.4 | Peer persistence | ✅ |
| 6.2.5 | NAT traversal | ❌ Skipped |

### 6.3 End-to-End Tests (Skipped)

| Test | Description | Status |
|------|-------------|--------|
| 6.3.1 | Fresh install bootstrap | ❌ Skipped |
| 6.3.2 | Restart reconnection | ❌ Skipped |
| 6.3.3 | Contact messaging | ❌ Skipped |
| 6.3.4 | Network partition recovery | ❌ Skipped |
| 6.3.5 | Scale test (20+ nodes) | ❌ Skipped |

### 6.4 Performance Benchmarks (Skipped)

| Metric | Target | Status |
|--------|--------|--------|
| Bootstrap time (cold) | <30 seconds | ❌ Skipped |
| Bootstrap time (warm) | <10 seconds | ❌ Skipped |
| DHT routing table size | >10 peers | ❌ Skipped |
| Connection success rate | >70% | ❌ Skipped |
| Message delivery latency (P95) | <5 seconds | ❌ Skipped |
| Peer DB size | ≤100 peers | ❌ Skipped |

**Deliverables:**
- ❌ All end-to-end tests passing (skipped)
- ❌ Performance benchmarks documented (skipped)
- ❌ Test report completed (skipped)
- ❌ No critical bugs remaining (skipped)

---

## Phase 7: Release Preparation ❌

**Status:** **SKIPPED** - This phase was cancelled as part of the decision to skip remaining PoC phases and move directly to Protocol v1 implementation.

**Goal:** ~~Prepare PoC for release.~~

| Task | Description | Status |
|------|-------------|--------|
| 7.1 | Cross-compilation setup | ❌ Skipped |
| 7.2 | Binary testing (Linux, macOS, Windows) | ❌ Skipped |
| 7.3 | Final code review | ❌ Skipped |
| 7.4 | Tag release (v0.1.0-poc) | ❌ Skipped |
| 7.5 | Release notes | ❌ Skipped |

**Deliverables:**
- ❌ Compiled binaries for 3 platforms (skipped)
- ❌ Git tag v0.1.0-poc (skipped)
- ❌ Release notes published (skipped)

---

## Testing Strategy

### Test Categories

1. **Unit Tests**: Individual functions and methods
   - Coverage target: >80% for core modules
   - Run: `make test` or `make test-coverage`

2. **Integration Tests**: Module interactions
   - Multi-node PubSub tests
   - Peer discovery tests
   - Run: `go test -tags=integration ./...`

3. **End-to-End Tests**: Full application flow
   - Two-instance chat tests
   - Contact exchange and messaging
   - Manual testing required

See [`testing.md`](testing.md) for comprehensive testing documentation.

---

## Risk Register

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| go-ipfs embedding complexity | High | Medium | Use libp2p directly if needed |
| BadgerDB corruption | Medium | Low | Proper transaction handling; graceful shutdown |
| NAT traversal issues | High | High | Document as PoC limitation; AutoNAT + hole punching implemented |
| Memory usage with embedded IPFS | Medium | Medium | Tune IPFS config; document minimum requirements |
| Dependency conflicts | Medium | Medium | Pin versions; use Go modules strictly |
| Cryptographic implementation errors | High | Low | Use well-tested libraries; thorough testing |

---

## Success Criteria

The PoC is considered successful when:

### Functional Criteria
- ✅ Two instances exchange messages without central server
- ✅ Messages are signed and verified correctly
- ✅ Identity derives from mnemonic and persists
- ✅ Contacts and messages stored locally
- ✅ CLI responds to all documented commands

### Technical Criteria
- ✅ All unit tests pass (>80% coverage for core modules)
- ⏳ Integration tests pass (with documented caveats)
- ⏳ End-to-end demo works reliably
- ✅ No external dependencies required (single binary)
- ✅ Linter passes with 0 issues

### Documentation Criteria
- ✅ README with build and usage instructions
- ✅ Technical specification complete
- ✅ Known limitations documented
- ✅ Architecture diagrams accurate

---

## Key Dependencies

```go
// IPFS / libp2p
github.com/ipfs/go-cid              // Content Identifier handling
github.com/ipfs/go-log/v2           // Logging
github.com/libp2p/go-libp2p         // Core libp2p framework (v0.47.0)
github.com/libp2p/go-libp2p-kad-dht // Distributed Hash Table
github.com/libp2p/go-libp2p-pubsub  // PubSub messaging
github.com/multiformats/go-multiaddr // Multiaddr format
github.com/multiformats/go-multihash // Multihash functions

// Cryptography
github.com/tyler-smith/go-bip39        // BIP39 mnemonic
golang.org/x/crypto/curve25519         // X25519 key agreement
golang.org/x/crypto/chacha20poly1305   // XChaCha20-Poly1305
golang.org/x/crypto/hkdf               // Key derivation
crypto/ed25519                         // Ed25519 signatures (stdlib)

// Storage
github.com/dgraph-io/badger/v3         // Embedded key-value store

// Protobuf
google.golang.org/protobuf

// CLI
github.com/chzyer/readline             // REPL input
```

---

## Protocol v1 Specification

See [`protocol-v2.md`](protocol-v2.md) for the complete v1 protocol specification (document still named protocol-v2.md for historical reasons) covering:
- X3DH + Double Ratchet (forward secrecy + post-compromise security)
- Multi-device support with independent device keys
- Private/public groups and channels (Sender Keys)
- Offline delivery (mailbox protocol)
- Voice/video calls (1:1 and group)
- Reputation rewards system
- Protocol versioning and cipher agility

---

## v1 Implementation Phases

| Phase | Status | Name | Dependencies |
|-------|--------|------|-------------|
| 8 | ⏹️ Pending | Identity v1 | — |
| 9 | ⏹️ Pending | X3DH & Double Ratchet | Phase 8 |
| 10 | ⏹️ Pending | Protocol v1 Wire Format | Phase 9 |
| 11 | ⏹️ Pending | Multi-Device | Phase 10 |
| 12 | ⏹️ Pending | Private Groups | Phase 10 |
| 13 | ⏹️ Pending | Public Groups & Channels | Phase 12 |
| 14 | ⏹️ Pending | Offline Delivery | Phase 10 |
| 15 | ⏹️ Pending | Voice & Video Calls | Phase 10 |
| 16 | ⏹️ Pending | Group Calls | Phase 12, 15 |
| 17 | ⏹️ Pending | Reputation System | Phase 14 |
| 18 | ⏹️ Pending | Integration & Hardening | All above |

---

## Phase 8: Identity v1 ⏹️

**Goal:** Implement the v1 identity system with master key derivation, device keys, and DHT-published identity documents.

### 8.1 Master Key Derivation

| Task | Description | Status |
|------|-------------|--------|
| 8.1.1 | Master secret intermediate step (HKDF from seed) | ⏹️ |
| 8.1.2 | v1 identity key derivation (new HKDF labels) | ⏹️ |
| 8.1.3 | PoC backward-compatible derivation mode | ⏹️ |
| 8.1.4 | Identity fingerprint generation (Base58) | ⏹️ |

### 8.2 Device Keys

| Task | Description | Status |
|------|-------------|--------|
| 8.2.1 | Random device key generation (DK_sign + DK_dh) | ⏹️ |
| 8.2.2 | DeviceCertificate creation and signing | ⏹️ |
| 8.2.3 | Device ID derivation (SHA256(DK_sign.pub)[:16]) | ⏹️ |

### 8.3 Identity Document

| Task | Description | Status |
|------|-------------|--------|
| 8.3.1 | IdentityDocument protobuf definition | ⏹️ |
| 8.3.2 | Document creation, signing, and validation | ⏹️ |
| 8.3.3 | Hash chain (sequence + previous_hash) | ⏹️ |
| 8.3.4 | DHT publication and retrieval | ⏹️ |
| 8.3.5 | Custom DHT validator for /bt/id/ namespace | ⏹️ |
| 8.3.6 | FeatureFlags and capability advertising | ⏹️ |

### 8.4 Prekey Management

| Task | Description | Status |
|------|-------------|--------|
| 8.4.1 | Signed prekey generation and signing | ⏹️ |
| 8.4.2 | One-time prekey batch generation | ⏹️ |
| 8.4.3 | Prekey bundle standalone DHT publication | ⏹️ |
| 8.4.4 | OPK replenishment logic (threshold-based) | ⏹️ |
| 8.4.5 | SPK rotation (7-day cycle with 24h overlap) | ⏹️ |

**Deliverables:**
- ⏹️ v1 identity generation with device keys
- ⏹️ IdentityDocument published to and retrieved from DHT
- ⏹️ Prekey generation, rotation, and distribution
- ⏹️ Unit tests with >80% coverage

---

## Phase 9: X3DH & Double Ratchet ⏹️

**Goal:** Implement forward-secret session establishment and ongoing message encryption.

### 9.1 X3DH

| Task | Description | Status |
|------|-------------|--------|
| 9.1.1 | Prekey bundle fetch from DHT | ✅ |
| 9.1.2 | X3DH initiator (4-DH computation + SK derivation) | ✅ |
| 9.1.3 | X3DH responder (mirror DH + OPK consumption) | ✅ |
| 9.1.4 | OPK exhaustion fallback (3-DH) | ✅ |
| 9.1.5 | OPK race condition handling | ✅ |

### 9.2 Double Ratchet

| Task | Description | Status |
|------|-------------|--------|
| 9.2.1 | KDF_RK and KDF_CK functions | ✅ |
| 9.2.2 | Ratchet state initialization (initiator + responder) | ✅ |
| 9.2.3 | Sending: chain ratchet → encrypt → RatchetHeader | ✅ |
| 9.2.4 | Receiving: DH ratchet step + chain ratchet → decrypt | ✅ |
| 9.2.5 | Skipped message key caching (max 256) | ✅ |
| 9.2.6 | Session state persistence in BadgerDB (dr: prefix) | ✅ |

### 9.3 Cipher Agility

| Task | Description | Status |
|------|-------------|--------|
| 9.3.1 | Cipher suite registry (0x0001 mandatory) | ✅ |
| 9.3.2 | Suite negotiation from IdentityDocument | ✅ |

**Deliverables:**
- ✅ X3DH session establishment working
- ✅ Double Ratchet encryption/decryption working
- ✅ Session persistence across restarts
- ✅ Comprehensive crypto test vectors

---

## Phase 10: Protocol v1 Wire Format ✅

**Goal:** Implement BabylonEnvelope, all message types, and backward compatibility.

| Task | Description | Status |
|------|-------------|--------|
| 10.1 | BabylonEnvelope protobuf definition | ✅ |
| 10.2 | MessageType enum (DM, Group, Channel, Control, RTC) | ✅ |
| 10.3 | DMPayload with RatchetHeader | ✅ |
| 10.4 | All content types (text, media, reaction, edit, delete, receipts) | ✅ |
| 10.5 | Control message payloads (X3DHHeader, DeviceAnnouncement, etc.) | ✅ |
| 10.6 | v1 topic routing (babylon-dm-, babylon-grp-, etc.) | ✅ |
| 10.7 | PoC backward compatibility (dual topic subscription, legacy parsing) | ✅ |
| 10.8 | Version negotiation logic | ✅ |

**Deliverables:**
- ✅ All protobuf definitions updated
- ✅ v1 envelope construction/parsing
- ✅ Backward-compatible with PoC messages

---

## Phase 11: Multi-Device ⏹️

**Goal:** Support multiple devices per identity with independent keys and synchronized state.

| Task | Description | Status |
|------|-------------|--------|
| 11.1 | Device registration flow (mnemonic → device cert) | ⏹️ |
| 11.2 | Per-device Double Ratchet sessions | ⏹️ |
| 11.3 | Message fanout (encrypt per device) | ⏹️ |
| 11.4 | Multi-device optimization (shared symmetric key) | ⏹️ |
| 11.5 | Device sync channel (babylon-sync- topic) | ⏹️ |
| 11.6 | Sync message types (contact, read, group, settings) | ⏹️ |
| 11.7 | History sync (request/batch) | ⏹️ |
| 11.8 | Vector clock conflict resolution | ⏹️ |
| 11.9 | Device revocation | ⏹️ |

**Deliverables:**
- ⏹️ Multiple devices per identity working
- ⏹️ Cross-device message delivery
- ⏹️ State synchronization

---

## Phase 12: Private Groups ⏹️

**Goal:** Implement encrypted group messaging with Sender Keys.

| Task | Description | Status |
|------|-------------|--------|
| 12.1 | GroupState and GroupStateUpdate protobufs | ⏹️ |
| 12.2 | Group creation (generate group_id, initial state) | ⏹️ |
| 12.3 | Sender Key generation and distribution | ⏹️ |
| 12.4 | Group message encryption/decryption (Sender Keys) | ⏹️ |
| 12.5 | Member addition (epoch++, key distribution) | ⏹️ |
| 12.6 | Member removal (epoch++, full key rotation) | ⏹️ |
| 12.7 | Group state chain validation | ⏹️ |
| 12.8 | Split-brain resolution | ⏹️ |
| 12.9 | Group CLI commands (/creategroup, /invite, /groupchat) | ⏹️ |

**Deliverables:**
- ⏹️ Private group messaging working
- ⏹️ Member management with key rotation
- ⏹️ Group state chain integrity

---

## Phase 13: Public Groups & Channels ⏹️

**Goal:** Public groups with moderation and channels with publisher-subscriber model.

| Task | Description | Status |
|------|-------------|--------|
| 13.1 | Public group creation and DHT discovery | ⏹️ |
| 13.2 | Signed (unencrypted) public group messages | ⏹️ |
| 13.3 | Moderation actions (BAN, MUTE, DELETE) | ⏹️ |
| 13.4 | Anti-spam (rate limiting, optional proof-of-work) | ⏹️ |
| 13.5 | Private channels (encrypted, publisher only) | ⏹️ |
| 13.6 | Public channels (signed, open) | ⏹️ |
| 13.7 | Channel content persistence (IPFS linked-list) | ⏹️ |

---

## Phase 14: Offline Delivery ⏹️

**Goal:** Implement mailbox protocol for offline message delivery.

| Task | Description | Status |
|------|-------------|--------|
| 14.1 | MailboxAnnouncement DHT publication | ⏹️ |
| 14.2 | Mailbox libp2p stream handler (/bt/mailbox/1.0.0) | ⏹️ |
| 14.3 | Deposit flow (sender → mailbox) | ⏹️ |
| 14.4 | Retrieval flow (recipient ← mailbox, with auth) | ⏹️ |
| 14.5 | Storage policies and eviction | ⏹️ |
| 14.6 | Deduplication and ordering | ⏹️ |
| 14.7 | Anti-abuse (rate limiting, quotas) | ⏹️ |
| 14.8 | IPFS-based media persistence | ⏹️ |

---

## Phase 15: Voice & Video Calls ⏹️

**Goal:** 1:1 voice and video calls with E2E encrypted signaling and media.

| Task | Description | Status |
|------|-------------|--------|
| 15.1 | RTC signaling messages over Double Ratchet | ⏹️ |
| 15.2 | SDP offer/answer generation | ⏹️ |
| 15.3 | ICE candidate exchange | ⏹️ |
| 15.4 | libp2p media stream protocol (/bt/media/1.0.0) | ⏹️ |
| 15.5 | DTLS-SRTP with session key binding | ⏹️ |
| 15.6 | Codec negotiation (Opus, VP8/VP9) | ⏹️ |
| 15.7 | Call lifecycle (ring, accept, reject, hangup, timeout) | ⏹️ |

---

## Phase 16: Group Calls ⏹️

**Goal:** Group voice/video calls with mesh and SFU topologies.

| Task | Description | Status |
|------|-------------|--------|
| 16.1 | Mesh topology for 2-6 participants | ⏹️ |
| 16.2 | SFU election (lowest lexicographic pubkey) | ⏹️ |
| 16.3 | SFU selective forwarding (encrypted SRTP) | ⏹️ |
| 16.4 | SFU failover and re-election | ⏹️ |

---

## Phase 17: Reputation System ⏹️

**Goal:** Local reputation tracking with optional attestation exchange.

| Task | Description | Status |
|------|-------------|--------|
| 17.1 | Per-peer reputation metrics tracking | ⏹️ |
| 17.2 | Composite score computation (5 dimensions) | ⏹️ |
| 17.3 | Reputation tiers and benefit enforcement | ⏹️ |
| 17.4 | Optional reputation attestation exchange via DHT | ⏹️ |
| 17.5 | Anti-gaming measures (max influence, connection time requirements) | ⏹️ |

---

## Phase 18: Integration & Hardening ⏹️

**Goal:** Full integration testing, security audit, and performance optimization.

| Task | Description | Status |
|------|-------------|--------|
| 18.1 | End-to-end X3DH + Double Ratchet integration tests | ⏹️ |
| 18.2 | Multi-device message delivery tests | ⏹️ |
| 18.3 | Group messaging stress tests (100+ members) | ⏹️ |
| 18.4 | Mailbox deposit/retrieval integration tests | ⏹️ |
| 18.5 | Voice/video call connectivity tests | ⏹️ |
| 18.6 | Protocol version negotiation tests | ⏹️ |
| 18.7 | Security audit (crypto review) | ⏹️ |
| 18.8 | Performance benchmarks (message latency, throughput) | ⏹️ |
| 18.9 | Network stress tests (100+ nodes) | ⏹️ |

---

## v2 Dependency Graph

```
Phase 8 (Identity v2)
    │
    ▼
Phase 9 (X3DH & Double Ratchet)
    │
    ▼
Phase 10 (Wire Format v2)
    │
    ├──► Phase 11 (Multi-Device)
    ├──► Phase 12 (Private Groups) ──► Phase 13 (Public Groups & Channels)
    │         │                               │
    │         └──► Phase 16 (Group Calls) ◄───┘
    ├──► Phase 14 (Offline Delivery) ──► Phase 17 (Reputation)
    └──► Phase 15 (Voice & Video) ──► Phase 16 (Group Calls)
                                            │
                                            ▼
                                    Phase 18 (Integration)
```

---

## Post-v2 Extensions (Future)

| Feature | Priority | Complexity | Description |
|---------|----------|------------|-------------|
| Metadata privacy (onion routing) | Medium | Very High | Sphinx packets, rendezvous points (see protocol-v2.md §13) |
| Encrypted local storage | High | Medium | Encrypt BadgerDB with master key |
| GUI (Tauri or native) | Medium | High | Desktop application |
| Mobile support | Low | Very High | iOS/Android clients (DHT client mode) |
| Token incentives | Low | High | Economic incentives for network contribution |

---

## Timeline Summary

### PoC (Unversioned) - Complete

| Phase | Duration | Cumulative | Status |
|-------|----------|------------|--------|
| 0: Setup | 1 day | Day 1 | ✅ Complete |
| 1: Identity & Crypto | 2 days | Day 3 | ✅ Complete |
| 2: Storage | 2 days | Day 5 | ✅ Complete |
| 3: IPFS Node | 6 days | Day 11 | ✅ Complete |
| 4: Messaging Protocol | 3 days | Day 14 | ✅ Complete |
| 5: CLI | 2 days | Day 16 | ✅ Complete |
| 6: Integration & Testing | — | — | ❌ Cancelled |
| 7: Release | — | — | ❌ Cancelled |

**PoC Total:** 16 working days (complete)

### Protocol v1 (Target) - Planning Phase

| Phase | Est. Duration | Cumulative | Status |
|-------|---------------|------------|--------|
| 8: Identity v1 | 5 days | Day 21 | ⏹️ Pending |
| 9: X3DH & Double Ratchet | 7 days | Day 28 | ⏹️ Pending |
| 10: Wire Format v1 | 4 days | Day 32 | ⏹️ Pending |
| 11: Multi-Device | 5 days | Day 37 | ⏹️ Pending |
| 12: Private Groups | 5 days | Day 42 | ⏹️ Pending |
| 13: Public Groups & Channels | 4 days | Day 46 | ⏹️ Pending |
| 14: Offline Delivery | 4 days | Day 50 | ⏹️ Pending |
| 15: Voice & Video | 6 days | Day 56 | ⏹️ Pending |
| 16: Group Calls | 4 days | Day 60 | ⏹️ Pending |
| 17: Reputation System | 3 days | Day 63 | ⏹️ Pending |
| 18: Integration & Hardening | 7 days | Day 70 | ⏹️ Pending |

**Protocol v1 Estimated:** 54 working days

**Total Project (PoC + v1):** ~70 working days

---

## Appendix: Configuration

### IPFS Configuration

```go
type IPFSConfig struct {
    // Bootstrap
    BootstrapPeers      []string        `json:"bootstrap_peers"`
    BootstrapTimeout    time.Duration   `json:"bootstrap_timeout"`
    
    // Peer persistence
    MaxStoredPeers      int             `json:"max_stored_peers"`  // 100
    MinPeerConnections  int             `json:"min_peer_connections"`  // 10
    
    // Connection management
    ConnectionTimeout   time.Duration   `json:"connection_timeout"`
    DialTimeout         time.Duration   `json:"dial_timeout"`
    MaxConnections      int             `json:"max_connections"`
    LowWater            int             `json:"low_water"`
    HighWater           int             `json:"high_water"`
    
    // NAT traversal
    EnableRelay         bool            `json:"enable_relay"`
    EnableHolePunching  bool            `json:"enable_hole_punching"`
    EnableAutoNAT       bool            `json:"enable_autonat"`
    
    // DHT
    DHTMode             string          `json:"dht_mode"`
    DHTBootstrapTimeout time.Duration   `json:"dht_bootstrap_timeout"`
    
    // Network
    ListenAddrs         []string        `json:"listen_addrs"`
    ProtocolID          string          `json:"protocol_id"`
}
```

### Default Bootstrap Peers

```json
{
  "bootstrap_peers": [
    "/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
    "/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
    "/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiRNN6vEC9qmL9egu92p",
    "/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt"
  ]
}
```

---

*Last updated: February 25, 2026*
*Version: 4.0 (Protocol v1 Planning, PoC Phases 6-7 Cancelled)*
