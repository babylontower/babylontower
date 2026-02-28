package identity

import (
	"crypto/ed25519"
	"testing"
	"time"

	pb "babylontower/pkg/proto"
	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/curve25519"
)

// testMnemonic is a fixed mnemonic for testing
const testMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

// generateTestIdentity creates a test identity from a fixed mnemonic
func generateTestIdentity(t *testing.T) *IdentityV1 {
	return generateTestIdentityFromMnemonic(t, testMnemonic)
}

// generateTestIdentityFromMnemonic creates a test identity from a specific mnemonic
func generateTestIdentityFromMnemonic(t *testing.T, mnemonic string) *IdentityV1 {
	identity, err := NewIdentityV1(mnemonic, "Test Device")
	if err != nil {
		t.Fatalf("Failed to create test identity: %v", err)
	}
	return identity
}

// TestDeriveMasterSecret tests master secret derivation from BIP39 seed
func TestDeriveMasterSecret(t *testing.T) {
	// Generate valid mnemonic
	entropy, _ := bip39.NewEntropy(128)
	mnemonic, _ := bip39.NewMnemonic(entropy)

	seed := bip39.NewSeed(mnemonic, "")

	// Test valid seed
	masterSecret, err := DeriveMasterSecret(seed)
	if err != nil {
		t.Fatalf("Failed to derive master secret: %v", err)
	}
	if len(masterSecret) != 32 {
		t.Errorf("Expected master secret length 32, got %d", len(masterSecret))
	}

	// Test invalid seed length
	_, err = DeriveMasterSecret([]byte("too short"))
	if err == nil {
		t.Error("Expected error for invalid seed length, got nil")
	}
}

// TestDeriveIdentityKeysV1 tests v1 identity key derivation
func TestDeriveIdentityKeysV1(t *testing.T) {
	entropy, _ := bip39.NewEntropy(128)
	mnemonic, _ := bip39.NewMnemonic(entropy)
	seed := bip39.NewSeed(mnemonic, "")
	masterSecret, _ := DeriveMasterSecret(seed)

	edPub, edPriv, dhPub, dhPriv, err := DeriveIdentityKeysV1(masterSecret)
	if err != nil {
		t.Fatalf("Failed to derive identity keys: %v", err)
	}

	// Verify key lengths
	if len(edPub) != ed25519.PublicKeySize {
		t.Errorf("Expected Ed25519 public key length %d, got %d", ed25519.PublicKeySize, len(edPub))
	}
	if len(edPriv) != ed25519.PrivateKeySize {
		t.Errorf("Expected Ed25519 private key length %d, got %d", ed25519.PrivateKeySize, len(edPriv))
	}
	if len(dhPub[:]) != 32 {
		t.Errorf("Expected X25519 public key length 32, got %d", len(dhPub[:]))
	}
	if len(dhPriv[:]) != 32 {
		t.Errorf("Expected X25519 private key length 32, got %d", len(dhPriv[:]))
	}

	// Verify X25519 key pair consistency
	computedPub, err := curve25519.X25519(dhPriv[:], curve25519.Basepoint)
	if err != nil {
		t.Fatalf("X25519 public key computation failed: %v", err)
	}
	for i := range computedPub {
		if computedPub[i] != dhPub[i] {
			t.Errorf("X25519 public key mismatch at byte %d", i)
		}
	}
}

// TestDeriveIdentityKeysPoC tests PoC compatibility mode
func TestDeriveIdentityKeysPoC(t *testing.T) {
	entropy, _ := bip39.NewEntropy(128)
	mnemonic, _ := bip39.NewMnemonic(entropy)
	seed := bip39.NewSeed(mnemonic, "")

	edPub, _, dhPub, _, err := DeriveIdentityKeysPoC(seed)
	if err != nil {
		t.Fatalf("Failed to derive PoC identity keys: %v", err)
	}

	// Verify key lengths
	if len(edPub) != ed25519.PublicKeySize {
		t.Errorf("Expected Ed25519 public key length %d, got %d", ed25519.PublicKeySize, len(edPub))
	}
	if len(dhPub[:]) != 32 {
		t.Errorf("Expected X25519 public key length 32, got %d", len(dhPub[:]))
	}
}

// TestGenerateDeviceKeys tests random device key generation
func TestGenerateDeviceKeys(t *testing.T) {
	deviceID, dkSignPub, dkSignPriv, dkDHPub, dkDHPriv, err := GenerateDeviceKeys("Test Device")
	if err != nil {
		t.Fatalf("Failed to generate device keys: %v", err)
	}

	// Verify device ID length
	if len(deviceID) != DeviceIDSize {
		t.Errorf("Expected device ID length %d, got %d", DeviceIDSize, len(deviceID))
	}

	// Verify key lengths
	if len(dkSignPub) != ed25519.PublicKeySize {
		t.Errorf("Expected device Ed25519 public key length %d, got %d", ed25519.PublicKeySize, len(dkSignPub))
	}
	if len(dkSignPriv) != ed25519.PrivateKeySize {
		t.Errorf("Expected device Ed25519 private key length %d, got %d", ed25519.PrivateKeySize, len(dkSignPriv))
	}
	if len(dkDHPub[:]) != 32 {
		t.Errorf("Expected device X25519 public key length 32, got %d", len(dkDHPub[:]))
	}
	if len(dkDHPriv[:]) != 32 {
		t.Errorf("Expected device X25519 private key length 32, got %d", len(dkDHPriv[:]))
	}

	// Verify device ID derivation
	expectedDeviceID := DeriveDeviceID(dkSignPub)
	for i := range expectedDeviceID {
		if expectedDeviceID[i] != deviceID[i] {
			t.Errorf("Device ID mismatch at byte %d", i)
		}
	}
}

