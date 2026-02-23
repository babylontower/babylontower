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

	// Interactive command state
	inInteractiveMode   bool
	interactiveCmd      string
	interactiveCancel   chan struct{}
	interactiveDone     chan struct{}

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

	// Check if in interactive mode first (empty line exits)
	if h.inInteractiveMode {
		return h.handleInteractiveInput(input)
	}

	// Check if in chat mode first (empty line exits chat)
	if h.inChatMode {
		return h.handleChatInput(input)
	}

	// Empty input when not in chat mode - ignore
	if input == "" {
		return false
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
	case "/exit":
		return true
	default:
		h.output(FormatErrorString(fmt.Sprintf("Unknown command: %s. Type /help for help.", cmd)))
	}

	return false
}

// handleInteractiveInput handles input while in interactive mode
func (h *CommandHandler) handleInteractiveInput(input string) bool {
	// Empty line exits interactive mode
	if strings.TrimSpace(input) == "" {
		h.exitInteractiveMode()
		return false
	}

	// In interactive mode, just wait - no other commands processed
	return false
}

// exitInteractiveMode stops the interactive display
func (h *CommandHandler) exitInteractiveMode() {
	h.inInteractiveMode = false
	if h.interactiveCancel != nil {
		close(h.interactiveCancel)
		h.interactiveCancel = nil
	}
	// Wait for interactive goroutine to finish
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

// handleMyID displays the user's public keys (Ed25519 and X25519)
func (h *CommandHandler) handleMyID() {
	ed25519Hex := FormatPublicKey(h.ed25519PubKey)
	ed25519Base58 := base58.Encode(h.ed25519PubKey)
	x25519Hex := hex.EncodeToString(h.x25519PubKey)
	x25519Base58 := base58.Encode(h.x25519PubKey)

	h.output(FormatInfo("Your Public Keys:"))
	h.output("")
	h.output("Ed25519 (for signatures and verification):")
	h.output(fmt.Sprintf("  Hex:    %s", ed25519Hex))
	h.output(fmt.Sprintf("  Base58: %s", ed25519Base58))
	h.output("")
	h.output("X25519 (for encryption - share this with contacts):")
	h.output(fmt.Sprintf("  Hex:    %s", x25519Hex))
	h.output(fmt.Sprintf("  Base58: %s", x25519Base58))
	h.output("")
	h.output(FormatInfo("Share your X25519 public key with contacts so they can encrypt messages to you."))
	h.output(FormatInfo("Your Ed25519 key is used to verify your signatures."))
	h.output("")
	
	// Show multiaddr for direct connection
	if h.ipfsNode != nil && h.ipfsNode.IsStarted() {
		addrs := h.ipfsNode.Multiaddrs()
		if len(addrs) > 0 {
			h.output(FormatInfo("Your Node Multiaddr (for /connect command):"))
			peerID := h.ipfsNode.PeerID()
			for _, addr := range addrs {
				h.output(fmt.Sprintf("  %s/p2p/%s", addr, peerID))
			}
		}
	}
}

// handleAdd adds a new contact
func (h *CommandHandler) handleAdd(args []string) {
	if len(args) == 0 {
		h.output(FormatErrorString("Usage: /add <pubkey> [nickname]"))
		h.output(FormatInfo("Public key can be in hex or base58 format."))
		h.output(FormatInfo("To enable encryption, also share your X25519 key:"))
		h.output(FormatInfo("  /add <ed25519_pubkey> <nickname> <x25519_pubkey>"))
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

	// Get optional X25519 public key (last argument if it looks like a key)
	var x25519PubKey []byte
	if len(args) >= 2 {
		// Check if last argument looks like an X25519 key (64 hex chars or 32+ base58 chars)
		lastArg := args[len(args)-1]
		if potentialKey, err := hex.DecodeString(lastArg); err == nil && len(potentialKey) == 32 {
			x25519PubKey = potentialKey
			// Remove X25519 key from nickname
			if len(args) > 2 {
				displayName = strings.Join(args[1:len(args)-1], " ")
			} else {
				displayName = ""
			}
		} else if potentialKey, err := base58.Decode(lastArg); err == nil && len(potentialKey) == 32 {
			x25519PubKey = potentialKey
			// Remove X25519 key from nickname
			if len(args) > 2 {
				displayName = strings.Join(args[1:len(args)-1], " ")
			} else {
				displayName = ""
			}
		}
	}

	// Create contact
	contact := &pb.Contact{
		PublicKey:       pubKey,
		DisplayName:     displayName,
		CreatedAt:       uint64(timeNow().Unix()),
		X25519PublicKey: x25519PubKey,
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
	
	if len(x25519PubKey) > 0 {
		h.output(FormatSuccess(fmt.Sprintf("Contact added: %s (with encryption)", name)))
	} else {
		h.output(FormatSuccess(fmt.Sprintf("Contact added: %s", name)))
		h.output(FormatInfo("Note: No X25519 key provided. Message encryption will not work."))
		h.output(FormatInfo("Ask contact to share their X25519 public key."))
	}
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

	// Load and display last 20 messages from history
	h.loadAndDisplayHistory(contact.PublicKey, 20)
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

	// Get contact from storage to retrieve X25519 key
	contact, err := h.storage.GetContact(h.chatContactPubKey)
	if err != nil {
		return fmt.Errorf("failed to get contact: %w", err)
	}
	if contact == nil {
		return fmt.Errorf("contact not found")
	}

	// Check if we have the recipient's X25519 key
	if len(contact.X25519PublicKey) != 32 {
		return fmt.Errorf("recipient X25519 key not available - ask contact to share their X25519 public key")
	}

	// Use the messaging service to send the message
	result, err := h.messaging.SendMessageToContact(text, h.chatContactPubKey, contact.X25519PublicKey)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Display sent message
	msg := &pb.Message{
		Text:      text,
		Timestamp: uint64(timeNow().Unix()),
	}
	h.output(FormatMessage(msg, h.chatContactName, true))

	logger.Debugw("message sent via service", "cid", result.CID, "text", text)

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

	// Get and decrypt messages with metadata
	messages, err := h.messaging.GetDecryptedMessagesWithMeta(contact.PublicKey, limit, 0)
	if err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Failed to get messages: %v", err)))
		return
	}

	if len(messages) == 0 {
		h.output(FormatInfo(fmt.Sprintf("No messages with %s.", contact.DisplayName)))
		return
	}

	// Display messages
	h.output(fmt.Sprintf("\n=== History with %s ===\n", contact.DisplayName))

	contactName := contact.DisplayName
	if contactName == "" {
		contactName = FormatPublicKeyBase58(contact.PublicKey)
	}

	for _, msgMeta := range messages {
		// Use the IsOutgoing flag from metadata
		h.output(FormatMessage(msgMeta.Message, contactName, msgMeta.IsOutgoing))
	}

	h.output("==========================\n")
}

// loadAndDisplayHistory loads and displays message history for a contact
func (h *CommandHandler) loadAndDisplayHistory(contactPubKey []byte, limit int) {
	messages, err := h.messaging.GetDecryptedMessagesWithMeta(contactPubKey, limit, 0)
	if err != nil {
		logger.Debugw("failed to load history", "error", err)
		return
	}

	if len(messages) == 0 {
		h.output(FormatSystemMessage("No previous messages"))
		return
	}

	h.output(FormatSystemMessage(fmt.Sprintf("Loading %d previous messages...", len(messages))))

	// Get contact info for display
	contact, err := h.storage.GetContact(contactPubKey)
	if err != nil {
		logger.Debugw("failed to get contact", "error", err)
		return
	}
	contactName := h.chatContactName
	if contact != nil && contact.DisplayName != "" {
		contactName = contact.DisplayName
	}

	// Display messages with proper sender info
	for _, msgMeta := range messages {
		h.output(FormatMessage(msgMeta.Message, contactName, msgMeta.IsOutgoing))
	}

	if len(messages) == limit {
		h.output(FormatSystemMessage(fmt.Sprintf("Showing last %d messages", limit)))
	}
}

// handleConnect connects to a peer node by multiaddr
func (h *CommandHandler) handleConnect(args []string) {
	if len(args) == 0 {
		h.output(FormatErrorString("Usage: /connect <multiaddr>"))
		h.output(FormatInfo("Multiaddr format: /ip4/127.0.0.1/tcp/4001/p2p/QmPeerID"))
		h.output(FormatInfo("Get multiaddr from other node using /myid"))
		return
	}

	maddr := args[0]

	if err := h.ipfsNode.ConnectToPeer(maddr); err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Failed to connect: %v", err)))
		return
	}

	h.output(FormatSuccess("Connected to peer!"))
}

// handleFind attempts to find and connect to a peer via DHT
func (h *CommandHandler) handleFind(args []string) {
	if len(args) == 0 {
		h.output(FormatErrorString("Usage: /find <peer_id>"))
		h.output(FormatInfo("Peer ID format: 12D3KooW... (base58 encoded)"))
		h.output(FormatInfo("Get peer ID from /myid or /list"))
		return
	}

	peerID := args[0]

	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	// Check DHT routing table first
	dhtInfo := h.ipfsNode.GetDHTInfo()
	if dhtInfo.RoutingTableSize == 0 {
		h.output(FormatErrorString("DHT routing table is empty"))
		h.output("")
		h.output(FormatInfo("Run /waitdht first to wait for DHT bootstrap"))
		h.output(FormatInfo("Or use /connect <multiaddr> for direct connection"))
		return
	}

	h.output(FormatInfo("Searching DHT for peer..."))
	h.output(fmt.Sprintf("Target Peer ID: %s", peerID))
	h.output(fmt.Sprintf("Our routing table has %d peers", dhtInfo.RoutingTableSize))

	peerInfo, err := h.ipfsNode.FindPeer(peerID)
	if err != nil {
		h.output(FormatErrorString(fmt.Sprintf("DHT lookup failed: %v", err)))
		h.output("")
		h.output(FormatInfo("The peer may be:"))
		h.output("  - Not connected to the DHT")
		h.output("  - Behind a NAT without port forwarding")
		h.output("  - Not advertising themselves")
		h.output("")
		h.output(FormatInfo("Try:"))
		h.output("  1. Ask the peer to run /advertise")
		h.output("  2. Get their multiaddr via /myid and use /connect")
		h.output("  3. Run /dhtinfo to check your routing table")
		return
	}

	h.output(FormatSuccess("Found peer via DHT!"))
	h.output("")
	h.output(fmt.Sprintf("Peer ID: %s", peerInfo.ID))
	h.output(fmt.Sprintf("Addresses (%d):", len(peerInfo.Addrs)))
	for i, addr := range peerInfo.Addrs {
		h.output(fmt.Sprintf("  %d. %s/p2p/%s", i+1, addr.String(), peerID))
	}

	// Try to connect
	h.output("")
	h.output(FormatInfo("Attempting to connect..."))

	if err := h.ipfsNode.ConnectToPeer(peerInfo.Addrs[0].String() + "/p2p/" + peerID); err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Connection failed: %v", err)))
		return
	}

	h.output(FormatSuccess("Successfully connected to peer!"))
}

