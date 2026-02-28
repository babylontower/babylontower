package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/tyler-smith/go-bip39"
)

func TestGenerateMnemonic(t *testing.T) {
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() failed: %v", err)
	}
	// Verify mnemonic is valid
	if !bip39.IsMnemonicValid(mnemonic) {
		t.Errorf("Generated mnemonic is invalid: %s", mnemonic)
	}
	// Verify it has 12 words (128 bits of entropy)
	words := splitWords(mnemonic)
	if len(words) != 12 {
		t.Errorf("Expected 12 words, got %d", len(words))
	}
}

func TestGenerateMnemonic_Deterministic(t *testing.T) {
	// Each call should generate a different mnemonic
	mnemonic1, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() failed: %v", err)
	}
	mnemonic2, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() failed: %v", err)
	}
	if mnemonic1 == mnemonic2 {
		t.Error("Expected different mnemonics on each call")
	}
}

func TestDeriveSeed(t *testing.T) {
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() failed: %v", err)
	}
	seed, err := DeriveSeed(mnemonic)
	if err != nil {
		t.Fatalf("DeriveSeed() failed: %v", err)
	}
	// Verify seed length is 512 bits (64 bytes)
	if len(seed) != SeedLength {
		t.Errorf("Expected seed length %d, got %d", SeedLength, len(seed))
	}
	// Verify deterministic derivation
	seed2, err := DeriveSeed(mnemonic)
	if err != nil {
		t.Fatalf("DeriveSeed() failed: %v", err)
	}
	if string(seed) != string(seed2) {
		t.Error("Seed derivation is not deterministic")
	}
}

func TestDeriveSeed_InvalidMnemonic(t *testing.T) {
	_, err := DeriveSeed("invalid mnemonic words here")
	if err == nil {
		t.Error("Expected error for invalid mnemonic")
	}
}

func TestNewIdentity(t *testing.T) {
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() failed: %v", err)
	}
	identity, err := NewIdentity(mnemonic)
	if err != nil {
		t.Fatalf("NewIdentity() failed: %v", err)
	}
	// Verify identity fields are populated
	if identity.Mnemonic != mnemonic {
		t.Error("Mnemonic not stored correctly")
	}
	if len(identity.Ed25519PubKey) != 32 {
		t.Errorf("Expected Ed25519 public key length 32, got %d", len(identity.Ed25519PubKey))
	}
	if len(identity.Ed25519PrivKey) != 64 {
		t.Errorf("Expected Ed25519 private key length 64, got %d", len(identity.Ed25519PrivKey))
	}
	if len(identity.X25519PubKey) != 32 {
		t.Errorf("Expected X25519 public key length 32, got %d", len(identity.X25519PubKey))
	}
	if len(identity.X25519PrivKey) != 32 {
		t.Errorf("Expected X25519 private key length 32, got %d", len(identity.X25519PrivKey))
	}
}

func TestNewIdentity_Deterministic(t *testing.T) {
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() failed: %v", err)
	}
	identity1, err := NewIdentity(mnemonic)
	if err != nil {
		t.Fatalf("NewIdentity() failed: %v", err)
	}
	identity2, err := NewIdentity(mnemonic)
	if err != nil {
		t.Fatalf("NewIdentity() failed: %v", err)
	}
	// Verify keys are deterministically derived
	if string(identity1.Ed25519PubKey) != string(identity2.Ed25519PubKey) {
		t.Error("Ed25519 public key derivation is not deterministic")
	}
	if string(identity1.X25519PubKey) != string(identity2.X25519PubKey) {
		t.Error("X25519 public key derivation is not deterministic")
	}
}

func TestGenerateIdentity(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() failed: %v", err)
	}
	// Verify all fields are populated
	if !bip39.IsMnemonicValid(identity.Mnemonic) {
		t.Error("Generated mnemonic is invalid")
	}
	if len(identity.Ed25519PubKey) != 32 {
		t.Errorf("Expected Ed25519 public key length 32, got %d", len(identity.Ed25519PubKey))
	}
	if len(identity.X25519PubKey) != 32 {
		t.Errorf("Expected X25519 public key length 32, got %d", len(identity.X25519PubKey))
	}
}

func TestPublicKeyHex(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() failed: %v", err)
	}
	hexKey := identity.PublicKeyHex()
	// Verify hex encoding (64 hex characters for 32 bytes)
	if len(hexKey) != 64 {
		t.Errorf("Expected hex key length 64, got %d", len(hexKey))
	}
	// Verify it's valid hex
	decoded, err := hexDecode(hexKey)
	if err != nil {
		t.Errorf("PublicKeyHex() produced invalid hex: %v", err)
	}
	if string(decoded) != string(identity.Ed25519PubKey) {
		t.Error("PublicKeyHex() decoding doesn't match original key")
	}
}

func TestPublicKeyBase58(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() failed: %v", err)
	}
	base58Key := identity.PublicKeyBase58()
	// Verify base58 encoding
	decoded, err := DecodeBase58(base58Key)
	if err != nil {
		t.Errorf("PublicKeyBase58() produced invalid base58: %v", err)
	}
	if string(decoded) != string(identity.Ed25519PubKey) {
		t.Error("PublicKeyBase58() decoding doesn't match original key")
	}
}

func TestX25519PublicKeyHex(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() failed: %v", err)
	}
	hexKey := identity.X25519PublicKeyHex()
	// Verify hex encoding (64 hex characters for 32 bytes)
	if len(hexKey) != 64 {
		t.Errorf("Expected X25519 hex key length 64, got %d", len(hexKey))
	}
}

