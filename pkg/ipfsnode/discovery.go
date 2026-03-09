package ipfsnode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	bterrors "babylontower/pkg/errors"
	"babylontower/pkg/identity"

	pb "babylontower/pkg/proto"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multihash"
	"google.golang.org/protobuf/proto"
)

// HandlePeerFound is called by mDNS when a peer is discovered
// This implements the mdns.PeerNotif interface
func (n *Node) HandlePeerFound(peerInfo peer.AddrInfo) {
	n.mdnsCount.Add(1)
	n.peerMu.Lock()
	n.lastPeerFound = time.Now()
	n.peerMu.Unlock()

	logger.Debugw("mDNS discovered peer",
		"peer", peerInfo.ID,
		"addrs", peerInfo.Addrs,
		"total_mdns_discoveries", n.mdnsCount.Load())

	// Try to connect to discovered peer
	ctx, cancel := context.WithTimeout(n.ctx, ConnectionTimeout)
	defer cancel()

	if err := n.host.Connect(ctx, peerInfo); err != nil {
		logger.Warnw("failed to connect to mDNS discovered peer",
			"peer", peerInfo.ID,
			"error", err,
			"addrs", peerInfo.Addrs)
	} else {
		logger.Debugw("connected to mDNS discovered peer", "peer", peerInfo.ID)
	}
}

// startPeerDiscovery starts background peer discovery via DHT
func (n *Node) startPeerDiscovery() {
	// Check if DHT has any peers before starting discovery
	routingTableSize := len(n.dht.RoutingTable().ListPeers())
	if routingTableSize == 0 {
		logger.Debug("DHT routing table empty, skipping peer discovery")
		return
	}

	// Use the discovery service to find peers
	peerChan, err := n.discovery.FindPeers(n.ctx, n.config.ProtocolID)
	if err != nil {
		logger.Warnw("discovery FindPeers failed", "error", err)
		return
	}
	n.peerChan = peerChan

	// Process discovered peers in background
	bterrors.SafeGo("ipfs-peer-discovery", func() {
		for peerInfo := range n.peerChan {
			if peerInfo.ID == n.host.ID() || len(peerInfo.Addrs) == 0 {
				continue
			}

			// Try to connect to discovered peer
			ctx, cancel := context.WithTimeout(n.ctx, ConnectionTimeout)
			if err := n.host.Connect(ctx, peerInfo); err != nil {
				logger.Debugw("failed to connect to discovered peer", "peer", peerInfo.ID, "error", err)
			} else {
				logger.Debugw("connected to discovered peer", "peer", peerInfo.ID)
			}
			cancel()
		}
	})
}

// startRendezvousAdvertise starts periodic advertisement on the Babylon rendezvous namespace.
// This makes the node discoverable by other Babylon nodes via the IPFS DHT.
func (n *Node) startRendezvousAdvertise() {
	if n.discovery == nil {
		logger.Warn("discovery service not initialized, cannot start rendezvous advertise")
		return
	}

	// Initial advertise
	n.advertiseOnRendezvous()

	// Periodic re-advertisement
	bterrors.SafeGo("rendezvous-readvertise", func() {
		ticker := time.NewTicker(RendezvousAdvertiseInterval)
		defer ticker.Stop()

		for {
			select {
			case <-n.ctx.Done():
				return
			case <-ticker.C:
				n.advertiseOnRendezvous()
			}
		}
	})
}

// advertiseOnRendezvous advertises this node on the Babylon DHT rendezvous namespace
func (n *Node) advertiseOnRendezvous() {
	ctx, cancel := context.WithTimeout(n.ctx, 30*time.Second)
	defer cancel()

	_, err := n.discovery.Advertise(ctx, BabylonRendezvousNS)
	if err != nil {
		logger.Debugw("rendezvous advertise failed (normal during startup)", "error", err)
		return
	}

	n.rendezvousActive.Store(true)
	logger.Infow("advertised on Babylon rendezvous",
		"namespace", BabylonRendezvousNS,
		"peer_id", n.host.ID())
}

