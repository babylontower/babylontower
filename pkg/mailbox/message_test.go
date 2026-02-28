package mailbox

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pb "babylontower/pkg/proto"
)

func TestMailboxAnnouncement(t *testing.T) {
	// Test basic announcement structure
	announcement := &pb.MailboxAnnouncement{
		MailboxPeerId:   []byte("QmTestPeerID"),
		TargetPubkey:    []byte("target_pubkey_32_bytes__________"),
		CapacityBytes:   67108864,
		MaxMessageSize:  262144,
		MaxMessages:     500,
		TtlSeconds:      604800,
		AnnouncedAt:     1234567890,
		Capabilities:    []string{"mailbox-v1", "content-route"},
		ReputationScore: 100,
	}

	assert.Equal(t, "QmTestPeerID", string(announcement.MailboxPeerId))
	assert.Equal(t, uint64(67108864), announcement.CapacityBytes)
	assert.Equal(t, uint32(500), announcement.MaxMessages)
	assert.Len(t, announcement.Capabilities, 2)
}

func TestDepositRequest(t *testing.T) {
	req := &pb.DepositRequest{
		TargetPubkey: []byte("target_pubkey_32_bytes__________"),
		Envelope:     []byte("test envelope data"),
		RequestId:    12345,
		Timestamp:    1234567890,
	}

	assert.Equal(t, uint64(12345), req.RequestId)
	assert.Len(t, req.TargetPubkey, 32)
}

func TestDepositResponse(t *testing.T) {
	resp := &pb.DepositResponse{
		RequestId:      12345,
		Accepted:       true,
		StoredUntil:    1234567890 + 604800,
	}

	assert.True(t, resp.Accepted)
	assert.Equal(t, uint64(12345), resp.RequestId)
}

func TestRetrievalRequest(t *testing.T) {
	req := &pb.RetrievalRequest{
		RecipientPubkey: make([]byte, 32),
		Nonce:           make([]byte, 32),
		Timestamp:       1234567890,
	}

	assert.Len(t, req.RecipientPubkey, 32)
	assert.Len(t, req.Nonce, 32)
}

func TestRetrievalResponse(t *testing.T) {
	resp := &pb.RetrievalResponse{
		Nonce:      make([]byte, 32),
		MessageIds: [][]byte{[]byte("msg1"), []byte("msg2")},
		Envelopes:  [][]byte{[]byte("env1"), []byte("env2")},
		Count:      2,
	}

	assert.Equal(t, uint64(2), resp.Count)
	assert.Len(t, resp.MessageIds, 2)
}

func TestAcknowledgmentRequest(t *testing.T) {
	req := &pb.AcknowledgmentRequest{
		RecipientPubkey: make([]byte, 32),
		MessageIds:      [][]byte{[]byte("msg1"), []byte("msg2")},
		Timestamp:       1234567890,
	}

	assert.Len(t, req.MessageIds, 2)
}

func TestAcknowledgmentResponse(t *testing.T) {
	resp := &pb.AcknowledgmentResponse{
		MessageIds: [][]byte{[]byte("msg1"), []byte("msg2")},
		Success:    true,
	}

	assert.True(t, resp.Success)
	assert.Len(t, resp.MessageIds, 2)
}

func TestMailboxStats(t *testing.T) {
	stats := &pb.MailboxStats{
		TargetPubkey:    []byte("target_pubkey_32_bytes__________"),
		StoredCount:     42,
		UsedBytes:       1024 * 1024,
		CapacityBytes:   64 * 1024 * 1024,
		OldestTimestamp: 1234567890,
		NewestTimestamp: 1234567990,
	}

	assert.Equal(t, uint32(42), stats.StoredCount)
	assert.Equal(t, uint64(1024*1024), stats.UsedBytes)
}

func TestMailboxConfig(t *testing.T) {
	config := &pb.MailboxConfig{
		MaxMessagesPerTarget:    500,
		MaxMessageSize:          262144,
		MaxTotalBytesPerTarget:  67108864,
		DefaultTtlSeconds:       604800,
		DepositRateLimit:        100,
		EnableContentRouting:    false,
	}

	assert.Equal(t, uint32(500), config.MaxMessagesPerTarget)
	assert.Equal(t, uint64(262144), config.MaxMessageSize)
	assert.Equal(t, uint64(67108864), config.MaxTotalBytesPerTarget)
	assert.Equal(t, uint64(604800), config.DefaultTtlSeconds)
	assert.Equal(t, uint32(100), config.DepositRateLimit)
	assert.False(t, config.EnableContentRouting)
}

func TestStoredMailboxMessage(t *testing.T) {
	msg := &pb.StoredMailboxMessage{
		MessageId:    []byte("message_id_16_by"),
		SenderPubkey: []byte("sender_pubkey_32_bytes__________"),
		Envelope:     []byte("test envelope data"),
		StoredAt:     1234567890,
		ExpiresAt:    1234567890 + 604800,
		Size:         100,
	}

	assert.Equal(t, uint64(100), msg.Size)
	assert.Len(t, msg.MessageId, 16)
	assert.Len(t, msg.SenderPubkey, 32)
}