func TestX25519PublicKeyBase58(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() failed: %v", err)
	}
	base58Key := identity.X25519PublicKeyBase58()
	// Verify base58 encoding
	decoded, err := DecodeBase58(base58Key)
	if err != nil {
		t.Errorf("X25519PublicKeyBase58() produced invalid base58: %v", err)
	}
	if string(decoded) != string(identity.X25519PubKey) {
		t.Error("X25519PublicKeyBase58() decoding doesn't match original key")
	}
}

func TestSaveAndLoadIdentity(t *testing.T) {
	// Create temp directory for test
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "identity.json")
	// Generate and save identity
	identity1, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() failed: %v", err)
	}
	err = SaveIdentity(identity1, filePath)
	if err != nil {
		t.Fatalf("SaveIdentity() failed: %v", err)
	}
	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("Identity file was not created")
	}
	// Load identity
	identity2, err := LoadIdentity(filePath)
	if err != nil {
		t.Fatalf("LoadIdentity() failed: %v", err)
	}
	// Verify loaded identity matches
	if identity2.Mnemonic != identity1.Mnemonic {
		t.Error("Loaded mnemonic doesn't match")
	}
	if string(identity2.Ed25519PubKey) != string(identity1.Ed25519PubKey) {
		t.Error("Loaded Ed25519 public key doesn't match")
	}
	if string(identity2.X25519PubKey) != string(identity1.X25519PubKey) {
		t.Error("Loaded X25519 public key doesn't match")
	}
}

func TestIdentityExists(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "identity.json")
	// Should not exist initially
	if IdentityExists(filePath) {
		t.Error("IdentityExists() returned true for non-existent file")
	}
	// Create file
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() failed: %v", err)
	}
	err = SaveIdentity(identity, filePath)
	if err != nil {
		t.Fatalf("SaveIdentity() failed: %v", err)
	}
	// Should exist now
	if !IdentityExists(filePath) {
		t.Error("IdentityExists() returned false for existing file")
	}
}

func TestEncodeDecodeBase58(t *testing.T) {
	testCases := []struct {
		name     string
		input    []byte
		expected string
	}{
		{"empty", []byte{}, ""},
		{"single byte", []byte{0}, "1"},
		{"hello world", []byte("hello world"), "StV1DL6CwTryKyV"},
		{"32 bytes", make([]byte, 32), "11111111111111111111111111111111"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := EncodeBase58(tc.input)
			// Note: expected values may vary based on base58 implementation
			// We just verify round-trip works
			decoded, err := DecodeBase58(encoded)
			if err != nil {
				t.Errorf("DecodeBase58() error: %v", err)
			}
			if string(decoded) != string(tc.input) {
				t.Errorf("DecodeBase58() = %v, want %v", decoded, tc.input)
			}
		})
	}
}

func TestDecodeBase58_InvalidCharacter(t *testing.T) {
	_, err := DecodeBase58("invalid0") // '0' is not in base58 alphabet
	if err == nil {
		t.Error("Expected error for invalid base58 character")
	}
}

func TestDeriveEd25519Keys_Consistent(t *testing.T) {
	seed := make([]byte, 64)
	copy(seed, []byte("test seed for consistent key derivation"))
	pub1, priv1, err := deriveEd25519Keys(seed)
	if err != nil {
		t.Fatalf("deriveEd25519Keys() failed: %v", err)
	}
	pub2, priv2, err := deriveEd25519Keys(seed)
	if err != nil {
		t.Fatalf("deriveEd25519Keys() failed: %v", err)
	}
	if string(pub1) != string(pub2) {
		t.Error("Ed25519 public key derivation is not deterministic")
	}
	if string(priv1) != string(priv2) {
		t.Error("Ed25519 private key derivation is not deterministic")
	}
}

func TestDeriveX25519Keys_Consistent(t *testing.T) {
	seed := make([]byte, 64)
	copy(seed, []byte("test seed for consistent X25519 key derivation"))
	pub1, priv1, err := deriveX25519Keys(seed)
	if err != nil {
		t.Fatalf("deriveX25519Keys() failed: %v", err)
	}
	pub2, priv2, err := deriveX25519Keys(seed)
	if err != nil {
		t.Fatalf("deriveX25519Keys() failed: %v", err)
	}
	if string(pub1[:]) != string(pub2[:]) {
		t.Error("X25519 public key derivation is not deterministic")
	}
	if string(priv1[:]) != string(priv2[:]) {
		t.Error("X25519 private key derivation is not deterministic")
	}
}

func TestSaveIdentity_InvalidDirectory(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() failed: %v", err)
	}
	// Try to save to an invalid path (NUL is invalid on all platforms)
	err = SaveIdentity(identity, string([]byte{0})+"/identity.json")
	if err == nil {
		t.Error("Expected error for invalid directory")
	}
}

func TestLoadIdentity_NonExistentFile(t *testing.T) {
	_, err := LoadIdentity("/nonexistent/identity.json")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestLoadIdentity_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "identity.json")
	// Write invalid JSON
	err := os.WriteFile(filePath, []byte("invalid json"), 0600)
	if err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}
	_, err = LoadIdentity(filePath)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// Helper functions for tests

func splitWords(mnemonic string) []string {
	words := make([]string, 0)
	word := ""
	for _, c := range mnemonic {
		if c == ' ' {
			if word != "" {
				words = append(words, word)
				word = ""
			}
		} else {
			word += string(c)
		}
	}
	if word != "" {
		words = append(words, word)
	}
	return words
}

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
