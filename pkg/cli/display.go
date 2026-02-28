package cli

import (
	"fmt"
	"strings"
	"time"

	pb "babylontower/pkg/proto"

	"github.com/mr-tron/base58"
)

const (
	// Key display lengths
	keyDisplayLen = 16 // Show first 16 chars of hex keys
	// Time format for messages
	timeFormat = "2006-01-02 15:04:05"
)

// FormatPublicKey formats a public key for display (truncated hex)
func FormatPublicKey(pubKey []byte) string {
	hex := fmt.Sprintf("%x", pubKey)
	if len(hex) > keyDisplayLen {
		return hex[:keyDisplayLen] + "..."
	}
	return hex
}

// FormatPublicKeyBase58 formats a public key as base58 for display (truncated)
func FormatPublicKeyBase58(pubKey []byte) string {
	encoded := base58.Encode(pubKey)
	if len(encoded) > keyDisplayLen {
		return encoded[:keyDisplayLen] + "..."
	}
	return encoded
}

// FormatContact formats a contact for display
func FormatContact(index int, contact *pb.Contact) string {
	name := contact.DisplayName
	if name == "" {
		name = "(no name)"
	}
	pubKey := FormatPublicKeyBase58(contact.PublicKey)
	return fmt.Sprintf("[%d] %s - %s", index, name, pubKey)
}

// FormatContactList formats a list of contacts for display
func FormatContactList(contacts []*pb.Contact) string {
	if len(contacts) == 0 {
		return "No contacts found.\nUse /add <pubkey> [nickname] to add a contact."
	}

	var sb strings.Builder
	sb.WriteString("\n=== Contacts ===\n")
	for i, contact := range contacts {
		sb.WriteString(FormatContact(i+1, contact))
		sb.WriteString("\n")
	}
	sb.WriteString("================\n")
	return sb.String()
}

// FormatMessage formats a decrypted message for display
func FormatMessage(msg *pb.Message, senderName string, isOutgoing bool) string {
	timestamp := time.Unix(int64(msg.Timestamp), 0).Format(timeFormat)

	var sender string
	if isOutgoing {
		sender = "You"
	} else {
		if senderName == "" {
			sender = "Unknown"
		} else {
			sender = senderName
		}
	}

	return fmt.Sprintf("[%s] %s: %s", timestamp, sender, msg.Text)
}

// FormatMessageFromEnvelope formats a message from a signed envelope
func FormatMessageFromEnvelope(envelope *pb.SignedEnvelope, contactName string, isOutgoing bool) string {
	// For PoC, we can't decrypt incoming messages without the full flow
	// This is a placeholder that shows the envelope was received
	timestamp := time.Now().Format(timeFormat)

	var sender string
	if isOutgoing {
		sender = "You"
	} else {
		if contactName == "" {
			sender = "Contact"
		} else {
			sender = contactName
		}
	}

	return fmt.Sprintf("[%s] %s: [Encrypted message received]", timestamp, sender)
}

// FormatError formats an error for display
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("❌ Error: %s", err.Error())
}

// FormatErrorString formats an error message string for display
func FormatErrorString(message string) string {
	if message == "" {
		return ""
	}
	return fmt.Sprintf("❌ Error: %s", message)
}

// FormatSuccess formats a success message
func FormatSuccess(message string) string {
	return fmt.Sprintf("✅ %s", message)
}

// FormatInfo formats an info message
func FormatInfo(message string) string {
	return fmt.Sprintf("ℹ️  %s", message)
}