// handleAdvertise advertises our node to the DHT
func (h *CommandHandler) handleAdvertise() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	h.output(FormatInfo("Advertising node to DHT..."))

	ctx := h.ipfsNode.Context()
	if err := h.ipfsNode.AdvertiseSelf(ctx); err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Advertisement failed: %v", err)))
		return
	}

	h.output(FormatSuccess("Successfully advertised to DHT!"))
	h.output("")
	h.output(FormatInfo("Other nodes can now find you via:"))
	h.output("  /find <your_peer_id>")
	h.output("")
	h.output(FormatInfo("Your Peer ID:"))
	h.output(fmt.Sprintf("  %s", h.ipfsNode.PeerID()))
}

// handleBootstrap displays bootstrap peer connection status
func (h *CommandHandler) handleBootstrap() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	info := h.ipfsNode.GetNetworkInfo()

	h.output("\n=== Bootstrap Peer Status ===\n")
	h.output("")
	h.output(fmt.Sprintf("Your Peer ID: %s", h.ipfsNode.PeerID()))
	h.output("")
	h.output(fmt.Sprintf("Connected peers: %d", info.ConnectedPeerCount))
	h.output("")

	if info.ConnectedPeerCount == 0 {
		h.output(FormatErrorString("Not connected to any peers"))
		h.output("")
		h.output(FormatInfo("Bootstrap connection may have failed. Try:"))
		h.output("  1. Check your internet connection")
		h.output("  2. Check firewall settings (outbound TCP)")
		h.output("  3. Wait a few seconds for connection retry")
		h.output("  4. Use /connect <multiaddr> for direct connection")
	} else {
		h.output(FormatSuccess("Connected to bootstrap network"))
		h.output("")
		h.output("Connected peers:")
		for i, peer := range info.ConnectedPeers {
			h.output(fmt.Sprintf("  %d. %s", i+1, peer.ID))
			if len(peer.Addresses) > 0 {
				h.output(fmt.Sprintf("     via: %s", peer.Addresses[0]))
			}
		}
	}

	h.output("")
	h.output("=============================\n")
}

