package rtc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"babylontower/pkg/crypto"
	"babylontower/pkg/identity"
	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"

	"github.com/google/uuid"
	"github.com/ipfs/go-log/v2"
	"google.golang.org/protobuf/proto"
)

var logger = log.Logger("babylontower/rtc")

// Call state constants
const (
	CallStateIdle       = "idle"
	CallStateOffered    = "offered"
	CallStateAccepted   = "accepted"
	CallStateConnecting = "connecting"
	CallStateActive     = "active"
	CallStateEnded      = "ended"
)

// Call type constants
const (
	CallTypeAudio = "audio"
	CallTypeVideo = "video"
)

// Hangup reasons
const (
	HangupNormal   = "normal"
	HangupBusy     = "busy"
	HangupDeclined = "declined"
	HangupTimeout  = "timeout"
	HangupError    = "error"
)

// Call timeouts
const (
	OfferTimeout     = 60 * time.Second // Offer expires after 60 seconds
	ICEGatherTimeout = 30 * time.Second // ICE gathering timeout
	NoMediaTimeout   = 15 * time.Second // No media timeout after answer
)

var (
	// ErrCallNotFound is returned when a call session doesn't exist
	ErrCallNotFound = errors.New("call not found")
	// ErrCallAlreadyExists is returned when trying to start a call that already exists
	ErrCallAlreadyExists = errors.New("call already exists")
	// ErrInvalidCallState is returned when an operation is invalid for the current call state
	ErrInvalidCallState = errors.New("invalid call state")
	// ErrCallExpired is returned when a call offer has expired
	ErrCallExpired = errors.New("call offer expired")
)

// CallSession represents an active or pending call session
type CallSession struct {
	CallID         string
	LocalIdentity  []byte // Our IK_sign.pub
	RemoteIdentity []byte // Remote IK_sign.pub
	CallType       string // "audio" or "video"
	State          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ExpiresAt      time.Time // For offers: when the offer expires

	// SDP information
	LocalSDP  string
	RemoteSDP string

	// ICE candidates
	LocalICECandidates  []string
	RemoteICECandidates []string

	// Media session
	MediaKey   []byte // Derived media encryption key
	SSRCLocal  uint32 // Our SSRC
	SSRCRemote uint32 // Remote SSRC

	// Timing
	ConnectedAt  *time.Time // When the call became active
	EndedAt      *time.Time // When the call ended
	HangupReason string
}

// SessionManager manages RTC call sessions
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*CallSession // key: call_id

	storage  storage.Storage
	identity *identity.Identity

	// Callbacks
	onOfferReceived  func(session *CallSession)
	onAnswerReceived func(session *CallSession)
	onICECandidate   func(session *CallSession, candidate string)
	onCallEnded      func(session *CallSession)
	onStateChanged   func(session *CallSession, oldState, newState string)

	ctx    context.Context
	cancel context.CancelFunc
}

// NewSessionManager creates a new RTC session manager
func NewSessionManager(storage storage.Storage, id *identity.Identity) *SessionManager {
	ctx, cancel := context.WithCancel(context.Background())

	sm := &SessionManager{
		sessions: make(map[string]*CallSession),
		storage:  storage,
		identity: id,
		ctx:      ctx,
		cancel:   cancel,
	}

	// Start cleanup goroutine for expired offers
	go sm.cleanupExpiredOffers()

	return sm
}

// cleanupExpiredOffers periodically removes expired call offers
func (sm *SessionManager) cleanupExpiredOffers() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-ticker.C:
			sm.mu.Lock()
			now := time.Now()
			for callID, session := range sm.sessions {
				if session.State == CallStateOffered && now.After(session.ExpiresAt) {
					logger.Debugw("cleaning up expired call offer", "call", callID)
					session.State = CallStateEnded
					session.HangupReason = HangupTimeout
					session.EndedAt = &now
					delete(sm.sessions, callID)

					if sm.onCallEnded != nil {
						sm.onCallEnded(session)
					}
				}
			}
			sm.mu.Unlock()
		}
	}
}

