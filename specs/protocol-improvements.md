# Babylon Tower Protocol Improvements - Implementation Plan

**Version**: 2.0.0  
**Status**: Planning  
**Created**: February 23, 2026  
**Target Completion**: Q2 2026

---

## Executive Summary

This document outlines the implementation plan for evolving Babylon Tower from a PoC to a production-ready, extensible P2P messaging protocol. The new architecture supports multiple encryption schemes, diverse messaging use cases, and a distributed address book system.

### Goals

1. **Independent and extensible protocol** - Modular design with versioning and extension mechanisms
2. **Flexible encryption** - Support for XChaCha20-Poly1305, Double Ratchet, and future schemes
3. **Multiple use cases** - Online/offline messages, private/public groups, channels, service messages
4. **Global distributed address book** - DHT-based contact discovery and verification
5. **Service information publishing** - Peer address and capability advertisement

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Babylon Tower v2                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │  Messaging   │  │   Address    │  │   Service    │          │
│  │   Protocol   │  │    Book      │  │   Publisher  │          │
│  │   (v2.0.0)   │  │  (DHT+IPFS)  │  │  (Identify)  │          │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │
│         │                 │                 │                   │
│  ┌──────▼─────────────────▼─────────────────▼───────┐          │
│  │           Encryption Abstraction Layer            │          │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ │          │
│  │  │ XChaCha20   │ │ Double      │ │ Future      │ │          │
│  │  │ Poly1305    │ │ Ratchet     │ │ Schemes     │ │          │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ │          │
│  └───────────────────────────────────────────────────┘          │
│         │                 │                 │                   │
│  ┌──────▼─────────────────▼─────────────────▼───────┐          │
│  │              IPFS / libp2p Stack                  │          │
│  │  DHT │ PubSub │ Blockstore │ Identify │ Relay    │          │
│  └───────────────────────────────────────────────────┘          │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Protocol Versioning

### Protocol IDs

```
/babylontower/1.0.0          # Legacy (current PoC)
/babylontower/2.0.0/envelope  # New envelope protocol
/babylontower/2.0.0/addressbook # Address book protocol
/babylontower/2.0.0/service   # Service info protocol
/babylontower/2.0.0/group     # Group messaging protocol
/babylontower/2.0.0/kad/1.0.0 # DHT protocol (isolated)
```

### Version Negotiation

```go
var ProtocolPreferences = []protocol.ID{
    "/babylontower/2.0.0/envelope",
    "/babylontower/1.0.0",  // Fallback for backward compat
}

// During connection setup
selected, err := host.NewStream(ctx, peerID, ProtocolPreferences...)
```

---

## Message Type Taxonomy

| Type | ID | Use Case | Encryption | Routing |
|------|-----|----------|------------|---------|
| `DIRECT_ONLINE` | 1 | Real-time 1:1 | X25519 + XChaCha20 | Recipient topic |
| `DIRECT_OFFLINE` | 2 | Stored for later | X25519 + XChaCha20 | Offline bundle |
| `GROUP_PRIVATE` | 3 | Member-only group | Group key (AES-256) | Group topic |
| `GROUP_PUBLIC` | 4 | Open group | Group key (shared) | Group topic |
| `CHANNEL_PUBLIC` | 5 | Public channel | Optional | Channel topic |
| `CHANNEL_PRIVATE` | 6 | Invite-only channel | Channel key | Channel topic |
| `SERVICE_ANNOUNCE` | 7 | Peer info publication | Signed, public | Service topic |
| `ADDRESS_UPDATE` | 8 | Contact record update | Signed, public | Contact topic |
| `KEY_ROTATION` | 9 | Double Ratchet DH update | Signed | Direct topic |
| `GROUP_INVITE` | 10 | Group membership invite | Encrypted to invitee | Direct topic |

---

## Implementation Phases

### Phase 1: Foundation (Weeks 1-2)

**Goal**: Add protocol versioning and fix current limitations

#### 1.1 Protocol Versioning in Envelope

**Files**: `pkg/proto/message.proto`, `pkg/messaging/`

```protobuf
message SignedEnvelope {
  bytes envelope = 1;
  bytes signature = 2;
  bytes sender_pubkey = 3;
  bytes encrypted_ephemeral_priv = 4;
  
  // NEW
  uint32 protocol_version = 5;  // Default: 1
  EncryptionScheme encryption_scheme = 6;  // Default: XCHACHA20
}

enum EncryptionScheme {
  SCHEME_UNSPECIFIED = 0;
  SCHEME_XCHACHA20_POLY1305 = 1;
  SCHEME_DOUBLE_RATCHET = 2;
}
```

**Tasks**:
- [ ] Add `protocol_version` and `encryption_scheme` fields to protobuf
- [ ] Regenerate protobuf code
- [ ] Update envelope creation to set version = 2
- [ ] Add version checking in envelope processing
- [ ] Maintain backward compatibility with version = 1

#### 1.2 Contact X25519 Key Storage

**Files**: `pkg/storage/contacts.go`, `pkg/messaging/service.go`

```go
// New storage methods
func (s *Storage) AddContactX25519Key(contactPubKey, x25519PubKey []byte) error
func (s *Storage) GetContactX25519Key(contactPubKey []byte) ([]byte, error)
func (s *Storage) DeleteContactX25519Key(contactPubKey []byte) error
```

**Tasks**:
- [ ] Add X25519 public key field to Contact struct
- [ ] Implement storage/retrieval methods
- [ ] Update `/add` CLI command to accept X25519 key
- [ ] Update messaging to store recipient's X25519 key
- [ ] Add migration for existing contacts (manual re-add)

