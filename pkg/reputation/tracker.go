package reputation

import (
	"encoding/hex"
	"sync"
	"time"

	"babylontower/pkg/proto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Default configuration values
const (
	DefaultRelayReliabilityWeight     = 0.25
	DefaultUptimeConsistencyWeight    = 0.20
	DefaultMailboxReliabilityWeight   = 0.25
	DefaultDHTResponsivenessWeight    = 0.15
	DefaultContentServingWeight       = 0.15

	DefaultMinConnectionHours        = 24
	DefaultMaxAttestationInfluence   = 0.1
	DefaultAttestationExpiryHours    = 168 // 7 days
	DefaultMaxAttestationsPerPeer    = 50

	DefaultTierContributorThreshold = 0.3
	DefaultTierReliableThreshold    = 0.6
	DefaultTierTrustedThreshold     = 0.8

	DefaultMetricUpdateIntervalSeconds  = 300 // 5 minutes
	DefaultAttestationPublishInterval   = 24  // hours

	HoursPerWeek = 168
	MaxDHTLatencyMS = 5000
)

// Tracker manages peer reputation tracking and scoring
type Tracker struct {
	mu           sync.RWMutex
	records      map[peer.ID]*Record
	config       *Config
	selfPeerID   peer.ID
	selfIdentity string // hex-encoded IK_sign.pub
}

// Record holds a peer's reputation data with thread-safe access
type Record struct {
	mu              sync.RWMutex
	peerID          peer.ID
	metrics         *Metrics
	compositeScore  float64
	tier            Tier
	attestations    []*Attestation
	trustAdjustment float64
	lastUpdated     time.Time
}

// Metrics holds the 5 dimensions of reputation
type Metrics struct {
	// Relay reliability
	RelayReliability   float64
	RelaySuccessCount  uint64
	RelayTotalCount    uint64

	// Uptime consistency
	UptimeConsistency float64
	HoursOnline7d     uint64
	LastSeen          time.Time

	// Mailbox reliability
	MailboxReliability   float64
	MailboxRetrievedCount uint64
	MailboxDepositedCount uint64

	// DHT responsiveness
	DHTResponsiveness float64
	AvgResponseMS     float64
	DHTQueryCount     uint64

	// Content serving
	ContentServing      float64
	BlocksServedCount   uint64
	BlocksRequestedCount uint64

	// Metadata
	FirstObserved   time.Time
	ObservationCount uint64
}

// Attestation holds a signed attestation from another peer
type Attestation struct {
	AttesterPeerID        peer.ID
	AttesterIdentityPub   string // hex-encoded
	SubjectPeerID         peer.ID
	Score                 float64
	ObservationPeriodHours uint64
	Timestamp             time.Time
	Signature             []byte
}

// Tier represents reputation tier classification
type Tier int

const (
	TierBasic Tier = iota // 0.0 - 0.3
	TierContributor       // 0.3 - 0.6
	TierReliable          // 0.6 - 0.8
	TierTrusted           // 0.8 - 1.0
)

// Config holds reputation system configuration
type Config struct {
	RelayReliabilityWeight   float64
	UptimeConsistencyWeight  float64
	MailboxReliabilityWeight float64
	DHTResponsivenessWeight  float64
	ContentServingWeight     float64

	MinConnectionHours       uint64
	MaxAttestationInfluence  float64
	AttestationExpiryHours   uint64
	MaxAttestationsPerPeer   uint32

	TierContributorThreshold float64
	TierReliableThreshold    float64
	TierTrustedThreshold     float64

	MetricUpdateIntervalSeconds uint64
	AttestationPublishInterval  uint64
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		RelayReliabilityWeight:     DefaultRelayReliabilityWeight,
		UptimeConsistencyWeight:    DefaultUptimeConsistencyWeight,
		MailboxReliabilityWeight:   DefaultMailboxReliabilityWeight,
		DHTResponsivenessWeight:    DefaultDHTResponsivenessWeight,
		ContentServingWeight:       DefaultContentServingWeight,

		MinConnectionHours:       DefaultMinConnectionHours,
		MaxAttestationInfluence:  DefaultMaxAttestationInfluence,
		AttestationExpiryHours:   DefaultAttestationExpiryHours,
		MaxAttestationsPerPeer:   DefaultMaxAttestationsPerPeer,

		TierContributorThreshold: DefaultTierContributorThreshold,
		TierReliableThreshold:    DefaultTierReliableThreshold,
		TierTrustedThreshold:     DefaultTierTrustedThreshold,

		MetricUpdateIntervalSeconds: DefaultMetricUpdateIntervalSeconds,
		AttestationPublishInterval:  DefaultAttestationPublishInterval,
	}
}

