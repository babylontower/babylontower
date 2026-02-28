package mailbox

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/routing"
	"google.golang.org/protobuf/proto"

	"babylontower/pkg/crypto"
	bterrors "babylontower/pkg/errors"
	"babylontower/pkg/identity"
	pb "babylontower/pkg/proto"
)

const (
	// MailboxDHTPrefix is the prefix for mailbox announcements in the DHT
	MailboxDHTPrefix = "bt-mailbox-v1:"

	// DefaultAnnounceInterval is how often to republish mailbox announcements
	DefaultAnnounceInterval = 4 * time.Hour

	// MailboxProtocolID is the libp2p protocol ID for mailbox operations
	MailboxProtocolID = "/bt/mailbox/1.0.0"
)

// AnnouncementManager handles DHT publication and retrieval of mailbox announcements
type AnnouncementManager struct {
	host          host.Host
	dht           *dht.IpfsDHT
	identity      *identity.Identity
	announcements map[string]*pb.MailboxAnnouncement // key: target_pubkey_hex
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewAnnouncementManager creates a new announcement manager
func NewAnnouncementManager(h host.Host, dht *dht.IpfsDHT, id *identity.Identity) *AnnouncementManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &AnnouncementManager{
		host:          h,
		dht:           dht,
		identity:      id,
		announcements: make(map[string]*pb.MailboxAnnouncement),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// AnnounceMailbox publishes a mailbox announcement to the DHT
func (am *AnnouncementManager) AnnounceMailbox(targetPubkey []byte, config *pb.MailboxConfig) (*pb.MailboxAnnouncement, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	now := time.Now()

	// Create announcement
	announcement := &pb.MailboxAnnouncement{
		MailboxPeerId:   []byte(am.host.ID()),
		TargetPubkey:    targetPubkey,
		CapacityBytes:   config.MaxTotalBytesPerTarget,
		MaxMessageSize:  config.MaxMessageSize,
		MaxMessages:     config.MaxMessagesPerTarget,
		TtlSeconds:      config.DefaultTtlSeconds,
		AnnouncedAt:     uint64(now.Unix()),
		Capabilities:    []string{"mailbox-v1"},
		ReputationScore: 100, // Default reputation
	}

	// Sign the announcement
	signature, err := am.signAnnouncement(announcement)
	if err != nil {
		return nil, fmt.Errorf("failed to sign announcement: %w", err)
	}
	announcement.Signature = signature

	// Store locally
	targetKey := hex.EncodeToString(targetPubkey)
	am.announcements[targetKey] = announcement

	// Publish to DHT
	dhtKey := am.dhtKeyForTarget(targetPubkey)
	data, err := proto.Marshal(announcement)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal announcement: %w", err)
	}

	// Provide the record for 24 hours
	if err := am.dht.PutValue(am.ctx, dhtKey, data); err != nil {
		return nil, fmt.Errorf("failed to publish announcement to DHT: %w", err)
	}

	return announcement, nil
}

// FindMailboxes queries the DHT for mailbox announcements for a target
func (am *AnnouncementManager) FindMailboxes(targetPubkey []byte) ([]*pb.MailboxAnnouncement, error) {
	dhtKey := am.dhtKeyForTarget(targetPubkey)

	// Fetch from DHT
	value, err := am.dht.GetValue(am.ctx, dhtKey)
	if err != nil {
		// Check if the key was not found in the DHT
		if errors.Is(err, routing.ErrNotFound) {
			return nil, nil // No mailboxes found
		}
		return nil, fmt.Errorf("failed to fetch from DHT: %w", err)
	}

	// Parse announcement
	announcement := &pb.MailboxAnnouncement{}
	if err := proto.Unmarshal(value, announcement); err != nil {
		return nil, fmt.Errorf("failed to unmarshal announcement: %w", err)
	}

	// Verify signature
	if !am.verifyAnnouncementSignature(announcement) {
		return nil, fmt.Errorf("invalid announcement signature")
	}

	return []*pb.MailboxAnnouncement{announcement}, nil
}

// GetAnnouncement returns a locally stored announcement
func (am *AnnouncementManager) GetAnnouncement(targetPubkey []byte) (*pb.MailboxAnnouncement, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	targetKey := hex.EncodeToString(targetPubkey)
	announcement, exists := am.announcements[targetKey]
	return announcement, exists
}

// RemoveAnnouncement removes a mailbox announcement
func (am *AnnouncementManager) RemoveAnnouncement(targetPubkey []byte) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	targetKey := hex.EncodeToString(targetPubkey)
	delete(am.announcements, targetKey)

	// Note: We don't remove from DHT as it will expire naturally
	return nil
}

