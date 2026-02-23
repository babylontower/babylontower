# Technical Specification: P2P Secure Messenger – Proof of Concept

## 1. Project Overview

**Goal:** Develop a minimal viable prototype of a decentralized, secure peer-to-peer messenger that operates without any central servers. The application embeds an IPFS node and uses libp2p for networking, providing end-to-end encrypted text messaging between two online users.

**Status:** Phase 5 Complete - CLI fully functional. Ready for Phase 6 testing.

### Key Principles

1. **Full decentralization** – no central coordination or servers
2. **User identity** derived from a master key (BIP39 mnemonic)
3. **All messages** encrypted end-to-end and signed
4. **Messages** stored on IPFS and delivered via libp2p PubSub
5. **Local data** (contacts, messages, peers) persisted in BadgerDB
6. **Single executable**, no external dependencies (IPFS daemon not required)

---

## 2. Functional Requirements (PoC)

### 2.1. Identity Management

**Status:** ✅ Complete

- On first launch, generate a new master key (BIP39 12-word mnemonic) and display it to the user
- Derive the following key pairs deterministically from the master seed:
  - Ed25519 key pair for signing/verification
  - X25519 key pair for encryption (static Diffie-Hellman)
- Persist the master seed to a file in the application data directory (`~/.babylontower/identity.json`)
- On subsequent launches, load the seed from the file and regenerate the keys
- Provide a command to show the user's own public key (hex or base58) to share with contacts

**Implementation:** `pkg/identity/`

---

### 2.2. Contact Management

**Status:** ✅ Complete

- Add a contact by their public key (hex or base58 string). The contact is stored locally
- List all contacts with an index and their public key (shortened for display)
- Contacts are stored in BadgerDB with the public key as the key and a protobuf structure containing:
  - `public_key`: Contact's Ed25519 public key
  - `x25519_public_key`: Contact's X25519 public key (for encryption)
  - `peer_id`: Contact's libp2p PeerID (optional, for faster discovery)
  - `multiaddrs`: Known multiaddrs for the contact (optional)
  - `display_name`: Optional nickname
  - `created_at`: Creation timestamp

**Implementation:** `pkg/storage/contacts.go`

---

### 2.3. Messaging

**Status:** ✅ Complete

#### Outgoing Message Flow

1. User selects a contact (by index or public key) and types a message
2. The message is encrypted and signed as follows:
   - Generate an ephemeral X25519 key pair
   - Compute a shared secret using the recipient's static X25519 public key and the ephemeral private key
   - Derive a symmetric key from the shared secret (use raw shared secret as key for XChaCha20-Poly1305)
   - Encrypt the plaintext message with XChaCha20-Poly1305 using a random 24-byte nonce
   - Create a protobuf `Envelope` containing the ciphertext, ephemeral public key, and nonce
   - Serialize the `Envelope` and sign it with the sender's Ed25519 private key, producing a `SignedEnvelope` (includes the serialized envelope, signature, and sender's public key)
   - Add the `SignedEnvelope` (binary) to IPFS using the embedded node, obtaining a CID (Content Identifier)
   - Publish the CID (as a string) via libp2p PubSub to the recipient's topic (topic = SHA256 of recipient's public key bytes)
3. Store the message locally in BadgerDB
4. Display the sent message to the user

#### Incoming Message Flow

1. The recipient's node is subscribed to its own topic (derived from its public key)
2. Upon receiving a PubSub message containing a CID, it fetches the corresponding data from IPFS using the CID
3. It verifies the signature against the sender's public key (which is included in the `SignedEnvelope`)
4. It decrypts the envelope using its static X25519 private key and the ephemeral public key from the envelope
5. If successful, the plaintext message is stored locally and displayed to the user in real-time

#### Message Enhancements (Phase 4-5)

- **Connection Pooling:** Maintain active connections to frequently-contacted peers
- **Message Retry Logic:** Exponential backoff retry (3 attempts) on send failure
- **Contact Peer Tracking:** Store and reuse contact's PeerID for faster discovery
- **PubSub Mesh Optimization:** Declare interest in contact topics for better delivery

**Implementation:** `pkg/messaging/`

---

### 2.4. Storage (Local Database)

**Status:** ✅ Complete

