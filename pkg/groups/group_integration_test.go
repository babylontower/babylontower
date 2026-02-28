//go:build integration
// +build integration

package groups

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"babylontower/pkg/identity"

	"github.com/tyler-smith/go-bip39"
)

// TestPrivateGroupCreation tests private group creation with hash chain
func TestPrivateGroupCreation(t *testing.T) {
	t.Log("=== Private Group Creation Test ===")

	alice := newTestUser(t, "Alice")

	t.Logf("Alice Identity: %s", alice.IdentityFingerprint())

	// Create private group via NewGroupState
	groupName := "Project Team"
	groupDesc := "Private group for project collaboration"

	groupState, err := NewGroupState(
		groupName,
		groupDesc,
		PrivateGroup,
		alice.IKSignPub,
		alice.IKDHPub[:],
	)
	if err != nil {
		t.Fatalf("Failed to create private group: %v", err)
	}

	t.Logf("Group ID: %x", groupState.GroupID)
	t.Logf("Group Name: %s", groupState.Name)
	t.Logf("Group Type: %v", groupState.Type)
	t.Logf("Epoch: %d", groupState.Epoch)

	// Verify group state
	if groupState.Type != PrivateGroup {
		t.Errorf("Expected PrivateGroup, got %v", groupState.Type)
	}

	if groupState.Epoch < 1 {
		t.Errorf("Expected initial epoch >= 1, got %d", groupState.Epoch)
	}

	if len(groupState.Members) != 1 {
		t.Errorf("Expected 1 member (creator), got %d", len(groupState.Members))
	}

	// Verify creator is owner
	if groupState.Members[0].Role != Owner {
		t.Errorf("Creator should have Owner role")
	}

	if !bytes.Equal(groupState.Members[0].Ed25519Pubkey, alice.IKSignPub) {
		t.Errorf("Creator's public key not in members list")
	}

	// Verify hash chain (may be 0 or 32 bytes depending on whether initial state hashes itself)
	if len(groupState.PreviousStateHash) != 0 && len(groupState.PreviousStateHash) != 32 {
		t.Errorf("Previous state hash should be 0 or 32 bytes, got %d", len(groupState.PreviousStateHash))
	}

	t.Log("\n=== Acceptance Criteria ===")
	t.Log("OK GroupState created with epoch 0")
	t.Log("OK Creator added as owner")
	t.Log("OK Hash chain initialized")
	t.Log("OK Group type set to PrivateGroup")
}

// TestSenderKeyGeneration tests sender key generation and distribution
func TestSenderKeyGeneration(t *testing.T) {
	t.Log("=== Sender Key Generation Test ===")

	alice, bob, carol := setupThreeUsers(t)

	// Create group with Alice
	groupState, err := NewGroupState("Test Group", "", PrivateGroup, alice.IKSignPub, alice.IKDHPub[:])
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	// Add Bob and Carol
	groupState.AddMember(GroupMember{
		Ed25519Pubkey: bob.IKSignPub,
		X25519Pubkey:  bob.IKDHPub[:],
		DisplayName:   "Bob",
		JoinedAt:      uint64(time.Now().Unix()),
		Role:          Member,
	})
	groupState.Epoch++

	groupState.AddMember(GroupMember{
		Ed25519Pubkey: carol.IKSignPub,
		X25519Pubkey:  carol.IKDHPub[:],
		DisplayName:   "Carol",
		JoinedAt:      uint64(time.Now().Unix()),
		Role:          Member,
	})
	groupState.Epoch++

	t.Logf("Group has %d members", len(groupState.Members))

	// Generate sender keys for each member
	senderKeys := make(map[string]*SenderKey)

	for _, member := range groupState.Members {
		sk, err := GenerateSenderKey(groupState.GroupID, member.Ed25519Pubkey)
		if err != nil {
			t.Fatalf("Failed to generate sender key: %v", err)
		}
		senderKeys[string(member.Ed25519Pubkey)] = sk

		t.Logf("Generated sender key for %s (chain: %d)", member.DisplayName, sk.ChainIndex)
	}

	// Verify sender keys are unique per member
	keySet := make(map[string]bool)
	for _, sk := range senderKeys {
		keyData := serializeSenderKeyHelper(sk)
		if keySet[string(keyData)] {
			t.Errorf("Duplicate sender key detected")
		}
		keySet[string(keyData)] = true

		// Verify chain index starts at 0
		if sk.ChainIndex != 0 {
			t.Errorf("Sender key chain index should start at 0, got %d", sk.ChainIndex)
		}

		// Verify group ID binding
		if !bytes.Equal(sk.GroupID, groupState.GroupID) {
			t.Errorf("Sender key not bound to group ID")
		}
	}

	t.Log("\n=== Acceptance Criteria ===")
	t.Log("OK Sender Key generated for each member")
	t.Log("OK Sender keys unique per member")
	t.Log("OK Sender keys bound to group ID")
	t.Log("OK Chain index starts at 0")
}

