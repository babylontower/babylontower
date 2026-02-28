package multidevice

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	pb "babylontower/pkg/proto"
	"golang.org/x/crypto/curve25519"
)

// DeviceManager handles multi-device registration and synchronization
type DeviceManager struct {
	// Identity keys (derived from mnemonic)
	identitySignPub  ed25519.PublicKey
	identitySignPriv ed25519.PrivateKey
	identityDHPub    []byte
	identityDHPriv   []byte

	// Current device keys
	deviceSignPub  ed25519.PublicKey
	deviceSignPriv ed25519.PrivateKey
	deviceDHPub    []byte
	deviceDHPriv   []byte
	deviceID       []byte // SHA256(DK_sign.pub)[:16]
	deviceName     string

	// Device group key for sync topic encryption
	deviceGroupKey []byte // 32-byte key derived from master seed
}

// DeviceConfig holds device configuration
type DeviceConfig struct {
	// IdentitySignPub is the identity Ed25519 public key
	IdentitySignPub ed25519.PublicKey
	// IdentitySignPriv is the identity Ed25519 private key
	IdentitySignPriv ed25519.PrivateKey
	// IdentityDHPub is the identity X25519 public key
	IdentityDHPub []byte
	// IdentityDHPriv is the identity X25519 private key
	IdentityDHPriv []byte
	// DeviceName is the human-readable device name
	DeviceName string
	// DeviceGroupKey is the symmetric key for device sync encryption
	DeviceGroupKey []byte
}

// NewDeviceManager creates a new device manager
func NewDeviceManager(config *DeviceConfig) *DeviceManager {
	return &DeviceManager{
		identitySignPub:  config.IdentitySignPub,
		identitySignPriv: config.IdentitySignPriv,
		identityDHPub:    config.IdentityDHPub,
		identityDHPriv:   config.IdentityDHPriv,
		deviceName:       config.DeviceName,
		deviceGroupKey:   config.DeviceGroupKey,
	}
}

// RegisterNewDevice generates random device keys and creates a DeviceCertificate
// This is called when a new device is added to an existing identity
func (dm *DeviceManager) RegisterNewDevice() (*pb.DeviceCertificate, error) {
	// Generate random Ed25519 device signing key
	deviceSignPub, deviceSignPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate device signing key: %w", err)
	}

	// Generate random X25519 device DH key
	deviceDHPriv := make([]byte, 32)
	if _, err := rand.Read(deviceDHPriv); err != nil {
		return nil, fmt.Errorf("failed to generate device DH private key: %w", err)
	}
	deviceDHPub, err := curve25519.X25519(deviceDHPriv, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("failed to compute device DH public key: %w", err)
	}

	// Compute device ID as SHA256(DK_sign.pub)[:16]
	hash := sha256.Sum256(deviceSignPub)
	deviceID := hash[:16]

	// Create device certificate
	cert := &pb.DeviceCertificate{
		DeviceId:     deviceID,
		DeviceSignPub: deviceSignPub,
		DeviceDhPub:   deviceDHPub,
		DeviceName:    dm.deviceName,
		CreatedAt:     uint64(time.Now().Unix()),
		ExpiresAt:     0, // No expiry
		IdentityPub:   dm.identitySignPub,
	}

	// Sign the certificate with identity key
	signature, err := dm.signDeviceCertificate(cert)
	if err != nil {
		return nil, fmt.Errorf("failed to sign device certificate: %w", err)
	}
	cert.Signature = signature

	// Update device manager state
	dm.deviceSignPub = deviceSignPub
	dm.deviceSignPriv = deviceSignPriv
	dm.deviceDHPub = deviceDHPub
	dm.deviceDHPriv = deviceDHPriv
	dm.deviceID = deviceID

	return cert, nil
}

// signDeviceCertificate signs a device certificate with the identity key
func (dm *DeviceManager) signDeviceCertificate(cert *pb.DeviceCertificate) ([]byte, error) {
	// Canonical serialization for signing (fields 1-7)
	data := make([]byte, 0, 16+32+32+len(cert.DeviceName)+8+8+32)
	data = append(data, cert.DeviceId...)
	data = append(data, cert.DeviceSignPub...)
	data = append(data, cert.DeviceDhPub...)
	data = append(data, []byte(cert.DeviceName)...)
	
	// Append timestamps as big-endian bytes
	tsBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		tsBytes[i] = byte(cert.CreatedAt >> (56 - i*8))
	}
	data = append(data, tsBytes...)
	
	tsBytes = make([]byte, 8)
	for i := 0; i < 8; i++ {
		tsBytes[i] = byte(cert.ExpiresAt >> (56 - i*8))
	}
	data = append(data, tsBytes...)
	
	data = append(data, cert.IdentityPub...)

	// Sign with identity key
	signature := ed25519.Sign(dm.identitySignPriv, data)
	return signature, nil
}

// VerifyDeviceCertificate verifies a device certificate signature
func VerifyDeviceCertificate(cert *pb.DeviceCertificate) error {
	// Verify signature against identity public key
	data := make([]byte, 0, 16+32+32+len(cert.DeviceName)+8+8+32)
	data = append(data, cert.DeviceId...)
	data = append(data, cert.DeviceSignPub...)
	data = append(data, cert.DeviceDhPub...)
	data = append(data, []byte(cert.DeviceName)...)
	
	tsBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		tsBytes[i] = byte(cert.CreatedAt >> (56 - i*8))
	}
	data = append(data, tsBytes...)
	
	tsBytes = make([]byte, 8)
	for i := 0; i < 8; i++ {
		tsBytes[i] = byte(cert.ExpiresAt >> (56 - i*8))
	}
	data = append(data, tsBytes...)
	
	data = append(data, cert.IdentityPub...)

	valid := ed25519.Verify(cert.IdentityPub, data, cert.Signature)
	if !valid {
		return fmt.Errorf("invalid device certificate signature")
	}

	return nil
}

// GetDeviceID returns the current device ID
func (dm *DeviceManager) GetDeviceID() []byte {
	return dm.deviceID
}

// GetDeviceSignPub returns the device signing public key
func (dm *DeviceManager) GetDeviceSignPub() ed25519.PublicKey {
	return dm.deviceSignPub
}

// GetDeviceSignPriv returns the device signing private key
func (dm *DeviceManager) GetDeviceSignPriv() ed25519.PrivateKey {
	return dm.deviceSignPriv
}

// GetDeviceDHPub returns the device DH public key
func (dm *DeviceManager) GetDeviceDHPub() []byte {
	return dm.deviceDHPub
}

// GetDeviceDHPriv returns the device DH private key
func (dm *DeviceManager) GetDeviceDHPriv() []byte {
	return dm.deviceDHPriv
}

// GetDeviceGroupKey returns the device group sync key
func (dm *DeviceManager) GetDeviceGroupKey() []byte {
	return dm.deviceGroupKey
}

// DeviceIDToHex converts a device ID to hex string
func DeviceIDToHex(deviceID []byte) string {
	return hex.EncodeToString(deviceID)
}

// DeviceIDFromHex converts a hex string to device ID
func DeviceIDFromHex(hexStr string) ([]byte, error) {
	return hex.DecodeString(hexStr)
}

// ComputeDeviceID computes device ID from a device signing public key
func ComputeDeviceID(deviceSignPub ed25519.PublicKey) []byte {
	hash := sha256.Sum256(deviceSignPub)
	return hash[:16]
}