// handleNetDebug displays detailed network debugging information
func (h *CommandHandler) handleNetDebug() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	info := h.ipfsNode.GetNetworkInfo()

	h.output("\n=== Network Debug Information ===\n")
	h.output("")
	h.output(fmt.Sprintf("Peer ID: %s", info.PeerID))
	h.output("")
	h.output("Listen Addresses:")
	for i, addr := range info.ListenAddrs {
		h.output(fmt.Sprintf("  %d. %s", i+1, addr))
	}
	h.output("")
	h.output(fmt.Sprintf("Connected Peers: %d", info.ConnectedPeerCount))
	
	if info.ConnectedPeerCount > 0 {
		h.output("")
		h.output("Connected:")
		for i, peer := range info.ConnectedPeers {
			h.output(fmt.Sprintf("  %d. %s", i+1, peer.ID))
			for j, addr := range peer.Addresses {
				if j == 0 {
					h.output(fmt.Sprintf("     via: %s", addr))
				}
			}
			for j, proto := range peer.Protocols {
				if j == 0 {
					h.output(fmt.Sprintf("     protocols: %s", proto))
				} else {
					h.output(fmt.Sprintf("                %s", proto))
				}
			}
		}
	} else {
		h.output("")
		h.output(FormatErrorString("NOT CONNECTED TO ANY PEERS"))
		h.output("")
		h.output("Possible causes:")
		h.output("  1. Firewall blocking outbound TCP connections")
		h.output("  2. DNS resolution failures")
		h.output("  3. Bootstrap peers unreachable")
		h.output("  4. Network isolation (container/VM)")
		h.output("")
		h.output("Try these commands:")
		h.output("  /connect /ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ")
		h.output("")
		h.output("Or check your firewall:")
		h.output("  Windows: Allow outbound TCP on port 4001")
		h.output("  Linux: Check iptables/ufw rules")
		h.output("  Docker: Ensure network is not isolated")
	}

	h.output("")
	h.output("===================================\n")
}

