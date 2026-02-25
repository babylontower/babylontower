# Babylon Tower Protocol Specification v1.0

**Version**: 1.0.0
**Status**: Draft
**Created**: February 25, 2026
**Supersedes**: `poc.md` (PoC specification - unversioned)

---

## Overview

This document specifies the complete messaging protocol for Babylon Tower v1 — a fully serverless, decentralized peer-to-peer messenger. It covers identity management, end-to-end encryption with forward secrecy, direct messages, private/public groups, private/public channels, voice/video calls, offline message delivery, multi-device synchronization, and protocol extensibility.

The protocol builds on libp2p for transport (GossipSub PubSub, Kademlia DHT, circuit relay) and uses established cryptographic primitives (X25519, Ed25519, XChaCha20-Poly1305, HKDF-SHA256).

### Design Principles

1. **Zero servers**: No central server, relay, or coordinator. Every function is distributed.
2. **Every node helps**: Desktop nodes participate as full DHT servers, PubSub mesh peers, mailbox relays, and content routers. More clients = faster, more reliable network.
3. **Strong encryption by default**: X3DH + Double Ratchet for DMs (forward secrecy + post-compromise security). Sender Keys for groups.
4. **Offline-capable**: Volunteer mailbox nodes store encrypted messages for offline recipients.
5. **Multi-device**: Independent device keys, per-device encryption, cross-device sync.
6. **Extensible**: Protocol versioning, cipher agility, feature flags, and extension namespaces.

---

## Table of Contents