#### 1.3 Protocol Registry

**Files**: `pkg/messaging/protocol.go` (new)

```go
type ProtocolRegistry struct {
    mu sync.RWMutex
    schemes map[EncryptionSchemeID]EncryptionScheme
    versions map[uint32]ProtocolHandler
}

func (r *ProtocolRegistry) RegisterScheme(scheme EncryptionScheme)
func (r *ProtocolRegistry) GetScheme(id EncryptionSchemeID) (EncryptionScheme, error)
func (r *ProtocolRegistry) SelectScheme(preferences []EncryptionSchemeID) (EncryptionSchemeID, error)
```

**Tasks**:
- [ ] Create `EncryptionScheme` interface
- [ ] Implement `XChaCha20Scheme` wrapper (minimal, no Double Ratchet yet)
- [ ] Create protocol registry with thread-safe access
- [ ] Add scheme negotiation logic (stub for future)
- [ ] Write unit tests for registry

**Deliverables**:
- ✅ Updated protobuf schema with versioning
- ✅ X25519 key storage in BadgerDB
- ✅ Protocol registry with encryption scheme abstraction (XChaCha20 only)
- ✅ Backward compatibility maintained

---

### Phase 2: Distributed Address Book (Weeks 3-4)

**Goal**: Implement DHT-based contact discovery system

#### 2.1 Contact Record Structure

**Files**: `pkg/proto/v2/addressbook.proto` (new)

```protobuf
message ContactRecord {
  bytes ed25519_pubkey = 1;
  bytes x25519_pubkey = 2;
  string display_name = 3;
  string avatar_cid = 4;
  uint64 sequence_num = 5;
  uint64 timestamp = 6;
  bytes previous_hash = 7;
  bytes encrypted_service_info = 8;
  bytes signature = 15;
}

message ContactRecordEnvelope {
  ContactRecord record = 1;
  bytes record_bytes = 2;
  bytes signature = 3;
}
```

**Tasks**:
- [ ] Define protobuf schema for contact records
- [ ] Generate Go code for v2 protos
- [ ] Implement record signing/verification
- [ ] Implement record chain validation (previous_hash)

#### 2.2 Address Book Core

**Files**: `pkg/addressbook/addressbook.go` (new)

```go
type AddressBook struct {
    dht *dht.IpfsDHT
    ipfs *ipfsnode.Node
    cache *lru.Cache
    privKey ed25519.PrivateKey
}

func (ab *AddressBook) Publish(ctx context.Context, record *ContactRecord) error
func (ab *AddressBook) Lookup(ctx context.Context, pubkey []byte) (*ContactRecord, error)
func (ab *AddressBook) Search(ctx context.Context, query string) ([]*ContactRecord, error)
func (ab *AddressBook) SubscribeUpdates() (<-chan *ContactUpdateNotification, error)
```

**DHT Key Derivation**:
```go
func ContactRecordDHTKey(ed25519PubKey []byte) string {
    hash := sha256.Sum256(ed25519PubKey)
    return fmt.Sprintf("/babylontower/contact/%x", hash[:16])
}
```

**Tasks**:
- [ ] Implement DHT put/get for contact records
- [ ] Add IPFS block storage for record persistence
- [ ] Implement LRU cache with TTL (1 hour default)
- [ ] Add record verification (signature, chain, timestamp)
- [ ] Implement search index (name hash → records)

#### 2.3 Update Propagation

**Files**: `pkg/addressbook/updates.go` (new)

```protobuf
message ContactUpdateNotification {
  bytes contact_pubkey = 1;
  uint64 new_sequence_num = 2;
  bytes new_record_cid = 3;
  uint64 timestamp = 4;
  bytes signature = 5;
}
```

**Tasks**:
- [ ] Subscribe to `/babylontower/contact-updates` PubSub topic
- [ ] Publish notification on record update
- [ ] Implement notification verification
- [ ] Fetch and validate new records on notification
- [ ] Handle concurrent updates (use highest sequence)

#### 2.4 CLI Integration

**Files**: `pkg/cli/commands.go`

**New Commands**:
```
/publish-contact [name]  - Publish own contact record to DHT
/lookup-contact <pubkey> - Lookup contact record from DHT
/search-contact <name>   - Search for contacts by name
/refresh-contact <pubkey> - Force refresh from DHT
```

**Tasks**:
- [ ] Add `/publish-contact` command
- [ ] Add `/lookup-contact` command
- [ ] Add `/search-contact` command
- [ ] Add `/refresh-contact` command
- [ ] Display contact record info (sequence, timestamp, keys)

**Deliverables**:
- ✅ Contact record protobuf schema
- ✅ AddressBook implementation with DHT + cache
- ✅ Update propagation via PubSub
- ✅ CLI commands for contact management

---

### Phase 3: Group Messaging (Weeks 5-6)

**Goal**: Support private and public group messaging

#### 3.1 Group Data Structures

**Files**: `pkg/proto/v2/group.proto` (new)

