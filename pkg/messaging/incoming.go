package messaging

import (
	"crypto/ed25519"
	"fmt"

	"babylontower/pkg/crypto"
	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"
)

// ReceiveResult contains the result of processing a received message
type ReceiveResult struct {
	// Message is the decrypted plaintext message
	Message *pb.Message
	// SenderPubKey is the sender's Ed25519 public key
	SenderPubKey ed25519.PublicKey
	// Envelope is the original signed envelope
	Envelope *pb.SignedEnvelope
}

// ReceiveMessage processes a raw envelope received from IPFS
// This is the complete incoming message flow:
// 1. Parse signed envelope
// 2. Verify signature
// 3. Decrypt envelope
// 4. Parse message
// 5. Store message locally
// 6. Return decrypted message
func (s *Service) ReceiveMessage(envelopeBytes []byte) (*ReceiveResult, error) {
	s.mu.RLock()
	if !s.isStarted {
		s.mu.RUnlock()
		return nil, ErrServiceNotStarted
	}
	s.mu.RUnlock()

	// Parse signed envelope
	signedEnvelope, err := ParseSignedEnvelope(envelopeBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse signed envelope: %w", err)
	}

	// Verify signature
	valid, err := VerifyEnvelope(signedEnvelope)
	if err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}
	if !valid {
		return nil, ErrInvalidSignature
	}

	// Parse envelope
	envelope, err := ParseEnvelope(signedEnvelope)
	if err != nil {
		return nil, fmt.Errorf("failed to parse envelope: %w", err)
	}

	// Decrypt envelope
	plaintext, err := DecryptEnvelope(envelope, s.config.OwnX25519PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt envelope: %w", err)
	}

	// Parse message
	msg, err := ParseMessage(plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	// Get sender's public key
	senderPubKey := ed25519.PublicKey(signedEnvelope.SenderPubkey)

	// Store message locally
	if err := s.storage.AddMessage(senderPubKey, signedEnvelope); err != nil {
		logger.Warnw("failed to store received message", "error", err)
		// Don't fail the receive if storage fails
	}

	logger.Debugw("message received", "from", fmt.Sprintf("%x", senderPubKey))

	return &ReceiveResult{
		Message:      msg,
		SenderPubKey: senderPubKey,
		Envelope:     signedEnvelope,
	}, nil
}

// ReceiveMessageDirect processes a signed envelope directly (bypassing IPFS fetch)
// This is useful for testing or when the envelope is already available
func (s *Service) ReceiveMessageDirect(signedEnvelope *pb.SignedEnvelope) (*ReceiveResult, error) {
	s.mu.RLock()
	if !s.isStarted {
		s.mu.RUnlock()
		return nil, ErrServiceNotStarted
	}
	s.mu.RUnlock()

	// Verify signature
	valid, err := VerifyEnvelope(signedEnvelope)
	if err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}
	if !valid {
		return nil, ErrInvalidSignature
	}

	// Parse envelope
	envelope, err := ParseEnvelope(signedEnvelope)
	if err != nil {
		return nil, fmt.Errorf("failed to parse envelope: %w", err)
	}

	// Decrypt envelope
	plaintext, err := DecryptEnvelope(envelope, s.config.OwnX25519PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt envelope: %w", err)
	}

	// Parse message
	msg, err := ParseMessage(plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	// Get sender's public key
	senderPubKey := ed25519.PublicKey(signedEnvelope.SenderPubkey)

	// Store message locally
	if err := s.storage.AddMessage(senderPubKey, signedEnvelope); err != nil {
		logger.Warnw("failed to store received message", "error", err)
	}

	logger.Debugw("message received (direct)", "from", fmt.Sprintf("%x", senderPubKey))

	return &ReceiveResult{
		Message:      msg,
		SenderPubKey: senderPubKey,
		Envelope:     signedEnvelope,
	}, nil
}

