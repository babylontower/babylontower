package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	pb "babylontower/pkg/proto"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// Identity v1 derivation constants
const (
	// Master key derivation
	MasterKeySalt  = "bt-master-key"
	MasterKeyInfo  = "babylon-tower-v1"
	IdentitySalt   = "bt-identity"
	SigningKeyInfo = "identity-signing-key-0"
	DHKeyInfo      = "identity-dh-key-0"

	// PoC compatibility derivation
	PoCEd25519Salt = "ed25519-derive"
	PoCX25519Salt  = "x25519-derive"
	PoCIndexInfo   = "index-0"

	// Key sizes
	DeviceIDSize = 16 // SHA256(DK_sign.pub)[:16]

	// Prekey management
	OPKTargetCount   = 100 // Target number of one-time prekeys
	OPKReplenishThreshold = 20  // Generate more when below this
	OPKBatchSize       = 80   // Number to generate in a batch
	SPKRotationDays    = 7    // SPK rotation interval
	SPKOverlapHours    = 24   // Overlap period for SPK rotation
)

// IdentityV1 represents a v1 identity with master keys and device keys
type IdentityV1 struct {
	// Master identity keys (derived from mnemonic)
	IKSignPub  ed25519.PublicKey
	IKSignPriv ed25519.PrivateKey
	IKDHPriv   *[32]byte
	IKDHPub    *[32]byte

	// Device keys (random, not derived from mnemonic)
	DeviceID      []byte
	DKSignPub     ed25519.PublicKey
	DKSignPriv    ed25519.PrivateKey
	DKDHPriv      *[32]byte
	DKDHPub       *[32]byte

	// Device metadata
	DeviceName  string
	CreatedAt   time.Time
	ExpiresAt   time.Time // 0 = no expiry

	// Mnemonic (for recovery)
	Mnemonic string
}

// DeriveMasterSecret derives the master secret from BIP39 seed using HKDF
// This is the v1 derivation method with master secret intermediate step
func DeriveMasterSecret(seed []byte) ([]byte, error) {
	if len(seed) != SeedLength {
		return nil, fmt.Errorf("invalid seed length: %d, expected %d", len(seed), SeedLength)
	}

	hkdfReader := hkdf.New(sha256.New, seed, []byte(MasterKeySalt), []byte(MasterKeyInfo))
	masterSecret := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, masterSecret); err != nil {
		return nil, fmt.Errorf("failed to derive master secret: %w", err)
	}

	return masterSecret, nil
}

// DeriveIdentityKeysV1 derives Ed25519 and X25519 identity keys from master secret
// This is the v1 derivation method (different from PoC)
func DeriveIdentityKeysV1(masterSecret []byte) (
	ed25519.PublicKey, ed25519.PrivateKey,
	*[32]byte, *[32]byte,
	error,
) {
	// Derive Ed25519 signing key
	edHKDF := hkdf.New(sha256.New, masterSecret, []byte(IdentitySalt), []byte(SigningKeyInfo))
	edSeed := make([]byte, ed25519.SeedSize)
	if _, err := io.ReadFull(edHKDF, edSeed); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to derive Ed25519 seed: %w", err)
	}
	edPriv := ed25519.NewKeyFromSeed(edSeed)
	edPub := edPriv.Public().(ed25519.PublicKey)

	// Derive X25519 DH key
	dhHKDF := hkdf.New(sha256.New, masterSecret, []byte(IdentitySalt), []byte(DHKeyInfo))
	dhSeed := make([]byte, 32)
	if _, err := io.ReadFull(dhHKDF, dhSeed); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to derive X25519 seed: %w", err)
	}

	var dhPriv [32]byte
	var dhPub [32]byte
	copy(dhPriv[:], dhSeed)
	dhResult, err := curve25519.X25519(dhPriv[:], curve25519.Basepoint)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("X25519 key derivation failed: %w", err)
	}
	copy(dhPub[:], dhResult)

	return edPub, edPriv, &dhPub, &dhPriv, nil
}

