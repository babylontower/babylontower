package rtc

import (
	"testing"
	"time"

	"babylontower/pkg/identity"
	"babylontower/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestIdentity(t *testing.T) *identity.Identity {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	id, err := identity.NewIdentity(mnemonic)
	require.NoError(t, err)
	return id
}

func TestSessionManager_CreateOutgoingCall(t *testing.T) {
	// Setup
	store := storage.NewMemoryStorage()
	id := newTestIdentity(t)

	sessionMgr := NewSessionManager(store, id)
	defer sessionMgr.Stop()

	// Create outgoing call
	remoteIdentity := []byte("remote_peer_identity_12345678901234567890")
	session, err := sessionMgr.CreateOutgoingCall(remoteIdentity, CallTypeAudio)

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, session.CallID)
	assert.Equal(t, CallStateOffered, session.State)
	assert.Equal(t, CallTypeAudio, session.CallType)
	assert.Equal(t, CallTypeAudio, session.CallType)
	assert.WithinDuration(t, time.Now(), session.CreatedAt, time.Second)
	assert.WithinDuration(t, time.Now(), session.ExpiresAt, OfferTimeout+time.Second)
}

func TestSessionManager_CreateIncomingCall(t *testing.T) {
	// Setup
	store := storage.NewMemoryStorage()
	id := newTestIdentity(t)

	sessionMgr := NewSessionManager(store, id)
	defer sessionMgr.Stop()

	// Create incoming call
	callID := "test-call-id-12345"
	remoteIdentity := []byte("remote_peer_identity_12345678901234567890")
	sdp := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\n"

	session, err := sessionMgr.CreateIncomingCall(callID, remoteIdentity, CallTypeVideo, sdp)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, callID, session.CallID)
	assert.Equal(t, CallStateOffered, session.State)
	assert.Equal(t, CallTypeVideo, session.CallType)
	assert.Equal(t, sdp, session.RemoteSDP)
}

func TestSessionManager_GetSession(t *testing.T) {
	// Setup
	store := storage.NewMemoryStorage()
	id := newTestIdentity(t)

	sessionMgr := NewSessionManager(store, id)
	defer sessionMgr.Stop()

	remoteIdentity := []byte("remote_peer_identity_12345678901234567890")
	session, err := sessionMgr.CreateOutgoingCall(remoteIdentity, CallTypeAudio)
	require.NoError(t, err)

	// Get existing session
	retrieved, err := sessionMgr.GetSession(session.CallID)
	require.NoError(t, err)
	assert.Equal(t, session.CallID, retrieved.CallID)

	// Get non-existent session
	_, err = sessionMgr.GetSession("non-existent-call-id")
	assert.Error(t, err)
	assert.Equal(t, ErrCallNotFound, err)
}

func TestSessionManager_UpdateState(t *testing.T) {
	// Setup
	store := storage.NewMemoryStorage()
	id := newTestIdentity(t)

	sessionMgr := NewSessionManager(store, id)
	defer sessionMgr.Stop()

	remoteIdentity := []byte("remote_peer_identity_12345678901234567890")
	session, err := sessionMgr.CreateOutgoingCall(remoteIdentity, CallTypeAudio)
	require.NoError(t, err)

	// Test valid state transitions
	err = sessionMgr.UpdateState(session.CallID, CallStateAccepted)
	assert.NoError(t, err)
	assert.Equal(t, CallStateAccepted, session.State)

	err = sessionMgr.UpdateState(session.CallID, CallStateConnecting)
	assert.NoError(t, err)
	assert.Equal(t, CallStateConnecting, session.State)

	err = sessionMgr.UpdateState(session.CallID, CallStateActive)
	assert.NoError(t, err)
	assert.Equal(t, CallStateActive, session.State)

	// Test invalid state transition - can't accept an active call
	err = sessionMgr.UpdateState(session.CallID, CallStateAccepted)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid call state")
}

func TestSessionManager_EndCall(t *testing.T) {
	// Setup
	store := storage.NewMemoryStorage()
	id := newTestIdentity(t)

	sessionMgr := NewSessionManager(store, id)
	defer sessionMgr.Stop()

	remoteIdentity := []byte("remote_peer_identity_12345678901234567890")
	session, err := sessionMgr.CreateOutgoingCall(remoteIdentity, CallTypeAudio)
	require.NoError(t, err)

	// End call
	err = sessionMgr.EndCall(session.CallID, HangupNormal)
	require.NoError(t, err)

	// Assert state
	assert.Equal(t, CallStateEnded, session.State)
	assert.Equal(t, HangupNormal, session.HangupReason)
	assert.NotNil(t, session.EndedAt)
}

