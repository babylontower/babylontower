package reputation

import (
	"testing"
	"time"

	pb "babylontower/pkg/proto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTracker(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	assert.NotNil(t, tracker)
	assert.Equal(t, selfPeerID, tracker.selfPeerID)
	assert.Equal(t, "identity-pub", tracker.selfIdentity)
	assert.NotNil(t, tracker.config)
	assert.Equal(t, DefaultRelayReliabilityWeight, tracker.config.RelayReliabilityWeight)
}

func TestGetOrCreateRecord(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peerID := peer.ID("peer-1")
	record := tracker.GetOrCreateRecord(peerID)

	assert.NotNil(t, record)
	assert.Equal(t, peerID, record.peerID)
	assert.Equal(t, TierBasic, record.tier)
	assert.Equal(t, 0.0, record.compositeScore)
	assert.NotNil(t, record.metrics)

	// Getting same record again should return same instance
	record2 := tracker.GetOrCreateRecord(peerID)
	assert.Equal(t, record, record2)
}

func TestGetRecord(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peerID := peer.ID("peer-1")

	// Non-existent record should return nil
	record := tracker.GetRecord(peerID)
	assert.Nil(t, record)

	// Create record and retrieve
	tracker.GetOrCreateRecord(peerID)
	record = tracker.GetRecord(peerID)
	assert.NotNil(t, record)
}

func TestRecordRelayEvent(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peerID := peer.ID("peer-1")

	// Record successful relay
	tracker.RecordRelayEvent(peerID, true)
	record := tracker.GetRecord(peerID)
	assert.Equal(t, uint64(1), record.metrics.RelayTotalCount)
	assert.Equal(t, uint64(1), record.metrics.RelaySuccessCount)
	assert.Equal(t, 1.0, record.metrics.RelayReliability)

	// Record failed relay
	tracker.RecordRelayEvent(peerID, false)
	assert.Equal(t, uint64(2), record.metrics.RelayTotalCount)
	assert.Equal(t, uint64(1), record.metrics.RelaySuccessCount)
	assert.Equal(t, 0.5, record.metrics.RelayReliability)

	// Record another success
	tracker.RecordRelayEvent(peerID, true)
	assert.Equal(t, uint64(3), record.metrics.RelayTotalCount)
	assert.Equal(t, uint64(2), record.metrics.RelaySuccessCount)
	assert.InDelta(t, 0.667, record.metrics.RelayReliability, 0.001)
}

func TestRecordUptimeObservation(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peerID := peer.ID("peer-1")

	// Record online observation
	tracker.RecordUptimeObservation(peerID, true)
	record := tracker.GetRecord(peerID)
	assert.Equal(t, uint64(1), record.metrics.HoursOnline7d)
	assert.InDelta(t, 1.0/168.0, record.metrics.UptimeConsistency, 0.001)

	// Record more observations
	for i := 0; i < 83; i++ {
		tracker.RecordUptimeObservation(peerID, true)
	}
	assert.Equal(t, uint64(84), record.metrics.HoursOnline7d)
	assert.InDelta(t, 0.5, record.metrics.UptimeConsistency, 0.01)

	// Cap at 168 hours
	for i := 0; i < 100; i++ {
		tracker.RecordUptimeObservation(peerID, true)
	}
	assert.Equal(t, uint64(168), record.metrics.HoursOnline7d)
	assert.Equal(t, 1.0, record.metrics.UptimeConsistency)
}

func TestRecordMailboxEvent(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peerID := peer.ID("peer-1")

	// Record deposit
	tracker.RecordMailboxEvent(peerID, true, false)
	record := tracker.GetRecord(peerID)
	assert.Equal(t, uint64(1), record.metrics.MailboxDepositedCount)
	assert.Equal(t, uint64(0), record.metrics.MailboxRetrievedCount)
	assert.Equal(t, 0.0, record.metrics.MailboxReliability)

	// Record retrieval
	tracker.RecordMailboxEvent(peerID, false, true)
	assert.Equal(t, uint64(1), record.metrics.MailboxDepositedCount)
	assert.Equal(t, uint64(1), record.metrics.MailboxRetrievedCount)
	assert.Equal(t, 1.0, record.metrics.MailboxReliability)

	// More deposits without retrieval
	tracker.RecordMailboxEvent(peerID, true, false)
	tracker.RecordMailboxEvent(peerID, true, false)
	assert.Equal(t, uint64(3), record.metrics.MailboxDepositedCount)
	assert.Equal(t, uint64(1), record.metrics.MailboxRetrievedCount)
	assert.InDelta(t, 0.333, record.metrics.MailboxReliability, 0.001)
}

