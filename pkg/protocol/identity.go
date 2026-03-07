// Package protocol implements the Babylon Tower Protocol v1 specification.
package protocol

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"babylontower/pkg/crypto"
	"babylontower/pkg/identity"
	pb "babylontower/pkg/proto"

	"google.golang.org/protobuf/proto"

	dht "github.com/libp2p/go-libp2p-kad-dht"
)

// IdentityManager handles identity document lifecycle management.
// It implements the IdentityStore interface and provides methods for:
//   - Creating and signing identity documents
//   - Managing device certificates
//   - Generating and rotating prekeys
//   - Publishing and retrieving identity documents from the DHT
//
// Identity Document Flow:
//   1. Load or create local identity from mnemonic
//   2. Generate device keys and certificates
//   3. Generate signed prekeys and one-time prekeys
//   4. Create and sign identity document
//   5. Publish to DHT at /bt/id/{identity_hash}
//   6. Periodically republish to maintain DHT presence
//   7. Rotate prekeys as needed
type IdentityManager struct {
	// config is the protocol configuration
	config *ProtocolConfig
	// networkNode is the underlying network interface
	networkNode NetworkNode
	// ipfsDHT is the DHT instance for publishing/retrieving
	ipfsDHT *dht.IpfsDHT

	// Local identity
	localIdentity   *identity.Identity
	localDeviceID   []byte
	localDeviceCert *DeviceCertificate

	// Prekey management
	signedPrekeys  []*SignedPrekey
	oneTimePrekeys []*OneTimePrekey
	prekeyMu       sync.RWMutex

	// Identity document cache
	localDoc      *IdentityDocument
	remoteCache   map[string]*IdentityDocument // identity_pub_hex -> document
	cacheMu       sync.RWMutex

	// Republish timer
	republishTimer *time.Ticker
	republishDone  chan struct{}

	// Mutex for protecting shared state
	mu sync.RWMutex
}

// IdentityManagerOpts contains options for creating an IdentityManager.
type IdentityManagerOpts struct {
	Config      *ProtocolConfig
	NetworkNode NetworkNode
	DHT         *dht.IpfsDHT
	Identity    *identity.Identity
	DeviceName  string
}

// NewIdentityManager creates a new identity manager.
// It requires the protocol configuration, network node, and local identity.
func NewIdentityManager(opts *IdentityManagerOpts) (*IdentityManager, error) {
	if opts.Config == nil {
		return nil, errors.New("config is required")
	}
	if opts.NetworkNode == nil {
		return nil, errors.New("network node is required")
	}
	if opts.Identity == nil {
		return nil, errors.New("identity is required")
	}

	deviceName := opts.DeviceName
	if deviceName == "" {
		deviceName = "Babylon Tower Device"
	}

	mgr := &IdentityManager{
		config:        opts.Config,
		networkNode:   opts.NetworkNode,
		ipfsDHT:       opts.DHT,
		localIdentity: opts.Identity,
		remoteCache:   make(map[string]*IdentityDocument),
		republishDone: make(chan struct{}),
	}

	// Generate device ID and certificate
	if err := mgr.initializeDevice(); err != nil {
		return nil, fmt.Errorf("failed to initialize device: %w", err)
	}

	// Generate initial prekeys
	if err := mgr.generatePrekeys(); err != nil {
		return nil, fmt.Errorf("failed to generate prekeys: %w", err)
	}

	return mgr, nil
}

// initializeDevice creates the device certificate.
func (m *IdentityManager) initializeDevice() error {
	// Generate device key pair
	deviceSignPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate device signing key: %w", err)
	}

	deviceDHPub, _, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate device DH key: %w", err)
	}

	// Create device ID (SHA256 of device signing pub, first 16 bytes)
	hash := sha256.Sum256(deviceSignPub)
	deviceID := hash[:16]

	// Create device certificate
	now := uint64(time.Now().Unix())
	cert := &DeviceCertificate{
		DeviceID:      deviceID,
		DeviceSignPub: deviceSignPub,
		DeviceDHPub:   deviceDHPub,
		DeviceName:    m.config.DeviceName,
		CreatedAt:     now,
		ExpiresAt:     m.config.DeviceExpiresAt,
		IdentityPub:   m.localIdentity.Ed25519PubKey,
	}

	// Sign the certificate with identity key
	cert.Signature, err = m.signDeviceCertificate(cert, m.localIdentity.Ed25519PrivKey)
	if err != nil {
		return fmt.Errorf("failed to sign device certificate: %w", err)
	}

	m.localDeviceID = deviceID
	m.localDeviceCert = cert

	logger.Infow("Initialized device",
		"device_id", hex.EncodeToString(deviceID),
		"device_name", m.config.DeviceName)

	return nil
}

// signDeviceCertificate signs a device certificate with the identity key.
func (m *IdentityManager) signDeviceCertificate(cert *DeviceCertificate, identityPriv ed25519.PrivateKey) ([]byte, error) {
	data, err := m.serializeDeviceCertificateForSigning(cert)
	if err != nil {
		return nil, err
	}
	signature := ed25519.Sign(identityPriv, data)
	return signature, nil
}

// serializeDeviceCertificateForSigning serializes certificate fields for signing.
func (m *IdentityManager) serializeDeviceCertificateForSigning(cert *DeviceCertificate) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Field 1: device_id (16 bytes)
	buf.Write(cert.DeviceID)

	// Field 2: device_sign_pub (32 bytes)
	buf.Write(cert.DeviceSignPub)

	// Field 3: device_dh_pub (32 bytes)
	buf.Write(cert.DeviceDHPub)

	// Field 4: device_name (length-prefixed)
	nameBytes := []byte(cert.DeviceName)
	lenBytes := make([]byte, 4)
	binaryLittleEndianPutUint32(lenBytes, uint32(len(nameBytes)))
	buf.Write(lenBytes)
	buf.Write(nameBytes)

	// Field 5: created_at (8 bytes)
	tsBytes := make([]byte, 8)
	binaryLittleEndianPutUint64(tsBytes, cert.CreatedAt)
	buf.Write(tsBytes)

	// Field 6: expires_at (8 bytes)
	tsBytes = make([]byte, 8)
	binaryLittleEndianPutUint64(tsBytes, cert.ExpiresAt)
	buf.Write(tsBytes)

	// Field 7: identity_pub (32 bytes)
	buf.Write(cert.IdentityPub)

	return buf.Bytes(), nil
}

