package reputation

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"

	pb "babylontower/pkg/proto"
	"babylontower/pkg/crypto"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multihash"
	"google.golang.org/protobuf/proto"
)

// AttestationExchange handles publishing and retrieving reputation attestations via DHT
type AttestationExchange struct {
	tracker    *Tracker
	ipfsNode   IPFSNode
	privKey    ed25519.PrivateKey
	identityPub string // hex-encoded IK_sign.pub
}

// IPFSNode interface for DHT operations
type IPFSNode interface {
	Publish(ctx context.Context, topic string, data []byte) error
	Subscribe(ctx context.Context, topic string) (<-chan []byte, error)
	Add(ctx context.Context, data []byte) (cid.Cid, error)
	Get(ctx context.Context, c cid.Cid) ([]byte, error)
	PublishTo(ctx context.Context, recipientPubKey []byte, data []byte) error
}

// NewAttestationExchange creates a new attestation exchange
func NewAttestationExchange(tracker *Tracker, ipfsNode IPFSNode, privKey ed25519.PrivateKey, identityPub string) *AttestationExchange {
	return &AttestationExchange{
		tracker:     tracker,
		ipfsNode:    ipfsNode,
		privKey:     privKey,
		identityPub: identityPub,
	}
}

// CreateAttestation creates a signed attestation for a peer
func (e *AttestationExchange) CreateAttestation(subject peer.ID, score float64, observationHours uint64) (*Attestation, error) {
	now := time.Now()

	attestation := &Attestation{
		AttesterPeerID:         e.tracker.selfPeerID,
		AttesterIdentityPub:    e.identityPub,
		SubjectPeerID:          subject,
		Score:                  score,
		ObservationPeriodHours: observationHours,
		Timestamp:              now,
	}

	// Create canonical representation for signing
	protoAtt := &pb.ReputationAttestation{
		AttesterPeerId:         []byte(attestation.AttesterPeerID),
		SubjectPeerId:          []byte(attestation.SubjectPeerID),
		Score:                  float32(attestation.Score),
		ObservationPeriodHours: attestation.ObservationPeriodHours,
		Timestamp:              uint64(attestation.Timestamp.Unix()),
		AttesterIdentityPub:    attestation.AttesterIdentityPub,
	}

	data, err := proto.Marshal(protoAtt)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal attestation: %w", err)
	}

	// Sign the attestation
	signature, err := crypto.Sign(e.privKey, data)
	if err != nil {
		return nil, fmt.Errorf("failed to sign attestation: %w", err)
	}
	attestation.Signature = signature

	return attestation, nil
}

// VerifyAttestation verifies an attestation's signature
func (e *AttestationExchange) VerifyAttestation(attestation *Attestation) error {
	// Parse attester identity public key
	identityPubBytes, err := hex.DecodeString(attestation.AttesterIdentityPub)
	if err != nil {
		return fmt.Errorf("failed to decode attester identity pubkey: %w", err)
	}

	// Create canonical representation
	protoAtt := &pb.ReputationAttestation{
		AttesterPeerId:         []byte(attestation.AttesterPeerID),
		SubjectPeerId:          []byte(attestation.SubjectPeerID),
		Score:                  float32(attestation.Score),
		ObservationPeriodHours: attestation.ObservationPeriodHours,
		Timestamp:              uint64(attestation.Timestamp.Unix()),
		AttesterIdentityPub:    attestation.AttesterIdentityPub,
	}

	data, err := proto.Marshal(protoAtt)
	if err != nil {
		return fmt.Errorf("failed to marshal attestation: %w", err)
	}

	// Verify signature
	if !crypto.Verify(identityPubBytes, data, attestation.Signature) {
		return ErrInvalidSignature
	}

	return nil
}

// PublishAttestation publishes an attestation to the DHT
func (e *AttestationExchange) PublishAttestation(ctx context.Context, attestation *Attestation) (cid.Cid, error) {
	// Verify attestation first
	if err := e.VerifyAttestation(attestation); err != nil {
		return cid.Cid{}, err
	}

	// Create envelope
	envelope := &pb.ReputationAttestationEnvelope{
		Attestation: &pb.ReputationAttestation{
			AttesterPeerId:         []byte(attestation.AttesterPeerID),
			SubjectPeerId:          []byte(attestation.SubjectPeerID),
			Score:                  float32(attestation.Score),
			ObservationPeriodHours: attestation.ObservationPeriodHours,
			Timestamp:              uint64(attestation.Timestamp.Unix()),
			Signature:              attestation.Signature,
			AttesterIdentityPub:    attestation.AttesterIdentityPub,
		},
		AttesterSignature: attestation.Signature,
	}

	data, err := proto.Marshal(envelope)
	if err != nil {
		return cid.Cid{}, fmt.Errorf("failed to marshal envelope: %w", err)
	}

	// Store in IPFS
	c, err := e.ipfsNode.Add(ctx, data)
	if err != nil {
		return cid.Cid{}, fmt.Errorf("failed to add attestation to IPFS: %w", err)
	}

	return c, nil
}

