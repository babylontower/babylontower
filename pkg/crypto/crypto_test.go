package crypto

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

func TestComputeSharedSecret(t *testing.T) {
	// Generate two key pairs for X25519
	// For this test, we'll generate raw 32-byte keys
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	copy(key1, []byte("test key 1........................"))
	copy(key2, []byte("test key 2........................"))
	// Just verify the function works with valid inputs
	_, err := ComputeSharedSecret(key1, key2)
	if err != nil {
		t.Fatalf("ComputeSharedSecret() failed: %v", err)
	}
}

func TestComputeSharedSecret_InvalidKeySize(t *testing.T) {
	_, err := ComputeSharedSecret([]byte("too short"), make([]byte, 32))
	if err == nil {
		t.Error("Expected error for invalid private key size")
	}
	_, err = ComputeSharedSecret(make([]byte, 32), []byte("too short"))
	if err == nil {
		t.Error("Expected error for invalid public key size")
	}
}

func TestGenerateNonce(t *testing.T) {
	nonce1, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce() failed: %v", err)
	}
	if len(nonce1) != NonceSize {
		t.Errorf("Expected nonce length %d, got %d", NonceSize, len(nonce1))
	}
	// Verify nonces are unique
	nonce2, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce() failed: %v", err)
	}
	if bytes.Equal(nonce1, nonce2) {
		t.Error("Expected unique nonces")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() failed: %v", err)
	}
	nonce, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce() failed: %v", err)
	}
	plaintext := []byte("Hello, Babylon Tower!")
	// Encrypt
	ciphertext, err := Encrypt(key, nonce, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() failed: %v", err)
	}
	// Verify ciphertext is different from plaintext
	if bytes.Equal(ciphertext, plaintext) {
		t.Error("Ciphertext should differ from plaintext")
	}
	// Verify ciphertext includes tag (longer than plaintext)
	if len(ciphertext) <= len(plaintext) {
		t.Error("Ciphertext should be longer than plaintext (includes tag)")
	}
	// Decrypt
	decrypted, err := Decrypt(key, nonce, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() failed: %v", err)
	}
	// Verify decryption matches original
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Decrypted text doesn't match: got %s, want %s", decrypted, plaintext)
	}
}

func TestDecrypt_InvalidKey(t *testing.T) {
	key1, _ := GenerateKey()
	key2, _ := GenerateKey()
	nonce, _ := GenerateNonce()
	plaintext := []byte("test message")
	ciphertext, _ := Encrypt(key1, nonce, plaintext)
	// Decrypt with wrong key should fail
	_, err := Decrypt(key2, nonce, ciphertext)
	if err == nil {
		t.Error("Expected error for wrong key")
	}
}

func TestDecrypt_InvalidNonce(t *testing.T) {
	key, _ := GenerateKey()
	nonce1, _ := GenerateNonce()
	nonce2, _ := GenerateNonce()
	plaintext := []byte("test message")
	ciphertext, _ := Encrypt(key, nonce1, plaintext)
	// Decrypt with wrong nonce should fail
	_, err := Decrypt(key, nonce2, ciphertext)
	if err == nil {
		t.Error("Expected error for wrong nonce")
	}
}

func TestEncrypt_InvalidKeySize(t *testing.T) {
	nonce, _ := GenerateNonce()
	_, err := Encrypt([]byte("too short"), nonce, []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid key size")
	}
}

func TestEncrypt_InvalidNonceSize(t *testing.T) {
	key, _ := GenerateKey()
	_, err := Encrypt(key, []byte("too short"), []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid nonce size")
	}
}

func TestDecrypt_InvalidKeySize(t *testing.T) {
	nonce, _ := GenerateNonce()
	_, err := Decrypt([]byte("too short"), nonce, []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid key size")
	}
}

func TestDecrypt_InvalidNonceSize(t *testing.T) {
	key, _ := GenerateKey()
	_, err := Decrypt(key, []byte("too short"), []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid nonce size")
	}
}

