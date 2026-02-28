package identity

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	pb "babylontower/pkg/proto"

	"google.golang.org/protobuf/proto"
)

// IdentityDocumentManager manages IdentityDocument creation, signing, and validation
type IdentityDocumentManager struct {
	identity *IdentityV1
}

// NewIdentityDocumentManager creates a new manager for an identity
func NewIdentityDocumentManager(identity *IdentityV1) *IdentityDocumentManager {
	return &IdentityDocumentManager{
		identity: identity,
	}
}

// CreateIdentityDocument creates a new IdentityDocument or updates an existing one
func (m *IdentityDocumentManager) CreateIdentityDocument(
	prevSequence uint64,
	prevHash []byte,
	devices []*pb.DeviceCertificate,
	signedPrekeys []*pb.SignedPrekey,
	oneTimePrekeys []*pb.OneTimePrekey,
	displayName string,
) (*pb.IdentityDocument, error) {
	now := time.Now()

	// Set default feature flags
	features := &pb.FeatureFlags{
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
	}

	sequence := prevSequence + 1
	if sequence == 0 {
		sequence = 1 // Start at 1 if no previous
	}

	doc := &pb.IdentityDocument{
		IdentitySignPub:       m.identity.IKSignPub,
		IdentityDhPub:         m.identity.IKDHPub[:],
		Sequence:              sequence,
		PreviousHash:          prevHash,
		CreatedAt:             uint64(now.Unix()),
		UpdatedAt:             uint64(now.Unix()),
		Devices:               devices,
		SignedPrekeys:         signedPrekeys,
		OneTimePrekeys:        oneTimePrekeys,
		SupportedVersions:     []uint32{1},
		SupportedCipherSuites: []string{"BT-X25519-XChaCha20Poly1305-SHA256"},
		PreferredVersion:      1,
		DisplayName:           displayName,
		Features:              features,
	}

	// Set created_at to previous timestamp if updating
	if prevSequence > 0 {
		doc.CreatedAt = prevSequence // Will be overwritten by actual creation time on first doc
	}

	// Sign the document
	signature, err := m.SignIdentityDocument(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to sign identity document: %w", err)
	}
	doc.Signature = signature

	return doc, nil
}

// SignIdentityDocument signs an IdentityDocument with the identity key
func (m *IdentityDocumentManager) SignIdentityDocument(doc *pb.IdentityDocument) ([]byte, error) {
	// Canonical serialization for signing (fields 1-16)
	data, err := m.serializeDocumentForSigning(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize document: %w", err)
	}

	signature := ed25519.Sign(m.identity.IKSignPriv, data)
	return signature, nil
}

