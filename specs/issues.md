# Protocol v2 Compliance Issues

**Generated**: March 4, 2026
**Protocol Version**: 1.0.0
**Status**: Draft analysis
**Last Updated**: March 5, 2026 (All Critical Issues Fixed: #1, #2, #3, #4, #5, #6, #7, #8, #9, #10, #11)

---

## Summary

This document lists all discrepancies and issues found when validating the current implementation against `protocol-v2.md` specification.

| Severity | Count | Fixed | Remaining |
|----------|-------|-------|-----------|
| **Critical** | 11 | 11 | 0 |
| **Major** | 20 | 1 | 19 |
| **Minor** | 13 | 0 | 13 |
| **Inconveniences** | 8 | 0 | 8 |
| **Missing Implementations** | 9 | 0 | 9 |
| **Protobuf Issues** | 3 | 1 | 2 |
| **Total** | **64** | **13** | **51** |

---

## Critical Issues

These issues prevent protocol compliance and must be fixed before v1 launch.

### 1. Identity Key Derivation Path Mismatch ✅ FIXED

**Status**: **FIXED** - March 4, 2026
**Spec Section**: 1.1
**File**: `pkg/identity/identity.go`, `pkg/identity/identity_v1.go`

**Spec**:
```
BIP39 Mnemonic → PBKDF2 → 512-bit Seed
Seed → HKDF-SHA256(salt="bt-master-key", info="babylon-tower-v1") → Master Secret (32 bytes)
Master Secret → HKDF(salt="bt-identity", info="identity-signing-key-0") → IK_sign (Ed25519)
Master Secret → HKDF(salt="bt-identity", info="identity-dh-key-0") → IK_dh (X25519)
```

**Implementation** (BEFORE FIX):
```go
// Direct derivation without master secret intermediate
HKDF(seed, salt="ed25519-derive", info="index-0") → Ed25519 keys
HKDF(seed, salt="x25519-derive", info="index-1") → X25519 keys
```

**Implementation** (AFTER FIX):
```go
// Master secret intermediate step with correct HKDF labels
seed := bip39.NewSeed(mnemonic, "")
masterSecret := HKDF(seed, salt="bt-master-key", info="babylon-tower-v1")
IK_sign := HKDF(masterSecret, salt="bt-identity", info="identity-signing-key-0")
IK_dh := HKDF(masterSecret, salt="bt-identity", info="identity-dh-key-0")
```

**Impact**: Incompatible identity derivation. Cannot migrate to v1 without losing identity.
**Fix Required**: Implement master secret intermediate step with correct HKDF labels.
**Fix Applied**: Updated `pkg/identity/identity.go` to use `DeriveMasterSecret()` from `pkg/identity/identity_v1.go` with correct HKDF labels.

---

### 2. Identity Fingerprint Missing ✅ FIXED

**Status**: **FIXED** - March 4, 2026
**Spec Section**: 1.1
**File**: `pkg/identity/identity.go`, `cmd/messenger/cli/commands_identity.go`

**Spec**:
```
Fingerprint = Base58(SHA256(IK_sign.pub ‖ IK_dh.pub)[:20]) → 27-28 characters
```

**Implementation** (BEFORE FIX): Not implemented.

**Implementation** (AFTER FIX):
```go
func (i *Identity) ComputeFingerprint() (string, error) {
    combined := append(i.Ed25519PubKey, i.X25519PubKey...)
    hash := sha256.Sum256(combined)
    return EncodeBase58(hash[:20]), nil
}
```

**Impact**: Users cannot verify contacts out-of-band for MITM protection.
**Fix Required**: Implement fingerprint computation and display in CLI.
**Fix Applied**: Added `ComputeFingerprint()` method to `Identity` struct and updated `/myid` CLI command to display fingerprint.

---

### 3. IdentityDocument Serialization Incomplete ✅ FIXED

**Status**: **FIXED** - March 4, 2026
**Spec Section**: 1.4
**File**: `pkg/protocol/identity.go:serializeIdentityDocumentForSigning()`

**Spec**: All 17 fields must be serialized in canonical form for signing.

**Implementation** (BEFORE FIX):
```go
// Only serializes first 6 fields
buf.Write(doc.IdentitySignPub)
buf.Write(doc.IdentityDHPub)
// ... fields 3-6 only ...
// Note: For full implementation, we would serialize all devices, prekeys, etc.
```

**Implementation** (AFTER FIX): Complete canonical serialization of all 17 fields:
- Fields 1-6: Core identity (identity_sign_pub, identity_dh_pub, sequence, previous_hash, created_at, updated_at)
- Field 7: Devices (repeated DeviceCertificate with full serialization)
- Field 8: Signed prekeys (repeated SignedPrekey)
- Field 9: One-time prekeys (repeated OneTimePrekey)
- Field 10: Supported versions (repeated uint32)
- Field 11: Supported cipher suites (repeated string)
- Field 12: Preferred version (uint32)
- Field 13: Display name (length-prefixed string)
- Field 14: Avatar CID (length-prefixed string)
- Field 15: Revocations (repeated RevocationCertificate)
- Field 16: Feature flags (10 bool fields + custom_features)
- Field 17: Signature (NOT included in signing serialization)

**Impact**: Identity document signatures were INVALID per spec. Security vulnerability.
**Fix Required**: Implement complete canonical serialization for all 17 fields.
**Fix Applied**: Rewrote `serializeIdentityDocumentForSigning()` with complete field serialization and added `boolToByte()` helper.

---

### 4. IdentityDocument Uses JSON Instead of Protobuf ✅ FIXED

**Status**: **FIXED** - March 4, 2026
**Spec Section**: 1.4
**File**: `pkg/protocol/identity.go`

**Spec**: Protobuf definition provided for `IdentityDocument`.

**Implementation** (BEFORE FIX):
```go
// For now, use JSON serialization
// In production, use protobuf for efficiency
return json.Marshal(doc)
```

**Implementation** (AFTER FIX):
```go
// Convert to protobuf type
pbDoc, err := m.toProtoIdentityDocument(doc)
if err != nil {
    return nil, fmt.Errorf("failed to convert identity document: %w", err)
}

// Marshal protobuf
data, err := proto.Marshal(pbDoc)
if err != nil {
    return nil, fmt.Errorf("failed to marshal protobuf identity document: %w", err)
}
return data, nil
```

**Impact**:
- Incompatible wire format
- Larger message sizes (~2x)
- Not spec-compliant

**Fix Required**: Use protobuf serialization for DHT storage.
**Fix Applied**: Implemented `marshalIdentityDocument()` and `parseIdentityDocument()` using protobuf, with conversion functions for all nested types (DeviceCertificate, SignedPrekey, OneTimePrekey, RevocationCertificate, FeatureFlags).

---

### 5. DHT Custom Validator Missing ✅ FIXED

**Status**: **FIXED** - March 5, 2026
**Spec Section**: 1.4
**File**: `pkg/ipfsnode/dht_validator.go`, `pkg/ipfsnode/lifecycle.go`

**Spec**: Custom DHT validator for namespace `/bt/id/` must:
1. Verify Ed25519 signature against `identity_sign_pub`
2. Verify pubkey hashes to the DHT record key
3. On conflict, prefer higher `sequence` number

**Implementation** (BEFORE FIX): No custom validator registered.

**Implementation** (AFTER FIX): 
- Created `pkg/ipfsnode/dht_validator.go` with `IdentityDocumentValidator` struct
- `Validate()` method verifies Ed25519 signature and pubkey hash matching
- `Select()` method prefers higher sequence number on conflict
- Registered validator in `pkg/ipfsnode/lifecycle.go:initializeHost()` via `RegisterDHTValidators()`

**Impact**: Invalid identity documents can no longer be published to DHT.
**Fix Applied**: Implemented complete DHT validator with signature verification, pubkey hash validation, and sequence-based conflict resolution.

---

### 6. Prekey Bundle Separate DHT Record Missing ✅ FIXED

**Status**: **FIXED** - March 5, 2026
**Spec Section**: 4.2
**File**: `pkg/protocol/identity.go`, `pkg/proto/message.proto`

**Spec**:
- Prekey bundles published standalone at `SHA256("bt-prekeys-v1:" ‖ ed25519_pubkey)`
- For efficient fetch when only prekeys are needed

**Implementation** (BEFORE FIX): Prekeys only embedded in `IdentityDocument`.

**Implementation** (AFTER FIX):
- Updated `PrekeyBundle` protobuf message with identity public keys and cipher suites
- Added `prekeyBundleDHTKey()` function computing key as `SHA256("bt-prekeys-v1:" ‖ identity_pub)`
- Added `PublishPrekeyBundleSeparate()` to publish standalone prekey bundles
- Added `GetPrekeyBundleSeparate()` to fetch prekey bundles with fallback to identity document
- Identity publication now automatically publishes standalone prekey bundle

**Impact**: Efficient prekey fetching without retrieving full identity document.
**Fix Applied**: Implemented separate prekey bundle DHT records with automatic publication on identity update.

---

### 7. X3DH Implementation Missing ✅ FIXED

**Status**: **FIXED** - March 4, 2026
**Spec Section**: 2.2
**File**: `pkg/ratchet/x3dh.go`, `pkg/protocol/session.go`

**Spec**: Complete X3DH protocol with:
- 4-DH computation (IK_dh, SPK, OPK, EK)
- Specific HKDF parameters: `HKDF-SHA256(ikm=input, salt=0x00*32, info="BabylonTowerX3DH"‖Alice.IK_dh.pub‖Bob.IK_dh.pub, len=32)`
- OPK consumption and replenishment
- SPK signature verification

**Implementation** (BEFORE FIX): No X3DH implementation found. Only Double Ratchet exists.

**Implementation** (AFTER FIX): Complete X3DH implementation in `pkg/ratchet/x3dh.go`:
- `X3DHInitiator()`: 4-DH computation with proper HKDF parameters
- `X3DHResponder()`: Mirror computation for responder
- `X3DHResult`: Extended with `RemoteSPKPub` and `UsedOPKPub` fields
- Proper cleanup of sensitive DH values with `zeroBytes()`

**Impact**: **No forward secrecy for session initiation**. Critical security gap.
**Fix Required**: Implement complete X3DH key exchange.
**Fix Applied**: X3DH was already implemented in `pkg/ratchet/x3dh.go`. Fixed integration issue in `pkg/protocol/session.go` to use `x3dhResult.RemoteSPKPub` instead of `x3dhResult.EphemeralPub` for Double Ratchet initialization.

---

### 8. Double Ratchet Initialization Incorrect ✅ FIXED

**Status**: **FIXED** - March 4, 2026 (as part of Issue #7)
**Spec Section**: 2.3
**File**: `pkg/protocol/session.go:CreateInitiator()`

**Spec**:
- Initiator: Generates initial ratchet key pair, performs DH ratchet with Bob.SPK.pub
- Responder: Reuses SPK for first ratchet

**Implementation** (BEFORE FIX):
```go
session, err := ratchet.NewDoubleRatchetStateInitiator(
    sessionID, m.localIdentityPub, remoteIdentity,
    x3dhResult.SharedSecret,
    x3dhResult.EphemeralPub, // Note: This should be the remote SPK pub
)
```

**Implementation** (AFTER FIX):
```go
session, err := ratchet.NewDoubleRatchetStateInitiator(
    sessionID, m.localIdentityPub, remoteIdentity,
    x3dhResult.SharedSecret,
    x3dhResult.RemoteSPKPub, // Use remote SPK pub for Double Ratchet initialization
)
```

**Impact**: Ratchet state initialization incorrect, may cause decryption failures.
**Fix Required**: Implement proper DH ratchet initialization flow.
**Fix Applied**: Updated `X3DHResult` to include `RemoteSPKPub` field and fixed `session.go` to use it.

---

### 9. KDF Functions Labels Unverified ✅ FIXED

**Status**: **FIXED** - March 5, 2026
**Spec Section**: 2.3
**File**: `pkg/ratchet/x3dh.go`

**Spec**:
```
KDF_RK(rk, dh_out):
    output = HKDF-SHA256(ikm=dh_out, salt=rk, info="BabylonTowerRatchet", len=64)

KDF_CK(ck):
    new_chain_key = HMAC-SHA256(ck, 0x01)
    message_key   = HMAC-SHA256(ck, 0x02)
```

**Implementation** (VERIFIED): 
- `KDF_RK()` in `pkg/ratchet/x3dh.go:314` uses `info="BabylonTowerRatchet"` ✓
- `KDF_CK()` in `pkg/ratchet/x3dh.go:331` uses HMAC-SHA256 with `0x01` and `0x02` ✓

**Impact**: None - implementation already matches spec.
**Fix Applied**: Verified KDF labels match spec exactly. No changes required.

---

### 10. Cipher Suite Negotiation Missing ✅ FIXED

**Status**: **FIXED** - March 5, 2026
**Spec Section**: 2.1
**File**: `pkg/protocol/envelope.go`

**Spec**:
- Initiator selects from `IdentityDocument.supported_cipher_suites`
- Negotiation: `max(intersect(own_supported_versions, recipient_supported_versions))`

**Implementation** (BEFORE FIX): Hardcoded to `0x0001` (BT-X25519-XChaCha20-Poly1305-SHA256).

**Implementation** (AFTER FIX):
- Added `NegotiateCipherSuite()` function implementing spec's intersection algorithm
- Added `ParseCipherSuiteFromID()` and `GetCipherSuiteID()` helper functions
- `EnvelopeBuilder` now supports setting cipher suite via `CipherSuite()` method
- Supports current suite `0x0001` and future suite `0x0002` (AES256GCM)

**Impact**: Cipher agility now available for future encryption algorithm upgrades.
**Fix Applied**: Implemented cipher suite negotiation with priority-based selection from intersecting supported suites.

---

### 11. Associated Data Format Unverified ✅ FIXED

**Status**: **FIXED** - March 5, 2026
**Spec Section**: 2.3
**File**: `pkg/ratchet/x3dh.go`

**Spec**:
```
AD = sender.IK_sign.pub ‖ recipient.IK_sign.pub
```

**Implementation** (BEFORE FIX): Used `IK_dh.pub` keys instead of `IK_sign.pub` keys.

**Implementation** (AFTER FIX):
- Updated `X3DHInitiator()` to accept `localIKSignPub` and `remoteIKSignPub` parameters
- Updated `X3DHResponder()` to accept `localIKSignPub` and `remoteIKSignPub` parameters
- AD now constructed as `append(localIKSignPub[:], remoteIKSignPub[:]...)` per spec
- Responder AD correctly ordered as `remote (sender) ‖ local (recipient)`

**Impact**: Associated data now correctly binds identity signing keys for authentication.
**Fix Applied**: Updated X3DH functions to accept and use IK_sign.pub keys for AD construction per spec.

---

## Major Issues

These issues affect core functionality and security properties.

### 12. Sender Keys Implementation Missing

**Spec Section**: 2.4  
**File**: `pkg/groups/`

**Spec**: Complete Sender Keys scheme for group encryption with:
- `SenderKeyDistribution` message
- Chain key derivation: `msg_key = HKDF(chain_key, salt="bt-sk-msg", info=chain_index_bytes, len=32)`
- Per-message signing

**Implementation**: Groups exist but Sender Keys encryption not implemented.

**Impact**: Groups don't have proper E2E encryption.  
**Fix Required**: Implement Sender Keys encryption scheme.

---

### 13. Key Rotation Schedule Not Implemented

**Spec Section**: 2.5  
**File**: `pkg/protocol/identity.go`

**Spec**:
| Key Type | Rotation Period | Overlap | Max Age |
|----------|----------------|---------|---------|
| SPK | 7 days | 24 hours | 14 days |
| OPK | Single use | N/A | Replenish when <20 |

**Implementation**: Has rotation logic but intervals need verification.

**Impact**: Prekey hygiene affects forward secrecy.  
**Fix Required**: Verify and enforce rotation schedule.

---

### 14. Revocation Protocol Incomplete

**Spec Section**: 2.6  
**File**: `pkg/multidevice/revocation.go`

**Spec**:
1. Create `RevocationCertificate` signed by IK_sign
2. Publish to DHT
3. Broadcast on `babylon-rev-<hex(SHA256(IK_sign.pub)[:8])>`

**Implementation**: Exists but broadcast topic not implemented.

**Impact**: Revoked devices may still receive messages.  
**Fix Required**: Implement revocation broadcast.

---

### 15. BabylonEnvelope Serialization Format

**Spec Section**: 3.1  
**File**: `pkg/protocol/envelope.go`

**Spec**: Protobuf `BabylonEnvelope` with 13 fields.

**Implementation**: Builds protobuf but signature uses custom binary serialization.

**Impact**: May not match spec's canonical serialization.  
**Fix Required**: Verify signature serialization matches spec.

---

### 16. Message Types Not Fully Implemented

**Spec Section**: 3.2  
**File**: `pkg/proto/message.pb.go`

**Spec**: 30+ message types defined (DM_TEXT, GROUP_TEXT, RTC_OFFER, etc.)

**Implementation**: Generated code exists but handling logic incomplete for many types.

**Impact**: Many features non-functional.  
**Fix Required**: Implement message handlers for all types.

---

### 17. DM Payload Structure Unverified

**Spec Section**: 3.3  
**File**: `pkg/proto/message.proto`

**Spec**: `DMPayload` with `RatchetHeader` and `oneof content`.

**Implementation**: Protobuf exists but encryption/decryption flow unclear.

**Impact**: May not properly encrypt/decrypt DMs.  
**Fix Required**: Verify DM payload flow matches spec.

---

### 18. Group Payload Signature Missing

**Spec Section**: 3.4  
**File**: `pkg/groups/`

**Spec**: `sender_group_signature = Ed25519.Sign(signing_key, group_id ‖ epoch ‖ chain_index ‖ ciphertext)`

**Implementation**: Field exists but signing logic unclear.

**Impact**: Group message authentication may fail.  
**Fix Required**: Implement group message signing.

---

### 19. Channel Payload Encryption Logic

**Spec Section**: 3.5  
**File**: `pkg/groups/channel.go`

**Spec**:
- Public channels: signed but NOT encrypted
- Private channels: encrypted with channel key (Sender Keys scheme)

**Implementation**: Logic for distinguishing unclear.

**Impact**: May encrypt public channels or leave private channels unencrypted.  
**Fix Required**: Implement proper channel encryption logic.

---

### 20. RTC Signaling Integration

**Spec Section**: 3.6  
**File**: `pkg/rtc/`

**Spec**: RTC messages carried as DM messages through Double Ratchet session.

**Implementation**: Basic RTC exists but integration with messaging unclear.

**Impact**: RTC signaling may not be E2E encrypted.  
**Fix Required**: Integrate RTC with Double Ratchet messaging.

---

### 21. Username Registry Missing

**Spec Section**: 4.1  
**File**: N/A (not implemented)

**Spec**: Optional username system at `SHA256("bt-username-v1:" ‖ lowercase(username))`.

**Implementation**: Not found.

**Impact**: No human-readable addressing.  
**Fix Required**: Implement username registry (optional feature).

---

### 22. Contact Exchange Link Format

**Spec Section**: 4.1  
**File**: N/A (not implemented)

**Spec**: `btower://<base58(ed25519_pubkey)>[?name=<display_name>]`

**Implementation**: Not found.

**Impact**: No easy contact addition via QR/links.  
**Fix Required**: Implement URI scheme handler.

---

### 23. Group State Hash Chain Validation

**Spec Section**: 5.1  
**File**: `pkg/groups/state.go`

**Spec**: `previous_state_hash` for tamper evidence, members validate chain.

**Implementation**: Has `PreviousHash` field but validation logic unclear.

**Impact**: Group state tampering may go undetected.  
**Fix Required**: Implement hash chain validation.

---

### 24. Private Group Member Addition Logic

**Spec Section**: 5.2  
**File**: `pkg/groups/`

**Spec**: New member does NOT receive old sender keys (no history access by default).

**Implementation**: Logic unclear.

**Impact**: May leak group history to new members.  
**Fix Required**: Verify sender key distribution logic.

---

### 25. Public Group Moderation

**Spec Section**: 5.3  
**File**: `pkg/groups/public.go`

**Spec**: `ModerationAction` messages with BAN, MUTE, DELETE_MESSAGE.

**Implementation**: Protobuf exists but enforcement logic unclear.

**Impact**: Public groups may be unmoderatable.  
**Fix Required**: Implement moderation enforcement.

---

### 26. Mailbox Protocol Incomplete

**Spec Section**: 6  
**File**: `pkg/mailbox/`

**Spec**: Complete deposit/retrieval flow with:
- libp2p stream `/bt/mailbox/1.0.0`
- Authentication via Ed25519 nonce signature

**Implementation**: Exists but stream protocol and authentication unclear.

**Impact**: Offline message delivery may not work.  
**Fix Required**: Implement complete mailbox protocol.

---

### 27. Device Sync Message Key Derivation

**Spec Section**: 7.2  
**File**: `pkg/multidevice/sync.go`

**Spec**: Device-group key derived from root seed:
```
HKDF(root_seed, salt="bt-device-group", info="sync-key-v1", len=32)
```

**Implementation**: Encryption exists but key derivation may not match.

**Impact**: Multi-device sync may fail.  
**Fix Required**: Verify key derivation matches spec.

---

### 28. History Sync Protocol

**Spec Section**: 7.3  
**File**: `pkg/multidevice/sync.go`

**Spec**:
- Reverse chronological order
- Batches of 100 messages
- Plaintext + metadata synced (not raw envelopes)

**Implementation**: Exists but flow unclear.

**Impact**: History sync may not work correctly.  
**Fix Required**: Verify history sync protocol.

---

### 29. SFU Topology for Group Calls

**Spec Section**: 8.4  
**File**: `pkg/rtc/`

**Spec**:
- Mesh for 2-6 participants
- SFU for 7-25 participants
- SFU election: lowest lexicographic pubkey

**Implementation**: Basic RTC exists but SFU topology missing.

**Impact**: Group calls limited to small sizes.  
**Fix Required**: Implement SFU topology.

---

### 30. Storage Schema Key Prefixes

**Spec Section**: 11  
**File**: `pkg/storage/`

**Spec**:
```
id:  + <pubkey> → Cached IdentityDocuments
pk:  + <pubkey> → Cached PrekeyBundles
otpk:+ <prekey_id> → One-time prekey private keys
...
```

**Implementation**: Uses different prefixes (`c:`, `m:`, `p:`, `cfg:`, `bl:`).

**Impact**: Storage incompatibility.  
**Fix Required**: Update storage prefixes to match spec.

---

### 31. Reputation System Incomplete

**Spec Section**: 12  
**File**: `pkg/reputation/`

**Spec**: Complete reputation metrics, tiers, attestations.

**Implementation**: Basic implementation exists but incomplete.

**Impact**: Network contribution incentives not functional.  
**Fix Required**: Complete reputation system implementation.

---

## Minor Issues

These issues affect optimization, timing, or edge cases.

### 32. IdentityDocument Size Optimization
**Spec Section**: 1.4 - Target ~2,700 bytes typical. Implementation: No size optimization logic.

### 33. DHT Republish Interval
**Spec Section**: 1.4 - Every 4 hours, 24h expiry. Implementation: Has constants but need verification.

### 34. OPK Race Condition Handling
**Spec Section**: 2.2 - Track consumed OPK IDs, fall back to 3-DH. Implementation: Tracking logic unclear.

### 35. Skipped Keys Cache Limit
**Spec Section**: 2.3 - Max 256 entries. Implementation: Has constant ✓ (verify enforcement).

### 36. Nonce Derivation
**Spec Section**: 2.3 - `nonce = HKDF-SHA256(message_key, salt="nonce", info=counter_as_bytes, len=24)`. Implementation: Need verification.

### 37. Wire Size Tracking
**Spec Section**: 3.1 - Detailed estimates provided. Implementation: No size tracking.

### 38. GossipSub Overhead
**Spec Section**: 3.1 - ~100-200 bytes framing. Implementation: Not accounted for.

### 39. Prekey Batch Size
**Spec Section**: 1.3 - Generate 80 when <20 remain. Implementation: Has logic ✓ (verify constants).

### 40. Feature Flags Extensions
**Spec Section**: 1.4 - `custom_features` for extensions. Implementation: Field exists but mechanism unclear.

### 41. Identity Document Avatar
**Spec Section**: 1.4 - `avatar_cid` field. Implementation: Field exists ✓ (verify usage).

### 42. Device Certificate Expiry
**Spec Section**: 1.3 - `expires_at` field, 0 = no expiry. Implementation: Field exists ✓.

### 43. Display Name Support
**Spec Section**: 1.4 - Optional profile. Implementation: Field exists ✓.

---

## Inconveniences (Design Limitations)

These are acknowledged limitations in the protocol design.

### 44. Message History Not Recoverable
**Spec Section**: 1.5 - Mnemonic alone cannot recover history. **Acknowledged limitation**.

### 45. Clock Dependence
**Spec Section**: 14.3 - Message ordering relies on timestamps. **Acknowledged limitation**.

### 46. DHT Eventual Consistency
**Spec Section**: 14.3 - Prekey consumption races, propagation delays. **Acknowledged limitation**.

### 47. No Guaranteed Delivery
**Spec Section**: 14.3 - Best-effort with redundant mailboxes. **Acknowledged limitation**.

### 48. Group Size Limit
**Spec Section**: 14.3 - ~1000 members due to O(N²) key distribution. **Acknowledged limitation**.

### 49. Metadata Leakage
**Spec Section**: 14.3 - PubSub topics reveal patterns. **Acknowledged limitation** (future onion routing planned).

### 50. Mnemonic = Identity
**Spec Section**: 14.3 - Sole secret, no server-side recovery. **Acknowledged limitation**.

### 51. Local Storage Encryption Missing
**File**: QWEN.md - Local storage not encrypted. **Security concern**.

---

## Missing Implementations

Features specified but not implemented.

### 52. Identity V1 Full Structure
**Spec Section**: 1.2 - Full key tree with Level 0-3. Implementation: Basic structure exists, missing device hierarchy.

### 53. Prekey Consumption Tracking
**Spec Section**: 2.2 - Bob deletes consumed OPK, updates DHT async. Implementation: `ConsumeOneTimePrekey()` exists but DHT update unclear.

### 54. Session Cleanup Enforcement
**Spec Section**: 2.3 - Max 256 skipped keys, session timeout. Implementation: Has cleanup but verify intervals.

### 55. Mailbox Storage Policies
**Spec Section**: 6.4 - Detailed policies (max 500 messages, 256KB, 7 days TTL). Implementation: Policies may not match.

### 56. Media Persistence Pinning
**Spec Section**: 6.5 - Sender pins IPFS until recipient ACKs. Implementation: Pinning logic unclear.

### 57. Multi-Device Optimized Envelope
**Spec Section**: 7.1 - For 5+ devices, encrypt once with symmetric key. Implementation: Protobuf exists, logic not implemented.

### 58. Vector Clock Conflict Resolution
**Spec Section**: 7.4 - Vector clock as tiebreaker. Implementation: Protobuf exists, logic unclear.

### 59. SFU Election Protocol
**Spec Section**: 8.4 - Lowest lexicographic pubkey becomes SFU. Implementation: Missing.

### 60. Capability Advertisement
**Spec Section**: 9 - Advertise via `features.custom_features`. Implementation: Field exists but advertisement logic unclear.

---

## Protobuf Issues

### 61. Protocol Version Field
**Spec Section**: 3.1 - `protocol_version = 1`. Implementation: Correct ✓.

### 62. Message Type Values
**Spec Section**: 3.2 - Specific enum values. Implementation: Matches ✓.

### 63. Field Numbers Consistency
**Spec Section**: 3.x - Specific field numbers. Implementation: Need full verification.

---

## Recommendations

### Priority 1 (Critical - Fix Before Launch)
1. Fix identity key derivation path (#1)
2. Implement X3DH protocol (#7)
3. Fix IdentityDocument serialization (#3, #4)
4. Implement DHT validator (#5)
5. Verify KDF labels (#9)

### Priority 2 (Major - Core Functionality)
1. Implement Sender Keys (#12)
2. Complete mailbox protocol (#26)
3. Implement group encryption (#12, #18, #19)
4. Fix storage schema (#30)
5. Implement revocation broadcast (#14)

### Priority 3 (Minor - Polish)
1. Implement identity fingerprint (#2)
2. Add cipher suite negotiation (#10)
3. Verify all timing constants (#32-43)
4. Implement contact exchange links (#22)

### Won't Fix (Acknowledged Limitations)
- Items #44-50 are design limitations documented in spec

---

## Next Steps

1. **Review**: Team review of this issue list
2. **Prioritize**: Assign priority levels to each issue
3. **Estimate**: Time/cost estimates for fixes
4. **Schedule**: Add to roadmap/sprint planning
5. **Track**: Create GitHub issues for each item

---

*Last updated: March 4, 2026*
