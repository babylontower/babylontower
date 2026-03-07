package mailbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/ipfs/go-log/v2"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"google.golang.org/protobuf/proto"

	"babylontower/pkg/identity"
	"babylontower/pkg/ipfsnode"
	pb "babylontower/pkg/proto"
)

var logger = log.Logger("babylontower/mailbox")

// Manager coordinates all mailbox operations
type Manager struct {
	host             host.Host
	dht              *dht.IpfsDHT
	ipfsNode         *ipfsnode.Node
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
	ipfsNode *ipfsnode.Node,
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
		ipfsNode:  ipfsNode,
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

	// Wait for Babylon DHT bootstrap before attempting announcements
	// This ensures we're connected to Babylon network for proper DHT operations
	if err := m.ipfsNode.WaitForBabylonBootstrap(60 * time.Second); err != nil {
		logger.Warnw("Babylon DHT bootstrap timeout, mailbox DHT announcements may fail", "error", err)
		// Continue anyway - mailbox can still work via direct peer connections
	}

	// Announce ourselves as a mailbox node (generic, serves any identity)
	// We announce for our own identity as a bootstrap mechanism
	// Other nodes can deposit messages for ANY identity to this mailbox
	var err error
	if m.announcement != nil {
		_, err = m.announcement.AnnounceMailbox(m.identity.Ed25519PubKey, m.config)
		if err != nil {
			// DHT announcement often fails due to key format restrictions
			// This is OK - mailbox still works for:
			// 1. Direct deposits from connected peers
			// 2. Local storage fallback
			logger.Debugw("mailbox DHT announcement failed (expected in some networks)",
				"error", err,
				"mode", "direct-peer-and-local-storage")
		}

		// Start periodic announcement refresh (if announcement succeeded)
		m.announcement.StartPeriodicAnnouncement()
	} else {
		logger.Debugw("mailbox announcement manager not available, skipping DHT announcements")
	}

	// Start cleanup goroutine
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runCleanup()
	}()

	logger.Info("mailbox manager started",
		"serves_any_identity", true,
		"dht_announced", err == nil)
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
//
// Delivery strategy (per protocol v1, Section 6):
// 1. Try connected peers that accept mailbox deposits (direct libp2p stream)
// 2. Try DHT-discovered mailboxes (up to 3, in parallel)
// 3. If no mailbox available, log warning and return (message already sent via PubSub)
func (m *Manager) DepositMessage(ctx context.Context, recipientPubkey []byte, envelope *pb.BabylonEnvelope) error {
	// Validate inputs to prevent panics
	if len(recipientPubkey) == 0 {
		return errors.New("recipient pubkey is empty")
	}
	if envelope == nil {
		return errors.New("envelope is nil")
	}

	logger.Infow("depositing message to mailbox",
		"recipient", hex.EncodeToString(recipientPubkey)[:16])

	// Step 1: Try connected peers that support the mailbox protocol
	// Only attempt deposit to peers that have registered the /bt/mailbox/1.0.0 handler
	connectedPeers := m.host.Network().Peers()
	babylonPeersAttempted := 0

	for _, peerID := range connectedPeers {
		if peerID == m.host.ID() {
			continue
		}

		// Check if peer supports mailbox protocol before opening stream
		protos, err := m.host.Peerstore().GetProtocols(peerID)
		if err != nil {
			continue
		}
		supportsMailbox := false
		for _, p := range protos {
			if string(p) == MailboxProtocolID {
				supportsMailbox = true
				break
			}
		}
		if !supportsMailbox {
			continue
		}

		babylonPeersAttempted++
		logger.Debugw("attempting deposit to mailbox peer",
			"peer_id", peerID,
			"recipient", hex.EncodeToString(recipientPubkey)[:16])

		resp, err := DepositToMailbox(ctx, m.host, peerID.String(), recipientPubkey, envelope, m.identity)
		if err == nil && resp.Accepted {
			logger.Infow("message deposited to connected peer",
				"recipient", hex.EncodeToString(recipientPubkey)[:16],
				"peer_id", peerID)
			return nil
		}
		logger.Debugw("mailbox peer declined or unavailable",
			"peer_id", peerID,
			"error", err)
	}

	if babylonPeersAttempted == 0 {
		logger.Debugw("no connected peers support mailbox protocol",
			"total_connected", len(connectedPeers))
	}

	// Step 2: Try DHT-discovered mailboxes in PARALLEL (up to 3)
	mailboxes, err := m.findMailboxNodes(ctx, recipientPubkey)
	if err != nil {
		logger.Debugw("mailbox DHT lookup failed", "recipient", hex.EncodeToString(recipientPubkey)[:16], "error", err)
	}

	if len(mailboxes) > 0 {
		logger.Debugw("found DHT mailboxes", "count", len(mailboxes))

		// Try up to 3 mailboxes in parallel
		maxMailboxes := 3
		if len(mailboxes) < maxMailboxes {
			maxMailboxes = len(mailboxes)
		}

		// Channel to collect results
		type depositResult struct {
			mailbox *pb.MailboxAnnouncement
			resp    *pb.DepositResponse
			err     error
		}
		results := make(chan depositResult, maxMailboxes)

		// Launch parallel deposits
		var wg sync.WaitGroup
		for i := 0; i < maxMailboxes; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				mailbox := mailboxes[idx]

				logger.Debugw("attempting deposit to DHT mailbox",
					"mailbox_peer", mailbox.MailboxPeerId,
					"recipient", hex.EncodeToString(recipientPubkey)[:16])

				resp, err := DepositToMailbox(ctx, m.host, string(mailbox.MailboxPeerId), recipientPubkey, envelope, m.identity)
				results <- depositResult{mailbox: mailbox, resp: resp, err: err}
			}(i)
		}

		// Wait for all attempts with timeout
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		// Collect results
		successCount := 0
		var lastErr error
		for i := 0; i < maxMailboxes; i++ {
			select {
			case result := <-results:
				if result.err == nil && result.resp.Accepted {
					successCount++
					logger.Infow("message deposited to DHT mailbox",
						"recipient", hex.EncodeToString(recipientPubkey)[:16],
						"mailbox_peer", result.mailbox.MailboxPeerId)
				} else {
					lastErr = result.err
					logger.Debugw("DHT mailbox deposit failed",
						"mailbox_peer", result.mailbox.MailboxPeerId,
						"error", result.err)
				}
			case <-done:
				break
			}
		}

		// Success if at least one mailbox accepted
		if successCount > 0 {
			logger.Infow("parallel mailbox deposit complete",
				"recipient", hex.EncodeToString(recipientPubkey)[:16],
				"successful_deposits", successCount,
				"attempted", maxMailboxes)
			return nil
		}

		if lastErr != nil {
			logger.Warnw("failed to deposit to DHT-discovered mailboxes",
				"recipient", hex.EncodeToString(recipientPubkey)[:16],
				"error", lastErr)
		}
	} else {
		logger.Debugw("no DHT mailboxes found")
	}

	// Step 3: Store locally as fallback
	// This node acts as its own mailbox — when the recipient connects later,
	// they can retrieve messages from our local storage via the retrieval protocol
	envelopeBytes, err := proto.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("failed to marshal envelope for local storage: %w", err)
	}

	messageID := generateMessageID(recipientPubkey, envelopeBytes)
	senderPubkey := envelope.SenderIdentity
	ttl := m.config.DefaultTtlSeconds
	if ttl == 0 {
		ttl = 604800 // 7 days default
	}

	if err := m.storage.StoreMessage(recipientPubkey, messageID, senderPubkey, envelopeBytes, ttl); err != nil {
		logger.Warnw("failed to store message in local mailbox",
			"recipient", hex.EncodeToString(recipientPubkey)[:16],
			"error", err)
		return fmt.Errorf("local mailbox deposit failed: %w", err)
	}

	logger.Infow("message stored in local mailbox for offline delivery",
		"recipient", hex.EncodeToString(recipientPubkey)[:16])
	return nil
}