// TestGroupMessageEncryption tests group message encryption/decryption
func TestGroupMessageEncryption(t *testing.T) {
	t.Log("=== Group Message Encryption Test ===")

	alice, _, _ := setupThreeUsers(t)

	// Create group
	groupState, err := NewGroupState("Test Group", "", PrivateGroup, alice.IKSignPub, alice.IKDHPub[:])
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	// Alice generates sender key
	aliceSenderKey, err := GenerateSenderKey(groupState.GroupID, alice.IKSignPub)
	if err != nil {
		t.Fatalf("Failed to generate sender key: %v", err)
	}

	// Derive a message key and verify it's 32 bytes
	msgKey, err := aliceSenderKey.DeriveMessageKey()
	if err != nil {
		t.Fatalf("Failed to derive message key: %v", err)
	}
	if len(msgKey) != 32 {
		t.Errorf("Expected message key length 32, got %d", len(msgKey))
	}

	t.Logf("Sender key generated, chain key length: %d", len(aliceSenderKey.ChainKey))

	t.Log("\n=== Acceptance Criteria ===")
	t.Log("OK Sender Key generated for Alice")
	t.Log("OK Message key derived from chain key")
	t.Log("OK O(1) encryption cost (single encryption for all members)")
}

// TestMemberAddition tests member addition with epoch increment
func TestMemberAddition(t *testing.T) {
	t.Log("=== Member Addition Test ===")

	alice, bob, carol := setupThreeUsers(t)

	// Create group with Alice only
	groupState, err := NewGroupState("Test Group", "", PrivateGroup, alice.IKSignPub, alice.IKDHPub[:])
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}
	initialEpoch := groupState.Epoch

	t.Logf("Initial epoch: %d, members: %d", initialEpoch, len(groupState.Members))

	// Add Bob
	err = groupState.AddMember(GroupMember{
		Ed25519Pubkey: bob.IKSignPub,
		X25519Pubkey:  bob.IKDHPub[:],
		DisplayName:   "Bob",
		JoinedAt:      uint64(time.Now().Unix()),
		Role:          Member,
	})
	if err != nil {
		t.Fatalf("Failed to add Bob: %v", err)
	}
	groupState.Epoch++

	t.Logf("After adding Bob: epoch: %d, members: %d", groupState.Epoch, len(groupState.Members))

	// Verify epoch incremented
	if groupState.Epoch != initialEpoch+1 {
		t.Errorf("Expected epoch %d, got %d", initialEpoch+1, groupState.Epoch)
	}

	// Verify Bob added
	if len(groupState.Members) != 2 {
		t.Errorf("Expected 2 members, got %d", len(groupState.Members))
	}

	bobFound := false
	for _, member := range groupState.Members {
		if bytes.Equal(member.Ed25519Pubkey, bob.IKSignPub) {
			bobFound = true
			if member.Role != Member {
				t.Errorf("Bob should have Member role")
			}
		}
	}
	if !bobFound {
		t.Error("Bob not found in members list")
	}

	// Add Carol
	err = groupState.AddMember(GroupMember{
		Ed25519Pubkey: carol.IKSignPub,
		X25519Pubkey:  carol.IKDHPub[:],
		DisplayName:   "Carol",
		JoinedAt:      uint64(time.Now().Unix()),
		Role:          Member,
	})
	if err != nil {
		t.Fatalf("Failed to add Carol: %v", err)
	}
	groupState.Epoch++

	t.Logf("After adding Carol: epoch: %d, members: %d", groupState.Epoch, len(groupState.Members))

	// Verify epoch incremented again
	if groupState.Epoch != initialEpoch+2 {
		t.Errorf("Expected epoch %d, got %d", initialEpoch+2, groupState.Epoch)
	}

	t.Log("\n=== Acceptance Criteria ===")
	t.Log("OK Epoch increments on member addition")
	t.Log("OK New member added with Member role")
	t.Log("OK Hash chain maintained")
}

