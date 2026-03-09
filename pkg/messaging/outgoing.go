package messaging

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	pb "babylontower/pkg/proto"
	"babylontower/pkg/protocol"
	"babylontower/pkg/ratchet"
	"babylontower/pkg/storage"

	"google.golang.org/protobuf/proto"
)

// SendResult contains the result of a sent message
type SendResult struct {
	// BabylonEnvelope is the Protocol v1 envelope that was sent
	BabylonEnvelope *pb.BabylonEnvelope
	// CID is the IPFS content identifier
	CID string
	// Message is the original plaintext message
	Message *pb.Message
}

// sessionResult holds the result of getOrCreateRatchetSession.
// For new sessions, x3dhResult is non-nil and should be used to build
// the X3DHHeader in the first envelope.
type sessionResult struct {
	state      *ratchet.DoubleRatchetState
	x3dhResult *ratchet.X3DHResult // non-nil only for newly created sessions
}

// SendMessage sends a message to a contact using Protocol v1 BabylonEnvelope
// with X3DH key agreement and Double Ratchet encryption per spec §2.2-§2.3.
//
// Flow:
// 1. Establish or reuse Double Ratchet session (X3DH for new sessions)
// 2. Build DMPayload with text content
// 3. Encrypt via Double Ratchet (forward secrecy)
// 4. Encode RatchetHeader + ciphertext into payload
// 5. For new sessions: attach X3DHHeader to envelope
// 6. Build BabylonEnvelope with EnvelopeBuilder (signed, versioned)
// 7. Publish via PubSub + deposit to mailbox
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

	// Contact-aware routing: Try to connect to recipient BEFORE sending
	logger.Infow("attempting DHT peer discovery for contact",
		"to", hex.EncodeToString(recipientEd25519PubKey)[:16])

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

	// Get or create Double Ratchet session (X3DH for new sessions)
	sess, err := s.getOrCreateRatchetSession(recipientEd25519PubKey, recipientX25519PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to establish ratchet session: %w", err)
	}

	// Build DMPayload
	msg := BuildMessageNow(text)
	dmPayload := &pb.DMPayload{
		Content: &pb.DMPayload_Text{
			Text: &pb.TextMessage{
				Text: text,
			},
		},
	}

	payloadBytes, err := proto.Marshal(dmPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal DMPayload: %w", err)
	}

	// Encrypt with Double Ratchet (per spec §2.3: AD = sender.IK_sign.pub ‖ recipient.IK_sign.pub)
	ad := append(s.config.OwnEd25519PubKey, recipientEd25519PubKey...)
	encryptedMsg, err := sess.state.Encrypt(payloadBytes, ad)
	if err != nil {
		return nil, fmt.Errorf("Double Ratchet encrypt failed: %w", err)
	}

	// Encode RatchetHeader + ciphertext into payload (per spec §3.3)
	ratchetHeader := &pb.RatchetHeader{
		DhRatchetPub:        encryptedMsg.Header.DHRatchetPub[:],
		PreviousChainLength: encryptedMsg.Header.PreviousChainLen,
		MessageNumber:       encryptedMsg.Header.MessageNumber,
	}
	payloadWire, err := encodeRatchetPayload(ratchetHeader, encryptedMsg.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to encode ratchet payload: %w", err)
	}

	// Determine device ID
	deviceID := s.config.OwnEd25519PubKey[:16]
	if s.config.IdentityV1 != nil {
		deviceID = s.config.IdentityV1.DeviceID
	}

	// Build BabylonEnvelope using EnvelopeBuilder
	babylonEnvelope, err := protocol.NewEnvelopeBuilder(
		s.config.OwnEd25519PubKey,
		deviceID,
		s.config.OwnEd25519PrivKey,
	).
		MessageType(pb.MessageType_DM_TEXT).
		Recipient(recipientEd25519PubKey).
		Payload(payloadWire).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build BabylonEnvelope: %w", err)
	}

	// Attach X3DHHeader for session establishment.
	// For new sessions, build and store it. For existing sessions that haven't
	// been confirmed by the recipient, re-attach the stored header so the
	// recipient can establish the session even if they missed earlier messages.
	recipientHex := hex.EncodeToString(recipientEd25519PubKey)
	if sess.x3dhResult != nil {
		x3dhHeader := buildX3DHHeader(sess.x3dhResult, s.config.OwnX25519PubKey)
		x3dhHeaderBytes, err := proto.Marshal(x3dhHeader)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal X3DHHeader: %w", err)
		}
		babylonEnvelope.X3DhHeader = x3dhHeaderBytes
		// Store for re-use until recipient confirms session
		s.ratchetMu.Lock()
		s.pendingX3DHHeaders[recipientHex] = x3dhHeaderBytes
		s.ratchetMu.Unlock()
	} else {
		// Re-attach stored X3DHHeader if session not yet confirmed
		s.ratchetMu.RLock()
		if stored, ok := s.pendingX3DHHeaders[recipientHex]; ok {
			babylonEnvelope.X3DhHeader = stored
		}
		s.ratchetMu.RUnlock()
	}

	// Serialize for transmission
	envelopeBytes, err := proto.Marshal(babylonEnvelope)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal BabylonEnvelope: %w", err)
	}

	// Publish via PubSub
	pubSubErr := s.ipfsNode.PublishTo(recipientEd25519PubKey, envelopeBytes)
	if pubSubErr == nil {
		logger.Infow("PubSub send succeeded",
			"to", hex.EncodeToString(recipientEd25519PubKey)[:16])
	} else {
		logger.Warnw("PubSub send failed",
			"to", hex.EncodeToString(recipientEd25519PubKey)[:16],
			"error", pubSubErr.Error())
	}

	// Deposit to mailbox for offline delivery
	s.depositToMailbox(recipientEd25519PubKey, babylonEnvelope, connectErr)

	// Store plaintext message locally
	storedMsg := &storage.StoredMessage{
		Text:         text,
		Timestamp:    msg.Timestamp,
		SenderPubKey: s.config.OwnEd25519PubKey,
		IsOutgoing:   true,
	}
	if err := s.storage.AddMessage(recipientEd25519PubKey, storedMsg); err != nil {
		logger.Warnw("failed to store sent message", "error", err)
	}

	cidStr := fmt.Sprintf("v1-%x", envelopeBytes[:8])

	return &SendResult{
		BabylonEnvelope: babylonEnvelope,
		CID:             cidStr,
		Message:         msg,
	}, nil
}

