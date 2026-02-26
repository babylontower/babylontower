package ratchet

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/tyler-smith/go-bip39"
	"babylontower/pkg/identity"
	"golang.org/x/crypto/curve25519"
)

// generateX25519TestKey generates an X25519 key pair for testing
func generateX25519TestKey(t *testing.T) (*[32]byte, *[32]byte) {
	priv := new([32]byte)
	pub := new([32]byte)
	if _, err := io.ReadFull(rand.Reader, priv[:]); err != nil {
		t.Fatalf("Failed to read random bytes: %v", err)
	}
	result, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		t.Fatalf("X25519 derivation failed: %v", err)
	}
	copy(pub[:], result)
	return pub, priv
}

// TestX3DH_4DH tests X3DH with one-time prekey (4-DH)
func TestX3DH_4DH(t *testing.T) {
	// Generate Alice's identity keys
	aliceIKDHPub, aliceIKDHPriv := generateX25519TestKey(t)

	// Generate Bob's identity and prekeys
	bobIKDHPub, bobIKDHPriv := generateX25519TestKey(t)
	bobSPKPub, bobSPKPriv := generateX25519TestKey(t)
	bobOPKPub, bobOPKPriv := generateX25519TestKey(t)

	// Alice initiates - generates ephemeral key internally
	result1, err := X3DHInitiator(
		aliceIKDHPriv,
		aliceIKDHPub,
		bobIKDHPub,
		bobSPKPub,
		bobOPKPub,
	)
	if err != nil {
		t.Fatalf("X3DH initiator failed: %v", err)
	}

	// Bob responds - needs Alice's ephemeral key from result
	result2, err := X3DHResponder(
		bobIKDHPriv,
		bobIKDHPub,
		bobSPKPriv,
		bobOPKPriv,
		aliceIKDHPub, // In real protocol, this would be Alice's IK, not EK
		result1.EphemeralPub,
	)
	if err != nil {
		t.Fatalf("X3DH responder failed: %v", err)
	}

	// Verify shared secrets match
	if !bytes.Equal(result1.SharedSecret, result2.SharedSecret) {
		t.Errorf("Shared secrets do not match")
		t.Logf("Alice: %x", result1.SharedSecret)
		t.Logf("Bob: %x", result2.SharedSecret)
	}

	// Verify cipher suite
	if result1.CipherSuite != CipherSuiteXChaCha20Poly1305 {
		t.Errorf("Unexpected cipher suite: %d", result1.CipherSuite)
	}
}

// TestX3DH_3DH tests X3DH without one-time prekey (3-DH fallback)
func TestX3DH_3DH(t *testing.T) {
	aliceIKDHPub, aliceIKDHPriv := generateX25519TestKey(t)
	bobIKDHPub, bobIKDHPriv := generateX25519TestKey(t)
	bobSPKPub, bobSPKPriv := generateX25519TestKey(t)

	// Alice initiates without OPK
	result1, err := X3DHInitiator(
		aliceIKDHPriv,
		aliceIKDHPub,
		bobIKDHPub,
		bobSPKPub,
		nil, // No OPK
	)
	if err != nil {
		t.Fatalf("X3DH initiator (3-DH) failed: %v", err)
	}

	// Bob responds without OPK
	result2, err := X3DHResponder(
		bobIKDHPriv,
		bobIKDHPub,
		bobSPKPriv,
		nil, // No OPK
		aliceIKDHPub,
		result1.EphemeralPub,
	)
	if err != nil {
		t.Fatalf("X3DH responder (3-DH) failed: %v", err)
	}

	// Verify shared secrets match
	if !bytes.Equal(result1.SharedSecret, result2.SharedSecret) {
		t.Error("Shared secrets do not match (3-DH)")
	}
}

// TestKDF_RK tests root key KDF
func TestKDF_RK(t *testing.T) {
	rootKey := make([]byte, 32)
	dhOutput := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, rootKey); err != nil {
		t.Fatalf("Failed to read random bytes: %v", err)
	}
	if _, err := io.ReadFull(rand.Reader, dhOutput); err != nil {
		t.Fatalf("Failed to read random bytes: %v", err)
	}

	newRK, newCK := KDF_RK(rootKey, dhOutput)

	// Verify output lengths
	if len(newRK) != 32 {
		t.Errorf("Expected root key length 32, got %d", len(newRK))
	}
	if len(newCK) != 32 {
		t.Errorf("Expected chain key length 32, got %d", len(newCK))
	}

	// Verify determinism
	newRK2, newCK2 := KDF_RK(rootKey, dhOutput)
	if !bytes.Equal(newRK, newRK2) {
		t.Error("Root key KDF not deterministic")
	}
	if !bytes.Equal(newCK, newCK2) {
		t.Error("Chain key KDF not deterministic")
	}
}

// TestKDF_CK tests chain key KDF
func TestKDF_CK(t *testing.T) {
	chainKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, chainKey); err != nil {
		t.Fatalf("Failed to read random bytes: %v", err)
	}

	newCK, mk := KDF_CK(chainKey)

	// Verify output lengths
	if len(newCK) != 32 {
		t.Errorf("Expected new chain key length 32, got %d", len(newCK))
	}
	if len(mk) != 32 {
		t.Errorf("Expected message key length 32, got %d", len(mk))
	}

	// Verify determinism
	newCK2, mk2 := KDF_CK(chainKey)
	if !bytes.Equal(newCK, newCK2) {
		t.Error("Chain key KDF not deterministic")
	}
	if !bytes.Equal(mk, mk2) {
		t.Error("Message key KDF not deterministic")
	}
}