// generatePrekeys generates signed prekeys and one-time prekeys.
func (m *IdentityManager) generatePrekeys() error {
	m.prekeyMu.Lock()
	defer m.prekeyMu.Unlock()

	now := uint64(time.Now().Unix())

	// Generate signed prekey
	spkPub, _, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate signed prekey: %w", err)
	}

	spk := &SignedPrekey{
		DeviceID:  m.localDeviceID,
		PrekeyPub: spkPub,
		PrekeyID:  1,
		CreatedAt: now,
		ExpiresAt: now + uint64(m.config.SignedPrekeyRotationInterval/time.Second),
	}

	// Sign the prekey with identity key
	spk.Signature, err = m.signPrekey(spk, m.localIdentity.Ed25519PrivKey)
	if err != nil {
		return fmt.Errorf("failed to sign prekey: %w", err)
	}

	m.signedPrekeys = []*SignedPrekey{spk}

	// Generate one-time prekeys
	targetCount := m.config.PrekeyTargetCount
	if targetCount <= 0 {
		targetCount = DefaultPrekeyTargetCount
	}

	m.oneTimePrekeys = make([]*OneTimePrekey, 0, targetCount)
	for i := 0; i < targetCount; i++ {
		opkPub, _, err := crypto.GenerateX25519KeyPair()
		if err != nil {
			return fmt.Errorf("failed to generate one-time prekey %d: %w", i, err)
		}

		opk := &OneTimePrekey{
			DeviceID:  m.localDeviceID,
			PrekeyPub: opkPub,
			PrekeyID:  uint64(i + 1),
		}
		m.oneTimePrekeys = append(m.oneTimePrekeys, opk)
	}

	logger.Infow("Generated prekeys",
		"signed_prekeys", len(m.signedPrekeys),
		"one_time_prekeys", len(m.oneTimePrekeys))

	return nil
}

// signPrekey signs a prekey with the identity key.
func (m *IdentityManager) signPrekey(prekey *SignedPrekey, identityPriv ed25519.PrivateKey) ([]byte, error) {
	data, err := m.serializePrekeyForSigning(prekey)
	if err != nil {
		return nil, err
	}
	signature := ed25519.Sign(identityPriv, data)
	return signature, nil
}

// serializePrekeyForSigning serializes prekey fields for signing.
func (m *IdentityManager) serializePrekeyForSigning(prekey *SignedPrekey) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Field 1: device_id (16 bytes)
	buf.Write(prekey.DeviceID)

	// Field 2: prekey_pub (32 bytes)
	buf.Write(prekey.PrekeyPub)

	// Field 3: prekey_id (8 bytes)
	idBytes := make([]byte, 8)
	binaryLittleEndianPutUint64(idBytes, prekey.PrekeyID)
	buf.Write(idBytes)

	// Field 4: created_at (8 bytes)
	tsBytes := make([]byte, 8)
	binaryLittleEndianPutUint64(tsBytes, prekey.CreatedAt)
	buf.Write(tsBytes)

	// Field 5: expires_at (8 bytes)
	tsBytes = make([]byte, 8)
	binaryLittleEndianPutUint64(tsBytes, prekey.ExpiresAt)
	buf.Write(tsBytes)

	return buf.Bytes(), nil
}

// CreateIdentityDocument creates a new identity document.
func (m *IdentityManager) CreateIdentityDocument() (*IdentityDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := uint64(time.Now().Unix())

	doc := &IdentityDocument{
		IdentitySignPub: m.localIdentity.Ed25519PubKey,
		IdentityDHPub:   m.localIdentity.X25519PubKey,
		Sequence:        1,
		PreviousHash:    []byte{}, // Empty for first document
		CreatedAt:       now,
		UpdatedAt:       now,
		Devices:         []DeviceCertificate{*m.localDeviceCert},
		SignedPrekeys:   make([]SignedPrekey, len(m.signedPrekeys)),
		OneTimePrekeys:  make([]OneTimePrekey, len(m.oneTimePrekeys)),
		SupportedVersions: []uint32{ProtocolVersion},
		SupportedCipherSuites: []string{"BT-X25519-XChaCha20Poly1305-SHA256"},
		PreferredVersion: ProtocolVersion,
		DisplayName:    m.config.DeviceName,
		Features: FeatureFlags{
			SupportsReadReceipts:     true,
			SupportsTypingIndicators: true,
			SupportsReactions:        true,
			SupportsEdits:            true,
			SupportsMedia:            true,
			SupportsVoiceCalls:       true,
			SupportsVideoCalls:       true,
			SupportsGroups:           true,
			SupportsChannels:         true,
			SupportsOfflineMessages:  true,
		},
	}

	// Copy prekeys
	for i, spk := range m.signedPrekeys {
		doc.SignedPrekeys[i] = *spk
	}
	for i, opk := range m.oneTimePrekeys {
		doc.OneTimePrekeys[i] = *opk
	}

	// Sign the document
	var err error
	doc.Signature, err = m.signIdentityDocument(doc, m.localIdentity.Ed25519PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign identity document: %w", err)
	}

	m.localDoc = doc
	return doc, nil
}

// signIdentityDocument signs an identity document with the identity key.
func (m *IdentityManager) signIdentityDocument(doc *IdentityDocument, identityPriv ed25519.PrivateKey) ([]byte, error) {
	data, err := m.serializeIdentityDocumentForSigning(doc)
	if err != nil {
		return nil, err
	}
	signature := ed25519.Sign(identityPriv, data)
	return signature, nil
}

