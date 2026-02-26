# Babylon Tower - Known Limitations Roadmap

## Overview

This document tracks all known limitations, technical debt, and areas for improvement in Babylon Tower. Each limitation is categorized by severity, affected component, and planned resolution approach.

**Purpose:** Provide transparency about current limitations and a clear roadmap for addressing them in future development phases.

---

## Limitation Categories

### Severity Levels

| Level | Description | Timeline |
|-------|-------------|----------|
| **Critical** | Security vulnerabilities, data loss risks | Immediate (next release) |
| **High** | Major functionality gaps, poor UX | Short-term (1-2 phases) |
| **Medium** | Missing features, performance issues | Medium-term (3-5 phases) |
| **Low** | Nice-to-have improvements | Long-term (future) |

### Resolution Strategies

| Strategy | Description |
|----------|-------------|
| **Implement** | Build the missing functionality |
| **Improve** | Enhance existing implementation |
| **Document** | Add documentation/workarounds |
| **Deprecate** | Remove in favor of better approach |
| **Research** | Investigate solutions (spike) |

---

## Critical Limitations

### C1: Local Storage Unencrypted

**Status:** ⏹️ Pending (Phase 19+)  
**Severity:** Critical  
**Component:** `pkg/storage`  
**Impact:** Private keys, messages, and contacts stored in plaintext on disk

**Description:**
The BadgerDB database stores all data unencrypted, including:
- Identity mnemonic and private keys in `identity.json`
- All message content
- Contact information
- Session state (ratchet keys)

An attacker with filesystem access can extract sensitive data.

**Current Workaround:**
- Rely on OS-level disk encryption (FileVault, BitLocker, LUKS)
- Secure file permissions (0600 for identity.json)

**Planned Resolution:**
1. Derive encryption key from master secret (HKDF)
2. Encrypt BadgerDB with XChaCha20-Poly1305
3. Encrypt identity.json separately
4. Zero-knowledge design: encryption keys never leave memory

**Implementation Plan:**
```go
// Phase 19: Storage Encryption
type EncryptedStorage struct {
    db *badger.DB
    encryptionKey []byte // Derived from master secret
}

func NewEncryptedStorage(path string, masterSecret []byte) (*EncryptedStorage, error) {
    // Derive key: HKDF(masterSecret, "storage-encryption", ...)
    // Initialize BadgerDB with encryption
}
```

**Dependencies:** None  
**Estimated Effort:** 3-5 days  
**Risk:** Medium (key management complexity)

---

### C2: No Metadata Privacy

**Status:** ⏹️ Pending (Security Roadmap Phase 5+)  
**Severity:** Critical  
**Component:** `pkg/ipfsnode`, `pkg/messaging`  
**Impact:** Network observers can see who communicates with whom

**Description:**
While message content is encrypted, metadata is visible:
- Topic names reveal communication patterns
- IPFS peer IDs can be tracked
- Message timing and size patterns
- DHT queries expose identity lookups

**Current Workaround:**
- None at protocol level

**Planned Resolution:**

**Censorship Resistance** (Security Roadmap):
- Phase 3: Transport obfuscation (defeats traffic analysis)
- Phase 5: DHT privacy with cover traffic

**UX/Privacy Features** (This roadmap):
- Rendezvous points for shared topics
- Topic padding to fixed sizes

**Dependencies:** Security Roadmap Phases 3, 5  
**Estimated Effort:** 10-15 days (UX features only, excluding censorship resistance)  
**Risk:** High (complexity, performance impact)

---

## High Priority Limitations

### H1: NAT Traversal Limited

**Status:** ⚠️ Partial (AutoNAT + hole punching implemented)  
**Severity:** High  
**Component:** `pkg/ipfsnode`  
**Impact:** Nodes behind symmetric NATs cannot connect directly

**Description:**
Current NAT traversal relies on:
- AutoNAT for NAT type detection
- libp2p hole punching

However, symmetric NATs (common in corporate networks, mobile) still block direct connections.

**Current Workaround:**
- Manual port forwarding
- Run on same local network

**Planned Resolution:**

**User-Run Infrastructure** (voluntary, no special status):
1. **UPnP/NAT-PMP:** Automatic port mapping (user-enabled)
2. **WebSocket Fallback:** Use WebSocket transport for restrictive NATs
3. **QUIC Transport:** Better NAT traversal via UDP
4. **Community Relays:** Users voluntarily run relay nodes (no special privileges)
5. **TURN Servers:** For WebRTC calls (user-run or community-run)