// MailboxRetrievalResult contains messages retrieved from mailbox
type MailboxRetrievalResult struct {
	Envelopes  []*pb.BabylonEnvelope
	MessageIDs [][]byte
}

// RetrieveMessages retrieves messages from mailbox nodes
// Strategy: Query connected peers first (they may have stored messages for us),
// then check local storage, then try DHT-discovered mailboxes
func (m *Manager) RetrieveMessages(ctx context.Context) (*MailboxRetrievalResult, error) {
	var allEnvelopes []*pb.BabylonEnvelope
	var allMessageIDs [][]byte

	logger.Infow("retrieving messages from mailbox", "method", "starting")

	// Step 1: Query connected peers that support mailbox protocol
	connectedPeers := m.host.Network().Peers()

	for _, peerID := range connectedPeers {
		if peerID == m.host.ID() {
			continue
		}

		// Only query peers that support the mailbox protocol
		protos, err := m.host.Peerstore().GetProtocols(peerID)
		if err != nil {
			continue
		}
		supportsMailbox := false
		for _, p := range protos {
			if string(p) == MailboxProtocolID {
				supportsMailbox = true
				break
			}
		}
		if !supportsMailbox {
			continue
		}

		{
			resp, err := RetrieveFromMailbox(ctx, m.host, peerID.String(), m.identity)
			if err != nil {
				logger.Debugw("peer mailbox query failed", "peer_id", peerID, "error", err)
				continue
			}

			if len(resp.Envelopes) > 0 {
				logger.Infow("retrieved messages from peer",
					"peer_id", peerID,
					"count", len(resp.Envelopes))
			}

			// Parse envelopes from response
			for _, envData := range resp.Envelopes {
				envelope := &pb.BabylonEnvelope{}
				if err := proto.Unmarshal(envData, envelope); err != nil {
					continue
				}
				allEnvelopes = append(allEnvelopes, envelope)
			}
			allMessageIDs = append(allMessageIDs, resp.MessageIds...)

			// Acknowledge receipt from this peer
			if len(resp.MessageIds) > 0 {
				_, _ = AcknowledgeMessages(ctx, m.host, peerID.String(), m.identity, resp.MessageIds)
			}
		}
	}

	// Step 2: Check local storage (for messages stored when both nodes on same machine)
	localMessages, err := m.storage.ListMessages(m.identity.Ed25519PubKey)
	if err != nil {
		logger.Debugw("local storage query failed", "error", err)
	} else if len(localMessages) > 0 {
		logger.Infow("retrieved messages from local storage", "count", len(localMessages))

		for _, msg := range localMessages {
			envelope := &pb.BabylonEnvelope{}
			if err := proto.Unmarshal(msg.Envelope, envelope); err != nil {
				continue
			}
			allEnvelopes = append(allEnvelopes, envelope)
			allMessageIDs = append(allMessageIDs, msg.MessageID)
		}

		// Delete from local storage after retrieving
		if len(localMessages) > 0 {
			_ = m.storage.DeleteMessages(m.identity.Ed25519PubKey,
				collectMessageIDs(localMessages))
		}
	}

	// Step 3: Try DHT-discovered mailboxes (if any)
	mailboxes, err := m.announcement.FindMailboxes(m.identity.Ed25519PubKey)
	if err == nil && len(mailboxes) > 0 {
		logger.Debugw("found DHT mailboxes", "count", len(mailboxes))
		for _, mailbox := range mailboxes {
			resp, err := RetrieveFromMailbox(ctx, m.host, string(mailbox.MailboxPeerId), m.identity)
			if err != nil {
				continue
			}

			if len(resp.Envelopes) > 0 {
				logger.Infow("retrieved messages from DHT mailbox",
					"mailbox_peer", mailbox.MailboxPeerId,
					"count", len(resp.Envelopes))
			}

			for _, envData := range resp.Envelopes {
				envelope := &pb.BabylonEnvelope{}
				if err := proto.Unmarshal(envData, envelope); err != nil {
					continue
				}
				allEnvelopes = append(allEnvelopes, envelope)
			}
			allMessageIDs = append(allMessageIDs, resp.MessageIds...)

			if len(resp.MessageIds) > 0 {
				_, _ = AcknowledgeMessages(ctx, m.host, string(mailbox.MailboxPeerId), m.identity, resp.MessageIds)
			}
		}
	} else {
		logger.Debugw("no DHT mailboxes found")
	}

	if len(allEnvelopes) > 0 {
		logger.Infow("mailbox retrieval complete",
			"total_messages", len(allEnvelopes),
			"from_peers", true)
	} else {
		logger.Debugw("mailbox retrieval complete", "total_messages", 0)
	}

	return &MailboxRetrievalResult{
		Envelopes:  allEnvelopes,
		MessageIDs: allMessageIDs,
	}, nil
}

