package rtc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"babylontower/pkg/crypto"
	bterrors "babylontower/pkg/errors"
	"babylontower/pkg/identity"
	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

// logger is declared in session.go for this package

// Group call constants
const (
	// MeshTopologyMaxParticipants is the maximum participants for mesh mode
	MeshTopologyMaxParticipants = 6
	// SFUTopologyMinParticipants is the minimum participants to switch to SFU
	SFUTopologyMinParticipants = 7
	// SFUTopologyMaxParticipants is the maximum participants for SFU mode
	SFUTopologyMaxParticipants = 25
	// SFUElectionTimeout is the timeout for SFU election
	SFUElectionTimeout = 5 * time.Second
	// ParticipantTimeout is the timeout for participant activity
	ParticipantTimeout = 30 * time.Second
)

// GroupCallSession extends CallSession for group calls
type GroupCallSession struct {
	pb.GroupCallSession

	mu sync.RWMutex

	// Media streams per participant (key: identity pubkey hex)
	mediaStreams map[string]*MediaStream

	// SFU state (if in SFU mode)
	sfuState *SFUState

	// Callbacks
	onSFUElected func(session *GroupCallSession, sfuIdentity []byte)
}

// SFUState holds the state for SFU mode
type SFUState struct {
	// Media forwarding table (key: sender identity, value: subscriber identities)
	forwardingTable map[string][][]byte

	// SSRC mapping (key: identity, value: SSRC)
	ssrcMap map[string]uint32

	// Media packet buffer (for late-joining participants)
	packetBuffer []*BufferedMediaPacket
}

// BufferedMediaPacket is a buffered media packet
type BufferedMediaPacket struct {
	SenderIdentity []byte
	Payload        []byte
	SSRC           uint32
	Timestamp      uint64
	Sequence       uint32
}

// MediaStream represents a media stream from a participant
type MediaStream struct {
	SSRC       uint32
	TrackType  string // "audio" or "video"
	LastActive time.Time
}

// GroupCallManager manages group calls
type GroupCallManager struct {
	identity  *identity.Identity
	messaging MessagingService
	storage   storage.Storage

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu sync.RWMutex

	// Active group calls (key: call_id)
	calls map[string]*GroupCallSession

	// Group call topic subscriptions (key: group_id)
	subscriptions map[string]Subscription

	// Configuration
	config *GroupCallConfig
}

// MessagingService interface for sending group call messages
type MessagingService interface {
	PublishToGroup(groupID []byte, message proto.Message) error
	PublishTo(identity []byte, message proto.Message) error
}

// Subscription represents a PubSub subscription
type Subscription interface {
	Messages() <-chan *PubSubMessage
	Cancel()
}

// PubSubMessage represents a PubSub message
type PubSubMessage struct {
	Data       []byte
	From       string
	ReceivedAt time.Time
}

// GroupCallConfig holds configuration for group calls
type GroupCallConfig struct {
	// AutoSwitchToSFU automatically switches to SFU mode when participants exceed threshold
	AutoSwitchToSFU bool
	// MaxParticipants is the maximum participants in a group call
	MaxParticipants int
	// EnableVideo enables video in group calls
	EnableVideo bool
	// MediaBufferDuration is how long to buffer media packets
	MediaBufferDuration time.Duration
	// ParticipantInactivityTimeout is the timeout for inactive participants
	ParticipantInactivityTimeout time.Duration
}

// DefaultGroupCallConfig returns the default group call configuration
func DefaultGroupCallConfig() *GroupCallConfig {
	return &GroupCallConfig{
		AutoSwitchToSFU:              true,
		MaxParticipants:              SFUTopologyMaxParticipants,
		EnableVideo:                  true,
		MediaBufferDuration:          5 * time.Second,
		ParticipantInactivityTimeout: ParticipantTimeout,
	}
}