**Key Principle:** Any user can run relay/TURN infrastructure, but relay nodes have **no special status** in the protocol - they're just regular nodes that happen to forward traffic.

**Implementation Plan:**
```go
// Phase 19: Improved NAT Traversal
config := libp2p.Config{
    EnableRelay: true,        // Any node can enable
    EnableAutoRelay: true,    // Auto-detect if behind NAT
    EnableHolePunching: true,
    EnableUPnP: true,
    EnableNATPMP: true,
    Transports: []Transport{
        TCP,
        WebSocket, // Fallback for restrictive networks
        QUIC,      // UDP-based, better for NAT traversal
    },
}

// User configuration - voluntary relay operation
type RelayConfig struct {
    EnableRelayService bool  // User chooses to help others
    MaxBandwidth       int   // User-set limits
    AllowedPeers       []peer.ID // Optional: only help specific peers
}

// Relay node has no special privileges
// Just forwards encrypted traffic
// Cannot read message content
```

**Dependencies:** None  
**Estimated Effort:** 2-3 days (configuration + testing)  
**Risk:** Low

**Community Coordination:**
- Wiki page for users to volunteer relay capacity
- No central registry - peers advertised via DHT
- Users can run privately (for friends) or publicly

---

### H2: IPFS Get Not Fully Implemented

**Status:** ⚠️ Partial (works for small blocks)  
**Severity:** High  
**Component:** `pkg/ipfsnode`  
**Impact:** Cannot reliably fetch large envelopes from IPFS by CID

**Description:**
The `Get()` function in IPFS node has limitations:
- Works for small blocks (<1MB)
- Fails for large blocks or when peer disconnected
- No chunking or sharding implemented

**Current Workaround:**
- Direct PubSub message passing (bypasses IPFS Get)
- Store messages locally, share CID only

**Planned Resolution:**
1. **Chunking:** Split large messages into chunks
2. **Pinning:** Pin important blocks to prevent GC
3. **Distributed Pinning:** Use IPFS Cluster for redundancy
4. **Complete Get Implementation:** Proper block traversal

**Implementation Plan:**
```go
// Phase 19: IPFS Get Completion
func (n *Node) Get(ctx context.Context, cid string) ([]byte, error) {
    // Parse CID
    // Fetch block from blockstore
    // If large block, reassemble from chunks
    // Return data
}

// Chunking for large messages
func ChunkMessage(data []byte, maxSize int) ([]*Chunk, error) {
    // Split into chunks
    // Create Merkle DAG
    // Return root CID
}
```

**Dependencies:** None  
**Estimated Effort:** 3-4 days  
**Risk:** Medium

---

### H3: No Forward Secrecy in PoC

**Status:** ✅ Resolved in Protocol v1 (Phase 9)  
**Severity:** High  
**Component:** `pkg/messaging` (PoC)  
**Impact:** Compromised long-term key exposes all past messages

**Description:**
The PoC messaging protocol uses static X25519 keys:
- Same shared secret for all messages
- No ratcheting
- Past messages decryptable if key compromised

**Resolution:**
Protocol v1 implements X3DH + Double Ratchet:
- Ephemeral keys for each message
- Ratcheting provides forward secrecy
- Post-compromise security via DH ratchet

**Status:** ✅ Complete (Phases 9, 10)

---

### H4: Contact Verification Missing

**Status:** ⏹️ Pending (Phase 19)  
**Severity:** High  
**Component:** `pkg/cli`, `pkg/identity`  
**Impact:** No protection against man-in-the-middle attacks

**Description:**
Users cannot verify they're communicating with the intended party:
- No safety number comparison (like Signal)
- No QR code for in-person verification
- No key change notifications

**Current Workaround:**
- Manual fingerprint comparison over trusted channel

**Planned Resolution:**
1. **Safety Numbers:** Generate short authentication string from session keys
2. **QR Codes:** Display identity fingerprint as QR code
3. **Key Change Alerts:** Notify when contact's key changes
4. **Trust Levels:** Visual indicators for verification status

**Implementation Plan:**
```go
// Phase 19: Contact Verification
func GenerateSafetyNumber(localKey, remoteKey []byte) string {
    // SHA256(concat(localKey, remoteKey))
    // Format as 60-digit number (like Signal)
    // Display as 12 groups of 5 digits
}

// QR code generation
func GenerateIdentityQR(fingerprint string) (image.Image, error)
```

**Dependencies:** None  
**Estimated Effort:** 2-3 days  
**Risk:** Low

---

## Medium Priority Limitations

### M1: mDNS Fails in Containerized Environments