// PublishAttestationToPeer publishes an attestation directly to a peer
func (e *AttestationExchange) PublishAttestationToPeer(ctx context.Context, target peer.ID, attestation *Attestation) error {
	// Verify attestation first
	if err := e.VerifyAttestation(attestation); err != nil {
		return err
	}

	envelope := &pb.ReputationAttestationEnvelope{
		Attestation: &pb.ReputationAttestation{
			AttesterPeerId:         []byte(attestation.AttesterPeerID),
			SubjectPeerId:          []byte(attestation.SubjectPeerID),
			Score:                  float32(attestation.Score),
			ObservationPeriodHours: attestation.ObservationPeriodHours,
			Timestamp:              uint64(attestation.Timestamp.Unix()),
			Signature:              attestation.Signature,
			AttesterIdentityPub:    attestation.AttesterIdentityPub,
		},
		AttesterSignature: attestation.Signature,
	}

	data, err := proto.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("failed to marshal envelope: %w", err)
	}

	// Get target's public key from peer ID
	// Note: In libp2p, peer.ID is derived from the public key
	// For simplicity, we use the peer.ID bytes directly
	targetPubBytes := []byte(target)

	return e.ipfsNode.PublishTo(ctx, targetPubBytes, data)
}

// GetAttestation retrieves an attestation from IPFS by CID
func (e *AttestationExchange) GetAttestation(ctx context.Context, c cid.Cid) (*Attestation, error) {
	data, err := e.ipfsNode.Get(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("failed to get attestation from IPFS: %w", err)
	}

	envelope := &pb.ReputationAttestationEnvelope{}
	if err := proto.Unmarshal(data, envelope); err != nil {
		return nil, fmt.Errorf("failed to unmarshal envelope: %w", err)
	}

	attestation := &Attestation{
		AttesterPeerID:         peer.ID(envelope.Attestation.AttesterPeerId),
		AttesterIdentityPub:    envelope.Attestation.AttesterIdentityPub,
		SubjectPeerID:          peer.ID(envelope.Attestation.SubjectPeerId),
		Score:                  float64(envelope.Attestation.Score),
		ObservationPeriodHours: envelope.Attestation.ObservationPeriodHours,
		Timestamp:              time.Unix(int64(envelope.Attestation.Timestamp), 0),
		Signature:              envelope.Attestation.Signature,
	}

	// Verify attestation
	if err := e.VerifyAttestation(attestation); err != nil {
		return nil, err
	}

	return attestation, nil
}

// RequestAttestations requests attestations for a peer from the network
func (e *AttestationExchange) RequestAttestations(ctx context.Context, target peer.ID) ([]*Attestation, error) {
	// Create query
	query := &pb.ReputationQuery{
		TargetPeerId: []byte(target),
		Timestamp:    uint64(time.Now().Unix()),
		QuerierPubkey: []byte(e.tracker.selfPeerID),
	}

	data, err := proto.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	// Publish query to reputation topic
	topic := fmt.Sprintf("babylon-rep-%s", hex.EncodeToString([]byte(target)[:8]))
	if err := e.ipfsNode.Publish(ctx, topic, data); err != nil {
		return nil, fmt.Errorf("failed to publish query: %w", err)
	}

	// Subscribe for responses
	responseChan, err := e.ipfsNode.Subscribe(ctx, topic)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe for responses: %w", err)
	}

	var attestations []*Attestation
	timeout := time.After(5 * time.Second)

	for {
		select {
		case data := <-responseChan:
			response := &pb.ReputationResponse{}
			if err := proto.Unmarshal(data, response); err != nil {
				continue
			}

			// Extract attestations from response
			for _, protoAtt := range response.Attestations {
				attestation := &Attestation{
					AttesterPeerID:         peer.ID(protoAtt.AttesterPeerId),
					AttesterIdentityPub:    protoAtt.AttesterIdentityPub,
					SubjectPeerID:          peer.ID(protoAtt.SubjectPeerId),
					Score:                  float64(protoAtt.Score),
					ObservationPeriodHours: protoAtt.ObservationPeriodHours,
					Timestamp:              time.Unix(int64(protoAtt.Timestamp), 0),
					Signature:              protoAtt.Signature,
				}

				// Verify attestation
				if err := e.VerifyAttestation(attestation); err != nil {
					continue
				}

				attestations = append(attestations, attestation)
			}

		case <-timeout:
			return attestations, nil
		case <-ctx.Done():
			return attestations, ctx.Err()
		}
	}
}