// serializeIdentityDocumentForSigning serializes document fields for signing.
// Per spec section 1.4, all 17 fields must be serialized in canonical form.
// This uses a deterministic binary serialization for signature computation.
func (m *IdentityManager) serializeIdentityDocumentForSigning(doc *IdentityDocument) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Field 1: identity_sign_pub (32 bytes)
	buf.Write(doc.IdentitySignPub)

	// Field 2: identity_dh_pub (32 bytes)
	buf.Write(doc.IdentityDHPub)

	// Field 3: sequence (8 bytes, little-endian)
	seqBytes := make([]byte, 8)
	binaryLittleEndianPutUint64(seqBytes, doc.Sequence)
	buf.Write(seqBytes)

	// Field 4: previous_hash (32 bytes)
	if len(doc.PreviousHash) > 0 {
		buf.Write(doc.PreviousHash)
	} else {
		buf.Write(make([]byte, 32))
	}

	// Field 5: created_at (8 bytes, little-endian)
	tsBytes := make([]byte, 8)
	binaryLittleEndianPutUint64(tsBytes, doc.CreatedAt)
	buf.Write(tsBytes)

	// Field 6: updated_at (8 bytes, little-endian)
	tsBytes = make([]byte, 8)
	binaryLittleEndianPutUint64(tsBytes, doc.UpdatedAt)
	buf.Write(tsBytes)

	// Field 7: devices (repeated DeviceCertificate)
	// Serialize as: count (4 bytes) + concatenated certificates
	devicesCount := uint32(len(doc.Devices))
	devicesCountBytes := make([]byte, 4)
	binaryLittleEndianPutUint32(devicesCountBytes, devicesCount)
	buf.Write(devicesCountBytes)
	
	for _, device := range doc.Devices {
		deviceBytes, err := m.serializeDeviceCertificateForSigning(&device)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize device: %w", err)
		}
		buf.Write(deviceBytes)
	}

	// Field 8: signed_prekeys (repeated SignedPrekey)
	// Serialize as: count (4 bytes) + concatenated prekeys
	spkCount := uint32(len(doc.SignedPrekeys))
	spkCountBytes := make([]byte, 4)
	binaryLittleEndianPutUint32(spkCountBytes, spkCount)
	buf.Write(spkCountBytes)
	
	for _, spk := range doc.SignedPrekeys {
		spkBytes, err := m.serializePrekeyForSigning(&spk)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize signed prekey: %w", err)
		}
		buf.Write(spkBytes)
	}

	// Field 9: one_time_prekeys (repeated OneTimePrekey)
	// Serialize as: count (4 bytes) + concatenated prekeys
	opkCount := uint32(len(doc.OneTimePrekeys))
	opkCountBytes := make([]byte, 4)
	binaryLittleEndianPutUint32(opkCountBytes, opkCount)
	buf.Write(opkCountBytes)
	
	for _, opk := range doc.OneTimePrekeys {
		// OneTimePrekey serialization: device_id (16) + prekey_pub (32) + prekey_id (8)
		buf.Write(opk.DeviceID)
		buf.Write(opk.PrekeyPub)
		opkIDBytes := make([]byte, 8)
		binaryLittleEndianPutUint64(opkIDBytes, opk.PrekeyID)
		buf.Write(opkIDBytes)
	}

	// Field 10: supported_versions (repeated uint32)
	// Serialize as: count (4 bytes) + concatenated version numbers
	versionsCount := uint32(len(doc.SupportedVersions))
	versionsCountBytes := make([]byte, 4)
	binaryLittleEndianPutUint32(versionsCountBytes, versionsCount)
	buf.Write(versionsCountBytes)
	
	for _, version := range doc.SupportedVersions {
		versionBytes := make([]byte, 4)
		binaryLittleEndianPutUint32(versionBytes, version)
		buf.Write(versionBytes)
	}

	// Field 11: supported_cipher_suites (repeated string)
	// Serialize as: count (4 bytes) + concatenated length-prefixed strings
	cipherSuitesCount := uint32(len(doc.SupportedCipherSuites))
	cipherSuitesCountBytes := make([]byte, 4)
	binaryLittleEndianPutUint32(cipherSuitesCountBytes, cipherSuitesCount)
	buf.Write(cipherSuitesCountBytes)
	
	for _, suite := range doc.SupportedCipherSuites {
		suiteBytes := []byte(suite)
		suiteLenBytes := make([]byte, 4)
		binaryLittleEndianPutUint32(suiteLenBytes, uint32(len(suiteBytes)))
		buf.Write(suiteLenBytes)
		buf.Write(suiteBytes)
	}

	// Field 12: preferred_version (4 bytes)
	preferredVersionBytes := make([]byte, 4)
	binaryLittleEndianPutUint32(preferredVersionBytes, doc.PreferredVersion)
	buf.Write(preferredVersionBytes)

	// Field 13: display_name (length-prefixed string)
	displayNameBytes := []byte(doc.DisplayName)
	displayNameLenBytes := make([]byte, 4)
	binaryLittleEndianPutUint32(displayNameLenBytes, uint32(len(displayNameBytes)))
	buf.Write(displayNameLenBytes)
	buf.Write(displayNameBytes)

	// Field 14: avatar_cid (length-prefixed string)
	avatarCIDBytes := []byte(doc.AvatarCID)
	avatarCIDLenBytes := make([]byte, 4)
	binaryLittleEndianPutUint32(avatarCIDLenBytes, uint32(len(avatarCIDBytes)))
	buf.Write(avatarCIDLenBytes)
	buf.Write(avatarCIDBytes)

	// Field 15: revocations (repeated RevocationCertificate)
	// Serialize as: count (4 bytes) + concatenated certificates
	revocationsCount := uint32(len(doc.Revocations))
	revocationsCountBytes := make([]byte, 4)
	binaryLittleEndianPutUint32(revocationsCountBytes, revocationsCount)
	buf.Write(revocationsCountBytes)
	
	for _, rev := range doc.Revocations {
		// RevocationCertificate serialization:
		// revoked_key (32) + revocation_type (len-prefixed) + reason (len-prefixed) + revoked_at (8)
		buf.Write(rev.RevokedKey)
		
		typeBytes := []byte(rev.RevocationType)
		typeLenBytes := make([]byte, 4)
		binaryLittleEndianPutUint32(typeLenBytes, uint32(len(typeBytes)))
		buf.Write(typeLenBytes)
		buf.Write(typeBytes)
		
		reasonBytes := []byte(rev.Reason)
		reasonLenBytes := make([]byte, 4)
		binaryLittleEndianPutUint32(reasonLenBytes, uint32(len(reasonBytes)))
		buf.Write(reasonLenBytes)
		buf.Write(reasonBytes)
		
		revokedAtBytes := make([]byte, 8)
		binaryLittleEndianPutUint64(revokedAtBytes, rev.RevokedAt)
		buf.Write(revokedAtBytes)
	}

	// Field 16: features (FeatureFlags)
	// Serialize all bool fields as bytes (0 or 1), then custom_features
	featureBytes := []byte{
		boolToByte(doc.Features.SupportsReadReceipts),
		boolToByte(doc.Features.SupportsTypingIndicators),
		boolToByte(doc.Features.SupportsReactions),
		boolToByte(doc.Features.SupportsEdits),
		boolToByte(doc.Features.SupportsMedia),
		boolToByte(doc.Features.SupportsVoiceCalls),
		boolToByte(doc.Features.SupportsVideoCalls),
		boolToByte(doc.Features.SupportsGroups),
		boolToByte(doc.Features.SupportsChannels),
		boolToByte(doc.Features.SupportsOfflineMessages),
	}
	buf.Write(featureBytes)
	
	// custom_features (repeated string)
	customFeaturesCount := uint32(len(doc.Features.CustomFeatures))
	customFeaturesCountBytes := make([]byte, 4)
	binaryLittleEndianPutUint32(customFeaturesCountBytes, customFeaturesCount)
	buf.Write(customFeaturesCountBytes)
	
	for _, feature := range doc.Features.CustomFeatures {
		featureBytes := []byte(feature)
		featureLenBytes := make([]byte, 4)
		binaryLittleEndianPutUint32(featureLenBytes, uint32(len(featureBytes)))
		buf.Write(featureLenBytes)
		buf.Write(featureBytes)
	}

	// Note: Field 17 (signature) is NOT included in the serialization for signing
	// The signature covers fields 1-16 only

	return buf.Bytes(), nil
}