```protobuf
message Group {
  bytes group_id = 1;           // Random 32-byte ID
  bytes creator_pubkey = 2;
  string name = 3;
  string description = 4;
  uint64 created_at = 5;
  bool is_public = 6;           // Public = anyone can join
  EncryptionScheme encryption_scheme = 7;
  bytes group_key_cid = 8;      // IPFS CID of encrypted group key
  repeated bytes member_pubkeys = 9;
  bytes admin_pubkey = 10;      // Group admin
  uint64 max_members = 11;
  bytes signature = 15;
}

message GroupKeyShare {
  bytes group_id = 1;
  bytes encrypted_group_key = 2;  // Encrypted to recipient's X25519
  bytes sender_pubkey = 3;
  uint64 sequence_num = 4;
  bytes signature = 5;
}

message GroupInvite {
  bytes group_id = 1;
  bytes invitee_pubkey = 2;
  bytes encrypted_group_key = 3;
  string inviter_name = 4;
  string group_name = 5;
  uint64 expires_at = 6;
  bytes signature = 15;
}

message GroupMembership {
  bytes group_id = 1;
  bytes member_pubkey = 2;
  uint64 joined_at = 3;
  string role = 4;  // "member", "admin", "creator"
  bytes signature = 15;
}
```

**Tasks**:
- [ ] Define group protobuf schema
- [ ] Implement group ID generation (random 32-byte)
- [ ] Add group key generation (AES-256-GCM)
- [ ] Implement group key encryption per member
- [ ] Add membership tracking

#### 3.2 Group Manager

**Files**: `pkg/messaging/group.go` (new)

```go
type GroupManager struct {
    storage *storage.Storage
    ipfs *ipfsnode.Node
    messaging *MessagingService
    privKey ed25519.PrivateKey
    groups map[string]*Group  // group_id → Group
    mu sync.RWMutex
}

func (gm *GroupManager) CreateGroup(name string, members [][]byte, isPublic bool) (*Group, error)
func (gm *GroupManager) InviteMember(groupID, memberPubKey []byte) error
func (gm *GroupManager) RemoveMember(groupID, memberPubKey []byte) error
func (gm *GroupManager) SendMessage(groupID []byte, text string) error
func (gm *GroupManager) ListGroups() []*Group
func (gm *GroupManager) GetGroup(groupID []byte) (*Group, error)
func (gm *GroupManager) AcceptInvite(invite *GroupInvite) error
```

**Group Creation Flow**:
```
1. Generate group_id (random 32 bytes)
2. Generate group_key (AES-256)
3. For each member:
   - Fetch member's X25519 pubkey from address book
   - Encrypt group_key with member's X25519
   - Store encrypted key share in IPFS
4. Create Group record with member list
5. Sign and publish Group to IPFS
6. Send GroupInvite to each member via direct message
7. Subscribe to group topic: /babylontower/group/<hash(group_id)>
```

**Tasks**:
- [ ] Implement group creation with key generation
- [ ] Add member invitation with encrypted key share
- [ ] Implement group message encryption (AES-256-GCM)
- [ ] Add group topic subscription
- [ ] Implement member removal (key rotation required)
- [ ] Add invite acceptance flow
- [ ] Write unit tests for group operations

#### 3.3 Group Key Rotation

**Files**: `pkg/messaging/group_rotation.go` (new)

```go
func (gm *GroupManager) RotateGroupKey(groupID []byte) error {
    // 1. Generate new group key
    newKey := generateAES256Key()
    
    // 2. Get current member list
    group := gm.groups[string(groupID)]
    
    // 3. Re-encrypt new key for each member
    for _, memberPubKey := range group.MemberPubkeys {
        encrypted := encryptKeyForMember(newKey, memberPubKey)
        // Store in IPFS...
    }
    
    // 4. Publish key rotation message
    rotation := &KeyRotationMessage{
        GroupId: groupID,
        NewKeyCid: newKeyCID,
        SequenceNum: group.KeyRotationCount + 1,
    }
    // Publish to group topic...
    
    // 5. Update local group state
    group.GroupKey = newKey
    group.KeyRotationCount++
}
```

**Tasks**:
- [ ] Implement key rotation trigger (member removal)
- [ ] Add rotation message to group topic
- [ ] Handle rotation on receive (update local key)
- [ ] Add rotation count tracking
- [ ] Test rotation with message ordering

#### 3.4 CLI Integration

**Files**: `pkg/cli/commands.go`

**New Commands**:
```
/group create <name> [members...]  - Create new group
/group list                        - List all groups
/group invite <group> <pubkey>     - Invite member to group
/group leave <group>               - Leave group
/group info <group>                - Show group details
/chat <group>                      - Enter group chat mode
```

**Tasks**:
- [ ] Add `/group create` command
- [ ] Add `/group list` command
- [ ] Add `/group invite` command
- [ ] Add `/group leave` command
- [ ] Add `/group info` command
- [ ] Update `/chat` to accept group names
- [ ] Display group messages with sender info

**Deliverables**:
- ✅ Group protobuf schema
- ✅ GroupManager with CRUD operations
- ✅ Group key distribution and rotation
- ✅ Group messaging with AES-256-GCM
- ✅ CLI commands for group management

---

### Phase 4: Service Information Publishing (Weeks 7-8)

**Goal**: Implement peer service discovery and capability advertisement

#### 4.1 Service Info Structure

**Files**: `pkg/proto/v2/service.proto` (new)

```protobuf
message ServiceInfo {
  bytes peer_id = 1;
  bytes ed25519_pubkey = 2;
  repeated string multiaddrs = 3;
  repeated string supported_protocols = 4;
  map<string, string> protocol_versions = 5;
  bool supports_offline_messages = 6;
  bool supports_group_messaging = 7;
  bool supports_double_ratchet = 8;
  repeated EncryptionScheme supported_encryption = 9;
  bool accepts_relay_connections = 10;
  repeated string preferred_relays = 11;
  uint64 last_seen = 12;
  uint64 ttl_seconds = 13;
  bytes encrypted_for_viewer = 14;  // Contact-specific encryption
  bytes signature = 15;
}

message PeerAddressBook {
  bytes peer_id = 1;
  repeated string multiaddrs = 2;
  uint64 last_connected = 3;
  uint64 connection_count = 4;
  bool is_relay = 5;
}
```

