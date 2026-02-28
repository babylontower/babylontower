package storage

import (
	"encoding/hex"
	"fmt"

	pb "babylontower/pkg/proto"
	"github.com/dgraph-io/badger/v3"
	"google.golang.org/protobuf/proto"
)

const (
	// Key prefixes for channels
	channelPrefix      = "ch:"
	channelPostPrefix  = "chp:"
)

// channelKey creates a key for channel storage
// Format: channelPrefix + channel_id (hex encoded)
func channelKey(channelID []byte) []byte {
	key := make([]byte, 0, 3+hex.EncodedLen(len(channelID)))
	key = append(key, channelPrefix...)
	key = append(key, []byte(hex.EncodeToString(channelID))...)
	return key
}

// channelPostKey creates a key for channel post storage
// Format: channelPostPrefix + channel_id (hex) + ":" + post_id (hex)
func channelPostKey(channelID, postID []byte) []byte {
	key := make([]byte, 0, 4+hex.EncodedLen(len(channelID))+1+hex.EncodedLen(len(postID)))
	key = append(key, channelPostPrefix...)
	key = append(key, []byte(hex.EncodeToString(channelID))...)
	key = append(key, ':')
	key = append(key, []byte(hex.EncodeToString(postID))...)
	return key
}

// SaveChannel stores a channel state in the database
func (s *BadgerStorage) SaveChannel(channel *pb.ChannelState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := proto.Marshal(channel)
	if err != nil {
		return fmt.Errorf("failed to marshal channel state: %w", err)
	}

	key := channelKey(channel.ChannelId)

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})

	if err != nil {
		return fmt.Errorf("failed to store channel: %w", err)
	}

	logger.Debugw("stored channel", "channel", hex.EncodeToString(channel.ChannelId[:8]))
	return nil
}

// GetChannel retrieves a channel state from the database
func (s *BadgerStorage) GetChannel(channelID []byte) (*pb.ChannelState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := channelKey(channelID)

	var channel *pb.ChannelState
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

		channel = &pb.ChannelState{}
		return proto.Unmarshal(data, channel)
	})

	if err != nil {
		if err == ErrGroupNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get channel: %w", err)
	}

	return channel, nil
}

// ListChannels returns all channels in the database
func (s *BadgerStorage) ListChannels() ([]*pb.ChannelState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var channels []*pb.ChannelState

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(channelPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			channel := &pb.ChannelState{}
			if err := proto.Unmarshal(data, channel); err != nil {
				return err
			}

			channels = append(channels, channel)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list channels: %w", err)
	}

	return channels, nil
}

// DeleteChannel removes a channel from the database
func (s *BadgerStorage) DeleteChannel(channelID []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := channelKey(channelID)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})

	if err != nil {
		return fmt.Errorf("failed to delete channel: %w", err)
	}

	logger.Debugw("deleted channel", "channel", hex.EncodeToString(channelID[:8]))
	return nil
}

// SaveChannelPost stores a channel post in the database
func (s *BadgerStorage) SaveChannelPost(post *pb.ChannelPost) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := proto.Marshal(post)
	if err != nil {
		return fmt.Errorf("failed to marshal channel post: %w", err)
	}

	key := channelPostKey(post.ChannelId, post.PostId)

	err = s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update channel's latest_post_cid
		channel, err := s.getChannelTxn(txn, post.ChannelId)
		if err == nil && channel != nil {
			channel.LatestPostCid = post.PreviousPostCid
			channel.UpdatedAt = post.Timestamp
			channelData, marshalErr := proto.Marshal(channel)
			if marshalErr == nil {
				if err := txn.Set(channelKey(post.ChannelId), channelData); err != nil {
					return fmt.Errorf("failed to update channel: %w", err)
				}
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to store channel post: %w", err)
	}

	logger.Debugw("stored channel post", "post", hex.EncodeToString(post.PostId[:8]), "channel", hex.EncodeToString(post.ChannelId[:8]))
	return nil
}

// GetChannelPosts retrieves posts from a channel
func (s *BadgerStorage) GetChannelPosts(channelID []byte, limit, offset int) ([]*pb.ChannelPost, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var posts []*pb.ChannelPost

	prefix := append([]byte(channelPostPrefix), []byte(hex.EncodeToString(channelID))...)
	prefix = append(prefix, ':')

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()

		count := 0
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			// Skip index key
			if len(item.Key()) > len(prefix) && string(item.Key()[len(prefix):]) == "idx" {
				continue
			}

			if count < offset {
				count++
				continue
			}

			if limit > 0 && len(posts) >= limit {
				break
			}

			data, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			post := &pb.ChannelPost{}
			if err := proto.Unmarshal(data, post); err != nil {
				return err
			}

			posts = append(posts, post)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list channel posts: %w", err)
	}

	return posts, nil
}

// GetLatestChannelPostCID retrieves the latest post CID for a channel
func (s *BadgerStorage) GetLatestChannelPostCID(channelID []byte) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := channelKey(channelID)

	var latestCID []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return err
		}

		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		channel := &pb.ChannelState{}
		if err := proto.Unmarshal(data, channel); err != nil {
			return err
		}

		latestCID = channel.LatestPostCid
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get latest post CID: %w", err)
	}

	return latestCID, nil
}

// getChannelTxn retrieves a channel within a transaction (internal helper)
func (s *BadgerStorage) getChannelTxn(txn *badger.Txn, channelID []byte) (*pb.ChannelState, error) {
	key := channelKey(channelID)

	item, err := txn.Get(key)
	if err != nil {
		return nil, err
	}

	data, err := item.ValueCopy(nil)
	if err != nil {
		return nil, err
	}

	channel := &pb.ChannelState{}
	err = proto.Unmarshal(data, channel)
	if err != nil {
		return nil, err
	}

	return channel, nil
}
