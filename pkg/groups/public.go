package groups

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
	// ErrNotModerator is returned when user is not a moderator
	ErrNotModerator = errors.New("not a moderator")
	// ErrAlreadyBanned is returned when member is already banned
	ErrAlreadyBanned = errors.New("member already banned")
	// ErrAlreadyMuted is returned when member is already muted
	ErrAlreadyMuted = errors.New("member already muted")
	// ErrInvalidModerationAction is returned for invalid action types
	ErrInvalidModerationAction = errors.New("invalid moderation action")
)

// ModerationActionType represents the type of moderation action
type ModerationActionType string

const (
	ActionBan           ModerationActionType = "ban"
	ActionMute          ModerationActionType = "mute"
	ActionDeleteMessage ModerationActionType = "delete_message"
)

// BannedMember represents a banned member
type BannedMember struct {
	Pubkey    []byte
	BannedAt  uint64
	Reason    string
	Moderator []byte
}

// MutedMember represents a muted member
type MutedMember struct {
	Pubkey      []byte
	MutedAt     uint64
	DurationSec uint64
	ExpiresAt   uint64
	Reason      string
	Moderator   []byte
}

// PublicGroupService manages public groups with moderation
type PublicGroupService struct {
	storage storage.Storage
	// Identity keys for signing
	identitySignPub  ed25519.PublicKey
	identitySignPriv ed25519.PrivateKey
	// Cache of public groups
	groups map[string]*GroupState
	// Banned members: groupID -> pubkey -> BannedMember
	banned map[string]map[string]*BannedMember
	// Muted members: groupID -> pubkey -> MutedMember
	muted map[string]map[string]*MutedMember
	// Message rate limiting: groupID -> senderPubkey -> timestamps
	rateLimits map[string]map[string][]uint64
	mu         sync.RWMutex
}

// NewPublicGroupService creates a new public groups service
func NewPublicGroupService(
	storage storage.Storage,
	identitySignPub ed25519.PublicKey,
	identitySignPriv ed25519.PrivateKey,
) *PublicGroupService {
	return &PublicGroupService{
		storage:          storage,
		identitySignPub:  identitySignPub,
		identitySignPriv: identitySignPriv,
		groups:           make(map[string]*GroupState),
		banned:           make(map[string]map[string]*BannedMember),
		muted:            make(map[string]map[string]*MutedMember),
		rateLimits:       make(map[string]map[string][]uint64),
	}
}

// CreatePublicGroup creates a new public group
func (s *PublicGroupService) CreatePublicGroup(name, description string) (*GroupState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get creator's X25519 pubkey from storage or identity
	x25519Pubkey := make([]byte, 32) // Placeholder - should come from identity

	state, err := NewGroupState(name, description, PublicGroup, s.identitySignPub, x25519Pubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to create group state: %w", err)
	}

	// Sign the initial state
	if err := state.Sign(s.identitySignPriv); err != nil {
		return nil, fmt.Errorf("failed to sign group state: %w", err)
	}

	// Store the group
	if err := s.storeGroup(state); err != nil {
		return nil, fmt.Errorf("failed to store group: %w", err)
	}

	s.groups[hex.EncodeToString(state.GroupID)] = state

	// Initialize moderation maps
	groupKey := hex.EncodeToString(state.GroupID)
	s.banned[groupKey] = make(map[string]*BannedMember)
	s.muted[groupKey] = make(map[string]*MutedMember)
	s.rateLimits[groupKey] = make(map[string][]uint64)

	return state, nil
}

