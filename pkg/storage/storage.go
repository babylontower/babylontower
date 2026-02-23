package storage

import (
	"time"

	pb "babylontower/pkg/proto"
	"github.com/mr-tron/base58"
)

// PeerSource indicates where a peer was discovered
type PeerSource string

const (
	SourceBootstrap    PeerSource = "bootstrap"
	SourceDHT          PeerSource = "dht"
	SourceMDNS         PeerSource = "mdns"
	SourcePeerExchange PeerSource = "peer_exchange"
)

// PeerRecord represents a discovered peer for persistence
type PeerRecord struct {
	PeerID        string     `json:"peer_id"`
	Multiaddrs    []string   `json:"multiaddrs"`
	FirstSeen     time.Time  `json:"first_seen"`
	LastSeen      time.Time  `json:"last_seen"`
	LastConnected time.Time  `json:"last_connected"`
	ConnectCount  int        `json:"connect_count"`
	FailCount     int        `json:"fail_count"`
	Source        PeerSource `json:"source"`
	Protocols     []string   `json:"protocols"`
	LatencyMs     int64      `json:"latency_ms"`
}

// SuccessRate returns the connection success rate (0.0 to 1.0)
func (p *PeerRecord) SuccessRate() float64 {
	total := p.ConnectCount + p.FailCount
	if total == 0 {
		return 0.0
	}
	return float64(p.ConnectCount) / float64(total)
}

// IsStale returns true if the peer hasn't been seen recently
func (p *PeerRecord) IsStale(maxAge time.Duration) bool {
	return time.Since(p.LastSeen) > maxAge
}

// BlacklistEntry represents a blacklisted peer
type BlacklistEntry struct {
	PeerID    string    `json:"peer_id"`
	Reason    string    `json:"reason"`
	BlacklistedAt time.Time `json:"blacklisted_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"` // Empty = permanent
}

// IsExpired returns true if the blacklist entry has expired
func (b *BlacklistEntry) IsExpired() bool {
	if b.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(b.ExpiresAt)
}

// Storage defines the interface for persistent storage
type Storage interface {
	// Contact operations
	AddContact(contact *pb.Contact) error
	GetContact(pubKey []byte) (*pb.Contact, error)
	GetContactByBase58(pubKeyBase58 string) (*pb.Contact, error)
	GetContactX25519Key(pubKey []byte) ([]byte, error)
	ListContacts() ([]*pb.Contact, error)
	DeleteContact(pubKey []byte) error

	// Message operations
	AddMessage(contactPubKey []byte, envelope *pb.SignedEnvelope) error
	GetMessages(contactPubKey []byte, limit, offset int) ([]*pb.SignedEnvelope, error)
	GetMessagesWithTimestamps(contactPubKey []byte, limit, offset int) ([]*MessageWithKey, error)
	DeleteMessages(contactPubKey []byte) error

	// Peer operations
	AddPeer(peer *PeerRecord) error
	GetPeer(peerID string) (*PeerRecord, error)
	ListPeers(limit int) ([]*PeerRecord, error)
	ListPeersBySource(source PeerSource) ([]*PeerRecord, error)
	DeletePeer(peerID string) error
	PrunePeers(maxAgeDays int, keepCount int) error

	// Peer blacklist operations
	BlacklistPeer(peerID string, reason string) error
	IsBlacklisted(peerID string) (bool, error)
	ListBlacklisted() ([]*BlacklistEntry, error)
	RemoveFromBlacklist(peerID string) error

	// Config operations
	GetConfig(key string) (string, error)
	SetConfig(key, value string) error
	DeleteConfig(key string) error

	// Lifecycle
	Close() error
}

// MessageWithKey contains an envelope with its storage key components
type MessageWithKey struct {
	Envelope  *pb.SignedEnvelope
	Timestamp uint64
	Nonce     []byte
}

// ContactKeyToBase58 converts a public key to base58 string for display
func ContactKeyToBase58(pubKey []byte) string {
	return base58.Encode(pubKey)
}

// ContactKeyFromBase58 converts a base58 string back to public key bytes
func ContactKeyFromBase58(pubKeyBase58 string) ([]byte, error) {
	return base58.Decode(pubKeyBase58)
}
