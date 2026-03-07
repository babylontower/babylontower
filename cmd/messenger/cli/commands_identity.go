package cli

import (
	"fmt"

	"github.com/mr-tron/base58"
)

// handleMyID displays the user's public keys (Ed25519 and X25519) and fingerprint
func (h *CommandHandler) handleMyID() {
	ed25519Hex := FormatPublicKey(h.ed25519PubKey)
	ed25519Base58 := base58.Encode(h.ed25519PubKey)
	x25519Hex := FormatPublicKey(h.x25519PubKey)
	x25519Base58 := base58.Encode(h.x25519PubKey)

	h.output(FormatInfo("Your Public Keys:"))
	h.output("")
	h.output("Ed25519 (for signatures and verification):")
	h.output("  Hex:    " + ed25519Hex)
	h.output("  Base58: " + ed25519Base58)
	h.output("")
	h.output("X25519 (for encryption - share this with contacts):")
	h.output("  Hex:    " + x25519Hex)
	h.output("  Base58: " + x25519Base58)
	h.output("")

	// Compute and display identity fingerprint
	if h.identity != nil {
		fingerprint, err := h.identity.ComputeFingerprint()
		if err != nil {
			h.output(FormatErrorString("Failed to compute fingerprint: " + err.Error()))
		} else {
			h.output("Identity Fingerprint (for out-of-band verification):")
			h.output("  " + fingerprint)
			h.output("")
			h.output(FormatInfo("Compare this fingerprint with your contact to verify identity and prevent MITM attacks."))
			h.output(FormatInfo("This is a 27-28 character string derived from both your Ed25519 and X25519 public keys."))
		}
	}

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

		// Show Babylon DHT status for identity publication
		h.output("")
		h.outputIdentityPublicationStatus()
	}
}

// outputIdentityPublicationStatus shows the status of identity document publication
func (h *CommandHandler) outputIdentityPublicationStatus() {
	bootstrap := h.ipfsNode.GetBootstrapStatus()

	h.output(FormatInfo("Identity Document Publication Status:"))

	if !bootstrap.BabylonBootstrapComplete && !bootstrap.BabylonBootstrapDeferred {
		h.output("  Babylon DHT: ⏳ Waiting for bootstrap")
		h.output("  Identity will be published once Babylon DHT is ready")
	} else if bootstrap.BabylonBootstrapDeferred {
		h.output("  Babylon DHT: ⏸️  Deferred (lazy bootstrap)")
		h.output("  Identity will be published when first peer connects")
	} else if bootstrap.BabylonBootstrapComplete {
		h.output("  Babylon DHT: ✓ Ready")
		h.output(fmt.Sprintf("  Babylon Peers: %d connected, %d stored",
			bootstrap.BabylonPeersConnected, bootstrap.BabylonPeersStored))
		h.output("  Identity document should be published to Babylon DHT")

		if bootstrap.BabylonPeersConnected == 0 && bootstrap.BabylonPeersStored == 0 {
			h.output("")
			h.output(FormatInfo("No Babylon peers yet. Your identity will propagate when peers connect."))
			h.output(FormatInfo("Run /waitdht --babylon to wait for Babylon DHT bootstrap"))
		}
	}

	h.output("")
	h.output(FormatInfo("Your identity document is automatically published to the Babylon DHT."))
	h.output(FormatInfo("This allows other nodes to fetch your prekeys and verify your identity."))
}
