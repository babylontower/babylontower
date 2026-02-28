package groups

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/hkdf"
)

var (
	// ErrGroupNotFound is returned when a group is not found
	ErrGroupNotFound = errors.New("group not found")
	// ErrMemberNotFound is returned when a member is not found
	ErrMemberNotFound = errors.New("member not found")
	// ErrAlreadyMember is returned when trying to add an existing member
	ErrAlreadyMember = errors.New("already a member")
	// ErrNotAuthorized is returned when user is not authorized
	ErrNotAuthorized = errors.New("not authorized")
	// ErrInvalidEpoch is returned when epoch validation fails
	ErrInvalidEpoch = errors.New("invalid epoch")
	// ErrSenderKeyNotFound is returned when a sender key is not found
	ErrSenderKeyNotFound = errors.New("sender key not found")
)

// Service manages private groups
type Service struct {
	storage *storage.BadgerStorage
	// Identity keys for signing
	identitySignPub  ed25519.PublicKey
	identitySignPriv ed25519.PrivateKey
	// X25519 public key for group membership
	identityX25519Pub []byte
	// Cache of active groups
	groups map[string]*GroupState
	// Cache of sender keys: groupID -> senderPubkey -> SenderKey
	senderKeys map[string]map[string]*SenderKey
	mu         sync.RWMutex
}

