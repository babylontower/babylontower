package messaging

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	bterrors "babylontower/pkg/errors"
	"babylontower/pkg/identity"
	"babylontower/pkg/ipfsnode"
	"babylontower/pkg/mailbox"
	pb "babylontower/pkg/proto"
	"babylontower/pkg/reputation"
	"babylontower/pkg/storage"

	"github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

var logger = log.Logger("babylontower/messaging")

// Re-export sentinel errors from the centralized errors package for backward compatibility.
var (
	ErrServiceNotStarted = bterrors.ErrServiceNotStarted
	ErrUnknownContact    = bterrors.ErrUnknownContact
	ErrSelfMessage       = bterrors.ErrSelfMessage
)

// Config holds configuration for the messaging service
type Config struct {
	// OwnEd25519PrivKey is the owner's Ed25519 private key for signing
	OwnEd25519PrivKey ed25519.PrivateKey
	// OwnEd25519PubKey is the owner's Ed25519 public key for identity
	OwnEd25519PubKey ed25519.PublicKey
	// OwnX25519PrivKey is the owner's X25519 private key for decryption
	OwnX25519PrivKey []byte
	// OwnX25519PubKey is the owner's X25519 public key for sharing
	OwnX25519PubKey []byte
	// IdentityV1 is the v1 identity (optional, for protocol v1 features)
	IdentityV1 *identity.IdentityV1
}

// Messenger is the interface for messaging operations.
// This allows dependency injection and testing without concrete implementations.
type Messenger interface {
	// Lifecycle
	Start() error
	Stop() error

	// Messaging operations
	SendMessageToContact(text string, recipientEd25519PubKey, recipientX25519PubKey []byte) (*SendResult, error)
	GetDecryptedMessagesWithMeta(contactPubKey []byte, limit, offset int) ([]*MessageWithMeta, error)
	Messages() <-chan *MessageEvent
	GetContactStatus(contactPubKey []byte) (*ContactStatus, error)
	GetAllContactStatuses() ([]*ContactStatus, error)
	IsStarted() bool

	// Mailbox
	GetMailboxManager() *mailbox.Manager
	RetrieveOfflineMessages() error

	// Reputation tracker access
	ReputationTracker() *reputation.Tracker
}

// Ensure Service implements Messenger interface
var _ Messenger = (*Service)(nil)

// MessageEvent represents a new message received
type MessageEvent struct {
	// ContactPubKey is the sender's Ed25519 public key
	ContactPubKey []byte
	// Message is the decrypted message
	Message *pb.Message
	// Envelope is the original signed envelope (for storage)
	Envelope *pb.SignedEnvelope
}

// Service is the main messaging service that handles all protocol operations
type Service struct {
	config       *Config
	storage      storage.Storage
	ipfsNode     *ipfsnode.Node
	subscription *ipfsnode.Subscription

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	isStarted bool

	// Channel for incoming message events
	messageChan chan *MessageEvent

	mu sync.RWMutex

	// Contact peer tracking (cached peer IDs for quick lookup)
	contactPeers map[string]*ContactPeerInfo // key: hex-encoded Ed25519 pubkey
	contactMu    sync.RWMutex

	// Reputation tracker
	reputationTracker *reputation.Tracker

	// Mailbox manager for offline message delivery
	mailboxManager *mailbox.Manager

	// Identity document manager for DHT publication
	identityManager *identity.DHTIdentityManager
	identityV1      *identity.IdentityV1

	// Lazy bootstrap tracking
	lazyBootstrapTriggered map[string]bool // key: hex-encoded sender pubkey
	lazyBootstrapMu        sync.RWMutex
}

// ContactPeerInfo contains cached information about a contact's peer presence
type ContactPeerInfo struct {
	PeerID     string
	Multiaddrs []string
	LastSeen   time.Time
	IsOnline   bool
	Connected  bool
}

// NewService creates a new messaging service
func NewService(config *Config, storage storage.Storage, ipfsNode *ipfsnode.Node) *Service {
	ctx, cancel := context.WithCancel(context.Background())

	return &Service{
		config:                 config,
		storage:                storage,
		ipfsNode:               ipfsNode,
		ctx:                    ctx,
		cancel:                 cancel,
		isStarted:              false,
		messageChan:            make(chan *MessageEvent, 100),
		contactPeers:           make(map[string]*ContactPeerInfo),
		lazyBootstrapTriggered: make(map[string]bool),
	}
}