// boolToByte converts a bool to a byte (0 or 1)
func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// GetLocal returns the local identity document.
func (m *IdentityManager) GetLocal() (*IdentityDocument, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.localDoc == nil {
		return nil, errors.New("local identity document not created")
	}

	return m.localDoc, nil
}

// GetRemote retrieves a remote identity document from the DHT.
func (m *IdentityManager) GetRemote(ctx context.Context, identityPub ed25519.PublicKey) (*IdentityDocument, error) {
	// Check cache first
	cacheKey := hex.EncodeToString(identityPub)
	m.cacheMu.RLock()
	if cached, ok := m.remoteCache[cacheKey]; ok {
		m.cacheMu.RUnlock()
		logger.Debugw("Found cached identity document", "identity", cacheKey)
		return cached, nil
	}
	m.cacheMu.RUnlock()

	// Fetch from DHT
	dhtKey := m.identityDHTKey(identityPub)

	result, err := m.ipfsDHT.GetValue(ctx, dhtKey)
	if err != nil {
		// Check for not found error
		if err.Error() == "key not found" || err.Error() == "record was not found" {
			return nil, ErrIdentityNotFound
		}
		return nil, fmt.Errorf("DHT get failed: %w", err)
	}

	// Parse the document
	doc, err := m.parseIdentityDocument(result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse identity document: %w", err)
	}

	// Validate the document
	if err := m.Validate(doc); err != nil {
		return nil, fmt.Errorf("identity validation failed: %w", err)
	}

	// Cache the document
	m.cacheMu.Lock()
	m.remoteCache[cacheKey] = doc
	m.cacheMu.Unlock()

	logger.Debugw("Retrieved identity document from DHT",
		"identity", cacheKey,
		"sequence", doc.Sequence)

	return doc, nil
}

// PutLocal stores the local identity document.
func (m *IdentityManager) PutLocal(doc *IdentityDocument) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.localDoc = doc
	return nil
}

// Publish publishes an identity document to the DHT.
func (m *IdentityManager) Publish(ctx context.Context, doc *IdentityDocument) error {
	// Validate first
	if err := m.Validate(doc); err != nil {
		return fmt.Errorf("identity validation failed: %w", err)
	}

	// Serialize the document
	data, err := m.marshalIdentityDocument(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal identity document: %w", err)
	}

	// Check size
	if len(data) > MaxIdentityDocumentSize {
		return fmt.Errorf("identity document too large: %d bytes (max %d)", len(data), MaxIdentityDocumentSize)
	}

	// Publish to DHT
	dhtKey := m.identityDHTKey(doc.IdentitySignPub)

	ttl := m.config.IdentityRecordTTL
	if ttl <= 0 {
		ttl = IdentityRecordTTL
	}

	err = m.ipfsDHT.PutValue(ctx, dhtKey, data)
	if err != nil {
		return fmt.Errorf("DHT put failed: %w", err)
	}

	logger.Infow("Published identity document to DHT",
		"identity", hex.EncodeToString(doc.IdentitySignPub),
		"sequence", doc.Sequence,
		"ttl", ttl)

	// Also publish standalone prekey bundle for efficient fetching
	if err := m.PublishPrekeyBundleSeparate(ctx); err != nil {
		logger.Warnw("Failed to publish standalone prekey bundle", "error", err)
		// Don't fail the publish - prekey bundle is optional optimization
	}

	return nil
}

// Validate validates an identity document.
func (m *IdentityManager) Validate(doc *IdentityDocument) error {
	// Verify signature
	if err := m.verifyIdentityDocumentSignature(doc); err != nil {
		return err
	}

	// Verify device certificate signatures
	for _, device := range doc.Devices {
		if err := m.verifyDeviceCertificate(&device); err != nil {
			return fmt.Errorf("device certificate invalid: %w", err)
		}
	}

	// Verify signed prekey signatures
	for _, spk := range doc.SignedPrekeys {
		if err := m.verifyPrekeySignature(&spk, doc.IdentitySignPub); err != nil {
			return fmt.Errorf("signed prekey invalid: %w", err)
		}
	}

	// Verify pubkey hashes match
	hash := sha256.Sum256(doc.IdentitySignPub)
	expectedKey := hex.EncodeToString(hash[:16])
	hash2 := sha256.Sum256(doc.IdentitySignPub)
	actualKey := hex.EncodeToString(hash2[:16])
	if expectedKey != actualKey {
		return errors.New("identity pubkey hash mismatch")
	}

	return nil
}

// verifyIdentityDocumentSignature verifies the identity document signature.
func (m *IdentityManager) verifyIdentityDocumentSignature(doc *IdentityDocument) error {
	data, err := m.serializeIdentityDocumentForSigning(doc)
	if err != nil {
		return err
	}

	if !ed25519.Verify(doc.IdentitySignPub, data, doc.Signature) {
		return ErrSignatureInvalid
	}

	return nil
}

// verifyDeviceCertificate verifies a device certificate signature.
func (m *IdentityManager) verifyDeviceCertificate(cert *DeviceCertificate) error {
	data, err := m.serializeDeviceCertificateForSigning(cert)
	if err != nil {
		return err
	}

	if !ed25519.Verify(cert.IdentityPub, data, cert.Signature) {
		return ErrSignatureInvalid
	}

	return nil
}

// verifyPrekeySignature verifies a prekey signature.
func (m *IdentityManager) verifyPrekeySignature(prekey *SignedPrekey, identityPub ed25519.PublicKey) error {
	data, err := m.serializePrekeyForSigning(prekey)
	if err != nil {
		return err
	}

	if !ed25519.Verify(identityPub, data, prekey.Signature) {
		return ErrSignatureInvalid
	}

	return nil
}

