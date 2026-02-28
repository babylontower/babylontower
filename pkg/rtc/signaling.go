package rtc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"babylontower/pkg/identity"
	"babylontower/pkg/messaging"
	pb "babylontower/pkg/proto"
	"google.golang.org/protobuf/proto"
)

// logger is declared in session.go for this package

var (
	// ErrSignalingNotStarted is returned when operations are attempted on a stopped signaling service
	ErrSignalingNotStarted = errors.New("RTC signaling service not started")
	// ErrInvalidSignalingMessage is returned when a signaling message is malformed
	ErrInvalidSignalingMessage = errors.New("invalid signaling message")
	// ErrPeerInCall is returned when trying to start a call with a peer already in a call
	ErrPeerInCall = errors.New("peer is already in a call")
)

// SignalingService handles RTC signaling over the messaging protocol
type SignalingService struct {
	identity   *identity.Identity
	messaging  *messaging.Service
	sessionMgr *SessionManager

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu sync.RWMutex

	// Callbacks for signaling events
	onIncomingOffer   func(callID string, remoteIdentity []byte, callType string, sdp string)
	onIncomingAnswer  func(callID string, remoteIdentity []byte, sdp string)
	onIncomingICE     func(callID string, remoteIdentity []byte, candidate string, sdpMid string, mlineIdx uint32)
	onIncomingHangup  func(callID string, remoteIdentity []byte, reason string)
}

// NewSignalingService creates a new RTC signaling service
func NewSignalingService(
	id *identity.Identity,
	msgService *messaging.Service,
	sessionMgr *SessionManager,
) *SignalingService {
	ctx, cancel := context.WithCancel(context.Background())

	return &SignalingService{
		identity:   id,
		messaging:  msgService,
		sessionMgr: sessionMgr,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start begins the signaling service
func (s *SignalingService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	logger.Info("RTC signaling service started")
	return nil
}

// Stop gracefully stops the signaling service
func (s *SignalingService) Stop() {
	s.cancel()
	s.wg.Wait()
	logger.Info("RTC signaling service stopped")
}

// SendOffer initiates a call by sending an RTC offer to a peer
func (s *SignalingService) SendOffer(remoteIdentity []byte, callType string, sdp string) (*CallSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if we already have an active call with this peer
	_, err := s.sessionMgr.GetActiveSessionWithPeer(remoteIdentity)
	if err == nil {
		return nil, ErrPeerInCall
	}

	// Create new call session
	session, err := s.sessionMgr.CreateOutgoingCall(remoteIdentity, callType)
	if err != nil {
		return nil, fmt.Errorf("failed to create call session: %w", err)
	}

	// Set local SDP
	if err := s.sessionMgr.SetLocalSDP(session.CallID, sdp); err != nil {
		return nil, fmt.Errorf("failed to set local SDP: %w", err)
	}

	// Build RTC offer message
	offer := session.ToRTCOffer()
	offerBytes, err := proto.Marshal(offer)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal offer: %w", err)
	}

	// Send via messaging service (will be encrypted with Double Ratchet)
	// The messaging service handles the actual transmission
	if err := s.sendSignalingMessage(remoteIdentity, pb.MessageType_RTC_OFFER, offerBytes); err != nil {
		// Clean up session on send failure
		_ = s.sessionMgr.EndCall(session.CallID, HangupError)
		_ = s.sessionMgr.DeleteSession(session.CallID)
		return nil, fmt.Errorf("failed to send offer: %w", err)
	}

	logger.Infow("sent RTC offer", "remote", hex.EncodeToString(remoteIdentity)[:16], "call", session.CallID, "type", callType)

	return session, nil
}

// SendAnswer sends an RTC answer in response to an offer
func (s *SignalingService) SendAnswer(callID string, sdp string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, err := s.sessionMgr.GetSession(callID)
	if err != nil {
		return ErrCallNotFound
	}

	// Set local SDP
	if err := s.sessionMgr.SetLocalSDP(callID, sdp); err != nil {
		return fmt.Errorf("failed to set local SDP: %w", err)
	}

	// Build RTC answer message
	answer := session.ToRTCAnswer()
	answerBytes, err := proto.Marshal(answer)
	if err != nil {
		return fmt.Errorf("failed to marshal answer: %w", err)
	}

	// Send via messaging service
	if err := s.sendSignalingMessage(session.RemoteIdentity, pb.MessageType_RTC_ANSWER, answerBytes); err != nil {
		return fmt.Errorf("failed to send answer: %w", err)
	}

	logger.Infow("sent RTC answer", "call", callID)

	return nil
}

// SendICECandidate sends an ICE candidate to the remote peer
func (s *SignalingService) SendICECandidate(callID string, candidate string, sdpMid string, mlineIdx uint32) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, err := s.sessionMgr.GetSession(callID)
	if err != nil {
		return ErrCallNotFound
	}

	// Build ICE candidate message
	iceCandidate := session.ToRTCIceCandidate(candidate, sdpMid, mlineIdx)
	iceBytes, err := proto.Marshal(iceCandidate)
	if err != nil {
		return fmt.Errorf("failed to marshal ICE candidate: %w", err)
	}

	// Send via messaging service
	if err := s.sendSignalingMessage(session.RemoteIdentity, pb.MessageType_RTC_ICE_CANDIDATE, iceBytes); err != nil {
		return fmt.Errorf("failed to send ICE candidate: %w", err)
	}

	logger.Debugw("sent ICE candidate", "call", callID)

	return nil
}