// serializeDocumentForSigning serializes fields 1-16 of IdentityDocument
func (m *IdentityDocumentManager) serializeDocumentForSigning(doc *pb.IdentityDocument) ([]byte, error) {
	data := make([]byte, 0, 256)

	// Field 1: identity_sign_pub (32 bytes)
	data = append(data, doc.IdentitySignPub...)

	// Field 2: identity_dh_pub (32 bytes)
	data = append(data, doc.IdentityDhPub...)

	// Field 3: sequence (8 bytes)
	seqBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(seqBytes, doc.Sequence)
	data = append(data, seqBytes...)

	// Field 4: previous_hash (32 bytes, may be empty)
	if len(doc.PreviousHash) > 0 {
		data = append(data, doc.PreviousHash...)
	}

	// Field 5: created_at (8 bytes)
	tsBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(tsBytes, doc.CreatedAt)
	data = append(data, tsBytes...)

	// Field 6: updated_at (8 bytes)
	updatedBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(updatedBytes, doc.UpdatedAt)
	data = append(data, updatedBytes...)

	// Field 7: devices (serialized)
	for _, device := range doc.Devices {
		deviceData, err := proto.Marshal(device)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal device: %w", err)
		}
		// Prefix with length
		lenBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBytes, uint32(len(deviceData)))
		data = append(data, lenBytes...)
		data = append(data, deviceData...)
	}

	// Field 8: signed_prekeys (serialized)
	for _, spk := range doc.SignedPrekeys {
		spkData, err := proto.Marshal(spk)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal signed prekey: %w", err)
		}
		lenBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBytes, uint32(len(spkData)))
		data = append(data, lenBytes...)
		data = append(data, spkData...)
	}

	// Field 9: one_time_prekeys (serialized)
	for _, opk := range doc.OneTimePrekeys {
		opkData, err := proto.Marshal(opk)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal one-time prekey: %w", err)
		}
		lenBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBytes, uint32(len(opkData)))
		data = append(data, lenBytes...)
		data = append(data, opkData...)
	}

	// Field 10: supported_versions (serialized)
	for _, ver := range doc.SupportedVersions {
		verBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(verBytes, ver)
		data = append(data, verBytes...)
	}

	// Field 11: supported_cipher_suites (serialized)
	for _, suite := range doc.SupportedCipherSuites {
		suiteBytes := []byte(suite)
		lenBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBytes, uint32(len(suiteBytes)))
		data = append(data, lenBytes...)
		data = append(data, suiteBytes...)
	}

	// Field 12: preferred_version (4 bytes)
	prefBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(prefBytes, doc.PreferredVersion)
	data = append(data, prefBytes...)

	// Field 13: display_name (serialized)
	displayNameBytes := []byte(doc.DisplayName)
	displayNameLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(displayNameLen, uint32(len(displayNameBytes)))
	data = append(data, displayNameLen...)
	data = append(data, displayNameBytes...)

	// Field 14: avatar_cid (serialized)
	avatarBytes := []byte(doc.AvatarCid)
	avatarLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(avatarLen, uint32(len(avatarBytes)))
	data = append(data, avatarLen...)
	data = append(data, avatarBytes...)

	// Field 15: revocations (serialized)
	for _, rev := range doc.Revocations {
		revData, err := proto.Marshal(rev)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal revocation: %w", err)
		}
		lenBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBytes, uint32(len(revData)))
		data = append(data, lenBytes...)
		data = append(data, revData...)
	}

	// Field 16: features (serialized)
	if doc.Features != nil {
		featuresData, err := proto.Marshal(doc.Features)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal features: %w", err)
		}
		featuresLen := make([]byte, 4)
		binary.LittleEndian.PutUint32(featuresLen, uint32(len(featuresData)))
		data = append(data, featuresLen...)
		data = append(data, featuresData...)
	}

	return data, nil
}

// VerifyIdentityDocument verifies an IdentityDocument signature and structure
func VerifyIdentityDocument(doc *pb.IdentityDocument) error {
	if len(doc.IdentitySignPub) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid identity public key length")
	}

	// Verify document signature
	data, err := serializeDocumentForSigning(doc)
	if err != nil {
		return fmt.Errorf("failed to serialize document: %w", err)
	}

	if !ed25519.Verify(doc.IdentitySignPub, data, doc.Signature) {
		return fmt.Errorf("invalid identity document signature")
	}

	// Verify all device certificates
	for _, device := range doc.Devices {
		if err := VerifyDeviceCertificate(device); err != nil {
			return fmt.Errorf("invalid device certificate: %w", err)
		}
	}

	// Verify all signed prekeys
	for _, spk := range doc.SignedPrekeys {
		if err := VerifySignedPrekey(spk, doc.IdentitySignPub); err != nil {
			return fmt.Errorf("invalid signed prekey: %w", err)
		}
	}

	// Verify sequence is positive
	if doc.Sequence == 0 {
		return fmt.Errorf("invalid sequence number")
	}

	// Verify timestamp is reasonable (within ±24 hours)
	now := time.Now()
	docTime := time.Unix(int64(doc.UpdatedAt), 0)
	if docTime.Before(now.Add(-24*time.Hour)) || docTime.After(now.Add(24*time.Hour)) {
		return fmt.Errorf("document timestamp out of reasonable range")
	}

	return nil
}