// GetPrekeyBundle retrieves a prekey bundle for X3DH.
func (m *IdentityManager) GetPrekeyBundle(ctx context.Context, identityPub ed25519.PublicKey) (*PrekeyBundle, error) {
	// Get the identity document
	doc, err := m.GetRemote(ctx, identityPub)
	if err != nil {
		return nil, err
	}

	// Extract prekey bundle
	if len(doc.SignedPrekeys) == 0 {
		return nil, ErrPrekeyInvalid
	}

	bundle := &PrekeyBundle{
		IdentityDHPub:   doc.IdentityDHPub,
		IdentitySignPub: doc.IdentitySignPub,
		SignedPrekeyPub: doc.SignedPrekeys[0].PrekeyPub,
		SignedPrekeySig: doc.SignedPrekeys[0].Signature,
		SignedPrekeyID:  doc.SignedPrekeys[0].PrekeyID,
	}

	// Include one-time prekey if available
	if len(doc.OneTimePrekeys) > 0 {
		bundle.OneTimePrekeyPub = doc.OneTimePrekeys[0].PrekeyPub
		bundle.OneTimePrekeyID = doc.OneTimePrekeys[0].PrekeyID
	}

	return bundle, nil
}

// identityDHTKey creates the DHT key for an identity document.
func (m *IdentityManager) identityDHTKey(identityPub ed25519.PublicKey) string {
	hash := sha256.Sum256(identityPub)
	return DHTNamespaceIdentity + hex.EncodeToString(hash[:16])
}

// prekeyBundleDHTKey creates the DHT key for a standalone prekey bundle.
// Per spec section 4.2: SHA256("bt-prekeys-v1:" ‖ ed25519_pubkey)
func (m *IdentityManager) prekeyBundleDHTKey(identityPub ed25519.PublicKey) string {
	// Concatenate prefix and pubkey
	prefix := []byte("bt-prekeys-v1:")
	data := append(prefix, identityPub...)
	// Compute SHA256
	hash := sha256.Sum256(data)
	return DHTNamespacePrekeys + hex.EncodeToString(hash[:])
}

// PublishPrekeyBundleSeparate publishes a standalone prekey bundle to the DHT.
// This allows efficient prekey fetching without retrieving the full identity document.
// Per spec section 4.2.
func (m *IdentityManager) PublishPrekeyBundleSeparate(ctx context.Context) error {
	m.mu.RLock()
	doc := m.localDoc
	m.prekeyMu.RLock()
	defer m.mu.RUnlock()
	defer m.mu.RUnlock()

	if doc == nil {
		return errors.New("no local identity document")
	}

	// Build prekey bundle
	bundle := &pb.PrekeyBundle{
		IdentitySignPub:       doc.IdentitySignPub,
		IdentityDhPub:         doc.IdentityDHPub,
		SignedPrekeys:         nil,
		OneTimePrekeys:        nil,
		SupportedCipherSuites: doc.SupportedCipherSuites,
		PublishedAt:           uint64(time.Now().Unix()),
	}

	// Include signed prekeys
	if len(doc.SignedPrekeys) > 0 {
		bundle.SignedPrekeys = make([]*pb.SignedPrekey, len(doc.SignedPrekeys))
		for i, spk := range doc.SignedPrekeys {
			bundle.SignedPrekeys[i] = m.toProtoSignedPrekey(&spk)
		}
	}

	// Include one-time prekeys
	if len(doc.OneTimePrekeys) > 0 {
		bundle.OneTimePrekeys = make([]*pb.OneTimePrekey, len(doc.OneTimePrekeys))
		for i, opk := range doc.OneTimePrekeys {
			bundle.OneTimePrekeys[i] = m.toProtoOneTimePrekey(&opk)
		}
	}

	// Marshal the bundle
	data, err := proto.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("failed to marshal prekey bundle: %w", err)
	}

	// Publish to DHT
	dhtKey := m.prekeyBundleDHTKey(doc.IdentitySignPub)

	ttl := m.config.IdentityRecordTTL
	if ttl <= 0 {
		ttl = IdentityRecordTTL
	}

	err = m.ipfsDHT.PutValue(ctx, dhtKey, data)
	if err != nil {
		return fmt.Errorf("DHT put failed for prekey bundle: %w", err)
	}

	logger.Infow("Published standalone prekey bundle to DHT",
		"identity", hex.EncodeToString(doc.IdentitySignPub),
		"signed_prekeys", len(bundle.SignedPrekeys),
		"one_time_prekeys", len(bundle.OneTimePrekeys),
		"ttl", ttl)

	return nil
}

// GetPrekeyBundleSeparate retrieves a standalone prekey bundle from the DHT.
// This is more efficient than fetching the full identity document when only prekeys are needed.
// Per spec section 4.2.
func (m *IdentityManager) GetPrekeyBundleSeparate(ctx context.Context, identityPub ed25519.PublicKey) (*PrekeyBundle, error) {
	dhtKey := m.prekeyBundleDHTKey(identityPub)

	data, err := m.ipfsDHT.GetValue(ctx, dhtKey)
	if err != nil {
		// Fall back to fetching from identity document
		if err.Error() == "key not found" || err.Error() == "record was not found" {
			logger.Debugw("Standalone prekey bundle not found, falling back to identity document",
				"identity", hex.EncodeToString(identityPub))
			return m.GetPrekeyBundle(ctx, identityPub)
		}
		return nil, fmt.Errorf("DHT get failed for prekey bundle: %w", err)
	}

	// Parse the protobuf bundle
	pbBundle := &pb.PrekeyBundle{}
	if err := proto.Unmarshal(data, pbBundle); err != nil {
		return nil, fmt.Errorf("failed to unmarshal prekey bundle: %w", err)
	}

	// Convert to internal type
	bundle := &PrekeyBundle{
		IdentityDHPub:     pbBundle.IdentityDhPub,
		IdentitySignPub:   pbBundle.IdentitySignPub,
		SignedPrekeyPub:   nil,
		SignedPrekeySig:   nil,
		SignedPrekeyID:    0,
	}

	// Include first signed prekey if available
	if len(pbBundle.SignedPrekeys) > 0 {
		spk := pbBundle.SignedPrekeys[0]
		bundle.SignedPrekeyPub = spk.PrekeyPub
		bundle.SignedPrekeySig = spk.Signature
		bundle.SignedPrekeyID = spk.PrekeyId
	}

	// Include first one-time prekey if available
	if len(pbBundle.OneTimePrekeys) > 0 {
		opk := pbBundle.OneTimePrekeys[0]
		bundle.OneTimePrekeyPub = opk.PrekeyPub
		bundle.OneTimePrekeyID = opk.PrekeyId
	}

	logger.Debugw("Retrieved standalone prekey bundle from DHT",
		"identity", hex.EncodeToString(identityPub),
		"has_signed_prekey", bundle.SignedPrekeyPub != nil,
		"has_one_time_prekey", bundle.OneTimePrekeyPub != nil)

	return bundle, nil
}

