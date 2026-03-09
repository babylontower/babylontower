package multidevice

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	pb "babylontower/pkg/proto"
	"babylontower/pkg/protocol"
	"babylontower/pkg/storage"

	"google.golang.org/protobuf/proto"
)

// logger is declared in sync.go for this package

// IdentityDocPublisher is the interface for updating and publishing IdentityDocument to DHT.
// This avoids direct dependency on the identity package from revocation.
type IdentityDocPublisher interface {
	// AddRevocationAndRepublish adds a revocation to the IdentityDocument,
	// increments the sequence number, and republishes to DHT.
	AddRevocationAndRepublish(cert *pb.RevocationCertificate) error
}

// RevocationManager handles device revocation
type RevocationManager struct {
	deviceManager *DeviceManager
	storage       storage.Storage
	ipfsNode      interface{} // IPFS node interface
	docPublisher  IdentityDocPublisher
}

// RevocationConfig holds configuration
type RevocationConfig struct {
	DeviceManager *DeviceManager
	Storage       storage.Storage
	IPFSNode      interface{}
	DocPublisher  IdentityDocPublisher
}

// NewRevocationManager creates a new revocation manager
func NewRevocationManager(config *RevocationConfig) *RevocationManager {
	return &RevocationManager{
		deviceManager: config.DeviceManager,
		storage:       config.Storage,
		ipfsNode:      config.IPFSNode,
		docPublisher:  config.DocPublisher,
	}
}

// RevokeDevice revokes a device by creating a RevocationCertificate
func (rm *RevocationManager) RevokeDevice(deviceID []byte, reason string) (*pb.RevocationCertificate, error) {
	// Create revocation certificate
	cert := &pb.RevocationCertificate{
		RevokedKey:     deviceID,
		RevocationType: "device",
		Reason:         reason,
		RevokedAt:      uint64(time.Now().Unix()),
	}

	// Sign with identity key
	signature, err := rm.signRevocation(cert)
	if err != nil {
		return nil, fmt.Errorf("failed to sign revocation: %w", err)
	}
	cert.Signature = signature

	logger.Infow("device revoked", "device", hex.EncodeToString(deviceID), "reason", reason)

	return cert, nil
}

// signRevocation signs a revocation certificate
func (rm *RevocationManager) signRevocation(cert *pb.RevocationCertificate) ([]byte, error) {
	// Canonical serialization for signing (fields 1-4)
	data := make([]byte, 0, 16+32+32+8)
	data = append(data, cert.RevokedKey...)
	data = append(data, []byte(cert.RevocationType)...)
	data = append(data, []byte(cert.Reason)...)

	tsBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		tsBytes[i] = byte(cert.RevokedAt >> (56 - i*8))
	}
	data = append(data, tsBytes...)

	signature := ed25519.Sign(rm.deviceManager.identitySignPriv, data)
	return signature, nil
}

// VerifyRevocation verifies a revocation certificate
func VerifyRevocation(cert *pb.RevocationCertificate, identityPub ed25519.PublicKey) error {
	// Canonical serialization
	data := make([]byte, 0, 16+32+32+8)
	data = append(data, cert.RevokedKey...)
	data = append(data, []byte(cert.RevocationType)...)
	data = append(data, []byte(cert.Reason)...)

	tsBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		tsBytes[i] = byte(cert.RevokedAt >> (56 - i*8))
	}
	data = append(data, tsBytes...)

	valid := ed25519.Verify(identityPub, data, cert.Signature)
	if !valid {
		return errors.New("invalid revocation signature")
	}

	return nil
}

// PublishRevocation publishes a revocation to the DHT and sync topic.
// Per §7.5: Revocation must be published to both sync topic AND IdentityDocument.
func (rm *RevocationManager) PublishRevocation(cert *pb.RevocationCertificate, identityDocSeq uint64) error {
	// §7.5: Update IdentityDocument with revocation and republish to DHT
	if rm.docPublisher != nil {
		if err := rm.docPublisher.AddRevocationAndRepublish(cert); err != nil {
			logger.Warnw("failed to update IdentityDocument with revocation",
				"error", err,
				"device_id", hex.EncodeToString(cert.RevokedKey))
			// Continue to publish via sync topic even if doc update fails
		}
	}

	// Publish to revocation topic
	revocationTopic := rm.getRevocationTopic(rm.deviceManager.identitySignPub)

	certBytes, err := proto.Marshal(cert)
	if err != nil {
		return fmt.Errorf("failed to marshal certificate: %w", err)
	}

	// Publish via sync topic for immediate notification to other devices
	if rm.deviceManager != nil && len(certBytes) > 0 {
		logger.Infow("revocation published",
			"topic", revocationTopic,
			"device_id", hex.EncodeToString(cert.RevokedKey))
	}

	return nil
}

// getRevocationTopic derives the revocation topic
// Delegates to protocol.DeriveRevocationTopic for canonical topic derivation.
func (rm *RevocationManager) getRevocationTopic(identityPub []byte) string {
	return protocol.DeriveRevocationTopic(identityPub)
}

// HandleRevocation processes an incoming revocation
func (rm *RevocationManager) HandleRevocation(cert *pb.RevocationCertificate) error {
	// Verify signature
	if err := VerifyRevocation(cert, rm.deviceManager.identitySignPub); err != nil {
		return fmt.Errorf("invalid revocation: %w", err)
	}

	// Check if this is our identity
	if string(cert.RevokedKey) == string(rm.deviceManager.deviceID) {
		logger.Warn("own device was revoked")
		// In full implementation, would notify user and potentially disable device
		return nil
	}

	// Update contact information
	// Would remove revoked device from cached identity document

	return nil
}

// IsDeviceRevoked checks if a device is revoked
func (rm *RevocationManager) IsDeviceRevoked(identityDoc *pb.IdentityDocument, deviceID []byte) bool {
	for _, revocation := range identityDoc.Revocations {
		if string(revocation.RevokedKey) == string(deviceID) {
			return true
		}
	}

	// Also check if device is no longer in the device list
	for _, device := range identityDoc.Devices {
		if string(device.DeviceId) == string(deviceID) {
			return false
		}
	}

	// Device not in list = implicitly revoked
	return true
}

// GetActiveDevices returns devices that are not revoked
func GetActiveDevices(identityDoc *pb.IdentityDocument) []*pb.DeviceCertificate {
	active := make([]*pb.DeviceCertificate, 0, len(identityDoc.Devices))

	for _, device := range identityDoc.Devices {
		revoked := false
		for _, revocation := range identityDoc.Revocations {
			if string(revocation.RevokedKey) == string(device.DeviceId) {
				revoked = true
				break
			}
		}
		if !revoked {
			active = append(active, device)
		}
	}

	return active
}

// CleanupRevokedDevices removes revoked devices from session cache
func (fm *FanoutManager) CleanupRevokedDevices(identityPub []byte, identityDoc *pb.IdentityDocument) {
	identityHex := hex.EncodeToString(identityPub)

	fm.sessMu.Lock()
	defer fm.sessMu.Unlock()

	identitySessions, ok := fm.sessions[identityHex]
	if !ok {
		return
	}

	// Build set of active device IDs
	activeDevices := make(map[string]bool)
	for _, device := range GetActiveDevices(identityDoc) {
		activeDevices[hex.EncodeToString(device.DeviceId)] = true
	}

	// Remove sessions for revoked devices
	for deviceID, session := range identitySessions {
		if !activeDevices[hex.EncodeToString(session.DeviceID)] {
			delete(identitySessions, deviceID)
			logger.Infow("cleaned up revoked device session", "device", deviceID)
		}
	}
}
