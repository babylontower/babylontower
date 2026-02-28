package cli

import (
	"fmt"

	"github.com/mr-tron/base58"
)

// handleMyID displays the user's public keys (Ed25519 and X25519)
func (h *CommandHandler) handleMyID() {
	ed25519Hex := FormatPublicKey(h.ed25519PubKey)
	ed25519Base58 := base58.Encode(h.ed25519PubKey)
	x25519Hex := FormatPublicKey(h.x25519PubKey)
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