// parseIdentityDocument parses a serialized identity document.
// Uses protobuf serialization per protocol spec v1.
func (m *IdentityManager) parseIdentityDocument(data []byte) (*IdentityDocument, error) {
	// Parse protobuf
	pbDoc := &pb.IdentityDocument{}
	if err := proto.Unmarshal(data, pbDoc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal protobuf identity document: %w", err)
	}

	// Convert to internal type
	doc, err := m.fromProtoIdentityDocument(pbDoc)
	if err != nil {
		return nil, fmt.Errorf("failed to convert identity document: %w", err)
	}

	return doc, nil
}

// marshalIdentityDocument marshals an identity document using protobuf.
// Per protocol spec v1, protobuf is used for efficient wire format.
func (m *IdentityManager) marshalIdentityDocument(doc *IdentityDocument) ([]byte, error) {
	// Convert to protobuf type
	pbDoc, err := m.toProtoIdentityDocument(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to convert identity document: %w", err)
	}

	// Marshal protobuf
	data, err := proto.Marshal(pbDoc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal protobuf identity document: %w", err)
	}

	return data, nil
}

// toProtoIdentityDocument converts internal IdentityDocument to protobuf.
func (m *IdentityManager) toProtoIdentityDocument(doc *IdentityDocument) (*pb.IdentityDocument, error) {
	pbDoc := &pb.IdentityDocument{
		IdentitySignPub:     doc.IdentitySignPub,
		IdentityDhPub:       doc.IdentityDHPub,
		Sequence:            doc.Sequence,
		PreviousHash:        doc.PreviousHash,
		CreatedAt:           doc.CreatedAt,
		UpdatedAt:           doc.UpdatedAt,
		SupportedVersions:   doc.SupportedVersions,
		SupportedCipherSuites: doc.SupportedCipherSuites,
		PreferredVersion:    doc.PreferredVersion,
		DisplayName:         doc.DisplayName,
		AvatarCid:           doc.AvatarCID,
		Signature:           doc.Signature,
	}

	// Convert devices
	pbDoc.Devices = make([]*pb.DeviceCertificate, len(doc.Devices))
	for i, device := range doc.Devices {
		pbDoc.Devices[i] = m.toProtoDeviceCertificate(&device)
	}

	// Convert signed prekeys
	pbDoc.SignedPrekeys = make([]*pb.SignedPrekey, len(doc.SignedPrekeys))
	for i, spk := range doc.SignedPrekeys {
		pbDoc.SignedPrekeys[i] = m.toProtoSignedPrekey(&spk)
	}

	// Convert one-time prekeys
	pbDoc.OneTimePrekeys = make([]*pb.OneTimePrekey, len(doc.OneTimePrekeys))
	for i, opk := range doc.OneTimePrekeys {
		pbDoc.OneTimePrekeys[i] = m.toProtoOneTimePrekey(&opk)
	}

	// Convert revocations
	pbDoc.Revocations = make([]*pb.RevocationCertificate, len(doc.Revocations))
	for i, rev := range doc.Revocations {
		pbDoc.Revocations[i] = m.toProtoRevocationCertificate(&rev)
	}

	// Convert feature flags
	pbDoc.Features = m.toProtoFeatureFlags(&doc.Features)

	return pbDoc, nil
}

// fromProtoIdentityDocument converts protobuf IdentityDocument to internal type.
func (m *IdentityManager) fromProtoIdentityDocument(pbDoc *pb.IdentityDocument) (*IdentityDocument, error) {
	doc := &IdentityDocument{
		IdentitySignPub:     pbDoc.IdentitySignPub,
		IdentityDHPub:       pbDoc.IdentityDhPub,
		Sequence:            pbDoc.Sequence,
		PreviousHash:        pbDoc.PreviousHash,
		CreatedAt:           pbDoc.CreatedAt,
		UpdatedAt:           pbDoc.UpdatedAt,
		SupportedVersions:   pbDoc.SupportedVersions,
		SupportedCipherSuites: pbDoc.SupportedCipherSuites,
		PreferredVersion:    pbDoc.PreferredVersion,
		DisplayName:         pbDoc.DisplayName,
		AvatarCID:           pbDoc.AvatarCid,
		Signature:           pbDoc.Signature,
	}

	// Convert devices
	doc.Devices = make([]DeviceCertificate, len(pbDoc.Devices))
	for i, pbDevice := range pbDoc.Devices {
		device, err := m.fromProtoDeviceCertificate(pbDevice)
		if err != nil {
			return nil, fmt.Errorf("failed to convert device %d: %w", i, err)
		}
		doc.Devices[i] = *device
	}

	// Convert signed prekeys
	doc.SignedPrekeys = make([]SignedPrekey, len(pbDoc.SignedPrekeys))
	for i, pbSpk := range pbDoc.SignedPrekeys {
		spk, err := m.fromProtoSignedPrekey(pbSpk)
		if err != nil {
			return nil, fmt.Errorf("failed to convert signed prekey %d: %w", i, err)
		}
		doc.SignedPrekeys[i] = *spk
	}

	// Convert one-time prekeys
	doc.OneTimePrekeys = make([]OneTimePrekey, len(pbDoc.OneTimePrekeys))
	for i, pbOpk := range pbDoc.OneTimePrekeys {
		opk := m.fromProtoOneTimePrekey(pbOpk)
		doc.OneTimePrekeys[i] = *opk
	}

	// Convert revocations
	doc.Revocations = make([]RevocationCertificate, len(pbDoc.Revocations))
	for i, pbRev := range pbDoc.Revocations {
		rev := m.fromProtoRevocationCertificate(pbRev)
		doc.Revocations[i] = *rev
	}

	// Convert feature flags
	if pbDoc.Features != nil {
		doc.Features = *m.fromProtoFeatureFlags(pbDoc.Features)
	}

	return doc, nil
}

// toProtoDeviceCertificate converts internal DeviceCertificate to protobuf.
func (m *IdentityManager) toProtoDeviceCertificate(cert *DeviceCertificate) *pb.DeviceCertificate {
	return &pb.DeviceCertificate{
		DeviceId:      cert.DeviceID,
		DeviceSignPub: cert.DeviceSignPub,
		DeviceDhPub:   cert.DeviceDHPub,
		DeviceName:    cert.DeviceName,
		CreatedAt:     cert.CreatedAt,
		ExpiresAt:     cert.ExpiresAt,
		IdentityPub:   cert.IdentityPub,
		Signature:     cert.Signature,
	}
}

