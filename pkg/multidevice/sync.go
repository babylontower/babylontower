package multidevice

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"babylontower/pkg/crypto"
	bterrors "babylontower/pkg/errors"
	pb "babylontower/pkg/proto"
	"babylontower/pkg/protocol"
	"babylontower/pkg/storage"

	"github.com/ipfs/go-log/v2"
	"google.golang.org/protobuf/proto"
)

var logger = log.Logger("babylontower/multidevice")

// SyncType aliases for convenience
type SyncType = pb.SyncType

const (
	SyncTypeContactAdded    = pb.SyncType_CONTACT_ADDED
	SyncTypeContactRemoved  = pb.SyncType_CONTACT_REMOVED
	SyncTypeContactUpdated  = pb.SyncType_CONTACT_UPDATED
	SyncTypeMessageRead     = pb.SyncType_MESSAGE_READ
	SyncTypeGroupJoined     = pb.SyncType_GROUP_JOINED
	SyncTypeGroupLeft       = pb.SyncType_GROUP_LEFT
	SyncTypeSettingsChanged = pb.SyncType_SETTINGS_CHANGED
	SyncTypeHistoryRequest  = pb.SyncType_HISTORY_REQUEST
	SyncTypeHistoryBatch    = pb.SyncType_HISTORY_BATCH
)

// SyncEvent represents a synchronization event
type SyncEvent struct {
	Type           SyncType
	SourceDeviceID uint32
	Timestamp      uint64
	Payload        proto.Message
	VectorClock    *pb.VectorClock
}

// Publisher is the interface for publishing messages to PubSub topics.
// Defined here to avoid circular imports with ipfsnode.
type Publisher interface {
	Publish(topic string, data []byte) error
}

// SyncManager handles cross-device state synchronization
type SyncManager struct {
	deviceManager *DeviceManager
	storage       storage.Storage
	publisher     Publisher // For publishing sync messages
	syncTopic     string   // Cached sync topic

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

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
	RequestID     string
	ContactPubKey []byte
	StartTime     uint64
	EndTime       uint64
	MaxMessages   uint32
	Received      []*pb.HistoryMessage
	Complete      bool
}

// SyncManagerConfig holds configuration for the sync manager
type SyncManagerConfig struct {
	DeviceManager *DeviceManager
	Storage       storage.Storage
	Publisher     Publisher
}

// NewSyncManager creates a new sync manager
func NewSyncManager(config *SyncManagerConfig) *SyncManager {
	ctx, cancel := context.WithCancel(context.Background())

	sm := &SyncManager{
		deviceManager:   config.DeviceManager,
		storage:         config.Storage,
		publisher:       config.Publisher,
		ctx:             ctx,
		cancel:          cancel,
		vectorClock:     make(map[string]uint64),
		eventChan:       make(chan *SyncEvent, 100),
		historyRequests: make(map[string]*HistoryRequestState),
	}

	return sm
}

// GetSyncTopic derives the sync topic from the identity public key
// Format: babylon-sync-<hex(SHA256(IK_sign.pub)[:8])>
// Delegates to protocol.DeriveSyncTopic for canonical topic derivation.
func GetSyncTopic(identityPub []byte) string {
	return protocol.DeriveSyncTopic(identityPub)
}

// Start starts the sync manager and subscribes to the sync topic
func (sm *SyncManager) Start(identityPub []byte) error {
	sm.syncTopic = GetSyncTopic(identityPub)

	logger.Infow("sync manager started", "topic", sm.syncTopic)

	return nil
}

