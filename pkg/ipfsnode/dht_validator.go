package ipfsnode

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"babylontower/pkg/identity"
	pb "babylontower/pkg/proto"

	"github.com/libp2p/go-libp2p-record"
	"google.golang.org/protobuf/proto"
)

// BabylonNamespaceValidator is a delegating validator for the /bt/ namespace.
// It routes validation to specific validators based on the sub-namespace:
//   - /bt/id/ -> IdentityDocumentValidator
//   - /bt/prekeys/ -> PrekeyBundleValidator (future)
//   - /bt/username/ -> UsernameRecordValidator (future)
//   - etc.
type BabylonNamespaceValidator struct {
	identityValidator *IdentityDocumentValidator
}

// NewBabylonNamespaceValidator creates a new namespace validator with sub-validators registered
func NewBabylonNamespaceValidator() *BabylonNamespaceValidator {
	return &BabylonNamespaceValidator{
		identityValidator: &IdentityDocumentValidator{},
	}
}

// Validate verifies a record in the /bt namespace by delegating to the appropriate sub-namespace validator
func (v *BabylonNamespaceValidator) Validate(key string, value []byte) error {
	// Route to appropriate validator based on sub-namespace
	// Note: Keys are in format /bt/id/xxx or /bt/prekeys/xxx
	switch {
	case strings.HasPrefix(key, "/bt/id/"):
		return v.identityValidator.Validate(key, value)
	case strings.HasPrefix(key, "/bt/prekeys/"):
		// For now, accept prekey records without validation
		// TODO: Implement PrekeyBundleValidator
		return nil
	default:
		// Unknown sub-namespace - reject by default for security
		return fmt.Errorf("unknown Babylon sub-namespace: %s", key)
	}
}

// Select chooses between two conflicting records in the /bt namespace
func (v *BabylonNamespaceValidator) Select(key string, vals [][]byte) (int, error) {
	// Route to appropriate validator based on sub-namespace
	switch {
	case strings.HasPrefix(key, "/bt/id/"):
		return v.identityValidator.Select(key, vals)
	case strings.HasPrefix(key, "/bt/prekeys/"):
		// For prekeys, select the first valid record
		return 0, nil
	default:
		return 0, fmt.Errorf("unknown Babylon sub-namespace: %s", key)
	}
}

// RegisterDHTValidatorsForBabylonDHT registers custom validators for Babylon DHT namespaces
// This should be called after the Babylon DHT is created but before it starts serving
// Per protocol spec section 1.4, custom validators are required for /bt/id/ namespace
func (n *Node) RegisterDHTValidatorsForBabylonDHT() error {
	if n.babylonDHT == nil {
		return fmt.Errorf("Babylon DHT not initialized")
	}

	// Get the namespaced validator from the Babylon DHT
	// The libp2p-kad-dht uses a NamespacedValidator by default
	nsValidator, ok := n.babylonDHT.Validator.(record.NamespacedValidator)
	if !ok {
		// This should never happen with standard libp2p-kad-dht
		// If it does, the DHT configuration is incorrect for Babylon protocol
		return fmt.Errorf("Babylon DHT validator is not a NamespacedValidator - Babylon protocol requires NamespacedValidator for custom namespace validation")
	}

	// Register validator for "bt" namespace (delegates to sub-namespace validators).
	// NamespacedValidator uses bare namespace names: key "/bt/id/x" → namespace "bt".
	validator := NewBabylonNamespaceValidator()
	nsValidator["bt"] = validator

	logger.Infow("Registered custom DHT validators for Babylon namespaces",
		"namespace", "bt",
		"validator", "BabylonNamespaceValidator",
		"dht", "babylon")

	return nil
}

// RegisterDHTValidators registers custom validators for the default DHT
// Deprecated: Use RegisterDHTValidatorsForBabylonDHT() instead
// This is kept for backward compatibility
func (n *Node) RegisterDHTValidators() error {
	if n.dht == nil {
		return fmt.Errorf("DHT not initialized")
	}

	// Get the namespaced validator from the DHT
	// The libp2p-kad-dht uses a NamespacedValidator by default
	nsValidator, ok := n.dht.Validator.(record.NamespacedValidator)
	if !ok {
		// This should never happen with standard libp2p-kad-dht
		// If it does, the DHT configuration is incorrect for Babylon protocol
		return fmt.Errorf("DHT validator is not a NamespacedValidator - Babylon protocol requires NamespacedValidator for custom namespace validation")
	}

	// Register validator for "bt" namespace (identity documents).
	// NamespacedValidator uses bare namespace names: key "/bt/id/x" → namespace "bt".
	validator := NewBabylonNamespaceValidator()
	nsValidator["bt"] = validator

	logger.Infow("Registered custom DHT validators",
		"namespace", "bt",
		"validator", "BabylonNamespaceValidator",
		"dht", "default")

	return nil
}

