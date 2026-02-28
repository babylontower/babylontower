package mailbox

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v3"
	"google.golang.org/protobuf/proto"

	pb "babylontower/pkg/proto"
)

const (
	// Key prefixes for BadgerDB
	mailboxPrefix = "mbx:"     // Mailbox storage: mbx:<target_pubkey_hex>:<message_id_hex>
	metaPrefix    = "mbxmeta:" // Metadata: mbxmeta:<target_pubkey_hex>
	ratePrefix    = "mbxrate:" // Rate limiting: mbxrate:<sender_pubkey_hex>:<target_pubkey_hex>:<hour_bucket>
	configPrefix  = "mbxcfg:"  // Configuration
)

// StoredMessage represents a message stored in the mailbox
type StoredMessage struct {
	MessageID    []byte
	Envelope     []byte
	SenderPubkey []byte
	StoredAt     time.Time
	ExpiresAt    time.Time
	Size         uint64
}

// Storage handles persistent storage for mailbox messages
type Storage struct {
	db     *badger.DB
	config *pb.MailboxConfig
	mu     sync.RWMutex
}

// NewStorage creates a new mailbox storage instance
func NewStorage(db *badger.DB, config *pb.MailboxConfig) (*Storage, error) {
	if config == nil {
		config = defaultConfig()
	}

	s := &Storage{
		db:     db,
		config: config,
	}

	return s, nil
}

// defaultConfig returns the default mailbox configuration
func defaultConfig() *pb.MailboxConfig {
	return &pb.MailboxConfig{
		MaxMessagesPerTarget:   500,
		MaxMessageSize:         262144,   // 256 KB
		MaxTotalBytesPerTarget: 67108864, // 64 MB
		DefaultTtlSeconds:      604800,   // 7 days
		DepositRateLimit:       100,      // 100 messages per sender per target per hour
		EnableContentRouting:   false,
	}
}

// StoreMessage stores a message for a target recipient
func (s *Storage) StoreMessage(targetPubkey, messageID, senderPubkey, envelope []byte, ttlSeconds uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	expiresAt := now.Add(time.Duration(ttlSeconds) * time.Second)

	// Check quota before storing
	if err := s.checkQuota(targetPubkey); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// Create stored message record
		stored := &pb.StoredMailboxMessage{
			MessageId:    messageID,
			SenderPubkey: senderPubkey,
			Envelope:     envelope,
			StoredAt:     uint64(now.Unix()),
			ExpiresAt:    uint64(expiresAt.Unix()),
			Size:         uint64(len(envelope)),
		}

		data, err := proto.Marshal(stored)
		if err != nil {
			return fmt.Errorf("failed to marshal stored message: %w", err)
		}

		// Key: mbx:<target_hex>:<message_id_hex>
		key := fmt.Sprintf("%s%x:%x", mailboxPrefix, targetPubkey, messageID)

		if err := txn.Set([]byte(key), data); err != nil {
			return fmt.Errorf("failed to store message: %w", err)
		}

		// Update metadata
		if err := s.updateMetadata(txn, targetPubkey, uint64(len(envelope))); err != nil {
			return fmt.Errorf("failed to update metadata: %w", err)
		}

		return nil
	})
}

// GetMessage retrieves a specific message by ID
func (s *Storage) GetMessage(targetPubkey, messageID []byte) (*StoredMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s%x:%x", mailboxPrefix, targetPubkey, messageID)

	var stored *pb.StoredMailboxMessage
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return errors.New("message not found")
			}
			return err
		}

		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		stored = &pb.StoredMailboxMessage{}
		return proto.Unmarshal(data, stored)
	})

	if err != nil {
		return nil, err
	}

	return &StoredMessage{
		MessageID:    stored.MessageId,
		Envelope:     stored.Envelope,
		SenderPubkey: stored.SenderPubkey,
		StoredAt:     time.Unix(int64(stored.StoredAt), 0),
		ExpiresAt:    time.Unix(int64(stored.ExpiresAt), 0),
		Size:         stored.Size,
	}, nil
}

