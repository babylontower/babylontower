package groups

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	pb "babylontower/pkg/proto"
	"golang.org/x/crypto/ed25519"
	"google.golang.org/protobuf/proto"
)

const (
	// GroupIDSize is the size of group identifiers in bytes
	GroupIDSize = 32
	// MaxGroupNameLength is the maximum length of group names
	MaxGroupNameLength = 128
	// MaxGroupDescriptionLength is the maximum length of group descriptions
	MaxGroupDescriptionLength = 512
	// MaxGroupMembers is the maximum number of members in a private group
	MaxGroupMembers = 1000
)

// GroupType represents the type of group
type GroupType int32

const (
	PrivateGroup   GroupType = 0
	PublicGroup    GroupType = 1
	PrivateChannel GroupType = 2
	PublicChannel  GroupType = 3
)

// GroupRole represents a member's role in a group
type GroupRole int32

const (
	Member GroupRole = 0
	Admin  GroupRole = 1
	Owner  GroupRole = 2
)

// GroupState represents the current state of a group
type GroupState struct {
	// GroupID is the random 32-byte identifier
	GroupID []byte
	// Epoch is incremented on every membership change
	Epoch uint64
	// Name is the group name (max 128 UTF-8 chars)
	Name string
	// Description is the group description (max 512 UTF-8 chars)
	Description string
	// AvatarCID is the IPFS CID of the group avatar
	AvatarCID string
	// Type is the group type
	Type GroupType
	// Members is the list of group members
	Members []GroupMember
	// CreatorPubkey is the Ed25519 public key of the group creator
	CreatorPubkey []byte
	// CreatedAt is the group creation timestamp
	CreatedAt uint64
	// UpdatedAt is the last update timestamp
	UpdatedAt uint64
	// StateSignature is the signature of this state update
	StateSignature []byte
	// PreviousStateHash is the SHA256 of the previous serialized state
	PreviousStateHash []byte
}

// GroupMember represents a member of a group
type GroupMember struct {
	// Ed25519Pubkey is the member's Ed25519 public key
	Ed25519Pubkey []byte
	// X25519Pubkey is the member's X25519 public key for encryption
	X25519Pubkey []byte
	// DisplayName is the member's display name in the group
	DisplayName string
	// JoinedAt is the timestamp when the member joined
	JoinedAt uint64
	// Role is the member's role in the group
	Role GroupRole
}

// GroupStateUpdate represents a state update for a group
type GroupStateUpdate struct {
	// NewState is the new group state
	NewState *GroupState
	// PreviousStateHash is the SHA256 of the previous serialized state
	PreviousStateHash []byte
	// UpdaterPubkey is the Ed25519 public key of the updater (must be admin/owner)
	UpdaterPubkey []byte
	// UpdaterSignature is the Ed25519 signature of this update
	UpdaterSignature []byte
}

// SenderKey represents a sender's key chain for group encryption
type SenderKey struct {
	// GroupID is the group this sender key belongs to
	GroupID []byte
	// SenderPubkey is the Ed25519 public key of the sender
	SenderPubkey []byte
	// ChainKey is the current chain key (32 bytes)
	ChainKey []byte
	// SigningKey is the Ed25519 public key for message authentication
	SigningKey []byte
	// SigningKeyPriv is the private part of the signing key
	SigningKeyPriv []byte
	// ChainIndex is the current chain index
	ChainIndex uint32
	// Epoch is the group epoch this sender key belongs to
	Epoch uint64
}

// NewGroupState creates a new group state with the given parameters
func NewGroupState(
	name string,
	description string,
	groupType GroupType,
	creatorPubkey []byte,
	creatorX25519Pubkey []byte,
) (*GroupState, error) {
	// Generate random group ID
	groupID := make([]byte, GroupIDSize)
	if _, err := rand.Read(groupID); err != nil {
		return nil, fmt.Errorf("failed to generate group ID: %w", err)
	}

	// Validate name length
	if len(name) > MaxGroupNameLength {
		return nil, fmt.Errorf("group name exceeds maximum length of %d", MaxGroupNameLength)
	}

	// Validate description length
	if len(description) > MaxGroupDescriptionLength {
		return nil, fmt.Errorf("group description exceeds maximum length of %d", MaxGroupDescriptionLength)
	}

	now := uint64(time.Now().Unix())

	// Create initial member list with creator as owner
	members := []GroupMember{
		{
			Ed25519Pubkey: creatorPubkey,
			X25519Pubkey:  creatorX25519Pubkey,
			DisplayName:   "Creator",
			JoinedAt:      now,
			Role:          Owner,
		},
	}

	state := &GroupState{
		GroupID:           groupID,
		Epoch:             1,
		Name:              name,
		Description:       description,
		AvatarCID:         "",
		Type:              groupType,
		Members:           members,
		CreatorPubkey:     creatorPubkey,
		CreatedAt:         now,
		UpdatedAt:         now,
		StateSignature:    nil,
		PreviousStateHash: nil, // Empty for initial state
	}

	return state, nil
}

