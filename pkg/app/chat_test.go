package app

import (
	"testing"
	"time"

	"babylontower/pkg/messaging"
	pb "babylontower/pkg/proto"

	"github.com/stretchr/testify/assert"
)

func TestChatMessageFromMeta(t *testing.T) {
	msg := &messaging.MessageWithMeta{
		Message: &pb.Message{
			Text:      "Hello",
			Timestamp: 1000,
		},
		IsOutgoing: false,
	}

	cm := chatMessageFromMeta(msg, "Alice", "abc123")
	assert.Equal(t, "Hello", cm.Text)
	assert.Equal(t, "Alice", cm.SenderName)
	assert.Equal(t, "abc123", cm.ContactPubKeyBase58)
	assert.False(t, cm.IsOutgoing)
	assert.Equal(t, time.Unix(1000, 0), cm.Timestamp)
}

func TestChatMessageFromMeta_Outgoing(t *testing.T) {
	msg := &messaging.MessageWithMeta{
		Message: &pb.Message{
			Text:      "Hi there",
			Timestamp: 2000,
		},
		IsOutgoing: true,
	}

	cm := chatMessageFromMeta(msg, "Bob", "xyz789")
	assert.Equal(t, "You", cm.SenderName)
	assert.True(t, cm.IsOutgoing)
}

func TestSortConversations(t *testing.T) {
	convs := []*Conversation{
		{
			Contact:     &ContactInfo{DisplayName: "Old", CreatedAt: time.Unix(100, 0)},
			LastMessage: &ChatMessage{Timestamp: time.Unix(100, 0)},
		},
		{
			Contact:     &ContactInfo{DisplayName: "New", CreatedAt: time.Unix(200, 0)},
			LastMessage: &ChatMessage{Timestamp: time.Unix(300, 0)},
		},
		{
			Contact:     &ContactInfo{DisplayName: "Mid", CreatedAt: time.Unix(150, 0)},
			LastMessage: &ChatMessage{Timestamp: time.Unix(200, 0)},
		},
	}

	sortConversations(convs)

	assert.Equal(t, "New", convs[0].Contact.DisplayName)
	assert.Equal(t, "Mid", convs[1].Contact.DisplayName)
	assert.Equal(t, "Old", convs[2].Contact.DisplayName)
}

func TestSortConversations_NoMessages(t *testing.T) {
	convs := []*Conversation{
		{
			Contact: &ContactInfo{DisplayName: "A", CreatedAt: time.Unix(100, 0)},
		},
		{
			Contact:     &ContactInfo{DisplayName: "B", CreatedAt: time.Unix(50, 0)},
			LastMessage: &ChatMessage{Timestamp: time.Unix(200, 0)},
		},
	}

	sortConversations(convs)

	// B has a message at t=200, A only has creation at t=100
	assert.Equal(t, "B", convs[0].Contact.DisplayName)
	assert.Equal(t, "A", convs[1].Contact.DisplayName)
}

func TestConversationTime(t *testing.T) {
	withMsg := &Conversation{
		Contact:     &ContactInfo{CreatedAt: time.Unix(100, 0)},
		LastMessage: &ChatMessage{Timestamp: time.Unix(200, 0)},
	}
	assert.Equal(t, time.Unix(200, 0), conversationTime(withMsg))

	withoutMsg := &Conversation{
		Contact: &ContactInfo{CreatedAt: time.Unix(100, 0)},
	}
	assert.Equal(t, time.Unix(100, 0), conversationTime(withoutMsg))
}
