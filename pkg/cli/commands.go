package cli

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"babylontower/pkg/groups"
	"babylontower/pkg/ipfsnode"
	"babylontower/pkg/messaging"
	"babylontower/pkg/storage"
	"github.com/ipfs/go-log/v2"
	"github.com/mr-tron/base58"
)

var logger = log.Logger("babylontower/cli")

// timeNow returns the current time (used for testing)
var timeNow = time.Now

// CommandHandler handles CLI commands
type CommandHandler struct {
	storage   storage.Storage
	ipfsNode  *ipfsnode.Node
	messaging *messaging.Service
	groups    *groups.Service

	ed25519PubKey  ed25519.PublicKey
	ed25519PrivKey ed25519.PrivateKey
	x25519PubKey   []byte
	x25519PrivKey  []byte

	inChatMode        bool
	chatContactPubKey []byte
	chatContactName   string

	inInteractiveMode bool
	interactiveCmd    string
	interactiveCancel chan struct{}
	interactiveDone   chan struct{}

	output func(string)
}

// NewCommandHandler creates a new command handler
func NewCommandHandler(
	storage storage.Storage,
	ipfsNode *ipfsnode.Node,
	messaging *messaging.Service,
	groupsSvc *groups.Service,
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
		groups:         groupsSvc,
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

	if h.inInteractiveMode {
		return h.handleInteractiveInput(input)
	}

	if h.inChatMode {
		return h.handleChatInput(input)
	}

	if input == "" {
		return false
	}

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
	case "/connect":
		h.handleConnect(args)
	case "/find":
		h.handleFind(args)
	case "/advertise":
		h.handleAdvertise()
	case "/bootstrap", "/bs":
		h.handleBootstrap()
	case "/reconnect", "/retry":
		h.handleReconnect()
	case "/debug", "/netdebug":
		h.handleNetDebug()
	case "/ipfslogs", "/ipfs", "/netstatus":
		h.handleIPFSLogs(args)
	case "/netlog", "/netinfo":
		h.handleNetLog(args)
	case "/peers":
		h.handlePeers()
	case "/myaddr":
		h.handleMyAddr()
	case "/dht":
		h.handleDHT()
	case "/dhtinfo":
		h.handleDHTInfo()
	case "/waitdht":
		h.handleWaitDHT(args)
	case "/mdns":
		h.handleMDNS()
	case "/network", "/netmetrics":
		h.handleNetworkStatus()
	case "/contactstatus", "/contacts":
		h.handleContactStatus()
	case "/creategroup":
		h.handleCreateGroup(args)
	case "/listgroups":
		h.handleListGroups()
	case "/invite":
		h.handleInvite(args)
	case "/groupchat":
		h.handleGroupChat(args)
	case "/reputation", "/rep":
		h.handleReputation(args)
	case "/exit":
		return true
	default:
		h.output(FormatErrorString(fmt.Sprintf("Unknown command: %s. Type /help for help.", cmd)))
	}

	return false
}

// handleInteractiveInput handles input while in interactive mode
func (h *CommandHandler) handleInteractiveInput(input string) bool {
	if strings.TrimSpace(input) == "" {
		h.exitInteractiveMode()
		return false
	}
	return false
}

// exitInteractiveMode stops the interactive display
func (h *CommandHandler) exitInteractiveMode() {
	h.inInteractiveMode = false
	if h.interactiveCancel != nil {
		close(h.interactiveCancel)
		h.interactiveCancel = nil
	}
	if h.interactiveDone != nil {
		<-h.interactiveDone
		h.interactiveDone = nil
	}
	h.output("\nExited interactive mode.")
}

// handleHelp displays help information
func (h *CommandHandler) handleHelp() {
	h.output(FormatHelp())
}

// handleCreateGroup creates a new private group
func (h *CommandHandler) handleCreateGroup(args []string) {
	if h.groups == nil {
		h.output(FormatErrorString("Groups service not available"))
		return
	}

	if len(args) < 2 {
		h.output(FormatErrorString("Usage: /creategroup <name> <description>"))
		return
	}

	name := args[0]
	description := strings.Join(args[1:], " ")

	state, err := h.groups.CreateGroup(name, description, groups.PrivateGroup)
	if err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Failed to create group: %v", err)))
		return
	}

	h.output(FormatSuccess(fmt.Sprintf("Created group '%s' (ID: %x)", state.Name, state.GroupID[:8])))
	h.output(FormatInfo(fmt.Sprintf("Epoch: %d, Members: %d", state.Epoch, len(state.Members))))
}

// handleListGroups lists all groups
func (h *CommandHandler) handleListGroups() {
	if h.groups == nil {
		h.output(FormatErrorString("Groups service not available"))
		return
	}

	groupStates := h.groups.ListGroups()
	if len(groupStates) == 0 {
		h.output(FormatInfo("No groups found."))
		return
	}

	var sb strings.Builder
	sb.WriteString("\n=== Groups ===\n")
	for i, state := range groupStates {
		sb.WriteString(fmt.Sprintf("[%d] %s (Epoch: %d, Members: %d)\n", 
			i+1, state.Name, state.Epoch, len(state.Members)))
		sb.WriteString(fmt.Sprintf("    ID: %x\n", state.GroupID[:8]))
		sb.WriteString(fmt.Sprintf("    Description: %s\n", state.Description))
	}
	sb.WriteString("================\n")
	h.output(sb.String())
}

// handleInvite invites a member to a group
func (h *CommandHandler) handleInvite(args []string) {
	if h.groups == nil {
		h.output(FormatErrorString("Groups service not available"))
		return
	}

	if len(args) < 3 {
		h.output(FormatErrorString("Usage: /invite <group_id_hex> <member_pubkey_base58> [display_name]"))
		return
	}

	groupIDHex := args[0]
	memberPubkeyBase58 := args[1]
	displayName := "Member"
	if len(args) > 2 {
		displayName = strings.Join(args[2:], " ")
	}

	// Parse group ID from hex
	groupID, err := hex.DecodeString(groupIDHex)
	if err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Invalid group ID (must be hex): %v", err)))
		return
	}

	// Parse member public key from base58
	memberPubkey, err := base58.Decode(memberPubkeyBase58)
	if err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Invalid public key (must be base58): %v", err)))
		return
	}

	// For now, use a placeholder X25519 key (this needs to be fetched from the contact)
	memberX25519Pubkey := make([]byte, 32)

	state, err := h.groups.AddMember(groupID, memberPubkey, memberX25519Pubkey, displayName, groups.Member)
	if err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Failed to invite member: %v", err)))
		return
	}

	h.output(FormatSuccess(fmt.Sprintf("Invited %s to group '%s'", displayName, state.Name)))
}

// handleGroupChat enters chat mode with a group
func (h *CommandHandler) handleGroupChat(args []string) {
	if h.groups == nil {
		h.output(FormatErrorString("Groups service not available"))
		return
	}

	if len(args) < 1 {
		h.output(FormatErrorString("Usage: /groupchat <group_id_hex>"))
		return
	}

	groupIDHex := args[0]
	groupID, err := hex.DecodeString(groupIDHex)
	if err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Invalid group ID (must be hex): %v", err)))
		return
	}

	state, err := h.groups.GetGroup(groupID)
	if err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Group not found: %v", err)))
		return
	}

	h.output(FormatSuccess(fmt.Sprintf("Entered chat with group '%s'", state.Name)))
	h.output(FormatInfo("Type a message to send, or empty line to exit"))
	
	// Note: Full group chat implementation would set chat mode similar to handleChat
	// For the PoC, we just show the group info
}
