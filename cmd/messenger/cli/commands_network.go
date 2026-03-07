package cli

import (
	"fmt"
	"time"

	"babylontower/pkg/app"
)

// handleConnect connects to a peer node by multiaddr
func (h *CommandHandler) handleConnect(args []string) {
	if len(args) == 0 {
		h.output(FormatErrorString("Usage: /connect <multiaddr>"))
		h.output(FormatInfo("Multiaddr format: /ip4/127.0.0.1/tcp/4001/p2p/QmPeerID"))
		h.output(FormatInfo("Get multiaddr from other node using /myid"))
		return
	}

	if err := h.ipfsNode.ConnectToPeer(args[0]); err != nil {
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
		return
	}

	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	dhtInfo := h.ipfsNode.GetDHTInfo()
	if dhtInfo.RoutingTableSize == 0 {
		h.output(FormatErrorString("DHT routing table is empty"))
		h.output("")
		h.output(FormatInfo("Run /waitdht first to wait for DHT bootstrap"))
		h.output(FormatInfo("Or use /connect <multiaddr> for direct connection"))
		return
	}

	h.output(FormatInfo("Searching DHT for peer..."))
	h.output("Target Peer ID: " + args[0])
	h.output(fmt.Sprintf("Our routing table has %d peers", dhtInfo.RoutingTableSize))

	peerInfo, err := h.ipfsNode.FindPeer(args[0])
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
	h.output("Peer ID: " + string(peerInfo.ID))
	h.output(fmt.Sprintf("Addresses (%d):", len(peerInfo.Addrs)))
	for i, addr := range peerInfo.Addrs {
		h.output(fmt.Sprintf("  %d. %s/p2p/%s", i+1, addr, args[0]))
	}

	h.output("")
	h.output(FormatInfo("Attempting to connect..."))

	if err := h.ipfsNode.ConnectToPeer(peerInfo.Addrs[0] + "/p2p/" + peerInfo.ID); err != nil {
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
	h.output("  " + h.ipfsNode.PeerID())
}

// handleBootstrap shows bootstrap status and can trigger re-bootstrap
func (h *CommandHandler) handleBootstrap(args []string) {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	// Show current status
	bootstrap := h.ipfsNode.GetBootstrapStatus()
	h.output("\n=== Bootstrap Status ===\n")
	h.output(fmt.Sprintf("IPFS Bootstrap: %v", boolToString(bootstrap.IPFSBootstrapComplete)))
	h.output(fmt.Sprintf("Babylon Bootstrap: %v", boolToString(bootstrap.BabylonBootstrapComplete)))
	h.output(fmt.Sprintf("Babylon Peers Stored: %d", bootstrap.BabylonPeersStored))
	h.output(fmt.Sprintf("Babylon Peers Connected: %d", bootstrap.BabylonPeersConnected))
	h.output("")

	// If --force flag, trigger re-bootstrap
	if len(args) > 0 && (args[0] == "--force" || args[0] == "-f") {
		h.output(FormatInfo("Triggering Babylon bootstrap..."))
		discoveredPeers := h.ipfsNode.TriggerRendezvousDiscovery()
		h.output(fmt.Sprintf("Discovered %d peers via DHT rendezvous", discoveredPeers))
		if discoveredPeers > 0 {
			h.output(FormatSuccess("Bootstrap request sent successfully"))
		} else {
			h.output(FormatInfo("No peers responded. Try again in a few seconds."))
		}
	}

	h.output("")
	h.output("============================\n")
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

	bootstrapAddr := "/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ"
	h.output("Connecting to: " + bootstrapAddr)

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

// handleClearPeers clears stored peers and dial backoff
func (h *CommandHandler) handleClearPeers() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	h.output("\n=== Clear Peer Cache ===\n")
	h.output("")
	h.output(FormatInfo("Clearing dial backoff and stored peers..."))
	h.output("")

	// Clear all dial backoff
	h.ipfsNode.ClearAllBackoffs()
	h.output(FormatSuccess("Cleared dial backoff for all peers"))

	// Note: Clearing stored peers requires storage access
	// For now, we just clear the backoff
	h.output("")
	h.output(FormatInfo("Tip: To clear stored peers completely, delete:"))
	h.output("  ~/.babylontower/storage/peers")
	h.output("")
	h.output(FormatInfo("Then restart the application."))
	h.output("")
	h.output("========================\n")
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
					h.output("    " + addr)
				}
			}
			if len(peer.Protocols) > 0 {
				h.output("  Protocols:")
				for _, proto := range peer.Protocols {
					h.output("    " + proto)
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
		h.output("  " + fullAddr)
	}
	h.output("")
	h.output(FormatInfo("Use /connect <multiaddr> on another instance to connect."))
	h.output("============================\n")
}