// Start initializes and starts the messaging service
// Subscribes to the owner's topic and starts the message listener
func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isStarted {
		return nil
	}

	if s.ipfsNode == nil || !s.ipfsNode.IsStarted() {
		return errors.New("IPFS node not started")
	}

	if s.storage == nil {
		return errors.New("storage not initialized")
	}

	// Initialize identity document manager if IdentityV1 is available
	if s.config.IdentityV1 != nil {
		s.identityV1 = s.config.IdentityV1
		s.identityManager = identity.NewDHTIdentityManager(s.ipfsNode)

		// Publish identity document to DHT AFTER Babylon bootstrap completes
		// This is critical per protocol spec - identity documents require Babylon DHT
		// Run asynchronously to avoid blocking startup
		go func() {
			// Wait for Babylon DHT bootstrap to complete (max 60 seconds)
			// Babylon DHT is the protocol layer required for identity operations
			if err := s.ipfsNode.WaitForBabylonBootstrap(60 * time.Second); err != nil {
				logger.Warnw("Babylon DHT bootstrap timeout, deferring identity publication", "error", err)
				return
			}

			// Now publish identity document
			if err := s.publishIdentityDocument(); err != nil {
				logger.Warnw("failed to publish identity document (will retry later)", "error", err)
			} else {
				logger.Infow("identity document published successfully",
					"fingerprint", s.identityV1.IdentityFingerprint())
			}
		}()

		// Start periodic republish (every 4 hours as per protocol spec)
		s.wg.Add(1)
		go s.periodicIdentityRepublish()

		logger.Infow("identity document publication scheduled (waiting for Babylon DHT bootstrap)",
			"fingerprint", s.identityV1.IdentityFingerprint())
	}

	// Subscribe to own topic
	topic := ipfsnode.TopicFromPublicKey(s.config.OwnEd25519PubKey)
	sub, err := s.ipfsNode.Subscribe(topic)
	if err != nil {
		return fmt.Errorf("failed to subscribe to topic: %w", err)
	}

	s.subscription = sub
	s.isStarted = true

	// Start message listener goroutine
	s.wg.Add(1)
	go s.listenForMessages()

	// Start periodic message retrieval from mailbox
	s.wg.Add(1)
	go s.periodicMessageRetrieval()

	// Start periodic presence announcement
	s.wg.Add(1)
	go s.periodicPresenceAnnouncement()

	logger.Infow("messaging service started", "topic", topic)

	return nil
}

// publishIdentityDocument creates and publishes an identity document to the DHT
func (s *Service) publishIdentityDocument() error {
	if s.identityManager == nil || s.identityV1 == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// Wait for Babylon DHT to be ready before publishing
	// Per protocol spec section 1.4, identity documents MUST be published to Babylon DHT
	// with custom validators, not the default IPFS DHT
	if err := s.waitForBabylonDHT(ctx); err != nil {
		return fmt.Errorf("Babylon DHT not ready, cannot publish identity document: %w", err)
	}

	// Create identity document manager
	docManager := identity.NewIdentityDocumentManager(s.identityV1)

	// Generate prekeys
	spk, err := s.identityV1.GenerateSignedPrekey(1)
	if err != nil {
		return fmt.Errorf("failed to generate SPK: %w", err)
	}

	opks, err := s.identityV1.GenerateOneTimePrekeys(1, identity.OPKBatchSize)
	if err != nil {
		return fmt.Errorf("failed to generate OPKs: %w", err)
	}

	// Create device certificate
	deviceCert, err := s.identityV1.CreateDeviceCertificate()
	if err != nil {
		return fmt.Errorf("failed to create device certificate: %w", err)
	}

	// Create identity document
	doc, err := docManager.CreateIdentityDocument(
		0, // prevSequence
		nil, // prevHash
		[]*pb.DeviceCertificate{deviceCert},
		[]*pb.SignedPrekey{spk},
		opks,
		s.identityV1.DeviceName,
	)
	if err != nil {
		return fmt.Errorf("failed to create identity document: %w", err)
	}

	// Publish to DHT
	if err := s.identityManager.PublishIdentityDocument(ctx, doc); err != nil {
		return fmt.Errorf("failed to publish identity document: %w", err)
	}

	// Also publish prekey bundle separately for efficient lookup
	if err := s.identityManager.PublishPrekeyBundle(ctx, s.identityV1.IKSignPub, []*pb.SignedPrekey{spk}, opks); err != nil {
		logger.Warnw("failed to publish prekey bundle", "error", err)
	}

	logger.Infow("identity document published to DHT",
		"sequence", doc.Sequence,
		"devices", len(doc.Devices),
		"spks", len(doc.SignedPrekeys),
		"opks", len(doc.OneTimePrekeys))

	return nil
}