// ToProto converts GroupState to protobuf format
func (gs *GroupState) ToProto() *pb.GroupState {
	members := make([]*pb.GroupMember, len(gs.Members))
	for i, m := range gs.Members {
		members[i] = &pb.GroupMember{
			Ed25519Pubkey: m.Ed25519Pubkey,
			X25519Pubkey:  m.X25519Pubkey,
			DisplayName:   m.DisplayName,
			JoinedAt:      m.JoinedAt,
			Role:          pb.GroupRole(m.Role),
		}
	}

	return &pb.GroupState{
		GroupId:         gs.GroupID,
		Epoch:           gs.Epoch,
		Name:            gs.Name,
		Description:     gs.Description,
		AvatarCid:       gs.AvatarCID,
		Type:            pb.GroupType(gs.Type),
		Members:         members,
		CreatorPubkey:   gs.CreatorPubkey,
		CreatedAt:       gs.CreatedAt,
		UpdatedAt:       gs.UpdatedAt,
		StateSignature:  gs.StateSignature,
		PreviousHash:    gs.PreviousStateHash,
	}
}

// FromProto converts protobuf GroupState to Go struct
func FromProto(protoState *pb.GroupState) *GroupState {
	members := make([]GroupMember, len(protoState.Members))
	for i, m := range protoState.Members {
		members[i] = GroupMember{
			Ed25519Pubkey: m.Ed25519Pubkey,
			X25519Pubkey:  m.X25519Pubkey,
			DisplayName:   m.DisplayName,
			JoinedAt:      m.JoinedAt,
			Role:          GroupRole(m.Role),
		}
	}

	return &GroupState{
		GroupID:           protoState.GroupId,
		Epoch:             protoState.Epoch,
		Name:              protoState.Name,
		Description:       protoState.Description,
		AvatarCID:         protoState.AvatarCid,
		Type:              GroupType(protoState.Type),
		Members:           members,
		CreatorPubkey:     protoState.CreatorPubkey,
		CreatedAt:         protoState.CreatedAt,
		UpdatedAt:         protoState.UpdatedAt,
		StateSignature:    protoState.StateSignature,
		PreviousStateHash: protoState.PreviousHash,
	}
}

// Serialize serializes the group state for hashing and signing
func (gs *GroupState) Serialize() ([]byte, error) {
	protoMsg := gs.ToProto()
	data, err := proto.Marshal(protoMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal group state: %w", err)
	}
	return data, nil
}

// ComputeHash computes the SHA256 hash of the serialized state
func (gs *GroupState) ComputeHash() ([]byte, error) {
	data, err := gs.Serialize()
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(data)
	return hash[:], nil
}

// Sign signs the group state with the updater's Ed25519 private key
func (gs *GroupState) Sign(privKey ed25519.PrivateKey) error {
	// Create a copy without the signature for signing
	copyState := &GroupState{
		GroupID:           gs.GroupID,
		Epoch:             gs.Epoch,
		Name:              gs.Name,
		Description:       gs.Description,
		AvatarCID:         gs.AvatarCID,
		Type:              gs.Type,
		Members:           gs.Members,
		CreatorPubkey:     gs.CreatorPubkey,
		CreatedAt:         gs.CreatedAt,
		UpdatedAt:         gs.UpdatedAt,
		PreviousStateHash: gs.PreviousStateHash,
	}

	data, err := copyState.Serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize state: %w", err)
	}

	signature := ed25519.Sign(privKey, data)
	gs.StateSignature = signature
	return nil
}

