package storage

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	pb "babylontower/pkg/proto"
	"github.com/dgraph-io/badger/v3"
	"google.golang.org/protobuf/proto"
)

// Identity v1 storage key prefixes
const (
	// Identity v1 prefixes
	identityPrefix    = "id:"    // Identity documents by identity pubkey hash
	devicePrefix      = "dev:"   // Device certificates by device ID
	spkPrefix         = "spk:"   // Signed prekeys by prekey ID
	opkPrefix         = "opk:"   // One-time prekeys by prekey ID
	sessionPrefix     = "dr:"    // Double Ratchet session state by session ID
	prekeyBundlePrefix = "pb:"   // Prekey bundle cache by identity pubkey hash
)

// IdentityDocumentRecord wraps a protobuf IdentityDocument with metadata
type IdentityDocumentRecord struct {
	Document      *pb.IdentityDocument `json:"document"`
	FetchedAt     time.Time            `json:"fetched_at"`
	Source        string               `json:"source"` // "local", "dht", "peer"
	DocumentHash  []byte               `json:"document_hash"`
}

// SessionState stores Double Ratchet session state
type SessionState struct {
	SessionID              string    `json:"session_id"`
	RemoteIdentityPub      []byte    `json:"remote_identity_pub"`
	LocalIdentityPub       []byte    `json:"local_identity_pub"`
	DHSendingKeyPriv       []byte    `json:"dh_sending_key_priv"`
	DHSendingKeyPub        []byte    `json:"dh_sending_key_pub"`
	DHReceivingPub         []byte    `json:"dh_receiving_pub"`
	RootKey                []byte    `json:"root_key"`
	SendingChainKey        []byte    `json:"sending_chain_key"`
	SendingChainCounter    uint32    `json:"sending_chain_counter"`
	ReceivingChainKey      []byte    `json:"receiving_chain_key"`
	ReceivingChainCounter  uint32    `json:"receiving_chain_counter"`
	SkippedKeys            []SkippedKey `json:"skipped_keys"`
	CreatedAt              time.Time `json:"created_at"`
	LastUsedAt             time.Time `json:"last_used_at"`
	CipherSuiteID          uint32    `json:"cipher_suite_id"`
}

// SkippedKey stores a skipped message key for out-of-order delivery
type SkippedKey struct {
	DHRatchetPub []byte `json:"dh_ratchet_pub"`
	Counter      uint32 `json:"counter"`
	Key          []byte `json:"key"`
}

// PrekeyBundleCache caches prekey bundles fetched from DHT
type PrekeyBundleCache struct {
	IdentityPub    []byte               `json:"identity_pub"`
	SignedPrekeys  []*pb.SignedPrekey   `json:"signed_prekeys"`
	OneTimePrekeys []*pb.OneTimePrekey  `json:"one_time_prekeys"`
	FetchedAt      time.Time            `json:"fetched_at"`
	ConsumedOPKs   map[uint64]bool      `json:"consumed_opks"` // Track consumed OPK IDs
}

// identityKey creates a key for identity document storage
func identityKey(identityPubHash []byte) []byte {
	key := make([]byte, 0, len(identityPrefix)+len(identityPubHash))
	key = append(key, identityPrefix...)
	key = append(key, identityPubHash...)
	return key
}

// deviceKey creates a key for device certificate storage
func deviceKey(deviceID []byte) []byte {
	key := make([]byte, 0, len(devicePrefix)+len(deviceID))
	key = append(key, devicePrefix...)
	key = append(key, deviceID...)
	return key
}

// spkKey creates a key for signed prekey storage
func spkKey(prekeyID uint64) []byte {
	key := make([]byte, 0, len(spkPrefix)+8)
	key = append(key, spkPrefix...)
	idBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(idBytes, prekeyID)
	key = append(key, idBytes...)
	return key
}

// opkKey creates a key for one-time prekey storage
func opkKey(prekeyID uint64) []byte {
	key := make([]byte, 0, len(opkPrefix)+8)
	key = append(key, opkPrefix...)
	idBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(idBytes, prekeyID)
	key = append(key, idBytes...)
	return key
}

// sessionKey creates a key for session state storage
func sessionKey(sessionID string) []byte {
	key := make([]byte, 0, len(sessionPrefix)+len(sessionID))
	key = append(key, sessionPrefix...)
	key = append(key, []byte(sessionID)...)
	return key
}

// prekeyBundleKey creates a key for prekey bundle cache
func prekeyBundleKey(identityPubHash []byte) []byte {
	key := make([]byte, 0, len(prekeyBundlePrefix)+len(identityPubHash))
	key = append(key, prekeyBundlePrefix...)
	key = append(key, identityPubHash...)
	return key
}

// SaveIdentityDocument stores an identity document
func (s *BadgerStorage) SaveIdentityDocument(identityPub ed25519.PublicKey, record *IdentityDocumentRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Compute identity pubkey hash for key derivation
	hash := sha256.Sum256(identityPub)
	key := identityKey(hash[:16])

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal identity document record: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("failed to store identity document: %w", err)
	}

	return nil
}