// NewGroupCallManager creates a new group call manager
func NewGroupCallManager(
	id *identity.Identity,
	messaging MessagingService,
	store storage.Storage,
	config *GroupCallConfig,
) (*GroupCallManager, error) {
	if config == nil {
		config = DefaultGroupCallConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	gcm := &GroupCallManager{
		identity:      id,
		messaging:     messaging,
		storage:       store,
		ctx:           ctx,
		cancel:        cancel,
		calls:         make(map[string]*GroupCallSession),
		subscriptions: make(map[string]Subscription),
		config:        config,
	}

	return gcm, nil
}

// Start begins the group call manager
func (gcm *GroupCallManager) Start() error {
	logger.Info("Group call manager started")
	return nil
}

// Stop gracefully stops the group call manager
func (gcm *GroupCallManager) Stop() {
	gcm.mu.Lock()
	defer gcm.mu.Unlock()

	// End all active group calls
	for _, call := range gcm.calls {
		gcm.endCallInternal(call, "manager_stopped")
	}

	// Cancel all subscriptions
	for _, sub := range gcm.subscriptions {
		sub.Cancel()
	}

	gcm.cancel()
	gcm.wg.Wait()

	logger.Info("Group call manager stopped")
}

// CreateGroupCall creates a new group call
func (gcm *GroupCallManager) CreateGroupCall(
	groupID []byte,
	isVideo bool,
) (*GroupCallSession, error) {
	gcm.mu.Lock()
	defer gcm.mu.Unlock()

	callID := uuid.New().String()
	now := uint64(time.Now().Unix())

	// Determine initial topology (start with mesh)
	callType := pb.GroupCallType_GROUP_CALL_TYPE_MESH

	session := &GroupCallSession{
		GroupCallSession: pb.GroupCallSession{
			CallId:          callID,
			GroupId:         groupID,
			CallType:        callType,
			State:           pb.GroupCallState_GROUP_CALL_STATE_INITIATING,
			OwnerIdentity:   gcm.identity.Ed25519PubKey,
			Participants:    []*pb.ParticipantInfo{},
			CreatedAt:       now,
			IsVideo:         isVideo,
			MaxParticipants: uint64(gcm.config.MaxParticipants),
		},
		mediaStreams: make(map[string]*MediaStream),
		sfuState: &SFUState{
			forwardingTable: make(map[string][][]byte),
			ssrcMap:         make(map[string]uint32),
			packetBuffer:    []*BufferedMediaPacket{},
		},
	}

	// Add owner as first participant
	ownerParticipant := &pb.ParticipantInfo{
		IdentityPubkey: gcm.identity.Ed25519PubKey,
		DeviceId:       computeDeviceID(gcm.identity.Ed25519PubKey),
		DisplayName:    "Owner",
		State:          pb.ParticipantState_PARTICIPANT_STATE_JOINING,
		JoinedAt:       now,
		IsOwner:        true,
		IsSfu:          false,
		Ssrc:           generateSSRC(),
	}
	session.Participants = append(session.Participants, ownerParticipant)
	session.sfuState.ssrcMap[hex.EncodeToString(gcm.identity.Ed25519PubKey)] = ownerParticipant.Ssrc

	gcm.calls[callID] = session

	// Subscribe to group call topic
	if err := gcm.subscribeToGroupCallTopic(groupID); err != nil {
		logger.Warnw("failed to subscribe to group call topic", "error", err)
	}

	logger.Infow("created group call", "call", callID, "group", hex.EncodeToString(groupID)[:16], "type", callType.String())

	return session, nil
}

// JoinGroupCall joins an existing group call
func (gcm *GroupCallManager) JoinGroupCall(callID string, displayName string) (*GroupCallSession, error) {
	gcm.mu.Lock()
	session, exists := gcm.calls[callID]
	gcm.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("group call not found: %s", callID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Check if already a participant
	for _, p := range session.Participants {
		if bytes.Equal(p.IdentityPubkey, gcm.identity.Ed25519PubKey) {
			return session, nil // Already joined
		}
	}

	// Check participant limit
	if uint64(len(session.Participants)) >= session.MaxParticipants {
		return nil, fmt.Errorf("group call is full (max %d participants)", session.MaxParticipants)
	}

	now := uint64(time.Now().Unix())

	// Create participant info
	participant := &pb.ParticipantInfo{
		IdentityPubkey: gcm.identity.Ed25519PubKey,
		DeviceId:       computeDeviceID(gcm.identity.Ed25519PubKey),
		DisplayName:    displayName,
		State:          pb.ParticipantState_PARTICIPANT_STATE_JOINING,
		JoinedAt:       now,
		IsOwner:        false,
		IsSfu:          false,
		Ssrc:           generateSSRC(),
	}

	session.Participants = append(session.Participants, participant)
	session.sfuState.ssrcMap[hex.EncodeToString(gcm.identity.Ed25519PubKey)] = participant.Ssrc

	// Check if we need to switch to SFU mode
	if gcm.config.AutoSwitchToSFU && len(session.Participants) >= SFUTopologyMinParticipants {
		logger.Infow("switching group call to SFU mode", "call", callID, "participants", len(session.Participants))
		session.CallType = pb.GroupCallType_GROUP_CALL_TYPE_SFU
		gcm.runSFUElection(session)
	}

	// Send join message to group
	joinMsg := &pb.GroupCallJoin{
		CallId:              callID,
		ParticipantIdentity: gcm.identity.Ed25519PubKey,
		DeviceId:            participant.DeviceId,
		DisplayName:         displayName,
		Timestamp:           now,
	}

	// Sign the join message
	data, err := proto.Marshal(joinMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal join message: %w", err)
	}
	signature, err := crypto.Sign(gcm.identity.Ed25519PrivKey, data)
	if err != nil {
		return nil, fmt.Errorf("failed to sign join message: %w", err)
	}
	joinMsg.Signature = signature

	if err := gcm.publishToGroupCallTopic(session.GroupId, joinMsg); err != nil {
		logger.Warnw("failed to publish join message", "error", err)
	}

	logger.Infow("joined group call", "call", callID, "display_name", displayName)

	return session, nil
}

// LeaveGroupCall leaves a group call
func (gcm *GroupCallManager) LeaveGroupCall(callID string, reason string) error {
	gcm.mu.Lock()
	session, exists := gcm.calls[callID]
	gcm.mu.Unlock()

	if !exists {
		return fmt.Errorf("group call not found: %s", callID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Find and remove our participant entry
	var ourParticipant *pb.ParticipantInfo
	for i, p := range session.Participants {
		if bytes.Equal(p.IdentityPubkey, gcm.identity.Ed25519PubKey) {
			ourParticipant = p
			session.Participants = append(session.Participants[:i], session.Participants[i+1:]...)
			break
		}
	}

	if ourParticipant == nil {
		return nil // Already left
	}

	// Send leave message
	leaveMsg := &pb.GroupCallLeave{
		CallId:              callID,
		ParticipantIdentity: gcm.identity.Ed25519PubKey,
		Reason:              reason,
		Timestamp:           uint64(time.Now().Unix()),
	}

	signature, err := func() ([]byte, error) {
		data, err := proto.Marshal(leaveMsg)
		if err != nil {
			return nil, err
		}
		return crypto.Sign(gcm.identity.Ed25519PrivKey, data)
	}()
	if err != nil {
		logger.Warnw("failed to sign leave message", "error", err)
	}
	leaveMsg.Signature = signature

	if err := gcm.publishToGroupCallTopic(session.GroupId, leaveMsg); err != nil {
		logger.Warnw("failed to publish leave message", "error", err)
	}

	logger.Infow("left group call", "call", callID, "reason", reason)

	// If we were the SFU, trigger re-election
	if ourParticipant.IsSfu {
		gcm.runSFUElection(session)
	}

	return nil
}

// EndGroupCall ends a group call (owner only)
func (gcm *GroupCallManager) EndGroupCall(callID string, reason string) error {
	gcm.mu.Lock()
	session, exists := gcm.calls[callID]
	gcm.mu.Unlock()

	if !exists {
		return fmt.Errorf("group call not found: %s", callID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Only owner can end the call
	if !session.Participants[0].IsOwner || !bytes.Equal(session.Participants[0].IdentityPubkey, gcm.identity.Ed25519PubKey) {
		return fmt.Errorf("only the call owner can end the call")
	}

	gcm.endCallInternal(session, reason)

	return nil
}

// endCallInternal ends a group call and cleans up
func (gcm *GroupCallManager) endCallInternal(session *GroupCallSession, reason string) {
	session.State = pb.GroupCallState_GROUP_CALL_STATE_ENDED
	session.HangupReason = reason
	session.EndedAt = uint64(time.Now().Unix())

	// Unsubscribe from group call topic
	if err := gcm.unsubscribeFromGroupCallTopic(session.GroupId); err != nil {
		logger.Warnw("failed to unsubscribe from group call topic", "error", err)
	}

	// Remove from active calls
	delete(gcm.calls, session.CallId)

	logger.Infow("ended group call", "call", session.CallId, "reason", reason, "duration", time.Duration(session.EndedAt-session.StartedAt)*time.Second)
}

// GetGroupCall retrieves a group call session
func (gcm *GroupCallManager) GetGroupCall(callID string) (*GroupCallSession, error) {
	gcm.mu.RLock()
	defer gcm.mu.RUnlock()

	session, exists := gcm.calls[callID]
	if !exists {
		return nil, fmt.Errorf("group call not found: %s", callID)
	}

	return session, nil
}

// GetAllGroupCalls returns all active group calls
func (gcm *GroupCallManager) GetAllGroupCalls() []*GroupCallSession {
	gcm.mu.RLock()
	defer gcm.mu.RUnlock()

	calls := make([]*GroupCallSession, 0, len(gcm.calls))
	for _, call := range gcm.calls {
		calls = append(calls, call)
	}

	return calls
}

// runSFUElection runs an SFU election among participants
func (gcm *GroupCallManager) runSFUElection(session *GroupCallSession) {
	if len(session.Participants) == 0 {
		return
	}

	// SFU election: lowest lexicographic pubkey wins
	sort.Slice(session.Participants, func(i, j int) bool {
		return hex.EncodeToString(session.Participants[i].IdentityPubkey) <
			hex.EncodeToString(session.Participants[j].IdentityPubkey)
	})

	// First participant (lowest pubkey) becomes SFU candidate
	candidate := session.Participants[0]

	logger.Infow("running SFU election", "call", session.CallId, "candidate", hex.EncodeToString(candidate.IdentityPubkey)[:16])

	// Sign the election message
	electionMsg := &pb.GroupCallSFUElection{
		CallId:            session.CallId,
		CandidateIdentity: candidate.IdentityPubkey,
		Timestamp:         uint64(time.Now().Unix()),
	}

	signature, err := func() ([]byte, error) {
		data, err := proto.Marshal(electionMsg)
		if err != nil {
			return nil, err
		}
		return crypto.Sign(gcm.identity.Ed25519PrivKey, data)
	}()
	if err != nil {
		logger.Warnw("failed to sign SFU election message", "error", err)
		return
	}
	electionMsg.Signature = signature

	if err := gcm.publishToGroupCallTopic(session.GroupId, electionMsg); err != nil {
		logger.Warnw("failed to publish SFU election message", "error", err)
		return
	}

	// Start election timeout
	bterrors.SafeGo("rtc-sfu-election", func() {
		select {
		case <-time.After(SFUElectionTimeout):
			session.mu.Lock()
			defer session.mu.Unlock()

			// Confirm election
			candidate.IsSfu = true
			session.SfuIdentity = candidate.IdentityPubkey

			logger.Infow("SFU elected", "call", session.CallId, "sfu", hex.EncodeToString(candidate.IdentityPubkey)[:16])

			if session.onSFUElected != nil {
				session.onSFUElected(session, candidate.IdentityPubkey)
			}

			// Broadcast state update
			gcm.broadcastStateUpdate(session)
		case <-gcm.ctx.Done():
			return
		}
	})
}

// broadcastStateUpdate broadcasts the current group call state
func (gcm *GroupCallManager) broadcastStateUpdate(session *GroupCallSession) {
	stateUpdate := &pb.GroupCallStateUpdate{
		CallId:       session.CallId,
		State:        session.State,
		Participants: session.Participants,
		CallType:     session.CallType,
		SfuIdentity:  session.SfuIdentity,
		Timestamp:    uint64(time.Now().Unix()),
	}

	signature, err := func() ([]byte, error) {
		data, err := proto.Marshal(stateUpdate)
		if err != nil {
			return nil, err
		}
		return crypto.Sign(gcm.identity.Ed25519PrivKey, data)
	}()
	if err != nil {
		logger.Warnw("failed to sign state update", "error", err)
		return
	}
	stateUpdate.Signature = signature

	if err := gcm.publishToGroupCallTopic(session.GroupId, stateUpdate); err != nil {
		logger.Warnw("failed to broadcast state update", "error", err)
	}
}

// subscribeToGroupCallTopic subscribes to a group call's PubSub topic
func (gcm *GroupCallManager) subscribeToGroupCallTopic(groupID []byte) error {
	topic := deriveGroupCallTopic(groupID)

	// Check if already subscribed
	if _, exists := gcm.subscriptions[topic]; exists {
		return nil
	}

	// In a real implementation, this would subscribe to the actual PubSub topic
	// For now, we just track the subscription
	gcm.subscriptions[topic] = &mockSubscription{
		messages: make(chan *PubSubMessage, 100),
	}

	logger.Debugw("subscribed to group call topic", "topic", topic)
	return nil
}

// unsubscribeFromGroupCallTopic unsubscribes from a group call's PubSub topic
func (gcm *GroupCallManager) unsubscribeFromGroupCallTopic(groupID []byte) error {
	topic := deriveGroupCallTopic(groupID)

	sub, exists := gcm.subscriptions[topic]
	if !exists {
		return nil
	}

	sub.Cancel()
	delete(gcm.subscriptions, topic)

	logger.Debugw("unsubscribed from group call topic", "topic", topic)
	return nil
}

// publishToGroupCallTopic publishes a message to a group call's PubSub topic
func (gcm *GroupCallManager) publishToGroupCallTopic(groupID []byte, message proto.Message) error {
	// In a real implementation, this would publish to the actual PubSub topic
	// For now, we just log
	logger.Debugw("published to group call topic", "topic", deriveGroupCallTopic(groupID))
	return nil
}

// deriveGroupCallTopic derives a PubSub topic from a group ID
func deriveGroupCallTopic(groupID []byte) string {
	hash := sha256.Sum256(groupID)
	return fmt.Sprintf("babylon-grpcall-%s", hex.EncodeToString(hash[:8]))
}

// computeDeviceID computes a device ID from an identity pubkey
func computeDeviceID(identityPubkey []byte) []byte {
	hash := sha256.Sum256(identityPubkey)
	return hash[:16]
}

// mockSubscription is a mock PubSub subscription for testing
type mockSubscription struct {
	messages chan *PubSubMessage
}

func (m *mockSubscription) Messages() <-chan *PubSubMessage {
	return m.messages
}

func (m *mockSubscription) Cancel() {
	close(m.messages)
}

// Callback setters

// SetOnParticipantJoined sets the callback for when a participant joins
func (gcm *GroupCallManager) SetOnParticipantJoined(callback func(session *GroupCallSession, participant *pb.ParticipantInfo)) {
	gcm.mu.Lock()
	defer gcm.mu.Unlock()
	// In a real implementation, this would set callbacks on all active sessions
}

// SetOnParticipantLeft sets the callback for when a participant leaves
func (gcm *GroupCallManager) SetOnParticipantLeft(callback func(session *GroupCallSession, participant *pb.ParticipantInfo, reason string)) {
	gcm.mu.Lock()
	defer gcm.mu.Unlock()
}

// SetOnSFUElected sets the callback for when an SFU is elected
func (gcm *GroupCallManager) SetOnSFUElected(callback func(session *GroupCallSession, sfuIdentity []byte)) {
	gcm.mu.Lock()
	defer gcm.mu.Unlock()
}

// SetOnStateChanged sets the callback for when group call state changes
func (gcm *GroupCallManager) SetOnStateChanged(callback func(session *GroupCallSession, oldState, newState pb.GroupCallState)) {
	gcm.mu.Lock()
	defer gcm.mu.Unlock()
}

// SetOnMediaPacketReceived sets the callback for when a media packet is received
func (gcm *GroupCallManager) SetOnMediaPacketReceived(callback func(session *GroupCallSession, senderIdentity []byte, packet []byte)) {
	gcm.mu.Lock()
	defer gcm.mu.Unlock()
}

// Helper functions

// GetParticipantCount returns the number of participants in a group call
func (session *GroupCallSession) GetParticipantCount() int {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return len(session.Participants)
}

// IsSFU returns true if the group call is in SFU mode
func (session *GroupCallSession) IsSFU() bool {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.CallType == pb.GroupCallType_GROUP_CALL_TYPE_SFU
}

// IsOwner returns true if the given identity is the call owner
func (session *GroupCallSession) IsOwner(identityPubkey []byte) bool {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return bytes.Equal(session.OwnerIdentity, identityPubkey)
}

// IsSFU returns true if the given identity is the SFU
func (session *GroupCallSession) IsSFUParticipant(identityPubkey []byte) bool {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return bytes.Equal(session.SfuIdentity, identityPubkey)
}

// GetParticipant retrieves a participant by identity
func (session *GroupCallSession) GetParticipant(identityPubkey []byte) (*pb.ParticipantInfo, error) {
	session.mu.RLock()
	defer session.mu.RUnlock()

	for _, p := range session.Participants {
		if bytes.Equal(p.IdentityPubkey, identityPubkey) {
			return p, nil
		}
	}

	return nil, fmt.Errorf("participant not found")
}

// Duration returns the duration of the group call
func (session *GroupCallSession) Duration() time.Duration {
	session.mu.RLock()
	defer session.mu.RUnlock()

	if session.StartedAt == 0 {
		return 0
	}

	endTime := uint64(time.Now().Unix())
	if session.EndedAt > 0 {
		endTime = session.EndedAt
	}

	return time.Duration(endTime-session.StartedAt) * time.Second
}

// gcmLogger alias removed — using package-level logger from session.go