// CreateOutgoingCall creates a new outgoing call session
func (sm *SessionManager) CreateOutgoingCall(remoteIdentity []byte, callType string) (*CallSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if there's already an active call with this peer
	for _, session := range sm.sessions {
		if session.RemoteIdentity != nil &&
			hex.EncodeToString(session.RemoteIdentity) == hex.EncodeToString(remoteIdentity) &&
			session.State != CallStateEnded {
			return nil, ErrCallAlreadyExists
		}
	}

	callID := uuid.New().String()
	now := time.Now()

	session := &CallSession{
		CallID:         callID,
		LocalIdentity:  sm.identity.Ed25519PubKey,
		RemoteIdentity: remoteIdentity,
		CallType:       callType,
		State:          CallStateOffered,
		CreatedAt:      now,
		UpdatedAt:      now,
		ExpiresAt:      now.Add(OfferTimeout),
		SSRCLocal:      generateSSRC(),
	}

	sm.sessions[callID] = session

	logger.Debugw("created outgoing call session", "call", callID, "type", callType)

	return session, nil
}

// CreateIncomingCall creates a new incoming call session
func (sm *SessionManager) CreateIncomingCall(callID string, remoteIdentity []byte, callType string, sdp string) (*CallSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if call already exists
	if _, exists := sm.sessions[callID]; exists {
		return nil, ErrCallAlreadyExists
	}

	now := time.Now()

	session := &CallSession{
		CallID:         callID,
		LocalIdentity:  sm.identity.Ed25519PubKey,
		RemoteIdentity: remoteIdentity,
		CallType:       callType,
		State:          CallStateOffered,
		CreatedAt:      now,
		UpdatedAt:      now,
		ExpiresAt:      now.Add(OfferTimeout),
		RemoteSDP:      sdp,
		SSRCRemote:     generateSSRC(),
		SSRCLocal:      generateSSRC(),
	}

	sm.sessions[callID] = session

	logger.Debugw("created incoming call session", "call", callID, "type", callType, "from", hex.EncodeToString(remoteIdentity)[:16])

	return session, nil
}

// GetSession retrieves a call session by ID
func (sm *SessionManager) GetSession(callID string) (*CallSession, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[callID]
	if !exists {
		return nil, ErrCallNotFound
	}

	return session, nil
}

// GetActiveSessionWithPeer retrieves an active call session with a specific peer
func (sm *SessionManager) GetActiveSessionWithPeer(remoteIdentity []byte) (*CallSession, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	remoteHex := hex.EncodeToString(remoteIdentity)

	for _, session := range sm.sessions {
		if session.RemoteIdentity != nil &&
			hex.EncodeToString(session.RemoteIdentity) == remoteHex &&
			session.State != CallStateEnded {
			return session, nil
		}
	}

	return nil, ErrCallNotFound
}

// GetAllActiveSessions returns all active call sessions
func (sm *SessionManager) GetAllActiveSessions() []*CallSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var active []*CallSession
	for _, session := range sm.sessions {
		if session.State != CallStateEnded {
			active = append(active, session)
		}
	}

	return active
}

// UpdateState updates the state of a call session
func (sm *SessionManager) UpdateState(callID string, newState string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[callID]
	if !exists {
		return ErrCallNotFound
	}

	oldState := session.State

	switch newState {
	case CallStateAccepted:
		if session.State != CallStateOffered {
			return fmt.Errorf("%w: cannot accept call in state %s", ErrInvalidCallState, session.State)
		}
		session.State = CallStateAccepted

	case CallStateConnecting:
		if session.State != CallStateAccepted && session.State != CallStateOffered {
			return fmt.Errorf("%w: cannot connect call in state %s", ErrInvalidCallState, session.State)
		}
		session.State = CallStateConnecting

	case CallStateActive:
		if session.State != CallStateConnecting && session.State != CallStateAccepted {
			return fmt.Errorf("%w: cannot activate call in state %s", ErrInvalidCallState, session.State)
		}
		now := time.Now()
		session.ConnectedAt = &now
		session.State = CallStateActive

	case CallStateEnded:
		if session.State == CallStateEnded {
			return nil // Already ended
		}
		now := time.Now()
		session.EndedAt = &now
		session.State = CallStateEnded

	default:
		session.State = newState
	}

	session.UpdatedAt = time.Now()

	logger.Debugw("call state changed", "call", callID, "old_state", oldState, "new_state", newState)

	if sm.onStateChanged != nil {
		sm.onStateChanged(session, oldState, newState)
	}

	return nil
}

// SetLocalSDP sets the local SDP offer/answer
func (sm *SessionManager) SetLocalSDP(callID string, sdp string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[callID]
	if !exists {
		return ErrCallNotFound
	}

	session.LocalSDP = sdp
	session.UpdatedAt = time.Now()

	return nil
}

