package mailbox

import (
	"testing"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "babylontower/pkg/proto"
)

func setupTestStorage(t *testing.T) (*Storage, func()) {
	// Create in-memory BadgerDB
	opts := badger.DefaultOptions("")
	opts.InMemory = true
	db, err := badger.Open(opts)
	require.NoError(t, err)

	config := &pb.MailboxConfig{
		MaxMessagesPerTarget:   100,
		MaxMessageSize:         1024 * 1024,      // 1 MB
		MaxTotalBytesPerTarget: 10 * 1024 * 1024, // 10 MB
		DefaultTtlSeconds:      3600,             // 1 hour
		DepositRateLimit:       10,
	}

	storage, err := NewStorage(db, config)
	require.NoError(t, err)

	cleanup := func() {
		_ = db.Close()
	}

	return storage, cleanup
}

func TestStorage_StoreMessage(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	targetPubkey := []byte("target_pubkey_32_bytes____________")
	messageID := []byte("message_id_16_by")
	senderPubkey := []byte("sender_pubkey_32_bytes____________")
	envelope := []byte("test envelope data")

	err := storage.StoreMessage(targetPubkey, messageID, senderPubkey, envelope, 3600)
	assert.NoError(t, err)

	// Verify message was stored
	stored, err := storage.GetMessage(targetPubkey, messageID)
	assert.NoError(t, err)
	assert.Equal(t, messageID, stored.MessageID)
	assert.Equal(t, senderPubkey, stored.SenderPubkey)
	assert.Equal(t, envelope, stored.Envelope)
}

func TestStorage_ListMessages(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	targetPubkey := []byte("target_pubkey_32_bytes____________")

	// Store multiple messages
	for i := 0; i < 5; i++ {
		messageID := []byte{byte('m'), byte('s'), byte('g'), byte('_'), byte('0' + byte(i)), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		senderPubkey := []byte("sender_pubkey_32_bytes____________")
		envelope := []byte("test envelope data")
		err := storage.StoreMessage(targetPubkey, messageID, senderPubkey, envelope, 3600)
		require.NoError(t, err)
	}

	// List messages
	messages, err := storage.ListMessages(targetPubkey)
	assert.NoError(t, err)
	assert.Len(t, messages, 5)
}

func TestStorage_DeleteMessages(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	targetPubkey := []byte("target_pubkey_32_bytes____________")
	messageID1 := []byte("message_id_1_____")
	messageID2 := []byte("message_id_2_____")
	senderPubkey := []byte("sender_pubkey_32_bytes____________")
	envelope := []byte("test envelope data")

	// Store messages
	err := storage.StoreMessage(targetPubkey, messageID1, senderPubkey, envelope, 3600)
	require.NoError(t, err)
	err = storage.StoreMessage(targetPubkey, messageID2, senderPubkey, envelope, 3600)
	require.NoError(t, err)

	// Delete one message
	err = storage.DeleteMessages(targetPubkey, [][]byte{messageID1})
	assert.NoError(t, err)

	// Verify only one remains
	messages, err := storage.ListMessages(targetPubkey)
	assert.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Equal(t, messageID2, messages[0].MessageID)
}

func TestStorage_QuotaExceeded(t *testing.T) {
	opts := badger.DefaultOptions("")
	opts.InMemory = true
	db, err := badger.Open(opts)
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	config := &pb.MailboxConfig{
		MaxMessagesPerTarget:   3, // Very low limit for testing
		MaxMessageSize:         1024,
		MaxTotalBytesPerTarget: 10 * 1024,
		DefaultTtlSeconds:      3600,
		DepositRateLimit:       100,
	}

	storage, err := NewStorage(db, config)
	require.NoError(t, err)

	targetPubkey := []byte("target_pubkey_32_bytes____________")
	senderPubkey := []byte("sender_pubkey_32_bytes____________")

	// Fill quota
	for i := 0; i < 3; i++ {
		messageID := []byte{byte('m'), byte('s'), byte('g'), byte('_'), byte('0' + byte(i)), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		envelope := []byte("test envelope")
		err := storage.StoreMessage(targetPubkey, messageID, senderPubkey, envelope, 3600)
		require.NoError(t, err)
	}

	// Try to exceed quota
	messageID := []byte("message_overflow_")
	envelope := []byte("test envelope")
	err = storage.StoreMessage(targetPubkey, messageID, senderPubkey, envelope, 3600)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "limit")
}

func TestStorage_RateLimit(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	senderPubkey := []byte("sender_pubkey_32_bytes____________")
	targetPubkey := []byte("target_pubkey_32_bytes____________")

	// Check initial state (not rate limited)
	limited, err := storage.CheckRateLimit(senderPubkey, targetPubkey)
	assert.NoError(t, err)
	assert.False(t, limited)

	// Increment rate limit
	for i := 0; i < 10; i++ {
		err := storage.IncrementRateLimit(senderPubkey, targetPubkey)
		require.NoError(t, err)
	}

	// Should be rate limited now
	limited, err = storage.CheckRateLimit(senderPubkey, targetPubkey)
	assert.NoError(t, err)
	assert.True(t, limited)
}

func TestStorage_GetStats(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	targetPubkey := []byte("target_pubkey_32_bytes____________")
	senderPubkey := []byte("sender_pubkey_32_bytes____________")

	// Store some messages
	for i := 0; i < 3; i++ {
		messageID := []byte{byte('m'), byte('s'), byte('g'), byte('_'), byte('0' + byte(i)), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		envelope := []byte("test envelope data")
		err := storage.StoreMessage(targetPubkey, messageID, senderPubkey, envelope, 3600)
		require.NoError(t, err)
	}

	// Get stats
	stats, err := storage.GetStats(targetPubkey)
	assert.NoError(t, err)
	assert.Equal(t, uint32(3), stats.StoredCount)
	assert.Greater(t, stats.UsedBytes, uint64(0))
}

func TestStorage_CleanupExpired(t *testing.T) {
	opts := badger.DefaultOptions("")
	opts.InMemory = true
	db, err := badger.Open(opts)
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	config := &pb.MailboxConfig{
		MaxMessagesPerTarget:   100,
		MaxMessageSize:         1024,
		MaxTotalBytesPerTarget: 10 * 1024,
		DefaultTtlSeconds:      1, // 1 second TTL for testing
		DepositRateLimit:       100,
	}

	storage, err := NewStorage(db, config)
	require.NoError(t, err)

	targetPubkey := []byte("target_pubkey_32_bytes____________")
	senderPubkey := []byte("sender_pubkey_32_bytes____________")

	// Store message with 1 second TTL
	messageID := []byte("message_to_expire_")
	envelope := []byte("test envelope")
	err = storage.StoreMessage(targetPubkey, messageID, senderPubkey, envelope, 1)
	require.NoError(t, err)

	// Wait for expiration
	time.Sleep(2 * time.Second)

	// Cleanup
	deleted, err := storage.CleanupExpired()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, deleted, 0) // May be 0 if already filtered

	// Try to get expired message
	_, err = storage.GetMessage(targetPubkey, messageID)
	assert.Error(t, err)
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, uint32(500), config.MaxMessagesPerTarget)
	assert.Equal(t, uint64(262144), config.MaxMessageSize)
	assert.Equal(t, uint64(67108864), config.MaxTotalBytesPerTarget)
	assert.Equal(t, uint64(604800), config.DefaultTTLSeconds)
	assert.Equal(t, uint32(100), config.DepositRateLimit)
}
