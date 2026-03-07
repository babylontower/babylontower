// Package protocol implements the Babylon Tower Protocol v1 specification.
package protocol

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"babylontower/pkg/ratchet"
)

// SessionManagerImpl implements the SessionManager interface.
// It manages the lifecycle of X3DH + Double Ratchet sessions,
// providing session creation, storage, retrieval, and cleanup.
//
// Session Lifecycle:
//   1. Creation: Sessions are created via X3DH key exchange (initiator or responder)
//   2. Active: Sessions are used for encrypting/decrypting messages via Double Ratchet
//   3. Idle: Sessions that haven't been used recently are candidates for cleanup
//   4. Expired: Sessions exceeding MaxSessionAge are removed
//
// Thread Safety:
//   All public methods are thread-safe using read-write mutexes.
type SessionManagerImpl struct {
	// config is the protocol configuration
	config *ProtocolConfig
	// store is the underlying session storage
	store SessionStore
	// localIdentityPub is the local identity's Ed25519 public key
	localIdentityPub ed25519.PublicKey
	// localDeviceID is the local device's identifier
	localDeviceID []byte

	// Session cache for fast lookup
	sessionsByRemoteIdentity map[string]*ratchet.DoubleRatchetState // remoteIdentity:deviceID -> session
	sessionsByID             map[string]*ratchet.DoubleRatchetState // sessionID -> session

	// Device ID mapping (sessionID -> remote device ID)
	// This is needed because DoubleRatchetState doesn't store device ID
	deviceIDs map[string][]byte // sessionID -> remoteDeviceID

	// Mutex for protecting shared state
	mu sync.RWMutex

	// Metrics
	sessionsCreated int64
	sessionsDeleted int64
}

// InMemorySessionStore is an in-memory implementation of SessionStore.
// It provides fast session storage for testing and single-instance deployments.
// For production use with persistence, implement SessionStore with a database backend.
type InMemorySessionStore struct {
	sessions map[string]*ratchet.DoubleRatchetState
	mu       sync.RWMutex
}

// NewInMemorySessionStore creates a new in-memory session store.
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions: make(map[string]*ratchet.DoubleRatchetState),
	}
}

// Get retrieves a session by session ID.
func (s *InMemorySessionStore) Get(sessionID string) (*ratchet.DoubleRatchetState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

// GetByRemoteIdentity retrieves a session by remote identity and device.
// Since we store by session ID, we need to search through all sessions.
func (s *InMemorySessionStore) GetByRemoteIdentity(
	remoteIdentity ed25519.PublicKey,
	remoteDeviceID []byte,
) (*ratchet.DoubleRatchetState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	remoteIdentityHex := hex.EncodeToString(remoteIdentity)
	for _, session := range s.sessions {
		if hex.EncodeToString(session.RemoteIdentityPub) == remoteIdentityHex {
			return session, nil
		}
	}
	return nil, ErrSessionNotFound
}

// Put stores or updates a session.
// Uses session ID as the key since DoubleRatchetState doesn't have RemoteDeviceID.
func (s *InMemorySessionStore) Put(session *ratchet.DoubleRatchetState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Use session ID as the key
	s.sessions[session.SessionID] = session
	return nil
}

// Delete removes a session.
func (s *InMemorySessionStore) Delete(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return ErrSessionNotFound
	}
	delete(s.sessions, sessionID)
	return nil
}

// List returns all session IDs.
func (s *InMemorySessionStore) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	return ids, nil
}