// DeriveIdentityKeysPoC derives keys using the PoC method for backward compatibility
func DeriveIdentityKeysPoC(seed []byte) (
	ed25519.PublicKey, ed25519.PrivateKey,
	*[32]byte, *[32]byte,
	error,
) {
	// PoC Ed25519 derivation
	edHKDF := hkdf.New(sha256.New, seed, []byte(PoCEd25519Salt), []byte(PoCIndexInfo))
	edSeed := make([]byte, ed25519.SeedSize)
	if _, err := io.ReadFull(edHKDF, edSeed); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to derive Ed25519 seed (PoC): %w", err)
	}
	edPriv := ed25519.NewKeyFromSeed(edSeed)
	edPub := edPriv.Public().(ed25519.PublicKey)

	// PoC X25519 derivation
	dhHKDF := hkdf.New(sha256.New, seed, []byte(PoCX25519Salt), []byte(PoCIndexInfo))
	dhSeed := make([]byte, 32)
	if _, err := io.ReadFull(dhHKDF, dhSeed); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to derive X25519 seed (PoC): %w", err)
	}

	var dhPriv [32]byte
	var dhPub [32]byte
	copy(dhPriv[:], dhSeed)
	dhResult, err := curve25519.X25519(dhPriv[:], curve25519.Basepoint)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("X25519 key derivation failed (PoC): %w", err)
	}
	copy(dhPub[:], dhResult)

	return edPub, edPriv, &dhPub, &dhPriv, nil
}

// GenerateDeviceKeys generates random device keys (not derived from mnemonic)
func GenerateDeviceKeys(deviceName string) (
	deviceID []byte,
	dkSignPub ed25519.PublicKey,
	dkSignPriv ed25519.PrivateKey,
	dkDHPub *[32]byte,
	dkDHPriv *[32]byte,
	err error,
) {
	// Generate random Ed25519 device signing key
	var genErr error
	_, dkSignPriv, genErr = ed25519.GenerateKey(rand.Reader)
	if genErr != nil {
		err = fmt.Errorf("failed to generate device Ed25519 key: %w", genErr)
		return
	}
	dkSignPub = dkSignPriv.Public().(ed25519.PublicKey)

	// Generate random X25519 device DH key
	dkDHPriv = new([32]byte)
	dkDHPub = new([32]byte)
	if _, genErr := io.ReadFull(rand.Reader, dkDHPriv[:]); genErr != nil {
		err = fmt.Errorf("failed to generate random X25519 seed: %w", genErr)
		return
	}
	dhResult, genErr := curve25519.X25519(dkDHPriv[:], curve25519.Basepoint)
	if genErr != nil {
		err = fmt.Errorf("X25519 device key derivation failed: %w", genErr)
		return
	}
	copy(dkDHPub[:], dhResult)

	// Derive device ID from DK_sign.pub
	deviceID = DeriveDeviceID(dkSignPub)

	return deviceID, dkSignPub, dkSignPriv, dkDHPub, dkDHPriv, nil
}

// DeriveDeviceID computes SHA256(DK_sign.pub)[:16]
func DeriveDeviceID(dkSignPub ed25519.PublicKey) []byte {
	hash := sha256.Sum256(dkSignPub)
	return hash[:DeviceIDSize]
}

// NewIdentityV1 creates a new v1 identity from a mnemonic
// Generates random device keys and signs the device certificate
func NewIdentityV1(mnemonic string, deviceName string) (*IdentityV1, error) {
	// Derive seed from mnemonic
	seed, err := DeriveSeed(mnemonic)
	if err != nil {
		return nil, fmt.Errorf("failed to derive seed: %w", err)
	}

	// Derive master secret (v1 method)
	masterSecret, err := DeriveMasterSecret(seed)
	if err != nil {
		return nil, fmt.Errorf("failed to derive master secret: %w", err)
	}

	// Derive identity keys (v1 method)
	ikSignPub, ikSignPriv, ikDHPub, ikDHPriv, err := DeriveIdentityKeysV1(masterSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to derive identity keys: %w", err)
	}

	// Generate random device keys
	deviceID, dkSignPub, dkSignPriv, dkDHPub, dkDHPriv, err := GenerateDeviceKeys(deviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to generate device keys: %w", err)
	}

	return &IdentityV1{
		// Identity keys
		IKSignPub:  ikSignPub,
		IKSignPriv: ikSignPriv,
		IKDHPriv:   ikDHPriv,
		IKDHPub:    ikDHPub,

		// Device keys
		DeviceID:   deviceID,
		DKSignPub:  dkSignPub,
		DKSignPriv: dkSignPriv,
		DKDHPriv:   dkDHPriv,
		DKDHPub:    dkDHPub,

		// Metadata
		DeviceName: deviceName,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Time{}, // No expiry

		// Recovery
		Mnemonic: mnemonic,
	}, nil
}

