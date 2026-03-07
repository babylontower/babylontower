package storage

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	bterrors "babylontower/pkg/errors"
	pb "babylontower/pkg/proto"

	"github.com/dgraph-io/badger/v3"
	"github.com/ipfs/go-log/v2"
	"google.golang.org/protobuf/proto"
)

var logger = log.Logger("babylontower/storage")

const (
	// Key prefixes
	contactPrefix   = "c:"
	messagePrefix   = "m:"
	peerPrefix      = "p:"
	configPrefix    = "cfg:"
	groupPrefix     = "g:"
	senderKeyPrefix = "sk:"
	blacklistPrefix = "bl:"

	// Key component sizes
	pubKeySize    = 32
	timestampSize = 8
	nonceSize     = 24
)

// BadgerStorage implements Storage using BadgerDB
type BadgerStorage struct {
	db *badger.DB
	mu sync.RWMutex
}

// Config holds BadgerDB configuration
type Config struct {
	// Path is the directory where BadgerDB stores its data
	Path string
	// InMemory enables in-memory mode (for testing)
	InMemory bool
}

// NewBadgerStorage creates a new BadgerDB-backed storage
func NewBadgerStorage(cfg Config) (*BadgerStorage, error) {
	opts := badger.DefaultOptions(cfg.Path)
	if cfg.InMemory {
		opts = opts.WithInMemory(true)
	}
	opts = opts.WithLoggingLevel(badger.ERROR)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
	}

	return &BadgerStorage{
		db: db,
	}, nil
}

// messageKey creates a composite key for message storage
// Format: messagePrefix + contact_pubkey + timestamp + nonce
// This ensures messages for the same contact are stored together and sorted by time
func messageKey(contactPubKey []byte, timestamp uint64, nonce []byte) []byte {
	key := make([]byte, 0, 2+pubKeySize+timestampSize+nonceSize)
	key = append(key, messagePrefix...)
	key = append(key, contactPubKey...)
	tsBytes := make([]byte, timestampSize)
	binary.BigEndian.PutUint64(tsBytes, timestamp)
	key = append(key, tsBytes...)
	key = append(key, nonce...)
	return key
}

// contactKey creates a key for contact storage
// Format: contactPrefix + public_key
func contactKey(pubKey []byte) []byte {
	key := make([]byte, 0, 2+len(pubKey))
	key = append(key, contactPrefix...)
	key = append(key, pubKey...)
	return key
}

// AddContact stores a contact in the database
func (s *BadgerStorage) AddContact(contact *pb.Contact) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := proto.Marshal(contact)
	if err != nil {
		return fmt.Errorf("failed to marshal contact: %w", err)
	}

	key := contactKey(contact.PublicKey)

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("failed to store contact: %w", err)
	}

	return nil
}

// GetContact retrieves a contact by public key
func (s *BadgerStorage) GetContact(pubKey []byte) (*pb.Contact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := contactKey(pubKey)

	var contact pb.Contact
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return proto.Unmarshal(data, &contact)
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve contact: %w", err)
	}

	return &contact, nil
}

// GetContactByBase58 retrieves a contact by base58-encoded public key
func (s *BadgerStorage) GetContactByBase58(pubKeyBase58 string) (*pb.Contact, error) {
	pubKey, err := ContactKeyFromBase58(pubKeyBase58)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base58 public key: %w", err)
	}
	return s.GetContact(pubKey)
}

// GetContactX25519Key retrieves the X25519 public key for a contact
func (s *BadgerStorage) GetContactX25519Key(pubKey []byte) ([]byte, error) {
	contact, err := s.GetContact(pubKey)
	if err != nil {
		return nil, err
	}
	if contact == nil {
		return nil, errors.New("contact not found")
	}
	if len(contact.X25519PublicKey) == 0 {
		return nil, errors.New("contact X25519 public key not stored")
	}
	return contact.X25519PublicKey, nil
}

