package identity

import (
	"testing"
	"time"

	pb "babylontower/pkg/proto"
)

// testMnemonic2 is a second fixed mnemonic for testing different identities
const testMnemonic2 = "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong"

// TestCreateIdentityDocument tests identity document creation
func TestCreateIdentityDocument(t *testing.T) {
	identity := generateTestIdentity(t)
	manager := NewIdentityDocumentManager(identity)

	// Generate prekeys
	spk, _ := identity.GenerateSignedPrekey(1)
	opks, _ := identity.GenerateOneTimePrekeys(1, 5)
	deviceCert, _ := identity.CreateDeviceCertificate()

	// Create first document (sequence=1)
	doc, err := manager.CreateIdentityDocument(
		0,
		nil,
		[]*pb.DeviceCertificate{deviceCert},
		[]*pb.SignedPrekey{spk},
		opks,
		"Test User",
	)
	if err != nil {
		t.Fatalf("Failed to create identity document: %v", err)
	}

	// Verify document fields
	if doc.Sequence != 1 {
		t.Errorf("Expected sequence 1, got %d", doc.Sequence)
	}
	if len(doc.PreviousHash) != 0 {
		t.Errorf("Expected empty previous hash for first document, got %d bytes", len(doc.PreviousHash))
	}
	if len(doc.Devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(doc.Devices))
	}
	if len(doc.SignedPrekeys) != 1 {
		t.Errorf("Expected 1 SPK, got %d", len(doc.SignedPrekeys))
	}
	if len(doc.OneTimePrekeys) != 5 {
		t.Errorf("Expected 5 OPKs, got %d", len(doc.OneTimePrekeys))
	}
	if doc.DisplayName != "Test User" {
		t.Errorf("Expected display name 'Test User', got '%s'", doc.DisplayName)
	}
	if len(doc.Signature) == 0 {
		t.Error("Document signature is empty")
	}

	// Verify document
	err = VerifyIdentityDocument(doc)
	if err != nil {
		t.Errorf("Identity document verification failed: %v", err)
	}
}

// TestIdentityDocumentHashChain tests hash chain integrity
func TestIdentityDocumentHashChain(t *testing.T) {
	identity := generateTestIdentity(t)
	manager := NewIdentityDocumentManager(identity)

	// Create first document
	doc1, _ := manager.CreateIdentityDocument(
		0,
		nil,
		nil,
		nil,
		nil,
		"Test User",
	)

	// Compute hash of first document
	hash1, err := ComputeDocumentHash(doc1)
	if err != nil {
		t.Fatalf("Failed to compute document hash: %v", err)
	}

	// Create second document with previous hash
	doc2, err := manager.CreateIdentityDocument(
		doc1.Sequence,
		hash1,
		nil,
		nil,
		nil,
		"Test User Updated",
	)
	if err != nil {
		t.Fatalf("Failed to create second document: %v", err)
	}

	// Verify sequence increment
	if doc2.Sequence != 2 {
		t.Errorf("Expected sequence 2, got %d", doc2.Sequence)
	}

	// Verify previous hash matches
	if len(doc2.PreviousHash) != len(hash1) {
		t.Errorf("Previous hash length mismatch: expected %d, got %d", len(hash1), len(doc2.PreviousHash))
	}
	for i := range hash1 {
		if doc2.PreviousHash[i] != hash1[i] {
			t.Errorf("Previous hash mismatch at byte %d", i)
		}
	}
}

// TestVerifyIdentityDocument tests document verification
func TestVerifyIdentityDocument(t *testing.T) {
	identity := generateTestIdentity(t)
	manager := NewIdentityDocumentManager(identity)

	doc, _ := manager.CreateIdentityDocument(0, nil, nil, nil, nil, "Test User")

	// Valid document should verify
	err := VerifyIdentityDocument(doc)
	if err != nil {
		t.Errorf("Valid document verification failed: %v", err)
	}

	// Tamper with signature should fail
	originalSig := doc.Signature
	doc.Signature = make([]byte, len(originalSig))
	copy(doc.Signature, originalSig)
	doc.Signature[0] ^= 0xFF // Flip bits
	err = VerifyIdentityDocument(doc)
	if err == nil {
		t.Error("Expected verification failure for tampered signature")
	}
	doc.Signature = originalSig

	// Tamper with identity key should fail
	originalPub := doc.IdentitySignPub
	wrongIdentity := generateTestIdentityFromMnemonic(t, testMnemonic2)
	doc.IdentitySignPub = wrongIdentity.IKSignPub
	err = VerifyIdentityDocument(doc)
	if err == nil {
		t.Error("Expected verification failure for wrong identity key")
	}
	doc.IdentitySignPub = originalPub

	// Invalid sequence should fail
	doc.Sequence = 0
	err = VerifyIdentityDocument(doc)
	if err == nil {
		t.Error("Expected verification failure for zero sequence")
	}
	doc.Sequence = 1
}

