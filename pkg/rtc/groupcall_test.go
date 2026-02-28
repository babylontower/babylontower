package rtc

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

	"babylontower/pkg/identity"
	pb "babylontower/pkg/proto"
	"google.golang.org/protobuf/proto"
)

// testMnemonic is a fixed mnemonic for testing
const testMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

// createTestIdentity creates a test identity
func createTestIdentity(t *testing.T) *identity.Identity {
	id, err := identity.NewIdentity(testMnemonic)
	if err != nil {
		t.Fatalf("Failed to create test identity: %v", err)
	}
	return id
}

// MockMessagingService is a mock implementation of MessagingService
type MockMessagingService struct{}

func (m *MockMessagingService) PublishToGroup(groupID []byte, message proto.Message) error {
	return nil
}

func (m *MockMessagingService) PublishTo(identityPubkey []byte, message proto.Message) error {
	return nil
}

// TestCreateGroupCall tests creating a new group call
func TestCreateGroupCall(t *testing.T) {
	id := createTestIdentity(t)
	messaging := &MockMessagingService{}

	gcm, err := NewGroupCallManager(id, messaging, nil, DefaultGroupCallConfig())
	if err != nil {
		t.Fatalf("Failed to create group call manager: %v", err)
	}

	groupID := sha256.Sum256([]byte("test-group"))
	session, err := gcm.CreateGroupCall(groupID[:], false)
	if err != nil {
		t.Fatalf("Failed to create group call: %v", err)
	}

	if session.CallId == "" {
		t.Error("Expected non-empty call ID")
	}

	if !bytes.Equal(session.GroupId, groupID[:]) {
		t.Error("Group ID mismatch")
	}

	if session.CallType != pb.GroupCallType_GROUP_CALL_TYPE_MESH {
		t.Error("Expected initial topology to be MESH")
	}

	if len(session.Participants) != 1 {
		t.Errorf("Expected 1 participant (owner), got %d", len(session.Participants))
	}

	if !session.Participants[0].IsOwner {
		t.Error("Expected first participant to be owner")
	}
}

// TestJoinGroupCall tests joining a group call
func TestJoinGroupCall(t *testing.T) {
	id := createTestIdentity(t)
	messaging := &MockMessagingService{}
	

	gcm, err := NewGroupCallManager(id, messaging, nil, DefaultGroupCallConfig())
	if err != nil {
		t.Fatalf("Failed to create group call manager: %v", err)
	}

	groupID := sha256.Sum256([]byte("test-group"))
	session, err := gcm.CreateGroupCall(groupID[:], false)
	if err != nil {
		t.Fatalf("Failed to create group call: %v", err)
	}

	// Join the call - owner is already in the call, so this is a no-op
	session2, err := gcm.JoinGroupCall(session.CallId, "TestUser")
	if err != nil {
		t.Fatalf("Failed to join group call: %v", err)
	}

	// Owner is already in the call, so count should still be 1
	if session2.GetParticipantCount() != 1 {
		t.Errorf("Expected 1 participant (owner already in call), got %d", session2.GetParticipantCount())
	}
}

// TestLeaveGroupCall tests leaving a group call
func TestLeaveGroupCall(t *testing.T) {
	id := createTestIdentity(t)
	messaging := &MockMessagingService{}
	

	gcm, err := NewGroupCallManager(id, messaging, nil, DefaultGroupCallConfig())
	if err != nil {
		t.Fatalf("Failed to create group call manager: %v", err)
	}

	groupID := sha256.Sum256([]byte("test-group"))
	session, err := gcm.CreateGroupCall(groupID[:], false)
	if err != nil {
		t.Fatalf("Failed to create group call: %v", err)
	}

	// Join and leave
	_, err = gcm.JoinGroupCall(session.CallId, "TestUser")
	if err != nil {
		t.Fatalf("Failed to join: %v", err)
	}

	err = gcm.LeaveGroupCall(session.CallId, "test_reason")
	if err != nil {
		t.Fatalf("Failed to leave: %v", err)
	}
}

// TestEndGroupCall tests ending a group call (owner only)
func TestEndGroupCall(t *testing.T) {
	id := createTestIdentity(t)
	messaging := &MockMessagingService{}
	

	gcm, err := NewGroupCallManager(id, messaging, nil, DefaultGroupCallConfig())
	if err != nil {
		t.Fatalf("Failed to create group call manager: %v", err)
	}

	groupID := sha256.Sum256([]byte("test-group"))
	session, err := gcm.CreateGroupCall(groupID[:], false)
	if err != nil {
		t.Fatalf("Failed to create group call: %v", err)
	}

	// End the call
	err = gcm.EndGroupCall(session.CallId, "test_ended")
	if err != nil {
		t.Fatalf("Failed to end call: %v", err)
	}

	if session.State != pb.GroupCallState_GROUP_CALL_STATE_ENDED {
		t.Errorf("Expected state ENDED, got %s", session.State.String())
	}

	if session.HangupReason != "test_ended" {
		t.Errorf("Expected reason 'test_ended', got '%s'", session.HangupReason)
	}
}