// FormatHelp formats the help message
func FormatHelp() string {
	var sb strings.Builder
	sb.WriteString("\n=== Babylon Tower Commands ===\n\n")
	sb.WriteString("Identity:\n")
	sb.WriteString("  /myid                 Display your public keys and node multiaddr\n\n")
	sb.WriteString("Contacts:\n")
	sb.WriteString("  /add <pubkey> [name] [x25519_key]  Add a contact\n")
	sb.WriteString("  /list                            List all contacts\n\n")
	sb.WriteString("Messaging:\n")
	sb.WriteString("  /chat <contact>       Enter chat mode with a contact\n")
	sb.WriteString("  /history <contact> [limit]  Show message history\n\n")
	sb.WriteString("Groups:\n")
	sb.WriteString("  /creategroup <name> <description>  Create a new private group\n")
	sb.WriteString("  /listgroups                        List all groups\n")
	sb.WriteString("  /invite <group_id> <pubkey> [name] Invite member to group\n")
	sb.WriteString("  /groupchat <group_id>              Enter chat mode with group\n\n")
	sb.WriteString("Network:\n")
	sb.WriteString("  /connect <multiaddr>  Connect to a peer node\n")
	sb.WriteString("  /find <peer_id>       Find and connect to peer via DHT\n")
	sb.WriteString("  /advertise            Advertise yourself to DHT\n")
	sb.WriteString("  /bootstrap            Show bootstrap peer status\n")
	sb.WriteString("  /reconnect            Retry bootstrap peer connection\n")
	sb.WriteString("  /debug                Show detailed network debug info\n")
	sb.WriteString("  /myaddr               Show your multiaddrs for sharing\n")
	sb.WriteString("  /peers                Show detailed peer connection info\n")
	sb.WriteString("  /dht                  Show DHT status\n")
	sb.WriteString("  /dhtinfo              Show detailed DHT routing table\n")
	sb.WriteString("  /waitdht [timeout]    Wait for DHT bootstrap (default: 30s)\n")
	sb.WriteString("  /mdns                 Show mDNS discovery statistics\n")
	sb.WriteString("  /network              Show comprehensive network health metrics\n")
	sb.WriteString("                        (alias: /netmetrics)\n")
	sb.WriteString("  /ipfslogs             Show IPFS network status (interactive)\n")
	sb.WriteString("                        (aliases: /ipfs, /netstatus)\n")
	sb.WriteString("  /netlog               Show network discovery log (interactive)\n")
	sb.WriteString("                        (alias: /netinfo)\n\n")
	sb.WriteString("Reputation:\n")
	sb.WriteString("  /reputation           Show reputation summary\n")
	sb.WriteString("                        (alias: /rep)\n")
	sb.WriteString("  /reputation list      List all peers with reputation scores\n")
	sb.WriteString("  /reputation tier      Show peers by tier (basic|contributor|reliable|trusted)\n")
	sb.WriteString("  /reputation top [n]   Show top N peers by reputation\n\n")
	sb.WriteString("System:\n")
	sb.WriteString("  /help              Show this help message\n")
	sb.WriteString("  /exit              Exit the application\n\n")
	sb.WriteString("In chat mode:\n")
	sb.WriteString("  - Type a message and press Enter to send\n")
	sb.WriteString("  - Press Enter on an empty line to exit chat\n")
	sb.WriteString("  - Incoming messages appear in real-time\n\n")
	sb.WriteString("Interactive commands:\n")
	sb.WriteString("  - /ipfslogs and /netlog run continuously\n")
	sb.WriteString("  - Press Enter on an empty line to exit\n\n")
	sb.WriteString("==============================\n")
	return sb.String()
}

// FormatBanner formats the application banner
func FormatBanner(version string, publicKey string) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("╔══════════════════════════════════════════╗\n")
	sb.WriteString("║     🏰  Babylon Tower v")
	sb.WriteString(version)
	sb.WriteString("          ║\n")
	sb.WriteString("║     Decentralized P2P Messenger          ║\n")
	sb.WriteString("╚══════════════════════════════════════════╝\n")
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "Your public key: %s\n", publicKey)
	sb.WriteString("\nType /help for available commands.\n\n")
	return sb.String()
}

// FormatChatHeader formats the chat mode header
func FormatChatHeader(contactName string, contactPubKey string) string {
	var sb strings.Builder
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "━━━ Chat with %s ━━━\n", contactName)
	fmt.Fprintf(&sb, "Public key: %s\n", contactPubKey)
	sb.WriteString("Type your message and press Enter to send.\n")
	sb.WriteString("Press Enter on an empty line to exit chat.\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return sb.String()
}

// FormatChatExit formats the chat exit message
func FormatChatExit() string {
	return "\nExited chat mode.\n"
}

// FormatSystemMessage formats a system message (non-user message)
func FormatSystemMessage(message string) string {
	return fmt.Sprintf("📌 %s", message)
}

// FormatIncomingNotification formats a notification for incoming message
func FormatIncomingNotification(contactName string) string {
	return fmt.Sprintf("\n📬 New message from %s", contactName)
}
