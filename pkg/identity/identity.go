package identity

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

const (
	// MnemonicEntropyBits is the entropy size for mnemonic generation
	MnemonicEntropyBits = 128
	// SeedLength is the length of the derived seed in bytes
	SeedLength = 64
	// IdentityFilePath is the default path for identity storage
	IdentityFilePath = "identity.json"
)

// Identity represents a user's cryptographic identity
type Identity struct {
	Mnemonic       string `json:"mnemonic"`
	Seed           []byte `json:"-"` // Never serialize seed to JSON
	Ed25519PubKey  []byte `json:"ed25519_pubkey"`
	Ed25519PrivKey []byte `json:"-"` // Never serialize private key to JSON
	X25519PubKey   []byte `json:"x25519_pubkey"`
	X25519PrivKey  []byte `json:"-"` // Never serialize private key to JSON
}

// GenerateMnemonic creates a new BIP39 mnemonic phrase
// Returns a 12-word mnemonic for easy backup
func GenerateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(MnemonicEntropyBits)
	if err != nil {
		return "", fmt.Errorf("failed to generate entropy: %w", err)
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to generate mnemonic: %w", err)
	}
	return mnemonic, nil
}

// DeriveSeed derives a 512-bit seed from a BIP39 mnemonic
// Uses PBKDF2 with 2048 iterations as per BIP39 specification
func DeriveSeed(mnemonic string) ([]byte, error) {
	// Validate mnemonic first
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, errors.New("invalid mnemonic")
	}
	seed := bip39.NewSeed(mnemonic, "")
	if len(seed) != SeedLength {
		return nil, fmt.Errorf("unexpected seed length: %d", len(seed))
	}
	return seed, nil
}

// deriveEd25519Keys derives Ed25519 key pair from master secret
// Per spec: HKDF(master, salt="bt-identity", info="identity-signing-key-0")
// Ed25519 keys are used for signing and verification
func deriveEd25519Keys(masterSecret []byte) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	// Use HKDF to derive key material for signing key
	hkdfReader := hkdf.New(sha256.New, masterSecret, []byte(IdentitySalt), []byte(SigningKeyInfo))
	derivedKey := make([]byte, ed25519.SeedSize)
	if _, err := hkdfReader.Read(derivedKey); err != nil {
		return nil, nil, fmt.Errorf("failed to derive Ed25519 key: %w", err)
	}
	privKey := ed25519.NewKeyFromSeed(derivedKey)
	pubKey := privKey.Public().(ed25519.PublicKey)
	return pubKey, privKey, nil
}

// deriveX25519Keys derives X25519 key pair from master secret
// Per spec: HKDF(master, salt="bt-identity", info="identity-dh-key-0")
// X25519 keys are used for key agreement (ECDH)
func deriveX25519Keys(masterSecret []byte) (*[32]byte, *[32]byte, error) {
	// Use HKDF to derive key material for DH key
	hkdfReader := hkdf.New(sha256.New, masterSecret, []byte(IdentitySalt), []byte(DHKeyInfo))
	derivedKey := make([]byte, 32)
	if _, err := hkdfReader.Read(derivedKey); err != nil {
		return nil, nil, fmt.Errorf("failed to derive X25519 key: %w", err)
	}
	var privKey [32]byte
	var pubKey [32]byte
	copy(privKey[:], derivedKey)
	result, err := curve25519.X25519(privKey[:], curve25519.Basepoint)
	if err != nil {
		return nil, nil, fmt.Errorf("X25519 key derivation failed: %w", err)
	}
	copy(pubKey[:], result)
	return &pubKey, &privKey, nil
}