Use BadgerDB as the embedded key-value store with a unified schema:

#### Key Prefixes

```
"c:" + <pubkey>                    → Contact records (protobuf)
"m:" + <pubkey> + <ts> + <nonce>   → Message envelopes (protobuf)
"p:" + <peer_id>                   → Peer records (JSON)
"cfg:" + <config_key>              → Configuration values (JSON)
```

#### Contact Schema

Key = `c:` + contact public key (32 bytes)
Value = serialized protobuf `Contact`:
```protobuf
message Contact {
  bytes public_key = 1;
  bytes x25519_public_key = 2;
  string peer_id = 3;
  repeated string multiaddrs = 4;
  string display_name = 5;
  uint64 created_at = 6;
}
```

#### Message Schema

Key = `m:` + contact_pubkey (32 bytes) + timestamp (uint64 big-endian) + nonce (24 bytes)
Value = serialized `SignedEnvelope` (protobuf) as received from IPFS

This ensures messages for the same contact are stored together and sorted by time.

#### Peer Record Schema (Phase 3+)

Key = `p:` + peer_id (string)
Value = JSON `PeerRecord`:
```go
type PeerRecord struct {
    PeerID        string      `json:"peer_id"`
    Multiaddrs    []string    `json:"multiaddrs"`
    FirstSeen     time.Time   `json:"first_seen"`
    LastSeen      time.Time   `json:"last_seen"`
    LastConnected time.Time   `json:"last_connected"`
    ConnectCount  int         `json:"connect_count"`
    FailCount     int         `json:"fail_count"`
    SuccessRate   float64     `json:"success_rate"`
    Source        PeerSource  `json:"source"`  // bootstrap, dht, mdns, peer_exchange
    Protocols     []string    `json:"protocols"`
}
```

#### CRUD Interface

```go
// Contacts
AddContact(contact *pb.Contact) error
GetContact(pubKey []byte) (*pb.Contact, error)
ListContacts() ([]*pb.Contact, error)
DeleteContact(pubKey []byte) error

// Messages
AddMessage(contactPubKey []byte, envelope *pb.SignedEnvelope) error
GetMessages(contactPubKey []byte, limit, offset int) ([]*pb.SignedEnvelope, error)
DeleteMessages(contactPubKey []byte) error

// Peers (Phase 3+)
AddPeer(peer *PeerRecord) error
GetPeer(peerID string) (*PeerRecord, error)
ListPeers(limit int) ([]*PeerRecord, error)
DeletePeer(peerID string) error
PrunePeers(maxAge time.Duration, keepCount int) error

// Lifecycle
Close() error
```

**Implementation:** `pkg/storage/`

---

### 2.5. CLI Interface

**Status:** ✅ Complete

Interactive REPL (read-eval-print loop) with commands:

| Command | Description |
|---------|-------------|
| `/help` | List all commands with descriptions |
| `/myid` | Show own public key (hex and base58 formats) |
| `/add <pubkey> [nickname]` | Add a contact by public key |
| `/list` | List all contacts with indices |
| `/chat <contact-index-or-pubkey>` | Enter chat mode with contact |
| `/history <contact> [limit]` | Show last N messages with contact |
| `/network` | Display network health metrics |
| `/contactstatus` | Show contact connection status |
| `/exit` | Quit application |

#### Chat Mode

While in chat mode:
- All input lines are sent as messages to the contact
- Empty line exits chat mode and returns to main prompt
- Incoming messages from that contact are displayed immediately with timestamp
- Messages are encrypted and published via IPFS PubSub

#### Real-Time Features

- Concurrent message listener: Incoming messages display without blocking user input
- Output synchronization: Mutex protects output to prevent interleaved messages
- Prompt refresh: `rl.Refresh()` redraws prompt after async message display
- Signal handling: Graceful shutdown on Ctrl+C (SIGINT)

**Implementation:** `pkg/cli/`

---

## 3. Non-Functional Requirements