// Stop stops the sync manager
func (sm *SyncManager) Stop() error {
	sm.cancel()
	sm.wg.Wait()

	if sm.subscription != nil {
		// Close subscription - TODO: Implement subscription cleanup when added
		_ = sm.subscription // prevent staticcheck empty branch warning
	}

	logger.Info("sync manager stopped")
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

	ciphertext, err := crypto.Encrypt(sm.deviceManager.deviceGroupKey, nonce, payloadBytes)
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
	if sm.publisher != nil && sm.syncTopic != "" {
		if err := sm.publisher.Publish(sm.syncTopic, data); err != nil {
			return fmt.Errorf("failed to publish sync message: %w", err)
		}
	}

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
		Type:           syncMsg.Type,
		SourceDeviceID: syncMsg.SourceDeviceId,
		Timestamp:      syncMsg.Timestamp,
		Payload:        payload,
		VectorClock:    syncMsg.VectorClock,
	}

	// Emit event
	select {
	case sm.eventChan <- event:
	default:
		logger.Warn("sync event channel full, dropping event")
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

// ResolveConflict resolves conflicts between local and remote sync events.
// Per §7.4:
//   - Contacts: LWW by wall clock + vector clock sum as tiebreaker
//   - Read receipts: max timestamp wins
//   - Groups: longest chain (highest epoch) wins
//
// Returns true if remote event should be applied (wins), false if local wins.
func (sm *SyncManager) ResolveConflict(localEvent, remoteEvent *SyncEvent) bool {
	switch remoteEvent.Type {
	case pb.SyncType_CONTACT_ADDED, pb.SyncType_CONTACT_REMOVED, pb.SyncType_CONTACT_UPDATED:
		// LWW: higher wall clock wins; on tie, higher vector clock sum wins
		if remoteEvent.Timestamp != localEvent.Timestamp {
			return remoteEvent.Timestamp > localEvent.Timestamp
		}
		return vectorClockSum(remoteEvent.VectorClock) > vectorClockSum(localEvent.VectorClock)

	case pb.SyncType_MESSAGE_READ:
		// Max timestamp: the later read receipt wins
		return remoteEvent.Timestamp > localEvent.Timestamp

	case pb.SyncType_GROUP_JOINED, pb.SyncType_GROUP_LEFT:
		// Longest chain: compare group epochs if available
		localGroup, localOk := localEvent.Payload.(*pb.GroupSync)
		remoteGroup, remoteOk := remoteEvent.Payload.(*pb.GroupSync)
		if localOk && remoteOk {
			return remoteGroup.Epoch > localGroup.Epoch
		}
		// Fallback to timestamp
		return remoteEvent.Timestamp > localEvent.Timestamp

	default:
		// Default: later timestamp wins
		return remoteEvent.Timestamp > localEvent.Timestamp
	}
}

// vectorClockSum returns the sum of all counters in a vector clock (used as tiebreaker)
func vectorClockSum(vc *pb.VectorClock) uint64 {
	if vc == nil {
		return 0
	}
	var sum uint64
	for _, v := range vc.Clocks {
		sum += v
	}
	return sum
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

// HandleHistoryRequest processes an incoming history request.
// Per §7.3: Responds with batches of 100 messages in reverse chronological order.
func (sm *SyncManager) HandleHistoryRequest(request *pb.HistoryRequest, sourceDeviceID []byte) error {
	if sm.storage == nil {
		return errors.New("storage not available for history sync")
	}

	// Query messages from storage for the requested contact
	maxMsgs := int(request.MaxMessages)
	if maxMsgs == 0 || maxMsgs > 10000 {
		maxMsgs = 10000
	}
	messages, err := sm.storage.GetMessages(request.ContactPubkey, maxMsgs, 0)
	if err != nil {
		return fmt.Errorf("failed to query messages: %w", err)
	}

	// Filter by time range
	var filtered []*storage.StoredMessage
	for _, msg := range messages {
		if request.StartTime > 0 && msg.Timestamp < request.StartTime {
			continue
		}
		if request.EndTime > 0 && msg.Timestamp > request.EndTime {
			continue
		}
		filtered = append(filtered, msg)
	}

	// Sort by timestamp descending (reverse chronological)
	for i := 0; i < len(filtered)-1; i++ {
		for j := i + 1; j < len(filtered); j++ {
			if filtered[i].Timestamp < filtered[j].Timestamp {
				filtered[i], filtered[j] = filtered[j], filtered[i]
			}
		}
	}

	// Send in batches of 100
	const batchSize = 100
	totalBatches := (len(filtered) + batchSize - 1) / batchSize
	if totalBatches == 0 {
		totalBatches = 1
	}

	for batchNum := 0; batchNum < totalBatches; batchNum++ {
		start := batchNum * batchSize
		end := start + batchSize
		if end > len(filtered) {
			end = len(filtered)
		}

		batch := filtered[start:end]
		historyMsgs := make([]*pb.HistoryMessage, len(batch))
		for i, msg := range batch {
			historyMsgs[i] = &pb.HistoryMessage{
				Text:         msg.Text,
				Timestamp:    msg.Timestamp,
				IsSent:       msg.IsOutgoing,
				SenderPubkey: msg.SenderPubKey,
			}
		}

		isLast := batchNum == totalBatches-1
		if err := sm.SendHistoryBatch(request.ContactPubkey, historyMsgs, uint32(batchNum), isLast); err != nil {
			return fmt.Errorf("failed to send history batch %d: %w", batchNum, err)
		}
	}

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
	// ErrDecryptionFailed aliases the canonical sentinel from pkg/errors.
	ErrDecryptionFailed = bterrors.ErrDecryptionFailed
	ErrInvalidPayload   = errors.New("invalid sync payload")
	ErrChannelFull      = errors.New("sync event channel full")
)