func TestSessionManager_DeriveMediaKey(t *testing.T) {
	// Setup
	store := storage.NewMemoryStorage()
	id := newTestIdentity(t)

	sessionMgr := NewSessionManager(store, id)
	defer sessionMgr.Stop()

	remoteIdentity := []byte("remote_peer_identity_12345678901234567890")
	session, err := sessionMgr.CreateOutgoingCall(remoteIdentity, CallTypeAudio)
	require.NoError(t, err)

	// Derive media key (simulated root key)
	rootKey := []byte("simulated_root_key_32_bytes_long!")
	mediaKey, err := sessionMgr.DeriveMediaKey(session.CallID, rootKey)

	// Assert
	require.NoError(t, err)
	assert.Len(t, mediaKey, 32)
	assert.Equal(t, mediaKey, session.MediaKey)

	// Derive again - should produce same key
	mediaKey2, err := sessionMgr.DeriveMediaKey(session.CallID, rootKey)
	require.NoError(t, err)
	assert.Equal(t, mediaKey, mediaKey2)
}

func TestSessionManager_GetActiveSessionWithPeer(t *testing.T) {
	// Setup
	store := storage.NewMemoryStorage()
	id := newTestIdentity(t)

	sessionMgr := NewSessionManager(store, id)
	defer sessionMgr.Stop()

	remoteIdentity := []byte("remote_peer_identity_12345678901234567890")
	session, err := sessionMgr.CreateOutgoingCall(remoteIdentity, CallTypeAudio)
	require.NoError(t, err)

	// Get active session with peer
	retrieved, err := sessionMgr.GetActiveSessionWithPeer(remoteIdentity)
	require.NoError(t, err)
	assert.Equal(t, session.CallID, retrieved.CallID)

	// End the call
	err = sessionMgr.EndCall(session.CallID, HangupNormal)
	require.NoError(t, err)

	// Should not find active session anymore
	_, err = sessionMgr.GetActiveSessionWithPeer(remoteIdentity)
	assert.Error(t, err)
	assert.Equal(t, ErrCallNotFound, err)
}

func TestSessionManager_GetAllActiveSessions(t *testing.T) {
	// Setup
	store := storage.NewMemoryStorage()
	id := newTestIdentity(t)

	sessionMgr := NewSessionManager(store, id)
	defer sessionMgr.Stop()

	// Create multiple calls
	remote1 := []byte("remote_peer_1_identity_1234567890123456789")
	remote2 := []byte("remote_peer_2_identity_1234567890123456789")
	remote3 := []byte("remote_peer_3_identity_1234567890123456789")

	session1, _ := sessionMgr.CreateOutgoingCall(remote1, CallTypeAudio)
	session2, _ := sessionMgr.CreateOutgoingCall(remote2, CallTypeVideo)
	session3, _ := sessionMgr.CreateOutgoingCall(remote3, CallTypeAudio)

	// End one call
	_ = sessionMgr.EndCall(session3.CallID, HangupNormal)

	// Get all active sessions
	active := sessionMgr.GetAllActiveSessions()
	require.Len(t, active, 2)

	// Verify both active sessions are returned
	callIDs := map[string]bool{
		session1.CallID: true,
		session2.CallID: true,
	}

	for _, s := range active {
		assert.True(t, callIDs[s.CallID])
		assert.NotEqual(t, CallStateEnded, s.State)
	}
}

func TestSessionManager_CallExpiration(t *testing.T) {
	// Setup
	store := storage.NewMemoryStorage()
	id := newTestIdentity(t)

	sessionMgr := NewSessionManager(store, id)
	defer sessionMgr.Stop()

	remoteIdentity := []byte("remote_peer_identity_12345678901234567890")
	session, err := sessionMgr.CreateOutgoingCall(remoteIdentity, CallTypeAudio)
	require.NoError(t, err)

	// Manually expire the offer
	session.ExpiresAt = time.Now().Add(-1 * time.Second)

	// Wait for cleanup goroutine to run
	time.Sleep(15 * time.Second)

	// Session should be cleaned up
	_, err = sessionMgr.GetSession(session.CallID)
	assert.Error(t, err)
	assert.Equal(t, ErrCallNotFound, err)
}

