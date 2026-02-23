package cli

import (
	"crypto/ed25519"
	"fmt"
	"strings"
	"time"

	"babylontower/pkg/ipfsnode"
	"babylontower/pkg/messaging"
	"babylontower/pkg/storage"
	"github.com/ipfs/go-log/v2"
)

var logger = log.Logger("babylontower/cli")

// timeNow returns the current time (used for testing)
var timeNow = time.Now

// CommandHandler handles CLI commands
type CommandHandler struct {
	storage   storage.Storage
	ipfsNode  *ipfsnode.Node
	messaging *messaging.Service

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
