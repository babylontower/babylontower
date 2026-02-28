package multidevice

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"babylontower/pkg/crypto"
	"babylontower/pkg/ipfsnode"
	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"

	"google.golang.org/protobuf/proto"
)

// logger is declared in sync.go for this package

// DeviceSession represents a Double Ratchet session with a specific device
type DeviceSession struct {
	// Recipient device information
	DeviceID      []byte
	DeviceSignPub ed25519.PublicKey
	DeviceDHPub   []byte

	// Session state (Double Ratchet)
	// In full implementation, this would contain the ratchet state
	SessionKey []byte // Placeholder for root key
	ChainKey   []byte // Current chain key
	DHRatchet  []byte // Current DH ratchet key

	// Metadata
	CreatedAt      time.Time
	LastUsedAt     time.Time
	MessageCounter uint32
}

// FanoutManager handles message encryption fanout to multiple devices
type FanoutManager struct {
	deviceManager *DeviceManager
	storage       storage.Storage
	ipfsNode      *ipfsnode.Node

	ctx    context.Context
	cancel context.CancelFunc

	// Sessions per recipient (identity -> device -> session)
	// Key: hex-encoded recipient identity pubkey
	// Value: map of device ID -> DeviceSession
	sessions map[string]map[string]*DeviceSession
	sessMu   sync.RWMutex

	// Optimization: symmetric key cache for recipients with 5+ devices
	symmetricKeys map[string][]byte // recipient identity -> cached symmetric key
	symKeyMu      sync.RWMutex
}

// FanoutConfig holds configuration for the fanout manager
type FanoutConfig struct {
	DeviceManager *DeviceManager
	Storage       storage.Storage
	IPFSNode      *ipfsnode.Node
}

// NewFanoutManager creates a new fanout manager
func NewFanoutManager(config *FanoutConfig) *FanoutManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &FanoutManager{
		deviceManager: config.DeviceManager,
		storage:       config.Storage,
		ipfsNode:      config.IPFSNode,
		ctx:           ctx,
		cancel:        cancel,
		sessions:      make(map[string]map[string]*DeviceSession),
		symmetricKeys: make(map[string][]byte),
	}
}

// SendMessageToIdentity sends a message to all devices of a recipient identity
// Implements the fanout pattern with optimization for 5+ devices
func (fm *FanoutManager) SendMessageToIdentity(
	text string,
	recipientIdentityPub []byte,
	recipientDevices []*pb.DeviceCertificate,
) (*SendResult, error) {
	if len(recipientDevices) == 0 {
		return nil, errors.New("no recipient devices")
	}

	// Optimization: for 5+ devices, use symmetric key encryption
	if len(recipientDevices) >= 5 {
		return fm.sendWithSymmetricKey(text, recipientIdentityPub, recipientDevices)
	}

	// Standard fanout: encrypt separately for each device
	return fm.sendWithFanout(text, recipientIdentityPub, recipientDevices)
}

// sendWithFanout encrypts the message separately for each device
func (fm *FanoutManager) sendWithFanout(
	text string,
	recipientIdentityPub []byte,
	recipientDevices []*pb.DeviceCertificate,
) (*SendResult, error) {
	results := make([]*DeviceSendResult, 0, len(recipientDevices))
	var firstError error
	var successCount int

	for _, device := range recipientDevices {
		result, err := fm.sendToDevice(text, recipientIdentityPub, device)
		if err != nil {
			logger.Warnw("failed to send to device", "device", hex.EncodeToString(device.DeviceId), "error", err)
			if firstError == nil {
				firstError = err
			}
			continue
		}
		results = append(results, result)
		successCount++
	}

	if successCount == 0 {
		return nil, fmt.Errorf("failed to send to all devices: %w", firstError)
	}

	return &SendResult{
		SuccessCount:     successCount,
		TotalDevices:     len(recipientDevices),
		DeviceResults:    results,
		OptimizationUsed: "fanout",
	}, nil
}