// TestSerializeDocumentForSigning tests document serialization
func TestSerializeDocumentForSigning(t *testing.T) {
	identity := generateTestIdentity(t)
	manager := NewIdentityDocumentManager(identity)

	doc, _ := manager.CreateIdentityDocument(0, nil, nil, nil, nil, "Test User")

	// Serialize twice and verify determinism
	data1, err := SerializeDocumentForSigning(doc)
	if err != nil {
		t.Fatalf("Failed to serialize document: %v", err)
	}

	data2, err := SerializeDocumentForSigning(doc)
	if err != nil {
		t.Fatalf("Failed to serialize document: %v", err)
	}

	if len(data1) != len(data2) {
		t.Errorf("Serialization length mismatch: %d vs %d", len(data1), len(data2))
	}

	for i := range data1 {
		if data1[i] != data2[i] {
			t.Errorf("Serialization mismatch at byte %d", i)
		}
	}
}

// TestDeriveIdentityDHTKey tests DHT key derivation
func TestDeriveIdentityDHTKey(t *testing.T) {
	identity := generateTestIdentity(t)

	dhtKey := DeriveIdentityDHTKey(identity.IKSignPub)

	// Verify format - using /bt/id/ namespace per protocol spec
	expectedPrefix := "/bt/id/"
	if len(dhtKey) <= len(expectedPrefix) {
		t.Errorf("DHT key too short: %s", dhtKey)
	}
	if dhtKey[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("DHT key has wrong prefix: expected %s, got %s", expectedPrefix, dhtKey[:len(expectedPrefix)])
	}

	// Verify determinism
	dhtKey2 := DeriveIdentityDHTKey(identity.IKSignPub)
	if dhtKey != dhtKey2 {
		t.Error("DHT key not deterministic")
	}

	// Verify different identities produce different keys
	identity2 := generateTestIdentityFromMnemonic(t, testMnemonic2)
	dhtKey3 := DeriveIdentityDHTKey(identity2.IKSignPub)
	if dhtKey == dhtKey3 {
		t.Error("Different identities produced same DHT key")
	}
}

// TestDerivePrekeyBundleDHTKey tests prekey bundle DHT key derivation
func TestDerivePrekeyBundleDHTKey(t *testing.T) {
	identity := generateTestIdentity(t)

	dhtKey := DerivePrekeyBundleDHTKey(identity.IKSignPub)

	// Verify format - using /bt/prekeys/ namespace per protocol spec
	expectedPrefix := "/bt/prekeys/"
	if len(dhtKey) <= len(expectedPrefix) {
		t.Errorf("DHT key too short: %s", dhtKey)
	}
	if dhtKey[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("DHT key has wrong prefix: expected %s, got %s", expectedPrefix, dhtKey[:len(expectedPrefix)])
	}

	// Verify different from identity DHT key
	identityDHTKey := DeriveIdentityDHTKey(identity.IKSignPub)
	if dhtKey == identityDHTKey {
		t.Error("Prekey bundle DHT key should differ from identity DHT key")
	}
}

// TestFeatureFlags tests feature flags creation
func TestFeatureFlags(t *testing.T) {
	identity := generateTestIdentity(t)
	manager := NewIdentityDocumentManager(identity)

	doc, _ := manager.CreateIdentityDocument(0, nil, nil, nil, nil, "Test User")

	// Verify default feature flags
	if doc.Features == nil {
		t.Fatal("Features not set")
	}

	// All default features should be enabled
	if !doc.Features.SupportsReadReceipts {
		t.Error("SupportsReadReceipts should be true")
	}
	if !doc.Features.SupportsTypingIndicators {
		t.Error("SupportsTypingIndicators should be true")
	}
	if !doc.Features.SupportsReactions {
		t.Error("SupportsReactions should be true")
	}
	if !doc.Features.SupportsEdits {
		t.Error("SupportsEdits should be true")
	}
	if !doc.Features.SupportsMedia {
		t.Error("SupportsMedia should be true")
	}
	if !doc.Features.SupportsVoiceCalls {
		t.Error("SupportsVoiceCalls should be true")
	}
	if !doc.Features.SupportsVideoCalls {
		t.Error("SupportsVideoCalls should be true")
	}
	if !doc.Features.SupportsGroups {
		t.Error("SupportsGroups should be true")
	}
	if !doc.Features.SupportsChannels {
		t.Error("SupportsChannels should be true")
	}
	if !doc.Features.SupportsOfflineMessages {
		t.Error("SupportsOfflineMessages should be true")
	}
}

// TestIdentityDocumentTimestamp tests timestamp validation
func TestIdentityDocumentTimestamp(t *testing.T) {
	identity := generateTestIdentity(t)
	manager := NewIdentityDocumentManager(identity)

	doc, _ := manager.CreateIdentityDocument(0, nil, nil, nil, nil, "Test User")

	// Valid timestamp should pass
	err := VerifyIdentityDocument(doc)
	if err != nil {
		t.Errorf("Valid timestamp verification failed: %v", err)
	}

	// Old timestamp should fail
	doc.UpdatedAt = uint64(time.Now().AddDate(0, 0, -2).Unix())
	err = VerifyIdentityDocument(doc)
	if err == nil {
		t.Error("Expected verification failure for old timestamp")
	}

	// Future timestamp should fail
	doc.UpdatedAt = uint64(time.Now().AddDate(0, 0, 2).Unix())
	err = VerifyIdentityDocument(doc)
	if err == nil {
		t.Error("Expected verification failure for future timestamp")
	}
}
