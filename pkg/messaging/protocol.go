package messaging

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"sync"
	"time"

	"babylontower/pkg/ipfsnode"
	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"
	"github.com/ipfs/go-log/v2"
	"google.golang.org/protobuf/proto"
)

var logger = log.Logger("babylontower/messaging")

var (
	// ErrServiceNotStarted is returned when operations are attempted on a stopped service
	ErrServiceNotStarted = errors.New("messaging service not started")
	// ErrUnknownContact is returned when trying to message an unknown contact
	ErrUnknownContact = errors.New("unknown contact")
	// ErrSelfMessage is returned when trying to send a message to oneself
	ErrSelfMessage = errors.New("cannot send message to self")
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
}

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
	config      *Config
	storage     storage.Storage
	ipfsNode    *ipfsnode.Node
	subscription *ipfsnode.Subscription

	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	isStarted  bool

	// Channel for incoming message events
	messageChan chan *MessageEvent

	mu sync.RWMutex

	// Contact peer tracking (cached peer IDs for quick lookup)
	contactPeers map[string]*ContactPeerInfo // key: hex-encoded Ed25519 pubkey
	contactMu    sync.RWMutex

	// Connection pooling for active contacts
	connectionPool *ConnectionPool
	poolConfig     *ConnectionPoolConfig
}

// ContactPeerInfo contains cached information about a contact's peer presence
type ContactPeerInfo struct {
	PeerID        string
	Multiaddrs    []string
	LastSeen      time.Time
	IsOnline      bool
	Connected     bool
}

// ConnectionPoolConfig holds configuration for connection pooling
type ConnectionPoolConfig struct {
	// MaxActiveContacts is the maximum number of contacts to maintain connections to
	MaxActiveContacts int
	// KeepAliveInterval is how often to check connection health
	KeepAliveInterval time.Duration
	// MinConnectionDuration is the minimum time to keep a connection alive
	MinConnectionDuration time.Duration
}

// DefaultConnectionPoolConfig returns default connection pool configuration
func DefaultConnectionPoolConfig() *ConnectionPoolConfig {
	return &ConnectionPoolConfig{
		MaxActiveContacts:     10,
		KeepAliveInterval:     30 * time.Second,
		MinConnectionDuration: 5 * time.Minute,
	}
}

// ConnectionPool manages connections to active contacts
type ConnectionPool struct {
	config       *ConnectionPoolConfig
	activeContacts map[string]*ActiveContact // key: hex-encoded Ed25519 pubkey
	mu           sync.RWMutex
	lastCleanup  time.Time
}

// ActiveContact represents a contact with an active connection
type ActiveContact struct {
	PubKey       []byte
	LastActivity time.Time
	MessageCount int
	PeerID       string
}

