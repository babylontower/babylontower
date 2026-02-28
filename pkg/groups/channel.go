package groups

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"

	"golang.org/x/crypto/ed25519"
	"google.golang.org/protobuf/proto"
)

var (
	// ErrNotChannelOwner is returned when user is not the channel owner
	ErrNotChannelOwner = errors.New("not channel owner")
	// ErrChannelNotFound is returned when channel is not found
	ErrChannelNotFound = errors.New("channel not found")
	// ErrInvalidChannelType is returned for invalid channel type
	ErrInvalidChannelType = errors.New("invalid channel type")
)

// ChannelService manages channels (private and public)
type ChannelService struct {
	storage storage.Storage
	// Identity keys for signing
	identitySignPub  ed25519.PublicKey
	identitySignPriv ed25519.PrivateKey
	// Cache of channels
	channels map[string]*ChannelState
	// Channel posts cache: channelID -> postID -> ChannelPost
	posts map[string]map[string]*ChannelPost
	// Subscribers for public channels: channelID -> subscriberPubkey -> subscribedAt
	subscribers map[string]map[string]uint64
	mu          sync.RWMutex
}

// ChannelState represents the state of a channel
type ChannelState struct {
	// ChannelID is the random 32-byte identifier
	ChannelID []byte
	// Name is the channel name (max 128 UTF-8 chars)
	Name string
	// Description is the channel description (max 512 UTF-8 chars)
	Description string
	// AvatarCID is the IPFS CID of the channel avatar
	AvatarCID string
	// Type is the channel type (PRIVATE_CHANNEL or PUBLIC_CHANNEL)
	Type GroupType
	// OwnerPubkey is the Ed25519 public key of the channel owner
	OwnerPubkey []byte
	// CreatedAt is the channel creation timestamp
	CreatedAt uint64
	// UpdatedAt is the last update timestamp
	UpdatedAt uint64
	// LatestPostCID is the CID of the latest post (for linked-list)
	LatestPostCID []byte
	// SubscriberCount is the approximate subscriber count
	SubscriberCount uint64
	// StateSignature is the signature of this state
	StateSignature []byte
}

// ChannelPost represents a post in a channel
type ChannelPost struct {
	// PostID is the random 16-byte identifier
	PostID []byte
	// ChannelID is the channel this post belongs to
	ChannelID []byte
	// AuthorPubkey is the Ed25519 pub of the author
	AuthorPubkey []byte
	// Timestamp is the Unix timestamp
	Timestamp uint64
	// Content is the message content
	Content interface{}
	// PreviousPostCID is the CID of previous post (for linked-list)
	PreviousPostCID []byte
	// Signature is the Ed25519 signature of author
	Signature []byte
}

// NewChannelService creates a new channel service
func NewChannelService(
	storage storage.Storage,
	identitySignPub ed25519.PublicKey,
	identitySignPriv ed25519.PrivateKey,
) *ChannelService {
	return &ChannelService{
		storage:          storage,
		identitySignPub:  identitySignPub,
		identitySignPriv: identitySignPriv,
		channels:         make(map[string]*ChannelState),
		posts:            make(map[string]map[string]*ChannelPost),
		subscribers:      make(map[string]map[string]uint64),
	}
}

// CreateChannel creates a new channel
func (s *ChannelService) CreateChannel(name, description string, channelType GroupType) (*ChannelState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if channelType != PrivateChannel && channelType != PublicChannel {
		return nil, ErrInvalidChannelType
	}

	// Generate random channel ID
	channelID := make([]byte, 32)
	if _, err := GenerateRandomID(32); err == nil {
		copy(channelID, make([]byte, 32))
		// Use actual random bytes
		if randID, err := GenerateRandomID(32); err == nil {
			channelID = randID
		}
	}

	now := uint64(time.Now().Unix())

	state := &ChannelState{
		ChannelID:       channelID,
		Name:            name,
		Description:     description,
		AvatarCID:       "",
		Type:            channelType,
		OwnerPubkey:     s.identitySignPub,
		CreatedAt:       now,
		UpdatedAt:       now,
		LatestPostCID:   nil,
		SubscriberCount: 0,
	}

	// Sign the state
	if err := state.Sign(s.identitySignPriv); err != nil {
		return nil, fmt.Errorf("failed to sign channel state: %w", err)
	}

	// Store the channel
	if err := s.storeChannel(state); err != nil {
		return nil, fmt.Errorf("failed to store channel: %w", err)
	}

	channelKey := hex.EncodeToString(channelID)
	s.channels[channelKey] = state
	s.posts[channelKey] = make(map[string]*ChannelPost)

	// For public channels, initialize subscriber map
	if channelType == PublicChannel {
		s.subscribers[channelKey] = make(map[string]uint64)
	}

	return state, nil
}

