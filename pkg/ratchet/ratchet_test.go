package ratchet

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	"babylontower/pkg/identity"

	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/curve25519"
)

// generateX25519TestKey generates an X25519 key pair for testing
func generateX25519TestKey(t testing.TB) (*[32]byte, *[32]byte) {
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

// generateEd25519TestKey generates an Ed25519 key pair for testing
func generateEd25519TestKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate Ed25519 key: %v", err)
	}
	return pub, priv
}

// TestX3DH_4DH tests X3DH with one-time prekey (4-DH)
func TestX3DH_4DH(t *testing.T) {
	// Generate Alice's identity keys
	aliceIKDHPub, aliceIKDHPriv := generateX25519TestKey(t)
	aliceIKSignPub, _ := generateEd25519TestKey(t)

	// Generate Bob's identity and prekeys
	bobIKDHPub, bobIKDHPriv := generateX25519TestKey(t)
	bobIKSignPub, _ := generateEd25519TestKey(t)
	bobSPKPub, bobSPKPriv := generateX25519TestKey(t)
	bobOPKPub, bobOPKPriv := generateX25519TestKey(t)

	// Alice initiates - generates ephemeral key internally
	result1, err := X3DHInitiator(
		aliceIKDHPriv,
		aliceIKDHPub,
		aliceIKSignPub,
		bobIKDHPub,
		bobIKSignPub,
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
		bobIKSignPub,
		bobSPKPriv,
		bobOPKPriv,
		aliceIKDHPub, // In real protocol, this would be Alice's IK, not EK
		aliceIKSignPub,
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
	aliceIKSignPub, _ := generateEd25519TestKey(t)
	
	bobIKDHPub, bobIKDHPriv := generateX25519TestKey(t)
	bobIKSignPub, _ := generateEd25519TestKey(t)
	bobSPKPub, bobSPKPriv := generateX25519TestKey(t)

	// Alice initiates without OPK
	result1, err := X3DHInitiator(
		aliceIKDHPriv,
		aliceIKDHPub,
		aliceIKSignPub,
		bobIKDHPub,
		bobIKSignPub,
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
		bobIKSignPub,
		bobSPKPriv,
		nil, // No OPK
		aliceIKDHPub,
		aliceIKSignPub,
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
	aliceState, bobState := setupRatchetPair(t)

	ad := []byte("test-associated-data")

	// Alice sends a message to Bob
	msg1, err := aliceState.Encrypt([]byte("Hello Bob!"), ad)
	if err != nil {
		t.Fatalf("Alice encrypt failed: %v", err)
	}

	plaintext1, err := bobState.Decrypt(msg1.Header, msg1.Ciphertext, ad)
	if err != nil {
		t.Fatalf("Bob decrypt failed: %v", err)
	}
	if string(plaintext1) != "Hello Bob!" {
		t.Errorf("Decrypted message mismatch: got %q", plaintext1)
	}

	// Bob replies to Alice
	msg2, err := bobState.Encrypt([]byte("Hi Alice!"), ad)
	if err != nil {
		t.Fatalf("Bob encrypt failed: %v", err)
	}

	plaintext2, err := aliceState.Decrypt(msg2.Header, msg2.Ciphertext, ad)
	if err != nil {
		t.Fatalf("Alice decrypt failed: %v", err)
	}
	if string(plaintext2) != "Hi Alice!" {
		t.Errorf("Decrypted reply mismatch: got %q", plaintext2)
	}

	// Alice sends another message (tests symmetric ratchet advancement)
	msg3, err := aliceState.Encrypt([]byte("How are you?"), ad)
	if err != nil {
		t.Fatalf("Alice second encrypt failed: %v", err)
	}

	plaintext3, err := bobState.Decrypt(msg3.Header, msg3.Ciphertext, ad)
	if err != nil {
		t.Fatalf("Bob second decrypt failed: %v", err)
	}
	if string(plaintext3) != "How are you?" {
		t.Errorf("Second decrypted message mismatch: got %q", plaintext3)
	}
}

// setupRatchetPair creates an Alice-Bob ratchet pair for testing.
// Alice is the initiator and sends the first message.
func setupRatchetPair(t *testing.T) (*DoubleRatchetState, *DoubleRatchetState) {
	t.Helper()

	aliceEntropy, _ := bip39.NewEntropy(128)
	aliceMnemonic, _ := bip39.NewMnemonic(aliceEntropy)
	bobEntropy, _ := bip39.NewEntropy(128)
	bobMnemonic, _ := bip39.NewMnemonic(bobEntropy)
	alice, _ := identity.NewIdentityV1(aliceMnemonic, "Alice")
	bob, _ := identity.NewIdentityV1(bobMnemonic, "Bob")

	// Generate Bob's prekeys as raw X25519 pairs (since proto doesn't store private keys)
	bobSPKPub, bobSPKPriv := generateX25519TestKey(t)
	bobOPKPub, bobOPKPriv := generateX25519TestKey(t)

	// X3DH key agreement
	x3dhAlice, err := X3DHInitiator(alice.IKDHPriv, alice.IKDHPub, alice.IKSignPub, bob.IKDHPub, bob.IKSignPub, bobSPKPub, bobOPKPub)
	if err != nil {
		t.Fatalf("X3DH initiator failed: %v", err)
	}

	x3dhBob, err := X3DHResponder(bob.IKDHPriv, bob.IKDHPub, bob.IKSignPub, bobSPKPriv, bobOPKPriv, alice.IKDHPub, alice.IKSignPub, x3dhAlice.EphemeralPub)
	if err != nil {
		t.Fatalf("X3DH responder failed: %v", err)
	}

	if !bytes.Equal(x3dhAlice.SharedSecret, x3dhBob.SharedSecret) {
		t.Fatal("X3DH shared secrets do not match")
	}

	aliceState, err := NewDoubleRatchetStateInitiator("test-session", alice.IKSignPub, bob.IKSignPub, x3dhAlice.SharedSecret, bobSPKPub)
	if err != nil {
		t.Fatalf("Failed to create initiator state: %v", err)
	}

	bobState, err := NewDoubleRatchetStateResponder("test-session", bob.IKSignPub, alice.IKSignPub, x3dhBob.SharedSecret, bobSPKPriv, bobSPKPub)
	if err != nil {
		t.Fatalf("Failed to create responder state: %v", err)
	}

	return aliceState, bobState
}

// TestDoubleRatchet_MultipleMessages tests multiple message exchange
func TestDoubleRatchet_MultipleMessages(t *testing.T) {
	aliceState, bobState := setupRatchetPair(t)
	ad := []byte("associated-data")

	// Alice sends 3 messages to Bob
	for i := 0; i < 3; i++ {
		plaintext := []byte(fmt.Sprintf("hello from alice %d", i))
		enc, err := aliceState.Encrypt(plaintext, ad)
		if err != nil {
			t.Fatalf("Alice encrypt %d failed: %v", i, err)
		}

		decrypted, err := bobState.Decrypt(enc.Header, enc.Ciphertext, ad)
		if err != nil {
			t.Fatalf("Bob decrypt %d failed: %v", i, err)
		}
		if !bytes.Equal(plaintext, decrypted) {
			t.Errorf("Message %d mismatch: got %q, want %q", i, decrypted, plaintext)
		}
	}

	// Bob responds with 2 messages
	for i := 0; i < 2; i++ {
		plaintext := []byte(fmt.Sprintf("hello from bob %d", i))
		enc, err := bobState.Encrypt(plaintext, ad)
		if err != nil {
			t.Fatalf("Bob encrypt %d failed: %v", i, err)
		}

		decrypted, err := aliceState.Decrypt(enc.Header, enc.Ciphertext, ad)
		if err != nil {
			t.Fatalf("Alice decrypt %d failed: %v", i, err)
		}
		if !bytes.Equal(plaintext, decrypted) {
			t.Errorf("Bob message %d mismatch: got %q, want %q", i, decrypted, plaintext)
		}
	}

	// Alice sends again (new ratchet step)
	plaintext := []byte("alice after bob's reply")
	enc, err := aliceState.Encrypt(plaintext, ad)
	if err != nil {
		t.Fatalf("Alice re-encrypt failed: %v", err)
	}
	decrypted, err := bobState.Decrypt(enc.Header, enc.Ciphertext, ad)
	if err != nil {
		t.Fatalf("Bob re-decrypt failed: %v", err)
	}
	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Re-encrypted message mismatch")
	}
}

// TestDoubleRatchet_OutOfOrder tests out-of-order message handling
func TestDoubleRatchet_OutOfOrder(t *testing.T) {
	aliceState, bobState := setupRatchetPair(t)
	ad := []byte("associated-data")

	// Alice sends 3 messages
	encrypted := make([]*EncryptedMessage, 3)
	for i := 0; i < 3; i++ {
		var err error
		encrypted[i], err = aliceState.Encrypt([]byte(fmt.Sprintf("msg-%d", i)), ad)
		if err != nil {
			t.Fatalf("Encrypt %d failed: %v", i, err)
		}
	}

	// Bob decrypts msg-2 first (skipping 0 and 1)
	dec, err := bobState.Decrypt(encrypted[2].Header, encrypted[2].Ciphertext, ad)
	if err != nil {
		t.Fatalf("Decrypt msg-2 failed: %v", err)
	}
	if string(dec) != "msg-2" {
		t.Errorf("Expected msg-2, got %q", dec)
	}

	// Bob decrypts msg-0 (out of order, should use skipped key)
	dec, err = bobState.Decrypt(encrypted[0].Header, encrypted[0].Ciphertext, ad)
	if err != nil {
		t.Fatalf("Decrypt msg-0 (out of order) failed: %v", err)
	}
	if string(dec) != "msg-0" {
		t.Errorf("Expected msg-0, got %q", dec)
	}

	// Bob decrypts msg-1 (out of order, should use skipped key)
	dec, err = bobState.Decrypt(encrypted[1].Header, encrypted[1].Ciphertext, ad)
	if err != nil {
		t.Fatalf("Decrypt msg-1 (out of order) failed: %v", err)
	}
	if string(dec) != "msg-1" {
		t.Errorf("Expected msg-1, got %q", dec)
	}
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
		SessionID:         "test",
		RootKey:           rootKey,
		SendingChainKey:   chainKey,
		SendingChainCount: 5,
		DHSendingKeyPub:   dhPub,
		DHSendingKeyPriv:  dhPriv,
		DHReceivingKeyPub: dhPub,
		SkippedKeys:       make(map[string][]byte),
		IsInitiator:       true,
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
	if len(nonce1) != 24 {
		t.Errorf("Expected nonce length 24, got %d", len(nonce1))
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
