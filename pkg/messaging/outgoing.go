package messaging

import (
	"crypto/ed25519"
	"fmt"

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
		return nil, fmt.Errorf("invalid recipient Ed25519 public key length")
	}
	if len(recipientX25519PubKey) != 32 {
		return nil, fmt.Errorf("invalid recipient X25519 public key length")
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
		Envelope:              signedEnvelope.Envelope,
		Signature:             signedEnvelope.Signature,
		SenderPubkey:          signedEnvelope.SenderPubkey,
		EncryptedEphemeralPriv: encryptedEphemeral,
	}

	// Serialize signed envelope for transmission (WITHOUT encrypted ephemeral key)
	envelopeBytes, err := proto.Marshal(signedEnvelope)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signed envelope: %w", err)
	}

	// Contact-aware routing: Try to connect to recipient before sending
	go func() {
		// Attempt connection (non-blocking, best effort)
		if err := s.connectToPeerID(string(recipientEd25519PubKey)); err != nil {
			logger.Debugw("pre-send connection attempt failed", "error", err)
		}

		// Optimize pubsub mesh for better delivery
		if err := s.OptimizePubSubMesh(recipientEd25519PubKey); err != nil {
			logger.Debugw("pubsub mesh optimization failed", "error", err)
		}
	}()

	// Publish envelope directly via PubSub to recipient's topic
	// This avoids the IPFS Get limitation in PoC
	// Note: envelopeBytes is from signedEnvelope WITHOUT encrypted ephemeral key
	if err := s.ipfsNode.PublishTo(recipientEd25519PubKey, envelopeBytes); err != nil {
		return nil, fmt.Errorf("failed to publish envelope: %w", err)
	}

	// Store message locally (sent messages)
	// We store under the recipient's key so we can see our sent messages in the conversation
	// Use signedEnvelopeForStorage which includes the encrypted ephemeral key for decryption
	if err := s.storage.AddMessage(recipientEd25519PubKey, signedEnvelopeForStorage); err != nil {
		logger.Warnw("failed to store sent message", "error", err)
		// Don't fail the send if storage fails
	}

	logger.Debugw("message sent", "to", fmt.Sprintf("%x", recipientEd25519PubKey))

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