// TestGetParticipant tests retrieving a participant
func TestGetParticipant(t *testing.T) {
	id := createTestIdentity(t)
	messaging := &MockMessagingService{}
	

	gcm, err := NewGroupCallManager(id, messaging, nil, DefaultGroupCallConfig())
	if err != nil {
		t.Fatalf("Failed to create group call manager: %v", err)
	}

	groupID := sha256.Sum256([]byte("test-group"))
	session, err := gcm.CreateGroupCall(groupID[:], false)
	if err != nil {
		t.Fatalf("Failed to create group call: %v", err)
	}

	// Get owner participant
	owner, err := session.GetParticipant(id.Ed25519PubKey)
	if err != nil {
		t.Fatalf("Failed to get owner participant: %v", err)
	}

	if !owner.IsOwner {
		t.Error("Expected owner participant")
	}
}

// TestIsOwner tests owner verification
func TestIsOwner(t *testing.T) {
	id := createTestIdentity(t)
	messaging := &MockMessagingService{}
	

	gcm, err := NewGroupCallManager(id, messaging, nil, DefaultGroupCallConfig())
	if err != nil {
		t.Fatalf("Failed to create group call manager: %v", err)
	}

	groupID := sha256.Sum256([]byte("test-group"))
	session, err := gcm.CreateGroupCall(groupID[:], false)
	if err != nil {
		t.Fatalf("Failed to create group call: %v", err)
	}

	if !session.IsOwner(id.Ed25519PubKey) {
		t.Error("Expected identity to be owner")
	}

	if session.IsOwner([]byte("non-existent")) {
		t.Error("Did not expect non-existent identity to be owner")
	}
}

// TestDuration tests call duration calculation
func TestDuration(t *testing.T) {
	id := createTestIdentity(t)
	messaging := &MockMessagingService{}
	

	gcm, err := NewGroupCallManager(id, messaging, nil, DefaultGroupCallConfig())
	if err != nil {
		t.Fatalf("Failed to create group call manager: %v", err)
	}

	groupID := sha256.Sum256([]byte("test-group"))
	session, err := gcm.CreateGroupCall(groupID[:], false)
	if err != nil {
		t.Fatalf("Failed to create group call: %v", err)
	}

	// Set started time
	session.mu.Lock()
	session.StartedAt = uint64(time.Now().Add(-1 * time.Minute).Unix())
	session.mu.Unlock()

	duration := session.Duration()
	if duration < time.Minute {
		t.Errorf("Expected duration >= 1 minute, got %v", duration)
	}
}

// TestMeshTopology tests mesh topology mode
func TestMeshTopology(t *testing.T) {
	id := createTestIdentity(t)
	messaging := &MockMessagingService{}
	

	gcm, err := NewGroupCallManager(id, messaging, nil, DefaultGroupCallConfig())
	if err != nil {
		t.Fatalf("Failed to create group call manager: %v", err)
	}

	groupID := sha256.Sum256([]byte("test-group"))
	session, err := gcm.CreateGroupCall(groupID[:], false)
	if err != nil {
		t.Fatalf("Failed to create group call: %v", err)
	}

	if session.CallType != pb.GroupCallType_GROUP_CALL_TYPE_MESH {
		t.Error("Expected MESH topology")
	}

	if session.IsSFU() {
		t.Error("Did not expect SFU mode")
	}
}

// TestDeriveGroupCallTopic tests topic derivation
func TestDeriveGroupCallTopic(t *testing.T) {
	groupID := []byte("test-group-id")
	topic := deriveGroupCallTopic(groupID)

	expectedPrefix := "babylon-grpcall-"
	if len(topic) <= len(expectedPrefix) {
		t.Errorf("Expected topic to have suffix, got '%s'", topic)
	}

	if topic[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Expected prefix '%s', got '%s'", expectedPrefix, topic[:len(expectedPrefix)])
	}
}

// TestComputeDeviceID tests device ID computation
func TestComputeDeviceID(t *testing.T) {
	identityPubkey := []byte("test-pubkey-32-bytes-long-test")
	deviceID := computeDeviceID(identityPubkey)

	if len(deviceID) != 16 {
		t.Errorf("Expected device ID to be 16 bytes, got %d", len(deviceID))
	}
}

// TestGroupCallSessionMethods tests GroupCallSession helper methods
func TestGroupCallSessionMethods(t *testing.T) {
	id := createTestIdentity(t)
	session := &GroupCallSession{
		GroupCallSession: pb.GroupCallSession{
			CallId:        "test-call",
			OwnerIdentity: id.Ed25519PubKey,
			CallType:      pb.GroupCallType_GROUP_CALL_TYPE_SFU,
			SfuIdentity:   id.Ed25519PubKey,
			Participants: []*pb.ParticipantInfo{
				{
					IdentityPubkey: id.Ed25519PubKey,
					IsOwner:        true,
					IsSfu:          true,
				},
			},
		},
	}

	if !session.IsSFU() {
		t.Error("Expected SFU mode")
	}

	if !session.IsOwner(id.Ed25519PubKey) {
		t.Error("Expected to be owner")
	}

	if !session.IsSFUParticipant(id.Ed25519PubKey) {
		t.Error("Expected to be SFU")
	}
}
