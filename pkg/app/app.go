// Package app provides the core application layer for Babylon Tower.
// It defines the main Application interface and facade for the Babylon Tower Core library.
package app

import (
	"context"
	"time"

	"babylontower/pkg/groups"
	"babylontower/pkg/messaging"
	"babylontower/pkg/storage"
)

// Application is the main interface for Babylon Tower Core.
// It provides lifecycle management and access to core services.
type Application interface {
	// Lifecycle
	Start() error
	Stop() error

	// Identity
	GetIdentity() *IdentityInfo

	// High-level managers for UI
	Contacts() ContactManager
	Chat() ChatManager

	// Core services (return interfaces, not concrete types)
	Messenger() Messenger
	Groups() GroupManager
	UIGroups() UIGroupManager
	Network() NetworkNode
	Storage() storage.Storage

	// Events
	MessageEvents() <-chan *MessageEvent
}

// IdentityInfo contains public identity information.
type IdentityInfo struct {
	// PublicKey is the Ed25519 public key in hex format
	PublicKey string
	// PublicKeyBase58 is the Ed25519 public key in base58 format
	PublicKeyBase58 string
	// X25519KeyBase58 is the X25519 public key in base58 format
	X25519KeyBase58 string
	// PeerID is the libp2p peer ID
	PeerID string
	// Multiaddrs is the list of listen addresses
	Multiaddrs []string
	// Mnemonic is the BIP39 mnemonic (only available on first generation)
	Mnemonic string
	// Fingerprint is the identity fingerprint for out-of-band verification
	Fingerprint string
	// ContactLink is the btower:// contact exchange link
	ContactLink string
	// DisplayName is the user's configured display name
	DisplayName string
}

// MessageEvent represents an incoming message event.
// Re-exported from messaging package for convenience.
type MessageEvent = messaging.MessageEvent

// Messenger is the interface for the messaging service.
type Messenger interface {
	// Lifecycle
	Start() error
	Stop() error

	// Messaging operations
	SendMessageToContact(text string, recipientEd25519PubKey, recipientX25519PubKey []byte) (*messaging.SendResult, error)
	GetDecryptedMessagesWithMeta(contactPubKey []byte, limit, offset int) ([]*messaging.MessageWithMeta, error)
	Messages() <-chan *MessageEvent
	GetContactStatus(contactPubKey []byte) (*messaging.ContactStatus, error)
	GetAllContactStatuses() ([]*messaging.ContactStatus, error)
	IsStarted() bool

	// Mailbox
	GetMailboxManager() MailboxManager
	RetrieveOfflineMessages() error

	// Reputation tracker access
	ReputationTracker() ReputationTracker
}

// GroupManager is the interface for group management.
type GroupManager interface {
	// Group operations
	CreateGroup(name, description string, groupType groups.GroupType) (*groups.GroupState, error)
	AddMember(groupID []byte, memberPubkey, memberX25519Pubkey []byte, displayName string, role groups.GroupRole) (*groups.GroupState, error)
	RemoveMember(groupID []byte, memberPubkey []byte) (*groups.GroupState, error)
	GetGroup(groupID []byte) (*groups.GroupState, error)
	ListGroups() []*groups.GroupState
}

// NetworkNode is the interface for network operations.
type NetworkNode interface {
	// Lifecycle
	Start() error
	Stop() error

	// Network operations
	ConnectToPeer(maddr string) error
	FindPeer(peerID string) (*PeerAddrInfo, error)
	Subscribe(topic string) (*NetworkSubscription, error)
	AdvertiseSelf(ctx context.Context) error
	WaitForDHT(timeout time.Duration) error
	WaitForBabylonBootstrap(timeout time.Duration) error

	// Identity
	PeerID() string
	Multiaddrs() []string
	IsStarted() bool
	Context() context.Context

	// Diagnostics and metrics (used by CLI)
	GetNetworkInfo() *NetworkInfo
	GetDHTInfo() *DHTInfo
	ClearAllBackoffs()
	GetMDnsStats() MDnsStats
	GetMetricsFull() *MetricsFull

	// Babylon DHT and bootstrap status (used by CLI)
	GetBabylonDHTInfo() *BabylonDHTInfo
	GetPeerCountsBySource() *PeerCountsBySource
	GetBootstrapStatus() *BootstrapStatus
	TriggerRendezvousDiscovery() int
}

// MailboxManager interface for mailbox operations
type MailboxManager interface {
	IsMailbox() bool
	GetStats() (*MailboxStats, error)
	GetAnnouncement(pubKey []byte) (*MailboxAnnouncement, bool)
	RetrieveMessages(ctx context.Context) (*MailboxRetrievalResult, error)
}

// MailboxRetrievalResult contains messages retrieved from mailbox
type MailboxRetrievalResult struct {
	Envelopes  [][]byte
	MessageIDs [][]byte
	Count      int
}

// MailboxStats contains mailbox statistics
type MailboxStats struct {
	StoredCount     int
	UsedBytes       int64
	CapacityBytes   int64
	OldestTimestamp int64
	NewestTimestamp int64
}

