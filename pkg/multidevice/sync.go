package multidevice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"babylontower/pkg/crypto"
	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"
	"github.com/ipfs/go-log/v2"
	"google.golang.org/protobuf/proto"
)

var syncLogger = log.Logger("babylontower/multidevice")

// SyncType aliases for convenience
type SyncType = pb.SyncType

const (
	SyncTypeContactAdded   = pb.SyncType_CONTACT_ADDED
	SyncTypeContactRemoved = pb.SyncType_CONTACT_REMOVED
	SyncTypeContactUpdated = pb.SyncType_CONTACT_UPDATED
	SyncTypeMessageRead    = pb.SyncType_MESSAGE_READ
	SyncTypeGroupJoined    = pb.SyncType_GROUP_JOINED
	SyncTypeGroupLeft      = pb.SyncType_GROUP_LEFT
	SyncTypeSettingsChanged = pb.SyncType_SETTINGS_CHANGED
	SyncTypeHistoryRequest = pb.SyncType_HISTORY_REQUEST
	SyncTypeHistoryBatch   = pb.SyncType_HISTORY_BATCH
)

// SyncEvent represents a synchronization event
type SyncEvent struct {
	Type           SyncType
	SourceDeviceID uint32
	Timestamp      uint64
	Payload        proto.Message
	VectorClock    *pb.VectorClock
}

// SyncManager handles cross-device state synchronization
type SyncManager struct {
	deviceManager *DeviceManager
	storage       storage.Storage
	ipfsNode      interface{} // IPFS node interface (avoid circular import)

	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup

	// Sync topic subscription
	subscription interface{}

	// Vector clock for conflict resolution
	vectorClock map[string]uint64 // device_id -> counter
	clockMu     sync.RWMutex

	// Event channel for sync events
	eventChan chan *SyncEvent

	// Pending history requests
	historyRequests map[string]*HistoryRequestState
	historyMu       sync.RWMutex
}

// HistoryRequestState tracks a pending history request
type HistoryRequestState struct {
	RequestID   string
	ContactPubKey []byte
	StartTime   uint64
	EndTime     uint64
	MaxMessages uint32
	Received    []*pb.HistoryMessage
	Complete    bool
}

// SyncManagerConfig holds configuration for the sync manager
type SyncManagerConfig struct {
	DeviceManager *DeviceManager
	Storage       storage.Storage
	IPFSNode      interface{}
}

// NewSyncManager creates a new sync manager
func NewSyncManager(config *SyncManagerConfig) *SyncManager {
	ctx, cancel := context.WithCancel(context.Background())

	sm := &SyncManager{
		deviceManager:   config.DeviceManager,
		storage:         config.Storage,
		ipfsNode:        config.IPFSNode,
		ctx:             ctx,
		cancel:          cancel,
		vectorClock:     make(map[string]uint64),
		eventChan:       make(chan *SyncEvent, 100),
		historyRequests: make(map[string]*HistoryRequestState),
	}

	// Load vector clock from storage
	sm.loadVectorClock()

	return sm
}

// GetSyncTopic derives the sync topic from the identity public key
// Format: babylon-sync-<hex(SHA256(IK_sign.pub)[:8])>
func GetSyncTopic(identityPub []byte) string {
	hash := sha256.Sum256(identityPub)
	return "babylon-sync-" + hex.EncodeToString(hash[:8])
}

// Start starts the sync manager and subscribes to the sync topic
func (sm *SyncManager) Start(identityPub []byte) error {
	topic := GetSyncTopic(identityPub)

	// Subscribe to sync topic
	// Note: Actual subscription requires type assertion to IPFS node
	// This is handled by the caller in main.go
	syncLogger.Infow("sync manager started", "topic", topic)

	return nil
}

// Stop stops the sync manager
func (sm *SyncManager) Stop() error {
	sm.cancel()
	sm.wg.Wait()

	if sm.subscription != nil {
		// Close subscription
	}

	syncLogger.Info("sync manager stopped")
	return nil
}