// startRendezvousDiscovery starts periodic discovery of Babylon peers via DHT rendezvous.
// It searches frequently while Babylon bootstrap is deferred (every 30s),
// then slows down once bootstrap completes (every 5m).
func (n *Node) startRendezvousDiscovery() {
	if n.discovery == nil {
		logger.Warn("discovery service not initialized, cannot start rendezvous discovery")
		return
	}

	bterrors.SafeGo("rendezvous-discovery", func() {
		// Start with fast interval while bootstrap is pending
		ticker := time.NewTicker(RendezvousDiscoveryInterval)
		defer ticker.Stop()
		slowMode := false

		for {
			select {
			case <-n.ctx.Done():
				return
			case <-ticker.C:
				// Switch to slow interval once Babylon bootstrap completes
				if !slowMode && n.babylonBootstrapComplete.Load() {
					slowMode = true
					ticker.Reset(RendezvousDiscoverySlowInterval)
					logger.Debugw("rendezvous discovery switching to slow interval")
				}

				// Skip if IPFS DHT isn't ready
				if !n.ipfsBootstrapComplete.Load() {
					continue
				}

				ctx, cancel := context.WithTimeout(n.ctx, 20*time.Second)
				peers := n.discoverBabylonPeers(ctx)
				cancel()

				if len(peers) == 0 {
					continue
				}

				// Save and connect to discovered peers
				n.saveDiscoveredBabylonPeers(peers)

				connectCtx, connectCancel := context.WithTimeout(n.ctx, 15*time.Second)
				connected := n.connectToPeersParallel(connectCtx, peers)
				connectCancel()

				logger.Infow("rendezvous discovery found Babylon peers",
					"discovered", len(peers),
					"connected", connected)

				// Trigger lazy bootstrap if Babylon DHT is still deferred
				if n.babylonBootstrapDeferred.Load() && connected > 0 {
					if err := n.TriggerLazyBootstrap(); err != nil {
						logger.Debugw("lazy bootstrap trigger failed", "error", err)
					}
				}
			}
		}
	})
}

// discoverBabylonPeers discovers other Babylon nodes via DHT rendezvous
func (n *Node) discoverBabylonPeers(ctx context.Context) []peer.AddrInfo {
	if n.discovery == nil {
		return nil
	}

	peerChan, err := n.discovery.FindPeers(ctx, BabylonRendezvousNS)
	if err != nil {
		logger.Warnw("rendezvous FindPeers failed", "error", err)
		return nil
	}

	var peers []peer.AddrInfo
	for p := range peerChan {
		if p.ID == n.host.ID() || len(p.Addrs) == 0 {
			continue
		}
		peers = append(peers, p)
		logger.Debugw("discovered Babylon peer via rendezvous", "peer", p.ID)
	}

	if len(peers) > 0 {
		logger.Infow("rendezvous discovery found Babylon peers", "count", len(peers))
	}

	return peers
}

// startDHTMaintenance starts a background goroutine that periodically refreshes the DHT
func (n *Node) startDHTMaintenance() {
	bterrors.SafeGo("ipfs-dht-maintenance", func() {
		ticker := time.NewTicker(DHTRefreshInterval)
		defer ticker.Stop()
		defer close(n.dhtMaintenanceDone)

		logger.Debugw("DHT maintenance started", "interval", DHTRefreshInterval)

		for {
			select {
			case <-n.ctx.Done():
				logger.Debug("DHT maintenance stopping")
				return
			case <-ticker.C:
				// Check if DHT is ready
				if n.dht == nil {
					logger.Debugw("DHT maintenance skipped - DHT not initialized")
					continue
				}

				// Check if routing table has peers
				routingTableSize := len(n.dht.RoutingTable().ListPeers())
				if routingTableSize == 0 {
					logger.Debugw("DHT maintenance skipped - routing table empty")
					continue
				}

				// Refresh routing table by querying a random peer ID
				randomID := peer.ID(fmt.Sprintf("refresh-%d", time.Now().UnixNano()))
				_, err := n.dht.GetClosestPeers(n.ctx, string(randomID))
				if err != nil {
					logger.Debugw("DHT refresh query failed", "error", err)
				} else {
					logger.Debugw("DHT routing table refreshed")
				}

				// Re-advertise self to stay discoverable
				ctx, cancel := context.WithTimeout(n.ctx, 10*time.Second)
				if err := n.AdvertiseSelf(ctx); err != nil {
					logger.Debugw("self-advertisement failed", "error", err)
				}
				cancel()

				// Log peer count
				peerCount := len(n.host.Network().Peers())
				logger.Debugw("DHT maintenance check", "connected_peers", peerCount)
			}
		}
	})

	// Start connection health checks
	bterrors.SafeGo("ipfs-health-checks", func() {
		n.startConnectionHealthChecks()
	})
}

