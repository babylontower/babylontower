package groups

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGroupState(t *testing.T) {
	// Generate test keys
	_, privKey, _ := ed25519.GenerateKey(nil)
	pubKey := privKey.Public().(ed25519.PublicKey)
	x25519PubKey := make([]byte, 32)

	state, err := NewGroupState("Test Group", "A test group", PrivateGroup, pubKey, x25519PubKey)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Equal(t, "Test Group", state.Name)
	assert.Equal(t, "A test group", state.Description)
	assert.Equal(t, PrivateGroup, state.Type)
	assert.Equal(t, uint64(1), state.Epoch)
	assert.Len(t, state.GroupID, GroupIDSize)
	assert.Len(t, state.Members, 1)
	assert.Equal(t, Owner, state.Members[0].Role)
}

func TestGroupStateSerialization(t *testing.T) {
	_, privKey, _ := ed25519.GenerateKey(nil)
	pubKey := privKey.Public().(ed25519.PublicKey)
	x25519PubKey := make([]byte, 32)

	state, err := NewGroupState("Test", "Desc", PrivateGroup, pubKey, x25519PubKey)
	require.NoError(t, err)

	data, err := state.Serialize()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	hash, err := state.ComputeHash()
	require.NoError(t, err)
	assert.Len(t, hash, 32)
}

func TestGroupStateSigning(t *testing.T) {
	_, privKey, _ := ed25519.GenerateKey(nil)
	pubKey := privKey.Public().(ed25519.PublicKey)
	x25519PubKey := make([]byte, 32)

	state, err := NewGroupState("Test", "Desc", PrivateGroup, pubKey, x25519PubKey)
	require.NoError(t, err)

	err = state.Sign(privKey)
	require.NoError(t, err)
	assert.NotEmpty(t, state.StateSignature)

	valid := state.VerifySignature(pubKey)
	assert.True(t, valid)
}

func TestGroupStateAddMember(t *testing.T) {
	_, creatorPrivKey, _ := ed25519.GenerateKey(nil)
	creatorPubKey := creatorPrivKey.Public().(ed25519.PublicKey)
	x25519PubKey := make([]byte, 32)

	state, err := NewGroupState("Test", "Desc", PrivateGroup, creatorPubKey, x25519PubKey)
	require.NoError(t, err)

	_, memberPrivKey, _ := ed25519.GenerateKey(nil)
	memberPubKey := memberPrivKey.Public().(ed25519.PublicKey)

	member := GroupMember{
		Ed25519Pubkey: memberPubKey,
		X25519Pubkey:  x25519PubKey,
		DisplayName:   "New Member",
		JoinedAt:      12345,
		Role:          Member,
	}

	err = state.AddMember(member)
	require.NoError(t, err)
	assert.Len(t, state.Members, 2)
	assert.True(t, state.IsMember(memberPubKey))
}

func TestGroupStateRemoveMember(t *testing.T) {
	_, creatorPrivKey, _ := ed25519.GenerateKey(nil)
	creatorPubKey := creatorPrivKey.Public().(ed25519.PublicKey)
	x25519PubKey := make([]byte, 32)

	state, err := NewGroupState("Test", "Desc", PrivateGroup, creatorPubKey, x25519PubKey)
	require.NoError(t, err)

	_, memberPrivKey, _ := ed25519.GenerateKey(nil)
	memberPubKey := memberPrivKey.Public().(ed25519.PublicKey)

	member := GroupMember{
		Ed25519Pubkey: memberPubKey,
		X25519Pubkey:  x25519PubKey,
		DisplayName:   "Member",
		JoinedAt:      12345,
		Role:          Member,
	}

	err = state.AddMember(member)
	require.NoError(t, err)

	err = state.RemoveMember(memberPubKey)
	require.NoError(t, err)
	assert.Len(t, state.Members, 1)
	assert.False(t, state.IsMember(memberPubKey))
}