**Tasks**:
- [ ] Define service info protobuf schema
- [ ] Add protocol capability fields
- [ ] Add encrypted viewer-specific section
- [ ] Implement TTL-based expiration

#### 4.2 Service Publisher

**Files**: `pkg/messaging/service.go` (new)

```go
type ServicePublisher struct {
    host host.Host
    dht *dht.IpfsDHT
    pubsub *pubsub.PubSub
    storage *storage.Storage
    privKey ed25519.PrivateKey
    info *ServiceInfo
    mu sync.RWMutex
}

func (sp *ServicePublisher) Initialize() error
func (sp *ServicePublisher) Publish(ctx context.Context) error
func (sp *ServicePublisher) Lookup(ctx context.Context, peerID string) (*ServiceInfo, error)
func (sp *ServicePublisher) Subscribe() (<-chan *ServiceInfo, error)
func (sp *ServicePublisher) EncryptForContact(info *ServiceInfo, contactPubKey []byte) ([]byte, error)
```

**Publication Flow**:
```
1. Gather local info:
   - Peer ID from libp2p host
   - Multiaddrs from host.Addrs()
   - Supported protocols from host.Mux()
   - Capabilities from local config

2. Sign service info with Ed25519

3. Publish via multiple channels:
   a) libp2p Identify Protocol (immediate peers)
   b) DHT provider record (discovery)
   c) PubSub /babylontower/service-announce (contacts)

4. Set up periodic refresh (every TTL/2)
```

**Tasks**:
- [ ] Implement service info gathering from libp2p host
- [ ] Add DHT provider record publication
- [ ] Subscribe to service announce topic
- [ ] Implement contact-specific encryption
- [ ] Add periodic refresh timer
- [ ] Handle TTL expiration

#### 4.3 libp2p Identify Integration

**Files**: `pkg/ipfsnode/node.go`

```go
// Extend existing node setup
func (n *Node) setupIdentify() error {
    // Register protocol handlers
    n.host.SetStreamHandler("/babylontower/2.0.0/envelope", n.handleEnvelopeStream)
    n.host.SetStreamHandler("/babylontower/2.0.0/service", n.handleServiceStream)
    
    // Advertise protocols via identify
    identifyService, _ := identify.NewIDService(n.host)
    n.host.SetStreamHandler(identify.ID, identifyService.IDService().IDFunc)
    
    return nil
}
```

**Tasks**:
- [ ] Register protocol handlers with libp2p host
- [ ] Integrate with libp2p identify protocol
- [ ] Advertise supported protocols in identify response
- [ ] Handle incoming protocol stream requests

#### 4.4 Peer Discovery and Connection

**Files**: `pkg/messaging/peer_discovery.go` (new)

```go
type PeerDiscovery struct {
    dht *dht.IpfsDHT
    host host.Host
    knownPeers map[string]*PeerInfo
    mu sync.RWMutex
}

type PeerInfo struct {
    PeerID string
    Multiaddrs []string
    LastSeen time.Time
    Connected bool
    ServiceInfo *ServiceInfo
}

func (pd *PeerDiscovery) FindPeers(ctx context.Context, targetPeerID string) ([]PeerInfo, error)
func (pd *PeerDiscovery) ConnectToPeer(ctx context.Context, peerID string, addrs []string) error
func (pd *PeerDiscovery) MaintainConnections() error
```

**Tasks**:
- [ ] Implement peer lookup via DHT
- [ ] Add connection management (connect/disconnect)
- [ ] Implement connection maintenance (keep-alive)
- [ ] Add peer info caching
- [ ] Handle NAT traversal (relay fallback)

#### 4.5 CLI Integration

**Files**: `pkg/cli/commands.go`

**New Commands**:
```
/myinfo                    - Show own service information
/peerinfo <peerid>         - Lookup peer service information
/peers                     - List known peers and status
/connect <peerid> [addr]   - Connect to specific peer
```

**Tasks**:
- [ ] Add `/myinfo` command
- [ ] Add `/peerinfo` command
- [ ] Add `/peers` command
- [ ] Add `/connect` command
- [ ] Display peer capabilities and protocols

**Deliverables**:
- ✅ Service info protobuf schema
- ✅ ServicePublisher with multi-channel publication
- ✅ libp2p identify integration
- ✅ Peer discovery and connection management
- ✅ CLI commands for peer information

---

### Phase 5: Offline Message Support (Week 9)

**Goal**: Enable message delivery when recipient is offline

#### 5.1 Offline Message Bundle

**Files**: `pkg/proto/v2/offline.proto` (new)

```protobuf
message OfflineMessageBundle {
  bytes recipient_pubkey = 1;
  repeated bytes message_cids = 2;  // IPFS CIDs of encrypted messages
  uint64 created_at = 3;
  uint64 expires_at = 4;
  uint64 sequence_num = 5;
  bytes signature = 6;
}

message OfflineMessageNotification {
  bytes sender_pubkey = 1;
  uint64 message_count = 2;
  bytes bundle_cid = 3;
  uint64 timestamp = 4;
  bytes signature = 5;
}
```