// handleReconnect attempts to reconnect to bootstrap peers
func (h *CommandHandler) handleReconnect() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	h.output("\n=== Reconnecting to Bootstrap Peers ===\n")
	h.output("")
	h.output(FormatInfo("Attempting to reconnect to bootstrap peers..."))
	h.output("")

	// Try direct connection to known bootstrap peer
	bootstrapAddr := "/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ"
	
	h.output(fmt.Sprintf("Connecting to: %s", bootstrapAddr))
	
	if err := h.ipfsNode.ConnectToPeer(bootstrapAddr); err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Connection failed: %v", err)))
		h.output("")
		h.output(FormatInfo("This peer may be offline. Try another:"))
		h.output("  /connect /ip4/104.236.179.241/tcp/4001/p2p/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM")
	} else {
		h.output(FormatSuccess("Successfully connected!"))
		h.output("")
		h.output(FormatInfo("Peer is now in your routing table."))
		h.output(FormatInfo("Run /bootstrap to verify connection."))
	}
	
	h.output("")
	h.output("====================================\n")
}

// handleIPFSLogs displays IPFS network status and logs interactively
func (h *CommandHandler) handleIPFSLogs(args []string) {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	// Enter interactive mode
	h.inInteractiveMode = true
	h.interactiveCmd = "ipfslogs"
	h.interactiveCancel = make(chan struct{})
	h.interactiveDone = make(chan struct{})

	h.output("\n=== IPFS Network Status (Interactive) ===")
	h.output("Press Enter on an empty line to exit.\n")

	// Start interactive display goroutine
	go h.runInteractiveIPFSLogs()
}

// runInteractiveIPFSLogs continuously displays IPFS network status
func (h *CommandHandler) runInteractiveIPFSLogs() {
	defer close(h.interactiveDone)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.interactiveCancel:
			return
		case <-ticker.C:
			h.displayIPFSStatus()
		}
	}
}

// displayIPFSStatus displays the current IPFS network status
func (h *CommandHandler) displayIPFSStatus() {
	info := h.ipfsNode.GetNetworkInfo()

	h.output(fmt.Sprintf("\n[%s] Peer ID: %s | Connected: %d peers",
		time.Now().Format("15:04:05"),
		truncatePeerID(info.PeerID),
		info.ConnectedPeerCount))

	if info.ConnectedPeerCount > 0 {
		for i, peer := range info.ConnectedPeers {
			if i >= 3 {
				h.output(fmt.Sprintf("  ... and %d more peers", info.ConnectedPeerCount-3))
				break
			}
			h.output(fmt.Sprintf("  → %s", truncatePeerID(peer.ID)))
		}
	}
}

// handleNetLog displays network discovery and connection events interactively
func (h *CommandHandler) handleNetLog(args []string) {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	// Enter interactive mode
	h.inInteractiveMode = true
	h.interactiveCmd = "netlog"
	h.interactiveCancel = make(chan struct{})
	h.interactiveDone = make(chan struct{})

	h.output("\n=== Network Discovery Log (Interactive) ===")
	h.output("Press Enter on an empty line to exit.\n")

	// Start interactive display goroutine
	go h.runInteractiveNetLog()
}

