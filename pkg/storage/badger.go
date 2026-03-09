package storage

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	bterrors "babylontower/pkg/errors"
	pb "babylontower/pkg/proto"

	"github.com/dgraph-io/badger/v3"
	"github.com/ipfs/go-log/v2"
	"google.golang.org/protobuf/proto"
)

var logger = log.Logger("babylontower/storage")

const (
	// Key prefixes — §11 defines 16 prefixes for Protocol v1
	// Core (PoC layer):
	contactPrefix   = "c:"
	messagePrefix   = "m:"
	peerPrefix      = "p:"
	configPrefix    = "cfg:"
	groupPrefix     = "g:"
	senderKeyPrefix = "sk:"
	blacklistPrefix = "bl:"
	// Protocol v1 prefixes (defined in identity_v1.go):
	//   identityPrefix     = "id:"   — Identity documents
	//   devicePrefix       = "dev:"  — Device certificates
	//   spkPrefix          = "spk:"  — Signed prekeys
	//   opkPrefix          = "opk:"  — One-time prekeys
	//   sessionPrefix      = "dr:"   — Double Ratchet sessions
	//   prekeyBundlePrefix = "pb:"   — Prekey bundle cache
	// Additional Protocol v1 prefixes:
	groupStatePrefix = "gs:" // Group state (distinct from group metadata in "g:")
	mailboxPrefix    = "mb:" // Mailbox messages
	unreadPrefix     = "un:" // Unread message counters

	// Key component sizes
	pubKeySize    = 32
	timestampSize = 8
	nonceSize     = 24
)

// BadgerStorage implements Storage using BadgerDB.
// No external mutex needed — BadgerDB v3 provides MVCC snapshot isolation.
type BadgerStorage struct {
	db *badger.DB
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


	key := contactKey(pubKey)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
	if err != nil {
		return fmt.Errorf("failed to delete contact: %w", err)
	}

	return nil
}

// AddMessage stores a plaintext message for a contact
func (s *BadgerStorage) AddMessage(contactPubKey []byte, msg *StoredMessage) error {
	timestamp := msg.Timestamp
	if timestamp == 0 {
		timestamp = uint64(time.Now().Unix())
	}

	// Generate random suffix for key uniqueness
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return fmt.Errorf("failed to generate key suffix: %w", err)
	}

	key := messageKey(contactPubKey, timestamp, suffix)

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("failed to store message: %w", err)
	}

	return nil
}

// GetMessages retrieves plaintext messages for a contact, ordered by timestamp.
// limit specifies maximum number of messages (0 = no limit).
// offset specifies number of messages to skip.
func (s *BadgerStorage) GetMessages(contactPubKey []byte, limit, offset int) ([]*StoredMessage, error) {
	var messages []*StoredMessage

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
			if skipped < offset {
				skipped++
				continue
			}
			if limit > 0 && count >= limit {
				break
			}

			item := it.Item()
			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			var msg StoredMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				// Skip entries that can't be decoded (e.g. old encrypted format)
				continue
			}
			messages = append(messages, &msg)
			count++
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve messages: %w", err)
	}

	return messages, nil
}

// DeleteMessages removes all messages for a contact
func (s *BadgerStorage) DeleteMessages(contactPubKey []byte) error {


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
			if err := s.RemoveFromBlacklist(peerID); err != nil {
				logger.Debugw("failed to delete expired blacklist entry", "peer", peerID, "error", err)
			}
		})
		return false, nil
	}

	return true, nil
}

// ListBlacklisted returns all blacklisted peers
func (s *BadgerStorage) ListBlacklisted() ([]*BlacklistEntry, error) {


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