// TestDeriveDeviceID tests device ID derivation
func TestDeriveDeviceID(t *testing.T) {
	_, dkSignPub, _, _, _, _ := GenerateDeviceKeys("Test")
	deviceID := DeriveDeviceID(dkSignPub)

	if len(deviceID) != DeviceIDSize {
		t.Errorf("Expected device ID length %d, got %d", DeviceIDSize, len(deviceID))
	}

	// Verify determinism
	deviceID2 := DeriveDeviceID(dkSignPub)
	for i := range deviceID {
		if deviceID[i] != deviceID2[i] {
			t.Errorf("Device ID not deterministic at byte %d", i)
		}
	}
}

// TestIdentityV1 tests full identity v1 creation
func TestNewIdentityV1(t *testing.T) {
	identity, err := NewIdentityV1(testMnemonic, "Test Device")
	if err != nil {
		t.Fatalf("Failed to create identity v1: %v", err)
	}

	// Verify identity keys are set
	if identity.IKSignPub == nil {
		t.Error("Identity signing public key not set")
	}
	if identity.IKDHPub == nil {
		t.Error("Identity DH public key not set")
	}

	// Verify device keys are set
	if identity.DKSignPub == nil {
		t.Error("Device signing public key not set")
	}
	if identity.DKDHPub == nil {
		t.Error("Device DH public key not set")
	}
	if identity.DeviceID == nil {
		t.Error("Device ID not set")
	}

	// Verify mnemonic is stored
	if identity.Mnemonic != testMnemonic {
		t.Error("Mnemonic not stored correctly")
	}
}

// TestIdentityFingerprint tests fingerprint generation
func TestIdentityFingerprint(t *testing.T) {
	identity := generateTestIdentity(t)
	fingerprint := identity.IdentityFingerprint()

	if fingerprint == "" {
		t.Error("Fingerprint is empty")
	}

	// Verify fingerprint length (should be 27-28 characters for 20 bytes base58)
	if len(fingerprint) < 25 || len(fingerprint) > 30 {
		t.Errorf("Unexpected fingerprint length: %d (expected ~27-28)", len(fingerprint))
	}

	// Verify determinism
	fingerprint2 := identity.IdentityFingerprint()
	if fingerprint != fingerprint2 {
		t.Error("Fingerprint not deterministic")
	}
}

// TestCreateDeviceCertificate tests device certificate creation and verification
func TestCreateDeviceCertificate(t *testing.T) {
	identity := generateTestIdentity(t)

	cert, err := identity.CreateDeviceCertificate()
	if err != nil {
		t.Fatalf("Failed to create device certificate: %v", err)
	}

	// Verify certificate fields
	if len(cert.DeviceId) != DeviceIDSize {
		t.Errorf("Expected device ID length %d, got %d", DeviceIDSize, len(cert.DeviceId))
	}
	if len(cert.DeviceSignPub) != ed25519.PublicKeySize {
		t.Errorf("Expected device sign pub length %d, got %d", ed25519.PublicKeySize, len(cert.DeviceSignPub))
	}
	if len(cert.Signature) != ed25519.SignatureSize {
		t.Errorf("Expected signature length %d, got %d", ed25519.SignatureSize, len(cert.Signature))
	}
	if cert.DeviceName != "Test Device" {
		t.Errorf("Expected device name 'Test Device', got '%s'", cert.DeviceName)
	}

	// Verify signature
	err = VerifyDeviceCertificate(cert)
	if err != nil {
		t.Errorf("Device certificate verification failed: %v", err)
	}
}

// TestSignDeviceCertificate tests certificate signing and verification
func TestSignDeviceCertificate(t *testing.T) {
	identity := generateTestIdentity(t)
	cert, _ := identity.CreateDeviceCertificate()

	// Verify with correct key
	err := VerifyDeviceCertificate(cert)
	if err != nil {
		t.Errorf("Valid certificate verification failed: %v", err)
	}

	// Tamper with certificate and verify failure
	originalName := cert.DeviceName
	cert.DeviceName = "Tampered Device"
	err = VerifyDeviceCertificate(cert)
	if err == nil {
		t.Error("Expected verification failure for tampered certificate")
	}
	cert.DeviceName = originalName

	// Verify with wrong identity key
	wrongIdentity := generateTestIdentityFromMnemonic(t, testMnemonic2)
	cert2, _ := wrongIdentity.CreateDeviceCertificate()
	cert2.IdentityPub = identity.IKSignPub // Use wrong identity pub
	err = VerifyDeviceCertificate(cert2)
	if err == nil {
		t.Error("Expected verification failure for wrong identity key")
	}
}