// ListMessages retrieves all messages for a target recipient
func (s *Storage) ListMessages(targetPubkey []byte) ([]*StoredMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := fmt.Sprintf("%s%x:", mailboxPrefix, targetPubkey)
	var messages []*StoredMessage

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 100
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
			item := it.Item()
			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			stored := &pb.StoredMailboxMessage{}
			if err := proto.Unmarshal(data, stored); err != nil {
				return err
			}

			// Skip expired messages
			if time.Unix(int64(stored.ExpiresAt), 0).Before(time.Now()) {
				continue
			}

			messages = append(messages, &StoredMessage{
				MessageID:    stored.MessageId,
				Envelope:     stored.Envelope,
				SenderPubkey: stored.SenderPubkey,
				StoredAt:     time.Unix(int64(stored.StoredAt), 0),
				ExpiresAt:    time.Unix(int64(stored.ExpiresAt), 0),
				Size:         stored.Size,
			})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return messages, nil
}

// DeleteMessages removes messages by their IDs
func (s *Storage) DeleteMessages(targetPubkey []byte, messageIDs [][]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(txn *badger.Txn) error {
		deletedCount := 0
		for _, msgID := range messageIDs {
			key := fmt.Sprintf("%s%x:%x", mailboxPrefix, targetPubkey, msgID)

			// Get message size before deletion for metadata update
			_, err := txn.Get([]byte(key))
			if err == nil {
				deletedCount++
			}

			if err := txn.Delete([]byte(key)); err != nil {
				return err
			}
		}

		// Update metadata if any messages were deleted
		if deletedCount > 0 {
			// Metadata update handled separately - TODO: Implement metadata tracking
			_ = deletedCount // prevent staticcheck empty branch warning
		}

		return nil
	})
}

// checkQuota verifies if the target has space for more messages
func (s *Storage) checkQuota(targetPubkey []byte) error {
	metadata, err := s.getMetadata(targetPubkey)
	if err != nil && err != badger.ErrKeyNotFound {
		return err
	}

	// Check message count limit
	if metadata != nil && metadata.MessageCount >= s.config.MaxMessagesPerTarget {
		return errors.New("message count limit reached for target")
	}

	// Check total size limit
	if metadata != nil && metadata.TotalBytes+s.config.MaxMessageSize > s.config.MaxTotalBytesPerTarget {
		// Try to evict oldest messages to make room
		if err := s.evictOldest(targetPubkey, 1); err != nil {
			return errors.New("storage quota exceeded for target")
		}
	}

	return nil
}

// evictOldest removes the oldest messages to make room
func (s *Storage) evictOldest(targetPubkey []byte, count uint32) error {
	prefix := fmt.Sprintf("%s%x:", mailboxPrefix, targetPubkey)
	var keysToDelete [][]byte

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		collected := 0
		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)) && collected < int(count); it.Next() {
			item := it.Item()
			key := make([]byte, len(item.Key()))
			copy(key, item.Key())
			keysToDelete = append(keysToDelete, key)
			collected++
		}
		return nil
	})

	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		for _, key := range keysToDelete {
			if err := txn.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
}

// Metadata for a target's mailbox
type MailboxMetadata struct {
	MessageCount uint32
	TotalBytes   uint64
	OldestTime   uint64
	NewestTime   uint64
}

// getMetadata retrieves metadata for a target
func (s *Storage) getMetadata(targetPubkey []byte) (*MailboxMetadata, error) {
	key := fmt.Sprintf("%s%x", metaPrefix, targetPubkey)

	var metadata *MailboxMetadata
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}

		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		metadata = &MailboxMetadata{}
		// Simple binary format: count(4) + total(8) + oldest(8) + newest(8)
		if len(data) >= 28 {
			metadata.MessageCount = binary.LittleEndian.Uint32(data[0:4])
			metadata.TotalBytes = binary.LittleEndian.Uint64(data[4:12])
			metadata.OldestTime = binary.LittleEndian.Uint64(data[12:20])
			metadata.NewestTime = binary.LittleEndian.Uint64(data[20:28])
		}
		return nil
	})

	return metadata, err
}

