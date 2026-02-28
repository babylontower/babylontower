package cli

import (
	"fmt"
	"strconv"
	"strings"

	pb "babylontower/pkg/proto"
)

// handleChat enters chat mode with a contact
func (h *CommandHandler) handleChat(args []string) {
	if len(args) == 0 {
		h.output(FormatErrorString("Usage: /chat <contact>"))
		h.output(FormatInfo("Contact can be specified by index (from /list) or public key."))
		return
	}

	contact, err := h.findContact(args[0])
	if err != nil {
		h.output(FormatErrorString(err.Error()))
		return
	}

	h.inChatMode = true
	h.chatContactPubKey = contact.PublicKey
	h.chatContactName = contact.DisplayName
	if h.chatContactName == "" {
		h.chatContactName = FormatPublicKeyBase58(contact.PublicKey)
	}

	h.output(FormatChatHeader(h.chatContactName, FormatPublicKeyBase58(contact.PublicKey)))
	h.loadAndDisplayHistory(contact.PublicKey, 20)
}

// handleChatInput handles input while in chat mode
func (h *CommandHandler) handleChatInput(input string) bool {
	if strings.TrimSpace(input) == "" {
		h.inChatMode = false
		h.chatContactPubKey = nil
		h.chatContactName = ""
		h.output(FormatChatExit())
		return false
	}

	if err := h.sendMessage(input); err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Failed to send message: %v", err)))
		return false
	}

	return false
}

// sendMessage sends a message to the current chat contact
func (h *CommandHandler) sendMessage(text string) error {
	if h.chatContactPubKey == nil {
		return fmt.Errorf("no active chat")
	}

	contact, err := h.storage.GetContact(h.chatContactPubKey)
	if err != nil {
		return fmt.Errorf("failed to get contact: %w", err)
	}
	if contact == nil {
		return fmt.Errorf("contact not found")
	}

	if len(contact.X25519PublicKey) != 32 {
		return fmt.Errorf("recipient X25519 key not available - ask contact to share their X25519 public key")
	}

	result, err := h.messaging.SendMessageToContact(text, h.chatContactPubKey, contact.X25519PublicKey)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	msg := &pb.Message{
		Text:      text,
		Timestamp: uint64(timeNow().Unix()),
	}
	h.output(FormatMessage(msg, h.chatContactName, true))

	logger.Debugw("message sent via service", "cid", result.CID, "text_len", len(text))
	return nil
}

// handleHistory shows message history with a contact
func (h *CommandHandler) handleHistory(args []string) {
	if len(args) == 0 {
		h.output(FormatErrorString("Usage: /history <contact> [limit]"))
		h.output(FormatInfo("Contact can be specified by index or public key."))
		return
	}

	limit := 10
	if len(args) > 1 {
		if l, err := strconv.Atoi(args[1]); err == nil && l > 0 {
			limit = l
		}
	}

	contact, err := h.findContact(args[0])
	if err != nil {
		h.output(FormatErrorString(err.Error()))
		return
	}

	messages, err := h.messaging.GetDecryptedMessagesWithMeta(contact.PublicKey, limit, 0)
	if err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Failed to get messages: %v", err)))
		return
	}

	if len(messages) == 0 {
		h.output(FormatInfo(fmt.Sprintf("No messages with %s.", contact.DisplayName)))
		return
	}

	h.output(fmt.Sprintf("\n=== History with %s ===\n", contact.DisplayName))

	contactName := contact.DisplayName
	if contactName == "" {
		contactName = FormatPublicKeyBase58(contact.PublicKey)
	}

	for _, msgMeta := range messages {
		h.output(FormatMessage(msgMeta.Message, contactName, msgMeta.IsOutgoing))
	}

	h.output("==========================\n")
}

// loadAndDisplayHistory loads and displays message history for a contact
func (h *CommandHandler) loadAndDisplayHistory(contactPubKey []byte, limit int) {
	messages, err := h.messaging.GetDecryptedMessagesWithMeta(contactPubKey, limit, 0)
	if err != nil {
		logger.Warnw("failed to load history", "error", err)
		return
	}

	if len(messages) == 0 {
		h.output(FormatSystemMessage("No previous messages"))
		return
	}

	h.output(FormatSystemMessage(fmt.Sprintf("Loading %d previous messages...", len(messages))))

	contactName := h.chatContactName
	if contact, err := h.storage.GetContact(contactPubKey); err == nil && contact != nil && contact.DisplayName != "" {
		contactName = contact.DisplayName
	}

	for _, msgMeta := range messages {
		h.output(FormatMessage(msgMeta.Message, contactName, msgMeta.IsOutgoing))
	}

	if len(messages) == limit {
		h.output(FormatSystemMessage(fmt.Sprintf("Showing last %d messages", limit)))
	}
}

// findContact finds a contact by index or public key
func (h *CommandHandler) findContact(contactArg string) (*pb.Contact, error) {
	if idx, err := strconv.Atoi(contactArg); err == nil {
		contacts, listErr := h.storage.ListContacts()
		if listErr != nil {
			return nil, fmt.Errorf("failed to list contacts: %v", listErr)
		}
		if idx < 1 || idx > len(contacts) {
			return nil, fmt.Errorf("invalid contact index: %d. Use /list to see contacts", idx)
		}
		return contacts[idx-1], nil
	}

	pubKey, err := decodePublicKey(contactArg)
	if err != nil {
		return nil, fmt.Errorf("invalid contact identifier. Use index or valid public key")
	}

	contact, err := h.storage.GetContact(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to find contact: %v", err)
	}
	if contact == nil {
		return nil, fmt.Errorf("contact not found. Use /add to add them first")
	}

	return contact, nil
}

// GetChatContactPubKey returns the current chat contact's public key
func (h *CommandHandler) GetChatContactPubKey() []byte {
	return h.chatContactPubKey
}

// IsInChatMode returns true if in chat mode
func (h *CommandHandler) IsInChatMode() bool {
	return h.inChatMode
}