// FetchAndReceiveMessage fetches an envelope from IPFS by CID and processes it
// This is the full flow for handling a CID received via PubSub
// Note: In PoC, IPFS Get is not fully implemented
func (s *Service) FetchAndReceiveMessage(cidStr string) (*ReceiveResult, error) {
	s.mu.RLock()
	if !s.isStarted {
		s.mu.RUnlock()
		return nil, ErrServiceNotStarted
	}
	s.mu.RUnlock()

	// Fetch envelope from IPFS
	envelopeBytes, err := s.ipfsNode.Get(cidStr)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch envelope from IPFS: %w", err)
	}

	// Process the envelope
	return s.ReceiveMessage(envelopeBytes)
}

// GetMessages retrieves message history for a contact
func (s *Service) GetMessages(contactPubKey []byte, limit, offset int) ([]*pb.SignedEnvelope, error) {
	s.mu.RLock()
	if !s.isStarted {
		s.mu.RUnlock()
		return nil, ErrServiceNotStarted
	}
	s.mu.RUnlock()

	return s.storage.GetMessages(contactPubKey, limit, offset)
}

// MessageWithMeta contains a decrypted message with metadata
type MessageWithMeta struct {
	Message    *pb.Message
	IsOutgoing bool
	Timestamp  uint64
}

// GetDecryptedMessagesWithMeta retrieves and decrypts message history with direction metadata
func (s *Service) GetDecryptedMessagesWithMeta(contactPubKey []byte, limit, offset int) ([]*MessageWithMeta, error) {
	s.mu.RLock()
	if !s.isStarted {
		s.mu.RUnlock()
		return nil, ErrServiceNotStarted
	}
	s.mu.RUnlock()

	msgsWithKeys, err := s.storage.GetMessagesWithTimestamps(contactPubKey, limit, offset)
	if err != nil {
		return nil, err
	}

	messages := make([]*MessageWithMeta, 0, len(msgsWithKeys))
	for _, mwk := range msgsWithKeys {
		msgMeta, err := s.decryptMessageWithMeta(mwk, contactPubKey)
		if err != nil {
			logger.Debugw("failed to decrypt message", "error", err)
			continue
		}
		if msgMeta != nil {
			messages = append(messages, msgMeta)
		}
	}

	return messages, nil
}

// decryptMessageWithMeta decrypts a single message and returns it with metadata
func (s *Service) decryptMessageWithMeta(mwk *storage.MessageWithKey, contactPubKey []byte) (*MessageWithMeta, error) {
	signedEnv := mwk.Envelope

	valid, err := VerifyEnvelope(signedEnv)
	if err != nil || !valid {
		logger.Warnw("invalid envelope in history", "error", err)
		return nil, nil
	}

	envelope, err := ParseEnvelope(signedEnv)
	if err != nil {
		logger.Warnw("failed to parse envelope", "error", err)
		return nil, nil
	}

	isOutgoing := s.isOutgoingMessage(signedEnv.SenderPubkey)
	msg, timestamp := s.decryptMessageContent(signedEnv, envelope, mwk.Timestamp, isOutgoing, contactPubKey)

	return &MessageWithMeta{
		Message:    msg,
		IsOutgoing: isOutgoing,
		Timestamp:  timestamp,
	}, nil
}

// isOutgoingMessage determines if a message was sent by us
func (s *Service) isOutgoingMessage(senderPubKey []byte) bool {
	if len(senderPubKey) != len(s.config.OwnEd25519PubKey) {
		return false
	}
	return string(senderPubKey) == string(s.config.OwnEd25519PubKey)
}

// decryptMessageContent decrypts the message content based on direction
func (s *Service) decryptMessageContent(signedEnv *pb.SignedEnvelope, envelope *pb.Envelope, storageTimestamp uint64, isOutgoing bool, contactPubKey []byte) (*pb.Message, uint64) {
	if isOutgoing {
		return s.decryptOutgoingMessage(signedEnv, envelope, storageTimestamp, contactPubKey)
	}
	return s.decryptIncomingMessage(envelope, storageTimestamp)
}