// CreatePost creates a new post in a channel
func (s *ChannelService) CreatePost(channelID []byte, content interface{}) (*ChannelPost, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	channelKey := hex.EncodeToString(channelID)
	state, exists := s.channels[channelKey]
	if !exists {
		return nil, ErrChannelNotFound
	}

	// For private channels, verify owner
	if state.Type == PrivateChannel && string(state.OwnerPubkey) != string(s.identitySignPub) {
		return nil, ErrNotChannelOwner
	}

	// Generate random post ID
	postID := make([]byte, 16)
	if randID, err := GenerateRandomID(16); err == nil {
		postID = randID
	}

	now := uint64(time.Now().Unix())

	post := &ChannelPost{
		PostID:          postID,
		ChannelID:       channelID,
		AuthorPubkey:    s.identitySignPub,
		Timestamp:       now,
		Content:         content,
		PreviousPostCID: state.LatestPostCID,
	}

	// Sign the post
	if err := post.Sign(s.identitySignPriv); err != nil {
		return nil, fmt.Errorf("failed to sign channel post: %w", err)
	}

	// Store the post
	if err := s.storePost(post); err != nil {
		return nil, fmt.Errorf("failed to store channel post: %w", err)
	}

	// Update channel's latest post CID
	state.LatestPostCID = postID
	state.UpdatedAt = now
	if err := state.Sign(s.identitySignPriv); err != nil {
		return nil, fmt.Errorf("failed to sign updated channel: %w", err)
	}

	if err := s.storeChannel(state); err != nil {
		return nil, fmt.Errorf("failed to update channel: %w", err)
	}

	// Cache the post
	if _, exists := s.posts[channelKey]; !exists {
		s.posts[channelKey] = make(map[string]*ChannelPost)
	}
	s.posts[channelKey][hex.EncodeToString(postID)] = post

	// For public channels, increment subscriber count
	if state.Type == PublicChannel {
		state.SubscriberCount++
	}

	return post, nil
}

// GetPosts retrieves posts from a channel
func (s *ChannelService) GetPosts(channelID []byte, limit, offset int) ([]*ChannelPost, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channelKey := hex.EncodeToString(channelID)
	postsMap, exists := s.posts[channelKey]
	if !exists {
		return []*ChannelPost{}, nil
	}

	posts := make([]*ChannelPost, 0, len(postsMap))
	for _, post := range postsMap {
		posts = append(posts, post)
	}

	// Sort by timestamp (descending - newest first)
	for i := 0; i < len(posts)-1; i++ {
		for j := i + 1; j < len(posts); j++ {
			if posts[i].Timestamp < posts[j].Timestamp {
				posts[i], posts[j] = posts[j], posts[i]
			}
		}
	}

	// Apply offset and limit
	if offset >= len(posts) {
		return []*ChannelPost{}, nil
	}
	posts = posts[offset:]
	if limit > 0 && len(posts) > limit {
		posts = posts[:limit]
	}

	return posts, nil
}

// Subscribe subscribes to a public channel
func (s *ChannelService) Subscribe(channelID []byte, subscriberPubkey []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	channelKey := hex.EncodeToString(channelID)
	state, exists := s.channels[channelKey]
	if !exists {
		return ErrChannelNotFound
	}

	if state.Type != PublicChannel {
		return ErrInvalidChannelType
	}

	if _, exists := s.subscribers[channelKey]; !exists {
		s.subscribers[channelKey] = make(map[string]uint64)
	}

	s.subscribers[channelKey][hex.EncodeToString(subscriberPubkey)] = uint64(time.Now().Unix())
	state.SubscriberCount = uint64(len(s.subscribers[channelKey]))

	return s.storeChannel(state)
}