func TestRecordDHTQuery(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peerID := peer.ID("peer-1")

	// Record fast query (100ms)
	tracker.RecordDHTQuery(peerID, 100)
	record := tracker.GetRecord(peerID)
	assert.Equal(t, uint64(1), record.metrics.DHTQueryCount)
	assert.Equal(t, float64(100), record.metrics.AvgResponseMS)
	assert.InDelta(t, 0.98, record.metrics.DHTResponsiveness, 0.01)

	// Record slow query (2000ms)
	tracker.RecordDHTQuery(peerID, 2000)
	assert.Equal(t, uint64(2), record.metrics.DHTQueryCount)
	assert.InDelta(t, 1050, record.metrics.AvgResponseMS, 1)
	assert.InDelta(t, 0.79, record.metrics.DHTResponsiveness, 0.01)

	// Record very slow query (should clamp responsiveness)
	tracker.RecordDHTQuery(peerID, 10000)
	assert.InDelta(t, 4033.33, record.metrics.AvgResponseMS, 1)
	assert.GreaterOrEqual(t, record.metrics.DHTResponsiveness, 0.0)
	assert.LessOrEqual(t, record.metrics.DHTResponsiveness, 1.0)
}

func TestRecordContentEvent(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peerID := peer.ID("peer-1")

	// Record content request
	tracker.RecordContentEvent(peerID, false, true)
	record := tracker.GetRecord(peerID)
	assert.Equal(t, uint64(1), record.metrics.BlocksRequestedCount)
	assert.Equal(t, uint64(0), record.metrics.BlocksServedCount)
	assert.Equal(t, 0.0, record.metrics.ContentServing)

	// Record content served
	tracker.RecordContentEvent(peerID, true, false)
	assert.Equal(t, uint64(1), record.metrics.BlocksRequestedCount)
	assert.Equal(t, uint64(1), record.metrics.BlocksServedCount)
	assert.Equal(t, 1.0, record.metrics.ContentServing)
}

func TestSetTrustAdjustment(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peerID := peer.ID("peer-1")

	// Set positive adjustment
	tracker.SetTrustAdjustment(peerID, 0.3)
	record := tracker.GetRecord(peerID)
	assert.Equal(t, 0.3, record.trustAdjustment)

	// Set negative adjustment
	tracker.SetTrustAdjustment(peerID, -0.2)
	assert.Equal(t, -0.2, record.trustAdjustment)

	// Clamp to -0.5
	tracker.SetTrustAdjustment(peerID, -1.0)
	assert.Equal(t, -0.5, record.trustAdjustment)

	// Clamp to 0.5
	tracker.SetTrustAdjustment(peerID, 1.0)
	assert.Equal(t, 0.5, record.trustAdjustment)
}

func TestGetCompositeScore(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peerID := peer.ID("peer-1")

	// Non-existent peer should return 0
	score := tracker.GetCompositeScore(peerID)
	assert.Equal(t, 0.0, score)

	// Create record with perfect metrics
	tracker.GetOrCreateRecord(peerID)
	tracker.RecordRelayEvent(peerID, true)
	tracker.RecordRelayEvent(peerID, true)
	for i := 0; i < 168; i++ {
		tracker.RecordUptimeObservation(peerID, true)
	}
	tracker.RecordMailboxEvent(peerID, true, false)
	tracker.RecordMailboxEvent(peerID, false, true)
	tracker.RecordDHTQuery(peerID, 0)
	tracker.RecordContentEvent(peerID, true, true)

	score = tracker.GetCompositeScore(peerID)
	assert.Greater(t, score, 0.9)
	assert.LessOrEqual(t, score, 1.0)
}