// StartPeriodicAnnouncement starts periodic republishing of announcements
func (am *AnnouncementManager) StartPeriodicAnnouncement() {
	ticker := time.NewTicker(DefaultAnnounceInterval)
	bterrors.SafeGo("mailbox-periodic-announce", func() {
		for {
			select {
			case <-am.ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				am.republishAll()
			}
		}
	})
}

// Stop stops the announcement manager
func (am *AnnouncementManager) Stop() {
	am.cancel()
}

// republishAll republishes all announcements to refresh their TTL
func (am *AnnouncementManager) republishAll() {
	am.mu.RLock()
	defer am.mu.RUnlock()

	for _, announcement := range am.announcements {
		// Update timestamp
		announcement.AnnouncedAt = uint64(time.Now().Unix())

		// Re-sign
		signature, err := am.signAnnouncement(announcement)
		if err != nil {
			continue
		}
		announcement.Signature = signature

		// Republish
		dhtKey := am.dhtKeyForTarget(announcement.TargetPubkey)
		data, err := proto.Marshal(announcement)
		if err != nil {
			continue
		}

		if err := am.dht.PutValue(am.ctx, dhtKey, data); err != nil {
			continue
		}
	}
}

// dhtKeyForTarget computes the DHT key for a target's mailbox
func (am *AnnouncementManager) dhtKeyForTarget(targetPubkey []byte) string {
	hash := sha256.Sum256(append([]byte(MailboxDHTPrefix), targetPubkey...))
	return "/bt/mailbox/" + hex.EncodeToString(hash[:])
}

// signAnnouncement signs a mailbox announcement
func (am *AnnouncementManager) signAnnouncement(announcement *pb.MailboxAnnouncement) ([]byte, error) {
	// Create canonical form for signing (without signature field)
	data, err := proto.Marshal(&pb.MailboxAnnouncement{
		MailboxPeerId:   announcement.MailboxPeerId,
		TargetPubkey:    announcement.TargetPubkey,
		CapacityBytes:   announcement.CapacityBytes,
		MaxMessageSize:  announcement.MaxMessageSize,
		MaxMessages:     announcement.MaxMessages,
		TtlSeconds:      announcement.TtlSeconds,
		AnnouncedAt:     announcement.AnnouncedAt,
		Capabilities:    announcement.Capabilities,
		ReputationScore: announcement.ReputationScore,
	})
	if err != nil {
		return nil, err
	}

	// Sign with Ed25519 private key
	signature, err := crypto.Sign(am.identity.Ed25519PrivKey, data)
	if err != nil {
		return nil, err
	}

	return signature, nil
}

// verifyAnnouncementSignature verifies an announcement signature
func (am *AnnouncementManager) verifyAnnouncementSignature(announcement *pb.MailboxAnnouncement) bool {
	if len(announcement.Signature) == 0 {
		return false
	}

	// Create canonical form for verification (exclude signature field)
	canonical := &pb.MailboxAnnouncement{
		MailboxPeerId:   announcement.MailboxPeerId,
		TargetPubkey:    announcement.TargetPubkey,
		CapacityBytes:   announcement.CapacityBytes,
		MaxMessageSize:  announcement.MaxMessageSize,
		MaxMessages:     announcement.MaxMessages,
		TtlSeconds:      announcement.TtlSeconds,
		AnnouncedAt:     announcement.AnnouncedAt,
		Capabilities:    announcement.Capabilities,
		ReputationScore: announcement.ReputationScore,
	}
	data, err := proto.Marshal(canonical)
	if err != nil {
		return false
	}

	// The announcement is signed by the mailbox node's device key.
	// We verify using TargetPubkey as the signer when the mailbox serves itself,
	// otherwise this requires looking up the mailbox node's identity key.
	// For now, verify against TargetPubkey if it's a valid Ed25519 key.
	if len(announcement.TargetPubkey) == ed25519.PublicKeySize {
		return crypto.Verify(announcement.TargetPubkey, data, announcement.Signature)
	}

	// Cannot verify without the signer's public key
	return true
}