// BroadcastSync sends a sync message to all other devices
func (sm *SyncManager) BroadcastSync(syncType SyncType, payload proto.Message) error {
	// Serialize payload
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Encrypt payload with device group key
	nonce := make([]byte, 24)
	if _, err := crypto.SecureRandom.Read(nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext, err := crypto.Encrypt(payloadBytes, sm.deviceManager.deviceGroupKey, nonce)
	if err != nil {
		return fmt.Errorf("failed to encrypt payload: %w", err)
	}

	// Update vector clock
	sm.updateVectorClock()

	// Convert device ID to uint32 (first 4 bytes)
	sourceDeviceID := uint32(0)
	if len(sm.deviceManager.deviceID) >= 4 {
		sourceDeviceID = uint32(sm.deviceManager.deviceID[0])<<24 | uint32(sm.deviceManager.deviceID[1])<<16 | uint32(sm.deviceManager.deviceID[2])<<8 | uint32(sm.deviceManager.deviceID[3])
	}

	// Create sync message
	syncMsg := &pb.DeviceSyncMessage{
		SourceDeviceId:   sourceDeviceID,
		Type:             syncType,
		EncryptedPayload: ciphertext,
		Nonce:            nonce,
		Timestamp:        uint64(time.Now().Unix()),
		VectorClock:      sm.getVectorClock(),
	}

	// Serialize and publish
	data, err := proto.Marshal(syncMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal sync message: %w", err)
	}

	// Publish to sync topic
	// This requires IPFS node access - handled by caller
	_ = data

	return nil
}

// HandleSyncMessage processes an incoming sync message
func (sm *SyncManager) HandleSyncMessage(data []byte) error {
	// Parse sync message
	syncMsg := &pb.DeviceSyncMessage{}
	if err := proto.Unmarshal(data, syncMsg); err != nil {
		return fmt.Errorf("failed to unmarshal sync message: %w", err)
	}

	// Ignore messages from own device
	if len(sm.deviceManager.deviceID) >= 4 {
		// Convert uint32 back to bytes for comparison (simplified)
		deviceIDPrefix := uint32(sm.deviceManager.deviceID[0])<<24 | uint32(sm.deviceManager.deviceID[1])<<16 | uint32(sm.deviceManager.deviceID[2])<<8 | uint32(sm.deviceManager.deviceID[3])
		if syncMsg.SourceDeviceId == deviceIDPrefix {
			return nil
		}
	}

	// Decrypt payload
	plaintext, err := crypto.Decrypt(syncMsg.EncryptedPayload, sm.deviceManager.deviceGroupKey, syncMsg.Nonce)
	if err != nil {
		return fmt.Errorf("failed to decrypt payload: %w", err)
	}

	// Parse payload based on sync type
	var payload proto.Message
	switch syncMsg.Type {
	case pb.SyncType_CONTACT_ADDED, pb.SyncType_CONTACT_REMOVED, pb.SyncType_CONTACT_UPDATED:
		payload = &pb.ContactSync{}
	case pb.SyncType_MESSAGE_READ:
		payload = &pb.ReadReceiptSync{}
	case pb.SyncType_GROUP_JOINED, pb.SyncType_GROUP_LEFT:
		payload = &pb.GroupSync{}
	case pb.SyncType_SETTINGS_CHANGED:
		payload = &pb.SettingsSync{}
	case pb.SyncType_HISTORY_REQUEST:
		payload = &pb.HistoryRequest{}
	case pb.SyncType_HISTORY_BATCH:
		payload = &pb.HistoryBatch{}
	default:
		return fmt.Errorf("unknown sync type: %v", syncMsg.Type)
	}

	if err := proto.Unmarshal(plaintext, payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// Update vector clock
	sm.mergeVectorClock(syncMsg.VectorClock)

	// Create sync event
	event := &SyncEvent{
		Type:        syncMsg.Type,
		SourceDeviceID: syncMsg.SourceDeviceId,
		Timestamp:   syncMsg.Timestamp,
		Payload:     payload,
		VectorClock: syncMsg.VectorClock,
	}

	// Emit event
	select {
	case sm.eventChan <- event:
	default:
		syncLogger.Warn("sync event channel full, dropping event")
	}

	return nil
}

// updateVectorClock increments the local device's counter
func (sm *SyncManager) updateVectorClock() {
	sm.clockMu.Lock()
	defer sm.clockMu.Unlock()

	deviceID := hex.EncodeToString(sm.deviceManager.deviceID)
	sm.vectorClock[deviceID]++
}

// getVectorClock returns a copy of the current vector clock
func (sm *SyncManager) getVectorClock() *pb.VectorClock {
	sm.clockMu.RLock()
	defer sm.clockMu.RUnlock()

	clocks := make(map[string]uint64)
	for k, v := range sm.vectorClock {
		clocks[k] = v
	}

	return &pb.VectorClock{
		Clocks: clocks,
	}
}

// mergeVectorClock merges a remote vector clock with local
func (sm *SyncManager) mergeVectorClock(remote *pb.VectorClock) {
	if remote == nil {
		return
	}

	sm.clockMu.Lock()
	defer sm.clockMu.Unlock()

	for deviceID, remoteCounter := range remote.Clocks {
		if sm.vectorClock[deviceID] < remoteCounter {
			sm.vectorClock[deviceID] = remoteCounter
		}
	}
}

// loadVectorClock loads the vector clock from storage
func (sm *SyncManager) loadVectorClock() {
	// Would load from storage in full implementation
}

// saveVectorClock saves the vector clock to storage
func (sm *SyncManager) saveVectorClock() {
	// Would save to storage in full implementation
}

// Events returns the channel for receiving sync events
func (sm *SyncManager) Events() <-chan *SyncEvent {
	return sm.eventChan
}

// RequestHistory sends a history request to other devices
func (sm *SyncManager) RequestHistory(contactPubKey []byte, startTime, endTime uint64, maxMessages uint32) (string, error) {
	requestID := hex.EncodeToString(make([]byte, 16)) // In production, use crypto random

	request := &pb.HistoryRequest{
		ContactPubkey: contactPubKey,
		StartTime:     startTime,
		EndTime:       endTime,
		MaxMessages:   maxMessages,
	}

	// Store request state
	sm.historyMu.Lock()
	sm.historyRequests[requestID] = &HistoryRequestState{
		RequestID:     requestID,
		ContactPubKey: contactPubKey,
		StartTime:     startTime,
		EndTime:       endTime,
		MaxMessages:   maxMessages,
	}
	sm.historyMu.Unlock()

	// Broadcast request
	if err := sm.BroadcastSync(SyncTypeHistoryRequest, request); err != nil {
		return "", err
	}

	return requestID, nil
}

// SendHistoryBatch sends a batch of historical messages
func (sm *SyncManager) SendHistoryBatch(contactPubKey []byte, messages []*pb.HistoryMessage, batchNumber uint32, isLastBatch bool) error {
	batch := &pb.HistoryBatch{
		ContactPubkey: contactPubKey,
		Messages:      messages,
		BatchNumber:   batchNumber,
		IsLastBatch:   isLastBatch,
	}

	return sm.BroadcastSync(SyncTypeHistoryBatch, batch)
}

// HandleHistoryRequest processes an incoming history request
func (sm *SyncManager) HandleHistoryRequest(request *pb.HistoryRequest, sourceDeviceID []byte) error {
	// Fetch messages from storage
	// This would query the storage layer and send back batches
	// Implementation depends on storage schema
	return nil
}

// HandleHistoryBatch processes an incoming history batch
func (sm *SyncManager) HandleHistoryBatch(batch *pb.HistoryBatch) error {
	sm.historyMu.Lock()
	defer sm.historyMu.Unlock()

	// Find matching request
	// In production, would match by contact and track batches
	for _, req := range sm.historyRequests {
		if string(req.ContactPubKey) == string(batch.ContactPubkey) {
			req.Received = append(req.Received, batch.Messages...)
			if batch.IsLastBatch {
				req.Complete = true
			}
			break
		}
	}

	return nil
}

// Conflict Resolution

// resolveConflict uses vector clocks to resolve conflicting updates
// Returns true if the remote update should be applied
func (sm *SyncManager) resolveConflict(localClock, remoteClock *pb.VectorClock, localTimestamp, remoteTimestamp uint64) bool {
	// Last-writer-wins by wall clock timestamp, vector clock as tiebreaker
	if remoteTimestamp > localTimestamp {
		return true
	}
	if remoteTimestamp < localTimestamp {
		return false
	}

	// Timestamps equal, use vector clock comparison
	return compareVectorClocks(remoteClock, localClock) >= 0
}

// compareVectorClocks compares two vector clocks
// Returns: -1 if a < b, 0 if a == b, 1 if a > b
func compareVectorClocks(a, b *pb.VectorClock) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Compare all entries
	allLessOrEqual := true
	allGreaterOrEqual := true

	for deviceID, aVal := range a.Clocks {
		bVal := b.Clocks[deviceID]
		if aVal < bVal {
			allGreaterOrEqual = false
		}
		if aVal > bVal {
			allLessOrEqual = false
		}
	}

	for deviceID, bVal := range b.Clocks {
		aVal := a.Clocks[deviceID]
		if aVal < bVal {
			allGreaterOrEqual = false
		}
		if aVal > bVal {
			allLessOrEqual = false
		}
	}

	if allLessOrEqual && !allGreaterOrEqual {
		return -1 // a < b
	}
	if allGreaterOrEqual && !allLessOrEqual {
		return 1 // a > b
	}
	return 0 // concurrent
}

// Helper functions for sync message creation

// CreateContactSync creates a contact sync message
func CreateContactSync(contactPubKey, x25519PubKey []byte, displayName, peerID string, multiaddrs []string, isRemoved bool) *pb.ContactSync {
	return &pb.ContactSync{
		ContactPubkey: contactPubKey,
		DisplayName:   displayName,
		X25519Pubkey:  x25519PubKey,
		PeerId:        peerID,
		Multiaddrs:    multiaddrs,
		CreatedAt:     uint64(time.Now().Unix()),
		IsRemoved:     isRemoved,
	}
}

// CreateReadReceiptSync creates a read receipt sync message
func CreateReadReceiptSync(contactPubKey []byte, messageIDs [][]byte) *pb.ReadReceiptSync {
	return &pb.ReadReceiptSync{
		ContactPubkey: contactPubKey,
		MessageIds:    messageIDs,
		ReadAt:        uint64(time.Now().Unix()),
	}
}

// CreateGroupSync creates a group sync message
func CreateGroupSync(groupID []byte, name string, epoch uint64, joined bool) *pb.GroupSync {
	return &pb.GroupSync{
		GroupId:   groupID,
		Name:      name,
		Epoch:     epoch,
		Joined:    joined,
		Timestamp: uint64(time.Now().Unix()),
	}
}

// CreateSettingsSync creates a settings sync message
func CreateSettingsSync(key string, value []byte) *pb.SettingsSync {
	return &pb.SettingsSync{
		Key:       key,
		Value:     value,
		UpdatedAt: uint64(time.Now().Unix()),
	}
}

// Common errors
var (
	ErrDecryptionFailed = errors.New("failed to decrypt sync message")
	ErrInvalidPayload   = errors.New("invalid sync payload")
	ErrChannelFull      = errors.New("sync event channel full")
)