// BanMember bans a member from the public group
func (s *PublicGroupService) BanMember(groupID []byte, memberPubkey []byte, reason string) (*ModerationAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupKey := hex.EncodeToString(groupID)
	state, exists := s.groups[groupKey]
	if !exists {
		return nil, ErrGroupNotFound
	}

	// Verify caller is admin or owner
	if !s.isMemberWithRole(state, s.identitySignPub, []GroupRole{Admin, Owner}) {
		return nil, ErrNotAuthorized
	}

	// Check if already banned
	if _, exists := s.banned[groupKey][hex.EncodeToString(memberPubkey)]; exists {
		return nil, ErrAlreadyBanned
	}

	// Create moderation action
	action := &ModerationAction{
		TargetMemberPubkey: memberPubkey,
		ActionType:         string(ActionBan),
		Reason:             reason,
		DurationSeconds:    0, // Permanent
		ModeratorPubkey:    s.identitySignPub,
		Timestamp:          uint64(time.Now().Unix()),
	}

	// Sign the action
	if err := action.Sign(s.identitySignPriv); err != nil {
		return nil, fmt.Errorf("failed to sign moderation action: %w", err)
	}

	// Add to banned list
	s.banned[groupKey][hex.EncodeToString(memberPubkey)] = &BannedMember{
		Pubkey:    memberPubkey,
		BannedAt:  action.Timestamp,
		Reason:    reason,
		Moderator: s.identitySignPub,
	}

	// Remove from group members
	if err := state.RemoveMember(memberPubkey); err != nil {
		return nil, fmt.Errorf("failed to remove member: %w", err)
	}

	// Update group state
	state.Epoch++
	state.UpdatedAt = uint64(time.Now().Unix())
	if err := state.Sign(s.identitySignPriv); err != nil {
		return nil, fmt.Errorf("failed to sign updated state: %w", err)
	}

	if err := s.storeGroup(state); err != nil {
		return nil, fmt.Errorf("failed to store updated group: %w", err)
	}

	return action, nil
}

// MuteMember mutes a member for a duration
func (s *PublicGroupService) MuteMember(groupID []byte, memberPubkey []byte, reason string, durationSec uint64) (*ModerationAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupKey := hex.EncodeToString(groupID)
	state, exists := s.groups[groupKey]
	if !exists {
		return nil, ErrGroupNotFound
	}

	// Verify caller is admin or owner
	if !s.isMemberWithRole(state, s.identitySignPub, []GroupRole{Admin, Owner}) {
		return nil, ErrNotAuthorized
	}

	now := uint64(time.Now().Unix())
	expiresAt := now + durationSec

	// Check if already muted and not expired
	if muted, exists := s.muted[groupKey][hex.EncodeToString(memberPubkey)]; exists {
		if muted.ExpiresAt > now {
			return nil, ErrAlreadyMuted
		}
	}

	// Create moderation action
	action := &ModerationAction{
		TargetMemberPubkey: memberPubkey,
		ActionType:         string(ActionMute),
		Reason:             reason,
		DurationSeconds:    durationSec,
		ModeratorPubkey:    s.identitySignPub,
		Timestamp:          now,
	}

	// Sign the action
	if err := action.Sign(s.identitySignPriv); err != nil {
		return nil, fmt.Errorf("failed to sign moderation action: %w", err)
	}

	// Add to muted list
	s.muted[groupKey][hex.EncodeToString(memberPubkey)] = &MutedMember{
		Pubkey:      memberPubkey,
		MutedAt:     now,
		DurationSec: durationSec,
		ExpiresAt:   expiresAt,
		Reason:      reason,
		Moderator:   s.identitySignPub,
	}

	return action, nil
}

// DeleteMessage creates a moderation action to delete a message
func (s *PublicGroupService) DeleteMessage(groupID []byte, messageID []byte, reason string) (*ModerationAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupKey := hex.EncodeToString(groupID)
	state, exists := s.groups[groupKey]
	if !exists {
		return nil, ErrGroupNotFound
	}

	// Verify caller is admin or owner
	if !s.isMemberWithRole(state, s.identitySignPub, []GroupRole{Admin, Owner}) {
		return nil, ErrNotAuthorized
	}

	// Create moderation action
	action := &ModerationAction{
		TargetMemberPubkey: nil, // Not targeting a specific member
		ActionType:         string(ActionDeleteMessage),
		Reason:             reason,
		TargetMessageID:    messageID,
		ModeratorPubkey:    s.identitySignPub,
		Timestamp:          uint64(time.Now().Unix()),
	}

	// Sign the action
	if err := action.Sign(s.identitySignPriv); err != nil {
		return nil, fmt.Errorf("failed to sign moderation action: %w", err)
	}

	return action, nil
}