// sendWithSymmetricKey uses a shared symmetric key for 5+ devices
func (fm *FanoutManager) sendWithSymmetricKey(
	text string,
	recipientIdentityPub []byte,
	recipientDevices []*pb.DeviceCertificate,
) (*SendResult, error) {
	// Get or create symmetric key
	symKey, err := fm.getOrCreateSymmetricKey(recipientIdentityPub)
	if err != nil {
		return nil, fmt.Errorf("failed to get symmetric key: %w", err)
	}

	// Encrypt message once with symmetric key
	plaintext, err := proto.Marshal(&pb.TextMessage{Text: text})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	nonce := make([]byte, 24)
	if _, err := crypto.SecureRandom.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext, err := crypto.Encrypt(plaintext, symKey, nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt message: %w", err)
	}

	// Encrypt symmetric key for each device
	keyResults := make([]*EncryptedKey, 0, len(recipientDevices))
	for _, device := range recipientDevices {
		encryptedKey, err := fm.encryptKeyForDevice(symKey, device)
		if err != nil {
			logger.Warnw("failed to encrypt key for device", "device", hex.EncodeToString(device.DeviceId), "error", err)
			continue
		}
		keyResults = append(keyResults, encryptedKey)
	}

	// Build multi-device envelope
	envelope := &pb.MultiDeviceEnvelope{
		ProtocolVersion:   1,
		MessageType:       pb.MessageType_DM_TEXT,
		SenderIdentity:    fm.deviceManager.identitySignPub,
		RecipientIdentity: recipientIdentityPub,
		Timestamp:         uint64(time.Now().Unix()),
		MessageId:         generateMessageID(),
		Payload:           ciphertext,
		SenderDeviceId:    fm.deviceManager.deviceID,
		Nonce:             nonce,
		CipherSuiteId:     0x0001,
		EncryptedKeys:     make([]*pb.EncryptedDeviceKey, 0, len(keyResults)),
	}

	for _, ek := range keyResults {
		envelope.EncryptedKeys = append(envelope.EncryptedKeys, &pb.EncryptedDeviceKey{
			DeviceId:       ek.DeviceID,
			EncryptedKey:   ek.EncryptedKey,
			EncryptedNonce: ek.EncryptedNonce,
		})
	}

	// Sign envelope
	signature, err := fm.signEnvelope(envelope)
	if err != nil {
		return nil, fmt.Errorf("failed to sign envelope: %w", err)
	}
	envelope.Signature = signature

	// Publish to recipient's topic
	topic := ipfsnode.TopicFromPublicKey(recipientIdentityPub)
	envelopeBytes, err := proto.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal envelope: %w", err)
	}

	if err := fm.ipfsNode.Publish(topic, envelopeBytes); err != nil {
		return nil, fmt.Errorf("failed to publish: %w", err)
	}

	return &SendResult{
		SuccessCount: len(keyResults),
		TotalDevices: len(recipientDevices),
		DeviceResults: []*DeviceSendResult{
			{
				DeviceID: recipientIdentityPub, // All devices share identity
				Success:  true,
			},
		},
		OptimizationUsed: "symmetric_key",
	}, nil
}

// sendToDevice sends a message to a single device using Double Ratchet
func (fm *FanoutManager) sendToDevice(
	text string,
	recipientIdentityPub []byte,
	device *pb.DeviceCertificate,
) (*DeviceSendResult, error) {
	deviceID := hex.EncodeToString(device.DeviceId)
	identityHex := hex.EncodeToString(recipientIdentityPub)

	// Get or create session
	session, err := fm.getOrCreateSession(recipientIdentityPub, device)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// In full implementation, this would use Double Ratchet to derive message key
	// For now, we use a simplified approach
	messageKey := fm.deriveMessageKey(session)

	// Build DMPayload
	ratchetHeader := &pb.RatchetHeader{
		DhRatchetPub:        session.DHRatchet,
		PreviousChainLength: 0, // Would track in full implementation
		MessageNumber:       session.MessageCounter,
	}

	payload := &pb.DMPayload{
		RatchetHeader: ratchetHeader,
		Content: &pb.DMPayload_Text{
			Text: &pb.TextMessage{
				Text: text,
			},
		},
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Encrypt payload with message key
	nonce := make([]byte, 24)
	if _, err := crypto.SecureRandom.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext, err := crypto.Encrypt(payloadBytes, messageKey, nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt payload: %w", err)
	}

	// Build BabylonEnvelope
	envelope := &pb.BabylonEnvelope{
		ProtocolVersion:   1,
		MessageType:       pb.MessageType_DM_TEXT,
		SenderIdentity:    fm.deviceManager.identitySignPub,
		RecipientIdentity: recipientIdentityPub,
		Timestamp:         uint64(time.Now().Unix()),
		MessageId:         generateMessageID(),
		Payload:           ciphertext,
		SenderDeviceId:    fm.deviceManager.deviceID,
		CipherSuiteId:     0x0001,
	}

	// Sign envelope with device key
	signature, err := fm.signBabylonEnvelope(envelope)
	if err != nil {
		return nil, fmt.Errorf("failed to sign envelope: %w", err)
	}
	envelope.Signature = signature

	// Publish to recipient's topic
	topic := ipfsnode.TopicFromPublicKey(recipientIdentityPub)
	envelopeBytes, err := proto.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal envelope: %w", err)
	}

	if err := fm.ipfsNode.Publish(topic, envelopeBytes); err != nil {
		return nil, fmt.Errorf("failed to publish: %w", err)
	}

	// Update session state
	session.LastUsedAt = time.Now()
	session.MessageCounter++

	// Persist session
	fm.persistSession(identityHex, deviceID, session)

	return &DeviceSendResult{
		DeviceID: device.DeviceId,
		Success:  true,
	}, nil
}

