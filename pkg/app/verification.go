package app

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/mr-tron/base58"
)

// SafetyNumber computes a safety number for two parties by hashing both
// public keys. The result is 12 groups of 5 digits, matching Signal's format.
// The computation is symmetric: SafetyNumber(A, B) == SafetyNumber(B, A).
func SafetyNumber(myPubKeyBase58, theirPubKeyBase58 string) (string, error) {
	myKey, err := base58.Decode(myPubKeyBase58)
	if err != nil {
		return "", fmt.Errorf("invalid own key: %w", err)
	}
	theirKey, err := base58.Decode(theirPubKeyBase58)
	if err != nil {
		return "", fmt.Errorf("invalid contact key: %w", err)
	}

	// Sort keys so the result is symmetric
	keys := [][]byte{myKey, theirKey}
	sort.Slice(keys, func(i, j int) bool {
		return string(keys[i]) < string(keys[j])
	})

	// SHA256(sorted key1 || sorted key2)
	combined := append(keys[0], keys[1]...)
	hash := sha256.Sum256(combined)

	// Convert to 60 decimal digits (12 groups of 5)
	num := new(big.Int).SetBytes(hash[:])
	mod := new(big.Int).SetInt64(100000) // 10^5

	groups := make([]string, 12)
	for i := 11; i >= 0; i-- {
		remainder := new(big.Int)
		num.DivMod(num, mod, remainder)
		groups[i] = fmt.Sprintf("%05d", remainder.Int64())
	}

	return strings.Join(groups, " "), nil
}

// ContactFingerprint computes a fingerprint for a contact's public key.
// Uses the same algorithm as Identity.ComputeFingerprint but works with
// raw key bytes. If x25519Key is empty, only the ed25519 key is hashed.
func ContactFingerprint(ed25519PubKeyBase58, x25519PubKeyBase58 string) (string, error) {
	edKey, err := base58.Decode(ed25519PubKeyBase58)
	if err != nil {
		return "", fmt.Errorf("invalid ed25519 key: %w", err)
	}

	combined := make([]byte, 0, 64)
	combined = append(combined, edKey...)

	if x25519PubKeyBase58 != "" {
		x25519Key, err := base58.Decode(x25519PubKeyBase58)
		if err == nil && len(x25519Key) == 32 {
			combined = append(combined, x25519Key...)
		}
	}

	hash := sha256.Sum256(combined)

	return base58.Encode(hash[:20]), nil
}