**Tasks**:
- [ ] Define offline message protobuf schema
- [ ] Implement bundle creation with message CIDs
- [ ] Add bundle expiration (7 days default)
- [ ] Implement notification system

#### 5.2 Offline Message Storage

**Files**: `pkg/messaging/offline.go` (new)

```go
type OfflineMessageStore struct {
    ipfs *ipfsnode.Node
    dht *dht.IpfsDHT
    pubsub *pubsub.PubSub
    storage *storage.Storage
}

func (os *OfflineMessageStore) StoreMessage(ctx context.Context, recipientPubKey []byte, envelope *SignedExtendedEnvelope) (string, error)
func (os *OfflineMessageStore) FetchMessages(ctx context.Context) ([]*SignedExtendedEnvelope, error)
func (os *OfflineMessageStore) NotifyRecipient(ctx context.Context, recipientPubKey []byte, bundleCID string) error
func (os *OfflineMessageStore) CleanupExpired(ctx context.Context) error
```

**Storage Flow**:
```
1. Encrypt message normally (XChaCha20 or Double Ratchet)
2. Store encrypted envelope in IPFS blockstore → get CID
3. Add CID to recipient's offline bundle
4. Sign bundle
5. Store bundle in IPFS
6. Publish notification to /babylontower/offline/<hash(recipient)>
```

**Retrieval Flow**:
```
1. Recipient comes online
2. Joins /babylontower/offline/<hash(own_pubkey)> topic
3. Receives notification with bundle CID
4. Fetches bundle from IPFS
5. Verifies bundle signature
6. Fetches each message CID from IPFS
7. Decrypts and stores messages locally
8. Displays to user
```

**Tasks**:
- [ ] Implement message storage in IPFS
- [ ] Create and sign offline bundles
- [ ] Publish notifications to offline topic
- [ ] Implement message retrieval on reconnect
- [ ] Add cleanup for expired messages
- [ ] Write integration tests

#### 5.3 CLI Integration

**Files**: `pkg/cli/commands.go`

**New Commands**:
```
/offline check             - Check for offline messages
/offline list              - List pending offline messages
/offline cleanup           - Remove expired messages
```

**Tasks**:
- [ ] Add `/offline check` command
- [ ] Add `/offline list` command
- [ ] Add `/offline cleanup` command
- [ ] Auto-check on startup

**Deliverables**:
- ✅ Offline message bundle schema
- ✅ OfflineMessageStore with IPFS storage
- ✅ Notification system for offline messages
- ✅ Message retrieval on reconnect

---

### Phase 6: Channel Support (Week 10)

**Goal**: Implement public and private channels

#### 6.1 Channel Data Structures

**Files**: `pkg/proto/v2/channel.proto` (new)

```protobuf
message Channel {
  bytes channel_id = 1;
  bytes creator_pubkey = 2;
  string name = 3;
  string description = 4;
  bool is_public = 5;       // Public = discoverable, anyone can read
  bool is_private = 6;      // Private = invite-only
  uint64 created_at = 7;
  bytes encryption_key_cid = 8;  // IPFS CID of encrypted channel key
  repeated bytes member_pubkeys = 9;
  uint64 max_members = 10;
  bytes signature = 15;
}

message ChannelInvite {
  bytes channel_id = 1;
  bytes invitee_pubkey = 2;
  bytes encrypted_channel_key = 3;
  string inviter_name = 4;
  string channel_name = 5;
  uint64 expires_at = 6;
  bytes signature = 15;
}

message ChannelMessage {
  bytes channel_id = 1;
  bytes sender_pubkey = 2;
  bytes ciphertext = 3;
  bytes ephemeral_pubkey = 4;
  bytes nonce = 5;
  uint64 timestamp = 6;
  bytes signature = 15;
}
```

**Tasks**:
- [ ] Define channel protobuf schema
- [ ] Implement channel ID generation
- [ ] Add channel key management
- [ ] Implement invite system (private channels)

#### 6.2 Channel Manager

**Files**: `pkg/messaging/channel.go` (new)

```go
type ChannelManager struct {
    storage *storage.Storage
    ipfs *ipfsnode.Node
    messaging *MessagingService
    privKey ed25519.PrivateKey
    channels map[string]*Channel
    mu sync.RWMutex
}

func (cm *ChannelManager) CreateChannel(name string, isPublic bool) (*Channel, error)
func (cm *ChannelManager) JoinChannel(channelID []byte, invite *ChannelInvite) error
func (cm *ChannelManager) LeaveChannel(channelID []byte) error
func (cm *ChannelManager) SendMessage(channelID []byte, text string) error
func (cm *ChannelManager) ListChannels() []*Channel
func (cm *ChannelManager) DiscoverPublicChannels(ctx context.Context) ([]*Channel, error)
```

**Tasks**:
- [ ] Implement channel creation
- [ ] Add channel join/leave
- [ ] Implement channel message encryption
- [ ] Add public channel discovery (DHT index)
- [ ] Implement private channel invites
- [ ] Write unit tests

#### 6.3 CLI Integration

**Files**: `pkg/cli/commands.go`

**New Commands**:
```
/channel create <name> [--public|--private]  - Create new channel
/channel list                                - List all channels
/channel discover                            - Discover public channels
/channel join <channel>                      - Join channel
/channel leave <channel>                     - Leave channel
/channel invite <channel> <pubkey>           - Invite to private channel
/chat <channel>                              - Enter channel chat mode
```