// Unsubscribe unsubscribes from a public channel
func (s *ChannelService) Unsubscribe(channelID []byte, subscriberPubkey []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	channelKey := hex.EncodeToString(channelID)
	state, exists := s.channels[channelKey]
	if !exists {
		return ErrChannelNotFound
	}

	delete(s.subscribers[channelKey], hex.EncodeToString(subscriberPubkey))
	state.SubscriberCount = uint64(len(s.subscribers[channelKey]))

	return s.storeChannel(state)
}

// IsSubscribed checks if a pubkey is subscribed to a channel
func (s *ChannelService) IsSubscribed(channelID []byte, subscriberPubkey []byte) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channelKey := hex.EncodeToString(channelID)
	_, exists := s.subscribers[channelKey][hex.EncodeToString(subscriberPubkey)]
	return exists
}

// GetChannel returns a channel by ID
func (s *ChannelService) GetChannel(channelID []byte) (*ChannelState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channelKey := hex.EncodeToString(channelID)
	state, exists := s.channels[channelKey]
	if !exists {
		return nil, ErrChannelNotFound
	}

	return state, nil
}

// ListChannels returns all channels
func (s *ChannelService) ListChannels() []*ChannelState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channels := make([]*ChannelState, 0, len(s.channels))
	for _, ch := range s.channels {
		channels = append(channels, ch)
	}

	return channels
}

// DeleteChannel deletes a channel
func (s *ChannelService) DeleteChannel(channelID []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	channelKey := hex.EncodeToString(channelID)
	_, exists := s.channels[channelKey]
	if !exists {
		return ErrChannelNotFound
	}

	// Verify owner
	if string(s.channels[channelKey].OwnerPubkey) != string(s.identitySignPub) {
		return ErrNotChannelOwner
	}

	if err := s.storage.DeleteChannel(channelID); err != nil {
		return fmt.Errorf("failed to delete channel: %w", err)
	}

	delete(s.channels, channelKey)
	delete(s.posts, channelKey)
	delete(s.subscribers, channelKey)

	return nil
}

// Helper functions

func (s *ChannelService) storeChannel(state *ChannelState) error {
	protoState := state.ToProto()
	return s.storage.SaveChannel(protoState)
}

func (s *ChannelService) storePost(post *ChannelPost) error {
	protoPost, err := post.ToProto()
	if err != nil {
		return fmt.Errorf("failed to convert post to proto: %w", err)
	}
	return s.storage.SaveChannelPost(protoPost)
}

// ChannelState methods

// ToProto converts ChannelState to protobuf
func (cs *ChannelState) ToProto() *pb.ChannelState {
	return &pb.ChannelState{
		ChannelId:       cs.ChannelID,
		Name:            cs.Name,
		Description:     cs.Description,
		AvatarCid:       cs.AvatarCID,
		Type:            pb.GroupType(cs.Type),
		OwnerPubkey:     cs.OwnerPubkey,
		CreatedAt:       cs.CreatedAt,
		UpdatedAt:       cs.UpdatedAt,
		LatestPostCid:   cs.LatestPostCID,
		SubscriberCount: cs.SubscriberCount,
		StateSignature:  cs.StateSignature,
	}
}

// FromProto converts protobuf ChannelState
func ChannelStateFromProto(protoCS *pb.ChannelState) *ChannelState {
	return &ChannelState{
		ChannelID:       protoCS.ChannelId,
		Name:            protoCS.Name,
		Description:     protoCS.Description,
		AvatarCID:       protoCS.AvatarCid,
		Type:            GroupType(protoCS.Type),
		OwnerPubkey:     protoCS.OwnerPubkey,
		CreatedAt:       protoCS.CreatedAt,
		UpdatedAt:       protoCS.UpdatedAt,
		LatestPostCID:   protoCS.LatestPostCid,
		SubscriberCount: protoCS.SubscriberCount,
		StateSignature:  protoCS.StateSignature,
	}
}

// Sign signs the channel state
func (cs *ChannelState) Sign(privKey ed25519.PrivateKey) error {
	data, err := cs.Serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}

	cs.StateSignature = ed25519.Sign(privKey, data)
	return nil
}

