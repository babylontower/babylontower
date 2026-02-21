package cli

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"babylontower/pkg/ipfsnode"
	"babylontower/pkg/messaging"
	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"
	"github.com/ipfs/go-log/v2"
	"github.com/mr-tron/base58"
)

var logger = log.Logger("babylontower/cli")

// timeNow returns the current time (used for testing)
var timeNow = time.Now

// CommandHandler handles CLI commands
type CommandHandler struct {
	storage  storage.Storage
	ipfsNode *ipfsnode.Node
	messaging *messaging.Service
	
	// Identity keys
	ed25519PubKey  ed25519.PublicKey
	ed25519PrivKey ed25519.PrivateKey
	x25519PubKey   []byte
	x25519PrivKey  []byte
	
	// Current chat state
	inChatMode bool
	chatContactPubKey []byte
	chatContactName   string
	
	// Output callback
	output func(string)
}

// NewCommandHandler creates a new command handler
func NewCommandHandler(
	storage storage.Storage,
	ipfsNode *ipfsnode.Node,
	messaging *messaging.Service,
	ed25519PubKey ed25519.PublicKey,
	ed25519PrivKey ed25519.PrivateKey,
	x25519PubKey []byte,
	x25519PrivKey []byte,
	output func(string),
) *CommandHandler {
	return &CommandHandler{
		storage:        storage,
		ipfsNode:       ipfsNode,
		messaging:      messaging,
		ed25519PubKey:  ed25519PubKey,
		ed25519PrivKey: ed25519PrivKey,
		x25519PubKey:   x25519PubKey,
		x25519PrivKey:  x25519PrivKey,
		output:         output,
	}
}

// HandleCommand processes a command and returns true if the app should exit
func (h *CommandHandler) HandleCommand(input string) bool {
	input = strings.TrimSpace(input)
	
	if input == "" {
		return false
	}
	
	// Check if in chat mode
	if h.inChatMode {
		return h.handleChatInput(input)
	}
	
	// Parse command
	if !strings.HasPrefix(input, "/") {
		h.output(FormatInfo("Not a command. Type /help for available commands."))
		return false
	}
	
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false
	}
	
	cmd := strings.ToLower(parts[0])
	args := parts[1:]
	
	switch cmd {
	case "/help":
		h.handleHelp()
	case "/myid":
		h.handleMyID()
	case "/add":
		h.handleAdd(args)
	case "/list":
		h.handleList()
	case "/chat":
		h.handleChat(args)
	case "/history":
		h.handleHistory(args)
	case "/exit":
		return true
	default:
		h.output(FormatErrorString(fmt.Sprintf("Unknown command: %s. Type /help for help.", cmd)))
	}
	
	return false
}

// handleHelp displays help information
func (h *CommandHandler) handleHelp() {
	h.output(FormatHelp())
}

// handleMyID displays the user's public key
func (h *CommandHandler) handleMyID() {
	hexKey := FormatPublicKey(h.ed25519PubKey)
	base58Key := base58.Encode(h.ed25519PubKey)
	
	h.output(FormatInfo("Your Public Key:"))
	h.output(fmt.Sprintf("  Hex:    %s", hexKey))
	h.output(fmt.Sprintf("  Base58: %s", base58Key))
	h.output("\nShare your public key with contacts so they can message you.")
}