// VerifySignature verifies the state signature
func (gs *GroupState) VerifySignature(pubKey ed25519.PublicKey) bool {
	if len(gs.StateSignature) == 0 {
		return false
	}

	// Create a copy without the signature for verification
	copyState := &GroupState{
		GroupID:           gs.GroupID,
		Epoch:             gs.Epoch,
		Name:              gs.Name,
		Description:       gs.Description,
		AvatarCID:         gs.AvatarCID,
		Type:              gs.Type,
		Members:           gs.Members,
		CreatorPubkey:     gs.CreatorPubkey,
		CreatedAt:         gs.CreatedAt,
		UpdatedAt:         gs.UpdatedAt,
		PreviousStateHash: gs.PreviousStateHash,
	}

	data, err := copyState.Serialize()
	if err != nil {
		return false
	}

	return ed25519.Verify(pubKey, data, gs.StateSignature)
}

// ValidateStateUpdate validates a state update
func ValidateStateUpdate(update *GroupStateUpdate, currentState *GroupState) error {
	// Verify previous state hash matches
	currentHash, err := currentState.ComputeHash()
	if err != nil {
		return fmt.Errorf("failed to compute current state hash: %w", err)
	}

	if len(update.PreviousStateHash) > 0 && len(currentState.PreviousStateHash) > 0 {
		if string(update.PreviousStateHash) != string(currentHash) {
			return fmt.Errorf("previous state hash mismatch")
		}
	}

	// Verify epoch is incremented
	if update.NewState.Epoch <= currentState.Epoch {
		return fmt.Errorf("epoch must be strictly increasing: current=%d, new=%d", currentState.Epoch, update.NewState.Epoch)
	}

	// Verify updater is an admin or owner
	updaterIsAdmin := false
	for _, member := range currentState.Members {
		if string(member.Ed25519Pubkey) == string(update.UpdaterPubkey) {
			if member.Role == Admin || member.Role == Owner {
				updaterIsAdmin = true
				break
			}
		}
	}

	if !updaterIsAdmin {
		return fmt.Errorf("updater is not an admin or owner")
	}

	// Verify updater's signature
	// Create a copy without signatures for verification
	stateForSig := &GroupState{
		GroupID:           update.NewState.GroupID,
		Epoch:             update.NewState.Epoch,
		Name:              update.NewState.Name,
		Description:       update.NewState.Description,
		AvatarCID:         update.NewState.AvatarCID,
		Type:              update.NewState.Type,
		Members:           update.NewState.Members,
		CreatorPubkey:     update.NewState.CreatorPubkey,
		CreatedAt:         update.NewState.CreatedAt,
		UpdatedAt:         update.NewState.UpdatedAt,
		PreviousStateHash: update.NewState.PreviousStateHash,
	}

	data, err := stateForSig.Serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize new state: %w", err)
	}

	if !ed25519.Verify(update.UpdaterPubkey, data, update.UpdaterSignature) {
		return fmt.Errorf("invalid updater signature")
	}

	return nil
}

// GenerateSenderKey generates a new sender key for a group member
func GenerateSenderKey(groupID []byte, senderPubkey []byte) (*SenderKey, error) {
	chainKey := make([]byte, 32)
	if _, err := rand.Read(chainKey); err != nil {
		return nil, fmt.Errorf("failed to generate chain key: %w", err)
	}

	// Generate Ed25519 signing key pair
	_, signingKeyPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate signing key: %w", err)
	}

	signingKey := make([]byte, ed25519.PublicKeySize)
	copy(signingKey, signingKeyPriv.Public().(ed25519.PublicKey))

	return &SenderKey{
		GroupID:        groupID,
		SenderPubkey:   senderPubkey,
		ChainKey:       chainKey,
		SigningKey:     signingKey,
		SigningKeyPriv: signingKeyPriv,
		ChainIndex:     0,
		Epoch:          1,
	}, nil
}

