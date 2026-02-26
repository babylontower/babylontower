package storage

import (
	"encoding/hex"
	"fmt"

	pb "babylontower/pkg/proto"
	"github.com/dgraph-io/badger/v3"
	"google.golang.org/protobuf/proto"
)

// Note: groupPrefix and senderKeyPrefix are defined in badger.go

// groupKey creates a key for group storage
// Format: groupPrefix + group_id (hex encoded)
func groupKey(groupID []byte) []byte {
	key := make([]byte, 0, 2+hex.EncodedLen(len(groupID)))
	key = append(key, groupPrefix...)
	key = append(key, []byte(hex.EncodeToString(groupID))...)
	return key
}

// senderKeyKey creates a key for sender key storage
// Format: senderKeyPrefix + group_id (hex) + ":" + sender_pubkey (hex)
func senderKeyKey(groupID, senderPubkey []byte) []byte {
	key := make([]byte, 0, 3+hex.EncodedLen(len(groupID))+1+hex.EncodedLen(len(senderPubkey)))
	key = append(key, senderKeyPrefix...)
	key = append(key, []byte(hex.EncodeToString(groupID))...)
	key = append(key, ':')
	key = append(key, []byte(hex.EncodeToString(senderPubkey))...)
	return key
}

// SaveGroup stores a group state in the database
func (s *BadgerStorage) SaveGroup(group *pb.GroupState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := proto.Marshal(group)
	if err != nil {
		return fmt.Errorf("failed to marshal group state: %w", err)
	}

	key := groupKey(group.GroupId)

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})

	if err != nil {
		return fmt.Errorf("failed to store group: %w", err)
	}

	logger.Debugf("Stored group %s with epoch %d", hex.EncodeToString(group.GroupId), group.Epoch)
	return nil
}

// GetGroup retrieves a group state from the database
func (s *BadgerStorage) GetGroup(groupID []byte) (*pb.GroupState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := groupKey(groupID)

	var group *pb.GroupState
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return ErrGroupNotFound
			}
			return err
		}

		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		group = &pb.GroupState{}
		return proto.Unmarshal(data, group)
	})

	if err != nil {
		if err == ErrGroupNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get group: %w", err)
	}

	return group, nil
}

// ListGroups returns all groups in the database
func (s *BadgerStorage) ListGroups() ([]*pb.GroupState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var groups []*pb.GroupState

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(groupPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			group := &pb.GroupState{}
			if err := proto.Unmarshal(data, group); err != nil {
				return err
			}

			groups = append(groups, group)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}

	return groups, nil
}

// DeleteGroup removes a group from the database
func (s *BadgerStorage) DeleteGroup(groupID []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := groupKey(groupID)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})

	if err != nil {
		return fmt.Errorf("failed to delete group: %w", err)
	}

	logger.Debugf("Deleted group %s", hex.EncodeToString(groupID))
	return nil
}

// SaveSenderKey stores a sender key distribution in the database
func (s *BadgerStorage) SaveSenderKey(sk *pb.SenderKeyDistribution) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := proto.Marshal(sk)
	if err != nil {
		return fmt.Errorf("failed to marshal sender key: %w", err)
	}

	key := senderKeyKey(sk.GroupId, sk.SenderPub)

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})

	if err != nil {
		return fmt.Errorf("failed to store sender key: %w", err)
	}

	logger.Debugf("Stored sender key for group %s, sender %s", 
		hex.EncodeToString(sk.GroupId), hex.EncodeToString(sk.SenderPub))
	return nil
}

// GetSenderKey retrieves a sender key from the database
func (s *BadgerStorage) GetSenderKey(groupID, senderPubkey []byte) (*pb.SenderKeyDistribution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := senderKeyKey(groupID, senderPubkey)

	var sk *pb.SenderKeyDistribution
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return ErrSenderKeyNotFound
			}
			return err
		}

		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		sk = &pb.SenderKeyDistribution{}
		return proto.Unmarshal(data, sk)
	})

	if err != nil {
		if err == ErrSenderKeyNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get sender key: %w", err)
	}

	return sk, nil
}

// ListSenderKeys returns all sender keys for a group
func (s *BadgerStorage) ListSenderKeys(groupID []byte) ([]*pb.SenderKeyDistribution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var senderKeys []*pb.SenderKeyDistribution

	prefix := append([]byte(senderKeyPrefix), []byte(hex.EncodeToString(groupID))...)
	prefix = append(prefix, ':')

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

			sk := &pb.SenderKeyDistribution{}
			if err := proto.Unmarshal(data, sk); err != nil {
				return err
			}

			senderKeys = append(senderKeys, sk)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list sender keys: %w", err)
	}

	return senderKeys, nil
}

// DeleteSenderKey removes a sender key from the database
func (s *BadgerStorage) DeleteSenderKey(groupID, senderPubkey []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := senderKeyKey(groupID, senderPubkey)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})

	if err != nil {
		return fmt.Errorf("failed to delete sender key: %w", err)
	}

	return nil
}

// DeleteAllSenderKeys removes all sender keys for a group
func (s *BadgerStorage) DeleteAllSenderKeys(groupID []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prefix := append([]byte(senderKeyPrefix), []byte(hex.EncodeToString(groupID))...)
	prefix = append(prefix, ':')

	err := s.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()

		var keysToDelete [][]byte
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			keysToDelete = append(keysToDelete, []byte(item.Key()))
		}

		for _, key := range keysToDelete {
			if err := txn.Delete(key); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to delete sender keys: %w", err)
	}

	return nil
}
