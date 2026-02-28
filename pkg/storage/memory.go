package storage

import (
	"fmt"
	"sync"
	"time"

	pb "babylontower/pkg/proto"

	"google.golang.org/protobuf/proto"
)

// MemoryStorage is an in-memory implementation of Storage for testing
type MemoryStorage struct {
	mu           sync.RWMutex
	contacts     map[string]*pb.Contact
	messages     map[string][]*pb.SignedEnvelope
	peers        map[string]*PeerRecord
	configs      map[string]string
	blacklist    map[string]*BlacklistEntry
	groups       map[string]*pb.GroupState
	senderKeys   map[string]map[string]*pb.SenderKeyDistribution
	channels     map[string]*pb.ChannelState
	channelPosts map[string]map[string]*pb.ChannelPost
}

// NewMemoryStorage creates a new in-memory storage
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		contacts:     make(map[string]*pb.Contact),
		messages:     make(map[string][]*pb.SignedEnvelope),
		peers:        make(map[string]*PeerRecord),
		configs:      make(map[string]string),
		blacklist:    make(map[string]*BlacklistEntry),
		groups:       make(map[string]*pb.GroupState),
		senderKeys:   make(map[string]map[string]*pb.SenderKeyDistribution),
		channels:     make(map[string]*pb.ChannelState),
		channelPosts: make(map[string]map[string]*pb.ChannelPost),
	}
}

// AddContact stores a contact in memory
func (s *MemoryStorage) AddContact(contact *pb.Contact) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(contact.PublicKey)
	s.contacts[key] = contact
	return nil
}

// GetContact retrieves a contact by public key
func (s *MemoryStorage) GetContact(pubKey []byte) (*pb.Contact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	contact, ok := s.contacts[string(pubKey)]
	if !ok {
		return nil, nil
	}
	return contact, nil
}

// GetContactByBase58 retrieves a contact by base58-encoded public key
func (s *MemoryStorage) GetContactByBase58(pubKeyBase58 string) (*pb.Contact, error) {
	pubKey, err := ContactKeyFromBase58(pubKeyBase58)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base58 public key: %w", err)
	}
	return s.GetContact(pubKey)
}

// GetContactX25519Key retrieves the X25519 public key for a contact
func (s *MemoryStorage) GetContactX25519Key(pubKey []byte) ([]byte, error) {
	contact, err := s.GetContact(pubKey)
	if err != nil {
		return nil, err
	}
	if contact == nil {
		return nil, fmt.Errorf("contact not found")
	}
	if len(contact.X25519PublicKey) == 0 {
		return nil, fmt.Errorf("contact X25519 public key not stored")
	}
	return contact.X25519PublicKey, nil
}

// ListContacts returns all contacts
func (s *MemoryStorage) ListContacts() ([]*pb.Contact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	contacts := make([]*pb.Contact, 0, len(s.contacts))
	for _, contact := range s.contacts {
		contacts = append(contacts, contact)
	}
	return contacts, nil
}

// DeleteContact removes a contact from memory
func (s *MemoryStorage) DeleteContact(pubKey []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(pubKey)
	delete(s.contacts, key)
	return nil
}

// AddMessage stores a message for a contact
func (s *MemoryStorage) AddMessage(contactPubKey []byte, envelope *pb.SignedEnvelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages[string(contactPubKey)] = append(s.messages[string(contactPubKey)], envelope)
	return nil
}

// GetMessages retrieves messages for a contact
// limit specifies maximum number of messages (0 = no limit)
// offset specifies number of messages to skip
func (s *MemoryStorage) GetMessages(contactPubKey []byte, limit, offset int) ([]*pb.SignedEnvelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allMessages, ok := s.messages[string(contactPubKey)]
	if !ok {
		return []*pb.SignedEnvelope{}, nil
	}

	// Apply offset
	if offset >= len(allMessages) {
		return []*pb.SignedEnvelope{}, nil
	}
	start := offset

	// Apply limit
	end := len(allMessages)
	if limit > 0 && start+limit < end {
		end = start + limit
	}

	result := make([]*pb.SignedEnvelope, end-start)
	copy(result, allMessages[start:end])
	return result, nil
}

// GetMessagesWithTimestamps retrieves messages with timestamps
// For in-memory storage, timestamps are generated from current time
func (s *MemoryStorage) GetMessagesWithTimestamps(contactPubKey []byte, limit, offset int) ([]*MessageWithKey, error) {
	envelopes, err := s.GetMessages(contactPubKey, limit, offset)
	if err != nil {
		return nil, err
	}

	// For in-memory storage, we don't have real timestamps
	// Return with zero timestamps (will be handled by caller)
	result := make([]*MessageWithKey, len(envelopes))
	for i, env := range envelopes {
		result[i] = &MessageWithKey{
			Envelope:  env,
			Timestamp: 0,
			Nonce:     nil,
		}
	}
	return result, nil
}

