# Babylon Tower

A decentralized, secure peer-to-peer messenger that operates without central servers.

## Overview

Babylon Tower is a peer-to-peer messaging implementation using:

- **End-to-end encryption** (XChaCha20-Poly1305, Ed25519 signatures)
- **X3DH + Double Ratchet** (forward secrecy + post-compromise security)
- **IPFS** for decentralized communication via PubSub
- **BadgerDB** for local message and contact storage
- **Go** for a single, portable binary
- **Interactive CLI** for user interaction

## ⚠️ Development Status

**Current Status:** Phase 18 (Integration & Hardening) In Progress

**Protocol v1 Implementation Complete:**
- ✅ X3DH + Double Ratchet (forward secrecy + post-compromise security)
- ✅ Multi-device support with sync and revocation
- ✅ Private groups with Sender Keys
- ✅ Public groups & channels with moderation
- ✅ Offline message delivery (mailbox protocol)
- ✅ Voice/video calls (WebRTC signaling)
- ✅ Reputation system (trust scores, attestations)

**Current Focus:** Testing, bug fixes, performance optimization, and addressing known limitations.

### Documentation

| Document | Description |
|----------|-------------|
| [`specs/roadmap.md`](specs/roadmap.md) | Main implementation roadmap (Phases 0-18) |
| [`specs/limitations-roadmap.md`](specs/limitations-roadmap.md) | Known limitations & UX improvements |
| [`specs/security-roadmap.md`](specs/security-roadmap.md) | Censorship resistance & security (10 phases) |
| [`specs/testing.md`](specs/testing.md) | Testing strategy & test plans |
| [`specs/protocol-v2.md`](specs/protocol-v2.md) | Protocol v1 specification |

## Quick Start

### Build

```bash
# Clone the repository
git clone <repository-url>
cd babylontower

# Build the application
make build

# Run the application
./bin/messenger
```

### First Launch

On first launch, Babylon Tower will:
1. Generate a new identity (BIP39 mnemonic)
2. Display your mnemonic phrase - **write this down safely!**
3. Show your public key - share this with contacts

```
╔══════════════════════════════════════════╗
║     🏰  Babylon Tower v0.1.0-poc          ║
║     Decentralized P2P Messenger          ║
╚══════════════════════════════════════════╝

Your public key: 3jYEF...

Type /help for available commands.
```

## Usage

### Basic Commands

```
/help                          - Show all available commands
/myid                          - Display your public key
/add <pubkey> [nickname]       - Add a contact
/list                          - List all contacts
/chat <contact>                - Enter chat mode with a contact
/history <contact> [limit]     - Show message history
/exit                          - Exit the application
```

### Adding a Contact

1. Get your contact's public key (they run `/myid`)
2. Add them: `/add <pubkey> [nickname]`

```
>>> /add 3jYEF... Alice
✅ Contact added: Alice
```

### Starting a Chat

```
>>> /list

=== Contacts ===
[1] Alice - 3jYEF...
[2] Bob - 5kLmn...
================

>>> /chat 1

━━━ Chat with Alice ━━━
Public key: 3jYEF...
Type your message and press Enter to send.
Press Enter on an empty line to exit chat.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

>>> Hello Alice!
[2026-02-21 10:30:00] You: Hello Alice!

>>> 
Exited chat mode.
```

### Viewing History

```
>>> /history Alice 10

=== History with Alice ===
[2026-02-21 10:30:00] You: [Encrypted message received]
[2026-02-21 10:30:15] Alice: [Encrypted message received]
==========================
```

## Project Status

### PoC (Unversioned) - Complete

| Phase | Module | Status |
|-------|--------|--------|
| 0 | Project Setup | ✅ Complete |
| 1 | Identity & Cryptography | ✅ Complete |
| 2 | Storage Layer | ✅ Complete |
| 3 | IPFS Node Integration | ✅ Complete |
| 4 | Messaging Protocol | ✅ Complete |
| 5 | CLI Interface | ✅ Complete |
| 6-7 | Integration & Release | ❌ Skipped (moved to Protocol v1) |

### Protocol v1 - Implementation Complete