**Status:** ⚠️ Known Issue  
**Severity:** Medium  
**Component:** `pkg/ipfsnode`  
**Impact:** Integration tests require manual peer connections in CI

**Description:**
mDNS discovery fails in Docker/containerized environments due to:
- UDP multicast (224.0.0.251:5353) blocked by Docker network isolation
- Missing D-Bus for avahi-daemon
- Firewall rules in containerized environments

**Current Workaround:**
- Use explicit peer connections in tests
- Host network mode in Docker

**Planned Resolution:**
- See **Security Roadmap Phase 1** for configurable bootstrap (file-based, environment variable)
- This is addressed as part of broader censorship resistance work

**Dependencies:** Security Roadmap Phase 1  
**Estimated Effort:** Included in Security Roadmap  
**Risk:** Low

---

### M2: No Message Reactions or Edits

**Status:** ⏹️ Planned (Protocol v1, Phase 10)  
**Severity:** Medium  
**Component:** `pkg/messaging`, `pkg/proto`  
**Impact:** Limited message interaction capabilities

**Description:**
Protocol v1 protobuf definitions include support for:
- Reactions (emoji responses to messages)
- Message edits (with edit history)
- Message deletes (tombstones)

But implementation is incomplete.

**Current Workaround:**
- Send new message with correction

**Planned Resolution:**
1. **Reactions:** Implement reaction payload handling
2. **Edits:** Store edit history, display edited indicator
3. **Deletes:** Soft delete with tombstone marker

**Status:** 🔄 In Progress (Phase 10 - protobuf defined, logic pending)

---

### M3: Limited Message Types

**Status:** ⏹️ Partial  
**Severity:** Medium  
**Component:** `pkg/messaging`  
**Impact:** Only text messages fully supported

**Description:**
Protocol v1 defines message types for:
- Text (✅ Complete)
- Media (images, video, audio) - ⏹️ Pending
- Location sharing - ⏹️ Pending
- Contact sharing - ⏹️ Pending
- Voice messages - ⏹️ Pending

**Current Workaround:**
- Share links to external storage

**Planned Resolution:**
1. **Media Messages:** Encrypt and upload to IPFS, share CID
2. **Location:** Encrypted coordinates with map preview
3. **Contacts:** Share identity document reference
4. **Voice:** Encrypted audio blobs

**Dependencies:** IPFS Get completion (H2)  
**Estimated Effort:** 5-7 days  
**Risk:** Medium

---

### M4: No Read Receipts

**Status:** ⏹️ Planned (Protocol v1)  
**Severity:** Medium  
**Component:** `pkg/messaging`  
**Impact:** Users cannot see if messages were read

**Description:**
Protocol v1 includes receipt message types but implementation is incomplete.

**Current Workaround:**
- None

**Planned Resolution:**
1. **Read Receipts:** Send receipt on message display
2. **Delivery Receipts:** Send receipt on message receipt
3. **Privacy Controls:** Option to disable receipts
4. **Group Receipts:** Aggregate receipts in groups

**Status:** 🔄 In Progress (Phase 10 - protobuf defined)

---

### M5: Group Member Limit

**Status:** ⏹️ Known Constraint  
**Severity:** Medium  
**Component:** `pkg/groups`  
**Impact:** Groups optimized for <100 members

**Description:**
Current group implementation:
- Member removal triggers full key rotation (O(n) re-encryption)
- Sender Keys distributed to all members
- Performance degrades with large groups

**Current Workaround:**
- Use channels for broadcast (1:many)
- Split large groups

**Planned Resolution:**
1. **Lazy Key Distribution:** Distribute keys on-demand
2. **Hierarchical Groups:** Subgroups with key aggregation
3. **Channel-Based Groups:** Hybrid approach for large groups

**Dependencies:** None  
**Estimated Effort:** 5-7 days  
**Risk:** Medium

---

### M6: No Message Search

**Status:** ⏹️ Pending (Phase 19)  
**Severity:** Medium  
**Component:** `pkg/storage`, `pkg/cli`  
**Impact:** Cannot search message history

**Description:**
BadgerDB stores messages but no search functionality:
- No full-text search
- No filtering by date, sender, type
- No message export

**Current Workaround:**
- Manual scrolling through history

**Planned Resolution:**
1. **Full-Text Index:** Index message content (encrypted)
2. **Search API:** Query by keyword, date range, contact
3. **Export:** Export conversation to JSON/text

**Implementation Plan:**
```go
// Phase 19: Message Search
type MessageIndex struct {
    db *badger.DB
    index *bleve.Index // Full-text search
}

func (m *MessageIndex) Search(query string, contact string) ([]Message, error)
```

