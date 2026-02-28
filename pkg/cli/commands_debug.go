package cli

import (
	"fmt"
	"strings"
	"time"
)

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

// handleIPFSLogs displays IPFS network status and logs interactively
func (h *CommandHandler) handleIPFSLogs(args []string) {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	h.inInteractiveMode = true
	h.interactiveCmd = "ipfslogs"
	h.interactiveCancel = make(chan struct{})
	h.interactiveDone = make(chan struct{})

	h.output("\n=== IPFS Network Status (Interactive) ===")
	h.output("Press Enter on an empty line to exit.\n")

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

	h.inInteractiveMode = true
	h.interactiveCmd = "netlog"
	h.interactiveCancel = make(chan struct{})
	h.interactiveDone = make(chan struct{})

	h.output("\n=== Network Discovery Log (Interactive) ===")
	h.output("Press Enter on an empty line to exit.\n")

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

	h.output(fmt.Sprintf("%-20s %-12s %-10s %-10s %-8s %-8s",
		"Contact", "Status", "Online", "Connected", "Active", "Mesh"))
	h.output(strings.Repeat("─", 70))

	for _, status := range statuses {
		name := status.DisplayName
		if name == "" {
			name = FormatPublicKeyBase58(status.PubKey)[:8] + "..."
		}
		if len(name) > 18 {
			name = name[:18]
		}

		statusStr := "○"
		if status.IsActive {
			statusStr = "●"
		} else if status.Connected {
			statusStr = "◉"
		}

		onlineStr := "No"
		if status.IsOnline {
			onlineStr = "Yes"
		}

		connectedStr := "No"
		if status.Connected {
			connectedStr = "Yes"
		}

		activeStr := "No"
		if status.IsActive {
			activeStr = "Yes"
		}

		h.output(fmt.Sprintf("%-20s %-12s %-10s %-10s %-8s %-8d",
			name, statusStr, onlineStr, connectedStr, activeStr, status.MeshSize))

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
