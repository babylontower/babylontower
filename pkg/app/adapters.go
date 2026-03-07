package app

import (
	"babylontower/pkg/mailbox"
	"babylontower/pkg/reputation"
	"context"

	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

// mailboxManagerAdapter adapts *mailbox.Manager to MailboxManager interface
type mailboxManagerAdapter struct {
	manager *mailbox.Manager
}

func (a *mailboxManagerAdapter) IsMailbox() bool {
	return a.manager.IsMailbox()
}

func (a *mailboxManagerAdapter) GetStats() (*MailboxStats, error) {
	stats, err := a.manager.GetStats()
	if err != nil {
		return nil, err
	}
	return &MailboxStats{
		StoredCount:     int(stats.StoredCount),
		UsedBytes:       int64(stats.UsedBytes),
		CapacityBytes:   int64(stats.CapacityBytes),
		OldestTimestamp: int64(stats.OldestTimestamp),
		NewestTimestamp: int64(stats.NewestTimestamp),
	}, nil
}

func (a *mailboxManagerAdapter) GetAnnouncement(pubKey []byte) (*MailboxAnnouncement, bool) {
	ann, ok := a.manager.GetAnnouncement(pubKey)
	if !ok {
		return nil, false
	}
	return &MailboxAnnouncement{
		MailboxPeerId:   string(ann.MailboxPeerId),
		MaxMessages:     int(ann.MaxMessages),
		MaxMessageSize:  int(ann.MaxMessageSize),
		TtlSeconds:      int(ann.TtlSeconds),
		ReputationScore: int64(ann.ReputationScore),
	}, true
}

func (a *mailboxManagerAdapter) RetrieveMessages(ctx context.Context) (*MailboxRetrievalResult, error) {
	result, err := a.manager.RetrieveMessages(ctx)
	if err != nil {
		return nil, err
	}

	// Convert envelopes to bytes
	envelopes := make([][]byte, len(result.Envelopes))
	for i, env := range result.Envelopes {
		data, _ := proto.Marshal(env)
		envelopes[i] = data
	}

	return &MailboxRetrievalResult{
		Envelopes:  envelopes,
		MessageIDs: result.MessageIDs,
		Count:      len(result.Envelopes),
	}, nil
}

// reputationTrackerAdapter adapts *reputation.Tracker to ReputationTracker interface
type reputationTrackerAdapter struct {
	tracker *reputation.Tracker
}

func (a *reputationTrackerAdapter) GetAllRecords() map[string]*ReputationRecord {
	concreteRecords := a.tracker.GetAllRecords()
	result := make(map[string]*ReputationRecord, len(concreteRecords))
	for pid, record := range concreteRecords {
		result[string(pid)] = convertReputationRecord(record)
	}
	return result
}

func (a *reputationTrackerAdapter) GetPeersByTier(tier string) []string {
	var targetTier reputation.Tier
	switch tier {
	case "Basic":
		targetTier = reputation.TierBasic
	case "Contributor":
		targetTier = reputation.TierContributor
	case "Reliable":
		targetTier = reputation.TierReliable
	case "Trusted":
		targetTier = reputation.TierTrusted
	default:
		return nil
	}

	concretePeers := a.tracker.GetPeersByTier(targetTier)
	result := make([]string, len(concretePeers))
	for i, pid := range concretePeers {
		result[i] = string(pid)
	}
	return result
}

func (a *reputationTrackerAdapter) GetTopPeers(n int) []string {
	concretePeers := a.tracker.GetTopPeers(n)
	result := make([]string, len(concretePeers))
	for i, pid := range concretePeers {
		result[i] = string(pid)
	}
	return result
}

func (a *reputationTrackerAdapter) GetRecord(peerID string) *ReputationRecord {
	record := a.tracker.GetRecord(peer.ID(peerID))
	if record == nil {
		return nil
	}
	return convertReputationRecord(record)
}

// convertReputationRecord converts a reputation.Record to app.ReputationRecord
func convertReputationRecord(record *reputation.Record) *ReputationRecord {
	if record == nil {
		return nil
	}

	metrics := record.GetMetrics()
	attestations := record.GetAttestations()

	return &ReputationRecord{
		CompositeScore: record.GetCompositeScore(),
		Tier:           record.GetTier().String(),
		Metrics: &ReputationMetrics{
			RelayReliability:      metrics.RelayReliability,
			RelaySuccessCount:     int(metrics.RelaySuccessCount),
			RelayTotalCount:       int(metrics.RelayTotalCount),
			UptimeConsistency:     metrics.UptimeConsistency,
			HoursOnline7d:         int(metrics.HoursOnline7d),
			MailboxReliability:    metrics.MailboxReliability,
			MailboxRetrievedCount: int(metrics.MailboxRetrievedCount),
			MailboxDepositedCount: int(metrics.MailboxDepositedCount),
			DHTResponsiveness:     metrics.DHTResponsiveness,
			AvgResponseMS:         metrics.AvgResponseMS,
			ContentServing:        metrics.ContentServing,
			BlocksServedCount:     int(metrics.BlocksServedCount),
			BlocksRequestedCount:  int(metrics.BlocksRequestedCount),
		},
		Attestations: convertAttestations(attestations),
	}
}

// convertAttestations converts reputation.Attestation slice to app.Attestation slice
func convertAttestations(attestations []*reputation.Attestation) []Attestation {
	if len(attestations) == 0 {
		return nil
	}

	result := make([]Attestation, len(attestations))
	for i, att := range attestations {
		result[i] = Attestation{
			FromPeerID:      string(att.AttesterPeerID),
			ToPeerID:        string(att.SubjectPeerID),
			Score:           int64(att.Score),
			Timestamp:       att.Timestamp.Unix(),
			AttestationType: "reputation",
		}
	}
	return result
}
