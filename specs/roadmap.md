# Babylon Tower - Implementation Roadmap

## Project Overview

**Babylon Tower** is a decentralized, secure peer-to-peer messenger that operates without central servers. This roadmap details the implementation plan for the Proof of Concept (PoC) as specified in `poc.md`.

---

## System Architecture

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
                          └────────┬────────┘
                                   │
                          ┌────────▼────────┐
                          │   Storage       │
                          │  (BadgerDB)     │
                          └─────────────────┘
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
│   │   ├── identity_test.go
│   │   └── keys.go              # Ed25519, X25519 key operations
│   ├── crypto/
│   │   ├── crypto.go            # Encrypt/decrypt (XChaCha20-Poly1305)
│   │   ├── crypto_test.go
│   │   └── sign.go              # Sign/verify (Ed25519)
│   ├── storage/
│   │   ├── storage.go           # Storage interface
│   │   ├── badger.go            # BadgerDB implementation
│   │   ├── contacts.go          # Contact CRUD
│   │   ├── messages.go          # Message CRUD
│   │   └── storage_test.go
│   ├── ipfsnode/
│   │   ├── node.go              # Embedded IPFS node wrapper
│   │   ├── pubsub.go            # PubSub subscribe/publish
│   │   └── node_test.go
│   ├── messaging/
│   │   ├── protocol.go          # Core protocol logic
│   │   ├── envelope.go          # Envelope building/parsing
│   │   ├── outgoing.go          # Send message flow
│   │   ├── incoming.go          # Receive message flow
│   │   └── messaging_test.go
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
    └── roadmap.md               # This document