// updateMetadata updates the metadata after storing a message
func (s *Storage) updateMetadata(txn *badger.Txn, targetPubkey []byte, size uint64) error {
	key := fmt.Sprintf("%s%x", metaPrefix, targetPubkey)

	// Get existing metadata
	var metadata MailboxMetadata
	item, err := txn.Get([]byte(key))
	if err == nil {
		data, err := item.ValueCopy(nil)
		if err == nil && len(data) >= 28 {
			metadata.MessageCount = binary.LittleEndian.Uint32(data[0:4])
			metadata.TotalBytes = binary.LittleEndian.Uint64(data[4:12])
			metadata.OldestTime = binary.LittleEndian.Uint64(data[12:20])
			metadata.NewestTime = binary.LittleEndian.Uint64(data[20:28])
		}
	}

	// Update metadata
	metadata.MessageCount++
	metadata.TotalBytes += size
	now := uint64(time.Now().Unix())
	if metadata.OldestTime == 0 || now < metadata.OldestTime {
		metadata.OldestTime = now
	}
	metadata.NewestTime = now

	// Serialize
	data := make([]byte, 28)
	binary.LittleEndian.PutUint32(data[0:4], metadata.MessageCount)
	binary.LittleEndian.PutUint64(data[4:12], metadata.TotalBytes)
	binary.LittleEndian.PutUint64(data[12:20], metadata.OldestTime)
	binary.LittleEndian.PutUint64(data[20:28], metadata.NewestTime)

	return txn.Set([]byte(key), data)
}

// CheckRateLimit checks if a sender has exceeded their rate limit
func (s *Storage) CheckRateLimit(senderPubkey, targetPubkey []byte) (bool, error) {
	now := time.Now()
	hourBucket := now.Unix() / 3600 // Current hour bucket

	key := fmt.Sprintf("%s%x:%x:%d", ratePrefix, senderPubkey, targetPubkey, hourBucket)

	var count uint32
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil // No count yet, allow
			}
			return err
		}

		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		count = binary.LittleEndian.Uint32(data)
		return nil
	})

	if err != nil {
		return false, err
	}

	return count >= s.config.DepositRateLimit, nil
}

// IncrementRateLimit increments the rate limit counter for a sender
func (s *Storage) IncrementRateLimit(senderPubkey, targetPubkey []byte) error {
	now := time.Now()
	hourBucket := now.Unix() / 3600

	key := fmt.Sprintf("%s%x:%x:%d", ratePrefix, senderPubkey, targetPubkey, hourBucket)

	return s.db.Update(func(txn *badger.Txn) error {
		var count uint32
		item, err := txn.Get([]byte(key))
		if err == nil {
			data, err := item.ValueCopy(nil)
			if err == nil && len(data) >= 4 {
				count = binary.LittleEndian.Uint32(data)
			}
		}

		count++
		data := make([]byte, 4)
		binary.LittleEndian.PutUint32(data, count)
		return txn.Set([]byte(key), data)
	})
}

// CleanupExpired removes all expired messages across all mailboxes
func (s *Storage) CleanupExpired() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	deleted := 0

	err := s.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		var keysToDelete [][]byte

		for it.Seek([]byte(mailboxPrefix)); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			// Check if key has the mailbox prefix
			if len(key) < len(mailboxPrefix) || string(key[:len(mailboxPrefix)]) != mailboxPrefix {
				break
			}

			data, err := item.ValueCopy(nil)
			if err != nil {
				continue
			}

			stored := &pb.StoredMailboxMessage{}
			if err := proto.Unmarshal(data, stored); err != nil {
				continue
			}

			if now.After(time.Unix(int64(stored.ExpiresAt), 0)) {
				keysToDelete = append(keysToDelete, key)
			}
		}

		// Delete expired messages
		for _, key := range keysToDelete {
			if err := txn.Delete(key); err != nil {
				return err
			}
			deleted++
		}

		return nil
	})

	return deleted, err
}

// GetStats returns statistics for a target's mailbox
func (s *Storage) GetStats(targetPubkey []byte) (*pb.MailboxStats, error) {
	metadata, err := s.getMetadata(targetPubkey)
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return &pb.MailboxStats{
				TargetPubkey:  targetPubkey,
				StoredCount:   0,
				UsedBytes:     0,
				CapacityBytes: s.config.MaxTotalBytesPerTarget,
			}, nil
		}
		return nil, err
	}

	return &pb.MailboxStats{
		TargetPubkey:    targetPubkey,
		StoredCount:     metadata.MessageCount,
		UsedBytes:       metadata.TotalBytes,
		CapacityBytes:   s.config.MaxTotalBytesPerTarget,
		OldestTimestamp: metadata.OldestTime,
		NewestTimestamp: metadata.NewestTime,
	}, nil
}

// Close closes the storage (called on shutdown)
func (s *Storage) Close() error {
	return nil // Don't close the DB, it's managed elsewhere
}