// waitForBabylonDHT waits for Babylon DHT to be ready for protocol operations
func (s *Service) waitForBabylonDHT(ctx context.Context) error {
	// Check if IPFS node is initialized
	if s.ipfsNode == nil {
		return fmt.Errorf("IPFS node not initialized")
	}

	// Wait for Babylon DHT with timeout
	if err := s.ipfsNode.WaitForBabylonDHT(30 * time.Second); err != nil {
		return fmt.Errorf("Babylon DHT not ready: %w", err)
	}

	logger.Debug("Babylon DHT ready for protocol operations")
	return nil
}

// periodicIdentityRepublish republishes the identity document every 4 hours
func (s *Service) periodicIdentityRepublish() {
	defer s.wg.Done()

	ticker := time.NewTicker(4 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.publishIdentityDocument(); err != nil {
				logger.Warnw("periodic identity republish failed", "error", err)
			} else {
				logger.Debug("identity document republished")
			}
		}
	}
}

// periodicPresenceAnnouncement publishes presence announcements periodically
func (s *Service) periodicPresenceAnnouncement() {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Check if service is still valid
			if s == nil || s.ipfsNode == nil || s.config == nil {
				logger.Debugw("presence announcement skipped - service not ready")
				continue
			}

			// Publish presence announcement to signal we're online
			if err := s.ipfsNode.PublishPresenceAnnouncement(s.ctx, s.config.OwnEd25519PubKey); err != nil {
				logger.Debugw("presence announcement failed", "error", err)
			}
		}
	}
}

// Stop gracefully shuts down the messaging service
func (s *Service) Stop() error {
	s.mu.Lock()
	if !s.isStarted {
		s.mu.Unlock()
		return nil
	}
	s.isStarted = false
	s.mu.Unlock()

	logger.Info("stopping messaging service...")

	// Cancel context to stop all goroutines
	s.cancel()

	// Wait for goroutines to finish
	s.wg.Wait()

	// Close subscription
	if s.subscription != nil {
		if err := s.subscription.Close(); err != nil {
			logger.Warnw("subscription close error", "error", err)
		}
	}

	logger.Info("messaging service stopped")

	return nil
}

// listenForMessages listens for incoming PubSub messages
func (s *Service) listenForMessages() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case pubsubMsg, ok := <-s.subscription.Messages():
			if !ok {
				return
			}
			s.handlePubSubMessage(pubsubMsg)
		case err, ok := <-s.subscription.Errors():
			if !ok {
				return
			}
			logger.Warnw("subscription error", "error", err)
		}
	}
}

// periodicMessageRetrieval periodically retrieves messages from mailbox nodes
func (s *Service) periodicMessageRetrieval() {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.RetrieveOfflineMessages(); err != nil {
				logger.Debugw("periodic message retrieval failed", "error", err)
			}
		}
	}
}

// RetrieveOfflineMessages retrieves messages from mailbox nodes
func (s *Service) RetrieveOfflineMessages() error {
	if s.mailboxManager == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	result, err := s.mailboxManager.RetrieveMessages(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve messages from mailbox: %w", err)
	}

	if len(result.Envelopes) == 0 {
		return nil
	}

	logger.Debugw("retrieved messages from mailbox", "count", len(result.Envelopes))

	// Process all envelopes first
	for _, envelope := range result.Envelopes {
		if err := s.processBabylonEnvelope(envelope); err != nil {
			logger.Warnw("failed to process mailbox message", "error", err)
		}
	}

	// Acknowledge messages after successful processing
	// This deletes them from the mailbox to prevent duplicate delivery
	if err := s.mailboxManager.AcknowledgeMessages(ctx, result.MessageIDs); err != nil {
		logger.Warnw("failed to acknowledge mailbox messages", "error", err)
		// Don't fail the retrieval - messages were already processed
	} else {
		logger.Debugw("acknowledged mailbox messages", "count", len(result.MessageIDs))
	}

	return nil
}