// generateMessageID generates a unique message ID from recipient and envelope data
func generateMessageID(recipientPubkey, envelopeBytes []byte) []byte {
	h := sha256.New()
	h.Write(recipientPubkey)
	h.Write(envelopeBytes)
	sum := h.Sum(nil)
	return sum[:16]
}

// collectMessageIDs extracts message IDs from a slice of StoredMessage
func collectMessageIDs(messages []*StoredMessage) [][]byte {
	ids := make([][]byte, len(messages))
	for i, msg := range messages {
		ids[i] = msg.MessageID
	}
	return ids
}

// AcknowledgeMessages acknowledges receipt of messages
// This is called after processing messages retrieved from remote peers
// For local storage, messages are deleted immediately after retrieval
func (m *Manager) AcknowledgeMessages(ctx context.Context, messageIDs [][]byte) error {
	// Delete from local storage (for messages we stored locally)
	return m.storage.DeleteMessages(m.identity.Ed25519PubKey, messageIDs)
}

// GetStats returns mailbox statistics (total across all stored targets)
func (m *Manager) GetStats() (*pb.MailboxStats, error) {
	if !m.isMailbox {
		return &pb.MailboxStats{}, nil
	}

	return m.storage.GetTotalStats()
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

// AnnounceAsMailbox announces this node as a mailbox for a target
func (m *Manager) AnnounceAsMailbox(targetPubkey []byte) error {
	if !m.isMailbox || m.announcement == nil {
		return errors.New("mailbox not enabled")
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

// CheckOffline checks if a recipient is offline by checking PubSub topic subscribers
// Note: This is a best-effort check. A recipient is considered "online" if they have
// subscribers on their topic (meaning they're actively receiving messages).
//
// In practice, the messaging flow is:
// 1. Try PubSub delivery first (works for online peers)
// 2. If PubSub fails AND recipient is offline, use mailbox as fallback
func CheckOffline(ipfsNode *ipfsnode.Node, recipientPubkey []byte) bool {
	topic := ipfsnode.TopicFromPublicKey(recipientPubkey)
	peers := ipfsNode.ListPeers(topic)

	// If there are subscribers on the topic, consider the recipient online
	// Note: This includes the recipient themselves (they subscribe to their own topic)
	return len(peers) == 0
}