**Tasks**:
- [ ] Add `/channel create` command
- [ ] Add `/channel list` command
- [ ] Add `/channel discover` command
- [ ] Add `/channel join` command
- [ ] Add `/channel leave` command
- [ ] Add `/channel invite` command
- [ ] Update `/chat` to accept channel names

**Deliverables**:
- ✅ Channel protobuf schema
- ✅ ChannelManager with CRUD operations
- ✅ Public channel discovery
- ✅ Private channel invite system
- ✅ CLI commands for channel management

---

### Phase 7: Encryption Abstraction & Double Ratchet (Weeks 11-12)

**Goal**: Create pluggable encryption layer with forward secrecy support

#### 7.1 Encryption Scheme Interface

**Files**: `pkg/crypto/scheme.go` (new)

```go
type EncryptionScheme interface {
    ID() EncryptionSchemeID
    Name() string
    KeyAgreement(privKey, peerPubKey []byte) ([]byte, error)
    Encrypt(sharedSecret, plaintext, params []byte) (ciphertext, nonce, encParams []byte, err error)
    Decrypt(sharedSecret, ciphertext, nonce, params []byte) ([]byte, error)
    GenerateKeyPair() (pubKey, privKey []byte, err error)
    SupportsOffline() bool
    SupportsForwardSecrecy() bool
}

type EncryptionSchemeID string

const (
    SchemeXChaCha20  EncryptionSchemeID = "xchacha20-poly1305"
    SchemeDoubleRatchet EncryptionSchemeID = "double-ratchet"
)
```

**Tasks**:
- [ ] Define `EncryptionScheme` interface
- [ ] Create scheme ID type and constants
- [ ] Add capability detection methods
- [ ] Write interface compliance tests

#### 7.2 XChaCha20-Poly1305 Implementation (Refactor)

**Files**: `pkg/crypto/xchacha20.go` (new)

```go
type XChaCha20Scheme struct{}

func (s *XChaCha20Scheme) ID() EncryptionSchemeID { return SchemeXChaCha20 }
func (s *XChaCha20Scheme) Name() string { return "XChaCha20-Poly1305" }
func (s *XChaCha20Scheme) SupportsOffline() bool { return true }
func (s *XChaCha20Scheme) SupportsForwardSecrecy() bool { return false }

func (s *XChaCha20Scheme) KeyAgreement(privKey, peerPubKey []byte) ([]byte, error) {
    return ComputeSharedSecret(privKey, peerPubKey)  // Existing function
}

func (s *XChaCha20Scheme) Encrypt(sharedSecret, plaintext, _ []byte) ([]byte, []byte, []byte, error) {
    nonce, ciphertext, err := EncryptWithSharedSecret(sharedSecret, plaintext)
    return ciphertext, nonce, nil, err
}
```

**Tasks**:
- [ ] Refactor existing crypto functions into scheme interface
- [ ] Ensure backward compatibility with current messaging
- [ ] Add encryption params support
- [ ] Write unit tests for scheme wrapper

#### 7.3 Double Ratchet Implementation

**Files**: `pkg/crypto/doubleratchet.go` (new)

```go
type DoubleRatchetScheme struct {
    rootKey []byte
    sendChainKey []byte
    recvChainKey []byte
    dhPrivKey []byte
    dhPubKey []byte
    sendChainNum uint32
    recvChainNum uint32
    skipKeys map[string][]byte  // For out-of-order
    mu sync.RWMutex
}

func NewDoubleRatchetScheme(initialSharedSecret, myDHPriv, myDHPub, theirDHPub []byte) *DoubleRatchetScheme
func (s *DoubleRatchetScheme) RatchetSend(theirNewPubKey []byte) error
func (s *DoubleRatchetScheme) RatchetRecv(theirNewPubKey []byte) error
func (s *DoubleRatchetScheme) Encrypt(plaintext []byte) (ciphertext, ephemeralPub, encParams []byte, err error)
func (s *DoubleRatchetScheme) Decrypt(ciphertext, ephemeralPub, encParams []byte) ([]byte, error)
func (s *DoubleRatchetScheme) SupportsForwardSecrecy() bool { return true }
func (s *DoubleRatchetScheme) SupportsOffline() bool { return false }  // Requires ordering
```

**Key Derivation**:
```go
// Root ratchet: new root key and chain key
func rkdf(rootKey, dhOutput []byte) (newRootKey, newChainKey []byte) {
    okm := hkdf.Expand(sha256.New, rootKey, dhOutput)
    newRootKey = okm[:32]
    newChainKey = okm[32:64]
    return
}

// Chain ratchet: derive message key
func ckdf(chainKey []byte) (msgKey, nextChainKey []byte) {
    okm := hkdf.Expand(sha256.New, chainKey, nil)
    msgKey = okm[:32]
    nextChainKey = okm[32:64]
    return
}
```

**Tasks**:
- [ ] Implement Double Ratchet state machine
- [ ] Add DH ratchet (asymmetric key update)
- [ ] Add symmetric key ratchet (chain advancement)
- [ ] Implement skip key handling for out-of-order messages
- [ ] Add key derivation functions (HKDF-SHA256)
- [ ] Write extensive unit tests (ratchet vectors)
- [ ] Add integration test (Alice ↔ Bob exchange)

#### 7.4 Scheme Negotiation Protocol

**Files**: `pkg/messaging/negotiation.go` (new)

```protobuf
message EncryptionNegotiation {
  repeated EncryptionScheme supported_schemes = 1;
  EncryptionScheme selected_scheme = 2;
  bytes handshake_data = 3;
  bytes initial_key_material = 4;
}
```

