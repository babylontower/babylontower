package messaging

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"sync"

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
}

// NewService creates a new messaging service
func NewService(config *Config, storage storage.Storage, ipfsNode *ipfsnode.Node) *Service {
	ctx, cancel := context.WithCancel(context.Background())

	return &Service{
		config:      config,
		storage:     storage,
		ipfsNode:    ipfsNode,
		ctx:         ctx,
		cancel:      cancel,
		isStarted:   false,
		messageChan: make(chan *MessageEvent, 100),
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
	// The PubSub message contains a CID string
	cidStr := string(pubsubMsg.Data)
	logger.Debugw("received CID via PubSub", "cid", cidStr, "from", pubsubMsg.From)

	// Fetch the SignedEnvelope from IPFS
	// Note: In PoC, direct block fetch is not implemented
	// For now, we log the limitation
	logger.Warnw("IPFS Get not implemented in PoC - message cannot be fetched", "cid", cidStr)

	// In a full implementation:
	// envelope, err := s.ipfsNode.Get(cidStr)
	// if err != nil {
	//     logger.Warnw("failed to fetch envelope", "cid", cidStr, "error", err)
	//     return
	// }
	// s.processEnvelope(envelope)
}

// processEnvelope verifies, decrypts, and stores an incoming envelope
// This is called internally when a PubSub message is received
// nolint:unused
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
		logger.Debugw("message event emitted", "from", fmt.Sprintf("%x", senderPubKey))
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