// NewTracker creates a new reputation tracker
func NewTracker(selfPeerID peer.ID, selfIdentity string, config *Config) *Tracker {
	if config == nil {
		config = DefaultConfig()
	}

	return &Tracker{
		records:      make(map[peer.ID]*Record),
		config:       config,
		selfPeerID:   selfPeerID,
		selfIdentity: selfIdentity,
	}
}

// GetOrCreateRecord gets or creates a reputation record for a peer
func (t *Tracker) GetOrCreateRecord(pid peer.ID) *Record {
	t.mu.RLock()
	record, exists := t.records[pid]
	t.mu.RUnlock()

	if exists {
		return record
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Double-check after acquiring write lock
	if record, exists = t.records[pid]; exists {
		return record
	}

	record = &Record{
		peerID: pid,
		metrics: &Metrics{
			FirstObserved: time.Now(),
		},
		compositeScore:  0.0,
		tier:            TierBasic,
		attestations:    make([]*Attestation, 0),
		trustAdjustment: 0.0,
		lastUpdated:     time.Now(),
	}

	t.records[pid] = record
	return record
}

// GetRecord gets a reputation record for a peer (returns nil if not found)
func (t *Tracker) GetRecord(pid peer.ID) *Record {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.records[pid]
}

// RecordRelayEvent records a relay operation (success or failure)
func (t *Tracker) RecordRelayEvent(pid peer.ID, success bool) {
	record := t.GetOrCreateRecord(pid)
	record.mu.Lock()
	defer record.mu.Unlock()

	record.metrics.RelayTotalCount++
	if success {
		record.metrics.RelaySuccessCount++
	}

	// Update relay reliability ratio
	if record.metrics.RelayTotalCount > 0 {
		record.metrics.RelayReliability = float64(record.metrics.RelaySuccessCount) / float64(record.metrics.RelayTotalCount)
	}

	record.updateCompositeScore(t.config)
	record.lastUpdated = time.Now()
}

// RecordUptimeObservation records whether a peer was observed online
func (t *Tracker) RecordUptimeObservation(pid peer.ID, online bool) {
	record := t.GetOrCreateRecord(pid)
	record.mu.Lock()
	defer record.mu.Unlock()

	if online {
		record.metrics.HoursOnline7d++
		record.metrics.LastSeen = time.Now()
	}

	// Cap at 168 hours (7 days)
	if record.metrics.HoursOnline7d > HoursPerWeek {
		record.metrics.HoursOnline7d = HoursPerWeek
	}

	// Update uptime consistency
	record.metrics.UptimeConsistency = float64(record.metrics.HoursOnline7d) / HoursPerWeek

	record.updateCompositeScore(t.config)
	record.lastUpdated = time.Now()
}

// RecordMailboxEvent records a mailbox operation
func (t *Tracker) RecordMailboxEvent(pid peer.ID, deposited bool, retrieved bool) {
	record := t.GetOrCreateRecord(pid)
	record.mu.Lock()
	defer record.mu.Unlock()

	if deposited {
		record.metrics.MailboxDepositedCount++
	}
	if retrieved {
		record.metrics.MailboxRetrievedCount++
	}

	// Update mailbox reliability ratio
	if record.metrics.MailboxDepositedCount > 0 {
		record.metrics.MailboxReliability = float64(record.metrics.MailboxRetrievedCount) / float64(record.metrics.MailboxDepositedCount)
		// Cap at 1.0
		if record.metrics.MailboxReliability > 1.0 {
			record.metrics.MailboxReliability = 1.0
		}
	}

	record.updateCompositeScore(t.config)
	record.lastUpdated = time.Now()
}

// RecordDHTQuery records a DHT query response time
func (t *Tracker) RecordDHTQuery(pid peer.ID, responseMS float64) {
	record := t.GetOrCreateRecord(pid)
	record.mu.Lock()
	defer record.mu.Unlock()

	// Running average
	totalQueries := float64(record.metrics.DHTQueryCount)
	newAvg := ((totalQueries * record.metrics.AvgResponseMS) + responseMS) / (totalQueries + 1)
	record.metrics.AvgResponseMS = newAvg
	record.metrics.DHTQueryCount++

	// Update DHT responsiveness: 1 - (avg_response_ms / 5000), clamped to [0, 1]
	record.metrics.DHTResponsiveness = 1.0 - (record.metrics.AvgResponseMS / MaxDHTLatencyMS)
	if record.metrics.DHTResponsiveness < 0 {
		record.metrics.DHTResponsiveness = 0
	}
	if record.metrics.DHTResponsiveness > 1.0 {
		record.metrics.DHTResponsiveness = 1.0
	}

	record.updateCompositeScore(t.config)
	record.lastUpdated = time.Now()
}

// RecordContentEvent records a content serving event
func (t *Tracker) RecordContentEvent(pid peer.ID, served bool, requested bool) {
	record := t.GetOrCreateRecord(pid)
	record.mu.Lock()
	defer record.mu.Unlock()

	if served {
		record.metrics.BlocksServedCount++
	}
	if requested {
		record.metrics.BlocksRequestedCount++
	}

	// Update content serving ratio
	if record.metrics.BlocksRequestedCount > 0 {
		record.metrics.ContentServing = float64(record.metrics.BlocksServedCount) / float64(record.metrics.BlocksRequestedCount)
		// Cap at 1.0
		if record.metrics.ContentServing > 1.0 {
			record.metrics.ContentServing = 1.0
		}
	}

	record.updateCompositeScore(t.config)
	record.lastUpdated = time.Now()
}

// SetTrustAdjustment sets a manual trust adjustment for a peer
func (t *Tracker) SetTrustAdjustment(pid peer.ID, adjustment float64) {
	// Clamp adjustment to [-0.5, 0.5]
	if adjustment < -0.5 {
		adjustment = -0.5
	}
	if adjustment > 0.5 {
		adjustment = 0.5
	}

	record := t.GetOrCreateRecord(pid)
	record.mu.Lock()
	defer record.mu.Unlock()

	record.trustAdjustment = adjustment
	record.updateCompositeScore(t.config)
	record.lastUpdated = time.Now()
}

// GetCompositeScore returns the composite score for a peer
func (t *Tracker) GetCompositeScore(pid peer.ID) float64 {
	record := t.GetRecord(pid)
	if record == nil {
		return 0.0
	}

	record.mu.RLock()
	defer record.mu.RUnlock()

	return record.compositeScore
}

// GetTier returns the reputation tier for a peer
func (t *Tracker) GetTier(pid peer.ID) Tier {
	record := t.GetRecord(pid)
	if record == nil {
		return TierBasic
	}

	record.mu.RLock()
	defer record.mu.RUnlock()

	return record.tier
}

// GetMetrics returns a copy of the metrics for a peer
func (r *Record) GetMetrics() *Metrics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to avoid race conditions
	metricsCopy := *r.metrics
	return &metricsCopy
}