// SetRemoteSDP sets the remote SDP offer/answer
func (sm *SessionManager) SetRemoteSDP(callID string, sdp string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[callID]
	if !exists {
		return ErrCallNotFound
	}

	session.RemoteSDP = sdp
	session.UpdatedAt = time.Now()

	return nil
}

// AddLocalICECandidate adds a local ICE candidate
func (sm *SessionManager) AddLocalICECandidate(callID string, candidate string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[callID]
	if !exists {
		return ErrCallNotFound
	}

	session.LocalICECandidates = append(session.LocalICECandidates, candidate)
	session.UpdatedAt = time.Now()

	if sm.onICECandidate != nil {
		sm.onICECandidate(session, candidate)
	}

	return nil
}

// AddRemoteICECandidate adds a remote ICE candidate
func (sm *SessionManager) AddRemoteICECandidate(callID string, candidate string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[callID]
	if !exists {
		return ErrCallNotFound
	}

	session.RemoteICECandidates = append(session.RemoteICECandidates, candidate)
	session.UpdatedAt = time.Now()

	return nil
}

// DeriveMediaKey derives the media encryption key from the session root key
// This binds the media encryption to the messaging session
func (sm *SessionManager) DeriveMediaKey(callID string, sessionRootKey []byte) ([]byte, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[callID]
	if !exists {
		return nil, ErrCallNotFound
	}

	// media_key = HKDF(session_root_key, salt=call_id, info="bt-media-v1", len=32)
	mediaKey, err := crypto.DeriveKey(
		sessionRootKey,
		[]byte(callID),
		[]byte("bt-media-v1"),
		32,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to derive media key: %w", err)
	}

	session.MediaKey = mediaKey

	logger.Debugw("derived media key", "call", callID)

	return mediaKey, nil
}

// EndCall ends a call session with the specified reason
func (sm *SessionManager) EndCall(callID string, reason string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[callID]
	if !exists {
		return ErrCallNotFound
	}

	if session.State == CallStateEnded {
		return nil // Already ended
	}

	now := time.Now()
	session.EndedAt = &now
	session.HangupReason = reason
	session.State = CallStateEnded
	session.UpdatedAt = now

	logger.Debugw("call ended", "call", callID, "reason", reason)

	if sm.onCallEnded != nil {
		sm.onCallEnded(session)
	}

	return nil
}

// DeleteSession removes a call session from the manager
func (sm *SessionManager) DeleteSession(callID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.sessions[callID]; !exists {
		return ErrCallNotFound
	}

	delete(sm.sessions, callID)
	logger.Debugw("deleted call session", "call", callID)

	return nil
}

// SetOnOfferReceived sets the callback for when an offer is received
func (sm *SessionManager) SetOnOfferReceived(callback func(session *CallSession)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.onOfferReceived = callback
}

// SetOnAnswerReceived sets the callback for when an answer is received
func (sm *SessionManager) SetOnAnswerReceived(callback func(session *CallSession)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.onAnswerReceived = callback
}

// SetOnICECandidate sets the callback for when an ICE candidate is ready
func (sm *SessionManager) SetOnICECandidate(callback func(session *CallSession, candidate string)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.onICECandidate = callback
}

// SetOnCallEnded sets the callback for when a call ends
func (sm *SessionManager) SetOnCallEnded(callback func(session *CallSession)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.onCallEnded = callback
}

// SetOnStateChanged sets the callback for when call state changes
func (sm *SessionManager) SetOnStateChanged(callback func(session *CallSession, oldState, newState string)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.onStateChanged = callback
}

// Stop gracefully stops the session manager
func (sm *SessionManager) Stop() {
	sm.cancel()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// End all active calls
	for _, session := range sm.sessions {
		if session.State != CallStateEnded {
			now := time.Now()
			session.EndedAt = &now
			session.HangupReason = HangupNormal
			session.State = CallStateEnded
		}
	}

	logger.Info("RTC session manager stopped")
}

