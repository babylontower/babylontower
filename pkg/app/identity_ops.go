package app

import (
	"fmt"
	"net/url"
	"strings"

	"babylontower/pkg/identity"

	"github.com/mr-tron/base58"
	"github.com/tyler-smith/go-bip39"
)

// NewIdentityResult contains the result of identity generation or restoration.
type NewIdentityResult struct {
	// Identity is the created identity (pass to NewApplication)
	Identity *identity.Identity
	// Mnemonic is the BIP39 mnemonic phrase (12 words)
	Mnemonic string
	// PublicKeyBase58 is the Ed25519 public key in base58
	PublicKeyBase58 string
	// X25519KeyBase58 is the X25519 public key in base58
	X25519KeyBase58 string
	// Fingerprint is the identity fingerprint for out-of-band verification
	Fingerprint string
}

// ContactLinkInfo contains parsed contact exchange link data.
type ContactLinkInfo struct {
	// PublicKeyBase58 is the Ed25519 public key in base58
	PublicKeyBase58 string
	// DisplayName is the optional display name
	DisplayName string
	// X25519KeyBase58 is the optional X25519 encryption key in base58
	X25519KeyBase58 string
}

// GenerateNewIdentity creates a brand new identity with a fresh mnemonic.
// The caller should display the mnemonic to the user for backup,
// then call SaveIdentityToFile to persist it.
func GenerateNewIdentity() (*NewIdentityResult, error) {
	ident, err := identity.GenerateIdentity()
	if err != nil {
		return nil, fmt.Errorf("failed to generate identity: %w", err)
	}
	return buildIdentityResult(ident)
}

// RestoreIdentityFromMnemonic restores an identity from a BIP39 mnemonic phrase.
// Use ValidateMnemonic first to check the mnemonic before calling this.
func RestoreIdentityFromMnemonic(mnemonic string) (*NewIdentityResult, error) {
	mnemonic = strings.TrimSpace(mnemonic)
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, fmt.Errorf("invalid mnemonic phrase")
	}
	ident, err := identity.NewIdentity(mnemonic)
	if err != nil {
		return nil, fmt.Errorf("failed to restore identity: %w", err)
	}
	return buildIdentityResult(ident)
}

// ValidateMnemonic checks whether a BIP39 mnemonic phrase is valid.
func ValidateMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(strings.TrimSpace(mnemonic))
}

// SaveIdentityToFile persists the identity to disk.
// Only mnemonic and public keys are stored; private keys are re-derived on load.
func SaveIdentityToFile(ident *identity.Identity, filePath string) error {
	return identity.SaveIdentity(ident, filePath)
}

// LoadIdentityFromFile loads an identity from disk, re-deriving private keys.
func LoadIdentityFromFile(filePath string) (*NewIdentityResult, error) {
	ident, err := identity.LoadIdentity(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load identity: %w", err)
	}
	return buildIdentityResult(ident)
}

// IdentityFileExists checks if an identity file exists at the given path.
func IdentityFileExists(filePath string) bool {
	return identity.IdentityExists(filePath)
}

// GenerateContactLink creates a btower:// contact exchange link including the X25519 encryption key.
// Format: btower://<base58(ed25519_pubkey)>?x25519=<base58(x25519_pubkey)>[&name=<display_name>]
func GenerateContactLink(ed25519PubKey, x25519PubKey []byte, displayName string) string {
	pubKeyBase58 := base58.Encode(ed25519PubKey)
	link := "btower://" + pubKeyBase58

	params := url.Values{}
	if len(x25519PubKey) == 32 {
		params.Set("x25519", base58.Encode(x25519PubKey))
	}
	if displayName != "" {
		params.Set("name", displayName)
	}
	if len(params) > 0 {
		link += "?" + params.Encode()
	}
	return link
}

// ParseContactLink parses a btower:// contact exchange link.
// Returns the public key and optional display name.
func ParseContactLink(link string) (*ContactLinkInfo, error) {
	link = strings.TrimSpace(link)

	// Accept both btower:// and btower: prefix forms
	var pubKeyPart string
	if strings.HasPrefix(link, "btower://") {
		pubKeyPart = strings.TrimPrefix(link, "btower://")
	} else if strings.HasPrefix(link, "btower:") {
		pubKeyPart = strings.TrimPrefix(link, "btower:")
	} else {
		return nil, fmt.Errorf("invalid contact link: must start with btower://")
	}

	// Split pubkey from query params
	var pubKeyStr, queryStr string
	if idx := strings.Index(pubKeyPart, "?"); idx >= 0 {
		pubKeyStr = pubKeyPart[:idx]
		queryStr = pubKeyPart[idx+1:]
	} else {
		pubKeyStr = pubKeyPart
	}

	pubKeyStr = strings.TrimSpace(pubKeyStr)
	if pubKeyStr == "" {
		return nil, fmt.Errorf("invalid contact link: empty public key")
	}

	// Validate that it decodes to 32 bytes (Ed25519 public key)
	pubKeyBytes, err := base58.Decode(pubKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid contact link: bad base58 public key: %w", err)
	}
	if len(pubKeyBytes) != 32 {
		return nil, fmt.Errorf("invalid contact link: public key must be 32 bytes, got %d", len(pubKeyBytes))
	}

	result := &ContactLinkInfo{
		PublicKeyBase58: pubKeyStr,
	}

	// Parse query parameters
	if queryStr != "" {
		params, err := url.ParseQuery(queryStr)
		if err == nil {
			result.DisplayName = params.Get("name")
			if x25519Str := params.Get("x25519"); x25519Str != "" {
				// Validate X25519 key decodes to 32 bytes
				x25519Bytes, decErr := base58.Decode(x25519Str)
				if decErr == nil && len(x25519Bytes) == 32 {
					result.X25519KeyBase58 = x25519Str
				}
			}
		}
	}

	return result, nil
}

func buildIdentityResult(ident *identity.Identity) (*NewIdentityResult, error) {
	fingerprint, err := ident.ComputeFingerprint()
	if err != nil {
		return nil, fmt.Errorf("failed to compute fingerprint: %w", err)
	}
	return &NewIdentityResult{
		Identity:        ident,
		Mnemonic:        ident.Mnemonic,
		PublicKeyBase58: ident.PublicKeyBase58(),
		X25519KeyBase58: ident.X25519PublicKeyBase58(),
		Fingerprint:     fingerprint,
	}, nil
}