// fromProtoDeviceCertificate converts protobuf DeviceCertificate to internal type.
func (m *IdentityManager) fromProtoDeviceCertificate(pbCert *pb.DeviceCertificate) (*DeviceCertificate, error) {
	if len(pbCert.DeviceSignPub) != 32 {
		return nil, fmt.Errorf("invalid device signing public key length")
	}
	return &DeviceCertificate{
		DeviceID:      pbCert.DeviceId,
		DeviceSignPub: ed25519.PublicKey(pbCert.DeviceSignPub),
		DeviceDHPub:   pbCert.DeviceDhPub,
		DeviceName:    pbCert.DeviceName,
		CreatedAt:     pbCert.CreatedAt,
		ExpiresAt:     pbCert.ExpiresAt,
		IdentityPub:   ed25519.PublicKey(pbCert.IdentityPub),
		Signature:     pbCert.Signature,
	}, nil
}

// toProtoSignedPrekey converts internal SignedPrekey to protobuf.
func (m *IdentityManager) toProtoSignedPrekey(spk *SignedPrekey) *pb.SignedPrekey {
	return &pb.SignedPrekey{
		DeviceId:  spk.DeviceID,
		PrekeyPub: spk.PrekeyPub,
		PrekeyId:  spk.PrekeyID,
		CreatedAt: spk.CreatedAt,
		ExpiresAt: spk.ExpiresAt,
		Signature: spk.Signature,
	}
}

// fromProtoSignedPrekey converts protobuf SignedPrekey to internal type.
func (m *IdentityManager) fromProtoSignedPrekey(pbSpk *pb.SignedPrekey) (*SignedPrekey, error) {
	return &SignedPrekey{
		DeviceID:  pbSpk.DeviceId,
		PrekeyPub: pbSpk.PrekeyPub,
		PrekeyID:  pbSpk.PrekeyId,
		CreatedAt: pbSpk.CreatedAt,
		ExpiresAt: pbSpk.ExpiresAt,
		Signature: pbSpk.Signature,
	}, nil
}

// toProtoOneTimePrekey converts internal OneTimePrekey to protobuf.
func (m *IdentityManager) toProtoOneTimePrekey(opk *OneTimePrekey) *pb.OneTimePrekey {
	return &pb.OneTimePrekey{
		DeviceId:  opk.DeviceID,
		PrekeyPub: opk.PrekeyPub,
		PrekeyId:  opk.PrekeyID,
	}
}

// fromProtoOneTimePrekey converts protobuf OneTimePrekey to internal type.
func (m *IdentityManager) fromProtoOneTimePrekey(pbOpk *pb.OneTimePrekey) *OneTimePrekey {
	return &OneTimePrekey{
		DeviceID:  pbOpk.DeviceId,
		PrekeyPub: pbOpk.PrekeyPub,
		PrekeyID:  pbOpk.PrekeyId,
	}
}

// toProtoRevocationCertificate converts internal RevocationCertificate to protobuf.
func (m *IdentityManager) toProtoRevocationCertificate(rev *RevocationCertificate) *pb.RevocationCertificate {
	return &pb.RevocationCertificate{
		RevokedKey:     rev.RevokedKey,
		RevocationType: rev.RevocationType,
		Reason:         rev.Reason,
		RevokedAt:      rev.RevokedAt,
		Signature:      rev.Signature,
	}
}

// fromProtoRevocationCertificate converts protobuf RevocationCertificate to internal type.
func (m *IdentityManager) fromProtoRevocationCertificate(pbRev *pb.RevocationCertificate) *RevocationCertificate {
	return &RevocationCertificate{
		RevokedKey:     pbRev.RevokedKey,
		RevocationType: pbRev.RevocationType,
		Reason:         pbRev.Reason,
		RevokedAt:      pbRev.RevokedAt,
		Signature:      pbRev.Signature,
	}
}

// toProtoFeatureFlags converts internal FeatureFlags to protobuf.
func (m *IdentityManager) toProtoFeatureFlags(flags *FeatureFlags) *pb.FeatureFlags {
	return &pb.FeatureFlags{
		SupportsReadReceipts:     flags.SupportsReadReceipts,
		SupportsTypingIndicators: flags.SupportsTypingIndicators,
		SupportsReactions:        flags.SupportsReactions,
		SupportsEdits:            flags.SupportsEdits,
		SupportsMedia:            flags.SupportsMedia,
		SupportsVoiceCalls:       flags.SupportsVoiceCalls,
		SupportsVideoCalls:       flags.SupportsVideoCalls,
		SupportsGroups:           flags.SupportsGroups,
		SupportsChannels:         flags.SupportsChannels,
		SupportsOfflineMessages:  flags.SupportsOfflineMessages,
		CustomFeatures:           flags.CustomFeatures,
	}
}

// fromProtoFeatureFlags converts protobuf FeatureFlags to internal type.
func (m *IdentityManager) fromProtoFeatureFlags(pbFlags *pb.FeatureFlags) *FeatureFlags {
	if pbFlags == nil {
		return &FeatureFlags{}
	}
	return &FeatureFlags{
		SupportsReadReceipts:     pbFlags.SupportsReadReceipts,
		SupportsTypingIndicators: pbFlags.SupportsTypingIndicators,
		SupportsReactions:        pbFlags.SupportsReactions,
		SupportsEdits:            pbFlags.SupportsEdits,
		SupportsMedia:            pbFlags.SupportsMedia,
		SupportsVoiceCalls:       pbFlags.SupportsVoiceCalls,
		SupportsVideoCalls:       pbFlags.SupportsVideoCalls,
		SupportsGroups:           pbFlags.SupportsGroups,
		SupportsChannels:         pbFlags.SupportsChannels,
		SupportsOfflineMessages:  pbFlags.SupportsOfflineMessages,
		CustomFeatures:           pbFlags.CustomFeatures,
	}
}

// GetLocalDeviceID returns the local device ID.
func (m *IdentityManager) GetLocalDeviceID() []byte {
	return m.localDeviceID
}

// GetLocalIdentity returns the local identity.
func (m *IdentityManager) GetLocalIdentity() *identity.Identity {
	return m.localIdentity
}