// IdentityDocumentValidator validates and selects identity documents in the DHT
type IdentityDocumentValidator struct{}

// Validate verifies an identity document record
// Per spec section 1.4, must:
// 1. Verify Ed25519 signature against identity_sign_pub
// 2. Verify pubkey hashes match the DHT record key
func (v *IdentityDocumentValidator) Validate(key string, value []byte) error {
	// Parse the protobuf identity document
	doc := &pb.IdentityDocument{}
	if err := proto.Unmarshal(value, doc); err != nil {
		return fmt.Errorf("failed to unmarshal identity document: %w", err)
	}

	// Validate basic structure
	if err := v.validateStructure(doc); err != nil {
		return err
	}

	// Verify signature
	if err := v.verifySignature(doc); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	// Verify pubkey hash matches DHT key
	if err := v.verifyPubkeyHash(key, doc); err != nil {
		return fmt.Errorf("pubkey hash verification failed: %w", err)
	}

	return nil
}

// validateStructure checks basic document structure
func (v *IdentityDocumentValidator) validateStructure(doc *pb.IdentityDocument) error {
	// Check required fields
	if len(doc.IdentitySignPub) != 32 {
		return fmt.Errorf("invalid identity_sign_pub length: %d", len(doc.IdentitySignPub))
	}
	if len(doc.IdentityDhPub) != 32 {
		return fmt.Errorf("invalid identity_dh_pub length: %d", len(doc.IdentityDhPub))
	}
	if doc.Sequence == 0 {
		return fmt.Errorf("sequence must be > 0")
	}
	if len(doc.Signature) != 64 {
		return fmt.Errorf("invalid signature length: %d", len(doc.Signature))
	}

	return nil
}

// verifySignature verifies the Ed25519 signature against identity_sign_pub
func (v *IdentityDocumentValidator) verifySignature(doc *pb.IdentityDocument) error {
	// Use the canonical serialization from pkg/identity (must match the signer exactly)
	data, err := identity.SerializeDocumentForSigning(doc)
	if err != nil {
		return fmt.Errorf("failed to serialize for signing: %w", err)
	}

	// Verify signature
	if !ed25519.Verify(doc.IdentitySignPub, data, doc.Signature) {
		return fmt.Errorf("invalid Ed25519 signature")
	}

	return nil
}

// verifyPubkeyHash verifies the DHT key matches the pubkey hash
// DHT key format: /bt/id/<hex(SHA256(identity_sign_pub)[:16])>
func (v *IdentityDocumentValidator) verifyPubkeyHash(dhtKey string, doc *pb.IdentityDocument) error {
	// Extract the hex portion from the DHT key
	// Key format: /bt/id/<hex_hash>
	const prefix = "/bt/id/"
	if len(dhtKey) <= len(prefix) {
		return fmt.Errorf("invalid DHT key format")
	}

	hexHash := dhtKey[len(prefix):]
	expectedHash, err := hex.DecodeString(hexHash)
	if err != nil {
		return fmt.Errorf("invalid hex in DHT key: %w", err)
	}

	// Compute SHA256 of identity_sign_pub
	hash := sha256.Sum256(doc.IdentitySignPub)
	actualHash := hash[:16]

	if !bytes.Equal(expectedHash, actualHash) {
		return fmt.Errorf("pubkey hash mismatch: expected %x, got %x", expectedHash, actualHash)
	}

	return nil
}

// Select chooses between two conflicting identity documents
// Per spec: prefers higher sequence number
func (v *IdentityDocumentValidator) Select(key string, vals [][]byte) (int, error) {
	if len(vals) == 0 {
		return 0, fmt.Errorf("no values to select from")
	}
	if len(vals) == 1 {
		return 0, nil
	}

	bestIdx := 0
	var bestSequence uint64 = 0

	for i, val := range vals {
		doc := &pb.IdentityDocument{}
		if err := proto.Unmarshal(val, doc); err != nil {
			// Skip invalid records
			logger.Debugw("failed to unmarshal document for selection", "index", i, "error", err)
			continue
		}

		// Prefer higher sequence number
		if doc.Sequence > bestSequence {
			bestSequence = doc.Sequence
			bestIdx = i
		} else if doc.Sequence == bestSequence {
			// If sequences are equal, prefer higher updated_at timestamp
			if bestIdx >= 0 && len(vals) > bestIdx {
				bestDoc := &pb.IdentityDocument{}
				if err := proto.Unmarshal(vals[bestIdx], bestDoc); err == nil {
					if doc.UpdatedAt > bestDoc.UpdatedAt {
						bestIdx = i
					}
				}
			}
		}
	}

	return bestIdx, nil
}