// DeleteMessages removes all messages for a contact
func (s *MemoryStorage) DeleteMessages(contactPubKey []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(contactPubKey)
	delete(s.messages, key)
	return nil
}

// Close is a no-op for in-memory storage
func (s *MemoryStorage) Close() error {
	return nil
}

// Clear removes all data from storage
func (s *MemoryStorage) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contacts = make(map[string]*pb.Contact)
	s.messages = make(map[string][]*pb.SignedEnvelope)
}

// ContactCount returns the number of contacts
func (s *MemoryStorage) ContactCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.contacts)
}

// MessageCount returns the number of messages for a contact
func (s *MemoryStorage) MessageCount(contactPubKey []byte) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages[string(contactPubKey)])
}

// Clone creates a deep copy of the storage (for testing)
func (s *MemoryStorage) Clone() *MemoryStorage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clone := &MemoryStorage{
		contacts:  make(map[string]*pb.Contact),
		messages:  make(map[string][]*pb.SignedEnvelope),
		peers:     make(map[string]*PeerRecord),
		configs:   make(map[string]string),
		blacklist: make(map[string]*BlacklistEntry),
	}

	for k, v := range s.contacts {
		clone.contacts[k] = proto.Clone(v).(*pb.Contact)
	}

	for k, msgs := range s.messages {
		clone.messages[k] = make([]*pb.SignedEnvelope, len(msgs))
		for i, msg := range msgs {
			clone.messages[k][i] = proto.Clone(msg).(*pb.SignedEnvelope)
		}
	}

	for k, v := range s.peers {
		peerCopy := *v
		clone.peers[k] = &peerCopy
	}

	for k, v := range s.configs {
		clone.configs[k] = v
	}

	for k, v := range s.blacklist {
		entryCopy := *v
		clone.blacklist[k] = &entryCopy
	}

	return clone
}

// AddPeer stores a peer record in memory
func (s *MemoryStorage) AddPeer(peer *PeerRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers[peer.PeerID] = peer
	return nil
}

// GetPeer retrieves a peer by peer ID
func (s *MemoryStorage) GetPeer(peerID string) (*PeerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	peer, ok := s.peers[peerID]
	if !ok {
		return nil, nil
	}
	return peer, nil
}

// ListPeers returns all peers, limited to the specified count (0 = no limit)
func (s *MemoryStorage) ListPeers(limit int) ([]*PeerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peers := make([]*PeerRecord, 0, len(s.peers))
	for _, peer := range s.peers {
		peers = append(peers, peer)
		if limit > 0 && len(peers) >= limit {
			break
		}
	}
	return peers, nil
}

// ListPeersBySource returns peers filtered by their source
func (s *MemoryStorage) ListPeersBySource(source PeerSource) ([]*PeerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var peers []*PeerRecord
	for _, peer := range s.peers {
		if peer.Source == source {
			peers = append(peers, peer)
		}
	}
	return peers, nil
}

// DeletePeer removes a peer from memory
func (s *MemoryStorage) DeletePeer(peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.peers, peerID)
	return nil
}

// PrunePeers removes stale peers (no-op for in-memory storage)
func (s *MemoryStorage) PrunePeers(maxAgeDays int, keepCount int) error {
	// No-op for testing
	return nil
}

// GetConfig retrieves a configuration value by key
func (s *MemoryStorage) GetConfig(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.configs[key]
	if !ok {
		return "", nil
	}
	return value, nil
}

// SetConfig stores a configuration value
func (s *MemoryStorage) SetConfig(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configs[key] = value
	return nil
}

// DeleteConfig removes a configuration value
func (s *MemoryStorage) DeleteConfig(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.configs, key)
	return nil
}

// BlacklistPeer adds a peer to the blacklist
func (s *MemoryStorage) BlacklistPeer(peerID string, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blacklist[peerID] = &BlacklistEntry{
		PeerID:        peerID,
		Reason:        reason,
		BlacklistedAt: time.Now(),
	}
	return nil
}

// IsBlacklisted checks if a peer is blacklisted
func (s *MemoryStorage) IsBlacklisted(peerID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.blacklist[peerID]
	if !ok {
		return false, nil
	}
	// Check if expired
	if entry.IsExpired() {
		return false, nil
	}
	return true, nil
}