// NewService creates a new messaging service
func NewService(config *Config, storage storage.Storage, ipfsNode *ipfsnode.Node) *Service {
	ctx, cancel := context.WithCancel(context.Background())

	poolConfig := DefaultConnectionPoolConfig()

	return &Service{
		config:      config,
		storage:     storage,
		ipfsNode:    ipfsNode,
		ctx:         ctx,
		cancel:      cancel,
		isStarted:   false,
		messageChan: make(chan *MessageEvent, 100),
		contactPeers: make(map[string]*ContactPeerInfo),
		connectionPool: &ConnectionPool{
			config:         poolConfig,
			activeContacts: make(map[string]*ActiveContact),
			lastCleanup:    time.Now(),
		},
		poolConfig: poolConfig,
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
		return fmt.Errorf("IPFS node not started")
	}

	if s.storage == nil {
		return fmt.Errorf("storage not initialized")
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

	logger.Infow("messaging service started", "topic", topic)

	return nil
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

// handlePubSubMessage processes an incoming PubSub message
func (s *Service) handlePubSubMessage(pubsubMsg *ipfsnode.Message) {
	// The PubSub message contains the encrypted envelope directly
	// (For PoC, we send envelopes directly instead of CIDs)
	logger.Debugw("received envelope via PubSub", "size", len(pubsubMsg.Data), "from", pubsubMsg.From)

	// Process the envelope directly
	if err := s.processEnvelope(pubsubMsg.Data); err != nil {
		logger.Errorw("failed to process envelope", "error", err, "from", pubsubMsg.From)
		return
	}
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
		logger.Warnw("message from unknown contact", "sender", fmt.Sprintf("%x", senderPubKey))
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
		logger.Infow("message event emitted", "from", fmt.Sprintf("%x", senderPubKey), "text", msg.Text)
	default:
		logger.Warnw("message channel full, dropping event")
	}

	return nil
}

// Messages returns the channel for receiving message events
func (s *Service) Messages() <-chan *MessageEvent {
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
	return nil, fmt.Errorf("contact X25519 key lookup not implemented - caller must provide key")
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
		logger.Debugw("already connected to contact", "contact", fmt.Sprintf("%x", contactPubKey))
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

	return nil, fmt.Errorf("failed to find and connect to contact - they may be offline")
}

// connectToPeerID attempts to connect to a peer by PeerID
func (s *Service) connectToPeerID(peerID string) error {
	if peerID == "" {
		return fmt.Errorf("empty peer ID")
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
		return fmt.Errorf("no addresses found for peer")
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
			logger.Debugw("persisted contact peer info", "contact", fmt.Sprintf("%x", contactPubKey), "peer", peerID)
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
		return false, fmt.Errorf("contact has no peer ID")
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

// GetAllContactStats returns statistics about all tracked contacts
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
		logger.Infow("Connected to peer via manual multiaddr", "multiaddr", multiaddr)
		return &FindAndConnectResult{
			Source:    "manual",
			Addresses: []string{multiaddr},
		}, nil
	}

	// DHT discovery would go here if we had PeerID
	// For now, return error indicating manual connection is needed
	return nil, fmt.Errorf("peer not found - provide multiaddr or add contact to address book")
}

// Connection Pooling Methods

// AddActiveContact adds a contact to the active connection pool
func (s *Service) AddActiveContact(contactPubKey []byte) {
	s.connectionPool.mu.Lock()
	defer s.connectionPool.mu.Unlock()

	key := string(contactPubKey)
	if _, exists := s.connectionPool.activeContacts[key]; exists {
		// Update last activity
		s.connectionPool.activeContacts[key].LastActivity = time.Now()
		return
	}

	// Check if we're at capacity
	if len(s.connectionPool.activeContacts) >= s.poolConfig.MaxActiveContacts {
		// Remove least active contact
		s.removeLeastActiveContact()
	}

	// Get peer info if available
	var peerID string
	if info, ok := s.contactPeers[key]; ok && info.PeerID != "" {
		peerID = info.PeerID
	}

	s.connectionPool.activeContacts[key] = &ActiveContact{
		PubKey:       contactPubKey,
		LastActivity: time.Now(),
		MessageCount: 0,
		PeerID:       peerID,
	}

	logger.Debugw("added contact to connection pool", "contact", fmt.Sprintf("%x", contactPubKey))
}

// RemoveActiveContact removes a contact from the active connection pool
func (s *Service) RemoveActiveContact(contactPubKey []byte) {
	s.connectionPool.mu.Lock()
	defer s.connectionPool.mu.Unlock()

	key := string(contactPubKey)
	delete(s.connectionPool.activeContacts, key)
	logger.Debugw("removed contact from connection pool", "contact", fmt.Sprintf("%x", contactPubKey))
}

// UpdateContactActivity updates the last activity time for a contact
func (s *Service) UpdateContactActivity(contactPubKey []byte) {
	s.connectionPool.mu.Lock()
	defer s.connectionPool.mu.Unlock()

	if contact, exists := s.connectionPool.activeContacts[string(contactPubKey)]; exists {
		contact.LastActivity = time.Now()
		contact.MessageCount++
	}
}

// removeLeastActiveContact removes the least active contact from the pool
// Must be called with connectionPool.mu held
func (s *Service) removeLeastActiveContact() {
	var leastActive string
	var oldestActivity time.Time

	for key, contact := range s.connectionPool.activeContacts {
		if leastActive == "" || contact.LastActivity.Before(oldestActivity) {
			leastActive = key
			oldestActivity = contact.LastActivity
		}
	}

	if leastActive != "" {
		delete(s.connectionPool.activeContacts, leastActive)
		logger.Debugw("removed least active contact from pool", "contact", leastActive)
	}
}

// GetActiveContacts returns the list of active contacts
func (s *Service) GetActiveContacts() [][]byte {
	s.connectionPool.mu.RLock()
	defer s.connectionPool.mu.RUnlock()

	contacts := make([][]byte, 0, len(s.connectionPool.activeContacts))
	for _, contact := range s.connectionPool.activeContacts {
		contacts = append(contacts, contact.PubKey)
	}
	return contacts
}

// GetConnectionPoolStats returns statistics about the connection pool
func (s *Service) GetConnectionPoolStats() map[string]interface{} {
	s.connectionPool.mu.RLock()
	defer s.connectionPool.mu.RUnlock()

	stats := map[string]interface{}{
		"active_contacts": len(s.connectionPool.activeContacts),
		"max_contacts":    s.poolConfig.MaxActiveContacts,
	}

	contacts := make([]map[string]interface{}, 0, len(s.connectionPool.activeContacts))
	for _, contact := range s.connectionPool.activeContacts {
		contacts = append(contacts, map[string]interface{}{
			"pubkey":        fmt.Sprintf("%x", contact.PubKey),
			"last_activity": contact.LastActivity.Format(time.RFC3339),
			"message_count": contact.MessageCount,
			"peer_id":       contact.PeerID,
		})
	}
	stats["contacts"] = contacts

	return stats
}

// Message Retry Logic

// SendResultWithRetry contains the result of a message send with retry information
type SendResultWithRetry struct {
	*SendResult
	// Attempts is the number of attempts made
	Attempts int
	// Success is true if the message was sent successfully
	Success bool
	// LastError is the last error encountered
	LastError error
}

// SendMessageWithRetry sends a message with retry logic
// It attempts to send via different peers on failure
func (s *Service) SendMessageWithRetry(
	text string,
	recipientEd25519PubKey []byte,
	recipientX25519PubKey []byte,
	maxAttempts int,
) (*SendResultWithRetry, error) {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	var lastResult *SendResultWithRetry
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Try to connect before sending
		if attempt > 1 {
			logger.Infow("retrying message send", "attempt", attempt, "to", fmt.Sprintf("%x", recipientEd25519PubKey))
			
			// Try different strategies on retry
			switch attempt {
			case 2:
				// Try DHT discovery
				if _, err := s.FindAndConnectToContact(recipientEd25519PubKey); err != nil {
					logger.Debugw("retry via DHT failed", "error", err)
				}
			case 3:
				// Try refreshing connection
				s.refreshConnection(recipientEd25519PubKey)
			}
		}

		// Attempt to send
		result, err := s.SendMessage(text, recipientEd25519PubKey, recipientX25519PubKey)
		
		if err == nil {
			// Success!
			return &SendResultWithRetry{
				SendResult: result,
				Attempts:   attempt,
				Success:    true,
			}, nil
		}

		lastErr = err
		lastResult = &SendResultWithRetry{
			Attempts:  attempt,
			Success:   false,
			LastError: err,
		}

		// Wait before retry (exponential backoff)
		if attempt < maxAttempts {
			backoff := time.Duration(attempt*500) * time.Millisecond
			select {
			case <-s.ctx.Done():
				return lastResult, s.ctx.Err()
			case <-time.After(backoff):
				// Continue to next attempt
			}
		}
	}

	return lastResult, fmt.Errorf("failed to send message after %d attempts: %w", maxAttempts, lastErr)
}

