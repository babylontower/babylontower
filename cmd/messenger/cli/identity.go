package cli

import (
	"crypto/ed25519"
	
	"babylontower/pkg/identity"
)

// Identity wraps the identity package's Identity for CLI use
// This avoids circular imports while providing needed functionality
type Identity struct {
	Ed25519PubKey  ed25519.PublicKey
	Ed25519PrivKey ed25519.PrivateKey
	X25519PubKey   []byte
	X25519PrivKey  []byte
	Mnemonic       string
}

// ComputeFingerprint computes the identity fingerprint
func (i *Identity) ComputeFingerprint() (string, error) {
	// Create a temporary identity package identity to compute fingerprint
	tempIdentity := &identity.Identity{
		Ed25519PubKey:  i.Ed25519PubKey,
		Ed25519PrivKey: i.Ed25519PrivKey,
		X25519PubKey:   i.X25519PubKey,
		X25519PrivKey:  i.X25519PrivKey,
		Mnemonic:       i.Mnemonic,
	}
	return tempIdentity.ComputeFingerprint()
}