// handleAdd adds a new contact
func (h *CommandHandler) handleAdd(args []string) {
	if len(args) == 0 {
		h.output(FormatErrorString("Usage: /add <pubkey> [nickname]"))
		h.output(FormatInfo("Public key can be in hex or base58 format."))
		return
	}

	pubKeyStr := args[0]
	var pubKey []byte
	var err error

	// Try to decode as base58 first, then hex
	pubKey, err = base58.Decode(pubKeyStr)
	if err != nil {
		// Try hex decoding
		pubKey, err = hex.DecodeString(pubKeyStr)
		if err != nil {
			h.output(FormatErrorString("Invalid public key format. Use hex or base58."))
			return
		}
	}
	
	// Validate key length
	if len(pubKey) != ed25519.PublicKeySize {
		h.output(FormatErrorString(fmt.Sprintf("Invalid public key length: expected %d bytes, got %d", ed25519.PublicKeySize, len(pubKey))))
		return
	}

	// Check if contact already exists
	existing, err := h.storage.GetContact(pubKey)
	if err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Error checking contacts: %v", err)))
		return
	}
	if existing != nil {
		h.output(FormatInfo("Contact already exists."))
		h.output(FormatContact(0, existing))
		return
	}

	// Get optional nickname
	displayName := ""
	if len(args) > 1 {
		displayName = strings.Join(args[1:], " ")
	}

	// Create contact
	contact := &pb.Contact{
		PublicKey:   pubKey,
		DisplayName: displayName,
		CreatedAt:   uint64(timeNow().Unix()),
	}

	// Store contact
	if err := h.storage.AddContact(contact); err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Failed to add contact: %v", err)))
		return
	}
	
	name := displayName
	if name == "" {
		name = FormatPublicKeyBase58(pubKey)
	}
	h.output(FormatSuccess(fmt.Sprintf("Contact added: %s", name)))
}

// handleList lists all contacts
func (h *CommandHandler) handleList() {
	contacts, err := h.storage.ListContacts()
	if err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Failed to list contacts: %v", err)))
		return
	}

	h.output(FormatContactList(contacts))
}

// handleChat enters chat mode with a contact
func (h *CommandHandler) handleChat(args []string) {
	if len(args) == 0 {
		h.output(FormatErrorString("Usage: /chat <contact>"))
		h.output(FormatInfo("Contact can be specified by index (from /list) or public key."))
		return
	}

	contactArg := args[0]
	var contact *pb.Contact

	// Try to parse as index first
	if idx, err := strconv.Atoi(contactArg); err == nil {
		// It's an index
		contacts, listErr := h.storage.ListContacts()
		if listErr != nil {
			h.output(FormatErrorString(fmt.Sprintf("Failed to list contacts: %v", listErr)))
			return
		}
		if idx < 1 || idx > len(contacts) {
			h.output(FormatErrorString(fmt.Sprintf("Invalid contact index: %d. Use /list to see contacts.", idx)))
			return
		}
		contact = contacts[idx-1]
	} else {
		// Try to decode as public key
		var pubKey []byte
		pubKey, err = base58.Decode(contactArg)
		if err != nil {
			pubKey, err = hex.DecodeString(contactArg)
			if err != nil {
				h.output(FormatErrorString("Invalid contact identifier. Use index or valid public key."))
				return
			}
		}
		contact, err = h.storage.GetContact(pubKey)
		if err != nil {
			h.output(FormatErrorString(fmt.Sprintf("Failed to find contact: %v", err)))
			return
		}
		if contact == nil {
			h.output(FormatErrorString("Contact not found. Use /add to add them first."))
			return
		}
	}
	
	// Enter chat mode
	h.inChatMode = true
	h.chatContactPubKey = contact.PublicKey
	h.chatContactName = contact.DisplayName
	if h.chatContactName == "" {
		h.chatContactName = FormatPublicKeyBase58(contact.PublicKey)
	}
	
	h.output(FormatChatHeader(h.chatContactName, FormatPublicKeyBase58(contact.PublicKey)))
}