// runInteractiveNetLog continuously displays network discovery status
func (h *CommandHandler) runInteractiveNetLog() {
	defer close(h.interactiveDone)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.interactiveCancel:
			return
		case <-ticker.C:
			h.displayNetLogStatus()
		}
	}
}

// displayNetLogStatus displays the current network discovery status
func (h *CommandHandler) displayNetLogStatus() {
	info := h.ipfsNode.GetNetworkInfo()

	h.output(fmt.Sprintf("\n[%s] Status: Running | Peers: %d | mDNS: ✓ | DHT: ✓",
		time.Now().Format("15:04:05"),
		info.ConnectedPeerCount))

	if info.ConnectedPeerCount == 0 {
		h.output("  Waiting for peer discovery...")
		h.output("  - Start another node on this network")
		h.output("  - Or use /connect <multiaddr>")
	} else {
		for i, peer := range info.ConnectedPeers {
			if i >= 3 {
				h.output(fmt.Sprintf("  ... and %d more", info.ConnectedPeerCount-3))
				break
			}
			h.output(fmt.Sprintf("  [%d] %s", i+1, truncatePeerID(peer.ID)))
		}
	}
}

// truncatePeerID truncates a peer ID for display
func truncatePeerID(peerID string) string {
	if len(peerID) <= 16 {
		return peerID
	}
	return peerID[:8] + "..." + peerID[len(peerID)-4:]
}

// handlePeers displays detailed peer connection information
func (h *CommandHandler) handlePeers() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	info := h.ipfsNode.GetNetworkInfo()

	h.output("\n=== Peer Connections ===\n")
	h.output(fmt.Sprintf("Total connected peers: %d\n", info.ConnectedPeerCount))

	if info.ConnectedPeerCount == 0 {
		h.output(FormatInfo("No peers connected."))
		h.output("")
		h.output("To connect manually:")
		h.output("  1. Get your multiaddr with /myaddr")
		h.output("  2. Share it with the other instance")
		h.output("  3. Use /connect <multiaddr> to connect")
	} else {
		for i, peer := range info.ConnectedPeers {
			h.output(fmt.Sprintf("\nPeer #%d: %s", i+1, peer.ID))
			if len(peer.Addresses) > 0 {
				h.output("  Addresses:")
				for _, addr := range peer.Addresses {
					h.output(fmt.Sprintf("    %s", addr))
				}
			}
			if len(peer.Protocols) > 0 {
				h.output("  Protocols:")
				for _, proto := range peer.Protocols {
					h.output(fmt.Sprintf("    %s", proto))
				}
			}
		}
	}
	h.output("\n=========================\n")
}

// handleMyAddr displays the full multiaddr for this node
func (h *CommandHandler) handleMyAddr() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	peerID := h.ipfsNode.PeerID()
	addrs := h.ipfsNode.Multiaddrs()

	h.output("\n=== Your Node Multiaddrs ===\n")
	h.output(fmt.Sprintf("Peer ID: %s\n", peerID))
	h.output("Multiaddrs (share these with peers):")

	for _, addr := range addrs {
		fullAddr := fmt.Sprintf("%s/p2p/%s", addr, peerID)
		h.output(fmt.Sprintf("  %s", fullAddr))
	}
	h.output("")
	h.output(FormatInfo("Use /connect <multiaddr> on another instance to connect."))
	h.output("============================\n")
}

// handleDHT displays DHT routing table status
func (h *CommandHandler) handleDHT() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	dhtInfo := h.ipfsNode.GetDHTInfo()

	h.output("\n=== DHT Status ===\n")
	h.output(fmt.Sprintf("Peer ID: %s", h.ipfsNode.PeerID()))
	h.output(fmt.Sprintf("DHT Mode: %s", dhtInfo.Mode))
	h.output(fmt.Sprintf("Routing Table Size: %d peers", dhtInfo.RoutingTableSize))
	h.output(fmt.Sprintf("Connected Peers: %d", dhtInfo.ConnectedPeerCount))
	h.output("")

	if dhtInfo.RoutingTableSize > 0 {
		h.output(FormatSuccess("DHT routing table is populated"))
		h.output("")
		h.output("Routing table peers (first 5):")
		for i, peer := range dhtInfo.RoutingTablePeers {
			if i >= 5 {
				h.output(fmt.Sprintf("  ... and %d more", dhtInfo.RoutingTableSize-5))
				break
			}
			h.output(fmt.Sprintf("  [%d] %s", i+1, truncatePeerID(peer)))
		}
	} else {
		h.output(FormatErrorString("DHT routing table is EMPTY"))
		h.output("")
		h.output("DHT bootstrap may not have completed. Try:")
		h.output("  /waitdht          - Wait for bootstrap to complete")
		h.output("  /bootstrap        - Reconnect to bootstrap peers")
		h.output("  /connect <addr>   - Direct connection to a peer")
		h.output("  /dhtinfo          - Detailed routing table info")
	}

	h.output("")
	h.output("==================\n")
}