// NewIdentity creates a new identity from a mnemonic
// Derives Ed25519 and X25519 key pairs deterministically per spec:
//   BIP39 Mnemonic → PBKDF2 → 512-bit Seed
//   Seed → HKDF-SHA256(salt="bt-master-key", info="babylon-tower-v1") → Master Secret
//   Master Secret → HKDF(salt="bt-identity", info="identity-signing-key-0") → IK_sign (Ed25519)
//   Master Secret → HKDF(salt="bt-identity", info="identity-dh-key-0") → IK_dh (X25519)
func NewIdentity(mnemonic string) (*Identity, error) {
	seed, err := DeriveSeed(mnemonic)
	if err != nil {
		return nil, fmt.Errorf("failed to derive seed: %w", err)
	}
	
	// Derive master secret from seed (v1 spec-compliant derivation)
	masterSecret, err := DeriveMasterSecret(seed)
	if err != nil {
		return nil, fmt.Errorf("failed to derive master secret: %w", err)
	}
	
	edPub, edPriv, err := deriveEd25519Keys(masterSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to derive Ed25519 keys: %w", err)
	}
	xPub, xPriv, err := deriveX25519Keys(masterSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to derive X25519 keys: %w", err)
	}
	return &Identity{
		Mnemonic:       mnemonic,
		Seed:           seed,
		Ed25519PubKey:  edPub,
		Ed25519PrivKey: edPriv,
		X25519PubKey:   xPub[:],
		X25519PrivKey:  xPriv[:],
	}, nil
}

// GenerateIdentity creates a completely new identity
// Generates a new mnemonic and derives all keys
func GenerateIdentity() (*Identity, error) {
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		return nil, fmt.Errorf("failed to generate mnemonic: %w", err)
	}
	return NewIdentity(mnemonic)
}

// PublicKeyHex returns the Ed25519 public key as a hex string
func (i *Identity) PublicKeyHex() string {
	return hex.EncodeToString(i.Ed25519PubKey)
}

// PublicKeyBase58 returns the Ed25519 public key as a base58 string
func (i *Identity) PublicKeyBase58() string {
	return EncodeBase58(i.Ed25519PubKey)
}

// X25519PublicKeyHex returns the X25519 public key as a hex string
func (i *Identity) X25519PublicKeyHex() string {
	return hex.EncodeToString(i.X25519PubKey)
}

// X25519PublicKeyBase58 returns the X25519 public key as a base58 string
func (i *Identity) X25519PublicKeyBase58() string {
	return EncodeBase58(i.X25519PubKey)
}

// ComputeFingerprint computes the identity fingerprint per spec:
// Fingerprint = Base58(SHA256(IK_sign.pub ‖ IK_dh.pub)[:20]) → 27-28 characters
// This is used for out-of-band verification to prevent MITM attacks
func (i *Identity) ComputeFingerprint() (string, error) {
	// Concatenate the two public keys
	combined := make([]byte, 0, 64)
	combined = append(combined, i.Ed25519PubKey...)
	combined = append(combined, i.X25519PubKey...)
	
	// Compute SHA256 hash
	hash := sha256.Sum256(combined)
	
	// Take first 20 bytes and encode as Base58
	fingerprint := EncodeBase58(hash[:20])
	return fingerprint, nil
}

// SaveIdentity persists the identity to a JSON file
// WARNING: Only mnemonic and public keys are stored
// Private keys are derived from mnemonic on load
func SaveIdentity(identity *Identity, filePath string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	// Create serializable identity (without sensitive fields)
	serializable := &Identity{
		Mnemonic:      identity.Mnemonic,
		Ed25519PubKey: identity.Ed25519PubKey,
		X25519PubKey:  identity.X25519PubKey,
	}
	data, err := marshalJSON(serializable)
	if err != nil {
		return fmt.Errorf("failed to marshal identity: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write identity file: %w", err)
	}
	return nil
}

// LoadIdentity loads an identity from a JSON file
// Derives all keys from the stored mnemonic
func LoadIdentity(filePath string) (*Identity, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity file: %w", err)
	}
	var identity Identity
	if err := unmarshalJSON(data, &identity); err != nil {
		return nil, fmt.Errorf("failed to unmarshal identity: %w", err)
	}
	// Re-derive all keys from mnemonic
	return NewIdentity(identity.Mnemonic)
}

// IdentityExists checks if an identity file exists at the given path
func IdentityExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}