// getOrCreateSession gets an existing session or creates a new one
func (fm *FanoutManager) getOrCreateSession(recipientIdentityPub []byte, device *pb.DeviceCertificate) (*DeviceSession, error) {
	identityHex := hex.EncodeToString(recipientIdentityPub)
	deviceID := hex.EncodeToString(device.DeviceId)

	fm.sessMu.RLock()
	if identitySessions, ok := fm.sessions[identityHex]; ok {
		if session, ok := identitySessions[deviceID]; ok {
			fm.sessMu.RUnlock()
			return session, nil
		}
	}
	fm.sessMu.RUnlock()

	// Create new session
	fm.sessMu.Lock()
	defer fm.sessMu.Unlock()

	if fm.sessions[identityHex] == nil {
		fm.sessions[identityHex] = make(map[string]*DeviceSession)
	}

	session := &DeviceSession{
		DeviceID:       device.DeviceId,
		DeviceSignPub:  device.DeviceSignPub,
		DeviceDHPub:    device.DeviceDhPub,
		CreatedAt:      time.Now(),
		LastUsedAt:     time.Now(),
		MessageCounter: 0,
		// In full implementation, would perform X3DH to establish session key
		SessionKey: make([]byte, 32),
		ChainKey:   make([]byte, 32),
		DHRatchet:  make([]byte, 32),
	}

	fm.sessions[identityHex][deviceID] = session
	return session, nil
}

// deriveMessageKey derives a message key from the session (simplified)
func (fm *FanoutManager) deriveMessageKey(session *DeviceSession) []byte {
	// In full implementation, this would use Double Ratchet KDF
	// For now, use a simple HKDF
	data := append(session.SessionKey, session.ChainKey...)
	hash := sha256.Sum256(data)
	return hash[:32]
}

// getOrCreateSymmetricKey gets or creates a symmetric key for a recipient
func (fm *FanoutManager) getOrCreateSymmetricKey(recipientIdentityPub []byte) ([]byte, error) {
	identityHex := hex.EncodeToString(recipientIdentityPub)

	fm.symKeyMu.RLock()
	if key, ok := fm.symmetricKeys[identityHex]; ok {
		fm.symKeyMu.RUnlock()
		return key, nil
	}
	fm.symKeyMu.RUnlock()

	// Generate new symmetric key
	key := make([]byte, 32)
	if _, err := crypto.SecureRandom.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate symmetric key: %w", err)
	}

	fm.symKeyMu.Lock()
	fm.symmetricKeys[identityHex] = key
	fm.symKeyMu.Unlock()

	// In full implementation, would distribute key to recipient devices
	// via pairwise channels

	return key, nil
}

// encryptKeyForDevice encrypts a symmetric key for a specific device
func (fm *FanoutManager) encryptKeyForDevice(symKey []byte, device *pb.DeviceCertificate) (*EncryptedKey, error) {
	// In full implementation, would use X3DH with device
	// For now, use a simplified ECDH
	ephemeralPub, ephemeralPriv, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		return nil, err
	}

	// Compute shared secret
	sharedSecret, err := crypto.ComputeSharedSecret(ephemeralPriv, device.DeviceDhPub)
	if err != nil {
		return nil, err
	}

	// Derive encryption key
	keyDerivation := sha256.Sum256(sharedSecret)
	encryptionKey := keyDerivation[:]

	// Encrypt symmetric key
	nonce := make([]byte, 24)
	if _, err := crypto.SecureRandom.Read(nonce); err != nil {
		return nil, err
	}

	encryptedKey, err := crypto.Encrypt(symKey, encryptionKey, nonce)
	if err != nil {
		return nil, err
	}

	return &EncryptedKey{
		DeviceID:       device.DeviceId,
		EncryptedKey:   encryptedKey,
		EncryptedNonce: nonce,
		EphemeralPub:   ephemeralPub,
	}, nil
}