// ListBlacklisted returns all blacklisted peers
func (s *MemoryStorage) ListBlacklisted() ([]*BlacklistEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]*BlacklistEntry, 0, len(s.blacklist))
	for _, entry := range s.blacklist {
		if !entry.IsExpired() {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// RemoveFromBlacklist removes a peer from the blacklist
func (s *MemoryStorage) RemoveFromBlacklist(peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.blacklist, peerID)
	return nil
}

// SaveGroup stores a group state
func (s *MemoryStorage) SaveGroup(group *pb.GroupState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(group.GroupId)
	s.groups[key] = group
	return nil
}

// GetGroup retrieves a group state
func (s *MemoryStorage) GetGroup(groupID []byte) (*pb.GroupState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	group, ok := s.groups[string(groupID)]
	if !ok {
		return nil, ErrGroupNotFound
	}
	return group, nil
}

// ListGroups returns all groups
func (s *MemoryStorage) ListGroups() ([]*pb.GroupState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	groups := make([]*pb.GroupState, 0, len(s.groups))
	for _, group := range s.groups {
		groups = append(groups, group)
	}
	return groups, nil
}

// DeleteGroup removes a group
func (s *MemoryStorage) DeleteGroup(groupID []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(groupID)
	delete(s.groups, key)
	return nil
}

// SaveSenderKey stores a sender key distribution
func (s *MemoryStorage) SaveSenderKey(sk *pb.SenderKeyDistribution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	groupKey := string(sk.GroupId)
	senderKey := string(sk.SenderPub)

	if _, ok := s.senderKeys[groupKey]; !ok {
		s.senderKeys[groupKey] = make(map[string]*pb.SenderKeyDistribution)
	}
	s.senderKeys[groupKey][senderKey] = sk
	return nil
}

// GetSenderKey retrieves a sender key
func (s *MemoryStorage) GetSenderKey(groupID, senderPubkey []byte) (*pb.SenderKeyDistribution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	skMap, ok := s.senderKeys[string(groupID)]
	if !ok {
		return nil, ErrSenderKeyNotFound
	}

	sk, ok := skMap[string(senderPubkey)]
	if !ok {
		return nil, ErrSenderKeyNotFound
	}
	return sk, nil
}

// ListSenderKeys returns all sender keys for a group
func (s *MemoryStorage) ListSenderKeys(groupID []byte) ([]*pb.SenderKeyDistribution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	skMap, ok := s.senderKeys[string(groupID)]
	if !ok {
		return nil, nil
	}

	keys := make([]*pb.SenderKeyDistribution, 0, len(skMap))
	for _, sk := range skMap {
		keys = append(keys, sk)
	}
	return keys, nil
}

// DeleteSenderKey removes a sender key
func (s *MemoryStorage) DeleteSenderKey(groupID, senderPubkey []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.senderKeys[string(groupID)], string(senderPubkey))
	return nil
}

// DeleteAllSenderKeys removes all sender keys for a group
func (s *MemoryStorage) DeleteAllSenderKeys(groupID []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	groupKey := string(groupID)
	delete(s.senderKeys, groupKey)
	return nil
}

// Channel storage methods

// SaveChannel stores a channel state
func (s *MemoryStorage) SaveChannel(channel *pb.ChannelState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels[string(channel.ChannelId)] = channel
	return nil
}

// GetChannel retrieves a channel state
func (s *MemoryStorage) GetChannel(channelID []byte) (*pb.ChannelState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	channel, ok := s.channels[string(channelID)]
	if !ok {
		return nil, ErrGroupNotFound
	}
	return channel, nil
}

// ListChannels returns all channels
func (s *MemoryStorage) ListChannels() ([]*pb.ChannelState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	channels := make([]*pb.ChannelState, 0, len(s.channels))
	for _, ch := range s.channels {
		channels = append(channels, ch)
	}
	return channels, nil
}

// DeleteChannel removes a channel
func (s *MemoryStorage) DeleteChannel(channelID []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.channels, string(channelID))
	return nil
}

// SaveChannelPost stores a channel post
func (s *MemoryStorage) SaveChannelPost(post *pb.ChannelPost) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	channelKey := string(post.ChannelId)
	postKey := string(post.PostId)

	if _, ok := s.channelPosts[channelKey]; !ok {
		s.channelPosts[channelKey] = make(map[string]*pb.ChannelPost)
	}
	s.channelPosts[channelKey][postKey] = post

	// Update channel's latest_post_cid
	if channel, ok := s.channels[channelKey]; ok {
		channel.LatestPostCid = post.PreviousPostCid
		channel.UpdatedAt = post.Timestamp
	}
	return nil
}

// GetChannelPosts retrieves posts from a channel
func (s *MemoryStorage) GetChannelPosts(channelID []byte, limit, offset int) ([]*pb.ChannelPost, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	postsMap, ok := s.channelPosts[string(channelID)]
	if !ok {
		return []*pb.ChannelPost{}, nil
	}

	posts := make([]*pb.ChannelPost, 0, len(postsMap))
	for _, post := range postsMap {
		posts = append(posts, post)
	}

	// Apply offset and limit
	if offset >= len(posts) {
		return []*pb.ChannelPost{}, nil
	}
	posts = posts[offset:]
	if limit > 0 && len(posts) > limit {
		posts = posts[:limit]
	}

	return posts, nil
}

// GetLatestChannelPostCID retrieves the latest post CID for a channel
func (s *MemoryStorage) GetLatestChannelPostCID(channelID []byte) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	channel, ok := s.channels[string(channelID)]
	if !ok {
		return nil, nil
	}
	return channel.LatestPostCid, nil
}