// decryptOutgoingMessage decrypts a message we sent (using stored ephemeral key)
func (s *Service) decryptOutgoingMessage(signedEnv *pb.SignedEnvelope, envelope *pb.Envelope, storageTimestamp uint64, contactPubKey []byte) (*pb.Message, uint64) {
	recipientX25519PubKey, err := s.storage.GetContactX25519Key(contactPubKey)
	if err != nil {
		logger.Warnw("failed to get contact X25519 key for decryption", "error", err)
		return s.placeholderMessage(storageTimestamp, "[Sent message]"), storageTimestamp
	}

	ephemeralPriv, err := decryptEphemeralKeyFromEnvelope(signedEnv, s.config.OwnX25519PrivKey, recipientX25519PubKey)
	if err != nil || ephemeralPriv == nil {
		logger.Warnw("failed to decrypt ephemeral key", "error", err)
		return s.placeholderMessage(storageTimestamp, "[Sent message]"), storageTimestamp
	}

	msg, err := s.decryptWithEphemeralKey(envelope, ephemeralPriv, recipientX25519PubKey)
	if err != nil {
		return s.placeholderMessage(storageTimestamp, "[Sent message]"), storageTimestamp
	}

	timestamp := msg.Timestamp
	if timestamp == 0 {
		timestamp = storageTimestamp
	}
	return msg, timestamp
}

// decryptWithEphemeralKey decrypts a message using the ephemeral private key
func (s *Service) decryptWithEphemeralKey(envelope *pb.Envelope, ephemeralPriv, recipientX25519PubKey []byte) (*pb.Message, error) {
	sharedSecret, err := crypto.ComputeSharedSecret(ephemeralPriv, recipientX25519PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to compute shared secret: %w", err)
	}

	encryptionKey, err := crypto.DeriveKey(sharedSecret, nil, []byte("encryption"), crypto.KeySize)
	if err != nil {
		return nil, fmt.Errorf("failed to derive encryption key: %w", err)
	}

	plaintext, err := crypto.Decrypt(encryptionKey, envelope.Nonce, envelope.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return ParseMessage(plaintext)
}

// decryptIncomingMessage decrypts a message we received
func (s *Service) decryptIncomingMessage(envelope *pb.Envelope, storageTimestamp uint64) (*pb.Message, uint64) {
	plaintext, err := DecryptEnvelope(envelope, s.config.OwnX25519PrivKey)
	if err != nil {
		logger.Warnw("failed to decrypt envelope", "error", err)
		return s.placeholderMessage(storageTimestamp, "[Undecryptable message]"), storageTimestamp
	}

	msg, err := ParseMessage(plaintext)
	if err != nil {
		logger.Warnw("failed to parse message", "error", err)
		return s.placeholderMessage(storageTimestamp, "[Corrupted message]"), storageTimestamp
	}

	timestamp := msg.Timestamp
	if timestamp == 0 {
		timestamp = storageTimestamp
	}
	return msg, timestamp
}

// placeholderMessage creates a placeholder message when decryption fails
func (s *Service) placeholderMessage(timestamp uint64, text string) *pb.Message {
	return &pb.Message{
		Text:      text,
		Timestamp: timestamp,
	}
}

// GetDecryptedMessages retrieves and decrypts message history for a contact
// Deprecated: Use GetDecryptedMessagesWithMeta for proper direction handling
func (s *Service) GetDecryptedMessages(contactPubKey []byte, limit, offset int) ([]*pb.Message, error) {
	meta, err := s.GetDecryptedMessagesWithMeta(contactPubKey, limit, offset)
	if err != nil {
		return nil, err
	}

	messages := make([]*pb.Message, 0, len(meta))
	for _, m := range meta {
		messages = append(messages, m.Message)
	}
	return messages, nil
}