**Negotiation Flow**:
```
Alice                              Bob
  |                                  |
  |--- EncryptionNegotiation ------->|  (lists: [XChaCha20, DoubleRatchet])
  |    supported: [1, 2]             |
  |                                  |  Selects highest mutual
  |<-- EncryptionNegotiation --------|  (selected: DoubleRatchet)
  |    selected: 2, handshake: DH_B  |
  |                                  |
  |--- DH_pubkey_A ----------------->|  Complete key agreement
  |    (in first message)            |
```

**Tasks**:
- [ ] Define negotiation message protobuf
- [ ] Implement scheme preference ordering
- [ ] Add negotiation handshake for first contact
- [ ] Store negotiated scheme per contact
- [ ] Handle negotiation failures (fallback to XChaCha20)

#### 7.5 Encryption Context Manager

**Files**: `pkg/crypto/context.go` (new)

```go
type EncryptionContext struct {
    scheme EncryptionScheme
    contactPubKey []byte
    x25519PrivKey []byte
    x25519PubKey []byte
    ratchetState *DoubleRatchetScheme  // If using Double Ratchet
    lastUsed time.Time
    messageCount uint64
}

type ContextManager struct {
    mu sync.RWMutex
    contexts map[string]*EncryptionContext  // Key: hex(contactPubKey)
    storage *storage.Storage
}

func (m *ContextManager) GetContext(contactPubKey []byte) (*EncryptionContext, error)
func (m *ContextManager) CreateContext(contactPubKey, x25519PubKey []byte) (*EncryptionContext, error)
func (m *ContextManager) RotateKeys(contactPubKey []byte) error
```

**Tasks**:
- [ ] Implement context per contact
- [ ] Add context persistence to BadgerDB
- [ ] Implement automatic key rotation (every N messages)
- [ ] Add context cleanup (unused contacts)
- [ ] Thread-safe context access

**Deliverables**:
- ✅ `EncryptionScheme` interface with capability detection
- ✅ XChaCha20-Poly1305 wrapper (refactored)
- ✅ Double Ratchet implementation with forward secrecy
- ✅ Scheme negotiation protocol
- ✅ Encryption context manager per contact

---

## Testing Strategy

### Unit Tests

**Coverage Target**: >80% for all new modules

```bash
# Run all unit tests
make test

# Run with coverage
make test-coverage

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

**Key Test Files**:
- `pkg/crypto/xchacha20_test.go` - XChaCha20 scheme wrapper
- `pkg/crypto/doubleratchet_test.go` - Double Ratchet vectors
- `pkg/addressbook/addressbook_test.go` - Contact record operations
- `pkg/messaging/group_test.go` - Group creation and messaging
- `pkg/messaging/offline_test.go` - Offline message storage

### Integration Tests

**Two-Node Communication**:
```go
func TestTwoNodeMessaging(t *testing.T) {
    // Create two nodes
    node1 := createTestNode()
    node2 := createTestNode()
    
    // Exchange contact info
    node1.AddContact(node2.PublicKey())
    node2.AddContact(node1.PublicKey())
    
    // Send message
    node1.SendMessage(node2.PublicKey(), "Hello")
    
    // Verify receipt
    select {
    case msg := <-node2.Messages():
        assert.Equal(t, "Hello", msg.Text)
    case <-time.After(5 * time.Second):
        t.Fatal("Message not received")
    }
}
```

**Group Messaging**:
```go
func TestGroupMessaging(t *testing.T) {
    // Create group with 3 members
    group := groupManager.CreateGroup("Test Group", []pubKey1, pubKey2, pubKey3)
    
    // Member 1 sends message
    groupManager.SendMessage(group.ID, "Hello everyone")
    
    // All members should receive
    for _, member := range members {
        msg := <-member.Messages()
        assert.Equal(t, "Hello everyone", msg.Text)
    }
}
```

### End-to-End Tests

**CLI E2E**:
```bash
# Start two CLI instances
./bin/messenger --config config1.json &
./bin/messenger --config config2.json &

# Automate interaction via expect script
./scripts/e2e_test.sh
```

---

## Migration Guide

### From v1.0.0 to v2.0.0

#### Backward Compatibility

The protocol maintains backward compatibility during migration:

```go
// Envelope processing with version detection
func ProcessEnvelope(signedEnv *SignedEnvelope) (*Message, error) {
    switch signedEnv.ProtocolVersion {
    case 0, 1:
        // Legacy processing
        return processLegacyEnvelope(signedEnv)
    case 2:
        // New processing with scheme selection
        scheme, err := registry.GetScheme(signedEnv.EncryptionScheme)
        if err != nil {
            return nil, err
        }
        return processV2Envelope(signedEnv, scheme)
    default:
        return nil, fmt.Errorf("unsupported protocol version: %d", signedEnv.ProtocolVersion)
    }
}
```

#### Data Migration

```go
// Migrate existing contacts
func MigrateContacts(oldStorage *storage.Storage, newStorage *storage.Storage) error {
    contacts, err := oldStorage.ListContacts()
    if err != nil {
        return err
    }
    
    for _, contact := range contacts {
        // Create contact record
        record := &ContactRecord{
            Ed25519Pubkey: contact.PublicKey,
            DisplayName: contact.DisplayName,
            SequenceNum: 1,
            Timestamp: uint64(time.Now().Unix()),
        }
        
        // Sign and publish to DHT
        err := addressBook.Publish(ctx, record)
        if err != nil {
            log.Printf("Failed to migrate contact %x: %v", contact.PublicKey, err)
        }
    }
    
    return nil
}
```

#### Feature Flags

```go
// Enable v2 features gradually
type Config struct {
    EnableV2Protocol bool `yaml:"enable_v2_protocol"`
    EnableAddressBook bool `yaml:"enable_address_book"`
    EnableGroupMessaging bool `yaml:"enable_group_messaging"`
    EnableDoubleRatchet bool `yaml:"enable_double_ratchet"`
    EnableOfflineMessages bool `yaml:"enable_offline_messages"`
}
```

---

## Performance Considerations

### DHT Query Optimization

```go
// Parallel DHT queries
func ParallelDHTLookup(ctx context.Context, dht *dht.IpfsDHT, key string) ([]byte, error) {
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    
    // Use DHT's built-in parallelism
    record, err := dht.GetValue(ctx, key, dht.Quorum(2))
    return record, err
}
```

### Cache Strategy

```go
// LRU cache with TTL
type CachedRecord struct {
    record *ContactRecord
    expiresAt time.Time
}