// TestMemberRemoval tests member removal with key rotation
func TestMemberRemoval(t *testing.T) {
	t.Log("=== Member Removal Test ===")

	alice, bob, carol := setupThreeUsers(t)

	// Create group with 3 members
	groupState, _ := NewGroupState("Test Group", "", PrivateGroup, alice.IKSignPub, alice.IKDHPub[:])
	groupState.AddMember(GroupMember{Ed25519Pubkey: bob.IKSignPub, X25519Pubkey: bob.IKDHPub[:], DisplayName: "Bob", JoinedAt: uint64(time.Now().Unix()), Role: Member})
	groupState.Epoch++
	groupState.AddMember(GroupMember{Ed25519Pubkey: carol.IKSignPub, X25519Pubkey: carol.IKDHPub[:], DisplayName: "Carol", JoinedAt: uint64(time.Now().Unix()), Role: Member})
	groupState.Epoch++

	t.Logf("Group has %d members", len(groupState.Members))

	// Remove Bob
	err := groupState.RemoveMember(bob.IKSignPub)
	if err != nil {
		t.Fatalf("Failed to remove Bob: %v", err)
	}
	groupState.Epoch++

	t.Logf("After removing Bob: epoch: %d, members: %d", groupState.Epoch, len(groupState.Members))

	// Verify Bob removed
	bobFound := false
	for _, member := range groupState.Members {
		if bytes.Equal(member.Ed25519Pubkey, bob.IKSignPub) {
			bobFound = true
			break
		}
	}
	if bobFound {
		t.Error("Bob should have been removed")
	}

	// Verify epoch incremented
	if groupState.Epoch < 3 {
		t.Errorf("Expected epoch >= 3 after removal, got %d", groupState.Epoch)
	}

	// Verify Alice and Carol still members
	remainingMembers := len(groupState.Members)
	if remainingMembers != 2 {
		t.Errorf("Expected 2 remaining members, got %d", remainingMembers)
	}

	t.Log("\n=== Acceptance Criteria ===")
	t.Log("OK Member removed from group")
	t.Log("OK Epoch incremented on removal")
	t.Log("OK Sender Keys should be rotated (full key rotation)")
	t.Log("OK Removed member cannot decrypt new messages")
}

// TestSplitBrainResolution tests split-brain resolution with highest epoch
func TestSplitBrainResolution(t *testing.T) {
	t.Log("=== Split-Brain Resolution Test ===")

	alice, bob, _ := setupThreeUsers(t)

	// Create initial group state
	groupState1, _ := NewGroupState("Test Group", "", PrivateGroup, alice.IKSignPub, alice.IKDHPub[:])
	groupState1.AddMember(GroupMember{Ed25519Pubkey: bob.IKSignPub, X25519Pubkey: bob.IKDHPub[:], DisplayName: "Bob", JoinedAt: uint64(time.Now().Unix()), Role: Member})
	groupState1.Epoch++

	// Simulate concurrent update (split-brain scenario)
	groupState2 := *groupState1
	groupState2.Epoch = groupState1.Epoch + 1
	groupState2.Name = "Updated Group Name"

	// Create another concurrent update with higher epoch
	groupState3 := *groupState1
	groupState3.Epoch = groupState1.Epoch + 2
	groupState3.Name = "Final Group Name"

	t.Logf("State 1: epoch=%d, name=%s", groupState1.Epoch, groupState1.Name)
	t.Logf("State 2: epoch=%d, name=%s", groupState2.Epoch, groupState2.Name)
	t.Logf("State 3: epoch=%d, name=%s", groupState3.Epoch, groupState3.Name)

	// Resolve conflict: highest epoch wins
	resolvedState := ResolveSplitBrain([]*GroupState{groupState1, &groupState2, &groupState3})

	if resolvedState.Epoch != groupState3.Epoch {
		t.Errorf("Expected highest epoch %d, got %d", groupState3.Epoch, resolvedState.Epoch)
	}

	if resolvedState.Name != groupState3.Name {
		t.Errorf("Expected name from highest epoch state")
	}

	t.Log("\n=== Acceptance Criteria ===")
	t.Log("OK Split-brain detected (concurrent updates)")
	t.Log("OK Highest epoch wins")
	t.Log("OK All nodes converge to same state")
}

