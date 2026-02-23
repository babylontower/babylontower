package cli

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"

	pb "babylontower/pkg/proto"
	"github.com/mr-tron/base58"
)

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
	pubKey, err := decodePublicKey(pubKeyStr)
	if err != nil {
		h.output(FormatErrorString("Invalid public key format. Use hex or base58."))
		return
	}

	if len(pubKey) != ed25519.PublicKeySize {
		h.output(FormatErrorString(fmt.Sprintf("Invalid public key length: expected %d bytes, got %d", ed25519.PublicKeySize, len(pubKey))))
		return
	}

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

	displayName, x25519PubKey := parseContactArgs(args)

	contact := &pb.Contact{
		PublicKey:       pubKey,
		DisplayName:     displayName,
		CreatedAt:       uint64(timeNow().Unix()),
		X25519PublicKey: x25519PubKey,
	}

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

// decodePublicKey decodes a public key from hex or base58 format
func decodePublicKey(pubKeyStr string) ([]byte, error) {
	pubKey, err := base58.Decode(pubKeyStr)
	if err != nil {
		return hex.DecodeString(pubKeyStr)
	}
	return pubKey, nil
}

// parseContactArgs parses nickname and X25519 key from command arguments
func parseContactArgs(args []string) (string, []byte) {
	displayName := ""
	var x25519PubKey []byte

	if len(args) < 2 {
		return "", nil
	}

	lastArg := args[len(args)-1]
	if key, err := hex.DecodeString(lastArg); err == nil && len(key) == 32 {
		x25519PubKey = key
		if len(args) > 2 {
			displayName = strings.Join(args[1:len(args)-1], " ")
		}
	} else if key, err := base58.Decode(lastArg); err == nil && len(key) == 32 {
		x25519PubKey = key
		if len(args) > 2 {
			displayName = strings.Join(args[1:len(args)-1], " ")
		}
	} else {
		displayName = strings.Join(args[1:], " ")
	}

	return displayName, x25519PubKey
}