// GetCompositeScore returns the composite score (thread-safe)
func (r *Record) GetCompositeScore() float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.compositeScore
}

// GetTier returns the tier (thread-safe)
func (r *Record) GetTier() Tier {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tier
}

// GetAttestations returns a copy of the attestations (thread-safe)
func (r *Record) GetAttestations() []*Attestation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	copy := make([]*Attestation, len(r.attestations))
	for i, att := range r.attestations {
		attCopy := *att
		copy[i] = &attCopy
	}
	return copy
}

// AddAttestation adds an attestation for a peer
func (t *Tracker) AddAttestation(attestation *Attestation) error {
	// Verify attestation is not expired
	expiryTime := time.Unix(int64(attestation.Timestamp.Unix()), 0).Add(
		time.Duration(t.config.AttestationExpiryHours) * time.Hour,
	)
	if time.Now().After(expiryTime) {
		return ErrAttestationExpired
	}

	// Verify minimum connection time (if we have a record of when we first saw the attester)
	attesterRecord := t.GetRecord(attestation.AttesterPeerID)
	if attesterRecord != nil {
		attesterRecord.mu.RLock()
		hoursSinceFirstObserved := uint64(time.Since(attesterRecord.metrics.FirstObserved).Hours())
		attesterRecord.mu.RUnlock()

		if hoursSinceFirstObserved < t.config.MinConnectionHours {
			return ErrAttesterNotTrusted
		}
	}

	record := t.GetOrCreateRecord(attestation.SubjectPeerID)
	record.mu.Lock()
	defer record.mu.Unlock()

	// Limit number of attestations
	if uint32(len(record.attestations)) >= t.config.MaxAttestationsPerPeer {
		// Remove oldest attestation
		record.attestations = record.attestations[1:]
	}

	record.attestations = append(record.attestations, attestation)
	record.updateCompositeScore(t.config)
	record.lastUpdated = time.Now()

	return nil
}