// refreshConnection attempts to refresh a connection to a peer
func (s *Service) refreshConnection(contactPubKey []byte) {
	s.contactMu.Lock()
	if info, exists := s.contactPeers[string(contactPubKey)]; exists {
		info.Connected = false
	}
	s.contactMu.Unlock()

	// Try to reconnect
	go func() {
		if _, err := s.FindAndConnectToContact(contactPubKey); err != nil {
			logger.Debugw("connection refresh failed", "error", err)
		}
	}()
}

// PubSub Mesh Optimization

// OptimizePubSubMesh attempts to optimize the PubSub mesh for better message delivery
// It ensures we have good connectivity to peers subscribed to contact topics
func (s *Service) OptimizePubSubMesh(contactPubKey []byte) error {
	topic := ipfsnode.TopicFromPublicKey(contactPubKey)
	
	// Get topic info
	topicInfo := s.ipfsNode.GetTopicInfo(topic)
	if topicInfo == nil {
		return fmt.Errorf("topic not joined")
	}

	// Check if we have enough peers in the mesh
	if topicInfo.MeshSize < 3 {
		logger.Debugw("mesh size small, attempting to improve", "topic", topic, "size", topicInfo.MeshSize)
		
		// Try to find more peers via DHT
		// This is a best-effort operation
		go func() {
			ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
			defer cancel()
			
			// Query DHT for peers
			if err := s.ipfsNode.RefreshDHT(ctx); err != nil {
				logger.Debugw("DHT refresh failed", "error", err)
			}
		}()
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
	// Get contact info
	contact, err := s.storage.GetContact(contactPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get contact: %w", err)
	}
	if contact == nil {
		return nil, ErrUnknownContact
	}

	// Get cached peer info
	var peerID string
	var isOnline, connected bool
	s.contactMu.RLock()
	if info, exists := s.contactPeers[string(contactPubKey)]; exists {
		peerID = info.PeerID
		isOnline = info.IsOnline
		connected = info.Connected
	}
	s.contactMu.RUnlock()

	// Check if active
	s.connectionPool.mu.RLock()
	isActive := false
	if _, exists := s.connectionPool.activeContacts[string(contactPubKey)]; exists {
		isActive = true
	}
	s.connectionPool.mu.RUnlock()

	// Get mesh size
	meshSize := s.GetTopicMeshSize(contactPubKey)

	return &ContactStatus{
		PubKey:      contact.PublicKey,
		DisplayName: contact.DisplayName,
		IsOnline:    isOnline,
		Connected:   connected,
		PeerID:      peerID,
		MeshSize:    meshSize,
		IsActive:    isActive,
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
			logger.Debugw("failed to get contact status", "contact", fmt.Sprintf("%x", contact.PublicKey), "error", err)
			continue
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}
