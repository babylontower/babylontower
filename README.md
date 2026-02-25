# Babylon Tower

A decentralized, secure peer-to-peer messenger that operates without central servers.

## Overview

Babylon Tower is a peer-to-peer messaging implementation using:

- **End-to-end encryption** (XChaCha20-Poly1305, Ed25519 signatures)
- **IPFS** for decentralized communication via PubSub
- **BadgerDB** for local message and contact storage
- **Go** for a single, portable binary
- **Interactive CLI** for user interaction

## ⚠️ Development Status

**Current PoC (Unversioned):** The existing implementation is a functional proof-of-concept that demonstrates the core architecture. It is **not production-ready** and will not be released as a standalone version.

**Protocol v1 (In Planning):** Development has shifted to implementing Protocol v1 (specified in `specs/protocol-v2.md`), which adds:
- X3DH + Double Ratchet (forward secrecy + post-compromise security)
- Multi-device support
- Private/public groups and channels
- Offline message delivery (mailbox protocol)
- Voice/video calls
- Reputation system

See [`specs/roadmap.md`](specs/roadmap.md) for the complete implementation plan.

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
| 6 | Integration & Testing | ❌ Skipped |
| 7 | Release Preparation | ❌ Skipped |

### Protocol v1 (Target) - Planning

| Phase | Module | Status |
|-------|--------|--------|
| 8 | Identity v1 | ⏹️ Pending |
| 9 | X3DH & Double Ratchet | ⏹️ Pending |
| 10 | Protocol v1 Wire Format | ⏹️ Pending |
| 11 | Multi-Device | ⏹️ Pending |
| 12 | Private Groups | ⏹️ Pending |
| 13 | Public Groups & Channels | ⏹️ Pending |
| 14 | Offline Delivery | ⏹️ Pending |
| 15 | Voice & Video Calls | ⏹️ Pending |
| 16 | Group Calls | ⏹️ Pending |
| 17 | Reputation System | ⏹️ Pending |
| 18 | Integration & Hardening | ⏹️ Pending |

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

## Known Limitations (Current PoC)

- **NAT traversal**: Not implemented; assumes direct connectivity
- **Local storage encryption**: Not encrypted (future enhancement)
- **Offline messages**: No queuing (both parties must be online)
- **Group chat**: Not supported (1:1 messaging only)
- **Forward secrecy**: Uses static keys only (no Double Ratchet) - **Protocol v1 will add X3DH + Double Ratchet**
- **Contact X25519 keys**: Not stored with contacts (requires manual exchange)

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