// GetAllRecords returns all reputation records
func (t *Tracker) GetAllRecords() map[peer.ID]*Record {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[peer.ID]*Record, len(t.records))
	for k, v := range t.records {
		result[k] = v
	}
	return result
}

// GetPeersByTier returns all peers in a specific tier
func (t *Tracker) GetPeersByTier(tier Tier) []peer.ID {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []peer.ID
	for pid, record := range t.records {
		record.mu.RLock()
		if record.tier == tier {
			result = append(result, pid)
		}
		record.mu.RUnlock()
	}
	return result
}

// GetTopPeers returns the top N peers by composite score
func (t *Tracker) GetTopPeers(n int) []peer.ID {
	t.mu.RLock()
	defer t.mu.RUnlock()

	type scoredPeer struct {
		pid   peer.ID
		score float64
	}

	peers := make([]scoredPeer, 0, len(t.records))
	for pid, record := range t.records {
		record.mu.RLock()
		peers = append(peers, scoredPeer{pid: pid, score: record.compositeScore})
		record.mu.RUnlock()
	}

	// Sort by score descending
	for i := 0; i < len(peers); i++ {
		for j := i + 1; j < len(peers); j++ {
			if peers[j].score > peers[i].score {
				peers[i], peers[j] = peers[j], peers[i]
			}
		}
	}

	// Return top N
	result := make([]peer.ID, 0, n)
	for i := 0; i < len(peers) && i < n; i++ {
		result = append(result, peers[i].pid)
	}
	return result
}