// startConnectionHealthChecks periodically checks connection health and reconnects if needed
func (n *Node) startConnectionHealthChecks() {
	ticker := time.NewTicker(n.healthCheckInterval)
	defer ticker.Stop()

	logger.Debugw("connection health checks started", "interval", n.healthCheckInterval)

	for {
		select {
		case <-n.ctx.Done():
			logger.Debug("connection health checks stopping")
			return
		case <-ticker.C:
			n.performHealthCheck()
		}
	}
}

// AdvertiseSelf refreshes our presence in the DHT by querying closest peers
// This helps keep our peer record visible in the routing table
func (n *Node) AdvertiseSelf(ctx context.Context) error {
	if !n.isStarted.Load() {
		return ErrNodeNotStarted
	}

	// Check if DHT is initialized
	if n.dht == nil {
		logger.Debugw("AdvertiseSelf skipped - DHT not initialized")
		return nil
	}

	// Check if host is initialized
	if n.host == nil {
		logger.Debugw("AdvertiseSelf skipped - host not initialized")
		return nil
	}

	// Method 1: Put our peer record into the DHT
	// Create a provider record by providing our own PeerID as the "content"
	// This makes us discoverable via FindPeer queries
	mh, err := multihash.Sum([]byte(n.host.ID()), multihash.SHA2_256, -1)
	if err != nil {
		return fmt.Errorf("failed to hash peer ID: %w", err)
	}
	peerCID := cid.NewCidV1(cid.Raw, mh)

	// Provide our PeerID to the DHT - this publishes our addresses
	err = n.dht.Provide(ctx, peerCID, true)
	if err != nil {
		// DHT provide often fails when node is starting or has few peers - log at debug level
		logger.Debugw("DHT provide failed (normal during startup)", "peer", n.host.ID(), "error", err)
		// Continue with fallback method
	}

	// Method 2: Query closest peers to refresh our presence
	closestPeers, err := n.dht.GetClosestPeers(ctx, string(n.host.ID()))
	if err != nil {
		return fmt.Errorf("failed to advertise self: %w", err)
	}

	logger.Debugw("advertised self to DHT",
		"peer_id", n.host.ID(),
		"closest_peers", len(closestPeers),
		"provide_cid", peerCID.String())

	return nil
}

// ==================== Identity-Based Discovery ====================

// FindContactByPubkey finds a contact's peer information by their Ed25519 public key
// This fetches the IdentityDocument from DHT and extracts peer addresses
func (n *Node) FindContactByPubkey(ctx context.Context, pubkey []byte) (*ContactDiscoveryResult, error) {
	if !n.isStarted.Load() {
		return nil, ErrNodeNotStarted
	}

	// Fetch identity document from DHT
	dhtKey := identity.DeriveIdentityDHTKey(pubkey)
	data, err := n.dht.GetValue(ctx, dhtKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch identity document: %w", err)
	}

	// Unmarshal document
	var doc pb.IdentityDocument
	if err := proto.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal identity document: %w", err)
	}

	// Verify document
	if err := identity.VerifyIdentityDocument(&doc); err != nil {
		return nil, fmt.Errorf("invalid identity document: %w", err)
	}

	// Extract peer information from devices
	// In a full implementation, devices would contain multiaddrs
	// For now, we use the DHT to find peers near this identity
	result := &ContactDiscoveryResult{
		IdentityDocument: &doc,
		IsOnline:         false,
		PeerID:           "",
		Multiaddrs:       nil,
	}

	// Try to find peer via DHT using identity pubkey as search key
	closestPeers, err := n.dht.GetClosestPeers(ctx, dhtKey)
	if err == nil && len(closestPeers) > 0 {
		// Check if any closest peer is connected
		for _, p := range closestPeers {
			if n.host.Network().Connectedness(p) == network.Connected {
				peerInfo := n.host.Peerstore().PeerInfo(p)
				if len(peerInfo.Addrs) > 0 {
					result.IsOnline = true
					result.PeerID = p.String()
					result.Multiaddrs = make([]string, len(peerInfo.Addrs))
					for i, addr := range peerInfo.Addrs {
						result.Multiaddrs[i] = addr.String()
					}
					break
				}
			}
		}
	}

	return result, nil
}