**Dependencies:** None  
**Estimated Effort:** 3-4 days  
**Risk:** Low

---

## Low Priority Limitations

### L1: No GUI

**Status:** ⏹️ Future Enhancement  
**Severity:** Low  
**Component:** CLI  
**Impact:** Limited to command-line interface

**Description:**
Application only has CLI interface:
- No desktop GUI
- No mobile app
- Steep learning curve for non-technical users

**Current Workaround:**
- Use CLI with `/help` for guidance

**Planned Resolution:**
1. **Desktop GUI:** Tauri or Electron app
2. **Mobile App:** React Native or Flutter
3. **Web Interface:** PWA for browser access

**Dependencies:** Protocol v1 stable  
**Estimated Effort:** 30-50 days  
**Risk:** High (UI/UX complexity)

---

### L2: No Backup/Restore

**Status:** ⏹️ Pending (Phase 19)  
**Severity:** Low  
**Component:** `pkg/identity`, `pkg/storage`  
**Impact:** Data loss if device fails

**Description:**
No automated backup mechanism:
- Manual backup of `~/.babylontower/`
- No cloud backup integration
- No encrypted backup format

**Current Workaround:**
- Manual file backup
- Save mnemonic securely

**Planned Resolution:**
1. **Encrypted Backup:** Export to encrypted archive
2. **Cloud Integration:** Optional backup to IPFS, S3, etc.
3. **Social Recovery:** Split backup among trusted contacts
4. **Incremental Backups:** Only backup changes

**Dependencies:** Storage encryption (C1)  
**Estimated Effort:** 3-5 days  
**Risk:** Low

---

### L3: No Push Notifications

**Status:** ⏹️ Future Enhancement  
**Severity:** Low  
**Component:** `pkg/cli`, `pkg/mailbox`  
**Impact:** Must keep app running to receive messages

**Description:**
No push notification support:
- App must be running to receive messages
- No integration with system notifications
- Mobile would require Firebase/APNs

**Current Workaround:**
- Keep app running in background
- Use mailbox for offline delivery

**Planned Resolution:**

**Desktop Notifications** (This roadmap):
- System tray integration
- Desktop notifications

**Mesh Networking** (Security Roadmap Phase 8):
- Local mesh can maintain connectivity without constant internet
- Messages can be delivered via mesh when internet returns

**Dependencies:** Mailbox (Phase 14 ✅)  
**Estimated Effort:** 5-7 days (desktop), 3-4 weeks (mesh - Security Roadmap)  
**Risk:** Medium

---

### L4: Limited Analytics/Metrics

**Status:** ⚠️ Partial  
**Severity:** Low  
**Component:** `pkg/ipfsnode`  
**Impact:** Hard to diagnose performance issues

**Description:**
Limited observability:
- Basic network metrics in IPFS node
- No application-level metrics
- No performance profiling
- No error tracking

**Current Workaround:**
- Manual logging
- `/network` command for basic stats

**Planned Resolution:**
1. **Metrics Collection:** Prometheus-style metrics
2. **Distributed Tracing:** OpenTelemetry integration
3. **Error Reporting:** Sentry-like error tracking
4. **Performance Dashboard:** Real-time monitoring

**Dependencies:** None  
**Estimated Effort:** 3-5 days  
**Risk:** Low

---

### L5: No Plugin System

**Status:** ⏹️ Future Enhancement  
**Severity:** Low  
**Component:** Architecture  
**Impact:** Cannot extend functionality

**Description:**
No plugin or extension mechanism:
- Cannot add custom commands
- No third-party integrations
- Monolithic architecture

**Current Workaround:**
- Modify source code

**Planned Resolution:**
1. **Plugin API:** Define plugin interface
2. **Command Extensions:** Allow custom `/commands`
3. **Webhook Support:** Integrate with external services

**Dependencies:** Protocol v1 stable  
**Estimated Effort:** 7-10 days  
**Risk:** Medium (security concerns)

---

## Technical Debt

### TD1: Inconsistent Error Handling

**Status:** ⚠️ Known Issue  
**Severity:** Medium  
**Component:** All packages  
**Impact:** Hard to debug, inconsistent UX

**Description:**
Error handling varies across codebase:
- Some functions return errors, some panic
- Inconsistent error wrapping
- Some errors logged, some silent

**Planned Resolution:**
1. **Error Handling Policy:** Define standards
2. **Error Wrapping:** Use `fmt.Errorf("%w")` consistently
3. **Error Types:** Define sentinel errors
4. **Logging Policy:** Consistent log levels