// IdentityFingerprint computes Base58(SHA256(IK_sign.pub ‖ IK_dh.pub)[:20])
func (i *IdentityV1) IdentityFingerprint() string {
	// Concatenate public keys
	combined := make([]byte, 0, 64)
	combined = append(combined, i.IKSignPub...)
	combined = append(combined, i.IKDHPub[:]...)

	// Hash and truncate
	hash := sha256.Sum256(combined)
	fingerprint := hash[:20]

	// Encode as base58
	return EncodeBase58(fingerprint)
}

// CreateDeviceCertificate creates a signed DeviceCertificate for this device
func (i *IdentityV1) CreateDeviceCertificate() (*pb.DeviceCertificate, error) {
	expiresAt := uint64(0)
	if !i.ExpiresAt.IsZero() {
		expiresAt = uint64(i.ExpiresAt.Unix())
	}

	cert := &pb.DeviceCertificate{
		DeviceId:     i.DeviceID,
		DeviceSignPub: i.DKSignPub,
		DeviceDhPub:  i.DKDHPub[:],
		DeviceName:   i.DeviceName,
		CreatedAt:    uint64(i.CreatedAt.Unix()),
		ExpiresAt:    expiresAt,
		IdentityPub:  i.IKSignPub,
	}

	// Sign the certificate fields (1-7)
	signature, err := SignDeviceCertificate(i.IKSignPriv, cert)
	if err != nil {
		return nil, fmt.Errorf("failed to sign device certificate: %w", err)
	}
	cert.Signature = signature

	return cert, nil
}

// SignDeviceCertificate signs a DeviceCertificate with the identity key
func SignDeviceCertificate(ikSignPriv ed25519.PrivateKey, cert *pb.DeviceCertificate) ([]byte, error) {
	// Canonical serialization for signing (fields 1-7)
	data := make([]byte, 0, 16+32+32+len(cert.DeviceName)+8+8+32)
	data = append(data, cert.DeviceId...)
	data = append(data, cert.DeviceSignPub...)
	data = append(data, cert.DeviceDhPub...)
	data = append(data, []byte(cert.DeviceName)...)
	tsBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(tsBytes, cert.CreatedAt)
	data = append(data, tsBytes...)
	expBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(expBytes, cert.ExpiresAt)
	data = append(data, expBytes...)
	data = append(data, cert.IdentityPub...)

	signature := ed25519.Sign(ikSignPriv, data)
	return signature, nil
}

// VerifyDeviceCertificate verifies a DeviceCertificate signature
func VerifyDeviceCertificate(cert *pb.DeviceCertificate) error {
	if len(cert.IdentityPub) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid identity public key length")
	}

	// Reconstruct the signed data
	data := make([]byte, 0, 16+32+32+len(cert.DeviceName)+8+8+32)
	data = append(data, cert.DeviceId...)
	data = append(data, cert.DeviceSignPub...)
	data = append(data, cert.DeviceDhPub...)
	data = append(data, []byte(cert.DeviceName)...)
	tsBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(tsBytes, cert.CreatedAt)
	data = append(data, tsBytes...)
	expBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(expBytes, cert.ExpiresAt)
	data = append(data, expBytes...)
	data = append(data, cert.IdentityPub...)

	if !ed25519.Verify(cert.IdentityPub, data, cert.Signature) {
		return fmt.Errorf("invalid device certificate signature")
	}

	return nil
}

