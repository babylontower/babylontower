package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

const (
	// NonceSize is the size of the nonce for XChaCha20-Poly1305
	NonceSize = chacha20poly1305.NonceSizeX
	// KeySize is the size of the key for XChaCha20-Poly1305
	KeySize = chacha20poly1305.KeySize
	// SharedSecretSize is the size of the X25519 shared secret
	SharedSecretSize = 32
	// SignatureSize is the size of an Ed25519 signature
	SignatureSize = 64
)

// SecureRandom is the random reader for cryptographic operations
var SecureRandom = rand.Reader

var (
	// ErrDecryptionFailed is returned when decryption fails
	ErrDecryptionFailed = errors.New("decryption failed")
	// ErrInvalidKeySize is returned when a key has an invalid size
	ErrInvalidKeySize = errors.New("invalid key size")
	// ErrInvalidNonceSize is returned when a nonce has an invalid size
	ErrInvalidNonceSize = errors.New("invalid nonce size")
)

// ComputeSharedSecret computes an X25519 shared secret
// Takes a private key and a peer's public key, both 32 bytes
// Returns a 32-byte shared secret for symmetric encryption
func ComputeSharedSecret(privKey, pubKey []byte) ([]byte, error) {
	if len(privKey) != SharedSecretSize {
		return nil, fmt.Errorf("private key must be %d bytes, got %d", SharedSecretSize, len(privKey))
	}
	if len(pubKey) != SharedSecretSize {
		return nil, fmt.Errorf("public key must be %d bytes, got %d", SharedSecretSize, len(pubKey))
	}
	var privArray [32]byte
	var pubArray [32]byte
	copy(privArray[:], privKey)
	copy(pubArray[:], pubKey)
	secret, err := curve25519.X25519(privArray[:], pubArray[:])
	if err != nil {
		return nil, fmt.Errorf("X25519 key agreement failed: %w", err)
	}
	return secret, nil
}

// GenerateNonce generates a random nonce for encryption
// Returns a 24-byte nonce suitable for XChaCha20-Poly1305
func GenerateNonce() ([]byte, error) {
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	return nonce, nil
}

// Encrypt encrypts plaintext using XChaCha20-Poly1305
// Returns ciphertext (includes authentication tag)
func Encrypt(key, nonce, plaintext []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("key must be %d bytes, got %d: %w", KeySize, len(key), ErrInvalidKeySize)
	}
	if len(nonce) != NonceSize {
		return nil, fmt.Errorf("nonce must be %d bytes, got %d: %w", NonceSize, len(nonce), ErrInvalidNonceSize)
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	var nonceArray [24]byte
	copy(nonceArray[:], nonce)
	ciphertext := aead.Seal(nil, nonceArray[:], plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using XChaCha20-Poly1305
// Returns plaintext if decryption succeeds and authentication tag is valid
func Decrypt(key, nonce, ciphertext []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("key must be %d bytes, got %d: %w", KeySize, len(key), ErrInvalidKeySize)
	}
	if len(nonce) != NonceSize {
		return nil, fmt.Errorf("nonce must be %d bytes, got %d: %w", NonceSize, len(nonce), ErrInvalidNonceSize)
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	var nonceArray [24]byte
	copy(nonceArray[:], nonce)
	plaintext, err := aead.Open(nil, nonceArray[:], ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}
	return plaintext, nil
}

// DeriveKey derives a key of specified length using HKDF
// Uses SHA-256 as the underlying hash function
// ikm: initial keying material
// salt: optional salt (can be nil)
// info: optional context info (can be nil)
// length: desired key length in bytes
func DeriveKey(ikm, salt, info []byte, length int) ([]byte, error) {
	if length <= 0 {
		return nil, errors.New("key length must be positive")
	}
	hkdfReader := hkdf.New(sha256.New, ikm, salt, info)
	key := make([]byte, length)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}
	return key, nil
}

// GenerateKey generates a random key for symmetric encryption
// Returns a 32-byte key suitable for XChaCha20-Poly1305
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	return key, nil
}

// GenerateX25519KeyPair generates a new X25519 key pair
// Returns public key (32 bytes), private key (32 bytes), and error
func GenerateX25519KeyPair() (pubKey, privKey []byte, err error) {
	privKey = make([]byte, SharedSecretSize)
	if _, err := io.ReadFull(rand.Reader, privKey); err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	var privArray [32]byte
	copy(privArray[:], privKey)
	result, err := curve25519.X25519(privArray[:], curve25519.Basepoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compute public key: %w", err)
	}
	pubKey = make([]byte, SharedSecretSize)
	copy(pubKey, result)
	return pubKey, privKey, nil
}

// EncryptWithSharedSecret encrypts a message using a shared secret
// Derives an encryption key from the shared secret using HKDF
// Returns nonce and ciphertext (caller should store both for decryption)
func EncryptWithSharedSecret(sharedSecret, plaintext []byte) (nonce, ciphertext []byte, err error) {
	if len(sharedSecret) != SharedSecretSize {
		return nil, nil, fmt.Errorf("shared secret must be %d bytes", SharedSecretSize)
	}
	// Derive encryption key from shared secret
	key, err := DeriveKey(sharedSecret, nil, []byte("encryption"), KeySize)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to derive encryption key: %w", err)
	}
	// Generate nonce
	nonce, err = GenerateNonce()
	if err != nil {
		return nil, nil, err
	}
	// Encrypt
	ciphertext, err = Encrypt(key, nonce, plaintext)
	if err != nil {
		return nil, nil, err
	}
	return nonce, ciphertext, nil
}

// DecryptWithSharedSecret decrypts a message using a shared secret
// Derives an encryption key from the shared secret using HKDF
func DecryptWithSharedSecret(sharedSecret, nonce, ciphertext []byte) ([]byte, error) {
	if len(sharedSecret) != SharedSecretSize {
		return nil, fmt.Errorf("shared secret must be %d bytes", SharedSecretSize)
	}
	// Derive encryption key from shared secret
	key, err := DeriveKey(sharedSecret, nil, []byte("encryption"), KeySize)
	if err != nil {
		return nil, fmt.Errorf("failed to derive encryption key: %w", err)
	}
	// Decrypt
	return Decrypt(key, nonce, ciphertext)
}