// handleChatInput handles input while in chat mode
func (h *CommandHandler) handleChatInput(input string) bool {
	// Empty line exits chat mode
	if strings.TrimSpace(input) == "" {
		h.inChatMode = false
		h.chatContactPubKey = nil
		h.chatContactName = ""
		h.output(FormatChatExit())
		return false
	}

	// Send message
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
	
	// For PoC, we need the recipient's X25519 public key
	// This is a limitation - in a full implementation, we'd store it with the contact
	// For now, we'll return an error explaining the limitation
	recipientX25519Key, err := messaging.GetContactX25519PubKey(h.chatContactPubKey)
	if err != nil {
		// For PoC demo, explain the limitation
		return fmt.Errorf("recipient X25519 key not available (PoC limitation - contacts need to share X25519 keys)")
	}
	
	// Create the encrypted message
	signedEnvelope, err := messaging.CreateOutgoingMessage(
		text,
		recipientX25519Key,
		h.ed25519PrivKey,
	)
	if err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}
	
	// Serialize envelope
	envelopeBytes, err := messaging.SerializeEnvelope(signedEnvelope)
	if err != nil {
		return fmt.Errorf("failed to serialize envelope: %w", err)
	}
	
	// Add to IPFS to get CID
	cidStr, err := h.ipfsNode.Add(envelopeBytes)
	if err != nil {
		return fmt.Errorf("failed to add to IPFS: %w", err)
	}

	// Get recipient's topic
	topic := ipfsnode.TopicFromPublicKey(h.chatContactPubKey)

	// Publish CID via PubSub
	if err := h.ipfsNode.Publish(topic, []byte(cidStr)); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}
	
	// Store message locally
	if err := h.storage.AddMessage(h.chatContactPubKey, signedEnvelope); err != nil {
		// Log but don't fail - message was sent
		logger.Warnw("failed to store sent message", "error", err)
	}
	
	// Display sent message
	msg := &pb.Message{
		Text:      text,
		Timestamp: uint64(timeNow().Unix()),
	}
	h.output(FormatMessage(msg, h.chatContactName, true))
	
	return nil
}

// handleHistory shows message history with a contact
func (h *CommandHandler) handleHistory(args []string) {
	if len(args) == 0 {
		h.output(FormatErrorString("Usage: /history <contact> [limit]"))
		h.output(FormatInfo("Contact can be specified by index or public key."))
		return
	}

	// Parse limit
	limit := 10
	if len(args) > 1 {
		if l, err := strconv.Atoi(args[1]); err == nil && l > 0 {
			limit = l
		}
	}

	// Find contact
	contactArg := args[0]
	var contact *pb.Contact

	if idx, err := strconv.Atoi(contactArg); err == nil {
		contacts, listErr := h.storage.ListContacts()
		if listErr != nil {
			h.output(FormatErrorString(fmt.Sprintf("Failed to list contacts: %v", listErr)))
			return
		}
		if idx < 1 || idx > len(contacts) {
			h.output(FormatErrorString(fmt.Sprintf("Invalid contact index: %d", idx)))
			return
		}
		contact = contacts[idx-1]
	} else {
		var pubKey []byte
		var err error
		pubKey, err = base58.Decode(contactArg)
		if err != nil {
			pubKey, err = hex.DecodeString(contactArg)
			if err != nil {
				h.output(FormatErrorString("Invalid contact identifier."))
				return
			}
		}
		contact, err = h.storage.GetContact(pubKey)
		if err != nil {
			h.output(FormatErrorString(fmt.Sprintf("Failed to find contact: %v", err)))
			return
		}
		if contact == nil {
			h.output(FormatErrorString("Contact not found."))
			return
		}
	}
	
	// Get messages
	envelopes, err := h.storage.GetMessages(contact.PublicKey, limit, 0)
	if err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Failed to get messages: %v", err)))
		return
	}
	
	if len(envelopes) == 0 {
		h.output(FormatInfo(fmt.Sprintf("No messages with %s.", contact.DisplayName)))
		return
	}
	
	// Display messages
	h.output(fmt.Sprintf("\n=== History with %s ===\n", contact.DisplayName))
	
	// For PoC, we can't decrypt messages without the full flow
	// Show placeholder messages
	for _, env := range envelopes {
		// Determine if message is outgoing by comparing sender pubkey
		isOutgoing := string(env.SenderPubkey) == string(h.ed25519PubKey)
		h.output(FormatMessageFromEnvelope(env, contact.DisplayName, isOutgoing))
	}
	
	h.output("==========================\n")
}

// GetChatContactPubKey returns the current chat contact's public key
func (h *CommandHandler) GetChatContactPubKey() []byte {
	return h.chatContactPubKey
}

// IsInChatMode returns true if in chat mode
func (h *CommandHandler) IsInChatMode() bool {
	return h.inChatMode
}