// Verify verifies the channel state signature
func (cs *ChannelState) Verify(pubKey ed25519.PublicKey) bool {
	if len(cs.StateSignature) == 0 {
		return false
	}

	data, err := cs.Serialize()
	if err != nil {
		return false
	}

	return ed25519.Verify(pubKey, data, cs.StateSignature)
}

// Serialize serializes the channel state for signing
func (cs *ChannelState) Serialize() ([]byte, error) {
	protoCS := cs.ToProto()
	protoCS.StateSignature = nil
	return proto.Marshal(protoCS)
}

// ChannelPost methods

// ToProto converts ChannelPost to protobuf
func (cp *ChannelPost) ToProto() (*pb.ChannelPost, error) {
	protoPost := &pb.ChannelPost{
		PostId:          cp.PostID,
		ChannelId:       cp.ChannelID,
		AuthorPubkey:    cp.AuthorPubkey,
		Timestamp:       cp.Timestamp,
		PreviousPostCid: cp.PreviousPostCID,
		Signature:       cp.Signature,
	}

	// Convert content to appropriate type
	switch content := cp.Content.(type) {
	case *pb.TextMessage:
		protoPost.Content = &pb.ChannelPost_Text{Text: content}
	case *pb.MediaMessage:
		protoPost.Content = &pb.ChannelPost_Media{Media: content}
	case *pb.EditMessage:
		protoPost.Content = &pb.ChannelPost_Edit{Edit: content}
	case *pb.DeleteMessage:
		protoPost.Content = &pb.ChannelPost_DeleteMsg{DeleteMsg: content}
	case []byte:
		// Raw bytes for encrypted content
		protoPost.Content = &pb.ChannelPost_Text{
			Text: &pb.TextMessage{
				Text: string(content),
			},
		}
	default:
		// Try to marshal as JSON
		jsonData, err := json.Marshal(content)
		if err != nil {
			return nil, fmt.Errorf("unsupported content type: %T", content)
		}
		protoPost.Content = &pb.ChannelPost_Text{
			Text: &pb.TextMessage{
				Text: string(jsonData),
			},
		}
	}

	return protoPost, nil
}

// FromProto converts protobuf ChannelPost
func ChannelPostFromProto(protoCP *pb.ChannelPost) (*ChannelPost, error) {
	var content interface{}

	switch c := protoCP.Content.(type) {
	case *pb.ChannelPost_Text:
		content = c.Text
	case *pb.ChannelPost_Media:
		content = c.Media
	case *pb.ChannelPost_Edit:
		content = c.Edit
	case *pb.ChannelPost_DeleteMsg:
		content = c.DeleteMsg
	default:
		content = []byte{}
	}

	return &ChannelPost{
		PostID:          protoCP.PostId,
		ChannelID:       protoCP.ChannelId,
		AuthorPubkey:    protoCP.AuthorPubkey,
		Timestamp:       protoCP.Timestamp,
		Content:         content,
		PreviousPostCID: protoCP.PreviousPostCid,
		Signature:       protoCP.Signature,
	}, nil
}

// Sign signs the channel post
func (cp *ChannelPost) Sign(privKey ed25519.PrivateKey) error {
	data, err := cp.Serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}

	cp.Signature = ed25519.Sign(privKey, data)
	return nil
}

// Verify verifies the channel post signature
func (cp *ChannelPost) Verify(pubKey ed25519.PublicKey) bool {
	if len(cp.Signature) == 0 {
		return false
	}

	data, err := cp.Serialize()
	if err != nil {
		return false
	}

	return ed25519.Verify(pubKey, data, cp.Signature)
}

// Serialize serializes the channel post for signing
func (cp *ChannelPost) Serialize() ([]byte, error) {
	protoCP, err := cp.ToProto()
	if err != nil {
		return nil, err
	}
	protoCP.Signature = nil
	return proto.Marshal(protoCP)
}

// ComputeChannelID computes a channel ID from a name (for discovery)
func ComputeChannelID(name string) []byte {
	hash := sha256.Sum256([]byte(name))
	return hash[:16]
}

// GetChannelTopic derives the PubSub topic for a channel
func GetChannelTopic(channelID []byte) string {
	hash := sha256.Sum256(channelID)
	return "babylon-ch-" + hex.EncodeToString(hash[:8])
}