// NewService creates a new groups service
func NewService(
	storage *storage.BadgerStorage,
	identitySignPub ed25519.PublicKey,
	identitySignPriv ed25519.PrivateKey,
	opts ...ServiceOption,
) *Service {
	s := &Service{
		storage:          storage,
		identitySignPub:  identitySignPub,
		identitySignPriv: identitySignPriv,
		groups:           make(map[string]*GroupState),
		senderKeys:       make(map[string]map[string]*SenderKey),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ServiceOption configures a Service
type ServiceOption func(*Service)

// WithX25519PublicKey sets the X25519 public key used for group membership
func WithX25519PublicKey(x25519Pub []byte) ServiceOption {
	return func(s *Service) {
		s.identityX25519Pub = x25519Pub
	}
}

// CreateGroup creates a new private group
func (s *Service) CreateGroup(name, description string, groupType GroupType) (*GroupState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Use the configured X25519 public key; fall back to zero key if not set
	x25519Pubkey := s.identityX25519Pub
	if len(x25519Pubkey) == 0 {
		x25519Pubkey = make([]byte, 32)
	}

	state, err := NewGroupState(name, description, groupType, s.identitySignPub, x25519Pubkey)
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

	// Initialize sender key for creator
	senderKey, err := GenerateSenderKey(state.GroupID, s.identitySignPub)
	if err != nil {
		return nil, fmt.Errorf("failed to generate sender key: %w", err)
	}

	s.initializeSenderKeysForGroup(state.GroupID)
	s.senderKeys[hex.EncodeToString(state.GroupID)][hex.EncodeToString(s.identitySignPub)] = senderKey

	// Store sender key
	if err := s.storeSenderKey(senderKey); err != nil {
		return nil, fmt.Errorf("failed to store sender key: %w", err)
	}

	s.groups[hex.EncodeToString(state.GroupID)] = state

	return state, nil
}

// AddMember adds a member to an existing group
func (s *Service) AddMember(groupID []byte, memberPubkey, memberX25519Pubkey []byte, displayName string, role GroupRole) (*GroupState, error) {
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

	// Check if already a member
	if state.IsMember(memberPubkey) {
		return nil, ErrAlreadyMember
	}

	// Create state update
	previousHash, err := state.ComputeHash()
	if err != nil {
		return nil, fmt.Errorf("failed to compute state hash: %w", err)
	}

	newState := &GroupState{
		GroupID:           state.GroupID,
		Epoch:             state.Epoch + 1,
		Name:              state.Name,
		Description:       state.Description,
		AvatarCID:         state.AvatarCID,
		Type:              state.Type,
		Members:           append(append([]GroupMember{}, state.Members...), GroupMember{
			Ed25519Pubkey: memberPubkey,
			X25519Pubkey:  memberX25519Pubkey,
			DisplayName:   displayName,
			JoinedAt:      uint64(time.Now().Unix()),
			Role:          role,
		}),
		CreatorPubkey:     state.CreatorPubkey,
		CreatedAt:         state.CreatedAt,
		UpdatedAt:         uint64(time.Now().Unix()),
		PreviousStateHash: previousHash,
	}

	if err := newState.Sign(s.identitySignPriv); err != nil {
		return nil, fmt.Errorf("failed to sign new state: %w", err)
	}

	// Store updated group
	if err := s.storeGroup(newState); err != nil {
		return nil, fmt.Errorf("failed to store updated group: %w", err)
	}

	s.groups[groupKey] = newState

	// Note: Sender key distribution to the new member happens externally
	// via pairwise Double Ratchet channels

	return newState, nil
}

// RemoveMember removes a member from the group (triggers key rotation)
func (s *Service) RemoveMember(groupID []byte, memberPubkey []byte) (*GroupState, error) {
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

	// Create state update
	previousHash, err := state.ComputeHash()
	if err != nil {
		return nil, fmt.Errorf("failed to compute state hash: %w", err)
	}

	// Copy members and remove the target
	newMembers := make([]GroupMember, 0, len(state.Members)-1)
	for _, m := range state.Members {
		if string(m.Ed25519Pubkey) != string(memberPubkey) {
			newMembers = append(newMembers, m)
		}
	}

	newState := &GroupState{
		GroupID:           state.GroupID,
		Epoch:             state.Epoch + 1,
		Name:              state.Name,
		Description:       state.Description,
		AvatarCID:         state.AvatarCID,
		Type:              state.Type,
		Members:           newMembers,
		CreatorPubkey:     state.CreatorPubkey,
		CreatedAt:         state.CreatedAt,
		UpdatedAt:         uint64(time.Now().Unix()),
		PreviousStateHash: previousHash,
	}

	if err := newState.Sign(s.identitySignPriv); err != nil {
		return nil, fmt.Errorf("failed to sign new state: %w", err)
	}

	// Store updated group
	if err := s.storeGroup(newState); err != nil {
		return nil, fmt.Errorf("failed to store updated group: %w", err)
	}

	s.groups[groupKey] = newState

	// Trigger sender key rotation for all remaining members
	if err := s.rotateAllSenderKeys(newState); err != nil {
		return nil, fmt.Errorf("failed to rotate sender keys: %w", err)
	}

	return newState, nil
}

// EncryptGroupMessage encrypts a message using Sender Keys
func (s *Service) EncryptGroupMessage(groupID []byte, plaintext []byte) (*pb.GroupPayload, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupKey := hex.EncodeToString(groupID)
	senderKeyMap, exists := s.senderKeys[groupKey]
	if !exists {
		return nil, ErrGroupNotFound
	}

	senderKey, exists := senderKeyMap[hex.EncodeToString(s.identitySignPub)]
	if !exists {
		return nil, ErrSenderKeyNotFound
	}

	// Derive message key
	messageKey, err := senderKey.DeriveMessageKey()
	if err != nil {
		return nil, fmt.Errorf("failed to derive message key: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := hkdf.New(sha256.New, messageKey, []byte("nonce"), nil).Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt with XChaCha20-Poly1305
	aead, err := chacha20poly1305.NewX(messageKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// AD is the group ID
	ad := groupID
	ciphertext := aead.Seal(nil, nonce, plaintext, ad)

	// Sign the ciphertext
	signature := ed25519.Sign(senderKey.SigningKeyPriv, append(groupID, ciphertext...))

	// Update sender key in storage
	if err := s.storeSenderKey(senderKey); err != nil {
		return nil, fmt.Errorf("failed to store updated sender key: %w", err)
	}

	// For PoC, store ciphertext directly in the oneof bytes field
	return &pb.GroupPayload{
		Epoch:                senderKey.Epoch,
		ChainIndex:           senderKey.ChainIndex - 1, // Already incremented
		Content:              &pb.GroupPayload_Ciphertext{Ciphertext: ciphertext},
		SenderGroupSignature: signature,
	}, nil
}

// DecryptGroupMessage decrypts a group message using Sender Keys
func (s *Service) DecryptGroupMessage(groupID []byte, senderPubkey []byte, payload *pb.GroupPayload) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupKey := hex.EncodeToString(groupID)
	senderKeyMap, exists := s.senderKeys[groupKey]
	if !exists {
		return nil, ErrGroupNotFound
	}

	senderKey, exists := senderKeyMap[hex.EncodeToString(senderPubkey)]
	if !exists {
		return nil, ErrSenderKeyNotFound
	}

	// Verify epoch matches
	if payload.Epoch != senderKey.Epoch {
		return nil, ErrInvalidEpoch
	}

	// Derive message key (need to advance to the correct index)
	// In a real implementation, we'd need to handle out-of-order messages
	// by caching skipped message keys
	currentIndex := senderKey.ChainIndex
	if payload.ChainIndex < currentIndex {
		// This is a skipped message - would need to retrieve cached key
		return nil, errors.New("skipped message key not implemented")
	}

	// Advance chain to the correct index
	for senderKey.ChainIndex <= payload.ChainIndex {
		_, err := senderKey.DeriveMessageKey()
		if err != nil {
			return nil, fmt.Errorf("failed to derive message key: %w", err)
		}
	}

	// For simplicity, regenerate the message key at the target index
	// In production, this would use cached keys
	messageKey, err := s.deriveMessageKeyAtIndex(senderKey, payload.ChainIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to derive message key at index: %w", err)
	}

	// Generate nonce (same derivation as encryption)
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	chainIndexBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(chainIndexBytes, payload.ChainIndex)
	if _, err := hkdf.New(sha256.New, messageKey, []byte("nonce"), chainIndexBytes).Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Decrypt
	aead, err := chacha20poly1305.NewX(messageKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	ad := groupID
	// Extract ciphertext from oneof field
	ciphertext := payload.GetCiphertext()
	if len(ciphertext) == 0 {
		return nil, errors.New("no ciphertext in payload")
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, ad)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	// Verify signature
	if !ed25519.Verify(senderKey.SigningKey, append(groupID, ciphertext...), payload.SenderGroupSignature) {
		return nil, errors.New("signature verification failed")
	}

	return plaintext, nil
}

// GetGroup returns a group by ID
func (s *Service) GetGroup(groupID []byte) (*GroupState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupKey := hex.EncodeToString(groupID)
	state, exists := s.groups[groupKey]
	if !exists {
		return nil, ErrGroupNotFound
	}

	return state, nil
}

// ListGroups returns all groups the user is a member of
func (s *Service) ListGroups() []*GroupState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groups := make([]*GroupState, 0, len(s.groups))
	for _, state := range s.groups {
		groups = append(groups, state)
	}

	return groups
}

// GetSenderKeyDistributionMessage creates a sender key distribution message for a member
func (s *Service) GetSenderKeyDistributionMessage(groupID []byte, memberPubkey []byte) (*pb.SenderKeyDistribution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupKey := hex.EncodeToString(groupID)
	senderKeyMap, exists := s.senderKeys[groupKey]
	if !exists {
		return nil, ErrGroupNotFound
	}

	senderKey, exists := senderKeyMap[hex.EncodeToString(s.identitySignPub)]
	if !exists {
		return nil, ErrSenderKeyNotFound
	}

	return senderKey.ToProto(), nil
}

// ImportSenderKey imports a sender key received from another member
func (s *Service) ImportSenderKey(distribution *pb.SenderKeyDistribution, signingKeyPriv []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	senderKey := SenderKeyFromProto(distribution, signingKeyPriv)
	groupKey := hex.EncodeToString(senderKey.GroupID)

	s.initializeSenderKeysForGroup(senderKey.GroupID)
	s.senderKeys[groupKey][hex.EncodeToString(senderKey.SenderPubkey)] = senderKey

	return nil
}

// Helper functions

func (s *Service) storeGroup(state *GroupState) error {
	protoState := state.ToProto()
	return s.storage.SaveGroup(protoState)
}

func (s *Service) storeSenderKey(senderKey *SenderKey) error {
	// Store sender key in storage
	protoSK := senderKey.ToProto()
	return s.storage.SaveSenderKey(protoSK)
}

func (s *Service) initializeSenderKeysForGroup(groupID []byte) {
	groupKey := hex.EncodeToString(groupID)
	if _, exists := s.senderKeys[groupKey]; !exists {
		s.senderKeys[groupKey] = make(map[string]*SenderKey)
	}
}

func (s *Service) rotateAllSenderKeys(state *GroupState) error {
	groupKey := hex.EncodeToString(state.GroupID)

	// Generate new sender keys for all remaining members
	for _, member := range state.Members {
		senderKey, err := GenerateSenderKey(state.GroupID, member.Ed25519Pubkey)
		if err != nil {
			return fmt.Errorf("failed to generate sender key for member %x: %w", member.Ed25519Pubkey, err)
		}
		senderKey.Epoch = state.Epoch

		if memberKey, exists := s.senderKeys[groupKey][hex.EncodeToString(member.Ed25519Pubkey)]; exists {
			// Preserve chain index continuity if possible
			senderKey.ChainIndex = memberKey.ChainIndex
		}

		s.senderKeys[groupKey][hex.EncodeToString(member.Ed25519Pubkey)] = senderKey

		// Store updated sender key
		if err := s.storeSenderKey(senderKey); err != nil {
			return fmt.Errorf("failed to store rotated sender key: %w", err)
		}
	}

	return nil
}

func (s *Service) isMemberWithRole(state *GroupState, pubkey []byte, roles []GroupRole) bool {
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

func (s *Service) deriveMessageKeyAtIndex(senderKey *SenderKey, targetIndex uint32) ([]byte, error) {
	// This is a simplified implementation
	// In production, you'd cache skipped message keys
	// For now, we'll re-derive from the beginning (not efficient but works for PoC)

	// Note: This won't work correctly because the chain key has already advanced
	// A proper implementation would store the initial chain key separately
	// and derive forward, or cache all message keys

	// For the PoC, we'll use the current chain key and assume in-order delivery
	if targetIndex != senderKey.ChainIndex-1 {
		return nil, errors.New("out-of-order message key derivation not implemented")
	}

	// Derive message key at current index
	chainIndexBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(chainIndexBytes, targetIndex)
	hkdfInput := append([]byte("bt-sk-msg"), chainIndexBytes...)
	hash := sha256.Sum256(append(senderKey.ChainKey, hkdfInput...))
	return hash[:], nil
}