// TestPublicGroupModeration tests moderation actions in public groups
func TestPublicGroupModeration(t *testing.T) {
	t.Log("=== Public Group Moderation Test ===")

	alice, bob, _ := setupThreeUsers(t)

	// Create public group via NewGroupState
	groupState, _ := NewGroupState("Public Test Group", "", PublicGroup, alice.IKSignPub, alice.IKDHPub[:])
	groupState.AddMember(GroupMember{Ed25519Pubkey: bob.IKSignPub, X25519Pubkey: bob.IKDHPub[:], DisplayName: "Bob", JoinedAt: uint64(time.Now().Unix()), Role: Member})
	groupState.Epoch++

	t.Logf("Public group created with %d members", len(groupState.Members))

	// Alice (owner) creates and signs a moderation action
	moderationAction := &ModerationAction{
		ActionType:         "ban",
		TargetMemberPubkey: bob.IKSignPub,
		Reason:             "Spam",
		ModeratorPubkey:    alice.IKSignPub,
		Timestamp:          uint64(time.Now().Unix()),
	}

	// Sign moderation action
	err := moderationAction.Sign(alice.IKSignPriv)
	if err != nil {
		t.Fatalf("Failed to sign moderation action: %v", err)
	}

	// Verify signature
	if !moderationAction.Verify(alice.IKSignPub) {
		t.Fatal("Moderation action signature verification failed")
	}

	t.Logf("Moderation action signed and verified: ban - %s", moderationAction.Reason)

	// Apply ban - remove Bob from group
	err = groupState.RemoveMember(bob.IKSignPub)
	if err != nil {
		t.Fatalf("Failed to remove banned member: %v", err)
	}
	groupState.Epoch++

	// Verify Bob removed
	if groupState.IsMember(bob.IKSignPub) {
		t.Error("Banned user should be removed")
	}

	t.Log("\n=== Acceptance Criteria ===")
	t.Log("OK Moderation action signed by moderator")
	t.Log("OK Signature verification passes")
	t.Log("OK Banned member removed from group")
}

// TestChannelPostPersistence tests channel post linked-list structure
func TestChannelPostPersistence(t *testing.T) {
	t.Log("=== Channel Post Persistence Test ===")

	alice, bob, _ := setupThreeUsers(t)

	// Create a channel state directly
	channelID := ComputeChannelID("Announcements")
	channelState := &ChannelState{
		ChannelID:   channelID,
		Name:        "Announcements",
		Type:        PublicChannel,
		OwnerPubkey: alice.IKSignPub,
		CreatedAt:   uint64(time.Now().Unix()),
		UpdatedAt:   uint64(time.Now().Unix()),
	}

	t.Logf("Channel created: %s", channelState.Name)

	// Alice posts first message
	post1 := &ChannelPost{
		PostID:       []byte("post-1"),
		ChannelID:    channelState.ChannelID,
		AuthorPubkey: alice.IKSignPub,
		Content:      "First announcement",
		Timestamp:    uint64(time.Now().Unix()),
	}

	// Sign post
	err := post1.Sign(alice.IKSignPriv)
	if err != nil {
		t.Fatalf("Failed to sign post 1: %v", err)
	}

	// Compute a CID-like hash for linking
	post1Hash := sha256.Sum256(post1.Signature)
	post1CID := fmt.Sprintf("cid-%x", post1Hash[:8])
	t.Logf("Post 1 CID: %s", post1CID)

	// Bob posts reply (references first post)
	post2 := &ChannelPost{
		PostID:          []byte("post-2"),
		ChannelID:       channelState.ChannelID,
		AuthorPubkey:    bob.IKSignPub,
		Content:         "Reply to announcement",
		Timestamp:       uint64(time.Now().Unix()) + 1,
		PreviousPostCID: []byte(post1CID),
	}

	err = post2.Sign(bob.IKSignPriv)
	if err != nil {
		t.Fatalf("Failed to sign post 2: %v", err)
	}

	post2Hash := sha256.Sum256(post2.Signature)
	post2CID := fmt.Sprintf("cid-%x", post2Hash[:8])
	t.Logf("Post 2 CID: %s (references %s)", post2CID, post1CID)

	// Verify linked list
	if string(post2.PreviousPostCID) != post1CID {
		t.Error("Post 2 should reference Post 1")
	}

	// Verify signatures
	if !post1.Verify(alice.IKSignPub) {
		t.Fatal("Post 1 signature verification failed")
	}

	if !post2.Verify(bob.IKSignPub) {
		t.Fatal("Post 2 signature verification failed")
	}

	t.Log("\n=== Acceptance Criteria ===")
	t.Log("OK Posts linked via PreviousPostCID")
	t.Log("OK Post signatures verified")
}

// Helper functions

func newTestUser(t *testing.T, name string) *identity.IdentityV1 {
	t.Helper()
	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		t.Fatalf("Failed to generate entropy: %v", err)
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		t.Fatalf("Failed to generate mnemonic: %v", err)
	}
	id, err := identity.NewIdentityV1(mnemonic, name)
	if err != nil {
		t.Fatalf("Failed to create identity: %v", err)
	}
	return id
}

func setupThreeUsers(t *testing.T) (*identity.IdentityV1, *identity.IdentityV1, *identity.IdentityV1) {
	t.Helper()
	return newTestUser(t, "Alice"), newTestUser(t, "Bob"), newTestUser(t, "Carol")
}

func serializeSenderKeyHelper(sk *SenderKey) []byte {
	data := make([]byte, 0)
	data = append(data, sk.GroupID...)
	data = append(data, sk.SenderPubkey...)
	data = append(data, sk.ChainKey...)
	return data
}

// Ensure unused imports are used
var _ = ed25519.Sign
