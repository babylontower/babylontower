package messaging

import (
	"crypto/ed25519"
	"fmt"

	pb "babylontower/pkg/proto"
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

	logger.Infow("message received", "from", fmt.Sprintf("%x", senderPubKey), "text", msg.Text)

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

	logger.Infow("message received (direct)", "from", fmt.Sprintf("%x", senderPubKey), "text", msg.Text)

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

// GetDecryptedMessages retrieves and decrypts message history for a contact
func (s *Service) GetDecryptedMessages(contactPubKey []byte, limit, offset int) ([]*pb.Message, error) {
	envelopes, err := s.GetMessages(contactPubKey, limit, offset)
	if err != nil {
		return nil, err
	}

	messages := make([]*pb.Message, 0, len(envelopes))
	for _, env := range envelopes {
		// Verify signature
		valid, err := VerifyEnvelope(env)
		if err != nil || !valid {
			logger.Warnw("invalid envelope in history", "error", err)
			continue
		}

		// Parse envelope
		envelope, err := ParseEnvelope(env)
		if err != nil {
			logger.Warnw("failed to parse envelope", "error", err)
			continue
		}

		// Decrypt envelope
		plaintext, err := DecryptEnvelope(envelope, s.config.OwnX25519PrivKey)
		if err != nil {
			logger.Warnw("failed to decrypt envelope", "error", err)
			continue
		}

		// Parse message
		msg, err := ParseMessage(plaintext)
		if err != nil {
			logger.Warnw("failed to parse message", "error", err)
			continue
		}

		messages = append(messages, msg)
	}

	return messages, nil
}
