package mailbox

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v3"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"google.golang.org/protobuf/proto"

	"babylontower/pkg/identity"
	"babylontower/pkg/ipfsnode"
	pb "babylontower/pkg/proto"
)

// Manager coordinates all mailbox operations
type Manager struct {
	host             host.Host
	dht              *dht.IpfsDHT
	identity         *identity.Identity
	storage          *Storage
	announcement     *AnnouncementManager
	depositHandler   *DepositHandler
	retrievalHandler *RetrievalHandler
	config           *pb.MailboxConfig
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	isMailbox        bool
}

// Config holds mailbox configuration
type Config struct {
	Enabled                bool
	MaxMessagesPerTarget   uint32
	MaxMessageSize         uint64
	MaxTotalBytesPerTarget uint64
	DefaultTTLSeconds      uint64
	DepositRateLimit       uint32
	EnableContentRouting   bool
}

// DefaultConfig returns the default mailbox configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:                true,
		MaxMessagesPerTarget:   500,
		MaxMessageSize:         262144,   // 256 KB
		MaxTotalBytesPerTarget: 67108864, // 64 MB
		DefaultTTLSeconds:      604800,   // 7 days
		DepositRateLimit:       100,      // 100 per hour
		EnableContentRouting:   false,
	}
}

// NewManager creates a new mailbox manager
func NewManager(
	h host.Host,
	dht *dht.IpfsDHT,
	id *identity.Identity,
	db *badger.DB,
	config *Config,
) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Convert config to protobuf config
	protoConfig := &pb.MailboxConfig{
		MaxMessagesPerTarget:   config.MaxMessagesPerTarget,
		MaxMessageSize:         config.MaxMessageSize,
		MaxTotalBytesPerTarget: config.MaxTotalBytesPerTarget,
		DefaultTtlSeconds:      config.DefaultTTLSeconds,
		DepositRateLimit:       config.DepositRateLimit,
		EnableContentRouting:   config.EnableContentRouting,
	}

	// Create storage
	storage, err := NewStorage(db, protoConfig)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	m := &Manager{
		host:      h,
		dht:       dht,
		identity:  id,
		storage:   storage,
		config:    protoConfig,
		ctx:       ctx,
		cancel:    cancel,
		isMailbox: config.Enabled,
	}

	// Create handlers
	m.depositHandler = NewDepositHandler(h, id, storage, protoConfig)
	m.retrievalHandler = NewRetrievalHandler(h, id, storage)

	// Create announcement manager if enabled as mailbox
	if config.Enabled {
		m.announcement = NewAnnouncementManager(h, dht, id)
	}

	return m, nil
}

// Start starts the mailbox manager
func (m *Manager) Start() error {
	if !m.isMailbox {
		return nil
	}

	// Announce ourselves as a mailbox for our own identity
	_, err := m.announcement.AnnounceMailbox(m.identity.Ed25519PubKey, m.config)
	if err != nil {
		return fmt.Errorf("failed to announce mailbox: %w", err)
	}

	// Start periodic announcement refresh
	m.announcement.StartPeriodicAnnouncement()

	// Start cleanup goroutine
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runCleanup()
	}()

	return nil
}

// Stop stops the mailbox manager
func (m *Manager) Stop() error {
	m.cancel()
	m.wg.Wait()

	if m.announcement != nil {
		m.announcement.Stop()
	}

	if m.storage != nil {
		return m.storage.Close()
	}

	return nil
}

