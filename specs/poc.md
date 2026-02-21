Technical Specification: P2P Secure Messenger – Proof of Concept (Go)
1. Project Overview

Goal: Develop a minimal viable prototype of a decentralized, secure peer-to-peer messenger that operates without any central servers. The application will embed an IPFS node and use libp2p for networking, providing end-to-end encrypted text messaging between two online users. The prototype will be a command-line interface (CLI) tool, demonstrating the core concepts and serving as a foundation for future enhancements.

Key Principles:

    Full decentralization – no central coordination or servers.

    User identity derived from a master key (BIP39 mnemonic).

    All messages are encrypted end-to-end and signed.

    Messages are stored on IPFS and delivered via libp2p PubSub.

    Local data (contacts, messages) persisted in an embedded key-value store (BadgerDB).

    Single executable, no external dependencies (IPFS daemon not required).

2. Functional Requirements (PoC)
2.1. Identity Management

    On first launch, generate a new master key (BIP39 12-word mnemonic) and display it to the user.

    Derive the following key pairs deterministically from the master seed:

        Ed25519 key pair for signing/verification.

        X25519 key pair for encryption (static Diffie-Hellman).

    Persist the master seed (encrypted with a passphrase? – for PoC, we may store it unencrypted in a file; but we should design for future encryption). At minimum, save the seed to a file in the application data directory.

    On subsequent launches, load the seed from the file and regenerate the keys.

    Provide a command to show the user's own public key (hex or base58) to share with contacts.

2.2. Contact Management

    Add a contact by their public key (hex string). The contact is stored locally.

    List all contacts with an index and their public key (shortened for display).

    Contacts are stored in BadgerDB with the public key as the key and a simple protobuf structure containing nickname (optional) and creation timestamp.

2.3. Messaging

    User selects a contact (by index or public key) and types a message.

    The message is encrypted and signed as follows:

        Generate an ephemeral X25519 key pair.

        Compute a shared secret using the recipient's static X25519 public key and the ephemeral private key.

        Derive a symmetric key from the shared secret (using HKDF or simply use the raw shared secret as key for XChaCha20-Poly1305 – ensure proper length).

        Encrypt the plaintext message with XChaCha20-Poly1305 using a random nonce.

        Create a protobuf Envelope containing the ciphertext, ephemeral public key, and nonce.

        Serialize the Envelope and sign it with the sender's Ed25519 private key, producing a SignedEnvelope (includes the serialized envelope, signature, and sender's public key).

        Add the SignedEnvelope (binary) to IPFS using the embedded node, obtaining a CID (Content Identifier).

        Publish the CID (as a string) via libp2p PubSub to the recipient's topic (topic = hash of recipient's public key, e.g., SHA256 of public key bytes).

    On the receiving side:

        The recipient's node is subscribed to its own topic.

        Upon receiving a PubSub message containing a CID, it fetches the corresponding data from IPFS using the CID.

        It verifies the signature against the sender's public key (which is included in the SignedEnvelope).

        It decrypts the envelope using its static X25519 private key and the ephemeral public key from the envelope.

        If successful, the plaintext message is stored locally and displayed to the user in real-time (or made available for later viewing).

    Messages are stored in BadgerDB, keyed by contact public key + timestamp + nonce, to allow retrieval of conversation history per contact.

    The CLI shall provide a command to view the last N messages with a given contact.

2.4. Storage (Local Database)

    Use BadgerDB as the embedded key-value store.

    Two main collections (implemented as Badger buckets or prefixed keys):

        Contacts: key = contact public key (32 bytes), value = serialized protobuf Contact (with fields: public_key, display_name (optional), created_at).

        Messages: key = composite: contact_pubkey (32 bytes) + timestamp (uint64 big-endian) + nonce (24 bytes). This ensures messages for the same contact are stored together and sorted by time.
        Value = serialized SignedEnvelope (protobuf) as received from IPFS.

    Provide a simple CRUD interface: AddContact, GetContact, ListContacts, AddMessage, GetMessages(contactPubKey, limit, offset).

2.5. CLI Interface

    Interactive REPL (read-eval-print loop) with commands:

        /help – list commands.

        /myid – show own public key.

        /add <pubkey> [nickname] – add a contact.

        /list – list contacts.

        /chat <contact-index-or-pubkey> – enter chat mode (then type messages line by line; empty line to exit).

        /history <contact-index-or-pubkey> [limit] – show last messages with contact.

        /exit – quit.

    While in chat mode, incoming messages from that contact are displayed immediately.

    The application must handle asynchronous events: incoming messages via PubSub and user input concurrently.