// processBabylonEnvelope processes a BabylonEnvelope from mailbox storage.
// The Payload field contains serialized SignedEnvelope bytes — the same format
// that arrives via PubSub — so we reuse the standard processEnvelope path.
func (s *Service) processBabylonEnvelope(envelope *pb.BabylonEnvelope) error {
	if len(envelope.Payload) == 0 {
		return errors.New("empty babylon envelope payload")
	}
	return s.processEnvelope(envelope.Payload)
}

// handlePubSubMessage processes an incoming PubSub message
// LAZY BOOTSTRAP: Triggers Babylon bootstrap on first message from unknown peer
func (s *Service) handlePubSubMessage(pubsubMsg *ipfsnode.Message) {
	logger.Infow("received PubSub message",
		"size", len(pubsubMsg.Data),
		"from", pubsubMsg.From,
		"topic", pubsubMsg.Topic)

	// === LAZY BOOTSTRAP TRIGGER ===
	// Check if this is the first message from a Babylon peer
	// and trigger lazy bootstrap if our Babylon DHT is not ready
	s.checkAndTriggerLazyBootstrap(pubsubMsg.From)

	// Process the envelope directly
	if err := s.processEnvelope(pubsubMsg.Data); err != nil {
		logger.Errorw("failed to process PubSub envelope", "error", err, "from", pubsubMsg.From)
		return
	}
	logger.Infow("PubSub message processed successfully", "from", pubsubMsg.From)
}

// checkAndTriggerLazyBootstrap checks if lazy bootstrap should be triggered
// and triggers it if conditions are met
func (s *Service) checkAndTriggerLazyBootstrap(sender peer.ID) {
	// Skip if Babylon bootstrap is already complete
	if s.ipfsNode.IsBabylonBootstrapComplete() {
		return
	}

	senderKey := sender.String()

	// Check if we already triggered bootstrap for this sender
	s.lazyBootstrapMu.RLock()
	alreadyTriggered := s.lazyBootstrapTriggered[senderKey]
	s.lazyBootstrapMu.RUnlock()

	if alreadyTriggered {
		return
	}

	// Mark as triggered to avoid duplicate triggers
	s.lazyBootstrapMu.Lock()
	s.lazyBootstrapTriggered[senderKey] = true
	s.lazyBootstrapMu.Unlock()

	// Trigger lazy bootstrap
	logger.Infow("lazy Babylon bootstrap triggered by first message from peer",
		"sender", sender.String(),
		"bootstrap_complete", s.ipfsNode.IsBabylonBootstrapComplete(),
		"bootstrap_deferred", s.ipfsNode.IsBabylonBootstrapDeferred())

	// Run bootstrap in goroutine to avoid blocking message processing
	go func() {
		if err := s.ipfsNode.TriggerLazyBootstrap(); err != nil {
			logger.Debugw("lazy bootstrap trigger failed",
				"sender", sender.String(),
				"error", err)
		} else {
			logger.Infow("lazy bootstrap completed successfully",
				"triggered_by", sender.String())
		}
	}()
}

// processEnvelope verifies, decrypts, and stores an incoming envelope
// This is called internally when a PubSub message is received
func (s *Service) processEnvelope(envelopeBytes []byte) error {
	// Parse signed envelope
	signedEnvelope, err := ParseSignedEnvelope(envelopeBytes)
	if err != nil {
		return fmt.Errorf("failed to parse signed envelope: %w", err)
	}

	// Verify signature
	valid, err := VerifyEnvelope(signedEnvelope)
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}
	if !valid {
		return ErrInvalidSignature
	}

	// Parse envelope
	envelope, err := ParseEnvelope(signedEnvelope)
	if err != nil {
		return fmt.Errorf("failed to parse envelope: %w", err)
	}

	// Decrypt envelope
	plaintext, err := DecryptEnvelope(envelope, s.config.OwnX25519PrivKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt envelope: %w", err)
	}

	// Parse message
	msg, err := ParseMessage(plaintext)
	if err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Get sender's public key
	senderPubKey := signedEnvelope.SenderPubkey

	// Check if sender is a known contact (optional for PoC)
	contact, err := s.storage.GetContact(senderPubKey)
	if err != nil {
		logger.Debugw("message from unknown contact", "sender", hex.EncodeToString(senderPubKey))
		// For PoC, we still process the message
	}
	_ = contact

	// Store message
	if err := s.storage.AddMessage(senderPubKey, signedEnvelope); err != nil {
		logger.Warnw("failed to store message", "error", err)
	}

	// Emit message event
	event := &MessageEvent{
		ContactPubKey: senderPubKey,
		Message:       msg,
		Envelope:      signedEnvelope,
	}

	select {
	case s.messageChan <- event:
		logger.Debugw("message event emitted", "from", hex.EncodeToString(senderPubKey))
	default:
		logger.Warnw("message channel full, dropping event")
	}

	return nil
}