// ToProto converts a Record to protobuf format
func (r *Record) ToProto() *proto.PeerReputationRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	protoAttestations := make([]*proto.ReputationAttestation, len(r.attestations))
	for i, att := range r.attestations {
		protoAttestations[i] = &proto.ReputationAttestation{
			AttesterPeerId:         []byte(att.AttesterPeerID),
			SubjectPeerId:          []byte(att.SubjectPeerID),
			Score:                  float32(att.Score),
			ObservationPeriodHours: att.ObservationPeriodHours,
			Timestamp:              uint64(att.Timestamp.Unix()),
			Signature:              att.Signature,
			AttesterIdentityPub:    att.AttesterIdentityPub,
		}
	}

	return &proto.PeerReputationRecord{
		PeerId: []byte(r.peerID),
		Metrics: &proto.PeerReputationMetrics{
			RelayReliability:       float32(r.metrics.RelayReliability),
			RelaySuccessCount:      r.metrics.RelaySuccessCount,
			RelayTotalCount:        r.metrics.RelayTotalCount,
			UptimeConsistency:      float32(r.metrics.UptimeConsistency),
			HoursOnline_7D:         r.metrics.HoursOnline7d,
			LastSeen:               uint64(r.metrics.LastSeen.Unix()),
			MailboxReliability:     float32(r.metrics.MailboxReliability),
			MailboxRetrievedCount:  r.metrics.MailboxRetrievedCount,
			MailboxDepositedCount:  r.metrics.MailboxDepositedCount,
			DhtResponsiveness:      float32(r.metrics.DHTResponsiveness),
			AvgResponseMs:          float32(r.metrics.AvgResponseMS),
			DhtQueryCount:          r.metrics.DHTQueryCount,
			ContentServing:         float32(r.metrics.ContentServing),
			BlocksServedCount:      r.metrics.BlocksServedCount,
			BlocksRequestedCount:   r.metrics.BlocksRequestedCount,
			FirstObserved:          uint64(r.metrics.FirstObserved.Unix()),
			LastUpdated:            uint64(r.lastUpdated.Unix()),
			ObservationCount:       r.metrics.ObservationCount,
		},
		CompositeScore:  float32(r.compositeScore),
		Tier:            proto.ReputationTier(r.tier),
		Attestations:    protoAttestations,
		TrustAdjustment: float32(r.trustAdjustment),
	}
}

// updateCompositeScore recalculates the composite score and tier
func (r *Record) updateCompositeScore(config *Config) {
	// Weighted sum of metrics
	score := (r.metrics.RelayReliability * config.RelayReliabilityWeight) +
		(r.metrics.UptimeConsistency * config.UptimeConsistencyWeight) +
		(r.metrics.MailboxReliability * config.MailboxReliabilityWeight) +
		(r.metrics.DHTResponsiveness * config.DHTResponsivenessWeight) +
		(r.metrics.ContentServing * config.ContentServingWeight)

	// Apply trust adjustment
	score += r.trustAdjustment

	// Clamp to [0.0, 1.0]
	if score < 0.0 {
		score = 0.0
	}
	if score > 1.0 {
		score = 1.0
	}

	r.compositeScore = score

	// Determine tier
	if score >= config.TierTrustedThreshold {
		r.tier = TierTrusted
	} else if score >= config.TierReliableThreshold {
		r.tier = TierReliable
	} else if score >= config.TierContributorThreshold {
		r.tier = TierContributor
	} else {
		r.tier = TierBasic
	}
}

// String returns a string representation of the tier
func (t Tier) String() string {
	switch t {
	case TierBasic:
		return "Basic"
	case TierContributor:
		return "Contributor"
	case TierReliable:
		return "Reliable"
	case TierTrusted:
		return "Trusted"
	default:
		return "Unknown"
	}
}

// ToProto converts Tier to protobuf enum
func (t Tier) ToProto() proto.ReputationTier {
	return proto.ReputationTier(t)
}

// FromProto converts protobuf Tier to local type
func TierFromProto(t proto.ReputationTier) Tier {
	return Tier(t)
}

// Errors
type Error string

func (e Error) Error() string {
	return string(e)
}

const (
	ErrAttestationExpired  Error = "attestation has expired"
	ErrAttesterNotTrusted  Error = "attester has not been observed long enough"
	ErrInvalidSignature    Error = "invalid attestation signature"
)

// Helper function to convert peer.ID to hex string
func PeerIDToHex(pid peer.ID) string {
	return hex.EncodeToString([]byte(pid))
}

// Helper function to convert hex string to peer.ID
func HexToPeerID(s string) (peer.ID, error) {
	data, err := hex.DecodeString(s)
	if err != nil {
		return "", err
	}
	return peer.ID(data), nil
}