// generateSSRC generates a random SSRC value
func generateSSRC() uint32 {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback to time-based generation
		return uint32(time.Now().UnixNano() & 0xFFFFFFFF)
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// ToRTCOffer converts a CallSession to an RTCOffer protobuf message
func (s *CallSession) ToRTCOffer() *pb.RTCOffer {
	return &pb.RTCOffer{
		Sdp:    s.LocalSDP,
		CallId: s.CallID,
		Video:  s.CallType == CallTypeVideo,
	}
}

// ToRTCAnswer converts a CallSession to an RTCAnswer protobuf message
func (s *CallSession) ToRTCAnswer() *pb.RTCAnswer {
	return &pb.RTCAnswer{
		Sdp:    s.LocalSDP,
		CallId: s.CallID,
	}
}

// ToRTCIceCandidate creates an RTCIceCandidate protobuf message
func (s *CallSession) ToRTCIceCandidate(candidate string, sdpMid string, mlineIdx uint32) *pb.RTCIceCandidate {
	return &pb.RTCIceCandidate{
		Candidate:   candidate,
		SdpMid:      sdpMid,
		SdpMlineIdx: mlineIdx,
		CallId:      s.CallID,
	}
}

// ToRTCHangup creates an RTCHangup protobuf message
func (s *CallSession) ToRTCHangup(reason string) *pb.RTCHangup {
	return &pb.RTCHangup{
		CallId: s.CallID,
		Reason: reason,
	}
}

// Duration returns the duration of the call
func (s *CallSession) Duration() time.Duration {
	if s.ConnectedAt == nil {
		return 0
	}

	endTime := time.Now()
	if s.EndedAt != nil {
		endTime = *s.EndedAt
	}

	return endTime.Sub(*s.ConnectedAt)
}

// IsExpired returns true if the call offer has expired
func (s *CallSession) IsExpired() bool {
	return s.State == CallStateOffered && time.Now().After(s.ExpiresAt)
}

// Marshal serializes a CallSession for storage
func (s *CallSession) Marshal() ([]byte, error) {
	var connectedAt, endedAt uint64
	if s.ConnectedAt != nil {
		connectedAt = uint64(s.ConnectedAt.Unix())
	}
	if s.EndedAt != nil {
		endedAt = uint64(s.EndedAt.Unix())
	}

	return proto.Marshal(&pb.CallSession{
		CallId:              s.CallID,
		LocalIdentity:       s.LocalIdentity,
		RemoteIdentity:      s.RemoteIdentity,
		CallType:            s.CallType,
		State:               s.State,
		CreatedAt:           uint64(s.CreatedAt.Unix()),
		UpdatedAt:           uint64(s.UpdatedAt.Unix()),
		ExpiresAt:           uint64(s.ExpiresAt.Unix()),
		LocalSdp:            s.LocalSDP,
		RemoteSdp:           s.RemoteSDP,
		LocalIceCandidates:  s.LocalICECandidates,
		RemoteIceCandidates: s.RemoteICECandidates,
		MediaKey:            s.MediaKey,
		SsrcLocal:           s.SSRCLocal,
		SsrcRemote:          s.SSRCRemote,
		ConnectedAt:         connectedAt,
		EndedAt:             endedAt,
		HangupReason:        s.HangupReason,
	})
}

// UnmarshalCallSession deserializes a CallSession from storage
func UnmarshalCallSession(data []byte) (*CallSession, error) {
	pbSession := &pb.CallSession{}
	if err := proto.Unmarshal(data, pbSession); err != nil {
		return nil, err
	}

	session := &CallSession{
		CallID:              pbSession.CallId,
		LocalIdentity:       pbSession.LocalIdentity,
		RemoteIdentity:      pbSession.RemoteIdentity,
		CallType:            pbSession.CallType,
		State:               pbSession.State,
		CreatedAt:           unixTime(pbSession.CreatedAt),
		UpdatedAt:           unixTime(pbSession.UpdatedAt),
		ExpiresAt:           unixTime(pbSession.ExpiresAt),
		LocalSDP:            pbSession.LocalSdp,
		RemoteSDP:           pbSession.RemoteSdp,
		LocalICECandidates:  pbSession.LocalIceCandidates,
		RemoteICECandidates: pbSession.RemoteIceCandidates,
		MediaKey:            pbSession.MediaKey,
		SSRCLocal:           pbSession.SsrcLocal,
		SSRCRemote:          pbSession.SsrcRemote,
		HangupReason:        pbSession.HangupReason,
	}

	if pbSession.ConnectedAt > 0 {
		t := unixTime(pbSession.ConnectedAt)
		session.ConnectedAt = &t
	}

	if pbSession.EndedAt > 0 {
		t := unixTime(pbSession.EndedAt)
		session.EndedAt = &t
	}

	return session, nil
}

func unixTime(secs uint64) time.Time {
	return time.Unix(int64(secs), 0)
}