| Requirement | Specification | Status |
|-------------|---------------|--------|
| **Language** | Go 1.25+ | ✅ |
| **Performance** | Message delivery within seconds | ✅ |
| **Reliability** | Basic error handling; no crashes on invalid input | ✅ |
| **Portability** | Linux, macOS, Windows (cross-compilation) | ⏳ Pending |
| **Security (Network)** | libp2p Noise protocol (automatic) | ✅ |
| **Security (E2E)** | XChaCha20-Poly1305 + Ed25519 | ✅ |
| **Security (Local)** | Local storage NOT encrypted in PoC | ⚠️ Known limitation |
| **Deployability** | Single binary, no external dependencies | ✅ |

---

## 4. System Architecture

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

### Module Descriptions

| Module | Description | Location |
|--------|-------------|----------|
| **Identity** | Master seed generation, key derivation (Ed25519, X25519), persistence/loading | `pkg/identity/` |
| **Crypto** | Encrypt/decrypt (X25519 + XChaCha20-Poly1305), sign/verify (Ed25519) | `pkg/crypto/` |
| **Storage** | BadgerDB interface and implementation for contacts, messages, peers | `pkg/storage/` |
| **IPFSNode** | Embedded IPFS node: initialization, Add/Get by CID, PubSub subscribe/publish, metrics | `pkg/ipfsnode/` |
| **Messaging** | Core protocol logic: envelope building, outgoing/incoming message flows, retry logic | `pkg/messaging/` |
| **CLI** | REPL implementation, command parsing, display formatting, signal handling | `pkg/cli/` |

---

## 5. Protocol Specification

### 5.1. Identifiers

- **User public key (Ed25519):** Used as the long-term identity. Represented as hex string or base58.
- **Topic for PubSub:** `SHA256(public_key)` (32 bytes) – used to route CID notifications.
- **Topic format:** `"babylon-"` + hex encoding of first 8 bytes of SHA256 hash (e.g., `babylon-3a5f8c2e`)

### 5.2. Protobuf Definitions

**File:** `proto/message.proto`

```protobuf
syntax = "proto3";

package messenger;

// Plaintext message (what the user types)
message Message {
  string text = 1;
  uint64 timestamp = 2; // Unix timestamp (seconds)
}

// Encrypted container
message Envelope {
  bytes ciphertext = 1;          // encrypted Message (XChaCha20-Poly1305)
  bytes ephemeral_pubkey = 2;    // X25519 ephemeral public key (32 bytes)
  bytes nonce = 3;               // 24-byte nonce
}

// Signed and self-contained unit sent over IPFS
message SignedEnvelope {
  bytes envelope = 1;             // serialized Envelope
  bytes signature = 2;            // Ed25519 signature of envelope (64 bytes)
  bytes sender_pubkey = 3;        // Ed25519 public key of sender (32 bytes)
}

// Contact stored locally (not transmitted)
message Contact {
  bytes public_key = 1;           // Ed25519 public key
  bytes x25519_public_key = 2;    // X25519 public key for encryption
  string peer_id = 3;             // libp2p PeerID (optional)
  repeated string multiaddrs = 4; // Known addresses (optional)
  string display_name = 5;
  uint64 created_at = 6;
}
```

### 5.3. Message Flow

#### Outgoing Message

```
1. Construct Message with text and current timestamp
2. Serialize Message to bytes
3. Generate ephemeral X25519 key pair (ephemeral_priv, ephemeral_pub)
4. Compute shared secret = X25519(ephemeral_priv, recipient_static_pub)
   - Use this as key for XChaCha20-Poly1305 (32 bytes)
5. Generate random 24-byte nonce
6. Encrypt: ciphertext = XChaCha20-Poly1305_Encrypt(key, nonce, plaintext)
7. Build Envelope with ciphertext, ephemeral_pub, nonce; serialize to bytes
8. Sign the serialized Envelope with sender's Ed25519 private key
9. Build SignedEnvelope with envelope bytes, signature, sender's public key
10. Add SignedEnvelope (binary) to IPFS → get CID
11. Publish CID string via PubSub to recipient's topic
12. Store message locally in BadgerDB
13. Display sent message to user
```

#### Incoming Message

```
1. Receive CID string via PubSub
2. Fetch data from IPFS by CID → bytes (should be a SignedEnvelope)
3. Parse SignedEnvelope (protobuf)
4. Verify signature using sender_pubkey against the envelope bytes
5. Parse Envelope from envelope bytes
6. Compute shared secret = X25519(recipient_static_priv, envelope.ephemeral_pubkey)
7. Decrypt ciphertext using key, nonce
8. Parse decrypted bytes as Message
9. Store SignedEnvelope in local DB with contact's public key
10. Notify UI of new message
```