// ListByRemoteIdentity returns all sessions for a remote identity.
func (s *InMemorySessionStore) ListByRemoteIdentity(remoteIdentity ed25519.PublicKey) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var ids []string
	remoteIdentityHex := hex.EncodeToString(remoteIdentity)

	for id, session := range s.sessions {
		if hex.EncodeToString(session.RemoteIdentityPub) == remoteIdentityHex {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// Count returns the number of stored sessions.
func (s *InMemorySessionStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// Cleanup removes expired sessions.
func (s *InMemorySessionStore) Cleanup(maxAge time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	maxAgeSeconds := int64(maxAge / time.Second)
	removed := 0

	for id, session := range s.sessions {
		if now-session.LastUsedAt > maxAgeSeconds {
			delete(s.sessions, id)
			removed++
		}
	}

	return removed, nil
}

// sessionKey creates a unique key for a session based on remote identity and device.
func sessionKey(remoteIdentity ed25519.PublicKey, remoteDeviceID []byte) string {
	hash := sha256.Sum256(append(remoteIdentity, remoteDeviceID...))
	return hex.EncodeToString(hash[:])
}

// generateSessionID creates a unique session ID from identity and device information.
func generateSessionID(localIdentity, remoteIdentity ed25519.PublicKey, localDeviceID, remoteDeviceID []byte) string {
	// Combine all identity information for unique session ID
	data := make([]byte, 0, 128)
	data = append(data, localIdentity...)
	data = append(data, remoteIdentity...)
	data = append(data, localDeviceID...)
	data = append(data, remoteDeviceID...)

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:16]) // 32 hex characters
}

// NewSessionManager creates a new session manager.
// It requires the protocol configuration and local identity information.
func NewSessionManager(
	config *ProtocolConfig,
	localIdentityPub ed25519.PublicKey,
	localDeviceID []byte,
) *SessionManagerImpl {
	return &SessionManagerImpl{
		config:                   config,
		store:                    NewInMemorySessionStore(),
		localIdentityPub:         localIdentityPub,
		localDeviceID:            localDeviceID,
		sessionsByRemoteIdentity: make(map[string]*ratchet.DoubleRatchetState),
		sessionsByID:             make(map[string]*ratchet.DoubleRatchetState),
		deviceIDs:                make(map[string][]byte),
	}
}

// NewSessionManagerWithStore creates a new session manager with a custom store.
// This allows using persistent storage implementations.
func NewSessionManagerWithStore(
	config *ProtocolConfig,
	localIdentityPub ed25519.PublicKey,
	localDeviceID []byte,
	store SessionStore,
) *SessionManagerImpl {
	return &SessionManagerImpl{
		config:                   config,
		store:                    store,
		localIdentityPub:         localIdentityPub,
		localDeviceID:            localDeviceID,
		sessionsByRemoteIdentity: make(map[string]*ratchet.DoubleRatchetState),
		sessionsByID:             make(map[string]*ratchet.DoubleRatchetState),
		deviceIDs:                make(map[string][]byte),
	}
}

// CreateInitiator creates a new session as X3DH initiator.
// This is called when we initiate a conversation with a remote party.
//
// Parameters:
//   - remoteIdentity: The remote party's Ed25519 identity public key
//   - remoteDeviceID: The remote device's identifier
//   - x3dhResult: The result of the X3DH key exchange
//
// Returns:
//   - The newly created Double Ratchet state
//   - An error if session creation fails
func (m *SessionManagerImpl) CreateInitiator(
	remoteIdentity ed25519.PublicKey,
	remoteDeviceID []byte,
	x3dhResult *ratchet.X3DHResult,
) (*ratchet.DoubleRatchetState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if session already exists
	key := sessionKey(remoteIdentity, remoteDeviceID)
	if existing, ok := m.sessionsByRemoteIdentity[key]; ok {
		logger.Debugw("Session already exists", "remote_identity", hex.EncodeToString(remoteIdentity))
		return existing, ErrSessionExists
	}

	// Generate session ID
	sessionID := generateSessionID(m.localIdentityPub, remoteIdentity, m.localDeviceID, remoteDeviceID)

	// Create Double Ratchet state as initiator
	// Per spec: Use remote SPK public key for initial DH ratchet
	session, err := ratchet.NewDoubleRatchetStateInitiator(
		sessionID,
		m.localIdentityPub,
		remoteIdentity,
		x3dhResult.SharedSecret,
		x3dhResult.RemoteSPKPub, // Use remote SPK pub for Double Ratchet initialization
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Double Ratchet state: %w", err)
	}

	// Set additional session metadata
	session.CipherSuiteID = x3dhResult.CipherSuite

	// Store the session
	if err := m.store.Put(session); err != nil {
		return nil, fmt.Errorf("failed to store session: %w", err)
	}

	m.sessionsByRemoteIdentity[key] = session
	m.sessionsByID[sessionID] = session
	m.deviceIDs[sessionID] = remoteDeviceID
	m.sessionsCreated++

	logger.Infow("Created initiator session",
		"session_id", sessionID,
		"remote_identity", hex.EncodeToString(remoteIdentity),
		"remote_device", hex.EncodeToString(remoteDeviceID))

	return session, nil
}

// CreateResponder creates a new session as X3DH responder.
// This is called when we receive an X3DH initial message from a remote party.
//
// Parameters:
//   - sessionID: The session ID from the incoming message
//   - remoteIdentity: The remote party's Ed25519 identity public key
//   - remoteDeviceID: The remote device's identifier
//   - x3dhResult: The result of the X3DH key exchange
//
// Returns:
//   - The newly created Double Ratchet state
//   - An error if session creation fails
func (m *SessionManagerImpl) CreateResponder(
	sessionID string,
	remoteIdentity ed25519.PublicKey,
	remoteDeviceID []byte,
	x3dhResult *ratchet.X3DHResult,
) (*ratchet.DoubleRatchetState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if session already exists
	if existing, ok := m.sessionsByID[sessionID]; ok {
		logger.Debugw("Session already exists", "session_id", sessionID)
		return existing, ErrSessionExists
	}

	// Create Double Ratchet state as responder
	// Note: We need the local SPK keys here, which should be passed in x3dhResult context
	session, err := ratchet.NewDoubleRatchetStateResponder(
		sessionID,
		m.localIdentityPub,
		remoteIdentity,
		x3dhResult.SharedSecret,
		nil, // localSPKPriv - should be provided
		nil, // localSPKPub - should be provided
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Double Ratchet state: %w", err)
	}

	// Set additional session metadata
	session.CipherSuiteID = x3dhResult.CipherSuite

	// Store the session
	if err := m.store.Put(session); err != nil {
		return nil, fmt.Errorf("failed to store session: %w", err)
	}

	key := sessionKey(remoteIdentity, remoteDeviceID)
	m.sessionsByRemoteIdentity[key] = session
	m.sessionsByID[sessionID] = session
	m.deviceIDs[sessionID] = remoteDeviceID
	m.sessionsCreated++

	logger.Infow("Created responder session",
		"session_id", sessionID,
		"remote_identity", hex.EncodeToString(remoteIdentity),
		"remote_device", hex.EncodeToString(remoteDeviceID))

	return session, nil
}

// Get retrieves an existing session by session ID.
func (m *SessionManagerImpl) Get(sessionID string) (*ratchet.DoubleRatchetState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessionsByID[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}

	return session, nil
}

// GetByRemoteIdentity retrieves a session by remote identity and device.
func (m *SessionManagerImpl) GetByRemoteIdentity(
	remoteIdentity ed25519.PublicKey,
	remoteDeviceID []byte,
) (*ratchet.DoubleRatchetState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := sessionKey(remoteIdentity, remoteDeviceID)
	session, ok := m.sessionsByRemoteIdentity[key]
	if !ok {
		return nil, ErrSessionNotFound
	}

	return session, nil
}

// GetOrCreateInitiator gets an existing session or creates a new one as initiator.
// This is a convenience method for the common pattern of checking for an existing
// session before creating a new one.
func (m *SessionManagerImpl) GetOrCreateInitiator(
	remoteIdentity ed25519.PublicKey,
	remoteDeviceID []byte,
	x3dhResult *ratchet.X3DHResult,
) (*ratchet.DoubleRatchetState, error) {
	// Try to get existing session
	session, err := m.GetByRemoteIdentity(remoteIdentity, remoteDeviceID)
	if err == nil {
		logger.Debugw("Found existing session",
			"remote_identity", hex.EncodeToString(remoteIdentity))
		return session, nil
	}

	if err != ErrSessionNotFound {
		return nil, err
	}

	// Create new session
	return m.CreateInitiator(remoteIdentity, remoteDeviceID, x3dhResult)
}

// Update updates an existing session.
// This should be called after the Double Ratchet state has been modified
// (e.g., after encrypting or decrypting a message).
func (m *SessionManagerImpl) Update(session *ratchet.DoubleRatchetState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update in store
	if err := m.store.Put(session); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	// Update caches
	key := sessionKey(session.RemoteIdentityPub, m.deviceIDs[session.SessionID])
	m.sessionsByRemoteIdentity[key] = session
	m.sessionsByID[session.SessionID] = session

	return nil
}

// Delete deletes a session by session ID.
func (m *SessionManagerImpl) Delete(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessionsByID[sessionID]
	if !ok {
		return ErrSessionNotFound
	}

	// Delete from store
	if err := m.store.Delete(sessionID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Remove from caches
	key := sessionKey(session.RemoteIdentityPub, m.deviceIDs[sessionID])
	delete(m.sessionsByRemoteIdentity, key)
	delete(m.sessionsByID, sessionID)
	delete(m.deviceIDs, sessionID)
	m.sessionsDeleted++

	logger.Debugw("Deleted session", "session_id", sessionID)
	return nil
}

// DeleteByRemoteIdentity deletes all sessions for a remote identity.
func (m *SessionManagerImpl) DeleteByRemoteIdentity(remoteIdentity ed25519.PublicKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find all sessions for this identity
	var sessionIDs []string
	remoteIdentityHex := hex.EncodeToString(remoteIdentity)

	for id, session := range m.sessionsByID {
		if hex.EncodeToString(session.RemoteIdentityPub) == remoteIdentityHex {
			sessionIDs = append(sessionIDs, id)
		}
	}

	// Delete all found sessions
	for _, id := range sessionIDs {
		session := m.sessionsByID[id]
		if err := m.store.Delete(id); err != nil {
			logger.Warnw("Failed to delete session", "session_id", id, "error", err)
			continue
		}

		key := sessionKey(session.RemoteIdentityPub, m.deviceIDs[id])
		delete(m.sessionsByRemoteIdentity, key)
		delete(m.sessionsByID, id)
		delete(m.deviceIDs, id)
		m.sessionsDeleted++
	}

	logger.Infow("Deleted sessions for remote identity",
		"remote_identity", remoteIdentityHex,
		"count", len(sessionIDs))

	return nil
}

// List lists all sessions with their metadata.
func (m *SessionManagerImpl) List() ([]*SessionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]*SessionInfo, 0, len(m.sessionsByID))
	for _, session := range m.sessionsByID {
		info := &SessionInfo{
			SessionID:         session.SessionID,
			RemoteIdentityPub: session.RemoteIdentityPub,
			RemoteDeviceID:    m.deviceIDs[session.SessionID],
			LocalDeviceID:     m.localDeviceID,
			CreatedAt:         time.Unix(session.CreatedAt, 0),
			LastUsedAt:        time.Unix(session.LastUsedAt, 0),
			IsInitiator:       session.IsInitiator,
			CipherSuite:       session.CipherSuiteID,
		}
		infos = append(infos, info)
	}

	return infos, nil
}