func TestDeriveKey(t *testing.T) {
	ikm := []byte("initial keying material")
	salt := []byte("salt")
	info := []byte("context info")
	// Derive key of specific length
	key, err := DeriveKey(ikm, salt, info, 32)
	if err != nil {
		t.Fatalf("DeriveKey() failed: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("Expected key length 32, got %d", len(key))
	}
	// Verify deterministic derivation
	key2, err := DeriveKey(ikm, salt, info, 32)
	if err != nil {
		t.Fatalf("DeriveKey() failed: %v", err)
	}
	if !bytes.Equal(key, key2) {
		t.Error("Key derivation is not deterministic")
	}
	// Derive different length
	key64, err := DeriveKey(ikm, salt, info, 64)
	if err != nil {
		t.Fatalf("DeriveKey() failed: %v", err)
	}
	if len(key64) != 64 {
		t.Errorf("Expected key length 64, got %d", len(key64))
	}
}

func TestDeriveKey_InvalidLength(t *testing.T) {
	_, err := DeriveKey([]byte("ikm"), nil, nil, 0)
	if err == nil {
		t.Error("Expected error for zero length")
	}
	_, err = DeriveKey([]byte("ikm"), nil, nil, -1)
	if err == nil {
		t.Error("Expected error for negative length")
	}
}

func TestEncryptWithSharedSecret(t *testing.T) {
	// Generate shared secret
	sharedSecret := make([]byte, 32)
	copy(sharedSecret, []byte("shared secret test key!!"))
	plaintext := []byte("Secret message")
	// Encrypt
	nonce, ciphertext, err := EncryptWithSharedSecret(sharedSecret, plaintext)
	if err != nil {
		t.Fatalf("EncryptWithSharedSecret() failed: %v", err)
	}
	// Decrypt
	decrypted, err := DecryptWithSharedSecret(sharedSecret, nonce, ciphertext)
	if err != nil {
		t.Fatalf("DecryptWithSharedSecret() failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Decrypted text doesn't match: got %s, want %s", decrypted, plaintext)
	}
}

func TestEncryptWithSharedSecret_InvalidSize(t *testing.T) {
	_, _, err := EncryptWithSharedSecret([]byte("too short"), []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid shared secret size")
	}
}

func TestDecryptWithSharedSecret_InvalidSize(t *testing.T) {
	_, err := DecryptWithSharedSecret([]byte("too short"), make([]byte, 24), []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid shared secret size")
	}
}

func TestGenerateKey(t *testing.T) {
	key1, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() failed: %v", err)
	}
	if len(key1) != KeySize {
		t.Errorf("Expected key length %d, got %d", KeySize, len(key1))
	}
	// Verify keys are unique
	key2, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() failed: %v", err)
	}
	if bytes.Equal(key1, key2) {
		t.Error("Expected unique keys")
	}
}

func TestSign(t *testing.T) {
	_, privKey, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair() failed: %v", err)
	}
	message := []byte("Hello, Babylon Tower!")
	signature, err := Sign(privKey, message)
	if err != nil {
		t.Fatalf("Sign() failed: %v", err)
	}
	if len(signature) != SignatureSize {
		t.Errorf("Expected signature length %d, got %d", SignatureSize, len(signature))
	}
}

func TestSign_InvalidPrivateKey(t *testing.T) {
	_, err := Sign([]byte("too short"), []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid private key")
	}
}

func TestVerify(t *testing.T) {
	pubKey, privKey, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair() failed: %v", err)
	}
	message := []byte("Hello, Babylon Tower!")
	signature, err := Sign(privKey, message)
	if err != nil {
		t.Fatalf("Sign() failed: %v", err)
	}
	// Verify valid signature
	if !Verify(pubKey, message, signature) {
		t.Error("Valid signature verification failed")
	}
	// Verify with wrong message
	if Verify(pubKey, []byte("wrong message"), signature) {
		t.Error("Signature should not verify for different message")
	}
}

func TestVerify_InvalidPublicKey(t *testing.T) {
	result := Verify([]byte("too short"), []byte("test"), make([]byte, 64))
	if result {
		t.Error("Should return false for invalid public key")
	}
}

func TestVerify_InvalidSignatureSize(t *testing.T) {
	pubKey, _, _ := GenerateEd25519KeyPair()
	result := Verify(pubKey, []byte("test"), []byte("too short"))
	if result {
		t.Error("Should return false for invalid signature size")
	}
}

func TestGenerateEd25519KeyPair(t *testing.T) {
	pubKey, privKey, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair() failed: %v", err)
	}
	if len(pubKey) != ed25519.PublicKeySize {
		t.Errorf("Expected public key length %d, got %d", ed25519.PublicKeySize, len(pubKey))
	}
	if len(privKey) != ed25519.PrivateKeySize {
		t.Errorf("Expected private key length %d, got %d", ed25519.PrivateKeySize, len(privKey))
	}
	// Verify key pairs are unique
	pubKey2, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair() failed: %v", err)
	}
	if bytes.Equal(pubKey, pubKey2) {
		t.Error("Expected unique key pairs")
	}
}