### 5.4. IPFS and PubSub Details

- The embedded IPFS node uses a persistent repository (e.g., `~/.babylontower/ipfs`)
- PubSub uses libp2p gossipsub protocol
- Each node subscribes to its own topic (derived from its public key) at startup
- When a node wants to send a message, it publishes the CID to the recipient's topic
- Incoming PubSub messages are delivered via a Go channel to the messaging module

#### Bootstrap Strategy (Phase 3+)

Multi-stage bootstrap for reliable connectivity:

1. **Stage 1:** Load stored peers from BadgerDB, filter by success rate (>50%) and recency (<7 days)
2. **Stage 2:** Attempt parallel connections to top N stored peers
3. **Stage 3:** If <3 connections succeed, try config bootstrap peers (DNS-resolved)
4. **Stage 4:** Wait for DHT routing table to populate (>5 peers)
5. **Stage 5:** Store newly connected peers to BadgerDB for faster restart

---

## 6. Technology Stack

| Component | Library / Package | Version |
|-----------|-------------------|---------|
| **Language** | Go | 1.25+ |
| **IPFS Embedding** | github.com/ipfs/go-ipfs | embedded |
| **libp2p Core** | github.com/libp2p/go-libp2p | v0.47.0 |
| **libp2p PubSub** | github.com/libp2p/go-libp2p-pubsub | included |
| **DHT** | github.com/libp2p/go-libp2p-kad-dht | included |
| **BIP39** | github.com/tyler-smith/go-bip39 | v1.x.x |
| **Ed25519** | crypto/ed25519 | stdlib |
| **X25519** | golang.org/x/crypto/curve25519 | v0.x.x |
| **XChaCha20-Poly1305** | golang.org/x/crypto/chacha20poly1305 | v0.x.x |
| **HKDF** | golang.org/x/crypto/hkdf | v0.x.x |
| **Protobuf** | google.golang.org/protobuf | v1.x.x |
| **BadgerDB** | github.com/dgraph-io/badger/v3 | v3.x.x |
| **CLI (REPL)** | github.com/chzyer/readline | v1.x.x |

---

## 7. Implementation Status

| Phase | Description | Status | Completion |
|-------|-------------|--------|------------|
| 0 | Project Setup | ✅ Complete | 100% |
| 1 | Identity & Crypto | ✅ Complete | 100% |
| 2 | Storage Layer | ✅ Complete | 100% |
| 3 | IPFS Node Integration | ✅ Complete | 100% |
| 4 | Messaging Protocol | ✅ Complete | 100% |
| 5 | CLI Interface | ✅ Complete | 100% |
| 6 | Integration & Testing | ⏳ In Progress | 50% |
| 7 | Release Preparation | ⏹️ Pending | 0% |

**Overall Progress:** 80% complete (Phases 0-5 done)

See [`roadmap.md`](roadmap.md) for detailed implementation plan and [`testing.md`](testing.md) for test status.

---

## 8. Known Limitations (PoC)

### Functional Limitations

| Limitation | Impact | Workaround | Future Enhancement |
|------------|--------|------------|-------------------|
| **IPFS Get not fully implemented** | Cannot fetch envelopes from IPFS by CID in all cases | Direct PubSub message passing works | Complete IPFS blockstore implementation |
| **X25519 keys not stored with contacts** | Cannot fully encrypt messages without manual key exchange | Manual exchange for testing | Store X25519 keys with contacts |
| **NAT traversal limited** | Nodes must have direct connectivity or be on same network | AutoNAT + hole punching implemented | Relay nodes, UPnP/NAT-PMP |
| **No offline message queuing** | Both parties must be online for message delivery | N/A | Supernode architecture |
| **Local storage not encrypted** | Private keys and messages stored in plaintext on disk | N/A | Encrypt BadgerDB with master key |
| **No group chat** | Only 1:1 messaging supported | N/A | Group key management |
| **No forward secrecy** | Uses static keys only (no Double Ratchet) | N/A | Implement Double Ratchet protocol |

### Testing Limitations