// IsBanned checks if a member is banned
func (s *PublicGroupService) IsBanned(groupID []byte, memberPubkey []byte) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupKey := hex.EncodeToString(groupID)
	_, exists := s.banned[groupKey][hex.EncodeToString(memberPubkey)]
	return exists
}

// IsMuted checks if a member is muted
func (s *PublicGroupService) IsMuted(groupID []byte, memberPubkey []byte) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupKey := hex.EncodeToString(groupID)
	muted, exists := s.muted[groupKey][hex.EncodeToString(memberPubkey)]
	if !exists {
		return false
	}

	// Check if mute has expired
	now := uint64(time.Now().Unix())
	if muted.ExpiresAt <= now {
		delete(s.muted[groupKey], hex.EncodeToString(memberPubkey))
		return false
	}

	return true
}

// CheckRateLimit checks if a sender has exceeded rate limit
// Returns true if message should be allowed
func (s *PublicGroupService) CheckRateLimit(groupID []byte, senderPubkey []byte, maxMessages uint64, windowSec uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupKey := hex.EncodeToString(groupID)
	senderKey := hex.EncodeToString(senderPubkey)
	now := uint64(time.Now().Unix())

	timestamps, exists := s.rateLimits[groupKey][senderKey]
	if !exists {
		s.rateLimits[groupKey][senderKey] = []uint64{now}
		return true
	}

	// Filter timestamps within window
	cutoff := now - windowSec
	validTimestamps := make([]uint64, 0)
	for _, ts := range timestamps {
		if ts > cutoff {
			validTimestamps = append(validTimestamps, ts)
		}
	}

	if uint64(len(validTimestamps)) >= maxMessages {
		s.rateLimits[groupKey][senderKey] = validTimestamps
		return false
	}

	validTimestamps = append(validTimestamps, now)
	s.rateLimits[groupKey][senderKey] = validTimestamps
	return true
}

// GetModerationActions returns recent moderation actions for a group
func (s *PublicGroupService) GetModerationActions(groupID []byte, limit int) ([]*ModerationAction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupKey := hex.EncodeToString(groupID)
	actions := make([]*ModerationAction, 0)

	// Collect ban actions
	for _, banned := range s.banned[groupKey] {
		action := &ModerationAction{
			TargetMemberPubkey: banned.Pubkey,
			ActionType:         string(ActionBan),
			Reason:             banned.Reason,
			ModeratorPubkey:    banned.Moderator,
			Timestamp:          banned.BannedAt,
		}
		actions = append(actions, action)
	}

	// Collect mute actions
	for _, muted := range s.muted[groupKey] {
		action := &ModerationAction{
			TargetMemberPubkey: muted.Pubkey,
			ActionType:         string(ActionMute),
			Reason:             muted.Reason,
			DurationSeconds:    muted.DurationSec,
			ModeratorPubkey:    muted.Moderator,
			Timestamp:          muted.MutedAt,
		}
		actions = append(actions, action)
	}

	// Sort by timestamp (descending) and limit
	if len(actions) > limit {
		actions = actions[:limit]
	}

	return actions, nil
}

// Helper functions

func (s *PublicGroupService) storeGroup(state *GroupState) error {
	protoState := state.ToProto()
	return s.storage.SaveGroup(protoState)
}

func (s *PublicGroupService) isMemberWithRole(state *GroupState, pubkey []byte, roles []GroupRole) bool {
	for _, member := range state.Members {
		if string(member.Ed25519Pubkey) == string(pubkey) {
			for _, role := range roles {
				if member.Role == role {
					return true
				}
			}
			return false
		}
	}
	return false
}