// Messages returns the channel for receiving message events
func (s *Service) Messages() <-chan *MessageEvent {
	if s == nil || s.messageChan == nil {
		// Return a closed channel instead of nil to prevent panics
		ch := make(chan *MessageEvent)
		close(ch)
		return ch
	}
	return s.messageChan
}

// GetTopic returns the PubSub topic for this service's owner
func (s *Service) GetTopic() string {
	return ipfsnode.TopicFromPublicKey(s.config.OwnEd25519PubKey)
}

// IsStarted returns true if the service is running
func (s *Service) IsStarted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isStarted
}

// GetContactX25519PubKey retrieves a contact's X25519 public key
// This is a helper that would need to be implemented with contact storage
// For PoC, we assume the caller provides the X25519 key directly
func GetContactX25519PubKey(contactEd25519PubKey []byte) ([]byte, error) {
	// In a full implementation, this would:
	// 1. Look up the contact in storage
	// 2. Retrieve their X25519 public key (stored when contact was added)
	// For PoC, we return an error indicating this needs to be handled by the caller
	return nil, errors.New("contact X25519 key lookup not implemented - caller must provide key")
}

// SerializeEnvelope serializes a SignedEnvelope to bytes
func SerializeEnvelope(envelope *pb.SignedEnvelope) ([]byte, error) {
	return proto.Marshal(envelope)
}

// DeserializeEnvelope deserializes bytes to a SignedEnvelope
func DeserializeEnvelope(data []byte) (*pb.SignedEnvelope, error) {
	return ParseSignedEnvelope(data)
}

// FindAndConnectResult contains the result of a find and connect operation
type FindAndConnectResult struct {
	// PeerID is the discovered peer's libp2p ID
	PeerID string
	// Addresses is the list of discovered multiaddresses
	Addresses []string
	// Source indicates how the peer was found ("address_book", "dht", "manual")
	Source string
}