// MailboxAnnouncement contains mailbox announcement information
type MailboxAnnouncement struct {
	MailboxPeerId   string
	MaxMessages     int
	MaxMessageSize  int
	TtlSeconds      int
	ReputationScore int64
}

// ReputationTracker interface for reputation operations
type ReputationTracker interface {
	GetAllRecords() map[string]*ReputationRecord
	GetPeersByTier(tier string) []string
	GetTopPeers(n int) []string
	GetRecord(peerID string) *ReputationRecord
}

// ReputationRecord contains reputation information for a peer
type ReputationRecord struct {
	CompositeScore float64
	Tier           string
	Metrics        *ReputationMetrics
	Attestations   []Attestation
}

// ReputationMetrics contains reputation metrics
type ReputationMetrics struct {
	RelayReliability      float64
	RelaySuccessCount     int
	RelayTotalCount       int
	UptimeConsistency     float64
	HoursOnline7d         int
	MailboxReliability    float64
	MailboxRetrievedCount int
	MailboxDepositedCount int
	DHTResponsiveness     float64
	AvgResponseMS         float64
	ContentServing        float64
	BlocksServedCount     int
	BlocksRequestedCount  int
}

// Attestation contains a reputation attestation
type Attestation struct {
	FromPeerID      string
	ToPeerID        string
	Score           int64
	Timestamp       int64
	AttestationType string
}

// NetworkInfo contains network status information.
type NetworkInfo struct {
	PeerID             string
	Multiaddrs         []string
	ListenAddrs        []string
	ConnectedPeers     []PeerInfo
	ConnectedPeerCount int
	IsStarted          bool
}

// PeerInfo contains information about a peer.
type PeerInfo struct {
	ID        string
	Addresses []string
	Protocols []string
	Connected bool
}

// DHTInfo contains DHT routing table information.
type DHTInfo struct {
	IsStarted              bool
	Mode                   string
	RoutingTableSize       int
	RoutingTablePeers      []string
	ConnectedPeerCount     int
	HasBootstrapConnection bool
}

// MDnsStats contains mDNS discovery statistics.
type MDnsStats struct {
	TotalDiscoveries int32
	LastPeerFound    time.Time
}

// MetricsFull contains comprehensive network metrics.
type MetricsFull struct {
	PeerID                  string
	IsStarted               bool
	StartTime               time.Time
	UptimeSeconds           int64
	CurrentConnections      int
	TotalConnections        int
	TotalDisconnections     int
	ConnectionSuccessRate   float64
	AverageLatencyMs        int64
	DHTDiscoveries          int64
	MDNSDiscoveries         int64
	PeerExchangeDiscoveries int64
	DiscoveryBySource       map[string]int64
	SuccessfulMessages      int64
	FailedMessages          int64
	MessageSuccessRate      float64
	BootstrapAttempts       int
	BootstrapSuccesses      int
	LastBootstrapTime       time.Time
}

// BabylonDHTInfo contains information about Babylon DHT peers
type BabylonDHTInfo struct {
	StoredBabylonPeers    int
	ConnectedBabylonPeers int
	BabylonPeerIDs        []string
	RendezvousActive bool
}

// PeerCountsBySource contains peer counts grouped by discovery source
type PeerCountsBySource struct {
	Babylon        int // Peers from Babylon bootstrap
	IPFSBootstrap  int // Config bootstrap peers
	IPFSDiscovery  int // Discovered via public IPFS DHT
	MDNS           int // Local mDNS discovery
	ConnectedTotal int // Currently connected peers
}

// BootstrapStatus contains the status of the bootstrap process
type BootstrapStatus struct {
	// IPFS DHT bootstrap (transport layer)
	IPFSBootstrapComplete bool
	IPFSRoutingTableSize  int

	// Babylon DHT bootstrap (protocol layer)
	BabylonBootstrapComplete bool
	BabylonPeersStored       int
	BabylonPeersConnected    int
	BabylonBootstrapDeferred bool

	// Rendezvous discovery
	RendezvousActive bool
}

// PeerAddrInfo contains peer address information.
// Simplified version to avoid direct libp2p dependency.
type PeerAddrInfo struct {
	ID        string
	Addrs     []string
	Protocols []string
}

// NetworkSubscription is a simplified subscription interface.
type NetworkSubscription struct {
	MessagesFn func() <-chan []byte
	ErrorsFn   <-chan error
	CloseFn    func() error
}

// Messages returns the messages channel.
func (s *NetworkSubscription) Messages() <-chan []byte {
	if s.MessagesFn != nil {
		return s.MessagesFn()
	}
	return nil
}

// Errors returns the errors channel.
func (s *NetworkSubscription) Errors() <-chan error {
	return s.ErrorsFn
}

// Close closes the subscription.
func (s *NetworkSubscription) Close() error {
	if s.CloseFn != nil {
		return s.CloseFn()
	}
	return nil
}