func TestGetTier(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peerID := peer.ID("peer-1")

	// Non-existent peer should return Basic
	tier := tracker.GetTier(peerID)
	assert.Equal(t, TierBasic, tier)

	// Create record and manipulate score to test tier thresholds
	record := tracker.GetOrCreateRecord(peerID)
	
	// Set metrics to achieve desired scores
	// For TierBasic (0.2): all metrics at 0.2
	record.mu.Lock()
	record.metrics.RelayReliability = 0.2
	record.metrics.UptimeConsistency = 0.2
	record.metrics.MailboxReliability = 0.2
	record.metrics.DHTResponsiveness = 0.2
	record.metrics.ContentServing = 0.2
	record.updateCompositeScore(tracker.config)
	record.mu.Unlock()
	assert.Equal(t, TierBasic, tracker.GetTier(peerID))

	// For TierContributor (0.5): all metrics at 0.5
	record.mu.Lock()
	record.metrics.RelayReliability = 0.5
	record.metrics.UptimeConsistency = 0.5
	record.metrics.MailboxReliability = 0.5
	record.metrics.DHTResponsiveness = 0.5
	record.metrics.ContentServing = 0.5
	record.updateCompositeScore(tracker.config)
	record.mu.Unlock()
	assert.Equal(t, TierContributor, tracker.GetTier(peerID))

	// For TierReliable (0.7): all metrics at 0.7
	record.mu.Lock()
	record.metrics.RelayReliability = 0.7
	record.metrics.UptimeConsistency = 0.7
	record.metrics.MailboxReliability = 0.7
	record.metrics.DHTResponsiveness = 0.7
	record.metrics.ContentServing = 0.7
	record.updateCompositeScore(tracker.config)
	record.mu.Unlock()
	assert.Equal(t, TierReliable, tracker.GetTier(peerID))

	// For TierTrusted (0.9): all metrics at 0.9
	record.mu.Lock()
	record.metrics.RelayReliability = 0.9
	record.metrics.UptimeConsistency = 0.9
	record.metrics.MailboxReliability = 0.9
	record.metrics.DHTResponsiveness = 0.9
	record.metrics.ContentServing = 0.9
	record.updateCompositeScore(tracker.config)
	record.mu.Unlock()
	assert.Equal(t, TierTrusted, tracker.GetTier(peerID))
}

func TestGetAllRecords(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peer1 := peer.ID("peer-1")
	peer2 := peer.ID("peer-2")

	tracker.GetOrCreateRecord(peer1)
	tracker.GetOrCreateRecord(peer2)

	records := tracker.GetAllRecords()
	assert.Len(t, records, 2)
	assert.Contains(t, records, peer1)
	assert.Contains(t, records, peer2)
}

func TestGetPeersByTier(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peer1 := peer.ID("peer-1")
	peer2 := peer.ID("peer-2")
	peer3 := peer.ID("peer-3")

	tracker.GetOrCreateRecord(peer1)
	tracker.GetOrCreateRecord(peer2)
	tracker.GetOrCreateRecord(peer3)

	// Set different tiers via metrics
	record1 := tracker.GetRecord(peer1)
	record1.mu.Lock()
	record1.metrics.RelayReliability = 0.9
	record1.metrics.UptimeConsistency = 0.9
	record1.metrics.MailboxReliability = 0.9
	record1.metrics.DHTResponsiveness = 0.9
	record1.metrics.ContentServing = 0.9
	record1.updateCompositeScore(tracker.config)
	record1.mu.Unlock()

	record2 := tracker.GetRecord(peer2)
	record2.mu.Lock()
	record2.metrics.RelayReliability = 0.5
	record2.metrics.UptimeConsistency = 0.5
	record2.metrics.MailboxReliability = 0.5
	record2.metrics.DHTResponsiveness = 0.5
	record2.metrics.ContentServing = 0.5
	record2.updateCompositeScore(tracker.config)
	record2.mu.Unlock()

	record3 := tracker.GetRecord(peer3)
	record3.mu.Lock()
	record3.metrics.RelayReliability = 0.2
	record3.metrics.UptimeConsistency = 0.2
	record3.metrics.MailboxReliability = 0.2
	record3.metrics.DHTResponsiveness = 0.2
	record3.metrics.ContentServing = 0.2
	record3.updateCompositeScore(tracker.config)
	record3.mu.Unlock()

	// Get peers by tier
	trusted := tracker.GetPeersByTier(TierTrusted)
	assert.Len(t, trusted, 1)
	assert.Contains(t, trusted, peer1)

	contributors := tracker.GetPeersByTier(TierContributor)
	assert.Len(t, contributors, 1)
	assert.Contains(t, contributors, peer2)

	basic := tracker.GetPeersByTier(TierBasic)
	assert.Len(t, basic, 1)
	assert.Contains(t, basic, peer3)
}

