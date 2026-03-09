package app

import (
	"encoding/hex"
	"fmt"
	"time"

	"babylontower/pkg/groups"
)

// GroupInfo contains UI-friendly group information.
type GroupInfo struct {
	// GroupID is the hex-encoded group identifier
	GroupID string
	// Name is the group name
	Name string
	// Description is the group description
	Description string
	// Type is the group type
	Type groups.GroupType
	// TypeLabel is a human-readable type label
	TypeLabel string
	// MemberCount is the number of members
	MemberCount int
	// CreatorPubKeyBase58 is the creator's public key
	CreatorPubKeyBase58 string
	// CreatedAt is when the group was created
	CreatedAt time.Time
	// Epoch is the current state epoch
	Epoch uint64
	// Members is the list of members
	Members []*GroupMemberInfo
	// IsAdmin indicates if the current user is an admin or owner
	IsAdmin bool
	// IsOwner indicates if the current user is the owner
	IsOwner bool
}

// GroupMemberInfo contains UI-friendly member information.
type GroupMemberInfo struct {
	// PubKeyHex is the member's Ed25519 public key in hex
	PubKeyHex string
	// DisplayName is the member's name in the group
	DisplayName string
	// Role is the member's role
	Role groups.GroupRole
	// RoleLabel is a human-readable role label
	RoleLabel string
	// JoinedAt is when the member joined
	JoinedAt time.Time
}

// UIGroupManager provides high-level group management for UI.
type UIGroupManager interface {
	// CreateGroup creates a new group.
	CreateGroup(name, description string, groupType groups.GroupType) (*GroupInfo, error)

	// ListGroups returns all groups the user is a member of.
	ListGroups() ([]*GroupInfo, error)

	// GetGroup returns group info by hex ID.
	GetGroup(groupIDHex string) (*GroupInfo, error)

	// AddMember adds a member to a group.
	AddMember(groupIDHex, memberPubKeyStr, memberX25519Str, displayName string, role groups.GroupRole) error

	// RemoveMember removes a member from a group.
	RemoveMember(groupIDHex, memberPubKeyStr string) error

	// LeaveGroup removes the current user from a group.
	LeaveGroup(groupIDHex string) error

	// UpdateGroupInfo updates the group name and description (admin/owner only).
	UpdateGroupInfo(groupIDHex, name, description string) error

	// DeleteGroup deletes a group (owner only).
	DeleteGroup(groupIDHex string) error
}

// uiGroupManager implements UIGroupManager.
type uiGroupManager struct {
	app *application
}

func newUIGroupManager(app *application) *uiGroupManager {
	return &uiGroupManager{app: app}
}

func (gm *uiGroupManager) CreateGroup(name, description string, groupType groups.GroupType) (*GroupInfo, error) {
	if gm.app.groups == nil {
		return nil, fmt.Errorf("groups service not available")
	}

	state, err := gm.app.groups.CreateGroup(name, description, groupType)
	if err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}

	return gm.groupInfoFromState(state), nil
}

func (gm *uiGroupManager) ListGroups() ([]*GroupInfo, error) {
	if gm.app.groups == nil {
		return nil, fmt.Errorf("groups service not available")
	}

	states := gm.app.groups.ListGroups()
	result := make([]*GroupInfo, 0, len(states))
	for _, s := range states {
		result = append(result, gm.groupInfoFromState(s))
	}
	return result, nil
}

func (gm *uiGroupManager) GetGroup(groupIDHex string) (*GroupInfo, error) {
	if gm.app.groups == nil {
		return nil, fmt.Errorf("groups service not available")
	}

	groupID, err := hex.DecodeString(groupIDHex)
	if err != nil {
		return nil, fmt.Errorf("invalid group ID: %w", err)
	}

	state, err := gm.app.groups.GetGroup(groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get group: %w", err)
	}

	return gm.groupInfoFromState(state), nil
}

