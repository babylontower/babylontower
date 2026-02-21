# Babylon Tower

A decentralized, secure peer-to-peer messenger that operates without central servers.

## Overview

Babylon Tower is a Proof of Concept (PoC) implementation of a serverless messaging system using:

- **End-to-end encryption** (XChaCha20-Poly1305, Ed25519 signatures)
- **IPFS** for decentralized communication via PubSub
- **BadgerDB** for local message and contact storage
- **Go** for a single, portable binary
- **Interactive CLI** for user interaction

## ⚠️ Initial Development Phase

This project is in **early development**. The PoC is functional but:

- Features and architecture may change significantly
- Security has not been audited
- Not suitable for production use
- Some limitations (e.g., NAT traversal) are documented as known issues

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

| Phase | Module | Status |
|-------|--------|--------|
| 0 | Project Setup | ✅ Complete |
| 1 | Identity & Cryptography | ✅ Complete |
| 2 | Storage Layer | ✅ Complete |
| 3 | IPFS Node Integration | ✅ Complete |
| 4 | Messaging Protocol | ✅ Complete |
| 5 | CLI Interface | ✅ Complete |
| 6 | Integration & Testing | 🔄 In Progress |
| 7 | Release Preparation | ⏳ Pending |

See [`specs/roadmap.md`](specs/roadmap.md) for the complete implementation plan.

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

## Known Limitations (PoC)

- **NAT traversal**: Not implemented; assumes direct connectivity
- **Local storage encryption**: Not encrypted (future enhancement)
- **Offline messages**: No queuing (both parties must be online)
- **Group chat**: Not supported (1:1 messaging only)
- **Forward secrecy**: Uses static keys only (no Double Ratchet)
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