func TestGetTopPeers(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peer1 := peer.ID("peer-1")
	peer2 := peer.ID("peer-2")
	peer3 := peer.ID("peer-3")

	tracker.GetOrCreateRecord(peer1)
	tracker.GetOrCreateRecord(peer2)
	tracker.GetOrCreateRecord(peer3)

	// Set different scores via metrics
	record1 := tracker.GetRecord(peer1)
	record1.mu.Lock()
	record1.metrics.RelayReliability = 0.5
	record1.metrics.UptimeConsistency = 0.5
	record1.metrics.MailboxReliability = 0.5
	record1.metrics.DHTResponsiveness = 0.5
	record1.metrics.ContentServing = 0.5
	record1.updateCompositeScore(tracker.config)
	record1.mu.Unlock()

	record2 := tracker.GetRecord(peer2)
	record2.mu.Lock()
	record2.metrics.RelayReliability = 0.9
	record2.metrics.UptimeConsistency = 0.9
	record2.metrics.MailboxReliability = 0.9
	record2.metrics.DHTResponsiveness = 0.9
	record2.metrics.ContentServing = 0.9
	record2.updateCompositeScore(tracker.config)
	record2.mu.Unlock()

	record3 := tracker.GetRecord(peer3)
	record3.mu.Lock()
	record3.metrics.RelayReliability = 0.7
	record3.metrics.UptimeConsistency = 0.7
	record3.metrics.MailboxReliability = 0.7
	record3.metrics.DHTResponsiveness = 0.7
	record3.metrics.ContentServing = 0.7
	record3.updateCompositeScore(tracker.config)
	record3.mu.Unlock()

	// Get top 2 peers
	top := tracker.GetTopPeers(2)
	require.Len(t, top, 2)
	assert.Equal(t, peer2, top[0]) // Highest score
	assert.Equal(t, peer3, top[1]) // Second highest
}

func TestTierString(t *testing.T) {
	assert.Equal(t, "Basic", TierBasic.String())
	assert.Equal(t, "Contributor", TierContributor.String())
	assert.Equal(t, "Reliable", TierReliable.String())
	assert.Equal(t, "Trusted", TierTrusted.String())
}

func TestCompositeScoreComputation(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peerID := peer.ID("peer-1")
	record := tracker.GetOrCreateRecord(peerID)

	// Set all metrics to 0.5
	record.metrics.RelayReliability = 0.5
	record.metrics.UptimeConsistency = 0.5
	record.metrics.MailboxReliability = 0.5
	record.metrics.DHTResponsiveness = 0.5
	record.metrics.ContentServing = 0.5

	record.updateCompositeScore(tracker.config)

	// Expected: 0.5 * (0.25 + 0.20 + 0.25 + 0.15 + 0.15) = 0.5 * 1.0 = 0.5
	assert.InDelta(t, 0.5, record.compositeScore, 0.001)

	// Test with different weights
	customConfig := &Config{
		RelayReliabilityWeight:     0.5,
		UptimeConsistencyWeight:    0.0,
		MailboxReliabilityWeight:   0.5,
		DHTResponsivenessWeight:    0.0,
		ContentServingWeight:       0.0,
		TierContributorThreshold:   DefaultTierContributorThreshold,
		TierReliableThreshold:      DefaultTierReliableThreshold,
		TierTrustedThreshold:       DefaultTierTrustedThreshold,
	}

	record.updateCompositeScore(customConfig)
	// Expected: 0.5 * 0.5 + 0.5 * 0.5 = 0.5
	assert.InDelta(t, 0.5, record.compositeScore, 0.001)
}