// SendHangup sends a hangup message to end a call
func (s *SignalingService) SendHangup(callID string, reason string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, err := s.sessionMgr.GetSession(callID)
	if err != nil {
		return ErrCallNotFound
	}

	// Build hangup message
	hangup := session.ToRTCHangup(reason)
	hangupBytes, err := proto.Marshal(hangup)
	if err != nil {
		return fmt.Errorf("failed to marshal hangup: %w", err)
	}

	// Send via messaging service
	if err := s.sendSignalingMessage(session.RemoteIdentity, pb.MessageType_RTC_HANGUP, hangupBytes); err != nil {
		logger.Warnw("failed to send hangup message", "error", err)
		// Continue with local cleanup even if send fails
	}

	logger.Infow("sent hangup", "call", callID, "reason", reason)

	return nil
}

// sendSignalingMessage sends a signaling message through the messaging service
func (s *SignalingService) sendSignalingMessage(
	recipientPubKey []byte,
	messageType pb.MessageType,
	payload []byte,
) error {
	// The messaging service should handle the actual encryption and transmission
	// This is a placeholder that would integrate with the messaging service's
	// Double Ratchet encryption

	// For now, we'll log the signaling message
	logger.Debugw("sending signaling message", "type", messageType.String(), "remote", hex.EncodeToString(recipientPubKey)[:16])

	// TODO: Integrate with messaging service to send encrypted signaling message
	// The messaging service needs to be extended to support sending raw payloads
	// that are encrypted with the Double Ratchet but not wrapped in DMPayload

	return nil
}

// HandleSignalingMessage processes an incoming signaling message
func (s *SignalingService) HandleSignalingMessage(
	senderIdentity []byte,
	messageType pb.MessageType,
	payload []byte,
) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	switch messageType {
	case pb.MessageType_RTC_OFFER:
		return s.handleOffer(senderIdentity, payload)

	case pb.MessageType_RTC_ANSWER:
		return s.handleAnswer(senderIdentity, payload)

	case pb.MessageType_RTC_ICE_CANDIDATE:
		return s.handleICECandidate(senderIdentity, payload)

	case pb.MessageType_RTC_HANGUP:
		return s.handleHangup(senderIdentity, payload)

	default:
		return fmt.Errorf("%w: unknown RTC message type %s",
			ErrInvalidSignalingMessage, messageType.String())
	}
}

// handleOffer processes an incoming RTC offer
func (s *SignalingService) handleOffer(senderIdentity []byte, payload []byte) error {
	offer := &pb.RTCOffer{}
	if err := proto.Unmarshal(payload, offer); err != nil {
		return fmt.Errorf("%w: failed to unmarshal offer: %w", ErrInvalidSignalingMessage, err)
	}

	callType := CallTypeAudio
	if offer.Video {
		callType = CallTypeVideo
	}

	// Create incoming call session
	_, err := s.sessionMgr.CreateIncomingCall(
		offer.CallId,
		senderIdentity,
		callType,
		offer.Sdp,
	)
	if err != nil {
		return fmt.Errorf("failed to create incoming call session: %w", err)
	}

	logger.Infow("received RTC offer", "from", hex.EncodeToString(senderIdentity)[:16], "call", offer.CallId, "type", callType)

	// Notify callback
	if s.onIncomingOffer != nil {
		s.onIncomingOffer(offer.CallId, senderIdentity, callType, offer.Sdp)
	}

	return nil
}

// handleIncomingAnswer processes an incoming RTC answer
func (s *SignalingService) handleAnswer(senderIdentity []byte, payload []byte) error {
	answer := &pb.RTCAnswer{}
	if err := proto.Unmarshal(payload, answer); err != nil {
		return fmt.Errorf("%w: failed to unmarshal answer: %w", ErrInvalidSignalingMessage, err)
	}

	// Verify the answer matches an existing call
	_, err := s.sessionMgr.GetSession(answer.CallId)
	if err != nil {
		return fmt.Errorf("%w: answer for unknown call", ErrCallNotFound)
	}

	logger.Infow("received RTC answer", "call", answer.CallId)

	// Notify callback
	if s.onIncomingAnswer != nil {
		s.onIncomingAnswer(answer.CallId, senderIdentity, answer.Sdp)
	}

	return nil
}

