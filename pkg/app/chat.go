package app

import (
	"encoding/hex"
	"fmt"
	"time"

	"babylontower/pkg/messaging"
	pb "babylontower/pkg/proto"

	"github.com/mr-tron/base58"
)

// MessageStatus represents the delivery state of an outgoing message.
type MessageStatus int

const (
	// StatusSending means the message is being sent
	StatusSending MessageStatus = iota
	// StatusSent means the message was published to PubSub
	StatusSent
	// StatusDelivered means the recipient's device received it
	StatusDelivered
	// StatusRead means the recipient has read it
	StatusRead
	// StatusFailed means sending failed
	StatusFailed
)

// ChatMessage is a UI-friendly message representation.
type ChatMessage struct {
	// Text is the message content
	Text string
	// Timestamp is when the message was sent/received
	Timestamp time.Time
	// IsOutgoing is true if we sent this message
	IsOutgoing bool
	// SenderName is the display name of the sender (contact name or "You")
	SenderName string
	// ContactPubKeyBase58 is the other party's public key
	ContactPubKeyBase58 string

	// ── Rich message fields (Phase UI-5) ──

	// MessageID is the unique identifier for this message (hex)
	MessageID string
	// Status is the delivery status (outgoing only)
	Status MessageStatus
	// IsEdited indicates the message has been edited
	IsEdited bool
	// EditedAt is when the message was last edited
	EditedAt *time.Time
	// IsDeleted indicates the message has been deleted (tombstone)
	IsDeleted bool
	// Reactions maps emoji to count
	Reactions map[string]int
	// ReplyToText is the text of the message being replied to (if any)
	ReplyToText string
	// ReplyToSender is the sender name of the replied message
	ReplyToSender string
}

// Conversation represents a chat conversation for the conversation list.
type Conversation struct {
	// Contact is the conversation partner's info
	Contact *ContactInfo
	// LastMessage is the most recent message (nil if no messages)
	LastMessage *ChatMessage
	// MessageCount is the total number of messages in the conversation
	MessageCount int
}

// SendMessageResult contains the result of sending a message.
type SendMessageResult struct {
	// Timestamp is when the message was sent
	Timestamp time.Time
	// CID is the IPFS content identifier
	CID string
}

// IncomingEventType distinguishes incoming event kinds.
type IncomingEventType int

const (
	// IncomingText is a regular text message
	IncomingText IncomingEventType = iota
	// IncomingReaction is an emoji reaction to a message
	IncomingReaction
	// IncomingEdit is an edit to an existing message
	IncomingEdit
	// IncomingDelete is a deletion of an existing message
	IncomingDelete
	// IncomingReadReceipt indicates messages were read
	IncomingReadReceipt
	// IncomingDeliveryReceipt indicates messages were delivered
	IncomingDeliveryReceipt
	// IncomingTyping is a typing indicator event
	IncomingTyping
)

// IncomingMessage is a UI-friendly incoming message event.
type IncomingMessage struct {
	// EventType distinguishes the kind of event (default: IncomingText)
	EventType IncomingEventType
	// ContactPubKeyBase58 is the sender's public key in base58
	ContactPubKeyBase58 string
	// ContactName is the sender's display name (empty if unknown)
	ContactName string
	// Text is the message content (for text messages and edits)
	Text string
	// Timestamp is when the message was received
	Timestamp time.Time
	// TargetMessageID is the hex ID of the target message (for reactions, edits, deletes)
	TargetMessageID string
	// Emoji is the reaction emoji (for reaction events)
	Emoji string
	// RemoveReaction is true if removing a reaction
	RemoveReaction bool
	// IsTyping is true if the contact started typing, false if stopped
	IsTyping bool
	// MessageIDs are the affected message IDs (for receipts)
	MessageIDs []string
}

// ChatManager provides high-level chat operations for UI.
type ChatManager interface {
	// SendMessage sends a text message to a contact identified by pubkey string.
	SendMessage(contactPubKeyStr, text string) (*SendMessageResult, error)

	// GetMessages returns message history with a contact.
	// Messages are returned in chronological order (oldest first).
	GetMessages(contactPubKeyStr string, limit, offset int) ([]*ChatMessage, error)

	// GetConversations returns all conversations with their last message
	// and online status. Sorted by most recent message first.
	GetConversations() ([]*Conversation, error)

	// GetConversationsLight returns conversations without querying contact
	// online status (skips expensive DHT/PubSub queries). Use this for
	// frequent refreshes where status is not needed.
	GetConversationsLight() ([]*Conversation, error)

	// DeleteConversation deletes all messages with a contact.
	DeleteConversation(contactPubKeyStr string) error

	// RetrieveOfflineMessages retrieves messages from mailbox nodes.
	// Returns the number of messages retrieved.
	RetrieveOfflineMessages() (int, error)

	// IncomingMessages returns a channel of incoming messages.
	// The channel delivers UI-friendly IncomingMessage events.
	// The channel remains open for the lifetime of the application.
	IncomingMessages() <-chan *IncomingMessage
}