3. Non-Functional Requirements

    Language: Go (1.19+).

    Performance: Acceptable for demonstration (message delivery within seconds).

    Reliability: Basic error handling; no crashes on invalid input.

    Portability: Should run on Linux, macOS, Windows (cross-compilation possible).

    Security:

        All network communication encrypted via libp2p's Noise protocol (automatically provided by libp2p transport).

        Message payloads encrypted end-to-end as described.

        Signatures prevent impersonation.

        Note: Local storage is not encrypted in PoC (future iterations will add encryption with master key).

    Deployability: Single binary with no external dependencies. The IPFS repository is created in the user's application data directory (e.g., ~/.local/share/messenger/ipfs).

4. System Architecture

The application consists of several loosely coupled modules communicating via channels or method calls. The main components:
text

+----------------+       +------------------+
|     CLI        |<----->|   Messaging      |
| (user input)   |       |   (business      |
|                |       |    logic)        |
+----------------+       +------------------+
                              |         ^
                              v         |
                     +------------------+-----+
                     |   IPFS Node            |
                     | (embedded go-ipfs)     |
                     |   + PubSub             |
                     +------------------------+
                              |         ^
                              v         |
                     +------------------+-----+
                     |   Storage              |
                     | (BadgerDB)             |
                     +------------------------+

Module Descriptions:

    Identity (pkg/identity): Handles master seed generation, key derivation (Ed25519, X25519), persistence/loading.

    Crypto (pkg/crypto): Provides functions for encrypt/decrypt (X25519 + XChaCha20-Poly1305) and sign/verify (Ed25519).

    Storage (pkg/storage): Defines interfaces and implements Badger-backed persistence for contacts and messages.

    IPFSNode (pkg/ipfsnode): Encapsulates the embedded IPFS node: initialization, start/stop, adding data, retrieving by CID, and PubSub subscribe/publish. It exposes channels for incoming messages.

    Messaging (pkg/messaging): Contains the core protocol logic: building protobuf structures, handling outgoing messages (encrypt → add → publish), handling incoming notifications (CID → get → verify → decrypt → store). It uses the other modules.

    CLI (pkg/cli): Implements the REPL, command parsing, and display. It starts the IPFSNode and Messaging in background goroutines and listens for events.

5. Protocol Specification (First Version)
5.1. Identifiers

    User public key (Ed25519): used as the long-term identity. Represented as hex string or base58.

    Topic for PubSub: sha256(public_key) (32 bytes) – used to route CID notifications.

5.2. Protobuf Definitions

We use Protocol Buffers (proto3) for all structured data.

message.proto
protobuf

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
  bytes ephemeral_pubkey = 2;    // X25519 ephemeral public key
  bytes nonce = 3;               // 24-byte nonce
}

// Signed and self-contained unit sent over IPFS
message SignedEnvelope {
  bytes envelope = 1;             // serialized Envelope
  bytes signature = 2;             // Ed25519 signature of envelope
  bytes sender_pubkey = 3;         // Ed25519 public key of sender
}

// Contact stored locally (not transmitted)
message Contact {
  bytes public_key = 1;
  string display_name = 2;
  uint64 created_at = 3;
}

5.3. Message Flow

Outgoing:

    Construct Message with text and current timestamp.

    Serialize Message to bytes.

    Generate ephemeral X25519 key pair (ephemeral_priv, ephemeral_pub).

    Compute shared secret = X25519(ephemeral_priv, recipient_static_pub). Use this as key for XChaCha20-Poly1305 (32 bytes).

    Generate random 24-byte nonce.

    Encrypt: ciphertext = XChaCha20-Poly1305_Encrypt(key, nonce, plaintext).

    Build Envelope with ciphertext, ephemeral_pub, nonce; serialize to bytes.

    Sign the serialized Envelope with sender's Ed25519 private key.

    Build SignedEnvelope with envelope bytes, signature, sender's public key.

    Add SignedEnvelope (binary) to IPFS → get CID.

    Publish CID string via PubSub to recipient's topic.

Incoming:

    Receive CID string via PubSub.

    Fetch data from IPFS by CID → bytes (should be a SignedEnvelope).

    Parse SignedEnvelope (protobuf).

    Verify signature using sender_pubkey (from envelope) against the envelope bytes.

    Parse Envelope from envelope bytes.

    Compute shared secret = X25519(recipient_static_priv, envelope.ephemeral_pubkey).

    Decrypt ciphertext using key, nonce.

    Parse decrypted bytes as Message.

    Store SignedEnvelope in local DB with contact's public key and timestamp/nonce from envelope (though nonce is already part of key).

    Notify UI of new message.