// ContactDiscoveryResult contains the result of a contact discovery operation
type ContactDiscoveryResult struct {
	// IdentityDocument is the fetched identity document
	IdentityDocument *pb.IdentityDocument
	// IsOnline indicates if the contact appears to be online
	IsOnline bool
	// PeerID is the contact's libp2p peer ID (if found)
	PeerID string
	// Multiaddrs is the list of contact's addresses (if found)
	Multiaddrs []string
}

// PublishPresenceAnnouncement publishes a presence announcement to signal online status
// This helps contacts know we're available for direct messaging
func (n *Node) PublishPresenceAnnouncement(ctx context.Context, identityPub []byte) error {
	if !n.isStarted.Load() {
		return ErrNodeNotStarted
	}

	// Check if host is initialized
	if n.host == nil {
		logger.Debugw("PublishPresenceAnnouncement skipped - host not initialized")
		return nil
	}

	// Check if pubsub is initialized
	if n.pubsub == nil {
		logger.Debugw("PublishPresenceAnnouncement skipped - pubsub not initialized")
		return nil
	}

	// Create presence announcement (simple JSON format)
	announcement := make(map[string]interface{})
	announcement["identity_pub"] = hex.EncodeToString(identityPub)
	announcement["peer_id"] = string(n.host.ID())
	announcement["timestamp"] = uint64(time.Now().Unix())

	// Get our multiaddrs
	addrs := make([]string, 0)
	for _, addr := range n.host.Addrs() {
		// Only include non-local addresses
		addrStr := addr.String()
		if !isLocalAddress(addrStr) {
			addrs = append(addrs, addrStr+"/p2p/"+string(n.host.ID()))
		}
	}
	announcement["addrs"] = addrs

	// Serialize to JSON
	data, err := json.Marshal(announcement)
	if err != nil {
		return fmt.Errorf("failed to marshal presence announcement: %w", err)
	}

	// Publish to presence topic
	topic := PresenceTopicFromPubkey(identityPub)
	if err := n.Publish(topic, data); err != nil {
		return fmt.Errorf("failed to publish presence announcement: %w", err)
	}

	logger.Debugw("published presence announcement", "topic", topic, "addrs", len(addrs))
	return nil
}

// SubscribeToPresenceTopic subscribes to a contact's presence topic
// This allows us to receive their online/offline announcements
func (n *Node) SubscribeToPresenceTopic(ctx context.Context, identityPub []byte) (*Subscription, error) {
	if !n.isStarted.Load() {
		return nil, ErrNodeNotStarted
	}

	topic := PresenceTopicFromPubkey(identityPub)
	return n.Subscribe(topic)
}

// PresenceTopicFromPubkey derives a presence topic from a public key
func PresenceTopicFromPubkey(pubkey []byte) string {
	hash := sha256.Sum256(pubkey)
	return "babylon-presence-" + hex.EncodeToString(hash[:8])
}

// isLocalAddress checks if an address is a local/private address
func isLocalAddress(addr string) bool {
	// Check for localhost/private IP ranges
	return containsAny(addr, []string{
		"/127.0.0.1",
		"/10.",
		"/192.168.",
		"/172.16.",
		"/172.17.",
		"/172.18.",
		"/172.19.",
		"/172.2",
		"/172.30.",
		"/172.31.",
		"/::1",
		"/fc",
		"/fd",
	})
}

// containsAny checks if a string contains any of the substrings
func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if len(s) >= len(sub) && s[:len(sub)] == sub {
			return true
		}
	}
	return false
}