func TestGroupStateRemoveCreator(t *testing.T) {
	_, creatorPrivKey, _ := ed25519.GenerateKey(nil)
	creatorPubKey := creatorPrivKey.Public().(ed25519.PublicKey)
	x25519PubKey := make([]byte, 32)

	state, err := NewGroupState("Test", "Desc", PrivateGroup, creatorPubKey, x25519PubKey)
	require.NoError(t, err)

	err = state.RemoveMember(creatorPubKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot remove group creator")
}

func TestGroupStateValidateStateUpdate(t *testing.T) {
	_, adminPrivKey, _ := ed25519.GenerateKey(nil)
	adminPubKey := adminPrivKey.Public().(ed25519.PublicKey)
	x25519PubKey := make([]byte, 32)

	currentState, err := NewGroupState("Test", "Desc", PrivateGroup, adminPubKey, x25519PubKey)
	require.NoError(t, err)

	newState := &GroupState{
		GroupID:           currentState.GroupID,
		Epoch:             2,
		Name:              currentState.Name,
		Description:       currentState.Description,
		AvatarCID:         currentState.AvatarCID,
		Type:              currentState.Type,
		Members:           currentState.Members,
		CreatorPubkey:     currentState.CreatorPubkey,
		CreatedAt:         currentState.CreatedAt,
		UpdatedAt:         currentState.UpdatedAt,
		PreviousStateHash: nil,
	}

	currentHash, _ := currentState.ComputeHash()
	update := &GroupStateUpdate{
		NewState:            newState,
		PreviousStateHash:   currentHash,
		UpdaterPubkey:       adminPubKey,
		UpdaterSignature:    nil,
	}

	err = newState.Sign(adminPrivKey)
	require.NoError(t, err)
	update.UpdaterSignature = newState.StateSignature

	err = ValidateStateUpdate(update, currentState)
	assert.NoError(t, err)
}

func TestGroupStateValidateStateUpdateInvalidEpoch(t *testing.T) {
	_, adminPrivKey, _ := ed25519.GenerateKey(nil)
	adminPubKey := adminPrivKey.Public().(ed25519.PublicKey)
	x25519PubKey := make([]byte, 32)

	currentState, err := NewGroupState("Test", "Desc", PrivateGroup, adminPubKey, x25519PubKey)
	require.NoError(t, err)

	newState := &GroupState{
		GroupID:       currentState.GroupID,
		Epoch:         1, // Same epoch - should fail
		Name:          currentState.Name,
		Description:   currentState.Description,
		Members:       currentState.Members,
		CreatorPubkey: currentState.CreatorPubkey,
	}

	currentHash, _ := currentState.ComputeHash()
	update := &GroupStateUpdate{
		NewState:          newState,
		PreviousStateHash: currentHash,
		UpdaterPubkey:     adminPubKey,
	}

	err = ValidateStateUpdate(update, currentState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "epoch must be strictly increasing")
}

func TestGroupStateValidateStateUpdateNonAdmin(t *testing.T) {
	_, adminPrivKey, _ := ed25519.GenerateKey(nil)
	adminPubKey := adminPrivKey.Public().(ed25519.PublicKey)
	x25519PubKey := make([]byte, 32)

	currentState, err := NewGroupState("Test", "Desc", PrivateGroup, adminPubKey, x25519PubKey)
	require.NoError(t, err)

	_, nonMemberPrivKey, _ := ed25519.GenerateKey(nil)
	nonMemberPubKey := nonMemberPrivKey.Public().(ed25519.PublicKey)

	newState := &GroupState{
		GroupID:       currentState.GroupID,
		Epoch:         2,
		Name:          currentState.Name,
		Description:   currentState.Description,
		Members:       currentState.Members,
		CreatorPubkey: currentState.CreatorPubkey,
	}

	currentHash, _ := currentState.ComputeHash()
	update := &GroupStateUpdate{
		NewState:          newState,
		PreviousStateHash: currentHash,
		UpdaterPubkey:     nonMemberPubKey,
	}

	err = ValidateStateUpdate(update, currentState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "updater is not an admin or owner")
}

func TestGenerateSenderKey(t *testing.T) {
	groupID := make([]byte, 32)
	senderPubkey := make([]byte, 32)

	senderKey, err := GenerateSenderKey(groupID, senderPubkey)
	require.NoError(t, err)
	require.NotNil(t, senderKey)

	assert.Equal(t, groupID, senderKey.GroupID)
	assert.Equal(t, senderPubkey, senderKey.SenderPubkey)
	assert.Len(t, senderKey.ChainKey, 32)
	assert.Len(t, senderKey.SigningKey, ed25519.PublicKeySize)
	assert.Len(t, senderKey.SigningKeyPriv, ed25519.PrivateKeySize)
	assert.Equal(t, uint32(0), senderKey.ChainIndex)
	assert.Equal(t, uint64(1), senderKey.Epoch)
}

func TestSenderKeyDeriveMessageKey(t *testing.T) {
	groupID := make([]byte, 32)
	senderPubkey := make([]byte, 32)

	senderKey, err := GenerateSenderKey(groupID, senderPubkey)
	require.NoError(t, err)

	key1, err := senderKey.DeriveMessageKey()
	require.NoError(t, err)
	assert.Len(t, key1, 32)
	assert.Equal(t, uint32(1), senderKey.ChainIndex)

	key2, err := senderKey.DeriveMessageKey()
	require.NoError(t, err)
	assert.Len(t, key2, 32)
	assert.Equal(t, uint32(2), senderKey.ChainIndex)

	assert.NotEqual(t, key1, key2)
}

func TestResolveSplitBrain(t *testing.T) {
	_, privKey1, _ := ed25519.GenerateKey(nil)
	pubKey1 := privKey1.Public().(ed25519.PublicKey)
	_, privKey2, _ := ed25519.GenerateKey(nil)
	pubKey2 := privKey2.Public().(ed25519.PublicKey)
	x25519PubKey := make([]byte, 32)

	state1, _ := NewGroupState("Test1", "Desc1", PrivateGroup, pubKey1, x25519PubKey)
	state1.Epoch = 2

	state2, _ := NewGroupState("Test2", "Desc2", PrivateGroup, pubKey2, x25519PubKey)
	state2.Epoch = 3

	state3, _ := NewGroupState("Test3", "Desc3", PrivateGroup, pubKey1, x25519PubKey)
	state3.Epoch = 3

	// Higher epoch wins
	best := ResolveSplitBrain([]*GroupState{state1, state2})
	assert.Equal(t, state2, best)

	// Same epoch, lower pubkey wins
	best = ResolveSplitBrain([]*GroupState{state2, state3})
	if string(state2.CreatorPubkey) < string(state3.CreatorPubkey) {
		assert.Equal(t, state2, best)
	} else {
		assert.Equal(t, state3, best)
	}
}

func TestGroupStateMaxMembers(t *testing.T) {
	_, creatorPrivKey, _ := ed25519.GenerateKey(nil)
	creatorPubKey := creatorPrivKey.Public().(ed25519.PublicKey)
	x25519PubKey := make([]byte, 32)

	state, err := NewGroupState("Test", "Desc", PrivateGroup, creatorPubKey, x25519PubKey)
	require.NoError(t, err)

	// Fill up to max
	for i := 0; i < MaxGroupMembers-1; i++ {
		_, memberPrivKey, _ := ed25519.GenerateKey(nil)
		memberPubKey := memberPrivKey.Public().(ed25519.PublicKey)

		member := GroupMember{
			Ed25519Pubkey: memberPubKey,
			X25519Pubkey:  x25519PubKey,
			DisplayName:   "Member",
			JoinedAt:      12345,
			Role:          Member,
		}
		err = state.AddMember(member)
		require.NoError(t, err)
	}

	// Try to add one more
	_, memberPrivKey, _ := ed25519.GenerateKey(nil)
	memberPubKey := memberPrivKey.Public().(ed25519.PublicKey)

	member := GroupMember{
		Ed25519Pubkey: memberPubKey,
		X25519Pubkey:  x25519PubKey,
		DisplayName:   "Extra Member",
		JoinedAt:      12345,
		Role:          Member,
	}

	err = state.AddMember(member)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maximum member limit")
}