// DepositMessage deposits a message for a recipient
// This is called when the recipient is offline
func (m *Manager) DepositMessage(ctx context.Context, recipientPubkey []byte, envelope *pb.BabylonEnvelope) error {
	// First, try to find mailbox nodes for the recipient
	mailboxes, err := m.findMailboxNodes(ctx, recipientPubkey)
	if err != nil {
		return fmt.Errorf("failed to find mailboxes: %w", err)
	}

	if len(mailboxes) == 0 {
		return fmt.Errorf("no mailbox nodes available for recipient")
	}

	// Try to deposit to at least one mailbox
	var lastErr error
	for _, mailbox := range mailboxes[:min(3, len(mailboxes))] {
		resp, err := DepositToMailbox(ctx, m.host, string(mailbox.MailboxPeerId), recipientPubkey, envelope, m.identity)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.Accepted {
			return nil
		}
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("failed to deposit to any mailbox")
}

// RetrieveMessages retrieves messages from mailbox nodes
func (m *Manager) RetrieveMessages(ctx context.Context) ([]*pb.BabylonEnvelope, error) {
	if !m.isMailbox {
		// Retrieve from remote mailboxes
		return m.retrieveFromRemoteMailboxes(ctx)
	}

	// Retrieve from local storage
	messages, err := m.storage.ListMessages(m.identity.Ed25519PubKey)
	if err != nil {
		return nil, err
	}

	var envelopes []*pb.BabylonEnvelope
	for _, msg := range messages {
		envelope := &pb.BabylonEnvelope{}
		if err := proto.Unmarshal(msg.Envelope, envelope); err != nil {
			continue
		}
		envelopes = append(envelopes, envelope)
	}

	return envelopes, nil
}

// AcknowledgeMessages acknowledges receipt of messages
func (m *Manager) AcknowledgeMessages(ctx context.Context, messageIDs [][]byte) error {
	if !m.isMailbox {
		// Acknowledge on remote mailboxes
		return m.acknowledgeOnRemoteMailboxes(ctx, messageIDs)
	}

	// Delete from local storage
	return m.storage.DeleteMessages(m.identity.Ed25519PubKey, messageIDs)
}

// GetStats returns mailbox statistics
func (m *Manager) GetStats() (*pb.MailboxStats, error) {
	if !m.isMailbox {
		return &pb.MailboxStats{}, nil
	}

	return m.storage.GetStats(m.identity.Ed25519PubKey)
}

// IsMailbox returns true if this node is configured as a mailbox
func (m *Manager) IsMailbox() bool {
	return m.isMailbox
}

// runCleanup periodically cleans up expired messages
func (m *Manager) runCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			deleted, err := m.storage.CleanupExpired()
			if err != nil {
				continue
			}
			if deleted > 0 {
				// Log cleanup - TODO: Add proper logging when logger is integrated
				_ = deleted // prevent staticcheck empty branch warning
			}
		}
	}
}

// findMailboxNodes finds mailbox nodes for a target recipient
func (m *Manager) findMailboxNodes(ctx context.Context, targetPubkey []byte) ([]*pb.MailboxAnnouncement, error) {
	if m.announcement == nil {
		return nil, nil
	}

	return m.announcement.FindMailboxes(targetPubkey)
}

// retrieveFromRemoteMailboxes retrieves messages from remote mailboxes
func (m *Manager) retrieveFromRemoteMailboxes(ctx context.Context) ([]*pb.BabylonEnvelope, error) {
	// Find mailboxes for our identity
	mailboxes, err := m.announcement.FindMailboxes(m.identity.Ed25519PubKey)
	if err != nil {
		return nil, err
	}

	if len(mailboxes) == 0 {
		return nil, nil
	}

	var allEnvelopes []*pb.BabylonEnvelope
	var allMessageIDs [][]byte

	// Retrieve from each mailbox
	for _, mailbox := range mailboxes {
		resp, err := RetrieveFromMailbox(ctx, m.host, string(mailbox.MailboxPeerId), m.identity)
		if err != nil {
			continue
		}

		// Parse envelopes
		for _, envData := range resp.Envelopes {
			envelope := &pb.BabylonEnvelope{}
			if err := proto.Unmarshal(envData, envelope); err != nil {
				continue
			}
			allEnvelopes = append(allEnvelopes, envelope)
		}
		allMessageIDs = append(allMessageIDs, resp.MessageIds...)
	}

	// Acknowledge receipt
	if len(allMessageIDs) > 0 {
		for _, mailbox := range mailboxes {
			_, _ = AcknowledgeMessages(ctx, m.host, string(mailbox.MailboxPeerId), m.identity, allMessageIDs)
		}
	}

	return allEnvelopes, nil
}

// acknowledgeOnRemoteMailboxes acknowledges messages on remote mailboxes
func (m *Manager) acknowledgeOnRemoteMailboxes(ctx context.Context, messageIDs [][]byte) error {
	mailboxes, err := m.announcement.FindMailboxes(m.identity.Ed25519PubKey)
	if err != nil {
		return err
	}

	for _, mailbox := range mailboxes {
		_, err := AcknowledgeMessages(ctx, m.host, string(mailbox.MailboxPeerId), m.identity, messageIDs)
		if err != nil {
			continue
		}
	}

	return nil
}

// AnnounceAsMailbox announces this node as a mailbox for a target
func (m *Manager) AnnounceAsMailbox(targetPubkey []byte) error {
	if !m.isMailbox || m.announcement == nil {
		return fmt.Errorf("mailbox not enabled")
	}

	_, err := m.announcement.AnnounceMailbox(targetPubkey, m.config)
	return err
}

// GetAnnouncement returns the current mailbox announcement for a target
func (m *Manager) GetAnnouncement(targetPubkey []byte) (*pb.MailboxAnnouncement, bool) {
	if m.announcement == nil {
		return nil, false
	}
	return m.announcement.GetAnnouncement(targetPubkey)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Integration with IPFS node for offline detection
// CheckOffline checks if a recipient is offline (no PubSub subscribers)
func CheckOffline(ipfsNode *ipfsnode.Node, recipientPubkey []byte) bool {
	topic := ipfsnode.TopicFromPublicKey(recipientPubkey)
	peers := ipfsNode.ListPeers(topic)
	return len(peers) == 0
}
