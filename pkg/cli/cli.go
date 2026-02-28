package cli

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"babylontower/pkg/groups"
	"babylontower/pkg/ipfsnode"
	"babylontower/pkg/messaging"
	"babylontower/pkg/storage"

	"github.com/chzyer/readline"
)

// logger is declared in commands.go for this package

// Config holds CLI configuration
type Config struct {
	// Version is the application version
	Version string
	// DataDir is the directory for application data
	DataDir string
	// IdentityPath is the path to the identity file
	IdentityPath string
}

// CLI represents the main command-line interface
type CLI struct {
	config    *Config
	storage   storage.Storage
	ipfsNode  *ipfsnode.Node
	messaging *messaging.Service
	groups    *groups.Service
	handler   *CommandHandler
	rl        *readline.Instance
	outputMu  sync.Mutex

	// Identity keys
	ed25519PubKey  ed25519.PublicKey
	ed25519PrivKey ed25519.PrivateKey
	x25519PubKey   []byte
	x25519PrivKey  []byte

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new CLI instance
func New(config *Config, identity *Identity, storage storage.Storage, ipfsNode *ipfsnode.Node, messaging *messaging.Service, groupsSvc *groups.Service) (*CLI, error) {
	// Create readline instance
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          ">>> ",
		HistoryFile:     "", // No history file for PoC
		InterruptPrompt: "^C",
		EOFPrompt:       "^D",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create readline: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	cli := &CLI{
		config:         config,
		storage:        storage,
		ipfsNode:       ipfsNode,
		messaging:      messaging,
		groups:         groupsSvc,
		rl:             rl,
		ed25519PubKey:  identity.Ed25519PubKey,
		ed25519PrivKey: identity.Ed25519PrivKey,
		x25519PubKey:   identity.X25519PubKey,
		x25519PrivKey:  identity.X25519PrivKey,
		ctx:            ctx,
		cancel:         cancel,
	}

	// Create command handler
	cli.handler = NewCommandHandler(
		storage,
		ipfsNode,
		messaging,
		groupsSvc,
		identity.Ed25519PubKey,
		identity.Ed25519PrivKey,
		identity.X25519PubKey,
		identity.X25519PrivKey,
		cli.output,
	)

	return cli, nil
}

// Start starts the CLI and begins the main loop
func (c *CLI) Start() error {
	// Display banner
	c.output(FormatBanner(c.config.Version, FormatPublicKeyBase58(c.ed25519PubKey)))

	// Start message listener
	c.wg.Add(1)
	go c.listenForMessages()

	// Setup signal handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		c.output("\n\nShutting down...")
		c.cancel()
	}()

	// Main input loop
	return c.runLoop()
}

// runLoop runs the main input processing loop
func (c *CLI) runLoop() error {
	for {
		select {
		case <-c.ctx.Done():
			return nil
		default:
			// Read input
			line, err := c.rl.Readline()
			if err != nil {
				if err == readline.ErrInterrupt || err == io.EOF {
					return nil
				}
				return fmt.Errorf("failed to read input: %w", err)
			}

			// Handle command (line may be empty string on empty line input)
			shouldExit := c.handler.HandleCommand(line)
			if shouldExit {
				return nil
			}
		}
	}
}

// listenForMessages listens for incoming messages from the messaging service
func (c *CLI) listenForMessages() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		case msgEvent, ok := <-c.messaging.Messages():
			if !ok {
				return
			}
			c.handleIncomingMessage(msgEvent)
		}
	}
}

// handleIncomingMessage processes an incoming message
func (c *CLI) handleIncomingMessage(event *messaging.MessageEvent) {
	// Get contact name
	contact, err := c.storage.GetContact(event.ContactPubKey)
	contactName := ""
	if err == nil && contact != nil {
		contactName = contact.DisplayName
	}
	if contactName == "" {
		contactName = FormatPublicKeyBase58(event.ContactPubKey)
	}

	// Check if we're in chat with this contact
	inChatWithSender := c.handler.IsInChatMode() && string(c.handler.GetChatContactPubKey()) == string(event.ContactPubKey)

	if !inChatWithSender {
		// Show notification
		c.output(FormatIncomingNotification(contactName))
	}

	// Display message - use decrypted message from event
	if event.Message != nil {
		c.output(FormatMessage(event.Message, contactName, false))
	} else {
		// Fallback to envelope if message not decrypted
		c.output(FormatMessageFromEnvelope(event.Envelope, contactName, false))
	}

	if !inChatWithSender {
		c.output(FormatInfo("Type /chat to reply."))
	}
}

// output writes a message to the output
func (c *CLI) output(message string) {
	c.outputMu.Lock()
	defer c.outputMu.Unlock()

	// Write to stdout (ignore error as it's non-critical)
	_, _ = fmt.Fprintln(c.rl.Stdout(), message)

	// Refresh the prompt
	c.rl.Refresh()
}

// Stop gracefully shuts down the CLI
func (c *CLI) Stop() error {
	logger.Info("stopping CLI...")

	// Cancel context
	c.cancel()

	// Wait for goroutines
	c.wg.Wait()

	// Close readline
	if c.rl != nil {
		if err := c.rl.Close(); err != nil {
			logger.Warnw("readline close error", "error", err)
		}
	}

	logger.Info("CLI stopped")
	return nil
}

// GetStorage returns the storage instance
func (c *CLI) GetStorage() storage.Storage {
	return c.storage
}

// GetIPFSNode returns the IPFS node
func (c *CLI) GetIPFSNode() *ipfsnode.Node {
	return c.ipfsNode
}

// GetMessaging returns the messaging service
func (c *CLI) GetMessaging() *messaging.Service {
	return c.messaging
}