// handleDHT displays DHT status for both IPFS and Babylon
// Updated for decoupled bootstrap architecture
func (h *CommandHandler) handleDHT() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	status := h.ipfsNode.GetBootstrapStatus()

	h.output("\n╔════════════════════════════════════════════════════════╗")
	h.output("║           DHT Status - Decoupled Bootstrap            ║")
	h.output("╚════════════════════════════════════════════════════════╝")
	h.output("")

	// IPFS DHT (Transport Layer)
	h.output("┌─ IPFS DHT (Transport Layer) ─────────────────────────┐")
	h.output("│ Purpose: PubSub connectivity, basic libp2p routing   │")
	if status.IPFSBootstrapComplete {
		h.output(fmt.Sprintf("│ Status:  ✓ Complete - %d peers in routing table    │", status.IPFSRoutingTableSize))
	} else {
		h.output(fmt.Sprintf("│ Status:  ⏳ Bootstrapping... - %d peers             │", status.IPFSRoutingTableSize))
	}
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	// Babylon DHT (Protocol Layer)
	h.output("┌─ Babylon DHT (Protocol Layer) ───────────────────────┐")
	h.output("│ Purpose: Messaging, groups, identity documents       │")
	if status.BabylonBootstrapComplete {
		h.output(fmt.Sprintf("│ Status:  ✓ Complete - %d peers connected           │", status.BabylonPeersConnected))
	} else if status.BabylonBootstrapDeferred {
		h.output("│ Status:  ⏸ Deferred - waiting for messages            │")
		h.output(fmt.Sprintf("│          %d peers stored, will bootstrap on first msg │", status.BabylonPeersStored))
	} else {
		h.output(fmt.Sprintf("│ Status:  ⏳ Bootstrapping... - %d peers connected    │", status.BabylonPeersConnected))
	}
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	// Rendezvous Discovery
	h.output("┌─ Rendezvous Discovery ───────────────────────────────┐")
	h.output("│ Purpose: DHT-based Babylon node discovery             │")
	if status.RendezvousActive {
		h.output("│ Status:  ✓ Active - discoverable by other nodes      │")
	} else {
		h.output("│ Status:  ⏳ Pending                                  │")
	}
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	// Peer Sources
	h.output("┌─ Peer Sources ───────────────────────────────────────┐")
	counts := h.ipfsNode.GetPeerCountsBySource()
	h.output(fmt.Sprintf("│ Babylon:       %d peers                            │", counts.Babylon))
	h.output(fmt.Sprintf("│ IPFS Bootstrap: %d peers                            │", counts.IPFSBootstrap))
	h.output(fmt.Sprintf("│ IPFS Discovery: %d peers                            │", counts.IPFSDiscovery))
	h.output(fmt.Sprintf("│ mDNS:          %d peers                            │", counts.MDNS))
	h.output(fmt.Sprintf("│ ──────────────────────────────────────────────── │"))
	h.output(fmt.Sprintf("│ Total Connected: %d peers                          │", counts.ConnectedTotal))
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")
}

