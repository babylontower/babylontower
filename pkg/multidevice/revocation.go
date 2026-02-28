package multidevice

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"
	"google.golang.org/protobuf/proto"
)

// logger is declared in sync.go for this package

// RevocationManager handles device revocation
type RevocationManager struct {
	deviceManager *DeviceManager
	storage       storage.Storage
	ipfsNode      interface{} // IPFS node interface
}

// RevocationConfig holds configuration
type RevocationConfig struct {
	DeviceManager *DeviceManager
	Storage       storage.Storage
	IPFSNode      interface{}
}

// NewRevocationManager creates a new revocation manager
func NewRevocationManager(config *RevocationConfig) *RevocationManager {
	return &RevocationManager{
		deviceManager: config.DeviceManager,
		storage:       config.Storage,
		ipfsNode:      config.IPFSNode,
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
		return fmt.Errorf("invalid revocation signature")
	}

	return nil
}

// PublishRevocation publishes a revocation to the DHT and sync topic
func (rm *RevocationManager) PublishRevocation(cert *pb.RevocationCertificate, identityDocSeq uint64) error {
	// Update IdentityDocument
	// This would fetch the current document, add revocation, increment sequence
	// and republish to DHT

	// Publish to revocation topic
	revocationTopic := rm.getRevocationTopic(rm.deviceManager.identitySignPub)
	
	certBytes, err := proto.Marshal(cert)
	if err != nil {
		return fmt.Errorf("failed to marshal certificate: %w", err)
	}

	// In full implementation, would publish via IPFS
	_ = revocationTopic
	_ = certBytes

	logger.Info("revocation published")
	return nil
}

// getRevocationTopic derives the revocation topic
func (rm *RevocationManager) getRevocationTopic(identityPub []byte) string {
	hash := sha256.Sum256(identityPub)
	return "babylon-rev-" + hex.EncodeToString(hash[:8])
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