// ListContacts returns all contacts in the database
func (s *BadgerStorage) ListContacts() ([]*pb.Contact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var contacts []*pb.Contact

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(contactPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			var contact pb.Contact
			if err := proto.Unmarshal(data, &contact); err != nil {
				return err
			}
			contacts = append(contacts, &contact)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list contacts: %w", err)
	}

	return contacts, nil
}

// DeleteContact removes a contact from the database
func (s *BadgerStorage) DeleteContact(pubKey []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := contactKey(pubKey)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
	if err != nil {
		return fmt.Errorf("failed to delete contact: %w", err)
	}

	return nil
}

// AddMessage stores an encrypted message for a contact
func (s *BadgerStorage) AddMessage(contactPubKey []byte, envelope *pb.SignedEnvelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Parse the envelope to get timestamp and nonce
	envData := envelope.Envelope
	var env pb.Envelope
	if err := proto.Unmarshal(envData, &env); err != nil {
		return fmt.Errorf("failed to parse envelope: %w", err)
	}

	// Extract timestamp from the encrypted message
	// We need to decrypt to get the timestamp for ordering
	// For now, we'll use current timestamp as fallback
	timestamp := uint64(time.Now().Unix())

	key := messageKey(contactPubKey, timestamp, env.Nonce)

	data, err := proto.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("failed to marshal envelope: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("failed to store message: %w", err)
	}

	return nil
}

// GetMessagesWithTimestamps retrieves messages for a contact with timestamps extracted from keys
// limit specifies maximum number of messages (0 = no limit)
// offset specifies number of messages to skip
func (s *BadgerStorage) GetMessagesWithTimestamps(contactPubKey []byte, limit, offset int) ([]*MessageWithKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var messages []*MessageWithKey

	// Build prefix for this contact's messages
	prefix := make([]byte, 0, 2+pubKeySize)
	prefix = append(prefix, messagePrefix...)
	prefix = append(prefix, contactPubKey...)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()

		count := 0
		skipped := 0

		for it.Rewind(); it.Valid(); it.Next() {
			// Handle offset
			if skipped < offset {
				skipped++
				continue
			}
			// Handle limit
			if limit > 0 && count >= limit {
				break
			}

			item := it.Item()
			key := item.Key()

			// Extract timestamp from key (format: prefix + pubkey + timestamp + nonce)
			// timestamp starts at position: len(prefix) + len(pubkey) = 2 + 32 = 34
			tsStart := len(messagePrefix) + len(contactPubKey)
			if len(key) < tsStart+timestampSize {
				continue
			}
			timestamp := binary.BigEndian.Uint64(key[tsStart : tsStart+timestampSize])

			// Extract nonce (after timestamp)
			nonceStart := tsStart + timestampSize
			if nonceStart >= len(key) {
				continue
			}
			nonce := make([]byte, len(key)-nonceStart)
			copy(nonce, key[nonceStart:])

			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			var envelope pb.SignedEnvelope
			if err := proto.Unmarshal(data, &envelope); err != nil {
				return err
			}
			messages = append(messages, &MessageWithKey{
				Envelope:  &envelope,
				Timestamp: timestamp,
				Nonce:     nonce,
			})
			count++
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve messages: %w", err)
	}

	return messages, nil
}

// GetMessages retrieves messages for a contact, ordered by timestamp
// limit specifies maximum number of messages (0 = no limit)
// offset specifies number of messages to skip
func (s *BadgerStorage) GetMessages(contactPubKey []byte, limit, offset int) ([]*pb.SignedEnvelope, error) {
	messages, err := s.GetMessagesWithTimestamps(contactPubKey, limit, offset)
	if err != nil {
		return nil, err
	}

	envelopes := make([]*pb.SignedEnvelope, 0, len(messages))
	for _, m := range messages {
		envelopes = append(envelopes, m.Envelope)
	}
	return envelopes, nil
}

// DeleteMessages removes all messages for a contact
func (s *BadgerStorage) DeleteMessages(contactPubKey []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Build prefix for this contact's messages
	prefix := make([]byte, 0, 2+pubKeySize)
	prefix = append(prefix, messagePrefix...)
	prefix = append(prefix, contactPubKey...)

	err := s.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()

		var keysToDelete [][]byte
		for it.Rewind(); it.Valid(); it.Next() {
			key := it.Item().KeyCopy(nil)
			keysToDelete = append(keysToDelete, key)
		}

		for _, key := range keysToDelete {
			if err := txn.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to delete messages: %w", err)
	}

	return nil
}