// DeriveMessageKey derives a message key from the chain key
func (sk *SenderKey) DeriveMessageKey() ([]byte, error) {
	// Derive message key using HKDF
	chainIndexBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(chainIndexBytes, sk.ChainIndex)

	// msg_key = HKDF(chain_key, salt="bt-sk-msg", info=chain_index_bytes, len=32)
	hkdfInput := append([]byte("bt-sk-msg"), chainIndexBytes...)
	hash := sha256.Sum256(append(sk.ChainKey, hkdfInput...))
	messageKey := hash[:]

	// Advance chain: chain_key = HKDF(chain_key, salt="bt-sk-chain", info="advance", len=32)
	advanceInput := append([]byte("bt-sk-chain"), []byte("advance")...)
	newHash := sha256.Sum256(append(sk.ChainKey, advanceInput...))
	sk.ChainKey = newHash[:]
	sk.ChainIndex++

	return messageKey, nil
}

// ToProto converts SenderKey to protobuf format
func (sk *SenderKey) ToProto() *pb.SenderKeyDistribution {
	return &pb.SenderKeyDistribution{
		GroupId:    sk.GroupID,
		Epoch:      sk.Epoch,
		SenderPub:  sk.SenderPubkey,
		ChainKey:   sk.ChainKey,
		SigningKey: sk.SigningKey,
		ChainIndex: sk.ChainIndex,
	}
}

// FromProto converts protobuf SenderKeyDistribution to Go struct
func SenderKeyFromProto(protoSK *pb.SenderKeyDistribution, signingKeyPriv []byte) *SenderKey {
	return &SenderKey{
		GroupID:        protoSK.GroupId,
		Epoch:          protoSK.Epoch,
		SenderPubkey:   protoSK.SenderPub,
		ChainKey:       protoSK.ChainKey,
		SigningKey:     protoSK.SigningKey,
		SigningKeyPriv: signingKeyPriv,
		ChainIndex:     protoSK.ChainIndex,
	}
}

// ResolveSplitBrain resolves split-brain scenarios by selecting the authoritative state
// Returns the state with the highest epoch, with lexicographically lower creator pubkey as tiebreaker
func ResolveSplitBrain(states []*GroupState) *GroupState {
	if len(states) == 0 {
		return nil
	}

	if len(states) == 1 {
		return states[0]
	}

	// Sort by epoch (descending), then by creator pubkey (ascending for tiebreak)
	best := states[0]
	for _, state := range states[1:] {
		if state.Epoch > best.Epoch {
			best = state
		} else if state.Epoch == best.Epoch {
			// Tiebreak by lexicographically lower creator pubkey
			if string(state.CreatorPubkey) < string(best.CreatorPubkey) {
				best = state
			}
		}
	}

	return best
}

// IsMember checks if a public key is a member of the group
func (gs *GroupState) IsMember(pubkey []byte) bool {
	for _, member := range gs.Members {
		if string(member.Ed25519Pubkey) == string(pubkey) {
			return true
		}
	}
	return false
}

// GetMemberRole returns the role of a member, or -1 if not a member
func (gs *GroupState) GetMemberRole(pubkey []byte) GroupRole {
	for _, member := range gs.Members {
		if string(member.Ed25519Pubkey) == string(pubkey) {
			return member.Role
		}
	}
	return -1
}

// AddMember adds a new member to the group (caller must handle epoch increment and signing)
func (gs *GroupState) AddMember(member GroupMember) error {
	if len(gs.Members) >= MaxGroupMembers {
		return fmt.Errorf("group has reached maximum member limit of %d", MaxGroupMembers)
	}

	// Check if member already exists
	for _, m := range gs.Members {
		if string(m.Ed25519Pubkey) == string(member.Ed25519Pubkey) {
			return fmt.Errorf("member already in group")
		}
	}

	gs.Members = append(gs.Members, member)
	gs.UpdatedAt = uint64(time.Now().Unix())
	return nil
}

// RemoveMember removes a member from the group (caller must handle epoch increment and signing)
func (gs *GroupState) RemoveMember(pubkey []byte) error {
	for i, member := range gs.Members {
		if string(member.Ed25519Pubkey) == string(pubkey) {
			// Don't allow creator to be removed
			if string(pubkey) == string(gs.CreatorPubkey) {
				return fmt.Errorf("cannot remove group creator")
			}
			gs.Members = append(gs.Members[:i], gs.Members[i+1:]...)
			gs.UpdatedAt = uint64(time.Now().Unix())
			return nil
		}
	}
	return fmt.Errorf("member not found")
}