// depositToMailbox deposits a BabylonEnvelope to mailbox for offline delivery
func (s *Service) depositToMailbox(recipientPubKey []byte, envelope *pb.BabylonEnvelope, connectErr error) {
	if s.mailboxManager == nil {
		logger.Debugw("mailbox manager not available, relying on PubSub only",
			"to", hex.EncodeToString(recipientPubKey)[:16])
		return
	}

	logger.Infow("starting mailbox deposit",
		"to", hex.EncodeToString(recipientPubKey)[:16],
		"dht_status", map[bool]string{true: "success", false: "failed"}[connectErr == nil])

	// Make defensive copy
	recipientKeyCopy := make([]byte, len(recipientPubKey))
	copy(recipientKeyCopy, recipientPubKey)

	// Deep copy the envelope for the goroutine
	envBytes, err := proto.Marshal(envelope)
	if err != nil {
		logger.Warnw("failed to marshal envelope for mailbox", "error", err)
		return
	}

	go func() {
		var envCopy pb.BabylonEnvelope
		if err := proto.Unmarshal(envBytes, &envCopy); err != nil {
			logger.Warnw("failed to unmarshal envelope copy for mailbox", "error", err)
			return
		}

		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
		defer cancel()

		if err := s.mailboxManager.DepositMessage(ctx, recipientKeyCopy, &envCopy); err != nil {
			logger.Debugw("mailbox deposit failed (recipient may need to come online)",
				"to", hex.EncodeToString(recipientKeyCopy)[:16],
				"error", err)
		}
	}()
}