// handleDHTInfo displays detailed DHT routing table information
func (h *CommandHandler) handleDHTInfo() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	dhtInfo := h.ipfsNode.GetDHTInfo()

	h.output("\n=== DHT Routing Table Status ===\n")
	h.output(fmt.Sprintf("DHT Mode: %s", dhtInfo.Mode))
	h.output(fmt.Sprintf("Routing Table Size: %d peers", dhtInfo.RoutingTableSize))
	h.output(fmt.Sprintf("Connected Peers: %d", dhtInfo.ConnectedPeerCount))
	h.output(fmt.Sprintf("Has Bootstrap Connection: %v", dhtInfo.HasBootstrapConnection))
	h.output("")

	if dhtInfo.RoutingTableSize == 0 {
		h.output(FormatErrorString("DHT routing table is EMPTY"))
		h.output("")
		h.output("This means DHT bootstrap has not completed or failed.")
		h.output("Try:")
		h.output("  1. Wait a few seconds for bootstrap to complete")
		h.output("  2. Run /waitdht to wait for bootstrap")
		h.output("  3. Run /bootstrap to reconnect to bootstrap peers")
		h.output("  4. Run /connect <multiaddr> for direct connection")
	} else {
		h.output(FormatSuccess("DHT routing table is populated"))
		h.output("")
		h.output("Routing table peers:")
		for i, peer := range dhtInfo.RoutingTablePeers {
			if i >= 10 {
				h.output(fmt.Sprintf("  ... and %d more", dhtInfo.RoutingTableSize-10))
				break
			}
			h.output(fmt.Sprintf("  [%d] %s", i+1, truncatePeerID(peer)))
		}
	}

	h.output("")
	h.output("===============================\n")
}

// handleWaitDHT waits for DHT bootstrap to complete
func (h *CommandHandler) handleWaitDHT(args []string) {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	// Parse optional timeout
	timeout := 30 * time.Second
	if len(args) > 0 {
		if d, err := time.ParseDuration(args[0]); err == nil {
			timeout = d
		}
	}

	h.output(FormatInfo(fmt.Sprintf("Waiting for DHT bootstrap (timeout: %s)...", timeout)))

	start := time.Now()
	if err := h.ipfsNode.WaitForDHT(timeout); err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Bootstrap wait failed: %v", err)))
		return
	}

	elapsed := time.Since(start)
	h.output(FormatSuccess(fmt.Sprintf("DHT bootstrap completed in %s", elapsed.Round(100*time.Millisecond))))

	// Show routing table status
	dhtInfo := h.ipfsNode.GetDHTInfo()
	h.output(fmt.Sprintf("Routing table now has %d peers", dhtInfo.RoutingTableSize))
}

// handleMDNS displays mDNS discovery statistics
func (h *CommandHandler) handleMDNS() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	stats := h.ipfsNode.GetMDnsStats()
	info := h.ipfsNode.GetNetworkInfo()

	h.output("\n=== mDNS Discovery Status ===\n")
	h.output(fmt.Sprintf("Total mDNS discoveries: %d", stats.TotalDiscoveries))

	if stats.LastPeerFound.IsZero() {
		h.output("Last peer found: Never")
	} else {
		h.output(fmt.Sprintf("Last peer found: %s ago", time.Since(stats.LastPeerFound).Round(time.Second)))
	}

	h.output("")
	h.output(fmt.Sprintf("Currently connected peers: %d", info.ConnectedPeerCount))
	h.output("")

	if stats.TotalDiscoveries == 0 {
		h.output(FormatErrorString("No peers discovered via mDNS yet"))
		h.output("")
		h.output("mDNS discovery may take a few seconds.")
		h.output("If no peers are found:")
		h.output("  - Check if firewall allows mDNS (UDP port 5353)")
		h.output("  - Ensure both nodes use the same protocol ID")
		h.output("  - Try /connect <multiaddr> for manual connection")
	} else if info.ConnectedPeerCount == 0 {
		h.output(FormatInfo("Peers were discovered but not currently connected"))
		h.output("This may indicate connection failures or peer disconnections.")
	} else {
		h.output(FormatSuccess("mDNS discovery is working"))
	}

	h.output("")
	h.output("===============================\n")
}