// GetIdentityDocument retrieves an identity document by identity public key
func (s *BadgerStorage) GetIdentityDocument(identityPub ed25519.PublicKey) (*IdentityDocumentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hash := sha256.Sum256(identityPub)
	key := identityKey(hash[:16])

	var record IdentityDocumentRecord
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, &record)
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve identity document: %w", err)
	}

	return &record, nil
}

// SaveDeviceCertificate stores a device certificate
func (s *BadgerStorage) SaveDeviceCertificate(deviceID []byte, cert *pb.DeviceCertificate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := deviceKey(deviceID)

	data, err := proto.Marshal(cert)
	if err != nil {
		return fmt.Errorf("failed to marshal device certificate: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("failed to store device certificate: %w", err)
	}

	return nil
}

// GetDeviceCertificate retrieves a device certificate by device ID
func (s *BadgerStorage) GetDeviceCertificate(deviceID []byte) (*pb.DeviceCertificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := deviceKey(deviceID)

	var cert pb.DeviceCertificate
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return proto.Unmarshal(data, &cert)
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve device certificate: %w", err)
	}

	return &cert, nil
}

// SaveSignedPrekey stores a signed prekey
func (s *BadgerStorage) SaveSignedPrekey(spk *pb.SignedPrekey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := spkKey(spk.PrekeyId)

	data, err := proto.Marshal(spk)
	if err != nil {
		return fmt.Errorf("failed to marshal signed prekey: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("failed to store signed prekey: %w", err)
	}

	return nil
}

// GetSignedPrekey retrieves a signed prekey by prekey ID
func (s *BadgerStorage) GetSignedPrekey(prekeyID uint64) (*pb.SignedPrekey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := spkKey(prekeyID)

	var spk pb.SignedPrekey
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return proto.Unmarshal(data, &spk)
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve signed prekey: %w", err)
	}

	return &spk, nil
}

// SaveOneTimePrekey stores a one-time prekey
func (s *BadgerStorage) SaveOneTimePrekey(opk *pb.OneTimePrekey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := opkKey(opk.PrekeyId)

	data, err := proto.Marshal(opk)
	if err != nil {
		return fmt.Errorf("failed to marshal one-time prekey: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("failed to store one-time prekey: %w", err)
	}

	return nil
}

// GetOneTimePrekey retrieves a one-time prekey by prekey ID
func (s *BadgerStorage) GetOneTimePrekey(prekeyID uint64) (*pb.OneTimePrekey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := opkKey(prekeyID)

	var opk pb.OneTimePrekey
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return proto.Unmarshal(data, &opk)
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve one-time prekey: %w", err)
	}

	return &opk, nil
}

// DeleteOneTimePrekey removes a consumed one-time prekey
func (s *BadgerStorage) DeleteOneTimePrekey(prekeyID uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := opkKey(prekeyID)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
	if err != nil {
		return fmt.Errorf("failed to delete one-time prekey: %w", err)
	}

	return nil
}

// ListOneTimePrekeys lists all stored one-time prekeys
func (s *BadgerStorage) ListOneTimePrekeys() ([]*pb.OneTimePrekey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var opks []*pb.OneTimePrekey

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(opkPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			var opk pb.OneTimePrekey
			if err := proto.Unmarshal(data, &opk); err != nil {
				return err
			}
			opks = append(opks, &opk)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list one-time prekeys: %w", err)
	}

	return opks, nil
}

// SaveSessionState stores a Double Ratchet session state
func (s *BadgerStorage) SaveSessionState(state *SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := sessionKey(state.SessionID)

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal session state: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("failed to store session state: %w", err)
	}

	return nil
}

// GetSessionState retrieves a session state by session ID
func (s *BadgerStorage) GetSessionState(sessionID string) (*SessionState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := sessionKey(sessionID)

	var state SessionState
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, &state)
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve session state: %w", err)
	}

	return &state, nil
}

// SavePrekeyBundleCache stores a prekey bundle cache
func (s *BadgerStorage) SavePrekeyBundleCache(identityPub ed25519.PublicKey, cache *PrekeyBundleCache) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	hash := sha256.Sum256(identityPub)
	key := prekeyBundleKey(hash[:16])

	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("failed to marshal prekey bundle cache: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("failed to store prekey bundle cache: %w", err)
	}

	return nil
}

// GetPrekeyBundleCache retrieves a prekey bundle cache
func (s *BadgerStorage) GetPrekeyBundleCache(identityPub ed25519.PublicKey) (*PrekeyBundleCache, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hash := sha256.Sum256(identityPub)
	key := prekeyBundleKey(hash[:16])

	var cache PrekeyBundleCache
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, &cache)
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve prekey bundle cache: %w", err)
	}

	return &cache, nil
}

// CountOneTimePrekeys counts the number of stored one-time prekeys
func (s *BadgerStorage) CountOneTimePrekeys() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(opkPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			count++
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count one-time prekeys: %w", err)
	}

	return count, nil
}
