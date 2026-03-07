package messaging

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	pb "babylontower/pkg/proto"

	"google.golang.org/protobuf/proto"
)

// SendResult contains the result of a sent message
type SendResult struct {
	// SignedEnvelope is the signed envelope that was sent
	SignedEnvelope *pb.SignedEnvelope
	// CID is the IPFS content identifier
	CID string
	// Message is the original plaintext message
	Message *pb.Message
}

// SendMessage sends a message to a contact
// This is the complete outgoing message flow:
// 1. Build message
// 2. Encrypt with recipient's X25519 public key
// 3. Sign with sender's Ed25519 private key
// 4. Attempt to connect to recipient's peer (contact-aware routing)
// 5. Publish signed envelope via PubSub to recipient's topic
// 6. Store message locally
//
// Note: For PoC, we send the envelope directly via PubSub instead of storing in IPFS
// and sending CID. This avoids the IPFS Get limitation in the PoC.
func (s *Service) SendMessage(
	text string,
	recipientEd25519PubKey []byte,
	recipientX25519PubKey []byte,
) (*SendResult, error) {
	s.mu.RLock()
	if !s.isStarted {
		s.mu.RUnlock()
		return nil, ErrServiceNotStarted
	}
	s.mu.RUnlock()

	// Validate inputs
	if len(recipientEd25519PubKey) != ed25519.PublicKeySize {
		return nil, errors.New("invalid recipient Ed25519 public key length")
	}
	if len(recipientX25519PubKey) != 32 {
		return nil, errors.New("invalid recipient X25519 public key length")
	}

	// Check not sending to self
	if string(recipientEd25519PubKey) == string(s.config.OwnEd25519PubKey) {
		return nil, ErrSelfMessage
	}

	// Build plaintext message
	msg := BuildMessageNow(text)
	plaintext, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// Build encrypted envelope (includes ephemeral key pair generation)
	envelope, ephemeralPriv, err := BuildEnvelope(plaintext, recipientX25519PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to build envelope: %w", err)
	}

	// Sign envelope
	signedEnvelope, err := SignEnvelope(envelope, s.config.OwnEd25519PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign envelope: %w", err)
	}

	// Encrypt ephemeral private key for local storage only
	// This allows the sender to decrypt their own messages from history
	// IMPORTANT: This is NOT sent to the recipient - only stored locally
	encryptedEphemeral, err := encryptEphemeralKey(
		ephemeralPriv,
		s.config.OwnX25519PrivKey,
		recipientX25519PubKey,
	)
	if err != nil {
		logger.Warnw("failed to encrypt ephemeral key", "error", err)
		// Continue without storing encrypted key - message will still be sent
		// but sender won't be able to decrypt from history
	}

	// Create a copy of signed envelope with encrypted ephemeral key for local storage
	signedEnvelopeForStorage := &pb.SignedEnvelope{
		Envelope:               signedEnvelope.Envelope,
		Signature:              signedEnvelope.Signature,
		SenderPubkey:           signedEnvelope.SenderPubkey,
		EncryptedEphemeralPriv: encryptedEphemeral,
	}

	// Serialize signed envelope for transmission (WITHOUT encrypted ephemeral key)
	envelopeBytes, err := proto.Marshal(signedEnvelope)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signed envelope: %w", err)
	}

	// Contact-aware routing: Try to connect to recipient BEFORE sending
	// This uses DHT to discover the peer's address if not already connected
	logger.Infow("attempting DHT peer discovery for contact",
		"to", hex.EncodeToString(recipientEd25519PubKey)[:16])

	// Try to connect via DHT (blocking, with internal timeout)
	connectErr := s.connectToPeerID(string(recipientEd25519PubKey))
	if connectErr == nil {
		logger.Infow("DHT peer discovery succeeded",
			"to", hex.EncodeToString(recipientEd25519PubKey)[:16])
	} else {
		logger.Debugw("DHT peer discovery failed (peer may be offline)",
			"to", hex.EncodeToString(recipientEd25519PubKey)[:16],
			"error", connectErr)
	}

	// Optimize pubsub mesh for better delivery
	if err := s.OptimizePubSubMesh(recipientEd25519PubKey); err != nil {
		logger.Debugw("pubsub mesh optimization failed", "error", err)
	}

	// Always try PubSub first - this is the primary delivery mechanism
	// PubSub works for online peers who are subscribed to their own topic
	// The recipient subscribes to their own topic on startup, so they receive messages there
	pubSubErr := s.ipfsNode.PublishTo(recipientEd25519PubKey, envelopeBytes)

	// Log PubSub result
	if pubSubErr == nil {
		logger.Infow("PubSub send succeeded", "to", hex.EncodeToString(recipientEd25519PubKey)[:16])
	} else {
		logger.Warnw("PubSub send failed",
			"to", hex.EncodeToString(recipientEd25519PubKey)[:16],
			"error", pubSubErr.Error())
	}

	// ALWAYS deposit to mailbox - this is the reliable delivery mechanism for offline recipients
	// Mailbox will try: connected peers → DHT mailboxes
	// This ensures delivery even when PubSub fails or DHT discovery fails
	// Note: If no mailbox is available, this is OK - the message was already sent via PubSub
	if s.mailboxManager != nil {
		logger.Infow("starting mailbox deposit",
			"to", hex.EncodeToString(recipientEd25519PubKey)[:16],
			"pubsub_status", map[bool]string{true: "success", false: "failed"}[pubSubErr == nil],
			"dht_status", map[bool]string{true: "success", false: "failed"}[connectErr == nil])

		// Make defensive copies of public keys to avoid race conditions in goroutine
		recipientKeyCopy := make([]byte, len(recipientEd25519PubKey))
		copy(recipientKeyCopy, recipientEd25519PubKey)
		
		senderKeyCopy := make([]byte, len(s.config.OwnEd25519PubKey))
		copy(senderKeyCopy, s.config.OwnEd25519PubKey)
		
		senderDeviceIDCopy := make([]byte, len(s.config.OwnEd25519PubKey[:16]))
		copy(senderDeviceIDCopy, s.config.OwnEd25519PubKey[:16])

		// Copy envelopeBytes for the goroutine (already serialized SignedEnvelope)
		envelopeBytesCopy := make([]byte, len(envelopeBytes))
		copy(envelopeBytesCopy, envelopeBytes)

		go func() {
			// Store the full serialized SignedEnvelope as payload — same format as PubSub
			babylonEnvelope := &pb.BabylonEnvelope{
				ProtocolVersion:   1,
				MessageType:       pb.MessageType_DM_TEXT,
				SenderIdentity:    senderKeyCopy,
				RecipientIdentity: recipientKeyCopy,
				Timestamp:         uint64(time.Now().Unix()),
				Payload:           envelopeBytesCopy,
				SenderDeviceId:    senderDeviceIDCopy,
			}

			// Deposit to mailbox with timeout
			ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
			defer cancel()

			if err := s.mailboxManager.DepositMessage(ctx, recipientKeyCopy, babylonEnvelope); err != nil {
				logger.Debugw("mailbox deposit failed (recipient may need to come online)",
					"to", hex.EncodeToString(recipientKeyCopy)[:16],
					"error", err)
			}
			// Success is logged inside DepositMessage
			// Note: DepositMessage returning nil is OK - message was sent via PubSub
		}()
	} else {
		logger.Debugw("mailbox manager not available, relying on PubSub only",
			"to", hex.EncodeToString(recipientEd25519PubKey)[:16])
	}

	// Store message locally (sent messages)
	// We store under the recipient's key so we can see our sent messages in the conversation
	// Use signedEnvelopeForStorage which includes the encrypted ephemeral key for decryption
	if err := s.storage.AddMessage(recipientEd25519PubKey, signedEnvelopeForStorage); err != nil {
		logger.Warnw("failed to store sent message", "error", err)
		// Don't fail the send if storage fails
	}

	logger.Debugw("message sent", "to", hex.EncodeToString(recipientEd25519PubKey))

	// Generate a pseudo-CID for reference (hash of envelope)
	cidStr := fmt.Sprintf("poc-%x", envelopeBytes[:8])

	return &SendResult{
		SignedEnvelope: signedEnvelope,
		CID:            cidStr,
		Message:        msg,
	}, nil
}

// SendMessageToContact sends a message to a contact by their Ed25519 public key
// This is a convenience wrapper that looks up the contact's X25519 key
// For PoC, the caller must provide both Ed25519 and X25519 keys
func (s *Service) SendMessageToContact(
	text string,
	contactEd25519PubKey []byte,
	contactX25519PubKey []byte,
) (*SendResult, error) {
	return s.SendMessage(text, contactEd25519PubKey, contactX25519PubKey)
}

// BuildMessageForTesting builds a message without sending
// Useful for testing the encryption flow
func BuildMessageForTesting(
	text string,
	recipientX25519PubKey []byte,
	senderPrivKey ed25519.PrivateKey,
) (*pb.SignedEnvelope, *pb.Message, error) {
	// Build plaintext message
	msg := BuildMessageNow(text)
	plaintext, err := proto.Marshal(msg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// Build encrypted envelope
	envelope, _, err := BuildEnvelope(plaintext, recipientX25519PubKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build envelope: %w", err)
	}

	// Sign envelope
	signedEnvelope, err := SignEnvelope(envelope, senderPrivKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign envelope: %w", err)
	}

	return signedEnvelope, msg, nil
}