// handleDHTInfo displays detailed DHT routing table information
func (h *CommandHandler) handleDHTInfo() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	dhtInfo := h.ipfsNode.GetDHTInfo()

	h.output("\n=== IPFS DHT Routing Table ===\n")
	h.output("DHT Mode: " + dhtInfo.Mode)
	h.output(fmt.Sprintf("Routing Table Size: %d peers", dhtInfo.RoutingTableSize))
	h.output(fmt.Sprintf("Connected Peers: %d", dhtInfo.ConnectedPeerCount))
	h.output(fmt.Sprintf("Has Bootstrap Connection: %v", dhtInfo.HasBootstrapConnection))
	h.output("")

	if dhtInfo.RoutingTableSize == 0 {
		h.output(FormatErrorString("IPFS DHT routing table is EMPTY"))
		h.output("")
		h.output("This means DHT bootstrap has not completed or failed.")
		h.output("Try:")
		h.output("  1. Wait a few seconds for bootstrap to complete")
		h.output("  2. Run /waitdht to wait for bootstrap")
		h.output("  3. Run /bootstrap to reconnect to bootstrap peers")
		h.output("  4. Run /connect <multiaddr> for direct connection")
	} else {
		h.output(FormatSuccess("IPFS DHT routing table is populated"))
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

	// Babylon DHT routing table
	babylonDHT := h.ipfsNode.GetBabylonDHTInfo()
	h.output("\n=== Babylon DHT Routing Table ===\n")
	h.output(fmt.Sprintf("Stored Babylon Peers: %d", babylonDHT.StoredBabylonPeers))
	h.output(fmt.Sprintf("Connected: %d", babylonDHT.ConnectedBabylonPeers))
	h.output("")

	if babylonDHT.StoredBabylonPeers > 0 {
		h.output("Babylon peers (first 10):")
		for i, peerID := range babylonDHT.BabylonPeerIDs {
			if i >= 10 {
				h.output(fmt.Sprintf("  ... and %d more", babylonDHT.StoredBabylonPeers-10))
				break
			}
			h.output(fmt.Sprintf("  [%d] %s", i+1, truncatePeerID(peerID)))
		}
	} else {
		h.output(FormatErrorString("No Babylon peers stored yet"))
		h.output("Run /bootstrap to manually request peers")
	}

	h.output("")
	h.output("===============================\n")
}

// handleBabylonStatus displays comprehensive Babylon network status
func (h *CommandHandler) handleBabylonStatus() {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	status := h.ipfsNode.GetBootstrapStatus()

	h.output("\n╔════════════════════════════════════════════════════════╗")
	h.output("║           Babylon Network Status                      ║")
	h.output("╚════════════════════════════════════════════════════════╝")
	h.output("")

	h.output("┌─ Bootstrap Architecture ─────────────────────────────┐")
	h.output("│ This node uses decoupled bootstrap:                  │")
	h.output("│   • IPFS DHT = Transport (PubSub)                    │")
	h.output("│   • Babylon DHT = Protocol (Messaging)               │")
	h.output("│   • Lazy Bootstrap = Triggered by messages           │")
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	h.output("┌─ IPFS DHT (Transport) ───────────────────────────────┐")
	if status.IPFSBootstrapComplete {
		h.output(fmt.Sprintf("│ ✓ Complete - %d peers in routing table             │", status.IPFSRoutingTableSize))
	} else {
		h.output(fmt.Sprintf("│ ⏳ Bootstrapping... - %d peers                      │", status.IPFSRoutingTableSize))
	}
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	h.output("┌─ Babylon DHT (Protocol) ─────────────────────────────┐")
	if status.BabylonBootstrapComplete {
		h.output(fmt.Sprintf("│ ✓ Complete - %d/%d peers connected              │",
			status.BabylonPeersConnected, status.BabylonPeersStored))
	} else if status.BabylonBootstrapDeferred {
		h.output("│ ⏸ Deferred - will bootstrap on first message       │")
		h.output(fmt.Sprintf("│   %d peers stored for later connection            │", status.BabylonPeersStored))
	} else {
		h.output(fmt.Sprintf("│ ⏳ Bootstrapping... - %d/%d peers connected       │",
			status.BabylonPeersConnected, status.BabylonPeersStored))
	}
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	h.output("┌─ Rendezvous Discovery ───────────────────────────────┐")
	if status.RendezvousActive {
		h.output("│ ✓ Active - discoverable by other nodes              │")
	} else {
		h.output("│ ⏳ Pending                                          │")
	}
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	if !status.BabylonBootstrapComplete && !status.BabylonBootstrapDeferred {
		h.output(FormatInfo("Babylon bootstrap in progress..."))
		h.output("Recommendations:")
		h.output("  1. Wait for PubSub discovery (automatic)")
		h.output("  2. Run /bootstrap to manually request peers")
		h.output("  3. Send/receive a message to trigger lazy bootstrap")
	} else if status.BabylonBootstrapDeferred {
		h.output(FormatSuccess("Babylon bootstrap deferred - node ready for lazy bootstrap"))
		h.output("The node will bootstrap when:")
		h.output("  • A message is received from a Babylon peer")
		h.output("  • A bootstrap request is received")
		h.output("  • You run /bootstrap manually")
	} else {
		h.output(FormatSuccess("Babylon bootstrap complete - node is fully integrated"))
	}

	h.output("\n============================\n")
}