1. [Identity & Key Hierarchy](#1-identity--key-hierarchy)
2. [Cryptographic Protocols](#2-cryptographic-protocols)
3. [Message Format](#3-message-format)
4. [Discovery & Routing](#4-discovery--routing)
5. [Groups & Channels](#5-groups--channels)
6. [Offline Message Delivery](#6-offline-message-delivery-mailbox-protocol)
7. [Multi-Device Synchronization](#7-multi-device-synchronization)
8. [Voice & Video Calls](#8-voice--video-calls)
9. [Network Contribution](#9-network-contribution)
10. [Protocol Versioning & Updates](#10-protocol-versioning--updates)
11. [Storage Schema](#11-storage-schema-badgerdb-key-prefixes)
12. [Reputation Rewards System](#12-reputation-rewards-system)
13. [Future: Metadata Privacy](#13-future-layer-metadata-privacy-stub)
14. [Threat Model & Security Properties](#14-threat-model--security-properties)

---

## 1. Identity & Key Hierarchy

### 1.1 Root Identity Derivation

The root identity derives from a BIP39 mnemonic phrase — the sole secret a user must protect.

```
BIP39 Mnemonic (12 or 24 words)
    │ PBKDF2 (2048 iterations, empty passphrase)
    ▼
512-bit Seed (64 bytes)
    │ HKDF-SHA256(seed, salt="bt-master-key", info="babylon-tower-v1")
    ▼
Master Secret (32 bytes)
    ├─► HKDF(master, salt="bt-identity", info="identity-signing-key-0") → IK_sign (Ed25519)
    └─► HKDF(master, salt="bt-identity", info="identity-dh-key-0")      → IK_dh (X25519)
```

**PoC compatibility note**: The PoC (unversioned) uses `HKDF(seed, salt="ed25519-derive", info="index-0")` and `HKDF(seed, salt="x25519-derive", info="index-1")`. The v1 derivation introduces a master secret intermediate step with new labels. Implementations must support both derivation paths during the transition period.

**Identity Fingerprint** (for out-of-band verification):

```
Fingerprint = Base58(SHA256(IK_sign.pub ‖ IK_dh.pub)[:20])  → 27-28 characters
```

### 1.2 Full Key Tree

```
Level 0: Root (BIP39 Mnemonic) — sole user secret
    │
Level 1: Identity Keys (derived deterministically from mnemonic, never change)
    │   IK_sign  — Ed25519, signs identity documents & device certificates
    │   IK_dh    — X25519, used in X3DH identity DH
    │
Level 2: Device Keys (random per device, signed by IK_sign)
    │   DK_sign  — Ed25519, signs messages from this device
    │   DK_dh    — X25519, device-level DH
    │   SPK      — X25519, signed prekey (rotated every 7 days)
    │   OPK[0…N] — X25519, one-time prekeys (consumed once)
    │
Level 3: Session Keys (per conversation per device, ephemeral)
        Root Key   → established via X3DH
        Chain Keys → advanced via Double Ratchet
        Message Keys → derived per message, deleted immediately after use
```

**Key sizes**:

| Key | Public | Private | Signature |
|-----|--------|---------|-----------|
| Ed25519 | 32 bytes | 64 bytes (32-byte seed + 32-byte public) | 64 bytes |
| X25519 | 32 bytes | 32 bytes | N/A |
| HKDF output | 32 bytes | — | — |

### 1.3 Device Registration

Device keys are **randomly generated** (not derived from the mnemonic). Compromising one device does not compromise others. The mnemonic proves ownership of the identity and signs device certificates.

```protobuf
message DeviceCertificate {
    bytes  device_id          = 1;  // SHA256(DK_sign.pub)[:16] — 16 bytes
    bytes  device_sign_pub    = 2;  // DK_sign.pub (32 bytes)
    bytes  device_dh_pub      = 3;  // DK_dh.pub (32 bytes)
    string device_name        = 4;  // Human-readable, e.g. "Alice's Desktop"
    uint64 created_at         = 5;  // Unix timestamp
    uint64 expires_at         = 6;  // Unix timestamp, 0 = no expiry
    bytes  identity_pub       = 7;  // IK_sign.pub (for self-contained verification)
    bytes  signature          = 8;  // Ed25519.Sign(IK_sign.priv, canonical(fields 1-7))
}
// Approximate size: ~210 bytes + device_name length
```

**Registration flow**:

1. New device generates random DK_sign + DK_dh key pairs.
2. User enters BIP39 mnemonic → device derives IK_sign and IK_dh from mnemonic.
3. Device creates `DeviceCertificate`, signs it with IK_sign.
4. Device generates one Signed Prekey (SPK) and a batch of 100 One-Time Prekeys (OPKs).
5. Device fetches current `IdentityDocument` from DHT (if exists), adds itself to device list.
6. Device increments `sequence`, publishes updated `IdentityDocument` to DHT.
7. Device subscribes to its identity PubSub topic for incoming messages.

### 1.4 Identity Document

The canonical, signed, versioned record published to the DHT. This is the single source of truth for a user's current keys, devices, and capabilities.

```protobuf
message IdentityDocument {
    // Core identity
    bytes  identity_sign_pub           = 1;   // IK_sign.pub (32 bytes)
    bytes  identity_dh_pub             = 2;   // IK_dh.pub (32 bytes)

    // Versioning (forms a hash chain)
    uint64 sequence                    = 3;   // Monotonically increasing, starts at 1
    bytes  previous_hash               = 4;   // SHA256(previous serialized doc), empty for seq=1
    uint64 created_at                  = 5;   // First publication timestamp
    uint64 updated_at                  = 6;   // This version's timestamp

    // Devices and prekeys
    repeated DeviceCertificate devices     = 7;
    repeated SignedPrekey signed_prekeys    = 8;
    repeated OneTimePrekey one_time_prekeys = 9;

    // Protocol capabilities
    repeated uint32 supported_versions     = 10;  // e.g. [1, 2]
    repeated string supported_cipher_suites = 11;  // e.g. ["BT-X25519-XChaCha20Poly1305-SHA256"]
    uint32 preferred_version               = 12;

    // Optional profile
    string display_name                    = 13;
    string avatar_cid                      = 14;  // IPFS CID of avatar image

    // Revocations
    repeated RevocationCertificate revocations = 15;

    // Feature flags
    FeatureFlags features                  = 16;

    // Signature (covers all fields above)
    bytes  signature                       = 17;  // Ed25519.Sign(IK_sign.priv, canonical(fields 1-16))
}

message SignedPrekey {
    bytes  device_id   = 1;   // Which device owns this prekey
    bytes  prekey_pub  = 2;   // X25519 public key (32 bytes)
    uint64 prekey_id   = 3;   // Unique monotonic ID
    uint64 created_at  = 4;   // Unix timestamp
    uint64 expires_at  = 5;   // Recommended: created_at + 7 days
    bytes  signature   = 6;   // Ed25519.Sign(IK_sign.priv, canonical(fields 1-5))
}

message OneTimePrekey {
    bytes  device_id   = 1;   // Which device owns this prekey
    bytes  prekey_pub  = 2;   // X25519 public key (32 bytes)
    uint64 prekey_id   = 3;   // Unique monotonic ID
}

message RevocationCertificate {
    bytes  revoked_key     = 1;  // Public key being revoked (device or prekey)
    string revocation_type = 2;  // "device" or "prekey"
    string reason          = 3;  // "compromised", "replaced", "expired"
    uint64 revoked_at      = 4;  // Unix timestamp
    bytes  signature       = 5;  // Ed25519.Sign(IK_sign.priv, canonical(fields 1-4))
}

message FeatureFlags {
    bool supports_read_receipts     = 1;
    bool supports_typing_indicators = 2;
    bool supports_reactions         = 3;
    bool supports_edits             = 4;
    bool supports_media             = 5;
    bool supports_voice_calls       = 6;
    bool supports_video_calls       = 7;
    bool supports_groups            = 8;
    bool supports_channels          = 9;
    bool supports_offline_messages  = 10;
    repeated string custom_features = 20;  // e.g. ["babylon.ext.stickers"]
}
```

**DHT publication**:

- **Key**: `/bt/id/` + hex(SHA256(IK_sign.pub)[:16])
- **Republish interval**: every 4 hours (DHT records expire in 24h)
- **Size estimate**: ~2,700 bytes typical (2 devices, 2 SPKs, 20 OPKs)
- **Custom DHT validator** for namespace `/bt/id/` must:
  1. Verify Ed25519 signature against `identity_sign_pub`
  2. Verify pubkey hashes to the DHT record key
  3. On conflict, prefer higher `sequence` number

**Validation rules** (for consumers):

1. `signature` MUST verify against `identity_sign_pub`
2. Each `DeviceCertificate.signature` MUST verify against `identity_sign_pub`
3. Each `SignedPrekey.signature` MUST verify against `identity_sign_pub`
4. `sequence` MUST be strictly greater than any previously seen for this identity
5. If `previous_hash` is present and we have the previous document, it MUST match SHA256 of that document
6. `updated_at` MUST be within ±24 hours of local time (clock skew tolerance)
7. Revoked devices/prekeys MUST NOT appear in active device/prekey lists

### 1.5 Account Recovery

1. User enters BIP39 mnemonic on new device → derives IK_sign + IK_dh.
2. New device generates random device keys (DK_sign, DK_dh).
3. New device creates DeviceCertificate, signs with IK_sign.
4. New device fetches current IdentityDocument from DHT.
5. If found: add new device cert, increment sequence, generate prekeys, republish.
6. If not found: create new IdentityDocument with sequence=1.
7. New device subscribes to its identity topic.

**Limitation**: Message history is device-local and cannot be recovered from the mnemonic alone. History sync requires at least one other device of the same identity to be online (see §7.3).

---

## 2. Cryptographic Protocols

### 2.1 Cipher Suites

```
ID       Name                                     Status
0x0001   BT-X25519-XChaCha20Poly1305-SHA256       MANDATORY
0x0002   BT-X25519-AES256GCM-SHA256               OPTIONAL
```

All implementations MUST support suite 0x0001. Negotiation: initiator selects the highest mutually supported suite from `IdentityDocument.supported_cipher_suites`. The selected suite is recorded in the X3DH initial message header.

### 2.2 X3DH — Extended Triple Diffie-Hellman

X3DH establishes a shared secret between two parties who may not both be online. This follows the Signal Protocol X3DH specification adapted for Babylon Tower's key hierarchy.

**Key types used in X3DH**:

| Abbreviation | Full Name | Type | Lifetime |
|---|---|---|---|
| IK_dh | Identity DH Key | X25519 | Permanent (derived from mnemonic) |
| SPK | Signed Prekey | X25519 | Rotated every 7 days |
| OPK | One-Time Prekey | X25519 | Single use, consumed |
| EK | Ephemeral Key | X25519 | Single use, per session init |

**Prekey Bundle** (fetched from recipient's IdentityDocument):

```
Field                  Size    Description
identity_dh_pub        32B     Recipient's IK_dh.pub
identity_sign_pub      32B     Recipient's IK_sign.pub (for verification)
signed_prekey_pub      32B     Recipient's SPK.pub
signed_prekey_sig      64B     Ed25519 signature of SPK.pub by IK_sign
signed_prekey_id       8B      Monotonic ID of this SPK
one_time_prekey_pub    32B     Recipient's OPK.pub (may be absent)
one_time_prekey_id     8B      Monotonic ID (0 if absent)
Total: 224 bytes (with OPK) or 160 bytes (without)
```

**Protocol (Alice initiates to Bob)**:

```
PRECONDITION: Alice has Bob's PrekeyBundle (fetched from DHT)

Step 1: Alice verifies SPK signature
    valid = Ed25519.Verify(Bob.IK_sign.pub, Bob.SPK.pub, Bob.signed_prekey_sig)
    If NOT valid: ABORT

Step 2: Alice generates ephemeral key pair
    EK_priv, EK_pub = X25519.Generate()

Step 3: Alice computes DH values
    DH1 = X25519(Alice.IK_dh.priv,  Bob.SPK.pub)       // Identity ↔ SignedPrekey
    DH2 = X25519(EK_priv,           Bob.IK_dh.pub)      // Ephemeral ↔ Identity
    DH3 = X25519(EK_priv,           Bob.SPK.pub)         // Ephemeral ↔ SignedPrekey
    If OPK present:
        DH4 = X25519(EK_priv,       Bob.OPK.pub)        // Ephemeral ↔ OneTimePrekey

Step 4: Alice derives shared secret
    If OPK present:
        input = DH1 ‖ DH2 ‖ DH3 ‖ DH4    (128 bytes)
    Else:
        input = DH1 ‖ DH2 ‖ DH3            (96 bytes)

    SK = HKDF-SHA256(
        ikm  = input,
        salt = 0x00 * 32,           // 32 zero bytes
        info = "BabylonTowerX3DH" ‖ Alice.IK_dh.pub ‖ Bob.IK_dh.pub,
        len  = 32
    )

Step 5: Associated data
    AD = Alice.IK_sign.pub ‖ Bob.IK_sign.pub

Step 6: Alice sends X3DH Initial Message containing:
    - Alice.IK_dh.pub, EK_pub, Bob.SPK_id, Bob.OPK_id (if used), cipher_suite_id
    - First message body encrypted via Double Ratchet initialized with SK

Step 7: Alice deletes EK_priv, DH1-DH4 from memory
    Alice stores SK as the initial root key for the Double Ratchet
```

**Bob's side (receiving X3DH Initial Message)**:

```
Step 1: Bob receives: Alice.IK_dh.pub, EK_pub, SPK_id, OPK_id

Step 2: Bob looks up SPK private key by SPK_id
    Bob looks up OPK private key by OPK_id (if non-zero)

Step 3: Bob computes DH values (mirror of Alice)
    DH1 = X25519(Bob.SPK.priv,     Alice.IK_dh.pub)
    DH2 = X25519(Bob.IK_dh.priv,   EK_pub)
    DH3 = X25519(Bob.SPK.priv,     EK_pub)
    If OPK_id != 0:
        DH4 = X25519(Bob.OPK.priv, EK_pub)

Step 4: Bob derives SK using same HKDF parameters as Alice

Step 5: AD = Alice.IK_sign.pub ‖ Bob.IK_sign.pub

Step 6: Bob decrypts the first message using Double Ratchet with SK

Step 7: Bob deletes consumed OPK private key (single use)
    Bob stores SK as initial root key for Double Ratchet
    Bob asynchronously removes consumed OPK from IdentityDocument and republishes
```

**Edge cases**:

- **OPK exhaustion**: Proceed with DH1+DH2+DH3 only (no DH4). Slightly weaker one-time forward secrecy but still secure.
- **OPK race condition** (DHT eventual consistency): Multiple initiators may fetch the same OPK. Bob tracks consumed OPK IDs locally. On duplicate, fall back to 3-DH.
- **Stale SPK**: Reject SPK with `created_at` older than 21 days (7 rotation + 14 overlap).

**Security properties**:
- **Mutual authentication**: Both parties' identity keys bound into SK derivation.
- **Forward secrecy**: Ephemeral keys ensure past sessions are secure even if long-term keys are later compromised.
- **Deniability**: No signatures on the key exchange itself.
- **Offline initiation**: Bob does not need to be online (Alice uses published prekeys).

### 2.3 Double Ratchet

After X3DH establishes SK, the Double Ratchet provides forward secrecy and post-compromise security for ongoing conversations.

**Per-session state** (each party maintains):

```
dh_sending_keypair      : X25519KeyPair   // Our current ratchet key pair
dh_receiving_pub        : bytes[32]       // Their current ratchet public key
root_key                : bytes[32]       // Current root key
sending_chain_key       : bytes[32]       // Current sending chain key
sending_chain_counter   : uint32          // Messages sent in current chain
receiving_chain_key     : bytes[32]       // Current receiving chain key
receiving_chain_counter : uint32          // Messages received in current chain
skipped_keys            : map[(ratchet_pub, counter) → message_key]  // Max 256 entries
```

**KDF functions**:

```
KDF_RK(rk, dh_out):
    output = HKDF-SHA256(ikm=dh_out, salt=rk, info="BabylonTowerRatchet", len=64)
    return (output[:32], output[32:])   // (new_root_key, new_chain_key)

KDF_CK(ck):
    new_chain_key = HMAC-SHA256(ck, 0x01)
    message_key   = HMAC-SHA256(ck, 0x02)
    return (new_chain_key, message_key)
```

**Initialization (Alice, the X3DH initiator)**:

```
1. Alice generates initial ratchet key pair:
     Alice.dh_sending_keypair = X25519.Generate()

2. Alice performs initial DH ratchet:
     dh_output = X25519(Alice.dh_sending_keypair.priv, Bob.SPK.pub)
     (Alice.root_key, Alice.sending_chain_key) = KDF_RK(SK, dh_output)

3. Alice sets:
     Alice.dh_receiving_pub      = Bob.SPK.pub
     Alice.sending_chain_counter = 0
     Alice.receiving_chain_key   = <unset until first message from Bob>
     Alice.receiving_chain_counter = 0
```

**Initialization (Bob, the X3DH responder)**:

```
1. Bob sets:
     Bob.dh_sending_keypair  = (Bob.SPK.priv, Bob.SPK.pub)  // Reuse SPK for first ratchet
     Bob.root_key            = SK
     Bob.dh_receiving_pub    = <unset until first message from Alice>
```

**Sending a message**:

```
1. Derive message key:
     (sending_chain_key, message_key) = KDF_CK(sending_chain_key)

2. Derive nonce:
     nonce = HKDF-SHA256(message_key, salt="nonce", info=counter_as_bytes, len=24)

3. Encrypt:
     AD = sender.IK_sign.pub ‖ recipient.IK_sign.pub
     ciphertext = XChaCha20-Poly1305.Encrypt(message_key, nonce, plaintext, AD)

4. Construct RatchetHeader:
     header = {
         dh_ratchet_pub:       dh_sending_keypair.pub,
         previous_chain_length: <prev chain length>,
         message_number:        sending_chain_counter
     }

5. Increment sending_chain_counter
6. Delete message_key from memory immediately
```

**Receiving a message**:

```
1. Extract header.dh_ratchet_pub from message

2. If header.dh_ratchet_pub != dh_receiving_pub:
     a. Skip remaining messages in current receiving chain (cache keys, max 256)
     b. DH ratchet step:
         dh_output = X25519(dh_sending_keypair.priv, header.dh_ratchet_pub)
         (root_key, receiving_chain_key) = KDF_RK(root_key, dh_output)
         dh_receiving_pub = header.dh_ratchet_pub
         receiving_chain_counter = 0
     c. New sending ratchet:
         dh_sending_keypair = X25519.Generate()
         dh_output2 = X25519(dh_sending_keypair.priv, header.dh_ratchet_pub)
         (root_key, sending_chain_key) = KDF_RK(root_key, dh_output2)
         sending_chain_counter = 0

3. Skip messages if header.message_number > receiving_chain_counter:
     Cache skipped message keys (max 256 total skipped keys)

4. Derive message key and decrypt:
     (receiving_chain_key, message_key) = KDF_CK(receiving_chain_key)
     receiving_chain_counter += 1

5. Delete message_key after decryption
```

**Security properties**:
- **Forward secrecy**: Each message key is derived and immediately deleted.
- **Post-compromise security**: Each DH ratchet step introduces new entropy. After a round-trip exchange, future messages are secure even if current state was compromised.
- **Out-of-order tolerance**: Up to 256 skipped message keys cached.

### 2.4 Sender Keys (for Groups)

Group messages use a Sender Keys scheme to avoid O(N) encryption per message. Each group member distributes their own sender key chain to all other members.

```protobuf
message SenderKeyDistribution {
    bytes  group_id    = 1;   // 32-byte group identifier
    uint64 epoch       = 2;   // Key epoch (incremented on rotation)
    bytes  sender_pub  = 3;   // Ed25519 pub of the sender
    bytes  chain_key   = 4;   // 32-byte initial chain key
    bytes  signing_key = 5;   // Ed25519 pub for message authentication in group
    uint32 chain_index = 6;   // Starting chain index
}
```

**Sender Key distribution**:

1. When joining a group, member generates: random `chain_key` (32 bytes) and `signing_key` (Ed25519).
2. For each other group member, encrypt the `SenderKeyDistribution` using the existing pairwise Double Ratchet session (or X3DH if no session exists) and send as a control message.
3. Each member stores received SenderKeys indexed by `(group_id, sender_pub)`.

**Group message encryption**:

```
1. Derive message key:
     msg_key = HKDF(chain_key, salt="bt-sk-msg", info=chain_index_bytes, len=32)

2. Advance chain:
     chain_key = HKDF(chain_key, salt="bt-sk-chain", info="advance", len=32)
     chain_index += 1

3. Derive nonce:
     nonce = HKDF(msg_key, salt="nonce", info=chain_index_bytes, len=24)

4. Encrypt:
     ciphertext = XChaCha20-Poly1305.Encrypt(msg_key, nonce, plaintext, group_id)

5. Sign:
     signature = Ed25519.Sign(signing_key.priv, group_id ‖ epoch ‖ chain_index ‖ ciphertext)

6. Publish to group PubSub topic
```

**Sender Key rotation** (triggered by member removal):

1. Group admin broadcasts `GROUP_MEMBER_REMOVED` message.
2. All remaining members generate new SenderKeys (new `chain_key`, new `signing_key`).
3. Each member distributes new SenderKey to all other remaining members via pairwise channels.
4. `epoch` increments. Old SenderKeys are kept for decrypting history.
5. Messages with old epoch are accepted for a 1-hour grace period.

### 2.5 Key Rotation Schedule

| Key Type | Rotation Period | Overlap | Max Age |
|----------|----------------|---------|---------|
| Signed Prekey (SPK) | 7 days | 24 hours (old still accepted) | 14 days (force reject) |
| One-Time Prekeys (OPK) | Single use (consumed) | N/A | Replenish when <20 remain |
| Sender Keys | On member removal | 1 hour grace period | Until group epoch changes |
| Device Keys | Manual only | N/A | Per `expires_at` field |

**OPK replenishment**: Target 100 OPKs in IdentityDocument. Generate batch of 80 when count drops below 20. Check every 6 hours or on application foreground.

**SPK rotation flow**:

1. Generate new SPK key pair.
2. Sign new SPK with IK_sign.
3. Update IdentityDocument: add new SPK, keep old SPK during overlap period.
4. Publish updated IdentityDocument to DHT.
5. After 24-hour overlap, remove old SPK from document.
6. Delete old SPK private key from device storage.

### 2.6 Key Revocation

**Device revocation**:

1. Create `RevocationCertificate` signed by IK_sign.
2. Remove revoked device from IdentityDocument's device list, remove its prekeys.
3. Add RevocationCertificate to `revocations` list, increment `sequence`.
4. Publish updated document to DHT.
5. Broadcast revocation notification on topic: `babylon-rev-<hex(SHA256(IK_sign.pub)[:8])>`.
6. Contacts verify certificate, invalidate sessions with the revoked device, re-establish X3DH to a different device.

**Identity revocation** (mnemonic compromised):

Publish identity-level RevocationCertificate + notify contacts out-of-band + generate new identity. This is an inherent limitation of mnemonic-based systems — the mnemonic IS the identity.

---

## 3. Message Format

### 3.1 Wire Envelope

All messages on the wire use this outer structure:

```protobuf
message BabylonEnvelope {
    // Header (unencrypted, used for routing)
    uint32 protocol_version   = 1;   // 2 for this spec
    uint32 message_type       = 2;   // MessageType enum value
    bytes  sender_identity    = 3;   // Sender's IK_sign.pub (32 bytes)
    bytes  recipient_identity = 4;   // Recipient's IK_sign.pub (32 bytes, empty for group/channel)
    uint64 timestamp          = 5;   // Unix timestamp (seconds)
    bytes  message_id         = 6;   // Random 16 bytes, unique per message

    // Routing (set for group/channel messages)
    bytes  group_id           = 7;   // 32 bytes for group messages
    bytes  channel_id         = 8;   // 32 bytes for channel messages

    // Encrypted payload (interpretation depends on message_type)
    bytes  payload            = 9;

    // Authentication
    bytes  sender_device_id   = 10;  // 16 bytes, identifies sending device
    bytes  signature          = 11;  // Ed25519.Sign(DK_sign.priv, canonical(fields 1-10))

    // Cipher suite used for payload encryption
    uint32 cipher_suite_id    = 12;  // 0x0001 = BT-X25519-XChaCha20Poly1305-SHA256

    // X3DH header (present only for session initialization)
    bytes  x3dh_header        = 13;  // Serialized X3DHHeader
}
```

**Wire size estimates**:

| Message Type | Typical Payload | Envelope Overhead | Total Wire Size |
|---|---|---|---|
| DM text (100 chars) | 100 bytes | ~256 bytes | ~356 bytes |
| DM text (1000 chars) | 1000 bytes | ~256 bytes | ~1,256 bytes |
| DM media reference | ~200 bytes | ~256 bytes | ~456 bytes |
| DM typing indicator | ~5 bytes | ~256 bytes | ~261 bytes |
| Group text (100 chars) | 100 bytes | ~220 bytes | ~320 bytes |
| X3DH initial + first msg | 176+ bytes | ~340 bytes | ~516+ bytes |

GossipSub adds ~100-200 bytes of framing per message.

### 3.2 Message Types

```protobuf
enum MessageType {
    MESSAGE_TYPE_UNSPECIFIED       = 0;

    // Direct Messages (1-19)
    DM_TEXT                        = 1;
    DM_MEDIA                       = 2;   // IPFS CID reference
    DM_REACTION                    = 3;
    DM_EDIT                        = 4;
    DM_DELETE                      = 5;
    DM_READ_RECEIPT                = 6;
    DM_TYPING                      = 7;
    DM_DELIVERY_RECEIPT            = 8;

    // Group Messages (20-39)
    GROUP_TEXT                     = 20;
    GROUP_MEDIA                    = 21;
    GROUP_REACTION                 = 22;
    GROUP_EDIT                     = 23;
    GROUP_DELETE                   = 24;
    GROUP_MEMBER_ADDED             = 25;
    GROUP_MEMBER_REMOVED           = 26;
    GROUP_KEY_ROTATION             = 27;
    GROUP_INFO_UPDATE              = 28;

    // Channel Messages (40-59)
    CHANNEL_POST                   = 40;
    CHANNEL_EDIT                   = 41;
    CHANNEL_DELETE                 = 42;

    // Control Messages (60-79)
    CTRL_X3DH_INITIAL              = 60;
    CTRL_PREKEY_BUNDLE             = 61;
    CTRL_DEVICE_ANNOUNCE           = 62;
    CTRL_DEVICE_REVOKE             = 63;
    CTRL_SENDER_KEY_DIST           = 64;
    CTRL_KEY_REQUEST               = 65;
    CTRL_IDENTITY_UPDATE           = 66;

    // RTC Signaling (80-99)
    RTC_OFFER                      = 80;
    RTC_ANSWER                     = 81;
    RTC_ICE_CANDIDATE              = 82;
    RTC_HANGUP                     = 83;
}
```

### 3.3 DM Payloads

For DM messages, `BabylonEnvelope.payload` contains Double Ratchet encrypted content. The plaintext before encryption:

```protobuf
message DMPayload {
    RatchetHeader ratchet_header = 1;
    oneof content {
        TextMessage      text             = 10;
        MediaMessage     media            = 11;
        ReactionMessage  reaction         = 12;
        EditMessage      edit             = 13;
        DeleteMessage    delete_msg       = 14;
        ReadReceipt      read_receipt     = 15;
        TypingIndicator  typing           = 16;
        DeliveryReceipt  delivery_receipt = 17;
    }
}

message RatchetHeader {
    bytes  dh_ratchet_pub        = 1;  // Current DH ratchet public key (32 bytes)
    uint32 previous_chain_length = 2;  // Length of previous sending chain
    uint32 message_number        = 3;  // Index in current sending chain
}

message TextMessage {
    string text     = 1;  // UTF-8 message text
    string language = 2;  // Optional BCP-47 language tag
}

message MediaMessage {
    string cid          = 1;   // IPFS CID of encrypted media
    string content_type = 2;   // MIME type: "image/jpeg", "audio/opus", etc.
    uint64 size_bytes   = 3;   // Size of the plaintext media
    string filename     = 4;   // Original filename
    bytes  thumbnail    = 5;   // Inline thumbnail (max 10KB, JPEG)
    bytes  media_key    = 6;   // AES-256 key to decrypt the media at CID
    bytes  media_hash   = 7;   // SHA256 of plaintext media (for verification)
    uint32 width        = 8;   // For images/video
    uint32 height       = 9;   // For images/video
    uint32 duration_ms  = 10;  // For audio/video
}

message ReactionMessage {
    bytes  target_message_id = 1;  // message_id of the target message
    string emoji             = 2;  // Unicode emoji or shortcode
    bool   remove            = 3;  // true = remove reaction
}

message EditMessage {
    bytes  target_message_id = 1;
    string new_text          = 2;
    uint64 edit_timestamp    = 3;
}

message DeleteMessage {
    bytes target_message_id = 1;
}

message ReadReceipt {
    repeated bytes message_ids = 1;  // Messages marked as read
    uint64 read_at             = 2;
}

message TypingIndicator {
    bool is_typing = 1;  // true = started typing, false = stopped
}

message DeliveryReceipt {
    repeated bytes message_ids = 1;
    uint64 delivered_at        = 2;
}
```

### 3.4 Group Payloads

For group messages, `payload` is encrypted with the Sender Key scheme:

```protobuf
message GroupPayload {
    uint64 epoch       = 1;  // Must match current group epoch
    uint32 chain_index = 2;  // Sender's chain index for this message
    oneof content {
        TextMessage      text         = 10;
        MediaMessage     media        = 11;
        ReactionMessage  reaction     = 12;
        EditMessage      edit         = 13;
        DeleteMessage    delete_msg   = 14;
        GroupMemberEvent member_event = 15;
        GroupInfoUpdate  info_update  = 16;
    }
    bytes sender_group_signature = 20;  // Ed25519.Sign(signing_key, content)
}

message GroupMemberEvent {
    bytes  member_pubkey = 1;  // Ed25519 pub of affected member
    string action        = 2;  // "added", "removed", "left", "promoted", "demoted"
    string role          = 3;  // "member", "admin", "owner"
    bytes  actor_pubkey  = 4;  // Who performed the action
}

message GroupInfoUpdate {
    string new_name        = 1;  // Max 128 UTF-8 chars
    string new_description = 2;  // Max 512 UTF-8 chars
    string new_avatar_cid  = 3;  // IPFS CID
}
```

### 3.5 Channel Payloads

```protobuf
message ChannelPayload {
    oneof content {
        TextMessage   text       = 1;
        MediaMessage  media      = 2;
        EditMessage   edit       = 3;
        DeleteMessage delete_msg = 4;
    }
}
```

- **Public channels**: payload is signed but NOT encrypted. Anyone subscribed can read.
- **Private channels**: payload is encrypted with a channel key (Sender Keys scheme, only channel publishers have sender keys).

### 3.6 RTC Signaling Payloads

Carried as DM messages through the existing Double Ratchet session (end-to-end encrypted signaling):

```protobuf
message RTCOffer {
    string sdp      = 1;  // SDP offer string
    string call_id  = 2;  // Random UUID for this call session
    bool   video    = 3;  // true = video call, false = audio only
}

message RTCAnswer {
    string sdp      = 1;  // SDP answer string
    string call_id  = 2;
}

message RTCIceCandidate {
    string candidate     = 1;  // ICE candidate string
    string sdp_mid       = 2;
    uint32 sdp_mline_idx = 3;
    string call_id       = 4;
}

message RTCHangup {
    string call_id = 1;
    string reason  = 2;  // "normal", "busy", "declined", "timeout"
}
```

### 3.7 Control Message Payloads

```protobuf
message X3DHHeader {
    bytes  initiator_identity_dh_pub = 1;  // Alice's IK_dh.pub (32 bytes)
    bytes  ephemeral_pub             = 2;  // Alice's EK.pub (32 bytes)
    uint64 signed_prekey_id          = 3;  // Bob's SPK ID used
    uint64 one_time_prekey_id        = 4;  // Bob's OPK ID used (0 if none)
    uint32 cipher_suite_id           = 5;  // Negotiated suite
}

message DeviceAnnouncement {
    DeviceCertificate device          = 1;
    uint64 identity_doc_sequence      = 2;  // Current IdentityDocument sequence
}

message KeyRequest {
    bytes  group_id     = 1;  // For group key requests
    bytes  session_id   = 2;  // For session key requests
    string request_type = 3;  // "sender_key", "session", "prekey_bundle"
}

message IdentityUpdateNotification {
    bytes  identity_pub  = 1;  // IK_sign.pub
    uint64 new_sequence  = 2;
    bytes  document_hash = 3;  // SHA256 of new IdentityDocument
    uint64 timestamp     = 4;
    bytes  signature     = 5;  // Signed by IK_sign
}
```

---

## 4. Discovery & Routing

### 4.1 User Discovery

**DHT-based identity lookup**: Given a public key, fetch the `IdentityDocument` from the `/bt/id/` DHT namespace.

**Username registry** (optional, decentralized, first-come-first-served):

```protobuf
message UsernameRecord {
    string username       = 1;  // Lowercase alphanumeric + underscores, 3-32 chars
    bytes  owner_pubkey   = 2;  // Ed25519 public key of the owner
    uint64 registered_at  = 3;  // Unix timestamp
    uint64 renewed_at     = 4;  // Must renew within 365 days or name expires
    bytes  signature      = 5;  // Ed25519 signature over fields 1-4
}
```

- **DHT key**: `SHA256("bt-username-v1:" ‖ lowercase(username))`
- **Expiry**: If `renewed_at` is older than 365 days, the name is available for re-registration.
- **Conflict resolution**: DHT validator prefers older `registered_at` timestamp.
- **Republish**: every 4 hours.

**Contact exchange link**: `btower://<base58(ed25519_pubkey)>[?name=<display_name>]`

Works as a clickable link or QR code. Recipient's client resolves the pubkey, fetches IdentityDocument from DHT, and adds the contact.

### 4.2 Prekey Distribution

Prekey bundles are:
1. Embedded in the `IdentityDocument` (fields 8-9).
2. Also published as a standalone DHT record at `SHA256("bt-prekeys-v1:" ‖ ed25519_pubkey)` for efficient fetch when only prekeys are needed.

### 4.3 Topic Routing

| Context | Topic Format | Example |
|---------|-------------|---------|
| Direct Message | `babylon-dm-` + hex(SHA256(recipient.pub)[:8]) | `babylon-dm-3a5f8c2e` |
| Private Group | `babylon-grp-` + hex(SHA256(group_id)[:8]) | `babylon-grp-b7c9e1f4` |
| Public Group | `babylon-pub-` + hex(SHA256(group_id)[:8]) | `babylon-pub-c1d2e3f4` |
| Channel | `babylon-ch-` + hex(SHA256(channel_id)[:8]) | `babylon-ch-d2a8f0c3` |
| Revocation | `babylon-rev-` + hex(SHA256(identity.pub)[:8]) | `babylon-rev-e4b2c1a9` |
| Device Sync | `babylon-sync-` + hex(SHA256(root.pub)[:8]) | `babylon-sync-f5a6b7c8` |

**PoC backward compatibility**: During the transition period, v1 clients subscribe to BOTH `babylon-` (PoC format) and `babylon-dm-` (v1 format) topics. v1 clients publish to v1 topics only.

### 4.4 Peer Routing Priority

Messages are delivered through the first successful mechanism:

1. **Direct connection**: Both online, known multiaddrs from IdentityDocument.
2. **DHT FindPeer**: Look up recipient's PeerID via DHT, connect directly.
3. **Circuit relay v2**: NAT traversal via libp2p relay + hole punching.
4. **Mailbox deposit**: Recipient offline — deposit at mailbox nodes (see §6).

---

## 5. Groups & Channels

### 5.1 Group State

```protobuf
message GroupState {
    bytes  group_id        = 1;   // Random 32 bytes
    uint64 epoch           = 2;   // Incremented on every membership change
    string name            = 3;   // Max 128 UTF-8 chars
    string description     = 4;   // Max 512 UTF-8 chars
    string avatar_cid      = 5;   // IPFS CID
    GroupType type          = 6;
    repeated GroupMember members = 7;
    bytes  creator_pubkey  = 8;   // Ed25519 pub of the group creator
    uint64 created_at      = 9;
    uint64 updated_at      = 10;
    bytes  state_signature = 15;  // Signed by the admin who made this update
}

enum GroupType {
    PRIVATE_GROUP   = 0;
    PUBLIC_GROUP    = 1;
    PRIVATE_CHANNEL = 2;
    PUBLIC_CHANNEL  = 3;
}

message GroupMember {
    bytes     ed25519_pubkey   = 1;
    bytes     x25519_pubkey    = 2;
    string    display_name     = 3;
    uint64    joined_at        = 4;
    GroupRole role             = 5;
}

enum GroupRole {
    MEMBER = 0;
    ADMIN  = 1;
    OWNER  = 2;
}
```

**Group state chain** (tamper evidence):

```protobuf
message GroupStateUpdate {
    GroupState new_state          = 1;
    bytes     previous_state_hash = 2;  // SHA256 of the previous GroupState
    bytes     updater_pubkey      = 3;  // Must be an admin or owner
    bytes     updater_signature   = 4;  // Ed25519 signature
}
```

Members validate: `SHA256(stored_current_state) == new_update.previous_state_hash`. If mismatch, reject and request reconciliation from other members.

### 5.2 Private Groups (Encrypted, Members-Only)

- **Creation**: Creator generates random `group_id`, creates initial `GroupState`, invites members via pairwise X3DH/Double Ratchet channels.
- **Encryption**: Sender Keys (§2.4) — each member has their own chain key, O(1) encryption per message.
- **Member addition**: `epoch++`, distribute group state + existing sender keys to new member via pairwise channel. New member does NOT receive old sender keys (no history access by default).
- **Member removal**: `epoch++`, ALL remaining members generate new Sender Keys and distribute via pairwise channels. Removed member cannot decrypt future messages.
- **Max size**: 1000 members (Sender Key distribution is O(N²) on membership change).
- **Split-brain resolution**: Prefer highest epoch. Tiebreak by lexicographically lower admin pubkey.

### 5.3 Public Groups (Discoverable, Unencrypted)

- Discoverable via DHT at `SHA256("bt-pubgroup-v1:" ‖ group_id)`.
- Messages are signed but NOT end-to-end encrypted. Any subscriber can read.
- **Moderation**: Advisory `ModerationAction` messages (BAN, MUTE, DELETE_MESSAGE) signed by admins. Moderation is advisory — compliant nodes enforce, malicious nodes may ignore.
- **Anti-spam**: Per-sender rate limit (10 messages per 60 seconds), optional HashCash proof-of-work, local reputation-based filtering.

### 5.4 Private Channels (Publisher → Subscribers, Encrypted)

- Publisher creates `channel_id` + channel key pair.
- Only OWNER (publisher) can post. Subscribers can only read.
- Channel key distributed to subscribers via pairwise channels.
- Subscriber removal → channel key rotation, new key distributed to remaining subscribers.

### 5.5 Public Channels (Publisher → Subscribers, Open)

- Publisher signs messages, anyone can subscribe. No encryption.
- Content persistence via IPFS linked-list: each post stores the CID of the previous post.
- Channel head (latest post CID) published to DHT at `SHA256("bt-chanhead-v1:" ‖ channel_id)`.
- Subscribers can traverse the CID chain to fetch historical posts.

---

## 6. Offline Message Delivery (Mailbox Protocol)

### 6.1 Mailbox Nodes

Desktop nodes volunteer to store encrypted messages for offline peers. A node typically serves as mailbox for its contacts.

```protobuf
message MailboxAnnouncement {
    bytes  mailbox_peer_id    = 1;  // libp2p PeerID of the mailbox node
    bytes  target_pubkey      = 2;  // Ed25519 pub of the peer this mailbox serves
    uint64 capacity_bytes     = 3;  // Remaining storage capacity
    uint64 max_message_size   = 4;  // Max single message size accepted
    uint32 max_messages       = 5;  // Max messages stored for this target
    uint64 ttl_seconds        = 6;  // How long messages are retained
    uint64 announced_at       = 7;  // Unix timestamp
    bytes  signature          = 8;  // Signed by mailbox node's device key
}
```

- **DHT key**: `SHA256("bt-mailbox-v1:" ‖ target_pubkey)`
- **Republish**: every 4 hours
- **Multiple mailboxes**: Multiple nodes can announce for the same target. Recipient queries all.

### 6.2 Deposit Flow

1. Sender publishes message to recipient's PubSub topic.
2. If no topic subscribers within 5 seconds (checked via `pubsub.ListPeers(topic)`), recipient is assumed offline.
3. Sender queries DHT for `MailboxAnnouncement` records for the recipient.
4. Sender selects up to 3 mailbox nodes (prefer highest reputation, closest in DHT space).
5. For each mailbox: open libp2p stream `/bt/mailbox/1.0.0`, send deposit request.
6. Mailbox validates: message size within limits, target pubkey matches, quota not exceeded.
7. Mailbox stores encrypted envelope, responds success/failure.
8. Delivery considered successful if ≥1 mailbox accepts.

### 6.3 Retrieval Flow

1. Recipient comes online, subscribes to their PubSub topic.
2. Recipient queries DHT for `MailboxAnnouncement` records listing their pubkey.
3. For each mailbox: open stream, authenticate (recipient signs a random nonce with Ed25519).
4. Mailbox returns all stored messages.
5. Recipient decrypts, deduplicates by `message_id` (SHA256 of envelope), processes.
6. Recipient sends ACK with list of received `message_id`s.
7. Mailbox deletes acknowledged messages.

### 6.4 Storage Policies

| Parameter | Default | Range |
|-----------|---------|-------|
| Max messages per target | 500 | 100-1000 |
| Max message size | 256 KB | — |
| Max total storage per target | 64 MB | 16-256 MB |
| Default TTL | 7 days | 1-30 days |
| Eviction policy | Oldest-first when full | — |
| Deposit rate limit | 100 msg/sender/target/hour | — |

### 6.5 Media Persistence

Large attachments are stored as encrypted IPFS objects. The mailbox stores only the message envelope (which contains IPFS CID references). Sender pins the IPFS object until recipient ACKs retrieval. Mailbox nodes optionally pin referenced CIDs if they have the `content-route` capability.

---

## 7. Multi-Device Synchronization

### 7.1 Message Fanout

When sending to a recipient with multiple devices:

1. Sender fetches recipient's `IdentityDocument` → reads device list.
2. Sender maintains a separate Double Ratchet session per recipient device.
3. Sender encrypts the message once per device.

**Optimization for 5+ devices**: Encrypt the message once with a random symmetric key, then encrypt that symmetric key individually for each device's session. Wire size: O(msg_size + N×32) instead of O(N×msg_size).

### 7.2 Device Sync Channel

All devices of the same identity subscribe to a sync topic:

**Topic**: `babylon-sync-<hex(SHA256(root_IK_sign.pub)[:8])>`

**Encryption**: Device-group key derived from root seed: `HKDF(root_seed, salt="bt-device-group", info="sync-key-v1", len=32)`. All devices that have the mnemonic can derive this key.

```protobuf
message DeviceSyncMessage {
    uint32 source_device_id  = 1;
    SyncType type            = 2;
    bytes  encrypted_payload = 3;  // Encrypted with device-group key
    bytes  nonce             = 4;
    uint64 timestamp         = 5;
    bytes  vector_clock      = 6;  // Serialized vector clock for conflict resolution
}

enum SyncType {
    CONTACT_ADDED   = 0;
    CONTACT_REMOVED = 1;
    CONTACT_UPDATED = 2;
    MESSAGE_READ    = 3;
    GROUP_JOINED    = 4;
    GROUP_LEFT      = 5;
    SETTINGS_CHANGED = 6;
    HISTORY_REQUEST  = 7;
    HISTORY_BATCH    = 8;
}
```

### 7.3 History Sync

1. New device sends `HISTORY_REQUEST` on the sync topic, specifying desired time range.
2. Existing online devices respond with `HISTORY_BATCH` messages.
3. History sent in reverse chronological order, batches of 100 messages.
4. Plaintext + metadata are synced (not raw encrypted envelopes, since the new device cannot decrypt envelopes encrypted for other devices).

### 7.4 Conflict Resolution

- **Contacts and settings**: Last-writer-wins by wall clock timestamp, vector clock as tiebreaker.
- **Read receipts**: Maximum timestamp wins (once read, always read).
- **Group state**: Longest valid chain is authoritative (see §5.1).

### 7.5 Device Revocation

```protobuf
message DeviceRevocation {
    uint32 revoked_device_id  = 1;
    bytes  revoker_pubkey     = 2;  // IK_sign.pub
    uint64 revoked_at         = 3;
    string reason             = 4;
    bytes  identity_signature = 5;  // Signed by IK_sign
}
```

1. Revocation published to sync topic and included in updated IdentityDocument.
2. Contacts who fetch the updated document see the device is no longer listed.
3. Senders stop encrypting to the revoked device.

---

## 8. Voice & Video Calls

### 8.1 Signaling

RTC signaling messages are carried as DM messages through the existing Double Ratchet session (end-to-end encrypted signaling).

**1:1 call flow**:

```
1. Caller creates call_id (UUID), generates SDP offer
2. Caller → RTC_OFFER (SDP + call_id + video flag) → callee's DM topic
3. Callee receives, presents incoming call notification
4. If callee accepts: generates SDP answer
5. Callee → RTC_ANSWER (SDP) → caller's DM topic
6. Both exchange RTC_ICE_CANDIDATE messages until connectivity established
7. Media flows peer-to-peer once ICE succeeds
8. Either party → RTC_HANGUP to end call
```

**Timeouts**:
- Offer expires: 60 seconds (no answer)
- ICE gathering: 30 seconds
- No media timeout: 15 seconds after answer → re-negotiate or terminate

### 8.2 Media Transport

Uses a dedicated libp2p stream protocol: `/bt/media/1.0.0`

1. After signaling completes, both peers open a libp2p stream.
2. DTLS handshake occurs over the stream.
3. SRTP session keys are bound to the messaging session: `media_key = HKDF(session_root_key, salt=call_id, info="bt-media-v1", len=32)`
4. Media flows as SRTP over the libp2p stream.
5. Fallback: circuit relay v2 as TURN equivalent.

### 8.3 Codecs

- **Audio**: Opus (MANDATORY). Default: 48kHz mono, 32kbps.
- **Video**: VP8 (MANDATORY baseline), VP9 (preferred), AV1 (optional). Default: VP9, 720p@30fps, 1.5Mbps.

Negotiation follows standard SDP offer/answer semantics.

### 8.4 Group Calls

**Mesh topology (2-6 participants)**:

Full mesh — each participant sends their media to every other participant. N participants = N-1 outgoing streams each.

**SFU topology (7-25 participants)**:

1. One participant volunteers as the **SFU (Selective Forwarding Unit)**.
2. All other participants send their media only to the SFU.
3. SFU selectively forwards encrypted SRTP packets to other participants.
4. **SFU cannot decrypt media** — it forwards encrypted packets based on SSRC and packet rate (as proxy for audio activity).
5. SFU election: lowest lexicographic pubkey among connected participants.
6. SFU re-election on disconnect: next lowest pubkey takes over, participants re-establish streams.

**Maximum**: 25 participants recommended.

---

## 9. Network Contribution

Every desktop node participates as a full network citizen. More clients = faster, more reliable communications.

| Role | Description | Configuration |
|------|-------------|---------------|
| **DHT server** | Full DHT mode, stores and serves records | `dht.ModeServer` |
| **PubSub mesh** | Active GossipSub participant, forwards gossip | Default with `D_low=4` |
| **Relay** | Optional circuit relay v2 for NAT-blocked peers | `libp2p.EnableRelay()` |
| **Mailbox** | Store-and-forward for offline contacts | Opt-in, advertised via DHT |
| **Prekey cache** | Cache and serve prekey bundles for contacts | Cache TTL: 4 hours |
| **Content routing** | IPFS bitswap for media files | Default participation |

Capabilities are advertised in `IdentityDocument.features.custom_features` using strings like `"dht-full"`, `"relay-v2"`, `"mailbox-v1"`, `"prekey-cache"`, `"content-route"`.

---

## 10. Protocol Versioning & Updates

### 10.1 Version Negotiation

1. `BabylonEnvelope.protocol_version` indicates the wire format version of each message.
2. `IdentityDocument.supported_versions` advertises all versions a client supports.
3. Sender selects: `max(intersect(own_supported_versions, recipient_supported_versions))`

| Version | Features |
|---------|----------|
| 0 | PoC (unversioned): static X25519 ECDH, XChaCha20-Poly1305, SignedEnvelope format, single device |
| 1 | X3DH, Double Ratchet, Sender Keys, BabylonEnvelope, multi-device, groups, channels, RTC, mailbox |

### 10.2 Backward Compatibility

- **v1 client receiving PoC message**: Detect by absence of `protocol_version` field. Parse as legacy `SignedEnvelope` (PoC format). Decrypt using static X25519 ECDH (no ratchet). Display with `[PoC]` indicator.
- **PoC client receiving v1 message**: Cannot parse `BabylonEnvelope`. Message is silently dropped. Users must upgrade.
- **Transition period**: v1 clients subscribe to BOTH `babylon-` (PoC format) and `babylon-dm-` (v1 format) topics.

### 10.3 Future Extensions

- **New protobuf fields**: Always optional. Old clients ignore unknown fields (proto3 behavior).
- **New message types**: Added to `MessageType` enum. Old clients ignore unknown types.
- **New cipher suites**: Added to suite registry. Negotiation ensures fallback to mandatory suite.
- **Breaking changes**: Require new major version. Clients support two versions simultaneously during transition (e.g., v1 and v2).
- **Deprecation timeline**: Once >95% of active nodes support v(N+1), v(N) is deprecated with a 6-month sunset.
- **Custom extensions**: Namespaced as `"babylon.ext.<vendor>.<feature>"` in `FeatureFlags.custom_features`.

---

## 11. Storage Schema (BadgerDB Key Prefixes)

Extends the existing v1 schema:

```
Existing (v1):
    c:     + <pubkey>                          → Contact records (protobuf)
    m:     + <pubkey> + <ts> + <nonce>         → Message envelopes (protobuf)
    p:     + <peer_id>                         → Peer records (JSON)
    cfg:   + <config_key>                      → Configuration values
    bl:    + <peer_id>                         → Blacklist entries

New (v1):
    id:    + <pubkey>                          → Cached IdentityDocuments
    pk:    + <pubkey>                          → Cached PrekeyBundles
    otpk:  + <prekey_id>                       → One-time prekey private keys
    spk:   + <signed_prekey_id>                → Signed prekey private keys
    dr:    + <pubkey> + <device_id>            → Double Ratchet session state
    gs:    + <group_id>                        → Group state
    sk:    + <group_id> + <member_pubkey>      → Sender key chains
    mb:    + <target_pubkey> + <msg_id>        → Mailbox stored messages (for mailbox nodes)
    rep:   + <peer_id>                         → Peer reputation records
    dev:   + <device_id>                       → Device certificates and private keys
    un:    + <username>                        → Username record cache
```

---

## 12. Reputation Rewards System

Nodes that contribute resources earn quantified reputation that translates to priority service. High-reputation nodes get chosen first for critical roles (mailbox, relay, SFU), giving them better connectivity and mesh positioning.

### 12.1 Reputation Metrics

Each node locally tracks other peers across these dimensions:

| Metric | Weight | Measurement |
|--------|--------|-------------|
| Relay reliability | 0.25 | Messages successfully relayed / total relay requests |
| Uptime consistency | 0.20 | Hours online in last 7 days / 168 |
| Mailbox reliability | 0.25 | Messages retrievable / messages deposited |
| DHT responsiveness | 0.15 | 1 - (avg_response_ms / 5000), clamped to [0, 1] |
| Content serving | 0.15 | IPFS blocks served / blocks requested from this peer |

**Composite score**: weighted sum, range [0.0, 1.0].

### 12.2 Reputation Tiers

| Score Range | Tier | Benefits |
|-------------|------|----------|
| 0.0 - 0.3 | Basic | Standard service, no priority |
| 0.3 - 0.6 | Contributor | Preferred for relay selection, mailbox queries answered first |
| 0.6 - 0.8 | Reliable | Priority GossipSub mesh grafting, prekey cache priority |
| 0.8 - 1.0 | Trusted | First choice for SFU in group calls, mailbox with extended TTL |

### 12.3 Reputation Exchange

Reputation is **local-first** — each node computes its own scores. Optionally, nodes can share attestations:

```protobuf
message ReputationAttestation {
    bytes  attester_pubkey           = 1;
    bytes  subject_peer_id           = 2;
    float  score                     = 3;  // Attester's computed score for subject
    uint64 observation_period_hours  = 4;
    uint64 timestamp                 = 5;
    bytes  signature                 = 6;
}
```

- Published to DHT at `SHA256("bt-rep-v1:" ‖ subject_peer_id)`.
- Consuming nodes weight attestations by the attester's own reputation (trust transitivity).
- Anti-gaming: no single attestation can move a score by more than 0.1. Sybil resistance via requiring attesters to have been connected for >24 hours.

### 12.4 Incentive Alignment

- Nodes benefit from helping because a healthier network delivers their own messages faster and more reliably.
- High-reputation nodes are chosen first for critical roles, giving them more connections and better mesh positioning — a positive feedback loop.
- Malicious or freeloading nodes naturally get deprioritized as their scores drop.

---

## 13. Future Layer: Metadata Privacy (Stub)

> **Status**: This section outlines a future anonymity layer. Not specified for v1 implementation.

### 13.1 Problem Statement

PubSub topic subscriptions reveal communication patterns. An observer can determine who communicates with whom by monitoring topic membership. GossipSub gossip (IHAVE/IWANT) further leaks message flow information.

### 13.2 Proposed Approach

A future protocol version (v3+) should consider:

1. **Sphinx Packet Format**: Wrap BabylonEnvelopes in Sphinx packets with layered encryption through a 3-hop circuit of relay nodes.
2. **Circuit Construction**: Initiator selects 3 relay nodes from high-reputation peers, builds an onion circuit.
3. **Rendezvous Points**: Instead of subscribing to personal topics (which reveal identity), recipients publish rendezvous descriptors to the DHT. Senders deliver messages through the rendezvous point.
4. **Cover Traffic**: Optional dummy messages at constant rate to mask communication patterns.
5. **Mix Network Alternative**: For asynchronous messages, a Loopix-style mix network with Poisson mixing delays for stronger anonymity.

### 13.3 Constraints

- Must integrate with existing libp2p transport.
- Latency overhead should be <500ms for interactive messaging.
- Cover traffic bandwidth must be bounded (configurable, e.g., 1 KB/s).
- Incompatible with real-time voice/video (latency too high) — RTC continues using direct connections.
- Requires a large enough network (>1000 nodes) for meaningful anonymity sets.
- Requires reliable reputation system to select trustworthy relay nodes.

---

## 14. Threat Model & Security Properties

### 14.1 Security Properties

| Property | Mechanism |
|----------|-----------|
| **DM confidentiality** | X3DH + Double Ratchet (XChaCha20-Poly1305 AEAD) |
| **Group confidentiality** | Sender Keys with per-member chain ratchets |
| **Forward secrecy** | Double Ratchet DH ratcheting (new DH per round-trip) |
| **Post-compromise security** | Double Ratchet (recovery after one round-trip) |
| **Authentication** | Ed25519 signatures on all messages |
| **Integrity** | AEAD (XChaCha20-Poly1305) + Ed25519 signatures |
| **Replay protection** | Unique message_id + timestamps + chain counters |
| **Device isolation** | Independent device keys (random, not from mnemonic), revocable |
| **Group forward secrecy** | Sender Key rotation on membership change |
| **Identity binding** | IdentityDocument hash chain signed by root key, published to DHT |

### 14.2 Threat Mitigations

| Threat | Mitigation |
|--------|------------|
| Mnemonic compromise | Sole user secret. No server-side mitigation in serverless design. |
| Device compromise | Independent device keys. Revocation invalidates device. Double Ratchet provides break-in recovery. |
| Network eavesdropping | All messages E2E encrypted. libp2p Noise protocol encrypts transport. |
| Replay attacks | Unique message_id per message. Receivers track seen IDs. Timestamps within 24h window. |
| Man-in-the-middle | X3DH binds both identity keys. Out-of-band fingerprint verification provides safety numbers. |
| Denial of service | Rate limiting at PubSub level. max_skip=256 bounds ratchet fast-forward. GossipSub scoring penalizes flood. |
| OPK exhaustion | Graceful fallback to 3-DH. Replenishment protocol maintains pool. |
| Stale prekeys | SPK has 14-day max age. Clients reject expired prekeys. |
| Group member removal | Sender Key rotation ensures removed members cannot decrypt future messages. |
| Clock manipulation | 24-hour tolerance. Sequence numbers provide ordering independent of timestamps. |

### 14.3 Known Limitations

- **Metadata leakage**: PubSub topics reveal communication patterns. Future onion routing layer planned (§13).
- **DHT eventual consistency**: Prekey consumption races, identity document propagation delays. Protocol degrades gracefully.
- **No guaranteed delivery**: Best-effort with redundant mailboxes (up to 3).
- **Group size**: Private groups limited to ~1000 members due to O(N²) key distribution on membership change.
- **Clock dependence**: Message ordering relies on timestamps. No NTP requirement enforced.
- **Mnemonic = identity**: Sole secret, no server-side recovery possible. If mnemonic is lost, identity is lost.

---

*Last updated: February 25, 2026*
*Version: 1.0.0 (First official protocol specification)*