func TestCallSession_MarshalUnmarshal(t *testing.T) {
	// Create a call session
	now := time.Now()
	connectedAt := now.Add(10 * time.Second)
	endedAt := now.Add(100 * time.Second)

	session := &CallSession{
		CallID:         "test-call-id",
		LocalIdentity:  []byte("local_identity_123456789012345678901"),
		RemoteIdentity: []byte("remote_identity_123456789012345678901"),
		CallType:       CallTypeVideo,
		State:          CallStateEnded,
		CreatedAt:      now,
		UpdatedAt:      now,
		ExpiresAt:      now.Add(60 * time.Second),
		LocalSDP:       "v=0\r\no=local...",
		RemoteSDP:      "v=0\r\no=remote...",
		MediaKey:       []byte("media_key_32_bytes_long_test!!"),
		SSRCLocal:      12345,
		SSRCRemote:     67890,
		ConnectedAt:    &connectedAt,
		EndedAt:        &endedAt,
		HangupReason:   HangupNormal,
	}

	// Marshal
	data, err := session.Marshal()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Unmarshal
	unmarshaled, err := UnmarshalCallSession(data)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, session.CallID, unmarshaled.CallID)
	assert.Equal(t, session.CallType, unmarshaled.CallType)
	assert.Equal(t, session.State, unmarshaled.State)
	assert.Equal(t, session.HangupReason, unmarshaled.HangupReason)
	assert.Equal(t, session.SSRCLocal, unmarshaled.SSRCLocal)
	assert.Equal(t, session.SSRCRemote, unmarshaled.SSRCRemote)
	assert.Equal(t, session.MediaKey, unmarshaled.MediaKey)
}

func TestCallSession_Duration(t *testing.T) {
	now := time.Now()
	connectedAt := now.Add(-100 * time.Second)
	endedAt := now.Add(-10 * time.Second)

	// Active call (no end time)
	activeSession := &CallSession{
		ConnectedAt: &connectedAt,
	}
	activeDuration := activeSession.Duration()
	assert.GreaterOrEqual(t, activeDuration, 100*time.Second)

	// Ended call
	endedSession := &CallSession{
		ConnectedAt: &connectedAt,
		EndedAt:     &endedAt,
	}
	endedDuration := endedSession.Duration()
	assert.InDelta(t, 90.0, endedDuration.Seconds(), 1.0)

	// Not connected call
	notConnectedSession := &CallSession{}
	assert.Equal(t, time.Duration(0), notConnectedSession.Duration())
}

func TestCallSession_IsExpired(t *testing.T) {
	now := time.Now()

	// Expired offer
	expiredSession := &CallSession{
		State:     CallStateOffered,
		ExpiresAt: now.Add(-10 * time.Second),
	}
	assert.True(t, expiredSession.IsExpired())

	// Valid offer
	validSession := &CallSession{
		State:     CallStateOffered,
		ExpiresAt: now.Add(10 * time.Second),
	}
	assert.False(t, validSession.IsExpired())

	// Active call (not expired)
	activeSession := &CallSession{
		State:     CallStateActive,
		ExpiresAt: now.Add(-10 * time.Second), // Expired but active
	}
	assert.False(t, activeSession.IsExpired())
}

func TestSessionManager_Callbacks(t *testing.T) {
	// Setup
	store := storage.NewMemoryStorage()
	id := newTestIdentity(t)

	sessionMgr := NewSessionManager(store, id)
	defer sessionMgr.Stop()

	// Track callback invocations
	stateChanges := 0
	callsEnded := 0

	sessionMgr.SetOnStateChanged(func(session *CallSession, oldState, newState string) {
		stateChanges++
	})

	sessionMgr.SetOnCallEnded(func(session *CallSession) {
		callsEnded++
	})

	// Create and end a call
	remoteIdentity := []byte("remote_peer_identity_12345678901234567890")
	session, err := sessionMgr.CreateOutgoingCall(remoteIdentity, CallTypeAudio)
	require.NoError(t, err)

	_ = sessionMgr.UpdateState(session.CallID, CallStateAccepted)
	_ = sessionMgr.UpdateState(session.CallID, CallStateActive)
	_ = sessionMgr.EndCall(session.CallID, HangupNormal)

	// Assert callbacks were invoked
	assert.Greater(t, stateChanges, 0)
	assert.Equal(t, 1, callsEnded)
}

func TestGenerateSSRC(t *testing.T) {
	// Generate multiple SSRCs and ensure uniqueness
	ssrcs := make(map[uint32]bool)
	for i := 0; i < 100; i++ {
		ssrc := generateSSRC()
		assert.False(t, ssrcs[ssrc], "Duplicate SSRC generated")
		ssrcs[ssrc] = true
	}
}
