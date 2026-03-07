package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	pb "babylontower/pkg/proto"

	"github.com/ipfs/go-log/v2"
	"google.golang.org/protobuf/proto"
)

var logger = log.Logger("babylontower/identity")

// DHTIdentityManager handles DHT publication and retrieval of identity documents
type DHTIdentityManager struct {
	dhtClient DHTClient
}

// DHTClient is an interface for DHT operations (implemented by ipfsnode.Node)
type DHTClient interface {
	// PutToDHT stores a value in the DHT with the given key
	// For Babylon protocol keys (/bt/id/, /bt/prekeys/), uses Babylon DHT
	PutToDHT(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// GetFromDHT retrieves a value from the DHT by key
	// For Babylon protocol keys, uses Babylon DHT
	GetFromDHT(ctx context.Context, key string) ([]byte, error)
	// GetClosestPeers finds peers closest to a given key
	GetClosestPeers(ctx context.Context, key string) ([]string, error)
	// IsBabylonDHTReady returns true if Babylon DHT is ready for protocol operations
	IsBabylonDHTReady() bool
	// WaitForBabylonDHT waits for Babylon DHT to be ready
	WaitForBabylonDHT(timeout time.Duration) error
}

// NewDHTIdentityManager creates a new DHT identity manager
func NewDHTIdentityManager(dhtClient DHTClient) *DHTIdentityManager {
	return &DHTIdentityManager{
		dhtClient: dhtClient,
	}
}

// PublishIdentityDocument publishes an IdentityDocument to the DHT
// The document is stored at /bt/id/<hex(SHA256(IK_sign.pub)[:16])>
// Uses Babylon DHT for protocol-layer storage
func (m *DHTIdentityManager) PublishIdentityDocument(ctx context.Context, doc *pb.IdentityDocument) error {
	// Check if Babylon DHT is ready
	if waiter, ok := m.dhtClient.(interface{ IsBabylonDHTReady() bool }); ok {
		if !waiter.IsBabylonDHTReady() {
			logger.Warnw("Babylon DHT not ready, identity publication may fail")
		}
	}

	// Serialize the document
	data, err := proto.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal identity document: %w", err)
	}

	// Derive DHT key
	dhtKey := DeriveIdentityDHTKey(doc.IdentitySignPub)

	// Store in Babylon DHT with 24-hour TTL (republish every 4 hours)
	ttl := 24 * time.Hour
	if err := m.dhtClient.PutToDHT(ctx, dhtKey, data, ttl); err != nil {
		return fmt.Errorf("failed to publish identity document to DHT: %w", err)
	}

	logger.Infow("published identity document to Babylon DHT", "dht_key", dhtKey)
	return nil
}

// FetchIdentityDocument retrieves an IdentityDocument from the DHT
func (m *DHTIdentityManager) FetchIdentityDocument(ctx context.Context, identityPub []byte) (*pb.IdentityDocument, error) {
	// Derive DHT key
	dhtKey := DeriveIdentityDHTKey(identityPub)

	// Fetch from DHT
	data, err := m.dhtClient.GetFromDHT(ctx, dhtKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch identity document from DHT: %w", err)
	}

	// Unmarshal
	var doc pb.IdentityDocument
	if err := proto.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal identity document: %w", err)
	}

	// Verify the document
	if err := VerifyIdentityDocument(&doc); err != nil {
		return nil, fmt.Errorf("invalid identity document: %w", err)
	}

	return &doc, nil
}

// PublishPrekeyBundle publishes a prekey bundle to the DHT
// The bundle is stored at /bt/prekeys/<hex(SHA256(IK_sign.pub)[:16])>
func (m *DHTIdentityManager) PublishPrekeyBundle(
	ctx context.Context,
	identityPub []byte,
	signedPrekeys []*pb.SignedPrekey,
	oneTimePrekeys []*pb.OneTimePrekey,
) error {
	// Create prekey bundle message
	bundle := &pb.PrekeyBundle{
		SignedPrekeys:  signedPrekeys,
		OneTimePrekeys: oneTimePrekeys,
		PublishedAt:    uint64(time.Now().Unix()),
	}

	// Serialize
	data, err := proto.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("failed to marshal prekey bundle: %w", err)
	}

	// Derive DHT key
	dhtKey := DerivePrekeyBundleDHTKey(identityPub)

	// Store in DHT with 24-hour TTL
	ttl := 24 * time.Hour
	if err := m.dhtClient.PutToDHT(ctx, dhtKey, data, ttl); err != nil {
		return fmt.Errorf("failed to publish prekey bundle to DHT: %w", err)
	}

	logger.Infow("published prekey bundle to DHT", "dht_key", dhtKey)
	return nil
}

// FetchPrekeyBundle retrieves a prekey bundle from the DHT
func (m *DHTIdentityManager) FetchPrekeyBundle(ctx context.Context, identityPub []byte) (*pb.PrekeyBundle, error) {
	// Derive DHT key
	dhtKey := DerivePrekeyBundleDHTKey(identityPub)

	// Fetch from DHT
	data, err := m.dhtClient.GetFromDHT(ctx, dhtKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch prekey bundle from DHT: %w", err)
	}

	// Unmarshal
	var bundle pb.PrekeyBundle
	if err := proto.Unmarshal(data, &bundle); err != nil {
		return nil, fmt.Errorf("failed to unmarshal prekey bundle: %w", err)
	}

	return &bundle, nil
}

// FindClosestPeers finds peers closest to an identity key in the DHT
func (m *DHTIdentityManager) FindClosestPeers(ctx context.Context, identityPub []byte) ([]string, error) {
	dhtKey := DeriveIdentityDHTKey(identityPub)
	return m.dhtClient.GetClosestPeers(ctx, dhtKey)
}

// ValidatePrekeyBundle validates signatures on all prekeys in a bundle
func ValidatePrekeyBundle(bundle *pb.PrekeyBundle, identityPub []byte) error {
	for _, spk := range bundle.SignedPrekeys {
		if err := VerifySignedPrekey(spk, identityPub); err != nil {
			return fmt.Errorf("invalid signed prekey %d: %w", spk.PrekeyId, err)
		}
	}
	// OPKs don't have signatures, so we only validate SPKs
	return nil
}

// DeriveIdentityDHTKey derives the DHT key for an identity document
// Format: /bt/id/<hex(SHA256(IK_sign.pub)[:16])>
// Per protocol spec section 1.4
func DeriveIdentityDHTKey(identityPub []byte) string {
	hash := sha256.Sum256(identityPub)
	hexPrefix := hex.EncodeToString(hash[:16])
	return "/bt/id/" + hexPrefix
}

// DerivePrekeyBundleDHTKey derives the DHT key for a prekey bundle
// Format: /bt/prekeys/<hex(SHA256(IK_sign.pub)[:16])>
// Per protocol spec section 4.2
func DerivePrekeyBundleDHTKey(identityPub []byte) string {
	hash := sha256.Sum256(identityPub)
	hexPrefix := hex.EncodeToString(hash[:16])
	return "/bt/prekeys/" + hexPrefix
}
