package reputation

import (
	"encoding/json"
	"fmt"
	"time"

	pb "babylontower/pkg/proto"
	"github.com/dgraph-io/badger/v3"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

const (
	// Key prefix for reputation records in BadgerDB
	reputationPrefix = "rep:"
)

// Storage handles persistence of reputation records in BadgerDB
type Storage struct {
	db *badger.DB
}

// NewStorage creates a new reputation storage
func NewStorage(db *badger.DB) *Storage {
	return &Storage{db: db}
}

// SaveRecord saves a reputation record to BadgerDB
func (s *Storage) SaveRecord(record *Record) error {
	protoRecord := record.ToProto()
	data, err := proto.Marshal(protoRecord)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}

	key := fmt.Sprintf("%s%s", reputationPrefix, string(record.peerID))

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
}

// LoadRecord loads a reputation record from BadgerDB
func (s *Storage) LoadRecord(pid peer.ID) (*Record, error) {
	key := fmt.Sprintf("%s%s", reputationPrefix, string(pid))

	var data []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		data, err = item.ValueCopy(nil)
		return err
	})

	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, nil
		}
		return nil, err
	}

	protoRecord := &pb.PeerReputationRecord{}
	if err := proto.Unmarshal(data, protoRecord); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	// Convert protobuf record to local Record
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

// DeleteRecord deletes a reputation record from BadgerDB
func (s *Storage) DeleteRecord(pid peer.ID) error {
	key := fmt.Sprintf("%s%s", reputationPrefix, string(pid))

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}

// ListRecords lists all reputation records in BadgerDB
func (s *Storage) ListRecords() ([]*Record, error) {
	var records []*Record

	prefix := []byte(reputationPrefix)
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			protoRecord := &pb.PeerReputationRecord{}
			if err := proto.Unmarshal(data, protoRecord); err != nil {
				return err
			}

			// Extract peer ID from key
			key := string(item.Key())
			pidStr := key[len(reputationPrefix):]
			pid := peer.ID(pidStr)

			// Convert to local Record
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

			record := &Record{
				peerID:          pid,
				metrics:         metrics,
				compositeScore:  float64(protoRecord.CompositeScore),
				tier:            Tier(protoRecord.Tier),
				attestations:    attestations,
				trustAdjustment: float64(protoRecord.TrustAdjustment),
				lastUpdated:     time.Now(),
			}

			records = append(records, record)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return records, nil
}

// SaveConfig saves the reputation configuration to BadgerDB
func (s *Storage) SaveConfig(config *Config) error {
	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("cfg:reputation"), data)
	})
}

// LoadConfig loads the reputation configuration from BadgerDB
func (s *Storage) LoadConfig() (*Config, error) {
	var data []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("cfg:reputation"))
		if err != nil {
			return err
		}
		data, err = item.ValueCopy(nil)
		return err
	})

	if err != nil {
		if err == badger.ErrKeyNotFound {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	config := &Config{}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return config, nil
}

// GetStats returns statistics about stored reputation records
type Stats struct {
	TotalRecords     int
	TierBasicCount   int
	TierContributorCount int
	TierReliableCount  int
	TierTrustedCount   int
	AverageScore     float64
}

// GetStats computes statistics about reputation records
func (s *Storage) GetStats() (*Stats, error) {
	records, err := s.ListRecords()
	if err != nil {
		return nil, err
	}

	stats := &Stats{
		TotalRecords: len(records),
	}

	totalScore := 0.0
	for _, record := range records {
		totalScore += record.compositeScore

		switch record.tier {
		case TierBasic:
			stats.TierBasicCount++
		case TierContributor:
			stats.TierContributorCount++
		case TierReliable:
			stats.TierReliableCount++
		case TierTrusted:
			stats.TierTrustedCount++
		}
	}

	if stats.TotalRecords > 0 {
		stats.AverageScore = totalScore / float64(stats.TotalRecords)
	}

	return stats, nil
}
