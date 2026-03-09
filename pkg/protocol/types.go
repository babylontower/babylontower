// Package protocol implements the Babylon Tower Protocol v1 specification.
// It provides high-level orchestration for identity management, session establishment,
// peer discovery, and message routing over the decentralized P2P network.
//
// Status: Phase 18 (Integration & Hardening). The active code in this package is
// EnvelopeBuilder (envelope.go) and topic derivation (envelope.go). The interface
// definitions (SessionStore, IdentityStore, PeerDiscovery, etc.) and
// ProtocolConfig/SessionManager types are specification scaffolding that will be
// wired into the runtime path during Phase 18 integration.
//
// Protocol Architecture:
//   - Transport Layer: IPFS DHT for peer routing and content addressing
//   - Protocol Layer: Babylon DHT for identity documents and protocol-specific data
//   - Session Layer: X3DH key agreement + Double Ratchet for E2E encryption
//   - Application Layer: Message types, groups, channels, and RTC signaling
package protocol

import (
	"context"
	"crypto/ed25519"
	"errors"
	"time"

	"babylontower/pkg/ratchet"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

// ============================================================================
// Protocol Constants
// ============================================================================

const (
	// ProtocolVersion is the current protocol version
	ProtocolVersion = 1

	// Protocol ID prefix for Babylon Tower
	ProtocolID = "/babylontower/1.0.0"
)

// DHT Key Namespaces per protocol-v2.md specification
const (
	// DHTNamespaceIdentity is the prefix for identity document keys
	DHTNamespaceIdentity = "/bt/id/"
	// DHTNamespaceUsername is the prefix for username registry keys
	DHTNamespaceUsername = "/bt/username/"
	// DHTNamespacePrekeys is the prefix for prekey bundle keys
	DHTNamespacePrekeys = "/bt/prekeys/"
	// DHTNamespacePublicGroup is the prefix for public group state keys
	DHTNamespacePublicGroup = "/bt/pubgroup/"
	// DHTNamespaceChannelHead is the prefix for public channel head keys
	DHTNamespaceChannelHead = "/bt/chanhead/"
)

// Timeouts and Limits
const (
	// DefaultBootstrapTimeout is the default timeout for bootstrap operations
	DefaultBootstrapTimeout = 60 * time.Second
	// DefaultDHTTimeout is the default timeout for DHT operations
	DefaultDHTTimeout = 30 * time.Second
	// DefaultPeerConnectTimeout is the default timeout for peer connections
	DefaultPeerConnectTimeout = 30 * time.Second
	// DefaultSessionTimeout is the default timeout for session operations
	DefaultSessionTimeout = 15 * time.Second

	// MaxIdentityDocumentSize is the maximum size of an identity document in bytes
	MaxIdentityDocumentSize = 64 * 1024 // 64 KB
	// MaxPrekeyBatchSize is the maximum number of one-time prekeys per batch
	MaxPrekeyBatchSize = 100
	// MinPrekeyThreshold is the threshold for prekey replenishment
	MinPrekeyThreshold = 20
	// DefaultPrekeyTargetCount is the target number of one-time prekeys to maintain
	DefaultPrekeyTargetCount = 100

	// SignedPrekeyRotationInterval is how often signed prekeys are rotated
	SignedPrekeyRotationInterval = 7 * 24 * time.Hour
	// SignedPrekeyOverlapPeriod is the overlap period for signed prekey rotation
	SignedPrekeyOverlapPeriod = 24 * time.Hour
	// SignedPrekeyMaxAge is the maximum age before rejection
	SignedPrekeyMaxAge = 14 * 24 * time.Hour

	// MaxSkippedKeys is the maximum number of skipped message keys to cache
	MaxSkippedKeys = 256
	// MaxSessionAge is the maximum age for idle sessions before cleanup
	MaxSessionAge = 30 * 24 * time.Hour
	// SessionCleanupInterval is how often session cleanup runs
	SessionCleanupInterval = 1 * time.Hour

	// IdentityRepublishInterval is how often identity documents are republished
	IdentityRepublishInterval = 4 * time.Hour
	// IdentityRecordTTL is the TTL for identity DHT records
	IdentityRecordTTL = 24 * time.Hour

	// MaxStoredSessions is the maximum number of sessions to store per device
	MaxStoredSessions = 1000
	// MaxContacts is the maximum number of contacts to track
	MaxContacts = 5000
)

// Cipher Suite IDs per protocol-v2.md specification
const (
	// CipherSuiteXChaCha20Poly1305 is the mandatory cipher suite
	CipherSuiteXChaCha20Poly1305 uint32 = 0x0001
	// CipherSuiteAES256GCM is the optional AES-GCM cipher suite
	CipherSuiteAES256GCM uint32 = 0x0002
)

// ============================================================================
// Protocol Errors
// ============================================================================

var (
	// ErrProtocolNotInitialized is returned when protocol operations are attempted before initialization
	ErrProtocolNotInitialized = errors.New("protocol not initialized")
	// ErrProtocolAlreadyInitialized is returned when trying to initialize an already initialized protocol
	ErrProtocolAlreadyInitialized = errors.New("protocol already initialized")
	// ErrIdentityNotFound is returned when an identity document is not found in the DHT
	ErrIdentityNotFound = errors.New("identity not found")
	// ErrIdentityInvalid is returned when an identity document fails validation
	ErrIdentityInvalid = errors.New("invalid identity document")
	// ErrIdentitySequenceTooLow is returned when receiving an outdated identity document
	ErrIdentitySequenceTooLow = errors.New("identity sequence too low")
	// ErrSignatureInvalid is returned when cryptographic signature verification fails
	ErrSignatureInvalid = errors.New("invalid signature")
	// ErrSessionNotFound is returned when a session is not found in the session store
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionExists is returned when trying to create a duplicate session
	ErrSessionExists = errors.New("session already exists")
	// ErrSessionExpired is returned when a session has exceeded its maximum age
	ErrSessionExpired = errors.New("session expired")
	// ErrPrekeyExhausted is returned when no one-time prekeys are available
	ErrPrekeyExhausted = errors.New("no one-time prekeys available")
	// ErrPrekeyInvalid is returned when a prekey fails validation
	ErrPrekeyInvalid = errors.New("invalid prekey")
	// ErrBootstrapIncomplete is returned when bootstrap has not completed
	ErrBootstrapIncomplete = errors.New("bootstrap incomplete")
	// ErrPeerNotFound is returned when a peer cannot be found in the DHT
	ErrPeerNotFound = errors.New("peer not found")
	// ErrPeerConnectFailed is returned when peer connection fails
	ErrPeerConnectFailed = errors.New("failed to connect to peer")
	// ErrDHTOperationFailed is returned when a DHT operation fails
	ErrDHTOperationFailed = errors.New("DHT operation failed")
	// ErrContextCancelled is returned when an operation is cancelled by context
	ErrContextCancelled = errors.New("operation cancelled")
	// ErrDeviceNotRegistered is returned when a device is not registered in the identity document
	ErrDeviceNotRegistered = errors.New("device not registered")
	// ErrDeviceRevoked is returned when a device has been revoked
	ErrDeviceRevoked = errors.New("device revoked")
)

// ============================================================================
// Protocol State Structures
// ============================================================================

// ProtocolState represents the overall state of the protocol instance
type ProtocolState struct {
	// Initialized indicates whether the protocol has been initialized
	Initialized bool
	// IdentityPubKey is the local identity's Ed25519 public key
	IdentityPubKey ed25519.PublicKey
	// DeviceID is the local device's unique identifier
	DeviceID []byte
	// StartedAt is when the protocol was started
	StartedAt time.Time
}

// BootstrapState tracks the state of the bootstrap process
type BootstrapState struct {
	// IPFSBootstrapComplete indicates if IPFS DHT bootstrap is complete
	IPFSBootstrapComplete bool
	// BabylonBootstrapComplete indicates if Babylon DHT bootstrap is complete
	BabylonBootstrapComplete bool
	// BabylonBootstrapDeferred indicates if Babylon bootstrap is deferred
	BabylonBootstrapDeferred bool
	// BootstrapHelperActive indicates if this node is acting as a bootstrap helper
	BootstrapHelperActive bool
	// IPFSRoutingTableSize is the size of the IPFS DHT routing table
	IPFSRoutingTableSize int
	// BabylonPeersStored is the number of Babylon peers stored
	BabylonPeersStored int
	// BabylonPeersConnected is the number of Babylon peers connected
	BabylonPeersConnected int
	// LastBootstrapAttempt is when the last bootstrap attempt occurred
	LastBootstrapAttempt time.Time
	// BootstrapStartTime is when bootstrap was initiated
	BootstrapStartTime time.Time
}

// SessionInfo contains metadata about a protocol session
type SessionInfo struct {
	// SessionID is the unique session identifier
	SessionID string
	// RemoteIdentityPub is the remote party's identity public key
	RemoteIdentityPub ed25519.PublicKey
	// RemoteDeviceID is the remote device's identifier
	RemoteDeviceID []byte
	// LocalDeviceID is the local device's identifier
	LocalDeviceID []byte
	// CreatedAt is when the session was created
	CreatedAt time.Time
	// LastUsedAt is when the session was last used
	LastUsedAt time.Time
	// IsInitiator indicates if we initiated the X3DH
	IsInitiator bool
	// CipherSuite is the negotiated cipher suite
	CipherSuite uint32
}

// ============================================================================
// Session Management Types
// ============================================================================

// SessionStore is the interface for session storage and retrieval
type SessionStore interface {
	// Get retrieves a session by session ID
	Get(sessionID string) (*ratchet.DoubleRatchetState, error)
	// GetByRemoteIdentity retrieves a session by remote identity and device
	GetByRemoteIdentity(remoteIdentity ed25519.PublicKey, remoteDeviceID []byte) (*ratchet.DoubleRatchetState, error)
	// Put stores or updates a session
	Put(session *ratchet.DoubleRatchetState) error
	// Delete removes a session
	Delete(sessionID string) error
	// List returns all session IDs
	List() ([]string, error)
	// ListByRemoteIdentity returns all sessions for a remote identity
	ListByRemoteIdentity(remoteIdentity ed25519.PublicKey) ([]string, error)
	// Count returns the number of stored sessions
	Count() int
	// Cleanup removes expired sessions
	Cleanup(maxAge time.Duration) (int, error)
}

// SessionManager handles session lifecycle management
type SessionManager interface {
	// CreateInitiator creates a new session as X3DH initiator
	CreateInitiator(remoteIdentity ed25519.PublicKey, remoteDeviceID []byte, x3dhResult *ratchet.X3DHResult) (*ratchet.DoubleRatchetState, error)
	// CreateResponder creates a new session as X3DH responder
	CreateResponder(sessionID string, remoteIdentity ed25519.PublicKey, remoteDeviceID []byte, x3dhResult *ratchet.X3DHResult) (*ratchet.DoubleRatchetState, error)
	// Get retrieves an existing session
	Get(sessionID string) (*ratchet.DoubleRatchetState, error)
	// GetOrCreateInitiator gets or creates a session as initiator
	GetOrCreateInitiator(remoteIdentity ed25519.PublicKey, remoteDeviceID []byte, x3dhResult *ratchet.X3DHResult) (*ratchet.DoubleRatchetState, error)
	// Update updates an existing session
	Update(session *ratchet.DoubleRatchetState) error
	// Delete deletes a session
	Delete(sessionID string) error
	// List lists all sessions
	List() ([]*SessionInfo, error)
	// Cleanup removes expired sessions
	Cleanup() (int, error)
}

// ============================================================================
// Identity Types
// ============================================================================

// IdentityDocument represents a signed identity document per protocol-v2.md
type IdentityDocument struct {
	// Core identity
	IdentitySignPub ed25519.PublicKey // IK_sign.pub (32 bytes)
	IdentityDHPub   []byte            // IK_dh.pub (32 bytes)

	// Versioning (forms a hash chain)
	Sequence    uint64
	PreviousHash []byte // SHA256(previous serialized doc), empty for seq=1
	CreatedAt   uint64 // Unix timestamp
	UpdatedAt   uint64 // This version's timestamp

	// Devices and prekeys
	Devices        []DeviceCertificate
	SignedPrekeys  []SignedPrekey
	OneTimePrekeys []OneTimePrekey

	// Protocol capabilities
	SupportedVersions   []uint32
	SupportedCipherSuites []string
	PreferredVersion    uint32

	// Optional profile
	DisplayName string
	AvatarCID   string // IPFS CID of avatar image

	// Revocations
	Revocations []RevocationCertificate

	// Feature flags
	Features FeatureFlags

	// Signature (covers all fields above)
	Signature []byte // Ed25519 signature
}

// DeviceCertificate represents a signed device certificate
type DeviceCertificate struct {
	DeviceID    []byte // SHA256(DK_sign.pub)[:16] - 16 bytes
	DeviceSignPub ed25519.PublicKey // DK_sign.pub (32 bytes)
	DeviceDHPub []byte // DK_dh.pub (32 bytes)
	DeviceName  string // Human-readable name
	CreatedAt   uint64 // Unix timestamp
	ExpiresAt   uint64 // Unix timestamp, 0 = no expiry
	IdentityPub ed25519.PublicKey // IK_sign.pub (for self-contained verification)
	Signature   []byte // Ed25519 signature
}

// SignedPrekey represents a signed prekey
type SignedPrekey struct {
	DeviceID  []byte // Which device owns this prekey
	PrekeyPub []byte // X25519 public key (32 bytes)
	PrekeyID  uint64 // Unique monotonic ID
	CreatedAt uint64 // Unix timestamp
	ExpiresAt uint64 // Unix timestamp
	Signature []byte // Ed25519 signature by IK_sign
}

// OneTimePrekey represents a one-time prekey
type OneTimePrekey struct {
	DeviceID  []byte // Which device owns this prekey
	PrekeyPub []byte // X25519 public key (32 bytes)
	PrekeyID  uint64 // Unique monotonic ID
}

// RevocationCertificate represents a key revocation
type RevocationCertificate struct {
	RevokedKey    []byte // Public key being revoked
	RevocationType string // "device" or "prekey"
	Reason        string // "compromised", "replaced", "expired"
	RevokedAt     uint64 // Unix timestamp
	Signature     []byte // Ed25519 signature
}

// FeatureFlags represents protocol feature support
type FeatureFlags struct {
	SupportsReadReceipts     bool
	SupportsTypingIndicators bool
	SupportsReactions        bool
	SupportsEdits            bool
	SupportsMedia            bool
	SupportsVoiceCalls       bool
	SupportsVideoCalls       bool
	SupportsGroups           bool
	SupportsChannels         bool
	SupportsOfflineMessages  bool
	CustomFeatures           []string
}

// PrekeyBundle is a convenience structure for X3DH
type PrekeyBundle struct {
	IdentityDHPub     []byte // IK_dh.pub
	IdentitySignPub   ed25519.PublicKey // IK_sign.pub
	SignedPrekeyPub   []byte // SPK.pub
	SignedPrekeySig   []byte // SPK signature
	SignedPrekeyID    uint64 // SPK ID
	OneTimePrekeyPub  []byte // OPK.pub (may be nil)
	OneTimePrekeyID   uint64 // OPK ID (0 if none)
}

// IdentityStore is the interface for identity document storage
type IdentityStore interface {
	// GetLocal returns the local identity document
	GetLocal() (*IdentityDocument, error)
	// GetRemote retrieves a remote identity document from DHT
	GetRemote(ctx context.Context, identityPub ed25519.PublicKey) (*IdentityDocument, error)
	// PutLocal stores the local identity document
	PutLocal(doc *IdentityDocument) error
	// Publish publishes an identity document to the DHT
	Publish(ctx context.Context, doc *IdentityDocument) error
	// Validate validates an identity document
	Validate(doc *IdentityDocument) error
	// GetPrekeyBundle retrieves a prekey bundle for X3DH
	GetPrekeyBundle(ctx context.Context, identityPub ed25519.PublicKey) (*PrekeyBundle, error)
}

// ============================================================================
// Discovery Types
// ============================================================================

// PeerDiscovery is the interface for peer discovery and routing
type PeerDiscovery interface {
	// DiscoverContact discovers a contact by identity public key
	DiscoverContact(ctx context.Context, identityPub ed25519.PublicKey) (*peer.AddrInfo, error)
	// ResolvePeerAddress resolves a peer's multiaddr from the DHT
	ResolvePeerAddress(ctx context.Context, peerID peer.ID) (*peer.AddrInfo, error)
	// FindPeers finds peers providing a specific service
	FindPeers(ctx context.Context, service string, limit int) (<-chan peer.AddrInfo, error)
	// AdvertiseService advertises that this node provides a service
	AdvertiseService(ctx context.Context, service string) error
	// CancelAdvertisement cancels a service advertisement
	CancelAdvertisement(ctx context.Context, service string) error
	// GetRoutingTableSize returns the DHT routing table size
	GetRoutingTableSize() int
	// GetConnectedPeers returns all connected peers
	GetConnectedPeers() []peer.ID
}

// ============================================================================
// Network Interfaces
// ============================================================================

// NetworkNode represents the underlying network interface
// This is typically implemented by the ipfsnode.Node
type NetworkNode interface {
	// Lifecycle
	Start() error
	Stop() error
	IsStarted() bool
	Context() context.Context

	// Network operations
	ConnectToPeer(maddr string) error
	FindPeer(peerID string) (*peer.AddrInfo, error)
	Host() host.Host
	DHT() *dht.IpfsDHT

	// Identity
	PeerID() string
	Multiaddrs() []string

	// Bootstrap state queries
	IsIPFSBootstrapComplete() bool
	IsBabylonBootstrapComplete() bool
	IsBabylonBootstrapDeferred() bool
	IsRendezvousActive() bool
}

// ProtocolDependencies contains all dependencies needed by the Protocol
type ProtocolDependencies struct {
	// NetworkNode is the underlying network interface
	NetworkNode NetworkNode
	// SessionStore is the session storage interface
	SessionStore SessionStore
	// IdentityStore is the identity storage interface
	IdentityStore IdentityStore
	// Discovery is the peer discovery interface
	Discovery PeerDiscovery
	// RoutingDiscovery is the libp2p routing discovery interface
	RoutingDiscovery *routing.RoutingDiscovery
}

// ============================================================================
// Utility Types
// ============================================================================

// KeyPair represents an Ed25519 key pair
type KeyPair struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// X25519KeyPair represents an X25519 key pair
type X25519KeyPair struct {
	PublicKey  *[32]byte
	PrivateKey *[32]byte
}

// ProtocolMetrics contains protocol-level metrics
type ProtocolMetrics struct {
	// SessionsActive is the number of active sessions
	SessionsActive int
	// SessionsTotal is the total number of sessions created
	SessionsTotal int
	// IdentitiesCached is the number of cached identity documents
	IdentitiesCached int
	// PrekeysAvailable is the number of available one-time prekeys
	PrekeysAvailable int
	// MessagesSent is the total number of messages sent
	MessagesSent int64
	// MessagesReceived is the total number of messages received
	MessagesReceived int64
	// BootstrapState is the current bootstrap state
	BootstrapState *BootstrapState
}
