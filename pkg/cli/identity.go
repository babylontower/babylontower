package cli

import (
	"crypto/ed25519"
)

// Identity is a type alias for the identity package's Identity
// This avoids circular imports
type Identity struct {
	Ed25519PubKey  ed25519.PublicKey
	Ed25519PrivKey ed25519.PrivateKey
	X25519PubKey   []byte
	X25519PrivKey  []byte
}