// FindAndConnectToContact attempts to find and connect to a contact's peer
// It uses contact-aware routing with priority connection
func (s *Service) FindAndConnectToContact(contactPubKey []byte) (*FindAndConnectResult, error) {
	s.mu.RLock()
	if !s.isStarted {
		s.mu.RUnlock()
		return nil, ErrServiceNotStarted
	}
	s.mu.RUnlock()

	// Step 1: Check cache first
	s.contactMu.RLock()
	cachedInfo, cached := s.contactPeers[string(contactPubKey)]
	s.contactMu.RUnlock()

	if cached && cachedInfo != nil && cachedInfo.Connected {
		logger.Debugw("already connected to contact", "contact", hex.EncodeToString(contactPubKey))
		return &FindAndConnectResult{
			PeerID:    cachedInfo.PeerID,
			Addresses: cachedInfo.Multiaddrs,
			Source:    "cached",
		}, nil
	}

	// Step 2: Get contact from storage
	contact, err := s.storage.GetContact(contactPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get contact: %w", err)
	}
	if contact == nil {
		return nil, ErrUnknownContact
	}

	// Step 3: Try cached PeerID first
	if cachedInfo != nil && cachedInfo.PeerID != "" {
		logger.Debugw("attempting connection to cached peer", "peer", cachedInfo.PeerID)
		if err := s.connectToPeerID(cachedInfo.PeerID); err == nil {
			s.updateContactPeer(contactPubKey, cachedInfo.PeerID, cachedInfo.Multiaddrs, true)
			return &FindAndConnectResult{
				PeerID:    cachedInfo.PeerID,
				Addresses: cachedInfo.Multiaddrs,
				Source:    "cached",
			}, nil
		}
	}

	// Step 4: Try contact's stored PeerID
	if contact.PeerId != "" {
		logger.Debugw("attempting connection to contact's stored peer", "peer", contact.PeerId)
		if err := s.connectToPeerID(contact.PeerId); err == nil {
			s.updateContactPeer(contactPubKey, contact.PeerId, contact.Multiaddrs, true)
			return &FindAndConnectResult{
				PeerID:    contact.PeerId,
				Addresses: contact.Multiaddrs,
				Source:    "stored",
			}, nil
		}
	}

	// Step 5: Try contact's stored multiaddrs
	if len(contact.Multiaddrs) > 0 {
		for _, addr := range contact.Multiaddrs {
			logger.Debugw("attempting connection to contact's addr", "addr", addr)
			if err := s.ipfsNode.ConnectToPeer(addr); err == nil {
				s.updateContactPeer(contactPubKey, contact.PeerId, contact.Multiaddrs, true)
				return &FindAndConnectResult{
					PeerID:    contact.PeerId,
					Addresses: contact.Multiaddrs,
					Source:    "multiaddr",
				}, nil
			}
		}
	}

	// Step 6: Try DHT discovery (if we have PeerID)
	if contact.PeerId != "" {
		logger.Debugw("attempting DHT discovery for contact", "peer", contact.PeerId)
		peerInfo, err := s.ipfsNode.FindPeer(contact.PeerId)
		if err == nil && len(peerInfo.Addrs) > 0 {
			// Found peer via DHT
			addrStr := peerInfo.Addrs[0].String() + "/p2p/" + contact.PeerId
			if err := s.ipfsNode.ConnectToPeer(addrStr); err == nil {
				addrs := make([]string, len(peerInfo.Addrs))
				for i, addr := range peerInfo.Addrs {
					addrs[i] = addr.String()
				}
				s.updateContactPeer(contactPubKey, contact.PeerId, addrs, true)
				return &FindAndConnectResult{
					PeerID:    contact.PeerId,
					Addresses: addrs,
					Source:    "dht",
				}, nil
			}
		}
	}

	return nil, errors.New("failed to find and connect to contact - they may be offline")
}

// connectToPeerID attempts to connect to a peer by PeerID
func (s *Service) connectToPeerID(peerID string) error {
	if peerID == "" {
		return errors.New("empty peer ID")
	}

	// Check if already connected
	for _, p := range s.ipfsNode.Host().Network().Peers() {
		if p.String() == peerID {
			return nil
		}
	}

	// Try to find peer in DHT
	peerInfo, err := s.ipfsNode.FindPeer(peerID)
	if err != nil {
		return fmt.Errorf("DHT lookup failed: %w", err)
	}

	if len(peerInfo.Addrs) == 0 {
		return errors.New("no addresses found for peer")
	}

	// Connect to first address
	addr := peerInfo.Addrs[0].String() + "/p2p/" + peerID
	return s.ipfsNode.ConnectToPeer(addr)
}

// updateContactPeer updates the cached peer info for a contact and persists to storage
func (s *Service) updateContactPeer(contactPubKey []byte, peerID string, multiaddrs []string, connected bool) {
	s.contactMu.Lock()
	defer s.contactMu.Unlock()

	s.contactPeers[string(contactPubKey)] = &ContactPeerInfo{
		PeerID:     peerID,
		Multiaddrs: multiaddrs,
		LastSeen:   time.Now(),
		IsOnline:   connected,
		Connected:  connected,
	}

	// Persist peer ID and multiaddrs to contact storage
	s.persistContactPeer(contactPubKey, peerID, multiaddrs)
}

// persistContactPeer updates the contact record in storage with peer information
func (s *Service) persistContactPeer(contactPubKey []byte, peerID string, multiaddrs []string) {
	contact, err := s.storage.GetContact(contactPubKey)
	if err != nil {
		logger.Debugw("failed to get contact for peer update", "error", err)
		return
	}
	if contact == nil {
		return
	}

	// Update peer ID and multiaddrs if changed
	updated := false
	if peerID != "" && contact.PeerId != peerID {
		contact.PeerId = peerID
		updated = true
	}
	if len(multiaddrs) > 0 {
		// Only update if multiaddrs are different
		if !multiaddrsEqual(contact.Multiaddrs, multiaddrs) {
			contact.Multiaddrs = multiaddrs
			updated = true
		}
	}

	if updated {
		contact.LastSeen = uint64(time.Now().Unix())
		if err := s.storage.AddContact(contact); err != nil {
			logger.Debugw("failed to persist contact peer info", "error", err)
		} else {
			logger.Debugw("persisted contact peer info", "contact", hex.EncodeToString(contactPubKey), "peer", peerID)
		}
	}
}