// Close gracefully shuts down the database
func (s *BadgerStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// DB returns the underlying BadgerDB instance
// This is used by subsystems that need direct database access (e.g., mailbox manager)
func (s *BadgerStorage) DB() *badger.DB {
	return s.db
}

// peerKey creates a key for peer storage
// Format: peerPrefix + peer_id
func peerKey(peerID string) []byte {
	key := make([]byte, 0, len(peerPrefix)+len(peerID))
	key = append(key, peerPrefix...)
	key = append(key, []byte(peerID)...)
	return key
}

// configKey creates a key for config storage
// Format: configPrefix + key
func configKey(key string) []byte {
	k := make([]byte, 0, len(configPrefix)+len(key))
	k = append(k, configPrefix...)
	k = append(k, []byte(key)...)
	return k
}

// AddPeer stores a peer record in the database
func (s *BadgerStorage) AddPeer(peer *PeerRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(peer)
	if err != nil {
		return fmt.Errorf("failed to marshal peer record: %w", err)
	}

	key := peerKey(peer.PeerID)

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("failed to store peer: %w", err)
	}

	return nil
}

// GetPeer retrieves a peer by peer ID
func (s *BadgerStorage) GetPeer(peerID string) (*PeerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := peerKey(peerID)

	var peer PeerRecord
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, &peer)
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve peer: %w", err)
	}

	return &peer, nil
}

// ListPeers returns all peers, limited to the specified count (0 = no limit)
func (s *BadgerStorage) ListPeers(limit int) ([]*PeerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var peers []*PeerRecord

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(peerPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		count := 0
		for it.Rewind(); it.Valid(); it.Next() {
			if limit > 0 && count >= limit {
				break
			}

			item := it.Item()
			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			var peer PeerRecord
			if err := json.Unmarshal(data, &peer); err != nil {
				return err
			}
			peers = append(peers, &peer)
			count++
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list peers: %w", err)
	}

	return peers, nil
}

// ListPeersBySource returns peers filtered by their source
func (s *BadgerStorage) ListPeersBySource(source PeerSource) ([]*PeerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var peers []*PeerRecord

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(peerPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			var peer PeerRecord
			if err := json.Unmarshal(data, &peer); err != nil {
				return err
			}
			if peer.Source == source {
				peers = append(peers, &peer)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list peers by source: %w", err)
	}

	return peers, nil
}

// DeletePeer removes a peer from the database
func (s *BadgerStorage) DeletePeer(peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := peerKey(peerID)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
	if err != nil {
		return fmt.Errorf("failed to delete peer: %w", err)
	}

	return nil
}

// PrunePeers removes stale peers and keeps only the best peers
// maxAgeDays: remove peers not seen in this many days
// keepCount: maximum number of peers to keep (0 = no limit)
func (s *BadgerStorage) PrunePeers(maxAgeDays int, keepCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	maxAge := time.Duration(maxAgeDays) * 24 * time.Hour
	cutoff := time.Now().Add(-maxAge)

	var peersToDelete []string

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(peerPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		var allPeers []*PeerRecord

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			var peer PeerRecord
			if err := json.Unmarshal(data, &peer); err != nil {
				return err
			}
			allPeers = append(allPeers, &peer)
		}

		// Sort peers by success rate and recency
		sort.Slice(allPeers, func(i, j int) bool {
			// Prefer higher success rate
			if allPeers[i].SuccessRate() != allPeers[j].SuccessRate() {
				return allPeers[i].SuccessRate() > allPeers[j].SuccessRate()
			}
			// Prefer more recent
			return allPeers[i].LastSeen.After(allPeers[j].LastSeen)
		})

		// Mark stale peers for deletion
		for _, peer := range allPeers {
			if peer.LastSeen.Before(cutoff) {
				peersToDelete = append(peersToDelete, peer.PeerID)
			}
		}

		// If we have too many peers, mark the lowest-ranked ones
		if keepCount > 0 && len(allPeers) > keepCount {
			for i := keepCount; i < len(allPeers); i++ {
				// Don't double-add if already marked as stale
				alreadyMarked := false
				for _, id := range peersToDelete {
					if id == allPeers[i].PeerID {
						alreadyMarked = true
						break
					}
				}
				if !alreadyMarked {
					peersToDelete = append(peersToDelete, allPeers[i].PeerID)
				}
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan peers for pruning: %w", err)
	}

	// Delete marked peers
	if len(peersToDelete) > 0 {
		err = s.db.Update(func(txn *badger.Txn) error {
			for _, peerID := range peersToDelete {
				key := peerKey(peerID)
				if err := txn.Delete(key); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to delete stale peers: %w", err)
		}
		logger.Debugw("pruned stale peers", "count", len(peersToDelete))
	}

	return nil
}

// GetConfig retrieves a configuration value by key
func (s *BadgerStorage) GetConfig(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dbKey := configKey(key)

	var value string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(dbKey)
		if err != nil {
			return err
		}
		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		value = string(data)
		return nil
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return "", nil
		}
		return "", fmt.Errorf("failed to retrieve config: %w", err)
	}

	return value, nil
}

// SetConfig stores a configuration value
func (s *BadgerStorage) SetConfig(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dbKey := configKey(key)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(dbKey, []byte(value))
	})
	if err != nil {
		return fmt.Errorf("failed to store config: %w", err)
	}

	return nil
}

// DeleteConfig removes a configuration value
func (s *BadgerStorage) DeleteConfig(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dbKey := configKey(key)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(dbKey)
	})
	if err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
	}

	return nil
}