```

---

## Implementation Phases

### Phase 0: Project Setup (Day 1)

**Goal:** Establish project foundation and tooling.

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 0.1 | Initialize Go module (`go mod init babylontower`) | `go.mod` exists with Go 1.19+ |
| 0.2 | Create directory structure | All directories from structure above exist |
| 0.3 | Add dependencies to `go.mod` | All dependencies from spec section 6 |
| 0.4 | Create Makefile with common targets | `make build`, `make test`, `make lint` work |
| 0.5 | Set up CI configuration (GitHub Actions) | `.github/workflows/ci.yml` runs tests on push |
| 0.6 | Create initial README.md | Build instructions, project overview |

**Deliverables:**
- [ ] Working Go module with all dependencies
- [ ] Makefile with build/test/lint targets
- [ ] CI pipeline passing
- [ ] Basic README

---

### Phase 1: Identity & Cryptography (Days 2-3)

**Goal:** Implement secure identity management and cryptographic operations.

#### 1.1 Identity Module (`pkg/identity`)

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 1.1.1 | BIP39 mnemonic generation | `GenerateMnemonic()` returns valid 12-word phrase |
| 1.1.2 | Seed derivation from mnemonic | `DeriveSeed(mnemonic)` produces 512-bit seed |
| 1.1.3 | Ed25519 key pair derivation | Deterministic key from seed index 0 |
| 1.1.4 | X25519 key pair derivation | Deterministic key from seed index 1 |
| 1.1.5 | Identity persistence (load/save) | Identity survives application restart |
| 1.1.6 | Public key export (hex/base58) | `PublicKeyHex()` and `PublicKeyBase58()` work |

#### 1.2 Crypto Module (`pkg/crypto`)

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 1.2.1 | X25519 shared secret computation | `ComputeSharedSecret(priv, pub)` returns 32 bytes |
| 1.2.2 | XChaCha20-Poly1305 encryption | `Encrypt(key, nonce, plaintext)` produces ciphertext+tag |
| 1.2.3 | XChaCha20-Poly1305 decryption | `Decrypt(key, nonce, ciphertext)` recovers plaintext |
| 1.2.4 | Ed25519 signing | `Sign(priv, message)` produces 64-byte signature |
| 1.2.5 | Ed25519 verification | `Verify(pub, message, signature)` returns bool |
| 1.2.6 | HKDF key derivation (if needed) | `DeriveKey(ikm, salt, info, length)` works |

**Deliverables:**
- [ ] Identity generation and persistence working
- [ ] All crypto functions implemented and tested
- [ ] Unit tests with >90% coverage for both modules
- [ ] No hardcoded secrets in tests

---

### Phase 2: Storage Layer (Days 4-5)

**Goal:** Implement embedded key-value storage with BadgerDB.

#### 2.1 Protobuf Definitions (`proto/`)

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 2.1.1 | Create `message.proto` | All messages from spec section 5.2 |
| 2.1.2 | Generate Go code | `message.pb.go` compiles correctly |
| 2.1.3 | Add Makefile proto target | `make proto` regenerates code |

#### 2.2 Storage Implementation (`pkg/storage`)

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 2.2.1 | BadgerDB initialization | DB opens in configurable directory |
| 2.2.2 | Contact storage interface | `AddContact`, `GetContact`, `ListContacts`, `DeleteContact` |
| 2.2.3 | Message storage interface | `AddMessage`, `GetMessages`, `DeleteMessages` |
| 2.2.4 | Composite key for messages | Format: `contact_pubkey + timestamp + nonce` |
| 2.2.5 | Transaction handling | Proper Badger transactions for writes |
| 2.2.6 | Graceful shutdown | DB closes without data loss |

**Deliverables:**
- [ ] Protobuf definitions compiled
- [ ] Storage interface fully implemented
- [ ] Unit tests with in-memory or temp DB
- [ ] CRUD operations verified

---

### Phase 3: IPFS Node Integration (Days 6-11)

**Goal:** Embed IPFS node with PubSub functionality.

#### 3.1 Core Node (`pkg/ipfsnode`)

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 3.1.1 | IPFS node initialization | Node starts with repo in app data dir |
| 3.1.2 | Repository configuration | Configurable path (`~/.babylontower/ipfs`) |
| 3.1.3 | Graceful shutdown | `Close()` stops node cleanly |
| 3.1.4 | Add data to IPFS | `Add(data)` returns CID |
| 3.1.5 | Get data from IPFS | `Get(cid)` returns bytes |
| 3.1.6 | Error handling | Network errors properly propagated |

#### 3.2 PubSub (`pkg/ipfsnode/pubsub.go`)

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 3.2.1 | Topic derivation | `TopicFromPublicKey(pubkey)` = SHA256(pubkey) |
| 3.2.2 | Subscribe to topic | `Subscribe(topic)` returns message channel |
| 3.2.3 | Publish to topic | `Publish(topic, data)` broadcasts message |
| 3.2.4 | Message channel handling | Channel receives incoming messages |
| 3.2.5 | Subscription lifecycle | Unsubscribe on shutdown |

#### 3.3 Integration Testing

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 3.3.1 | Two-node test setup | Two nodes in-process on different ports |
| 3.3.2 | Add/Get test | Node A adds data, Node B retrieves |
| 3.3.3 | PubSub test | Node A publishes, Node B receives |
| 3.3.4 | Connection test | Nodes discover and connect |

**Deliverables:**
- [ ] Embedded IPFS node working
- [ ] PubSub subscribe/publish functional
- [ ] Integration tests passing (2 nodes)
- [ ] No external IPFS daemon required

---

### Phase 4: Messaging Protocol (Days 12-14)

**Goal:** Implement end-to-end encrypted messaging protocol.

#### 4.1 Protocol Core (`pkg/messaging`)

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 4.1.1 | Message protobuf builder | `BuildMessage(text, timestamp)` |
| 4.1.2 | Envelope creation | `BuildEnvelope(plaintext, recipient_pubkey)` |
| 4.1.3 | Signed envelope creation | `SignEnvelope(envelope, sender_privkey)` |
| 4.1.4 | Envelope parsing | `ParseSignedEnvelope(bytes)` |
| 4.1.5 | Signature verification | `VerifyEnvelope(envelope)` returns bool |

#### 4.2 Outgoing Messages (`pkg/messaging/outgoing.go`)

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 4.2.1 | Full encryption flow | plaintext → envelope → signed → CID |
| 4.2.2 | IPFS add integration | Signed envelope added to IPFS |
| 4.2.3 | PubSub publish | CID published to recipient topic |
| 4.2.4 | Local message storage | Sent messages stored in BadgerDB |
| 4.2.5 | Error handling | Failures logged and returned |

#### 4.3 Incoming Messages (`pkg/messaging/incoming.go`)

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 4.3.1 | PubSub message handler | Receives CID from channel |
| 4.3.2 | IPFS fetch | Retrieves SignedEnvelope by CID |
| 4.3.3 | Signature verification | Verifies sender signature |
| 4.3.4 | Decryption | Decrypts with static + ephemeral keys |
| 4.3.5 | Message storage | Stores in BadgerDB by contact |
| 4.3.6 | Callback/notification | Notifies CLI of new message |

#### 4.4 Messaging Service

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 4.4.1 | Service initialization | Wiring all dependencies |
| 4.4.2 | Background goroutines | Listener runs concurrently |
| 4.4.3 | Contact validation | Only accept from known contacts (optional for PoC) |
| 4.4.4 | Message history retrieval | `GetHistory(contact, limit)` |

**Deliverables:**
- [ ] Full message encryption/decryption working
- [ ] End-to-end message delivery verified
- [ ] Messages persisted and retrievable
- [ ] Unit and integration tests passing

---

### Phase 5: CLI Interface (Days 15-16)

**Goal:** Build interactive command-line interface.

#### 5.1 REPL Engine (`pkg/cli`)

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 5.1.1 | Read-eval-print loop | Continuous input processing |
| 5.1.2 | Command parsing | `/command args` pattern recognized |
| 5.1.3 | Concurrent input/events | User input and incoming messages don't block |
| 5.1.4 | Graceful exit | `/exit` closes all resources |

#### 5.2 Commands (`pkg/cli/commands.go`)

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 5.2.1 | `/help` | Lists all commands with descriptions |
| 5.2.2 | `/myid` | Displays own public key (hex/base58) |
| 5.2.3 | `/add <pubkey> [nickname]` | Adds contact to storage |
| 5.2.4 | `/list` | Shows numbered contact list |
| 5.2.5 | `/chat <contact>` | Enters chat mode with contact |
| 5.2.6 | `/history <contact> [limit]` | Shows last N messages |
| 5.2.7 | `/exit` | Quits application |

#### 5.3 Chat Mode

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 5.3.1 | Message input loop | Lines sent as messages |
| 5.3.2 | Empty line exits chat | Returns to main prompt |
| 5.3.3 | Real-time message display | Incoming messages shown immediately |
| 5.3.4 | Message formatting | Timestamps, sender info displayed |

#### 5.4 Display (`pkg/cli/display.go`)

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 5.4.1 | Contact list formatting | Shortened keys, nice table |
| 5.4.2 | Message formatting | `[timestamp] sender: text` |
| 5.4.3 | Error display | User-friendly error messages |
| 5.4.4 | Help formatting | Clear command descriptions |

**Deliverables:**
- [ ] All commands implemented
- [ ] Chat mode with real-time updates
- [ ] Clean, readable UI
- [ ] Manual testing completed

---

### Phase 6: Integration & Testing (Days 17-19)

**Goal:** End-to-end testing and bug fixes.

#### 6.1 End-to-End Tests

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 6.1.1 | Two-instance test | Two CLI instances on same machine |
| 6.1.2 | Identity exchange | Users share public keys |
| 6.1.3 | Bidirectional chat | Both users send and receive |
| 6.1.4 | History verification | Messages persist after restart |
| 6.1.5 | Error scenarios | Invalid keys, network issues handled |

#### 6.2 Bug Fixes & Hardening

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 6.2.1 | Fix identified bugs | All critical bugs resolved |
| 6.2.2 | Improve error messages | User-friendly and actionable |
| 6.2.3 | Add logging | Structured logs for debugging |
| 6.2.4 | Handle edge cases | Empty messages, large messages, etc. |
| 6.2.5 | Resource cleanup | No goroutine leaks, proper shutdown |

#### 6.3 Documentation

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 6.3.1 | README usage section | Step-by-step examples |
| 6.3.2 | Build instructions | Cross-platform build commands |
| 6.3.3 | Architecture overview | Diagrams and module descriptions |
| 6.3.4 | Troubleshooting guide | Common issues and solutions |

**Deliverables:**
- [ ] All end-to-end tests passing
- [ ] No critical bugs remaining
- [ ] Complete README documentation
- [ ] Demo-ready application

---

### Phase 7: Release Preparation (Day 20)

**Goal:** Prepare PoC for release.

| Task | Description | Acceptance Criteria |
|------|-------------|---------------------|
| 7.1 | Cross-compilation setup | Build for Linux, macOS, Windows |
| 7.2 | Binary testing | Binaries run on each platform |
| 7.3 | Final code review | No obvious issues remaining |
| 7.4 | Tag release (v0.1.0-poc) | Git tag created |
| 7.5 | Release notes | Summary of features and limitations |

**Deliverables:**
- [ ] Compiled binaries for 3 platforms
- [ ] Git tag v0.1.0-poc
- [ ] Release notes published

---

## Testing Strategy

### Unit Tests

**Scope:** Individual functions and methods.

| Module | Key Test Areas |
|--------|----------------|
| `identity` | Mnemonic generation, key derivation, persistence |
| `crypto` | Encrypt/decrypt roundtrip, sign/verify |
| `storage` | CRUD operations, key formatting, transactions |
| `ipfsnode` | Add/Get, PubSub mock tests |
| `messaging` | Envelope building, encryption flow |
| `cli` | Command parsing, input handling |

**Coverage Target:** >80% overall

### Integration Tests

**Scope:** Module interactions.

| Test | Description |
|------|-------------|
| Identity + Storage | Save and load identity |
| Messaging + IPFS | Send/receive via embedded node |
| Messaging + Storage | Persist and retrieve messages |
| Two-node PubSub | Publish/subscribe between nodes |

### End-to-End Tests

**Scope:** Full application flow.

| Scenario | Steps |
|----------|-------|
| First launch | Generate identity, display public key |
| Add contact | Input public key, verify stored |
| Send message | Type message, verify received on other instance |
| Chat history | Send multiple messages, restart, verify history |
| Network error | Disconnect network, verify error handling |

### Manual Testing Checklist

- [ ] Fresh install generates valid mnemonic
- [ ] Identity persists across restarts
- [ ] Contact addition works with valid public key
- [ ] Invalid public key rejected
- [ ] Messages encrypted and decrypted correctly
- [ ] Signatures verified correctly
- [ ] PubSub delivers messages in real-time
- [ ] Message history sorted by timestamp
- [ ] All CLI commands functional
- [ ] Graceful shutdown on `/exit` and Ctrl+C

---

## Risk Register

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| go-ipfs embedding complexity | High | Medium | Use `ipfs-core` examples; consider minimal libp2p-only fallback |
| BadgerDB corruption | Medium | Low | Proper transaction handling; graceful shutdown |
| NAT traversal issues | High | High | Document as PoC limitation; assume direct connectivity |
| Memory usage with embedded IPFS | Medium | Medium | Tune IPFS config; document minimum requirements |
| Dependency conflicts | Medium | Medium | Pin versions; use Go modules strictly |
| Cryptographic implementation errors | High | Low | Use well-tested libraries; thorough testing |

---

## Success Criteria

The PoC is considered successful when:

1. **Functional:**
   - Two instances can exchange encrypted messages without any central server
   - Messages are signed and verified correctly
   - Identity is derived from mnemonic and persists
   - Contacts and messages are stored locally

2. **Technical:**
   - All unit tests pass (>80% coverage)
   - Integration tests pass
   - End-to-end demo works reliably
   - No external dependencies (single binary)

3. **Documentation:**
   - README with build and usage instructions
   - Protocol specification complete
   - Known limitations documented

---

## Post-PoC Extensions (Future Phases)

| Feature | Priority | Complexity |
|---------|----------|------------|
| Encrypted local storage | High | Medium |
| Group chat support | High | High |
| Double Ratchet (forward secrecy) | Medium | High |
| Offline message queuing | Medium | High |
| GUI (Tauri or native) | Low | High |
| Mobile support | Low | Very High |
| Supernode architecture | Low | Very High |

---

## Timeline Summary

| Phase | Duration | Cumulative |
|-------|----------|------------|
| 0: Setup | 1 day | Day 1 |
| 1: Identity & Crypto | 2 days | Day 3 |
| 2: Storage | 2 days | Day 5 |
| 3: IPFS Node | 6 days | Day 11 |
| 4: Messaging Protocol | 3 days | Day 14 |
| 5: CLI | 2 days | Day 16 |
| 6: Integration & Testing | 3 days | Day 19 |
| 7: Release | 1 day | Day 20 |

**Total Estimated Duration:** 20 working days

---

## Appendix: Key Dependencies

```go
// Core
go 1.19

// IPFS / libp2p
github.com/ipfs/go-ipfs v0.x.x
github.com/libp2p/go-libp2p-pubsub v0.x.x

// Cryptography
github.com/tyler-smith/go-bip39 v1.x.x
golang.org/x/crypto v0.x.x  // curve25519, chacha20poly1305, hkdf
crypto/ed25519              // standard library

// Storage
github.com/dgraph-io/badger/v3 v3.x.x

// Protobuf
google.golang.org/protobuf v1.x.x

// CLI (optional helpers)
github.com/chzyer/readline v1.x.x  // or similar for REPL
```

---

*Last updated: February 21, 2026*
*Version: 1.0*