// GetSignedPrekeys returns the current signed prekeys.
func (m *IdentityManager) GetSignedPrekeys() []*SignedPrekey {
	m.prekeyMu.RLock()
	defer m.prekeyMu.RUnlock()

	result := make([]*SignedPrekey, len(m.signedPrekeys))
	copy(result, m.signedPrekeys)
	return result
}

// GetOneTimePrekeys returns the current one-time prekeys.
func (m *IdentityManager) GetOneTimePrekeys() []*OneTimePrekey {
	m.prekeyMu.RLock()
	defer m.prekeyMu.RUnlock()

	result := make([]*OneTimePrekey, len(m.oneTimePrekeys))
	copy(result, m.oneTimePrekeys)
	return result
}

// GetOneTimePrekeyCount returns the number of available one-time prekeys.
func (m *IdentityManager) GetOneTimePrekeyCount() int {
	m.prekeyMu.RLock()
	defer m.prekeyMu.RUnlock()
	return len(m.oneTimePrekeys)
}

// ConsumeOneTimePrekey removes a one-time prekey after use.
func (m *IdentityManager) ConsumeOneTimePrekey(prekeyID uint64) error {
	m.prekeyMu.Lock()
	defer m.prekeyMu.Unlock()

	for i, opk := range m.oneTimePrekeys {
		if opk.PrekeyID == prekeyID {
			m.oneTimePrekeys = append(m.oneTimePrekeys[:i], m.oneTimePrekeys[i+1:]...)
			logger.Debugw("Consumed one-time prekey", "prekey_id", prekeyID)
			return nil
		}
	}

	return ErrPrekeyInvalid
}

// RotateSignedPrekey generates a new signed prekey.
func (m *IdentityManager) RotateSignedPrekey() error {
	m.prekeyMu.Lock()
	defer m.prekeyMu.Unlock()

	now := uint64(time.Now().Unix())

	// Generate new signed prekey
	spkPub, _, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate signed prekey: %w", err)
	}

	// Find max prekey ID
	maxID := uint64(0)
	for _, spk := range m.signedPrekeys {
		if spk.PrekeyID > maxID {
			maxID = spk.PrekeyID
		}
	}

	spk := &SignedPrekey{
		DeviceID:  m.localDeviceID,
		PrekeyPub: spkPub,
		PrekeyID:  maxID + 1,
		CreatedAt: now,
		ExpiresAt: now + uint64(m.config.SignedPrekeyRotationInterval/time.Second),
	}

	// Sign the prekey
	spk.Signature, err = m.signPrekey(spk, m.localIdentity.Ed25519PrivKey)
	if err != nil {
		return fmt.Errorf("failed to sign prekey: %w", err)
	}

	// Add new prekey
	m.signedPrekeys = append(m.signedPrekeys, spk)

	// Remove expired prekeys
	var valid []*SignedPrekey
	expiryThreshold := now - uint64(m.config.SignedPrekeyMaxAge/time.Second)
	for _, spk := range m.signedPrekeys {
		if spk.CreatedAt >= expiryThreshold {
			valid = append(valid, spk)
		}
	}
	m.signedPrekeys = valid

	logger.Infow("Rotated signed prekey",
		"new_prekey_id", spk.PrekeyID,
		"active_prekeys", len(m.signedPrekeys))

	return nil
}

// ReplenishOneTimePrekeys generates new one-time prekeys if below threshold.
func (m *IdentityManager) ReplenishOneTimePrekeys() error {
	m.prekeyMu.Lock()
	defer m.prekeyMu.Unlock()

	currentCount := len(m.oneTimePrekeys)
	targetCount := m.config.PrekeyTargetCount
	threshold := m.config.PrekeyReplenishThreshold

	if currentCount >= threshold {
		return nil // No replenishment needed
	}

	// Find max prekey ID
	maxID := uint64(0)
	for _, opk := range m.oneTimePrekeys {
		if opk.PrekeyID > maxID {
			maxID = opk.PrekeyID
		}
	}

	// Generate new prekeys
	batchSize := m.config.PrekeyBatchSize
	if batchSize <= 0 {
		batchSize = 80
	}

	for i := 0; i < batchSize && len(m.oneTimePrekeys) < targetCount; i++ {
		opkPub, _, err := crypto.GenerateX25519KeyPair()
		if err != nil {
			return fmt.Errorf("failed to generate one-time prekey: %w", err)
		}

		opk := &OneTimePrekey{
			DeviceID:  m.localDeviceID,
			PrekeyPub: opkPub,
			PrekeyID:  maxID + uint64(i) + 1,
		}
		m.oneTimePrekeys = append(m.oneTimePrekeys, opk)
	}

	logger.Infow("Replenished one-time prekeys",
		"previous_count", currentCount,
		"new_count", len(m.oneTimePrekeys))

	return nil
}

// StartRepublish starts the identity document republishing goroutine.
func (m *IdentityManager) StartRepublish(ctx context.Context) error {
	interval := m.config.IdentityRepublishInterval
	if interval <= 0 {
		interval = IdentityRepublishInterval
	}

	m.republishTimer = time.NewTicker(interval)

	go func() {
		defer m.republishTimer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-m.republishTimer.C:
				if err := m.republishIdentity(ctx); err != nil {
					logger.Warnw("Failed to republish identity", "error", err)
				}
			}
		}
	}()

	logger.Infow("Started identity republish", "interval", interval)
	return nil
}

// republishIdentity republishes the local identity document.
func (m *IdentityManager) republishIdentity(ctx context.Context) error {
	m.mu.RLock()
	doc := m.localDoc
	m.mu.RUnlock()

	if doc == nil {
		return errors.New("no local identity document")
	}

	// Update timestamp and republish
	doc.UpdatedAt = uint64(time.Now().Unix())

	var err error
	doc.Signature, err = m.signIdentityDocument(doc, m.localIdentity.Ed25519PrivKey)
	if err != nil {
		return fmt.Errorf("failed to re-sign identity document: %w", err)
	}

	return m.Publish(ctx, doc)
}

// Stop stops the identity manager.
func (m *IdentityManager) Stop() error {
	if m.republishTimer != nil {
		m.republishTimer.Stop()
	}
	close(m.republishDone)
	return nil
}

// Helper functions for binary encoding
func binaryLittleEndianPutUint32(b []byte, v uint32) {
	_ = b[3]
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func binaryLittleEndianPutUint64(b []byte, v uint64) {
	_ = b[7]
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
	b[4] = byte(v >> 32)
	b[5] = byte(v >> 40)
	b[6] = byte(v >> 48)
	b[7] = byte(v >> 56)
}