// blacklistKey creates a key for blacklist storage
// Format: blacklistPrefix + peer_id
func blacklistKey(peerID string) []byte {
	key := make([]byte, 0, len(blacklistPrefix)+len(peerID))
	key = append(key, blacklistPrefix...)
	key = append(key, []byte(peerID)...)
	return key
}

// BlacklistPeer adds a peer to the blacklist with a reason
func (s *BadgerStorage) BlacklistPeer(peerID string, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := &BlacklistEntry{
		PeerID:        peerID,
		Reason:        reason,
		BlacklistedAt: time.Now(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal blacklist entry: %w", err)
	}

	key := blacklistKey(peerID)

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("failed to store blacklist entry: %w", err)
	}

	logger.Infow("peer blacklisted", "peer", peerID, "reason", reason)
	return nil
}

// IsBlacklisted checks if a peer is blacklisted
func (s *BadgerStorage) IsBlacklisted(peerID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := blacklistKey(peerID)

	var entry BlacklistEntry
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, &entry)
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return false, nil
		}
		return false, fmt.Errorf("failed to check blacklist: %w", err)
	}

	// Check if entry has expired
	if entry.IsExpired() {
		// Auto-remove expired entry
		bterrors.SafeGo("blacklist-expire-cleanup", func() {
			if err := s.DeleteConfig(string(blacklistKey(peerID))); err != nil {
				logger.Debugw("failed to delete expired blacklist entry", "peer", peerID, "error", err)
			}
		})
		return false, nil
	}

	return true, nil
}

// ListBlacklisted returns all blacklisted peers
func (s *BadgerStorage) ListBlacklisted() ([]*BlacklistEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var entries []*BlacklistEntry

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(blacklistPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			var entry BlacklistEntry
			if err := json.Unmarshal(data, &entry); err != nil {
				return err
			}
			// Skip expired entries
			if !entry.IsExpired() {
				entries = append(entries, &entry)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list blacklisted peers: %w", err)
	}

	return entries, nil
}

// RemoveFromBlacklist removes a peer from the blacklist
func (s *BadgerStorage) RemoveFromBlacklist(peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := blacklistKey(peerID)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
	if err != nil {
		return fmt.Errorf("failed to remove from blacklist: %w", err)
	}

	logger.Infow("peer removed from blacklist", "peer", peerID)
	return nil
}