// ModerationAction implementation

// ModerationAction represents a moderation action
type ModerationAction struct {
	TargetMemberPubkey []byte
	ActionType         string
	Reason             string
	DurationSeconds    uint64
	TargetMessageID    []byte
	ModeratorPubkey    []byte
	Timestamp          uint64
	Signature          []byte
}

// ToProto converts to protobuf
func (ma *ModerationAction) ToProto() *pb.ModerationAction {
	return &pb.ModerationAction{
		TargetMemberPubkey: ma.TargetMemberPubkey,
		ActionType:         ma.ActionType,
		Reason:             ma.Reason,
		DurationSeconds:    ma.DurationSeconds,
		TargetMessageId:    ma.TargetMessageID,
		ModeratorPubkey:    ma.ModeratorPubkey,
		Timestamp:          ma.Timestamp,
		Signature:          ma.Signature,
	}
}

// FromProto converts from protobuf
func ModerationActionFromProto(protoMA *pb.ModerationAction) *ModerationAction {
	return &ModerationAction{
		TargetMemberPubkey: protoMA.TargetMemberPubkey,
		ActionType:         protoMA.ActionType,
		Reason:             protoMA.Reason,
		DurationSeconds:    protoMA.DurationSeconds,
		TargetMessageID:    protoMA.TargetMessageId,
		ModeratorPubkey:    protoMA.ModeratorPubkey,
		Timestamp:          protoMA.Timestamp,
		Signature:          protoMA.Signature,
	}
}

// Sign signs the moderation action
func (ma *ModerationAction) Sign(privKey ed25519.PrivateKey) error {
	data, err := ma.Serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}

	ma.Signature = ed25519.Sign(privKey, data)
	return nil
}

// Verify verifies the moderation action signature
func (ma *ModerationAction) Verify(pubKey ed25519.PublicKey) bool {
	data, err := ma.Serialize()
	if err != nil {
		return false
	}

	return ed25519.Verify(pubKey, data, ma.Signature)
}

// Serialize serializes the moderation action for signing
func (ma *ModerationAction) Serialize() ([]byte, error) {
	protoMA := ma.ToProto()
	// Clear signature for signing
	protoMA.Signature = nil
	return proto.Marshal(protoMA)
}

// ComputeProofOfWork computes a simple HashCash-style proof of work
func ComputeProofOfWork(data []byte, difficulty uint8) ([]byte, error) {
	var nonce uint64
	var hash []byte

	for {
		nonce++
		h := sha256.Sum256(append(data, byte(nonce&0xFF), byte((nonce>>8)&0xFF), byte((nonce>>16)&0xFF), byte((nonce>>24)&0xFF), byte((nonce>>32)&0xFF), byte((nonce>>40)&0xFF), byte((nonce>>48)&0xFF), byte((nonce>>56)&0xFF)))
		hash = h[:]

		// Check if first byte is below difficulty threshold
		if hash[0] < difficulty {
			break
		}

		// Prevent infinite loop
		if nonce > 1000000 {
			return nil, errors.New("proof of work failed after max attempts")
		}
	}

	// Return nonce as 8-byte little-endian
	result := make([]byte, 8)
	for i := uint(0); i < 8; i++ {
		result[i] = byte(nonce >> (i * 8))
	}
	return result, nil
}

// VerifyProofOfWork verifies proof of work
func VerifyProofOfWork(data []byte, nonce []byte, difficulty uint8) bool {
	if len(nonce) != 8 {
		return false
	}

	hashInput := append(data, nonce...)
	hash := sha256.Sum256(hashInput)

	return hash[0] < difficulty
}

// GenerateRandomID generates a random identifier
func GenerateRandomID(size int) ([]byte, error) {
	id := make([]byte, size)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	return id, nil
}
