package storage

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	pb "babylontower/pkg/proto"
	"github.com/dgraph-io/badger/v3"
	"google.golang.org/protobuf/proto"
)

const (
	// Key prefixes
	contactPrefix = "c:"
	messagePrefix = "m:"

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

// GetMessages retrieves messages for a contact, ordered by timestamp
// limit specifies maximum number of messages (0 = no limit)
// offset specifies number of messages to skip
func (s *BadgerStorage) GetMessages(contactPubKey []byte, limit, offset int) ([]*pb.SignedEnvelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var envelopes []*pb.SignedEnvelope

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
			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			var envelope pb.SignedEnvelope
			if err := proto.Unmarshal(data, &envelope); err != nil {
				return err
			}
			envelopes = append(envelopes, &envelope)
			count++
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve messages: %w", err)
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