5.4. IPFS and PubSub Details

    The embedded IPFS node uses a persistent repository (e.g., ~/.local/share/messenger/ipfs).

    PubSub uses libp2p gossipsub.

    Each node subscribes to its own topic (derived from its public key) at startup.

    When a node wants to send a CID, it publishes to the recipient's topic.

    Incoming PubSub messages are delivered via a Go channel to the messaging module.

6. Technology Stack
Component	Library / Package
Language	Go 1.19+
IPFS Embedding	github.com/ipfs/go-ipfs (core)
libp2p PubSub	github.com/libp2p/go-libp2p-pubsub (already included in go-ipfs)
BIP39	github.com/tyler-smith/go-bip39
Ed25519	crypto/ed25519 (standard library)
X25519	golang.org/x/crypto/curve25519
XChaCha20-Poly1305	golang.org/x/crypto/chacha20poly1305
HKDF	golang.org/x/crypto/hkdf (if needed for key derivation)
Protobuf	google.golang.org/protobuf
BadgerDB	github.com/dgraph-io/badger/v3
Configuration	(optional) github.com/spf13/viper
Logging	Standard log package or github.com/sirupsen/logrus (optional)
7. Development Phases and Timeline (Estimated: 20 working days)

Phase 0: Setup (1 day)

    Initialize Go module.

    Create project structure (cmd/, pkg/).

    Choose and add dependencies.

Phase 1: Identity & Crypto (2 days)

    Implement identity generation (BIP39) and persistence.

    Implement crypto functions (encrypt/decrypt, sign/verify).

    Write unit tests for both.

Phase 2: Storage (2 days)

    Set up BadgerDB.

    Define protobuf structures for Contact and SignedEnvelope.

    Implement storage interface (AddContact, GetContact, ListContacts, AddMessage, GetMessages).

    Test with temporary database.

Phase 3: IPFS Node (6 days)

    Study go-ipfs embedding examples.

    Implement IPFSNode module:

        Initialization and start (with configurable repo path).

        Graceful shutdown.

        Add and Get methods (using UnixFS).

        PubSub subscription to own topic, handling incoming messages via a channel.

    Write integration test with two nodes in-process (different ports).

Phase 4: Messaging Logic (3 days)

    Implement protobuf (de)serialization.

    Build outgoing flow: encrypt → add → publish.

    Build incoming flow: receive CID → get → verify → decrypt → store.

    Integrate with storage and IPFSNode.

Phase 5: CLI (2 days)

    Implement REPL with goroutines for user input and event listening.

    Connect CLI to messaging module.

    Handle commands: /add, /list, /chat, /history, /myid, /exit.

    Display messages in real-time.

Phase 6: Integration and Testing (3 days)

    End-to-end testing with two instances (locally on different ports).

    Fix bugs, improve error handling.

    Add basic logging to trace execution.

Phase 7: Documentation (1 day)

    Write README with build/run instructions.

    Document protocol (this specification).

8. Risks and Mitigations

    Embedding go-ipfs complexity: go-ipfs has many dependencies; initial setup may be time-consuming. Mitigation: use the core package and examples from ipfs-cluster or ipfs-embedded repos.

    BadgerDB concurrency: Badger supports multiple readers/writers; we'll use a single global DB instance and ensure proper transaction handling.

    CID serialization: CID type must be converted to string for PubSub; ensure proper encoding/decoding.

    NAT traversal: Not addressed in PoC; we assume nodes can connect directly. In real world, libp2p can use relay and hole punching, but we skip for simplicity.

9. Deliverables

    Source code repository with the complete Go application.

    README with build instructions and usage examples.

    Protocol specification (this document, included in repo).

    Compiled binaries for Linux, macOS, Windows (optional for PoC, but can be provided).

10. Future Extensions (Post-PoC)

    Group chats with key rotation.

    Perfect Forward Secrecy (Double Ratchet).

    Multiple device synchronization.

    Metadata protection (global feed, ring signatures).

    GUI using Tauri (Rust frontend) or a native Go UI.

    Supernodes for offline message storage and relay.

    Encrypted local database.

This technical specification provides a clear roadmap for the PoC implementation. The focus is on simplicity, demonstrability, and a solid foundation for later enhancements.
