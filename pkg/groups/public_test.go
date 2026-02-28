package groups

import (
	"testing"
	"time"

	"babylontower/pkg/storage"
	"golang.org/x/crypto/ed25519"
)

func setupTestPublicGroupService(t *testing.T) (*PublicGroupService, ed25519.PublicKey, ed25519.PrivateKey) {
	// Generate identity keys
	pubKey, privKey, _ := ed25519.GenerateKey(nil)

	// Create storage
	stor := storage.NewMemoryStorage()

	// Create service (using interface)
	service := NewPublicGroupService(stor, pubKey, privKey)

	return service, pubKey, privKey
}

func TestCreatePublicGroup(t *testing.T) {
	service, _, _ := setupTestPublicGroupService(t)

	group, err := service.CreatePublicGroup("Test Group", "A test public group")
	if err != nil {
		t.Fatalf("Failed to create public group: %v", err)
	}

	if group.Type != PublicGroup {
		t.Errorf("Expected group type PublicGroup, got %v", group.Type)
	}

	if group.Name != "Test Group" {
		t.Errorf("Expected name 'Test Group', got '%s'", group.Name)
	}

	if group.Epoch != 1 {
		t.Errorf("Expected initial epoch 1, got %d", group.Epoch)
	}
}

func TestBanMember(t *testing.T) {
	service, _, _ := setupTestPublicGroupService(t)

	// Create group
	group, err := service.CreatePublicGroup("Test Group", "A test public group")
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	// Create a member key
	memberPubKey, _, _ := ed25519.GenerateKey(nil)

	// Add member to group (simulate)
	group.Members = append(group.Members, GroupMember{
		Ed25519Pubkey: memberPubKey,
		Role:          Member,
	})

	// Ban the member
	action, err := service.BanMember(group.GroupID, memberPubKey, "Test ban reason")
	if err != nil {
		t.Fatalf("Failed to ban member: %v", err)
	}

	if action.ActionType != string(ActionBan) {
		t.Errorf("Expected action type ban, got %s", action.ActionType)
	}

	// Verify member is banned
	if !service.IsBanned(group.GroupID, memberPubKey) {
		t.Error("Expected member to be banned")
	}

	// Verify member was removed from group
	if group.IsMember(memberPubKey) {
		t.Error("Expected banned member to be removed from group")
	}
}

func TestMuteMember(t *testing.T) {
	service, _, _ := setupTestPublicGroupService(t)

	// Create group
	group, err := service.CreatePublicGroup("Test Group", "A test public group")
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	// Create a member key
	memberPubKey, _, _ := ed25519.GenerateKey(nil)

	// Mute the member for 60 seconds
	action, err := service.MuteMember(group.GroupID, memberPubKey, "Test mute reason", 60)
	if err != nil {
		t.Fatalf("Failed to mute member: %v", err)
	}

	if action.ActionType != string(ActionMute) {
		t.Errorf("Expected action type mute, got %s", action.ActionType)
	}

	if action.DurationSeconds != 60 {
		t.Errorf("Expected duration 60 seconds, got %d", action.DurationSeconds)
	}

	// Verify member is muted
	if !service.IsMuted(group.GroupID, memberPubKey) {
		t.Error("Expected member to be muted")
	}
}

func TestDeleteMessage(t *testing.T) {
	service, _, _ := setupTestPublicGroupService(t)

	// Create group
	group, err := service.CreatePublicGroup("Test Group", "A test public group")
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	// Create a message ID
	messageID := []byte("test-message-id-12345678")

	// Delete the message
	action, err := service.DeleteMessage(group.GroupID, messageID, "Test delete reason")
	if err != nil {
		t.Fatalf("Failed to delete message: %v", err)
	}

	if action.ActionType != string(ActionDeleteMessage) {
		t.Errorf("Expected action type delete_message, got %s", action.ActionType)
	}

	if string(action.TargetMessageID) != string(messageID) {
		t.Error("Target message ID doesn't match")
	}
}

func TestRateLimiting(t *testing.T) {
	service, _, _ := setupTestPublicGroupService(t)

	// Create group
	group, err := service.CreatePublicGroup("Test Group", "A test public group")
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	// Create a sender key
	senderPubKey, _, _ := ed25519.GenerateKey(nil)

	// Test rate limit: 5 messages per 60 seconds
	maxMessages := uint64(5)
	windowSec := uint64(60)

	// First 5 messages should pass
	for i := 0; i < 5; i++ {
		if !service.CheckRateLimit(group.GroupID, senderPubKey, maxMessages, windowSec) {
			t.Errorf("Message %d should have passed rate limit", i+1)
		}
	}

	// 6th message should be blocked
	if service.CheckRateLimit(group.GroupID, senderPubKey, maxMessages, windowSec) {
		t.Error("6th message should have been rate limited")
	}
}

func TestModerationActionSignature(t *testing.T) {
	_, pubKey, privKey := setupTestPublicGroupService(t)

	action := &ModerationAction{
		TargetMemberPubkey: []byte("target-pubkey"),
		ActionType:         string(ActionBan),
		Reason:             "Test reason",
		Timestamp:          uint64(time.Now().Unix()),
	}

	// Sign the action
	err := action.Sign(privKey)
	if err != nil {
		t.Fatalf("Failed to sign action: %v", err)
	}

	// Verify the signature
	if !action.Verify(pubKey) {
		t.Error("Signature verification failed")
	}

	// Tamper with the action
	action.Reason = "Tampered reason"
	if action.Verify(pubKey) {
		t.Error("Signature should not verify after tampering")
	}
}

func TestProofOfWork(t *testing.T) {
	data := []byte("test-data")
	difficulty := uint8(10)

	// Compute proof of work
	nonce, err := ComputeProofOfWork(data, difficulty)
	if err != nil {
		t.Fatalf("Failed to compute proof of work: %v", err)
	}

	// Verify proof of work
	if !VerifyProofOfWork(data, nonce, difficulty) {
		t.Error("Proof of work verification failed")
	}

	// Verify with tampered data should fail
	tamperedData := []byte("tampered-data")
	if VerifyProofOfWork(tamperedData, nonce, difficulty) {
		t.Error("Proof of work should fail with tampered data")
	}
}

func TestPublicGroupPersistence(t *testing.T) {
	stor := storage.NewMemoryStorage()
	pubKey, privKey, _ := ed25519.GenerateKey(nil)

	service := NewPublicGroupService(stor, pubKey, privKey)

	// Create group
	group, err := service.CreatePublicGroup("Persistent Group", "A persistent group")
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	// Retrieve from storage
	retrieved, err := stor.GetGroup(group.GroupID)
	if err != nil {
		t.Fatalf("Failed to retrieve group: %v", err)
	}

	if string(retrieved.GroupId) != string(group.GroupID) {
		t.Error("Retrieved group ID doesn't match")
	}
}