func NewAddressBookCache(size int) *Cache {
    return &Cache{
        lru: lru.New(size),
        defaultTTL: time.Hour,
    }
}

func (c *Cache) Get(key string) (*ContactRecord, bool) {
    if val, ok := c.lru.Get(key); ok {
        cached := val.(*CachedRecord)
        if time.Now().Before(cached.expiresAt) {
            return cached.record, true
        }
        c.lru.Remove(key)  // Expired
    }
    return nil, false
}
```

### Message Batching

```go
// Batch offline message retrieval
func (os *OfflineMessageStore) FetchMessagesBatch(ctx context.Context, bundleCID string, batchSize int) ([]*SignedExtendedEnvelope, error) {
    bundle, err := os.fetchBundle(bundleCID)
    if err != nil {
        return nil, err
    }
    
    var messages []*SignedExtendedEnvelope
    for i := 0; i < len(bundle.MessageCids) && i < batchSize; i++ {
        cid := bundle.MessageCids[i]
        env, err := os.ipfs.Get(ctx, cid)
        if err != nil {
            continue  // Skip failed fetches
        }
        messages = append(messages, env)
    }
    
    return messages, nil
}
```

---

## Security Considerations

### Signature Verification

All messages and records must be signed:

```go
func VerifyEnvelope(env *SignedExtendedEnvelope) error {
    // Verify envelope signature
    if !ed25519.Verify(env.SenderPubkey, env.EnvelopeBytes, env.Signature) {
        return ErrInvalidSignature
    }
    
    // Verify sender is who they claim to be
    senderID, err := peer.IDFromPublicKey(env.SenderPubkey)
    if err != nil {
        return err
    }
    
    // Optional: Check against known contacts
    // ...
    
    return nil
}
```

### Replay Attack Prevention

```go
type ReplayCache struct {
    seen map[string]time.Time  // Key: message_hash
    mu sync.RWMutex
    ttl time.Duration
}

func (rc *ReplayCache) IsReplay(messageHash string) bool {
    rc.mu.RLock()
    defer rc.mu.RUnlock()
    
    if _, ok := rc.seen[messageHash]; ok {
        return true
    }
    return false
}

func (rc *ReplayCache) MarkSeen(messageHash string) {
    rc.mu.Lock()
    defer rc.mu.Unlock()
    
    rc.seen[messageHash] = time.Now()
    
    // Cleanup old entries
    go rc.cleanup()
}
```

### Rate Limiting

```go
type RateLimiter struct {
    mu sync.RWMutex
    requests map[string][]time.Time  // Key: peer_id
    limit int
    window time.Duration
}

func (rl *RateLimiter) Allow(peerID string) bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    now := time.Now()
    cutoff := now.Add(-rl.window)
    
    // Filter old requests
    var recent []time.Time
    for _, t := range rl.requests[peerID] {
        if t.After(cutoff) {
            recent = append(recent, t)
        }
    }
    
    if len(recent) >= rl.limit {
        return false
    }
    
    rl.requests[peerID] = append(recent, now)
    return true
}
```

---

## Appendix: Complete Timeline

| Phase | Weeks | Deliverables |
|-------|-------|--------------|
| **Phase 1** | 1-2 | Protocol versioning, X25519 storage, protocol registry (XChaCha20 only) |
| **Phase 2** | 3-4 | Address book with DHT, update propagation, CLI commands |
| **Phase 3** | 5-6 | Group messaging, key distribution, rotation, CLI integration |
| **Phase 4** | 7-8 | Service publishing, peer discovery, libp2p identify |
| **Phase 5** | 9 | Offline message storage and retrieval |
| **Phase 6** | 10 | Public/private channels, discovery system |
| **Phase 7** | 11-12 | Encryption abstraction, Double Ratchet, scheme negotiation |

**Total Duration**: 12 weeks (3 months)

---

## Success Metrics

- [ ] **Protocol Versioning**: 100% of messages include version field
- [ ] **Address Book**: <5 second lookup time for contact records
- [ ] **Group Messaging**: Support for groups up to 100 members
- [ ] **Service Discovery**: Peer info available within 10 seconds of connection
- [ ] **Offline Messages**: 99% delivery rate within 24 hours
- [ ] **Channel Support**: Public channel discovery and private invites working
- [ ] **Encryption Flexibility**: Support for 2+ encryption schemes (Phase 7)
- [ ] **Test Coverage**: >80% for all new modules
- [ ] **Backward Compatibility**: Zero breaking changes for v1.0.0 clients

---

*Last updated: February 23, 2026*