**Dependencies:** None  
**Estimated Effort:** 2-3 days  
**Risk:** Low

---

### TD2: Insufficient Test Coverage in Service Layers

**Status:** ⚠️ Known Gap  
**Severity:** Medium  
**Component:** `pkg/messaging`, `pkg/ipfsnode`  
**Impact:** Potential bugs in integration points

**Description:**
Test coverage varies:
- Core crypto: >90% ✅
- Service layers: ~30-70% ⚠️
- Integration tests: Partial

**Planned Resolution:**
1. **Phase 18 Focus:** Improve service layer coverage
2. **Integration Tests:** Add more end-to-end tests
3. **Test Automation:** CI/CD integration

**Dependencies:** None  
**Estimated Effort:** 5-7 days  
**Risk:** Low

---

### TD3: Documentation Gaps

**Status:** ⚠️ Known Issue  
**Severity:** Medium  
**Component:** Documentation  
**Impact:** Hard for contributors to onboard

**Description:**
Missing documentation:
- API documentation
- Architecture decision records (ADRs)
- Deployment guides
- Troubleshooting guides

**Planned Resolution:**
1. **API Docs:** Generate from code comments
2. **ADRs:** Document key decisions
3. **User Guide:** Comprehensive usage guide
4. **Contributor Guide:** Onboarding for developers

**Dependencies:** None  
**Estimated Effort:** 3-5 days  
**Risk:** Low

---

### TD4: Code Duplication

**Status:** ⚠️ Known Issue  
**Severity:** Low  
**Component:** Multiple packages  
**Impact:** Maintenance burden

**Description:**
Some code duplication exists:
- Similar functions in multiple packages
- PoC and Protocol v1 coexist
- Shared utilities not extracted

**Planned Resolution:**
1. **Refactoring:** Extract common utilities
2. **Deprecate PoC:** Remove PoC code after v1 stable
3. **Code Review:** Enforce DRY in reviews

**Dependencies:** Protocol v1 stable  
**Estimated Effort:** 3-5 days  
**Risk:** Low

---

## Limitation Tracking

### Status Legend

| Symbol | Meaning |
|--------|---------|
| ✅ | Resolved |
| 🔄 | In Progress |
| ⏹️ | Pending |
| ⚠️ | Partial/Workaround Available |
| ❌ | Won't Fix (for now) |

### Priority Matrix

```
                    Impact
            Low     Medium     High     Critical
      +----------------------------------------+
      |                                        |
Easy  |  L1, L5   M6, TD3    H4, M1    C1     |
      |                                        |
      |  L4       M2, M3     H1, H2    C2     |
Med   |            TD2                        |
      |                                        |
      |  L2       M5         TD1              |
Hard  |                                        |
      |  L3       TD4                          |
      +----------------------------------------+
```

### Phase Allocation

**Main Roadmap (Protocol Features):**
| Phase | Limitations Addressed |
|-------|----------------------|
| **Phase 18** | TD2 (test coverage), H3 (forward secrecy - complete) |
| **Phase 19** | C1 (storage encryption), H4 (contact verification), M6 (search), L2 (backup), TD1 (error handling) |
| **Phase 20** | M2 (reactions/edits), M3 (media types), M4 (read receipts) |
| **Phase 21** | M5 (large groups) |
| **Phase 22+** | L1 (GUI), L4 (analytics), L5 (plugin system) |

**Security Roadmap (Censorship Resistance):**
| Phase | Limitations Addressed |
|-------|----------------------|
| **Security Phase 1** | M1 (mDNS/container bootstrap) |
| **Security Phase 3** | C2 (metadata privacy - transport obfuscation) |
| **Security Phase 5** | C2 (metadata privacy - DHT cover traffic) |
| **Security Phase 8** | L3 (push notifications - via mesh networking) |
| **Security Phase 9** | H1 (NAT traversal - user-run relays) |
| **Security Phase 10** | H1 (private relay networks for trusted groups) |

**Note:** C2 (Metadata Privacy) requires collaboration between both roadmaps - censorship resistance (Security Roadmap) and UX privacy features (this roadmap).

---

## Contributing

If you encounter a limitation not listed here, please:

1. **Check Existing Issues:** Search GitHub issues
2. **Create Issue:** Document the limitation with:
   - Description
   - Impact
   - Workaround (if any)
   - Proposed solution
3. **Label:** Add `limitation` label
4. **Prioritize:** Team will assign severity and phase

---

*Last updated: February 26, 2026*
*Version: 1.0*