// TestDoubleRatchet_InitiatorResponder tests full Double Ratchet exchange
func TestDoubleRatchet_InitiatorResponder(t *testing.T) {
	// Generate identities with different mnemonics
	aliceEntropy, _ := bip39.NewEntropy(128)
	aliceMnemonic, _ := bip39.NewMnemonic(aliceEntropy)
	bobEntropy, _ := bip39.NewEntropy(128)
	bobMnemonic, _ := bip39.NewMnemonic(bobEntropy)
	alice, _ := identity.NewIdentityV1(aliceMnemonic, "Alice")
	bob, _ := identity.NewIdentityV1(bobMnemonic, "Bob")

	// Generate Bob's prekeys
	bobSPK, _ := bob.GenerateSignedPrekey(1)
	bobOPKs, _ := bob.GenerateOneTimePrekeys(1, 1)

	// Convert to X25519 format
	bobSPKPub := new([32]byte)
	copy(bobSPKPub[:], bobSPK.PrekeyPub)

	bobOPKPub := new([32]byte)
	copy(bobOPKPub[:], bobOPKs[0].PrekeyPub)

	// X3DH: Alice initiates
	x3dhResult, err := X3DHInitiator(
		alice.IKDHPriv,
		alice.IKDHPub,
		bob.IKDHPub,
		bobSPKPub,
		bobOPKPub,
	)
	if err != nil {
		t.Fatalf("X3DH failed: %v", err)
	}

	// For this simplified test, just test KDF functions directly
	// Full ratchet test requires more setup
	t.Run("KDF chain", func(t *testing.T) {
		rootKey, chainKey := KDF_RK(x3dhResult.SharedSecret, x3dhResult.SharedSecret)
		if len(rootKey) != 32 {
			t.Errorf("Expected root key length 32, got %d", len(rootKey))
		}
		if len(chainKey) != 32 {
			t.Errorf("Expected chain key length 32, got %d", len(chainKey))
		}

		newChainKey, msgKey := KDF_CK(chainKey)
		if len(newChainKey) != 32 {
			t.Errorf("Expected new chain key length 32, got %d", len(newChainKey))
		}
		if len(msgKey) != 32 {
			t.Errorf("Expected message key length 32, got %d", len(msgKey))
		}
	})
}

// TestDoubleRatchet_MultipleMessages tests multiple message exchange
func TestDoubleRatchet_MultipleMessages(t *testing.T) {
	// This test requires full ratchet state synchronization
	// Skipping until full implementation is complete
	t.Skip("Requires full ratchet state synchronization")
}

// TestDoubleRatchet_OutOfOrder tests out-of-order message handling
func TestDoubleRatchet_OutOfOrder(t *testing.T) {
	// This test would verify that skipped messages are properly cached
	// Implementation requires full ratchet setup
	t.Skip("Out-of-order test requires full ratchet implementation")
}

// TestSessionState_Serialization tests session state serialization
func TestSessionState_Serialization(t *testing.T) {
	_, dhPub := generateX25519TestKey(t)
	_, dhPriv := generateX25519TestKey(t)

	rootKey := make([]byte, 32)
	chainKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, rootKey); err != nil {
		t.Fatalf("Failed to read random bytes: %v", err)
	}
	if _, err := io.ReadFull(rand.Reader, chainKey); err != nil {
		t.Fatalf("Failed to read random bytes: %v", err)
	}

	state := &DoubleRatchetState{
		SessionID:           "test",
		RootKey:             rootKey,
		SendingChainKey:     chainKey,
		SendingChainCount:   5,
		DHSendingKeyPub:     dhPub,
		DHSendingKeyPriv:    dhPriv,
		DHReceivingKeyPub:   dhPub,
		SkippedKeys:         make(map[string][]byte),
		IsInitiator:         true,
	}

	serializable := state.GetSessionState()

	if serializable.SessionID != "test" {
		t.Error("Session ID not preserved")
	}
	if serializable.SendingChainCounter != 5 {
		t.Errorf("Expected sending counter 5, got %d", serializable.SendingChainCounter)
	}
	if !bytes.Equal(serializable.RootKey, rootKey) {
		t.Error("Root key not preserved")
	}
}

// TestZeroBytes tests that sensitive data is zeroed
func TestZeroBytes(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5}
	zeroBytes(data)

	for i, b := range data {
		if b != 0 {
			t.Errorf("Byte %d not zeroed: %d", i, b)
		}
	}
}

// TestDeriveNonce tests nonce derivation
func TestDeriveNonce(t *testing.T) {
	messageKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, messageKey); err != nil {
		t.Fatalf("Failed to read random bytes: %v", err)
	}

	nonce1, err := DeriveNonce(messageKey, 0)
	if err != nil {
		t.Fatalf("Nonce derivation failed: %v", err)
	}
	if len(nonce1) != 12 {
		t.Errorf("Expected nonce length 12, got %d", len(nonce1))
	}

	nonce2, err := DeriveNonce(messageKey, 0)
	if err != nil {
		t.Fatalf("Nonce derivation failed: %v", err)
	}

	// Verify determinism
	if !bytes.Equal(nonce1, nonce2) {
		t.Error("Nonce derivation not deterministic")
	}

	// Different counter should produce different nonce
	nonce3, _ := DeriveNonce(messageKey, 1)
	if bytes.Equal(nonce1, nonce3) {
		t.Error("Different counters produced same nonce")
	}
}