// handleNetworkStatus displays comprehensive network health metrics
func (h *CommandHandler) handleNetworkStatus() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	metrics := h.ipfsNode.GetMetricsFull()
	dhtInfo := h.ipfsNode.GetDHTInfo()

	h.output("\n╔════════════════════════════════════════════════════════╗")
	h.output("║        Babylon Tower - Network Health Metrics         ║")
	h.output("╚════════════════════════════════════════════════════════╝")
	h.output("")

	// Node info
	h.output("┌─ Node Information ───────────────────────────────────┐")
	h.output(fmt.Sprintf("│ Peer ID:      %s", truncatePeerID(metrics.PeerID)))
	h.output(fmt.Sprintf("│ Uptime:       %s", formatDuration(metrics.UptimeSeconds)))
	h.output(fmt.Sprintf("│ Started:      %s", metrics.StartTime.Format("2006-01-02 15:04:05")))
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	// Connection metrics
	h.output("┌─ Connection Metrics ─────────────────────────────────┐")
	h.output(fmt.Sprintf("│ Current Connections:    %d", metrics.CurrentConnections))
	h.output(fmt.Sprintf("│ Total Connections:      %d", metrics.TotalConnections))
	h.output(fmt.Sprintf("│ Total Disconnections:   %d", metrics.TotalDisconnections))
	h.output(fmt.Sprintf("│ Connection Success Rate: %.1f%%", metrics.ConnectionSuccessRate*100))
	h.output(fmt.Sprintf("│ Average Latency:        %d ms", metrics.AverageLatencyMs))
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	// Discovery metrics
	h.output("┌─ Discovery Metrics ──────────────────────────────────┐")
	h.output(fmt.Sprintf("│ DHT Discoveries:        %d", metrics.DHTDiscoveries))
	h.output(fmt.Sprintf("│ mDNS Discoveries:       %d", metrics.MDNSDiscoveries))
	h.output(fmt.Sprintf("│ Peer Exchange:          %d", metrics.PeerExchangeDiscoveries))
	h.output("│ Discovery by Source:")
	for source, count := range metrics.DiscoveryBySource {
		h.output(fmt.Sprintf("│   %-20s %d", source+":", count))
	}
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	// DHT status
	h.output("┌─ DHT Status ─────────────────────────────────────────┐")
	h.output(fmt.Sprintf("│ Routing Table Size:     %d peers", dhtInfo.RoutingTableSize))
	h.output(fmt.Sprintf("│ DHT Mode:               %s", dhtInfo.Mode))
	h.output(fmt.Sprintf("│ Has Bootstrap:          %v", dhtInfo.HasBootstrapConnection))
	if dhtInfo.RoutingTableSize > 0 && dhtInfo.RoutingTableSize <= 10 {
		h.output("│ Routing Table Peers:")
		for i, peer := range dhtInfo.RoutingTablePeers {
			h.output(fmt.Sprintf("│   [%d] %s", i+1, truncatePeerID(peer)))
		}
	} else if dhtInfo.RoutingTableSize > 10 {
		h.output(fmt.Sprintf("│ Routing Table Peers:    %d total (showing first 5)", dhtInfo.RoutingTableSize))
		for i := 0; i < 5 && i < len(dhtInfo.RoutingTablePeers); i++ {
			h.output(fmt.Sprintf("│   [%d] %s", i+1, truncatePeerID(dhtInfo.RoutingTablePeers[i])))
		}
	}
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	// Message metrics
	h.output("┌─ Message Metrics ────────────────────────────────────┐")
	h.output(fmt.Sprintf("│ Successful Messages:    %d", metrics.SuccessfulMessages))
	h.output(fmt.Sprintf("│ Failed Messages:        %d", metrics.FailedMessages))
	h.output(fmt.Sprintf("│ Message Success Rate:   %.1f%%", metrics.MessageSuccessRate*100))
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	// Bootstrap status
	h.output("┌─ Bootstrap Status ───────────────────────────────────┐")
	h.output(fmt.Sprintf("│ Bootstrap Attempts:     %d", metrics.BootstrapAttempts))
	h.output(fmt.Sprintf("│ Bootstrap Successes:    %d", metrics.BootstrapSuccesses))
	if !metrics.LastBootstrapTime.IsZero() {
		h.output(fmt.Sprintf("│ Last Bootstrap:       %s ago", time.Since(metrics.LastBootstrapTime).Round(time.Second)))
	} else {
		h.output("│ Last Bootstrap:         Never")
	}
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	// Health summary
	h.output("┌─ Network Health Summary ─────────────────────────────┐")
	var healthStatus string
	var healthColor func(string) string

	if metrics.CurrentConnections == 0 {
		healthStatus = "CRITICAL - No connections"
		healthColor = FormatErrorString
	} else if metrics.CurrentConnections < 3 {
		healthStatus = "WARNING - Low connectivity"
		healthColor = func(s string) string { return "⚠️ " + s }
	} else if dhtInfo.RoutingTableSize < 5 {
		healthStatus = "WARNING - Small routing table"
		healthColor = func(s string) string { return "⚠️ " + s }
	} else {
		healthStatus = "HEALTHY"
		healthColor = FormatSuccess
	}

	h.output(healthColor(fmt.Sprintf("│ Status:  %s", healthStatus)))
	h.output(fmt.Sprintf("│ Score:   %.0f%%", calculateHealthScore(metrics, dhtInfo)))
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")
}