// handleWaitDHT waits for DHT bootstrap to complete
func (h *CommandHandler) handleWaitDHT(args []string) {
	if h.ipfsNode == nil || !h.ipfsNode.IsStarted() {
		h.output(FormatErrorString("IPFS node not started"))
		return
	}

	// Check for --babylon flag to wait for Babylon DHT specifically
	waitForBabylon := false
	for _, arg := range args {
		if arg == "--babylon" || arg == "-b" {
			waitForBabylon = true
			break
		}
	}

	timeout := 30 * time.Second
	if len(args) > 0 && args[0] != "--babylon" && args[0] != "-b" {
		if d, err := time.ParseDuration(args[0]); err == nil {
			timeout = d
		}
	}

	if waitForBabylon {
		// Wait for Babylon DHT (protocol layer)
		h.output(FormatInfo(fmt.Sprintf("Waiting for Babylon DHT bootstrap (timeout: %s)...", timeout)))
		h.output(FormatInfo("Babylon DHT is used for identity documents and protocol operations"))

		start := time.Now()
		if err := h.ipfsNode.WaitForBabylonBootstrap(timeout); err != nil {
			h.output(FormatErrorString(fmt.Sprintf("Babylon DHT bootstrap wait failed: %v", err)))
			h.output(FormatInfo("Babylon DHT may be deferred - it will trigger on first message/request"))
			return
		}

		elapsed := time.Since(start)
		h.output(FormatSuccess(fmt.Sprintf("Babylon DHT bootstrap completed in %s", elapsed.Round(100*time.Millisecond))))

		bootstrap := h.ipfsNode.GetBootstrapStatus()
		h.output(fmt.Sprintf("Babylon peers connected: %d", bootstrap.BabylonPeersConnected))
		h.output(fmt.Sprintf("Babylon peers stored: %d", bootstrap.BabylonPeersStored))
	} else {
		// Wait for IPFS DHT (transport layer)
		h.output(FormatInfo(fmt.Sprintf("Waiting for IPFS DHT bootstrap (timeout: %s)...", timeout)))

		start := time.Now()
		if err := h.ipfsNode.WaitForDHT(timeout); err != nil {
			h.output(FormatErrorString(fmt.Sprintf("Bootstrap wait failed: %v", err)))
			return
		}

		elapsed := time.Since(start)
		h.output(FormatSuccess(fmt.Sprintf("DHT bootstrap completed in %s", elapsed.Round(100*time.Millisecond))))

		dhtInfo := h.ipfsNode.GetDHTInfo()
		h.output(fmt.Sprintf("Routing table now has %d peers", dhtInfo.RoutingTableSize))
	}
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
	bootstrap := h.ipfsNode.GetBootstrapStatus()

	h.output("\n╔════════════════════════════════════════════════════════╗")
	h.output("║        Babylon Tower - Network Health Metrics         ║")
	h.output("╚════════════════════════════════════════════════════════╝")
	h.output("")

	h.output("┌─ Node Information ───────────────────────────────────┐")
	h.output("│ Peer ID:      " + truncatePeerID(metrics.PeerID))
	h.output("│ Uptime:       " + formatDuration(metrics.UptimeSeconds))
	h.output("│ Started:      " + metrics.StartTime.Format("2006-01-02 15:04:05"))
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	h.output("┌─ Bootstrap Status (Decoupled) ───────────────────────┐")
	if bootstrap.IPFSBootstrapComplete {
		h.output(fmt.Sprintf("│ IPFS DHT:     ✓ Complete (%d peers)               │", bootstrap.IPFSRoutingTableSize))
	} else {
		h.output(fmt.Sprintf("│ IPFS DHT:     ⏳ Bootstrapping (%d peers)           │", bootstrap.IPFSRoutingTableSize))
	}
	if bootstrap.BabylonBootstrapComplete {
		h.output(fmt.Sprintf("│ Babylon DHT:  ✓ Complete (%d peers)               │", bootstrap.BabylonPeersConnected))
	} else if bootstrap.BabylonBootstrapDeferred {
		h.output("│ Babylon DHT:  ⏸ Deferred (lazy bootstrap)             │")
	} else {
		h.output(fmt.Sprintf("│ Babylon DHT:  ⏳ Bootstrapping (%d peers)           │", bootstrap.BabylonPeersConnected))
	}
	if bootstrap.RendezvousActive {
		h.output("│ Rendezvous:   ✓ Active (discoverable)                 │")
	} else {
		h.output("│ Rendezvous:   ⏳ Pending                               │")
	}
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	h.output("┌─ Connection Metrics ─────────────────────────────────┐")
	h.output(fmt.Sprintf("│ Current Connections:    %d", metrics.CurrentConnections))
	h.output(fmt.Sprintf("│ Total Connections:      %d", metrics.TotalConnections))
	h.output(fmt.Sprintf("│ Total Disconnections:   %d", metrics.TotalDisconnections))
	h.output(fmt.Sprintf("│ Connection Success Rate: %.1f%%", metrics.ConnectionSuccessRate*100))
	h.output(fmt.Sprintf("│ Average Latency:        %d ms", metrics.AverageLatencyMs))
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

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

	h.output("┌─ DHT Status ─────────────────────────────────────────┐")
	h.output(fmt.Sprintf("│ Routing Table Size:     %d peers", dhtInfo.RoutingTableSize))
	h.output("│ DHT Mode:               " + dhtInfo.Mode)
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

	h.output("┌─ Message Metrics ────────────────────────────────────┐")
	h.output(fmt.Sprintf("│ Successful Messages:    %d", metrics.SuccessfulMessages))
	h.output(fmt.Sprintf("│ Failed Messages:        %d", metrics.FailedMessages))
	h.output(fmt.Sprintf("│ Message Success Rate:   %.1f%%", metrics.MessageSuccessRate*100))
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")

	h.output("┌─ Network Health Summary ─────────────────────────────┐")
	var healthStatus string
	if metrics.CurrentConnections == 0 {
		healthStatus = "CRITICAL - No connections"
	} else if metrics.CurrentConnections < 3 {
		healthStatus = "WARNING - Low connectivity"
	} else if dhtInfo.RoutingTableSize < 5 {
		healthStatus = "WARNING - Small routing table"
	} else {
		healthStatus = "HEALTHY"
	}

	h.output("│ Status:  " + healthStatus)
	h.output(fmt.Sprintf("│ Score:   %.0f%%", calculateHealthScore(metrics, dhtInfo)))
	h.output("└────────────────────────────────────────────────────────┘")
	h.output("")
}

// truncatePeerID truncates a peer ID for display
func truncatePeerID(peerID string) string {
	if len(peerID) <= 16 {
		return peerID
	}
	return peerID[:8] + "..." + peerID[len(peerID)-4:]
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
func calculateHealthScore(metrics *app.MetricsFull, dhtInfo *app.DHTInfo) float64 {
	score := 0.0
	if metrics.CurrentConnections > 0 {
		score += 20
	}
	if metrics.CurrentConnections >= 3 {
		score += 10
	}
	if metrics.CurrentConnections >= 10 {
		score += 10
	}
	if dhtInfo.RoutingTableSize > 0 {
		score += 15
	}
	if dhtInfo.RoutingTableSize >= 5 {
		score += 15
	}
	score += metrics.ConnectionSuccessRate * 15
	score += metrics.MessageSuccessRate * 15
	return score
}

// boolToString converts a bool to "Active" or "Inactive"
func boolToString(b bool) string {
	if b {
		return "Active"
	}
	return "Inactive"
}
