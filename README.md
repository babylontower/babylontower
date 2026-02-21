# Babylon Tower

A decentralized, secure peer-to-peer messenger that operates without central servers.

## Overview

Babylon Tower is a Proof of Concept (PoC) implementation of a serverless messaging system using:

- **End-to-end encryption** (XChaCha20-Poly1305, Ed25519 signatures)
- **IPFS** for decentralized communication via PubSub
- **BadgerDB** for local message and contact storage
- **Go** for a single, portable binary

## ⚠️ Initial Development Phase

This project is in **early development**. The PoC is functional but:

- Features and architecture may change significantly
- Security has not been audited
- Not suitable for production use
- Some limitations (e.g., NAT traversal) are documented as known issues

## Project Status

See [`specs/roadmap.md`](specs/roadmap.md) for the complete implementation plan and current progress.

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