// formatDuration formats seconds into human-readable duration
func formatDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	} else if seconds < 3600 {
		return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
	} else if seconds < 86400 {
		return fmt.Sprintf("%dh %dm", seconds/3600, (seconds%3600)/60)
	}
	return fmt.Sprintf("%dd %dh", seconds/86400, (seconds%86400)/3600)
}

// calculateHealthScore calculates a network health score (0-100)
func calculateHealthScore(metrics *ipfsnode.MetricsFull, dhtInfo *ipfsnode.DHTInfo) float64 {
	score := 0.0

	// Connection score (max 40 points)
	if metrics.CurrentConnections > 0 {
		score += 20
	}
	if metrics.CurrentConnections >= 3 {
		score += 10
	}
	if metrics.CurrentConnections >= 10 {
		score += 10
	}

	// DHT score (max 30 points)
	if dhtInfo.RoutingTableSize > 0 {
		score += 15
	}
	if dhtInfo.RoutingTableSize >= 5 {
		score += 15
	}

	// Success rate score (max 30 points)
	score += metrics.ConnectionSuccessRate * 15
	score += metrics.MessageSuccessRate * 15

	return score
}

// handleContactStatus displays detailed status for all contacts
func (h *CommandHandler) handleContactStatus() {
	if h.messaging == nil {
		h.output(FormatErrorString("Messaging service not available"))
		return
	}

	h.output("\n=== Contact Status ===\n")
	h.output("")

	statuses, err := h.messaging.GetAllContactStatuses()
	if err != nil {
		h.output(FormatErrorString(fmt.Sprintf("Failed to get contact statuses: %v", err)))
		return
	}

	if len(statuses) == 0 {
		h.output(FormatInfo("No contacts in your contact list."))
		h.output(FormatInfo("Use /add <pubkey> [name] to add contacts."))
		h.output("")
		h.output("======================\n")
		return
	}

	// Display header
	h.output(fmt.Sprintf("%-20s %-12s %-10s %-10s %-8s %-8s",
		"Contact", "Status", "Online", "Connected", "Active", "Mesh"))
	h.output(strings.Repeat("-", 70))

	for _, status := range statuses {
		name := status.DisplayName
		if name == "" {
			name = FormatPublicKeyBase58(status.PubKey)[:8] + "..."
		}
		if len(name) > 18 {
			name = name[:18]
		}

		// Status indicator
		statusStr := "○"
		if status.IsActive {
			statusStr = "●"
		} else if status.Connected {
			statusStr = "◉"
		}

		// Online indicator
		onlineStr := "No"
		if status.IsOnline {
			onlineStr = "Yes"
		}

		// Connected indicator
		connectedStr := "No"
		if status.Connected {
			connectedStr = "Yes"
		}

		// Active indicator
		activeStr := "No"
		if status.IsActive {
			activeStr = "Yes"
		}

		h.output(fmt.Sprintf("%-20s %-12s %-10s %-10s %-8s %-8d",
			name, statusStr, onlineStr, connectedStr, activeStr, status.MeshSize))

		// Show peer ID if available
		if status.PeerID != "" {
			h.output(fmt.Sprintf("  └─ Peer: %s", truncatePeerID(status.PeerID)))
		}
	}

	h.output("")
	h.output(FormatInfo("Legend: ● Active contact, ◉ Connected, ○ Inactive"))
	h.output(FormatInfo("Use /chat <contact> to start chatting with a contact."))
	h.output("")
	h.output("======================\n")
}

// GetChatContactPubKey returns the current chat contact's public key
func (h *CommandHandler) GetChatContactPubKey() []byte {
	return h.chatContactPubKey
}

// IsInChatMode returns true if in chat mode
func (h *CommandHandler) IsInChatMode() bool {
	return h.inChatMode
}
