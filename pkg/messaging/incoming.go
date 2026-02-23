package messaging

import (
	"crypto/ed25519"
	"fmt"

	"babylontower/pkg/crypto"
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

// MessageWithMeta contains a decrypted message with metadata
type MessageWithMeta struct {
	Message    *pb.Message
	IsOutgoing bool
	Timestamp  uint64
}

// GetDecryptedMessagesWithMeta retrieves and decrypts message history with direction metadata
// It handles both incoming messages (encrypted with our X25519 public key)
// and outgoing messages (encrypted with contact's X25519 public key)
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
		env := mwk.Envelope

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

		// Determine if this is an outgoing message (we sent it)
		// Compare sender's Ed25519 public key with our own
		isOutgoing := string(env.SenderPubkey) == string(s.config.OwnEd25519PubKey)
		
		// Debug logging for sender detection
		logger.Debugw("message direction check",
			"sender_pub", fmt.Sprintf("%x", env.SenderPubkey[:8]),
			"own_pub", fmt.Sprintf("%x", s.config.OwnEd25519PubKey[:8]),
			"isOutgoing", isOutgoing,
			"timestamp", mwk.Timestamp)

		var msg *pb.Message
		var timestamp uint64

		if isOutgoing {
			// For outgoing messages, try to decrypt using stored encrypted ephemeral key
			// The ephemeral key is encrypted with our X25519 key for local storage only
			ephemeralPriv, err := decryptEphemeralKeyFromEnvelope(
				env,
				s.config.OwnX25519PrivKey,
				contactPubKey,
			)
			if err != nil {
				logger.Warnw("failed to decrypt ephemeral key", "error", err)
			}

			if ephemeralPriv != nil {
				// Successfully decrypted ephemeral key - now decrypt the message
				// Compute shared secret: X25519(ephemeral_priv, recipient_static_pub)
				// We need the recipient's X25519 public key
				recipientX25519PubKey, err := s.storage.GetContactX25519Key(contactPubKey)
				if err != nil {
					logger.Warnw("failed to get contact X25519 key", "error", err)
				} else {
					sharedSecret, err := crypto.ComputeSharedSecret(ephemeralPriv, recipientX25519PubKey)
					if err != nil {
						logger.Warnw("failed to compute shared secret", "error", err)
					} else {
						// Derive encryption key from shared secret
						encryptionKey, err := crypto.DeriveKey(sharedSecret, nil, []byte("encryption"), crypto.KeySize)
						if err != nil {
							logger.Warnw("failed to derive encryption key", "error", err)
						} else {
							// Decrypt with stored key
							plaintext, err := crypto.Decrypt(encryptionKey, envelope.Nonce, envelope.Ciphertext)
							if err != nil {
								logger.Warnw("failed to decrypt message with ephemeral key", "error", err)
							} else {
								// Parse message
								msg, err = ParseMessage(plaintext)
								if err != nil {
									logger.Warnw("failed to parse message", "error", err)
								}
							}
						}
					}
				}
			}

			// If we couldn't decrypt, use placeholder
			if msg == nil {
				timestamp = mwk.Timestamp
				msg = &pb.Message{
					Text:      "[Sent message]",
					Timestamp: timestamp,
				}
			} else {
				// Use message timestamp if available
				if msg.Timestamp > 0 {
					timestamp = msg.Timestamp
				} else {
					timestamp = mwk.Timestamp
				}
			}
		} else {
			// For incoming messages, decrypt with our X25519 private key
			plaintext, err := DecryptEnvelope(envelope, s.config.OwnX25519PrivKey)
			if err != nil {
				logger.Warnw("failed to decrypt envelope", "error", err)
				continue
			}

			// Parse message
			msg, err = ParseMessage(plaintext)
			if err != nil {
				logger.Warnw("failed to parse message", "error", err)
				continue
			}
			// Use message timestamp if available, otherwise use storage timestamp
			if msg.Timestamp > 0 {
				timestamp = msg.Timestamp
			} else {
				timestamp = mwk.Timestamp
			}
		}

		messages = append(messages, &MessageWithMeta{
			Message:    msg,
			IsOutgoing: isOutgoing,
			Timestamp:  timestamp,
		})
	}

	return messages, nil
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