func (gm *uiGroupManager) AddMember(groupIDHex, memberPubKeyStr, memberX25519Str, displayName string, role groups.GroupRole) error {
	if gm.app.groups == nil {
		return fmt.Errorf("groups service not available")
	}

	groupID, err := hex.DecodeString(groupIDHex)
	if err != nil {
		return fmt.Errorf("invalid group ID: %w", err)
	}

	memberPubKey, err := decodePubKey(memberPubKeyStr)
	if err != nil {
		return fmt.Errorf("invalid member key: %w", err)
	}

	var memberX25519 []byte
	if memberX25519Str != "" {
		memberX25519, err = decodePubKey(memberX25519Str)
		if err != nil {
			return fmt.Errorf("invalid X25519 key: %w", err)
		}
	}

	_, err = gm.app.groups.AddMember(groupID, memberPubKey, memberX25519, displayName, role)
	return err
}

func (gm *uiGroupManager) RemoveMember(groupIDHex, memberPubKeyStr string) error {
	if gm.app.groups == nil {
		return fmt.Errorf("groups service not available")
	}

	groupID, err := hex.DecodeString(groupIDHex)
	if err != nil {
		return fmt.Errorf("invalid group ID: %w", err)
	}

	memberPubKey, err := decodePubKey(memberPubKeyStr)
	if err != nil {
		return fmt.Errorf("invalid member key: %w", err)
	}

	_, err = gm.app.groups.RemoveMember(groupID, memberPubKey)
	return err
}

func (gm *uiGroupManager) LeaveGroup(groupIDHex string) error {
	myPubKeyHex := hex.EncodeToString(gm.app.identity.Ed25519PubKey)
	return gm.RemoveMember(groupIDHex, myPubKeyHex)
}

func (gm *uiGroupManager) UpdateGroupInfo(groupIDHex, name, description string) error {
	// Groups service doesn't have an update method yet.
	// This would require re-signing the state with updated fields.
	return fmt.Errorf("group info update not yet implemented")
}

func (gm *uiGroupManager) DeleteGroup(groupIDHex string) error {
	if gm.app.storage == nil {
		return fmt.Errorf("storage not available")
	}

	groupID, err := hex.DecodeString(groupIDHex)
	if err != nil {
		return fmt.Errorf("invalid group ID: %w", err)
	}

	return gm.app.storage.DeleteGroup(groupID)
}

func (gm *uiGroupManager) groupInfoFromState(state *groups.GroupState) *GroupInfo {
	myPubKey := gm.app.identity.Ed25519PubKey

	info := &GroupInfo{
		GroupID:     hex.EncodeToString(state.GroupID),
		Name:        state.Name,
		Description: state.Description,
		Type:        state.Type,
		TypeLabel:   groupTypeLabel(state.Type),
		MemberCount: len(state.Members),
		CreatedAt:   time.Unix(int64(state.CreatedAt), 0),
		Epoch:       state.Epoch,
		Members:     make([]*GroupMemberInfo, 0, len(state.Members)),
	}

	if len(state.CreatorPubkey) > 0 {
		info.CreatorPubKeyBase58 = hex.EncodeToString(state.CreatorPubkey)
	}

	for _, m := range state.Members {
		mi := &GroupMemberInfo{
			PubKeyHex:   hex.EncodeToString(m.Ed25519Pubkey),
			DisplayName: m.DisplayName,
			Role:        m.Role,
			RoleLabel:   roleLabel(m.Role),
			JoinedAt:    time.Unix(int64(m.JoinedAt), 0),
		}
		info.Members = append(info.Members, mi)

		// Check if current user
		if hex.EncodeToString(m.Ed25519Pubkey) == hex.EncodeToString(myPubKey) {
			if m.Role == groups.Owner {
				info.IsOwner = true
				info.IsAdmin = true
			} else if m.Role == groups.Admin {
				info.IsAdmin = true
			}
		}
	}

	return info
}

func groupTypeLabel(t groups.GroupType) string {
	switch t {
	case groups.PrivateGroup:
		return "Private Group"
	case groups.PublicGroup:
		return "Public Group"
	case groups.PrivateChannel:
		return "Private Channel"
	case groups.PublicChannel:
		return "Public Channel"
	default:
		return "Unknown"
	}
}

func roleLabel(r groups.GroupRole) string {
	switch r {
	case groups.Owner:
		return "Owner"
	case groups.Admin:
		return "Admin"
	case groups.Member:
		return "Member"
	default:
		return "Unknown"
	}
}