// getOrCreateRatchetSession gets an existing Double Ratchet session or creates
// a new one via X3DH key agreement per protocol spec §2.2.
//
// For new sessions, the returned sessionResult.x3dhResult is non-nil and
// contains the ephemeral public key needed for the X3DHHeader.
//
// Currently uses the recipient's IK_dh as SPK fallback since prekey bundles
// are not yet fetched from DHT. Full SPK/OPK support will be added when
// IdentityDocument lookup is wired.
func (s *Service) getOrCreateRatchetSession(
	recipientEd25519PubKey []byte,
	recipientX25519PubKey []byte,
) (*sessionResult, error) {
	recipientHex := hex.EncodeToString(recipientEd25519PubKey)

	// Check for existing session
	s.ratchetMu.RLock()
	if state, ok := s.ratchetSessions[recipientHex]; ok {
		s.ratchetMu.RUnlock()
		return &sessionResult{state: state}, nil
	}
	s.ratchetMu.RUnlock()

	if s.config.IdentityV1 == nil {
		return nil, errors.New("IdentityV1 not configured, cannot establish X3DH session")
	}

	// Create new session via X3DH (per spec §2.2)
	s.ratchetMu.Lock()
	defer s.ratchetMu.Unlock()

	// Double-check after acquiring write lock
	if state, ok := s.ratchetSessions[recipientHex]; ok {
		return &sessionResult{state: state}, nil
	}

	// Use recipient's IK_dh as SPK fallback
	// (full SPK/OPK lookup from DHT IdentityDocument will replace this)
	var recipientDHPub [32]byte
	copy(recipientDHPub[:], recipientX25519PubKey)

	x3dhResult, err := ratchet.X3DHInitiator(
		s.config.IdentityV1.IKDHPriv,
		s.config.IdentityV1.IKDHPub,
		s.config.IdentityV1.IKSignPub,
		&recipientDHPub, // recipient IK_dh
		recipientEd25519PubKey,
		&recipientDHPub, // use as SPK (best available without DHT prekey bundle)
		nil,             // no OPK available
	)
	if err != nil {
		return nil, fmt.Errorf("X3DH key agreement failed: %w", err)
	}

	sessionID := fmt.Sprintf("dm:%s", recipientHex)
	state, err := ratchet.NewDoubleRatchetStateInitiator(
		sessionID,
		s.config.OwnEd25519PubKey,
		recipientEd25519PubKey,
		x3dhResult.SharedSecret,
		&recipientDHPub,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Double Ratchet: %w", err)
	}

	s.ratchetSessions[recipientHex] = state
	logger.Infow("established X3DH ratchet session (initiator)",
		"to", recipientHex[:16])

	return &sessionResult{state: state, x3dhResult: x3dhResult}, nil
}

// buildX3DHHeader constructs the X3DHHeader protobuf from X3DH result.
// Per spec §2.2 step 6: Alice sends IK_dh.pub, EK.pub, SPK_id, OPK_id, cipher_suite_id.
func buildX3DHHeader(result *ratchet.X3DHResult, initiatorIKDHPub []byte) *pb.X3DHHeader {
	header := &pb.X3DHHeader{
		InitiatorIdentityDhPub: initiatorIKDHPub,
		CipherSuiteId:          result.CipherSuite,
	}
	if result.EphemeralPub != nil {
		header.EphemeralPub = result.EphemeralPub[:]
	}
	return header
}

// GetRatchetSession returns the Double Ratchet session for a contact, if one exists
func (s *Service) GetRatchetSession(recipientEd25519PubKey []byte) (*ratchet.DoubleRatchetState, bool) {
	recipientHex := hex.EncodeToString(recipientEd25519PubKey)
	s.ratchetMu.RLock()
	defer s.ratchetMu.RUnlock()
	state, ok := s.ratchetSessions[recipientHex]
	return state, ok
}

// SendMessageToContact sends a message to a contact by their Ed25519 public key
func (s *Service) SendMessageToContact(
	text string,
	contactEd25519PubKey []byte,
	contactX25519PubKey []byte,
) (*SendResult, error) {
	return s.SendMessage(text, contactEd25519PubKey, contactX25519PubKey)
}