// multiaddrsEqual compares two slices of multiaddresses
func multiaddrsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// IsContactOnline checks if a contact is currently reachable
func (s *Service) IsContactOnline(contactPubKey []byte) (bool, error) {
	s.mu.RLock()
	if !s.isStarted {
		s.mu.RUnlock()
		return false, ErrServiceNotStarted
	}
	s.mu.RUnlock()

	// Check cache first
	s.contactMu.RLock()
	info, cached := s.contactPeers[string(contactPubKey)]
	s.contactMu.RUnlock()

	if cached && info.Connected {
		return true, nil
	}

	// Try to find peer in DHT
	contact, err := s.storage.GetContact(contactPubKey)
	if err != nil {
		return false, fmt.Errorf("failed to get contact: %w", err)
	}
	if contact == nil {
		return false, ErrUnknownContact
	}

	if contact.PeerId == "" {
		return false, errors.New("contact has no peer ID")
	}

	// Query DHT
	peerInfo, err := s.ipfsNode.FindPeer(contact.PeerId)
	if err != nil {
		return false, nil // Not online, but not an error
	}

	// Update cache
	if len(peerInfo.Addrs) > 0 {
		addrs := make([]string, len(peerInfo.Addrs))
		for i, addr := range peerInfo.Addrs {
			addrs[i] = addr.String()
		}
		s.updateContactPeer(contactPubKey, contact.PeerId, addrs, true)
	}

	return true, nil
}

// GetContactPeerInfo returns cached peer information for a contact
func (s *Service) GetContactPeerInfo(contactPubKey []byte) (*ContactPeerInfo, bool) {
	s.contactMu.RLock()
	defer s.contactMu.RUnlock()

	info, ok := s.contactPeers[string(contactPubKey)]
	if !ok {
		return nil, false
	}

	// Return a copy
	infoCopy := *info
	return &infoCopy, true
}

// GetAllContactStats returns statistics for all contacts
func (s *Service) GetAllContactStats() map[string]*ContactPeerInfo {
	s.contactMu.RLock()
	defer s.contactMu.RUnlock()

	stats := make(map[string]*ContactPeerInfo, len(s.contactPeers))
	for k, v := range s.contactPeers {
		infoCopy := *v
		stats[k] = &infoCopy
	}
	return stats
}

// FindAndConnect attempts to find and connect to a peer by their public key
// It tries multiple discovery mechanisms in order:
// 1. Address book lookup (fastest, if previously connected)
// 2. DHT FindPeer query (requires bootstrap peers)
// 3. Manual connection (if multiaddr provided)
func (s *Service) FindAndConnect(contactPubKey []byte, addrBook interface{}, multiaddr string) (*FindAndConnectResult, error) {
	s.mu.RLock()
	if !s.isStarted {
		s.mu.RUnlock()
		return nil, ErrServiceNotStarted
	}
	s.mu.RUnlock()

	// Try address book first (if provided)
	// Note: addrBook is passed as interface{} to avoid circular import
	// In main.go, this will be type-asserted to *peerstore.AddrBook

	// Try DHT FindPeer
	// For this to work, we need the peer's PeerID
	// In a full implementation, PeerID would be stored with the contact
	// For now, we attempt DHT lookup assuming PeerID might match public key hash

	// If multiaddr is provided, try direct connection
	if multiaddr != "" {
		if err := s.ipfsNode.ConnectToPeer(multiaddr); err != nil {
			return nil, fmt.Errorf("failed to connect to peer: %w", err)
		}
		logger.Debugw("connected to peer via manual multiaddr", "multiaddr", multiaddr)
		return &FindAndConnectResult{
			Source:    "manual",
			Addresses: []string{multiaddr},
		}, nil
	}

	// DHT discovery would go here if we had PeerID
	// For now, return error indicating manual connection is needed
	return nil, errors.New("peer not found - provide multiaddr or add contact to address book")
}

// Message Retry Logic