func TestSignAndEncode(t *testing.T) {
	_, privKey, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair() failed: %v", err)
	}
	message := []byte("test message")
	sigHex, err := SignAndEncode(privKey, message)
	if err != nil {
		t.Fatalf("SignAndEncode() failed: %v", err)
	}
	// Verify hex string length (64 bytes = 128 hex chars)
	if len(sigHex) != SignatureSize*2 {
		t.Errorf("Expected signature hex length %d, got %d", SignatureSize*2, len(sigHex))
	}
}

func TestVerifyAndDecode(t *testing.T) {
	pubKey, privKey, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair() failed: %v", err)
	}
	message := []byte("test message")
	sigHex, err := SignAndEncode(privKey, message)
	if err != nil {
		t.Fatalf("SignAndEncode() failed: %v", err)
	}
	// Verify valid signature
	if !VerifyAndDecode(pubKey, message, sigHex) {
		t.Error("Valid signature verification failed")
	}
	// Verify invalid hex
	if VerifyAndDecode(pubKey, message, "invalid") {
		t.Error("Should return false for invalid hex")
	}
}

func TestValidatePrivateKey(t *testing.T) {
	_, privKey, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair() failed: %v", err)
	}
	if !ValidatePrivateKey(privKey) {
		t.Error("Valid private key validation failed")
	}
	// Invalid key
	if ValidatePrivateKey([]byte("too short")) {
		t.Error("Should return false for invalid key")
	}
}

func TestValidatePublicKey(t *testing.T) {
	pubKey, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair() failed: %v", err)
	}
	if !ValidatePublicKey(pubKey) {
		t.Error("Valid public key validation failed")
	}
	// Invalid key
	if ValidatePublicKey([]byte("too short")) {
		t.Error("Should return false for invalid key")
	}
}

func TestRandomBytes(t *testing.T) {
	bytes1, err := RandomBytes(32)
	if err != nil {
		t.Fatalf("RandomBytes() failed: %v", err)
	}
	if len(bytes1) != 32 {
		t.Errorf("Expected length 32, got %d", len(bytes1))
	}
	// Verify uniqueness
	bytes2, err := RandomBytes(32)
	if err != nil {
		t.Fatalf("RandomBytes() failed: %v", err)
	}
	if bytes.Equal(bytes1, bytes2) {
		t.Error("Expected unique random bytes")
	}
}

func TestRandomBytes_InvalidLength(t *testing.T) {
	_, err := RandomBytes(0)
	if err == nil {
		t.Error("Expected error for zero length")
	}
	_, err = RandomBytes(-1)
	if err == nil {
		t.Error("Expected error for negative length")
	}
}

func TestGenerateNonce_Error(t *testing.T) {
	// This test verifies the nonce generation works
	// In practice, rand.Reader should never fail
	nonce, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce() failed: %v", err)
	}
	if len(nonce) != NonceSize {
		t.Errorf("Expected nonce length %d, got %d", NonceSize, len(nonce))
	}
}

func TestGenerateKey_Error(t *testing.T) {
	// This test verifies the key generation works
	// In practice, rand.Reader should never fail
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() failed: %v", err)
	}
	if len(key) != KeySize {
		t.Errorf("Expected key length %d, got %d", KeySize, len(key))
	}
}

func TestEncryptWithSharedSecret_Error(t *testing.T) {
	sharedSecret := make([]byte, 32)
	plaintext := []byte("test")
	// This should work
	nonce, ciphertext, err := EncryptWithSharedSecret(sharedSecret, plaintext)
	if err != nil {
		t.Fatalf("EncryptWithSharedSecret() failed: %v", err)
	}
	if len(nonce) != NonceSize {
		t.Errorf("Expected nonce length %d, got %d", NonceSize, len(nonce))
	}
	if len(ciphertext) == 0 {
		t.Error("Expected non-empty ciphertext")
	}
}

func TestDecryptWithSharedSecret_Error(t *testing.T) {
	sharedSecret := make([]byte, 32)
	nonce := make([]byte, NonceSize)
	ciphertext := []byte("invalid ciphertext")
	// This should fail with invalid ciphertext
	_, err := DecryptWithSharedSecret(sharedSecret, nonce, ciphertext)
	if err == nil {
		t.Error("Expected error for invalid ciphertext")
	}
}