// SendSignalingPayload sends a raw signaling payload (e.g., RTC offer/answer/ICE)
// to a recipient, encrypted with Double Ratchet via BabylonEnvelope.
// Per §8.1: RTC signaling is carried as DM through Double Ratchet.
func (s *Service) SendSignalingPayload(
	recipientEd25519PubKey []byte,
	messageType pb.MessageType,
	payload []byte,
) error {
	s.mu.RLock()
	if !s.isStarted {
		s.mu.RUnlock()
		return ErrServiceNotStarted
	}
	s.mu.RUnlock()

	if len(recipientEd25519PubKey) != ed25519.PublicKeySize {
		return errors.New("invalid recipient Ed25519 public key length")
	}

	// Look up X25519 key for recipient from storage
	recipientX25519PubKey, err := s.storage.GetContactX25519Key(recipientEd25519PubKey)
	if err != nil || len(recipientX25519PubKey) != 32 {
		return fmt.Errorf("cannot find X25519 key for recipient: %w", err)
	}

	// Get or create Double Ratchet session
	sess, err := s.getOrCreateRatchetSession(recipientEd25519PubKey, recipientX25519PubKey)
	if err != nil {
		return fmt.Errorf("failed to establish ratchet session: %w", err)
	}

	// Encrypt payload with Double Ratchet
	ad := append(s.config.OwnEd25519PubKey, recipientEd25519PubKey...)
	encryptedMsg, err := sess.state.Encrypt(payload, ad)
	if err != nil {
		return fmt.Errorf("Double Ratchet encrypt failed: %w", err)
	}

	// Encode RatchetHeader + ciphertext into payload
	ratchetHeader := &pb.RatchetHeader{
		DhRatchetPub:        encryptedMsg.Header.DHRatchetPub[:],
		PreviousChainLength: encryptedMsg.Header.PreviousChainLen,
		MessageNumber:       encryptedMsg.Header.MessageNumber,
	}
	payloadWire, err := encodeRatchetPayload(ratchetHeader, encryptedMsg.Ciphertext)
	if err != nil {
		return fmt.Errorf("failed to encode ratchet payload: %w", err)
	}

	// Determine device ID
	deviceID := s.config.OwnEd25519PubKey[:16]
	if s.config.IdentityV1 != nil {
		deviceID = s.config.IdentityV1.DeviceID
	}

	// Build BabylonEnvelope
	babylonEnvelope, err := protocol.NewEnvelopeBuilder(
		s.config.OwnEd25519PubKey,
		deviceID,
		s.config.OwnEd25519PrivKey,
	).
		MessageType(messageType).
		Recipient(recipientEd25519PubKey).
		Payload(payloadWire).
		Build()
	if err != nil {
		return fmt.Errorf("failed to build BabylonEnvelope: %w", err)
	}

	// Attach X3DHHeader for session establishment (same logic as SendMessage)
	sigRecipientHex := hex.EncodeToString(recipientEd25519PubKey)
	if sess.x3dhResult != nil {
		x3dhHeader := buildX3DHHeader(sess.x3dhResult, s.config.OwnX25519PubKey)
		x3dhHeaderBytes, err := proto.Marshal(x3dhHeader)
		if err != nil {
			return fmt.Errorf("failed to marshal X3DHHeader: %w", err)
		}
		babylonEnvelope.X3DhHeader = x3dhHeaderBytes
		s.ratchetMu.Lock()
		s.pendingX3DHHeaders[sigRecipientHex] = x3dhHeaderBytes
		s.ratchetMu.Unlock()
	} else {
		s.ratchetMu.RLock()
		if stored, ok := s.pendingX3DHHeaders[sigRecipientHex]; ok {
			babylonEnvelope.X3DhHeader = stored
		}
		s.ratchetMu.RUnlock()
	}

	// Serialize and publish via PubSub
	envelopeBytes, err := proto.Marshal(babylonEnvelope)
	if err != nil {
		return fmt.Errorf("failed to marshal BabylonEnvelope: %w", err)
	}

	if err := s.ipfsNode.PublishTo(recipientEd25519PubKey, envelopeBytes); err != nil {
		return fmt.Errorf("failed to publish signaling message: %w", err)
	}

	logger.Infow("sent signaling payload",
		"type", messageType.String(),
		"to", hex.EncodeToString(recipientEd25519PubKey)[:16])

	return nil
}