// RespondToAttestationRequest handles incoming attestation requests
func (e *AttestationExchange) RespondToAttestationRequest(ctx context.Context, query *pb.ReputationQuery) error {
	target := peer.ID(query.TargetPeerId)
	record := e.tracker.GetRecord(target)

	if record == nil {
		return nil // No record to share
	}

	// Get attestations for this peer
	record.mu.RLock()
	attestations := make([]*pb.ReputationAttestation, len(record.attestations))
	for i, att := range record.attestations {
		attestations[i] = &pb.ReputationAttestation{
			AttesterPeerId:         []byte(att.AttesterPeerID),
			SubjectPeerId:          []byte(att.SubjectPeerID),
			Score:                  float32(att.Score),
			ObservationPeriodHours: att.ObservationPeriodHours,
			Timestamp:              uint64(att.Timestamp.Unix()),
			Signature:              att.Signature,
			AttesterIdentityPub:    att.AttesterIdentityPub,
		}
	}
	record.mu.RUnlock()

	// Create response
	response := &pb.ReputationResponse{
		TargetPeerId:   []byte(target),
		Record:         record.ToProto(),
		Attestations:   attestations,
		Timestamp:      uint64(time.Now().Unix()),
		ResponderPubkey: []byte(e.tracker.selfPeerID),
	}

	data, err := proto.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	// Publish response to query topic
	topic := fmt.Sprintf("babylon-rep-%s", hex.EncodeToString(query.TargetPeerId[:8]))
	return e.ipfsNode.Publish(ctx, topic, data)
}

// ComputeDHTKey computes the DHT key for storing reputation data
func ComputeDHTKey(peerID peer.ID) string {
	// Use SHA256 hash of peer ID for DHT key
	hash, err := multihash.Sum([]byte(peerID), multihash.SHA2_256, -1)
	if err != nil {
		return ""
	}
	// Extract the digest from the multihash
	return fmt.Sprintf("/bt/rep/%s", hex.EncodeToString(hash[2:]))
}

// PublishReputationRecord publishes a peer's reputation record to the DHT
func (e *AttestationExchange) PublishReputationRecord(ctx context.Context, pid peer.ID) (cid.Cid, error) {
	record := e.tracker.GetRecord(pid)
	if record == nil {
		return cid.Cid{}, fmt.Errorf("no record found for peer")
	}

	data, err := proto.Marshal(record.ToProto())
	if err != nil {
		return cid.Cid{}, fmt.Errorf("failed to marshal record: %w", err)
	}

	c, err := e.ipfsNode.Add(ctx, data)
	if err != nil {
		return cid.Cid{}, fmt.Errorf("failed to add record to IPFS: %w", err)
	}

	return c, nil
}

// GetReputationRecord retrieves a reputation record from the DHT
func (e *AttestationExchange) GetReputationRecord(ctx context.Context, pid peer.ID, c cid.Cid) (*Record, error) {
	data, err := e.ipfsNode.Get(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("failed to get record from IPFS: %w", err)
	}

	protoRecord := &pb.PeerReputationRecord{}
	if err := proto.Unmarshal(data, protoRecord); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	// Convert protobuf record to local Record
	// Note: This is a simplified conversion - in production you'd want to validate all fields
	metrics := &Metrics{
		RelayReliability:        float64(protoRecord.Metrics.RelayReliability),
		RelaySuccessCount:       protoRecord.Metrics.RelaySuccessCount,
		RelayTotalCount:         protoRecord.Metrics.RelayTotalCount,
		UptimeConsistency:       float64(protoRecord.Metrics.UptimeConsistency),
		HoursOnline7d:           protoRecord.Metrics.HoursOnline_7D,
		LastSeen:                time.Unix(int64(protoRecord.Metrics.LastSeen), 0),
		MailboxReliability:      float64(protoRecord.Metrics.MailboxReliability),
		MailboxRetrievedCount:   protoRecord.Metrics.MailboxRetrievedCount,
		MailboxDepositedCount:   protoRecord.Metrics.MailboxDepositedCount,
		DHTResponsiveness:       float64(protoRecord.Metrics.DhtResponsiveness),
		AvgResponseMS:           float64(protoRecord.Metrics.AvgResponseMs),
		DHTQueryCount:           protoRecord.Metrics.DhtQueryCount,
		ContentServing:          float64(protoRecord.Metrics.ContentServing),
		BlocksServedCount:       protoRecord.Metrics.BlocksServedCount,
		BlocksRequestedCount:    protoRecord.Metrics.BlocksRequestedCount,
		FirstObserved:           time.Unix(int64(protoRecord.Metrics.FirstObserved), 0),
		ObservationCount:        protoRecord.Metrics.ObservationCount,
	}

	attestations := make([]*Attestation, len(protoRecord.Attestations))
	for i, protoAtt := range protoRecord.Attestations {
		attestations[i] = &Attestation{
			AttesterPeerID:         peer.ID(protoAtt.AttesterPeerId),
			AttesterIdentityPub:    protoAtt.AttesterIdentityPub,
			SubjectPeerID:          peer.ID(protoAtt.SubjectPeerId),
			Score:                  float64(protoAtt.Score),
			ObservationPeriodHours: protoAtt.ObservationPeriodHours,
			Timestamp:              time.Unix(int64(protoAtt.Timestamp), 0),
			Signature:              protoAtt.Signature,
		}
	}

	return &Record{
		peerID:          pid,
		metrics:         metrics,
		compositeScore:  float64(protoRecord.CompositeScore),
		tier:            Tier(protoRecord.Tier),
		attestations:    attestations,
		trustAdjustment: float64(protoRecord.TrustAdjustment),
		lastUpdated:     time.Now(),
	}, nil
}