| Limitation | Impact | Notes |
|------------|--------|-------|
| **mDNS fails in containerized environments** | Integration tests skipped in CI | Docker blocks UDP multicast (224.0.0.251:5353) |
| **Multi-node tests require network** | Some tests skipped in isolated environments | Manual connection tests pass |
| **Messaging coverage 29.8%** | Service layer integration not fully tested | Core crypto logic has >90% coverage |

---

## 9. Security Considerations

### Cryptographic Design

1. **Identity:** BIP39 mnemonic → 512-bit seed → deterministic Ed25519 and X25519 keys
2. **Encryption:** XChaCha20-Poly1305 with ephemeral X25519 key agreement
3. **Signatures:** Ed25519 for message authentication
4. **Key Derivation:** HKDF for deriving symmetric keys from shared secrets

### Security Properties

- **End-to-End Encryption:** Messages encrypted with recipient's public key
- **Authentication:** All messages signed with sender's private key
- **Forward Secrecy:** Ephemeral keys provide some forward secrecy (not full Double Ratchet)
- **Transport Security:** libp2p Noise protocol encrypts all network traffic

### Known Security Limitations

- **Local storage unencrypted:** Private keys stored in plaintext in `identity.json`
- **No key rotation:** Static keys used throughout session
- **No ratcheting:** Messages use same shared secret derivation
- **Contact verification:** No out-of-band verification mechanism

---

## 10. Building and Running

### Requirements

- Go 1.25 or later
- GNU Make
- protoc (optional, for protobuf generation)

### Build Commands

```bash
# Build the application
make build

# Run all tests
make test

# Run tests with coverage report
make test-coverage

# Run linter (requires golangci-lint)
make install-deps
make lint

# Build and run the application
make run
```

### Run the Application

```bash
# After building, run directly
./bin/messenger
```

### First Launch

On first launch, the application will:
1. Generate a new BIP39 mnemonic (12 words)
2. Display the mnemonic with a warning to save it securely
3. Derive Ed25519 and X25519 key pairs
4. Save identity to `~/.babylontower/identity.json`
5. Display your public key for sharing with contacts

### Usage Example

```bash
# Terminal 1 (Alice)
./bin/messenger
>>> /myid
Your Public Key:
  Hex:    0e1ba4bb4d0bb9d0...
  Base58: x5A38TJihSa1rofx...

>>> /add <Bob's public key> Bob
>>> /list
=== Contacts ===
[1] Bob - 2Uw1bppLugs5Bqtr...
================

>>> /chat 1
━━━ Chat with Bob ━━━
Type your message and press Enter to send.
Press Enter on an empty line to exit chat.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Hello Bob!
[2026-02-23 10:30:00] You: Hello Bob!
```

---

## 11. Deliverables

### Completed

- ✅ Source code repository with complete Go application
- ✅ README with build/run instructions
- ✅ Technical specification (this document)
- ✅ Implementation roadmap (`roadmap.md`)
- ✅ Testing specification (`testing.md`)
- ✅ Unit tests (>80% coverage for core modules)
- ✅ Integration tests (with documented caveats)

### Pending (Phase 7)

- ⏹️ Compiled binaries for Linux, macOS, Windows
- ⏹️ Git tag v0.1.0-poc
- ⏹️ Release notes published

---

## 12. Future Extensions (Post-PoC)

| Feature | Priority | Complexity | Description |
|---------|----------|------------|-------------|
| **Encrypted local storage** | High | Medium | Encrypt BadgerDB with master key |
| **Full IPFS Get implementation** | High | Medium | Complete CID-based message retrieval |
| **Contact X25519 key storage** | High | Low | Store X25519 keys with contacts |
| **Group chat support** | High | High | Multi-party messaging with key rotation |
| **Double Ratchet** | Medium | High | Signal Protocol-like forward secrecy |
| **Offline message queuing** | Medium | High | Store-and-forward via supernodes |
| **GUI (Tauri or native)** | Low | High | Desktop application |
| **Mobile support** | Low | Very High | iOS/Android clients |
| **Supernode architecture** | Low | Very High | Dedicated relay nodes for offline messages |

---

*Last updated: February 23, 2026*
*Version: 2.0 (PoC Phase 5 Complete)*