// TestGenerateSignedPrekey tests SPK generation and verification
func TestGenerateSignedPrekey(t *testing.T) {
	identity := generateTestIdentity(t)

	spk, err := identity.GenerateSignedPrekey(1)
	if err != nil {
		t.Fatalf("Failed to generate signed prekey: %v", err)
	}

	// Verify SPK fields
	if len(spk.DeviceId) != DeviceIDSize {
		t.Errorf("Expected device ID length %d, got %d", DeviceIDSize, len(spk.DeviceId))
	}
	if len(spk.PrekeyPub) != 32 {
		t.Errorf("Expected prekey pub length 32, got %d", len(spk.PrekeyPub))
	}
	if spk.PrekeyId != 1 {
		t.Errorf("Expected prekey ID 1, got %d", spk.PrekeyId)
	}
	if len(spk.Signature) != ed25519.SignatureSize {
		t.Errorf("Expected signature length %d, got %d", ed25519.SignatureSize, len(spk.Signature))
	}

	// Verify signature
	err = VerifySignedPrekey(spk, identity.IKSignPub)
	if err != nil {
		t.Errorf("Signed prekey verification failed: %v", err)
	}
}

// TestGenerateOneTimePrekeys tests OPK batch generation
func TestGenerateOneTimePrekeys(t *testing.T) {
	identity := generateTestIdentity(t)

	opks, err := identity.GenerateOneTimePrekeys(100, 10)
	if err != nil {
		t.Fatalf("Failed to generate one-time prekeys: %v", err)
	}

	if len(opks) != 10 {
		t.Errorf("Expected 10 OPKs, got %d", len(opks))
	}

	// Verify OPK fields
	for i, opk := range opks {
		if len(opk.DeviceId) != DeviceIDSize {
			t.Errorf("OPK %d: Expected device ID length %d, got %d", i, DeviceIDSize, len(opk.DeviceId))
		}
		if len(opk.PrekeyPub) != 32 {
			t.Errorf("OPK %d: Expected prekey pub length 32, got %d", i, len(opk.PrekeyPub))
		}
		if opk.PrekeyId != uint64(100+i) {
			t.Errorf("OPK %d: Expected prekey ID %d, got %d", i, 100+i, opk.PrekeyId)
		}
	}

	// Verify uniqueness
	seen := make(map[string]bool)
	for _, opk := range opks {
		key := string(opk.PrekeyPub)
		if seen[key] {
			t.Error("Duplicate OPK generated")
		}
		seen[key] = true
	}
}

// TestShouldReplenishOPKs tests OPK replenishment logic
func TestShouldReplenishOPKs(t *testing.T) {
	tests := []struct {
		count    int
		expected bool
	}{
		{0, true},
		{10, true},
		{19, true},
		{20, false},
		{50, false},
		{100, false},
	}

	for _, tt := range tests {
		result := ShouldReplenishOPKs(tt.count)
		if result != tt.expected {
			t.Errorf("ShouldReplenishOPKs(%d) = %v, expected %v", tt.count, result, tt.expected)
		}
	}
}

// TestSPKNeedsRotation tests SPK rotation logic
func TestSPKNeedsRotation(t *testing.T) {
	identity := generateTestIdentity(t)

	// Nil SPK needs rotation
	if !SPKNeedsRotation(nil) {
		t.Error("Nil SPK should need rotation")
	}

	// Fresh SPK doesn't need rotation
	spk, _ := identity.GenerateSignedPrekey(1)
	if SPKNeedsRotation(spk) {
		t.Error("Fresh SPK should not need rotation")
	}

	// Expired SPK needs rotation
	expiredSPK := &pb.SignedPrekey{
		DeviceId:  spk.DeviceId,
		PrekeyPub: spk.PrekeyPub,
		PrekeyId:  spk.PrekeyId,
		CreatedAt: uint64(time.Now().AddDate(0, 0, -10).Unix()),
		ExpiresAt: uint64(time.Now().Add(-time.Hour).Unix()), // Expired 1 hour ago
		Signature: spk.Signature,
	}
	if !SPKNeedsRotation(expiredSPK) {
		t.Error("Expired SPK should need rotation")
	}

	// SPK in overlap period needs rotation
	overlapSPK := &pb.SignedPrekey{
		DeviceId:  spk.DeviceId,
		PrekeyPub: spk.PrekeyPub,
		PrekeyId:  spk.PrekeyId,
		CreatedAt: uint64(time.Now().AddDate(0, 0, -7).Unix()),
		ExpiresAt: uint64(time.Now().Add(12 * time.Hour).Unix()), // Expires in 12 hours (within 24h overlap)
		Signature: spk.Signature,
	}
	if !SPKNeedsRotation(overlapSPK) {
		t.Error("SPK in overlap period should need rotation")
	}
}