// serializeDocumentForSigning is a package-level function for verification
func serializeDocumentForSigning(doc *pb.IdentityDocument) ([]byte, error) {
	data := make([]byte, 0, 256)

	// Field 1: identity_sign_pub (32 bytes)
	data = append(data, doc.IdentitySignPub...)

	// Field 2: identity_dh_pub (32 bytes)
	data = append(data, doc.IdentityDhPub...)

	// Field 3: sequence (8 bytes)
	seqBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(seqBytes, doc.Sequence)
	data = append(data, seqBytes...)

	// Field 4: previous_hash (32 bytes, may be empty)
	if len(doc.PreviousHash) > 0 {
		data = append(data, doc.PreviousHash...)
	}

	// Field 5: created_at (8 bytes)
	tsBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(tsBytes, doc.CreatedAt)
	data = append(data, tsBytes...)

	// Field 6: updated_at (8 bytes)
	updatedBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(updatedBytes, doc.UpdatedAt)
	data = append(data, updatedBytes...)

	// Field 7: devices (serialized)
	for _, device := range doc.Devices {
		deviceData, err := proto.Marshal(device)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal device: %w", err)
		}
		lenBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBytes, uint32(len(deviceData)))
		data = append(data, lenBytes...)
		data = append(data, deviceData...)
	}

	// Field 8: signed_prekeys (serialized)
	for _, spk := range doc.SignedPrekeys {
		spkData, err := proto.Marshal(spk)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal signed prekey: %w", err)
		}
		lenBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBytes, uint32(len(spkData)))
		data = append(data, lenBytes...)
		data = append(data, spkData...)
	}

	// Field 9: one_time_prekeys (serialized)
	for _, opk := range doc.OneTimePrekeys {
		opkData, err := proto.Marshal(opk)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal one-time prekey: %w", err)
		}
		lenBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBytes, uint32(len(opkData)))
		data = append(data, lenBytes...)
		data = append(data, opkData...)
	}

	// Field 10: supported_versions (serialized)
	for _, ver := range doc.SupportedVersions {
		verBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(verBytes, ver)
		data = append(data, verBytes...)
	}

	// Field 11: supported_cipher_suites (serialized)
	for _, suite := range doc.SupportedCipherSuites {
		suiteBytes := []byte(suite)
		lenBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBytes, uint32(len(suiteBytes)))
		data = append(data, lenBytes...)
		data = append(data, suiteBytes...)
	}

	// Field 12: preferred_version (4 bytes)
	prefBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(prefBytes, doc.PreferredVersion)
	data = append(data, prefBytes...)

	// Field 13: display_name (serialized)
	displayNameBytes := []byte(doc.DisplayName)
	displayNameLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(displayNameLen, uint32(len(displayNameBytes)))
	data = append(data, displayNameLen...)
	data = append(data, displayNameBytes...)

	// Field 14: avatar_cid (serialized)
	avatarBytes := []byte(doc.AvatarCid)
	avatarLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(avatarLen, uint32(len(avatarBytes)))
	data = append(data, avatarLen...)
	data = append(data, avatarBytes...)

	// Field 15: revocations (serialized)
	for _, rev := range doc.Revocations {
		revData, err := proto.Marshal(rev)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal revocation: %w", err)
		}
		lenBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBytes, uint32(len(revData)))
		data = append(data, lenBytes...)
		data = append(data, revData...)
	}

	// Field 16: features (serialized)
	if doc.Features != nil {
		featuresData, err := proto.Marshal(doc.Features)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal features: %w", err)
		}
		featuresLen := make([]byte, 4)
		binary.LittleEndian.PutUint32(featuresLen, uint32(len(featuresData)))
		data = append(data, featuresLen...)
		data = append(data, featuresData...)
	}

	return data, nil
}

// ComputeDocumentHash computes SHA256 hash of a serialized IdentityDocument
func ComputeDocumentHash(doc *pb.IdentityDocument) ([]byte, error) {
	data, err := proto.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal document: %w", err)
	}
	hash := sha256.Sum256(data)
	return hash[:], nil
}