// SendMessageWithRetry sends a message with retry logic (deprecated - use SendMessage directly)
// This method is kept for backward compatibility but simply calls SendMessage
func (s *Service) SendMessageWithRetry(
	text string,
	recipientEd25519PubKey []byte,
	recipientX25519PubKey []byte,
	maxAttempts int,
) (*SendResult, error) {
	return s.SendMessage(text, recipientEd25519PubKey, recipientX25519PubKey)
}

// SendResultWithRetry is deprecated - use SendResult instead
type SendResultWithRetry = SendResult

// PubSub Mesh Optimization

// OptimizePubSubMesh attempts to optimize the PubSub mesh for better message delivery
// It ensures we have good connectivity to peers subscribed to contact topics
func (s *Service) OptimizePubSubMesh(contactPubKey []byte) error {
	topic := ipfsnode.TopicFromPublicKey(contactPubKey)

	// Get topic info
	topicInfo := s.ipfsNode.GetTopicInfo(topic)
	if topicInfo == nil {
		return errors.New("topic not joined")
	}

	// Check if we have enough peers in the mesh
	if topicInfo.MeshSize < 3 {
		logger.Debugw("mesh size small, attempting to improve", "topic", topic, "size", topicInfo.MeshSize)

		// Try to find more peers via DHT
		// This is a best-effort operation
		bterrors.SafeGo("messaging-dht-refresh", func() {
			ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
			defer cancel()

			// Query DHT for peers
			if err := s.ipfsNode.RefreshDHT(ctx); err != nil {
				logger.Debugw("DHT refresh failed", "error", err)
			}
		})
	}

	logger.Debugw("pubsub mesh optimized", "topic", topic, "mesh_size", topicInfo.MeshSize)
	return nil
}

// GetTopicMeshSize returns the current mesh size for a topic
func (s *Service) GetTopicMeshSize(contactPubKey []byte) int {
	topic := ipfsnode.TopicFromPublicKey(contactPubKey)
	topicInfo := s.ipfsNode.GetTopicInfo(topic)
	if topicInfo == nil {
		return 0
	}
	return topicInfo.MeshSize
}

// ContactStatus represents the status of a contact
type ContactStatus struct {
	PubKey      []byte
	DisplayName string
	IsOnline    bool
	Connected   bool
	PeerID      string
	MeshSize    int
	IsActive    bool
}

// GetContactStatus returns detailed status for a contact
func (s *Service) GetContactStatus(contactPubKey []byte) (*ContactStatus, error) {
	contact, err := s.storage.GetContact(contactPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get contact: %w", err)
	}
	if contact == nil {
		return nil, ErrUnknownContact
	}

	var peerID string
	var isOnline, connected bool
	s.contactMu.RLock()
	if info, exists := s.contactPeers[string(contactPubKey)]; exists {
		peerID = info.PeerID
		isOnline = info.IsOnline
		connected = info.Connected
	}
	s.contactMu.RUnlock()

	meshSize := s.GetTopicMeshSize(contactPubKey)

	return &ContactStatus{
		PubKey:      contact.PublicKey,
		DisplayName: contact.DisplayName,
		IsOnline:    isOnline,
		Connected:   connected,
		PeerID:      peerID,
		MeshSize:    meshSize,
		IsActive:    connected,
	}, nil
}

// GetAllContactStatuses returns status for all contacts
func (s *Service) GetAllContactStatuses() ([]*ContactStatus, error) {
	contacts, err := s.storage.ListContacts()
	if err != nil {
		return nil, fmt.Errorf("failed to list contacts: %w", err)
	}

	statuses := make([]*ContactStatus, 0, len(contacts))
	for _, contact := range contacts {
		status, err := s.GetContactStatus(contact.PublicKey)
		if err != nil {
			logger.Debugw("failed to get contact status", "contact", hex.EncodeToString(contact.PublicKey), "error", err)
			continue
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// ReputationTracker returns the reputation tracker instance
func (s *Service) ReputationTracker() *reputation.Tracker {
	return s.reputationTracker
}

// SetReputationTracker sets the reputation tracker instance
func (s *Service) SetReputationTracker(tracker *reputation.Tracker) {
	s.reputationTracker = tracker
}

// GetMailboxManager returns the mailbox manager instance
func (s *Service) GetMailboxManager() *mailbox.Manager {
	return s.mailboxManager
}

// SetMailboxManager sets the mailbox manager for offline message delivery
func (s *Service) SetMailboxManager(m *mailbox.Manager) {
	s.mailboxManager = m
}