// handleICECandidate processes an incoming ICE candidate
func (s *SignalingService) handleICECandidate(senderIdentity []byte, payload []byte) error {
	iceCandidate := &pb.RTCIceCandidate{}
	if err := proto.Unmarshal(payload, iceCandidate); err != nil {
		return fmt.Errorf("%w: failed to unmarshal ICE candidate: %w", ErrInvalidSignalingMessage, err)
	}

	// Verify the candidate matches an existing call
	session, err := s.sessionMgr.GetSession(iceCandidate.CallId)
	if err != nil {
		return fmt.Errorf("%w: ICE candidate for unknown call", ErrCallNotFound)
	}

	// Verify sender matches the expected remote identity
	if hex.EncodeToString(session.RemoteIdentity) != hex.EncodeToString(senderIdentity) {
		return fmt.Errorf("%w: ICE candidate from unexpected sender", ErrInvalidSignalingMessage)
	}

	// Add remote ICE candidate
	if err := s.sessionMgr.AddRemoteICECandidate(iceCandidate.CallId, iceCandidate.Candidate); err != nil {
		return fmt.Errorf("failed to add ICE candidate: %w", err)
	}

	logger.Debugw("received ICE candidate", "call", iceCandidate.CallId)

	// Notify callback
	if s.onIncomingICE != nil {
		s.onIncomingICE(iceCandidate.CallId, senderIdentity,
			iceCandidate.Candidate, iceCandidate.SdpMid, iceCandidate.SdpMlineIdx)
	}

	return nil
}

// handleHangup processes an incoming hangup message
func (s *SignalingService) handleHangup(senderIdentity []byte, payload []byte) error {
	hangup := &pb.RTCHangup{}
	if err := proto.Unmarshal(payload, hangup); err != nil {
		return fmt.Errorf("%w: failed to unmarshal hangup: %w", ErrInvalidSignalingMessage, err)
	}

	// Find the call session
	session, err := s.sessionMgr.GetSession(hangup.CallId)
	if err != nil {
		logger.Warnw("received hangup for unknown call", "call", hangup.CallId)
		return nil // Don't error, just log
	}

	// Verify sender matches the expected remote identity
	if hex.EncodeToString(session.RemoteIdentity) != hex.EncodeToString(senderIdentity) {
		return fmt.Errorf("%w: hangup from unexpected sender", ErrInvalidSignalingMessage)
	}

	logger.Infow("received hangup", "call", hangup.CallId, "reason", hangup.Reason)

	// End the call
	if err := s.sessionMgr.EndCall(hangup.CallId, hangup.Reason); err != nil {
		logger.Warnw("failed to end call", "call", hangup.CallId, "error", err)
	}

	// Notify callback
	if s.onIncomingHangup != nil {
		s.onIncomingHangup(hangup.CallId, senderIdentity, hangup.Reason)
	}

	return nil
}

// SetOnIncomingOffer sets the callback for incoming offers
func (s *SignalingService) SetOnIncomingOffer(callback func(callID string, remoteIdentity []byte, callType string, sdp string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onIncomingOffer = callback
}

// SetOnIncomingAnswer sets the callback for incoming answers
func (s *SignalingService) SetOnIncomingAnswer(callback func(callID string, remoteIdentity []byte, sdp string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onIncomingAnswer = callback
}

// SetOnIncomingICE sets the callback for incoming ICE candidates
func (s *SignalingService) SetOnIncomingICE(callback func(callID string, remoteIdentity []byte, candidate string, sdpMid string, mlineIdx uint32)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onIncomingICE = callback
}

// SetOnIncomingHangup sets the callback for incoming hangup messages
func (s *SignalingService) SetOnIncomingHangup(callback func(callID string, remoteIdentity []byte, reason string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onIncomingHangup = callback
}

// GetSessionManager returns the session manager
func (s *SignalingService) GetSessionManager() *SessionManager {
	return s.sessionMgr
}

// IsInCall checks if we're currently in a call with any peer
func (s *SignalingService) IsInCall() bool {
	sessions := s.sessionMgr.GetAllActiveSessions()
	return len(sessions) > 0
}

// IsInCallWithPeer checks if we're currently in a call with a specific peer
func (s *SignalingService) IsInCallWithPeer(remoteIdentity []byte) bool {
	_, err := s.sessionMgr.GetActiveSessionWithPeer(remoteIdentity)
	return err == nil
}