// Count returns the number of stored sessions.
func (m *SessionManagerImpl) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessionsByID)
}

// Cleanup removes expired sessions.
// It returns the number of sessions removed.
func (m *SessionManagerImpl) Cleanup() (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	maxAge := m.config.MaxSessionAge
	if maxAge <= 0 {
		maxAge = MaxSessionAge
	}

	now := time.Now().Unix()
	maxAgeSeconds := int64(maxAge / time.Second)
	removed := 0

	var toDelete []string
	for id, session := range m.sessionsByID {
		if now-session.LastUsedAt > maxAgeSeconds {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		session := m.sessionsByID[id]

		if err := m.store.Delete(id); err != nil {
			logger.Warnw("Failed to delete expired session", "session_id", id, "error", err)
			continue
		}

		key := sessionKey(session.RemoteIdentityPub, m.deviceIDs[id])
		delete(m.sessionsByRemoteIdentity, key)
		delete(m.sessionsByID, id)
		delete(m.deviceIDs, id)
		m.sessionsDeleted++
		removed++
	}

	if removed > 0 {
		logger.Infow("Cleaned up expired sessions", "count", removed)
	}

	return removed, nil
}

// GetSessionInfo returns metadata about a specific session.
func (m *SessionManagerImpl) GetSessionInfo(sessionID string) (*SessionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessionsByID[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}

	return &SessionInfo{
		SessionID:         session.SessionID,
		RemoteIdentityPub: session.RemoteIdentityPub,
		RemoteDeviceID:    m.deviceIDs[sessionID],
		LocalDeviceID:     m.localDeviceID,
		CreatedAt:         time.Unix(session.CreatedAt, 0),
		LastUsedAt:        time.Unix(session.LastUsedAt, 0),
		IsInitiator:       session.IsInitiator,
		CipherSuite:       session.CipherSuiteID,
	}, nil
}

// GetMetrics returns session manager metrics.
func (m *SessionManagerImpl) GetMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"sessions_active":    len(m.sessionsByID),
		"sessions_created":   m.sessionsCreated,
		"sessions_deleted":   m.sessionsDeleted,
		"max_sessions":       m.config.MaxStoredSessions,
		"max_session_age":    m.config.MaxSessionAge.String(),
		"max_skipped_keys":   m.config.MaxSkippedKeys,
	}
}