// GenerateSignedPrekey generates a new signed prekey
func (i *IdentityV1) GenerateSignedPrekey(prekeyID uint64) (*pb.SignedPrekey, error) {
	// Generate random X25519 key pair
	spPriv := new([32]byte)
	spPub := new([32]byte)
	if _, err := io.ReadFull(rand.Reader, spPriv[:]); err != nil {
		return nil, fmt.Errorf("failed to generate random SPK seed: %w", err)
	}
	dhResult, err := curve25519.X25519(spPriv[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("X25519 SPK derivation failed: %w", err)
	}
	copy(spPub[:], dhResult)

	now := time.Now()
	expiresAt := now.AddDate(0, 0, SPKRotationDays)

	spk := &pb.SignedPrekey{
		DeviceId:  i.DeviceID,
		PrekeyPub: spPub[:],
		PrekeyId:  prekeyID,
		CreatedAt: uint64(now.Unix()),
		ExpiresAt: uint64(expiresAt.Unix()),
	}

	// Sign the prekey
	signature, err := SignSignedPrekey(i.IKSignPriv, spk)
	if err != nil {
		return nil, fmt.Errorf("failed to sign SPK: %w", err)
	}
	spk.Signature = signature

	return spk, nil
}

// SignSignedPrekey signs a SignedPrekey with the identity key
func SignSignedPrekey(ikSignPriv ed25519.PrivateKey, spk *pb.SignedPrekey) ([]byte, error) {
	// Canonical serialization for signing (fields 1-5)
	data := make([]byte, 0, 16+32+8+8+8)
	data = append(data, spk.DeviceId...)
	data = append(data, spk.PrekeyPub...)
	idBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(idBytes, spk.PrekeyId)
	data = append(data, idBytes...)
	tsBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(tsBytes, spk.CreatedAt)
	data = append(data, tsBytes...)
	expBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(expBytes, spk.ExpiresAt)
	data = append(data, expBytes...)

	signature := ed25519.Sign(ikSignPriv, data)
	return signature, nil
}

// VerifySignedPrekey verifies a SignedPrekey signature
func VerifySignedPrekey(spk *pb.SignedPrekey, identityPub ed25519.PublicKey) error {
	if len(identityPub) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid identity public key length")
	}

	// Reconstruct the signed data
	data := make([]byte, 0, 16+32+8+8+8)
	data = append(data, spk.DeviceId...)
	data = append(data, spk.PrekeyPub...)
	idBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(idBytes, spk.PrekeyId)
	data = append(data, idBytes...)
	tsBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(tsBytes, spk.CreatedAt)
	data = append(data, tsBytes...)
	expBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(expBytes, spk.ExpiresAt)
	data = append(data, expBytes...)

	if !ed25519.Verify(identityPub, data, spk.Signature) {
		return fmt.Errorf("invalid signed prekey signature")
	}

	return nil
}

// GenerateOneTimePrekeys generates a batch of one-time prekeys
func (i *IdentityV1) GenerateOneTimePrekeys(startID uint64, count int) ([]*pb.OneTimePrekey, error) {
	opks := make([]*pb.OneTimePrekey, count)

	for idx := 0; idx < count; idx++ {
		// Generate random X25519 key
		opPriv := new([32]byte)
		opPub := new([32]byte)
		if _, err := io.ReadFull(rand.Reader, opPriv[:]); err != nil {
			return nil, fmt.Errorf("failed to generate random OPK seed: %w", err)
		}
		dhResult, err := curve25519.X25519(opPriv[:], curve25519.Basepoint)
		if err != nil {
			return nil, fmt.Errorf("X25519 OPK derivation failed: %w", err)
		}
		copy(opPub[:], dhResult)

		opks[idx] = &pb.OneTimePrekey{
			DeviceId:  i.DeviceID,
			PrekeyPub: opPub[:],
			PrekeyId:  startID + uint64(idx),
		}
	}

	return opks, nil
}

// ShouldReplenishOPKs checks if OPK count is below threshold
func ShouldReplenishOPKs(currentCount int) bool {
	return currentCount < OPKReplenishThreshold
}

// SPKNeedsRotation checks if a signed prekey needs rotation
func SPKNeedsRotation(spk *pb.SignedPrekey) bool {
	if spk == nil {
		return true
	}

	now := time.Now()
	expiresAt := time.Unix(int64(spk.ExpiresAt), 0)

	// Rotate if expired or within overlap period
	overlapStart := expiresAt.Add(-time.Duration(SPKOverlapHours) * time.Hour)
	return now.After(overlapStart)
}
