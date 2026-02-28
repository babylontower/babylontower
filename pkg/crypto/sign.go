package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrInvalidSignature is returned when signature verification fails
	ErrInvalidSignature = errors.New("invalid signature")
	// ErrInvalidPrivateKey is returned when a private key is invalid
	ErrInvalidPrivateKey = errors.New("invalid private key")
	// ErrInvalidPublicKey is returned when a public key is invalid
	ErrInvalidPublicKey = errors.New("invalid public key")
)

// Sign signs a message using Ed25519
// Returns a 64-byte signature
func Sign(privKey ed25519.PrivateKey, message []byte) ([]byte, error) {
	if len(privKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: expected %d bytes, got %d", ErrInvalidPrivateKey, ed25519.PrivateKeySize, len(privKey))
	}
	signature := ed25519.Sign(privKey, message)
	return signature, nil
}

// Verify verifies an Ed25519 signature
// Returns true if the signature is valid, false otherwise
func Verify(pubKey ed25519.PublicKey, message, signature []byte) bool {
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	if len(signature) != SignatureSize {
		return false
	}
	return ed25519.Verify(pubKey, message, signature)
}

// GenerateEd25519KeyPair generates a new Ed25519 key pair
// Returns public and private keys
func GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate Ed25519 key pair: %w", err)
	}
	return pubKey, privKey, nil
}

// SignAndEncode signs a message and returns the signature as a hex string
func SignAndEncode(privKey ed25519.PrivateKey, message []byte) (string, error) {
	signature, err := Sign(privKey, message)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", signature), nil
}

// VerifyAndDecode verifies a signature from hex string
func VerifyAndDecode(pubKey ed25519.PublicKey, message []byte, signatureHex string) bool {
	signature, err := hexDecode(signatureHex)
	if err != nil {
		return false
	}
	return Verify(pubKey, message, signature)
}

// ValidatePrivateKey checks if a private key is valid
func ValidatePrivateKey(privKey ed25519.PrivateKey) bool {
	if len(privKey) != ed25519.PrivateKeySize {
		return false
	}
	// Verify the key can sign and verify
	testMessage := []byte("test")
	signature, err := Sign(privKey, testMessage)
	if err != nil {
		return false
	}
	pubKey := privKey.Public().(ed25519.PublicKey)
	return Verify(pubKey, testMessage, signature)
}

// ValidatePublicKey checks if a public key is valid
func ValidatePublicKey(pubKey ed25519.PublicKey) bool {
	return len(pubKey) == ed25519.PublicKeySize
}

// hexDecode decodes a hex string to bytes
func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd length hex string")
	}
	result := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		var b byte
		_, err := fmt.Sscanf(s[i:i+2], "%02x", &b)
		if err != nil {
			return nil, err
		}
		result[i/2] = b
	}
	return result, nil
}

// RandomBytes generates random bytes of specified length
func RandomBytes(length int) ([]byte, error) {
	if length <= 0 {
		return nil, fmt.Errorf("length must be positive")
	}
	bytes := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return bytes, nil
}