| Phase | Module | Status |
|-------|--------|--------|
| 8 | Identity v1 | ✅ Complete |
| 9 | X3DH & Double Ratchet | ✅ Complete |
| 10 | Protocol v1 Wire Format | ✅ Complete |
| 11 | Multi-Device | ✅ Complete |
| 12 | Private Groups | ✅ Complete |
| 13 | Public Groups & Channels | ✅ Complete |
| 14 | Offline Delivery | ✅ Complete |
| 15 | Voice & Video Calls | ✅ Complete |
| 16 | Group Calls | ✅ Complete |
| 17 | Reputation System | ✅ Complete |
| 18 | Integration & Hardening | 🔄 In Progress |

### Security Roadmap - Censorship Resistance

| Phase | Module | Status |
|-------|--------|--------|
| 1 | Configurable Bootstrap | ⏹️ Pending |
| 2 | Peer Exchange Protocol | ⏹️ Pending |
| 3 | Transport Obfuscation | ⏹️ Pending |
| 4 | Peer Scoring & Anti-Eclipse | ⏹️ Pending |
| 5 | DHT Privacy | ⏹️ Pending |
| 6 | User-Hosted Peer Lists | ⏹️ Pending |
| 7 | Social Network Bootstrap | ⏹️ Pending |
| 8 | Mesh Networking | ⏹️ Pending |
| 9 | User-Run Relay Infrastructure | ⏹️ Pending |
| 10 | Private Relay Networks | ⏹️ Pending |

**See [`specs/roadmap.md`](specs/roadmap.md) for detailed status.**

## Architecture

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
                          ┌────────▼─────────┐
                          │   Storage       │
                          │  (BadgerDB)     │
                          └─────────────────┘
```

### Protocol v1 (Target)

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
┌──────────────────────────┬───────────────────────────┐
│                   Identity v1                         │
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

## Known Limitations

### Current Limitations

| Category | Limitation | Status |
|----------|-----------|--------|
| **Security** | Local storage unencrypted | ⏹️ Phase 19 |
| **Security** | Metadata privacy (traffic analysis) | ⏹️ Security Phases 3, 5 |
| **Connectivity** | NAT traversal (symmetric NATs) | ⏹️ Security Phase 9 |
| **UX** | Contact verification (safety numbers) | ⏹️ Phase 19 |
| **UX** | Message search | ⏹️ Phase 19 |
| **UX** | GUI (desktop/mobile) | ⏹️ Future |
| **UX** | Backup/restore | ⏹️ Phase 19 |

### Resolved in Protocol v1

- ✅ **Forward secrecy**: X3DH + Double Ratchet implemented
- ✅ **Group chat**: Private groups with Sender Keys
- ✅ **Offline messages**: Mailbox protocol implemented
- ✅ **Multi-device**: Full support with sync and revocation
- ✅ **Group calls**: Mesh + SFU architecture
- ✅ **Reputation**: Trust scores and attestations

### Censorship Resistance Strategy

Babylon Tower maintains **node equality** - no special infrastructure required:

- **No bridge nodes**: Any node can perform any function
- **User-run relays**: Voluntary infrastructure, no special privileges
- **Configurable bootstrap**: Multiple discovery channels (env, file, URL, DNS, QR)
- **Mesh networking**: Works without internet via Bluetooth/WiFi Direct
- **Transport obfuscation**: Defeats DPI fingerprinting

**See [`specs/limitations-roadmap.md`](specs/limitations-roadmap.md) for comprehensive limitations and [`specs/security-roadmap.md`](specs/security-roadmap.md) for censorship resistance roadmap.**

## Testing

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run CLI tests only
go test ./pkg/cli/... -v
```

## Project Structure

```
babylontower/
├── cmd/
│   └── messenger/          # Application entry point
├── pkg/
│   ├── identity/           # Identity and key management
│   ├── crypto/             # Cryptographic operations
│   ├── storage/            # BadgerDB storage layer
│   ├── ipfsnode/           # Embedded IPFS node
│   ├── messaging/          # Messaging protocol
│   ├── cli/                # Command-line interface
│   └── proto/              # Generated protobuf code
├── proto/
│   └── message.proto       # Protobuf definitions
├── configs/                # Configuration management
├── internal/
│   └── testutil/           # Test utilities
├── specs/                  # Technical specifications
├── Makefile
└── go.mod
```

## License

See [LICENSE](LICENSE)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, build instructions, and contribution guidelines.