// signEnvelope signs a BabylonEnvelope
func (fm *FanoutManager) signBabylonEnvelope(envelope *pb.BabylonEnvelope) ([]byte, error) {
	// Canonical serialization for signing (fields 1-10, 12)
	data := make([]byte, 0, 4+4+32+32+8+16+len(envelope.Payload)+16+24+4)

	// Append fields in order
	data = append(data, byte(envelope.ProtocolVersion))
	data = append(data, byte(envelope.MessageType))
	data = append(data, envelope.SenderIdentity...)
	data = append(data, envelope.RecipientIdentity...)

	tsBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		tsBytes[i] = byte(envelope.Timestamp >> (56 - i*8))
	}
	data = append(data, tsBytes...)

	data = append(data, envelope.MessageId...)
	data = append(data, envelope.Payload...)
	data = append(data, envelope.SenderDeviceId...)

	suiteBytes := make([]byte, 4)
	for i := 0; i < 4; i++ {
		suiteBytes[i] = byte(envelope.CipherSuiteId >> (24 - i*8))
	}
	data = append(data, suiteBytes...)

	signature := ed25519.Sign(fm.deviceManager.deviceSignPriv, data)
	return signature, nil
}

// signEnvelope signs a MultiDeviceEnvelope
func (fm *FanoutManager) signEnvelope(envelope *pb.MultiDeviceEnvelope) ([]byte, error) {
	// Canonical serialization for signing
	data := make([]byte, 0, 256) // Approximate size

	data = append(data, byte(envelope.ProtocolVersion))
	data = append(data, byte(envelope.MessageType))
	data = append(data, envelope.SenderIdentity...)
	data = append(data, envelope.RecipientIdentity...)

	tsBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		tsBytes[i] = byte(envelope.Timestamp >> (56 - i*8))
	}
	data = append(data, tsBytes...)

	data = append(data, envelope.MessageId...)
	data = append(data, envelope.Payload...)
	data = append(data, envelope.SenderDeviceId...)

	signature := ed25519.Sign(fm.deviceManager.deviceSignPriv, data)
	return signature, nil
}

// persistSession persists a session to storage using config key-value pairs
func (fm *FanoutManager) persistSession(identityHex, deviceID string, session *DeviceSession) {
	if fm.storage == nil {
		return
	}
	key := fmt.Sprintf("session:%s:%s", identityHex, deviceID)

	data, err := json.Marshal(session)
	if err != nil {
		logger.Warnw("failed to marshal session for persistence", "error", err)
		return
	}

	if err := fm.storage.SetConfig(key, string(data)); err != nil {
		logger.Warnw("failed to persist session", "identity", identityHex, "device", deviceID, "error", err)
	}
}

// generateMessageID generates a random 16-byte message ID
func generateMessageID() []byte {
	id := make([]byte, 16)
	if _, err := crypto.SecureRandom.Read(id); err != nil {
		// Fallback to timestamp hash
		hash := sha256.Sum256([]byte(time.Now().String()))
		return hash[:16]
	}
	return id
}

// SendResult contains the result of a multi-device send operation
type SendResult struct {
	SuccessCount     int
	TotalDevices     int
	DeviceResults    []*DeviceSendResult
	OptimizationUsed string // "fanout" or "symmetric_key"
}

// DeviceSendResult contains the result for a single device
type DeviceSendResult struct {
	DeviceID []byte
	Success  bool
	Error    error
}

// EncryptedKey contains an encrypted symmetric key for a device
type EncryptedKey struct {
	DeviceID       []byte
	EncryptedKey   []byte
	EncryptedNonce []byte
	EphemeralPub   []byte
}

// Stop stops the fanout manager
func (fm *FanoutManager) Stop() error {
	fm.cancel()
	return nil
}

// Common errors
var (
	ErrNoDevices       = errors.New("no recipient devices")
	ErrSessionNotFound = errors.New("session not found")
	ErrKeyDistribution = errors.New("failed to distribute symmetric key")
)