// chatManager implements ChatManager.
type chatManager struct {
	app       *application
	incomingCh chan *IncomingMessage
}

// newChatManager creates a chat manager and starts the message relay goroutine.
func newChatManager(app *application) *chatManager {
	cm := &chatManager{
		app:        app,
		incomingCh: make(chan *IncomingMessage, 100),
	}
	// Start goroutine to transform raw message events into UI-friendly events
	go cm.relayMessages()
	return cm
}

func (cm *chatManager) SendMessage(contactPubKeyStr, text string) (*SendMessageResult, error) {
	pubKey, err := decodePubKey(contactPubKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid contact key: %w", err)
	}

	if cm.app.messaging == nil || !cm.app.messaging.IsStarted() {
		return nil, fmt.Errorf("messaging service not started")
	}

	// Look up contact to get X25519 key
	contact, err := cm.app.storage.GetContact(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get contact: %w", err)
	}
	if contact == nil {
		return nil, fmt.Errorf("contact not found: add contact first with Contacts().AddContact()")
	}
	if len(contact.X25519PublicKey) != 32 {
		return nil, fmt.Errorf("contact has no encryption key: ask them to share their X25519 public key")
	}

	result, err := cm.app.messaging.SendMessageToContact(text, pubKey, contact.X25519PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	return &SendMessageResult{
		Timestamp: time.Now(),
		CID:       result.CID,
	}, nil
}

func (cm *chatManager) GetMessages(contactPubKeyStr string, limit, offset int) ([]*ChatMessage, error) {
	pubKey, err := decodePubKey(contactPubKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid contact key: %w", err)
	}

	if cm.app.messaging == nil || !cm.app.messaging.IsStarted() {
		return nil, fmt.Errorf("messaging service not started")
	}

	// Get contact display name
	contactName := ""
	if contact, err := cm.app.storage.GetContact(pubKey); err == nil && contact != nil {
		contactName = contact.DisplayName
	}
	if contactName == "" {
		contactName = base58.Encode(pubKey)
	}

	messages, err := cm.app.messaging.GetDecryptedMessagesWithMeta(pubKey, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	pubKeyBase58 := base58.Encode(pubKey)
	result := make([]*ChatMessage, 0, len(messages))
	for _, m := range messages {
		result = append(result, chatMessageFromMeta(m, contactName, pubKeyBase58))
	}
	return result, nil
}

func (cm *chatManager) GetConversations() ([]*Conversation, error) {
	return cm.GetConversationsWithStatuses(nil)
}

func (cm *chatManager) GetConversationsLight() ([]*Conversation, error) {
	// Pass an empty map to skip the expensive GetAllContactStatuses call.
	return cm.GetConversationsWithStatuses(make(map[string]statusInfo))
}

// GetConversationsWithStatuses returns conversations using an optional pre-fetched
// status map. Pass nil to skip status enrichment (faster for frequent refreshes).
func (cm *chatManager) GetConversationsWithStatuses(statusMap map[string]statusInfo) ([]*Conversation, error) {
	contacts, err := cm.app.storage.ListContacts()
	if err != nil {
		return nil, fmt.Errorf("failed to list contacts: %w", err)
	}

	if cm.app.messaging == nil || !cm.app.messaging.IsStarted() {
		return nil, fmt.Errorf("messaging service not started")
	}

	// Only fetch statuses if not pre-provided
	if statusMap == nil {
		statuses, err := cm.app.messaging.GetAllContactStatuses()
		if err == nil {
			statusMap = make(map[string]statusInfo, len(statuses))
			for _, s := range statuses {
				statusMap[hex.EncodeToString(s.PubKey)] = statusInfo{
					isOnline:  s.IsOnline,
					connected: s.Connected,
					peerID:    s.PeerID,
				}
			}
		}
	}

	conversations := make([]*Conversation, 0, len(contacts))
	for _, c := range contacts {
		info := contactInfoFromProto(c)
		if statusMap != nil {
			if s, ok := statusMap[info.PublicKeyHex]; ok {
				info.IsOnline = s.isOnline
				info.Connected = s.connected
				info.PeerID = s.peerID
			}
		}

		conv := &Conversation{
			Contact: info,
		}

		// Get last message only (skip expensive message count)
		msgs, err := cm.app.messaging.GetDecryptedMessagesWithMeta(c.PublicKey, 1, 0)
		if err == nil && len(msgs) > 0 {
			contactName := c.DisplayName
			if contactName == "" {
				contactName = info.PublicKeyBase58
			}
			conv.LastMessage = chatMessageFromMeta(msgs[0], contactName, info.PublicKeyBase58)
		}

		conversations = append(conversations, conv)
	}

	// Sort by most recent message first
	sortConversations(conversations)
	return conversations, nil
}

func (cm *chatManager) DeleteConversation(contactPubKeyStr string) error {
	pubKey, err := decodePubKey(contactPubKeyStr)
	if err != nil {
		return fmt.Errorf("invalid contact key: %w", err)
	}
	return cm.app.storage.DeleteMessages(pubKey)
}

func (cm *chatManager) RetrieveOfflineMessages() (int, error) {
	if cm.app.messaging == nil || !cm.app.messaging.IsStarted() {
		return 0, fmt.Errorf("messaging service not started")
	}

	err := cm.app.messaging.RetrieveOfflineMessages()
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve offline messages: %w", err)
	}

	// The messaging service processes retrieved messages through the normal
	// incoming message pipeline, so they'll arrive via IncomingMessages().
	// We can't know the exact count here since processing is async.
	return 0, nil
}

func (cm *chatManager) IncomingMessages() <-chan *IncomingMessage {
	return cm.incomingCh
}

// relayMessages transforms raw MessageEvents into UI-friendly IncomingMessages.
func (cm *chatManager) relayMessages() {
	if cm.app.messaging == nil {
		logger.Warn("relayMessages: messaging service is nil, exiting")
		return
	}

	rawCh := cm.app.messaging.Messages()
	if rawCh == nil {
		logger.Warn("relayMessages: Messages() returned nil channel, exiting")
		return
	}

	logger.Info("relayMessages: started reading from messaging channel")

	for evt := range rawCh {
		if evt == nil || evt.Message == nil {
			continue
		}

		contactName := ""
		pubKeyBase58 := base58.Encode(evt.ContactPubKey)
		if contact, err := cm.app.storage.GetContact(evt.ContactPubKey); err == nil && contact != nil {
			contactName = contact.DisplayName
		}

		logger.Debugw("relayMessages: relaying message to UI",
			"from", pubKeyBase58[:min(16, len(pubKeyBase58))],
			"text_len", len(evt.Message.Text))

		msg := &IncomingMessage{
			ContactPubKeyBase58: pubKeyBase58,
			ContactName:         contactName,
			Text:                evt.Message.Text,
			Timestamp:           time.Unix(int64(evt.Message.Timestamp), 0),
		}

		select {
		case cm.incomingCh <- msg:
		default:
			// Drop if channel full to avoid blocking the messaging pipeline
			logger.Warnw("incoming message channel full, dropping UI event")
		}
	}

	logger.Warn("relayMessages: messaging channel closed, exiting")
}

// chatMessageFromMeta converts a MessageWithMeta to a ChatMessage.
func chatMessageFromMeta(m *messaging.MessageWithMeta, contactName, contactPubKeyBase58 string) *ChatMessage {
	senderName := contactName
	if m.IsOutgoing {
		senderName = "You"
	}
	return &ChatMessage{
		Text:                m.Message.Text,
		Timestamp:           time.Unix(int64(m.Message.Timestamp), 0),
		IsOutgoing:          m.IsOutgoing,
		SenderName:          senderName,
		ContactPubKeyBase58: contactPubKeyBase58,
	}
}

// sortConversations sorts by most recent message first.
// Conversations without messages are placed at the end.
func sortConversations(convs []*Conversation) {
	for i := 1; i < len(convs); i++ {
		for j := i; j > 0; j-- {
			if conversationIsNewer(convs[j], convs[j-1]) {
				convs[j], convs[j-1] = convs[j-1], convs[j]
			}
		}
	}
}

func conversationIsNewer(a, b *Conversation) bool {
	aTime := conversationTime(a)
	bTime := conversationTime(b)
	return aTime.After(bTime)
}

func conversationTime(c *Conversation) time.Time {
	if c.LastMessage != nil {
		return c.LastMessage.Timestamp
	}
	return c.Contact.CreatedAt
}

// Ensure pb.Message is used (for field access)
var _ = (*pb.Message)(nil)