func TestAddAttestation(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	subjectPeerID := peer.ID("subject-peer")
	attesterPeerID := peer.ID("attester-peer")

	// Create attester record with sufficient observation time
	attesterRecord := tracker.GetOrCreateRecord(attesterPeerID)
	attesterRecord.mu.Lock()
	attesterRecord.metrics.FirstObserved = time.Now().Add(-25 * time.Hour)
	attesterRecord.mu.Unlock()

	attestation := &Attestation{
		AttesterPeerID:         attesterPeerID,
		AttesterIdentityPub:    "attester-identity",
		SubjectPeerID:          subjectPeerID,
		Score:                  0.8,
		ObservationPeriodHours: 100,
		Timestamp:              time.Now(),
		Signature:              []byte("signature"),
	}

	err := tracker.AddAttestation(attestation)
	require.NoError(t, err)

	record := tracker.GetRecord(subjectPeerID)
	require.NotNil(t, record)
	assert.Len(t, record.attestations, 1)
	assert.Equal(t, 0.8, record.attestations[0].Score)
}

func TestAddAttestationExpired(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	subjectPeerID := peer.ID("subject-peer")
	attesterPeerID := peer.ID("attester-peer")

	// Create expired attestation
	attestation := &Attestation{
		AttesterPeerID:         attesterPeerID,
		AttesterIdentityPub:    "attester-identity",
		SubjectPeerID:          subjectPeerID,
		Score:                  0.8,
		ObservationPeriodHours: 100,
		Timestamp:              time.Now().Add(-200 * time.Hour), // Expired
		Signature:              []byte("signature"),
	}

	err := tracker.AddAttestation(attestation)
	assert.Error(t, err)
	assert.Equal(t, ErrAttestationExpired, err)
}

func TestAddAttestationNotTrusted(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	subjectPeerID := peer.ID("subject-peer")
	attesterPeerID := peer.ID("attester-peer")

	// Create attester record with insufficient observation time
	attesterRecord := tracker.GetOrCreateRecord(attesterPeerID)
	attesterRecord.mu.Lock()
	attesterRecord.metrics.FirstObserved = time.Now().Add(-1 * time.Hour)
	attesterRecord.mu.Unlock()

	attestation := &Attestation{
		AttesterPeerID:         attesterPeerID,
		AttesterIdentityPub:    "attester-identity",
		SubjectPeerID:          subjectPeerID,
		Score:                  0.8,
		ObservationPeriodHours: 100,
		Timestamp:              time.Now(),
		Signature:              []byte("signature"),
	}

	err := tracker.AddAttestation(attestation)
	assert.Error(t, err)
	assert.Equal(t, ErrAttesterNotTrusted, err)
}

func TestToProto(t *testing.T) {
	selfPeerID := peer.ID("test-peer")
	tracker := NewTracker(selfPeerID, "identity-pub", nil)

	peerID := peer.ID("peer-1")
	record := tracker.GetOrCreateRecord(peerID)

	record.mu.Lock()
	record.metrics.RelayReliability = 0.8
	record.metrics.RelaySuccessCount = 80
	record.metrics.RelayTotalCount = 100
	record.compositeScore = 0.75
	record.tier = TierReliable
	record.trustAdjustment = 0.1
	record.mu.Unlock()

	protoRecord := record.ToProto()

	require.NotNil(t, protoRecord)
	assert.Equal(t, []byte(peerID), protoRecord.PeerId)
	assert.InDelta(t, 0.8, protoRecord.Metrics.RelayReliability, 0.01)
	assert.Equal(t, uint64(80), protoRecord.Metrics.RelaySuccessCount)
	assert.InDelta(t, 0.75, protoRecord.CompositeScore, 0.01)
	assert.Equal(t, pb.ReputationTier_REPUTATION_TIER_RELIABLE, protoRecord.Tier)
	assert.InDelta(t, 0.1, protoRecord.TrustAdjustment, 0.01)
}
