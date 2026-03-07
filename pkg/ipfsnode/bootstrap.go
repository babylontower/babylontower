package ipfsnode

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"babylontower/pkg/storage"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	"github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
)

// BootstrapResult contains statistics about the bootstrap process
type BootstrapResult struct {
	StoredPeersAttempted  int
	StoredPeersConnected  int
	ConfigPeersAttempted  int
	ConfigPeersConnected  int
	BabylonPeersAttempted int
	BabylonPeersConnected int
	PubSubDiscoveredCount int
	TotalConnected        int
	RoutingTableSize      int
	Duration              time.Duration
	VerifiedPeers         int // Number of peers verified as responsive
}

// bootstrapIPFSDHT bootstraps to PUBLIC IPFS DHT.
// This is the TRANSPORT LAYER bootstrap - used only for:
// - PubSub topic membership
// - Basic libp2p connectivity
// - General DHT routing
//
// This function runs asynchronously and sets ipfsBootstrapComplete on success.
func (n *Node) bootstrapIPFSDHT() error {
	logger.Info("bootstrapping IPFS DHT (transport layer)...")
	startTime := time.Now()

	ctx, cancel := context.WithTimeout(n.ctx, DHTBootstrapTimeout)
	defer cancel()

	// Start DHT bootstrap process (schedules routing table refresh)
	if err := n.dht.Bootstrap(ctx); err != nil {
		return fmt.Errorf("IPFS DHT bootstrap failed: %w", err)
	}

	// Stage 1: Try stored peers from previous sessions (fast path for returning nodes)
	storedPeers, _ := n.loadStoredPeers()
	storedConnected := 0
	if len(storedPeers) > 0 {
		storedCtx, storedCancel := context.WithTimeout(ctx, 15*time.Second)
		storedConnected = n.connectToPeersParallel(storedCtx, storedPeers)
		storedCancel()
		logger.Infow("IPFS bootstrap stage 1: stored peers",
			"connected", storedConnected, "attempted", len(storedPeers))
	}

	// Stage 2: Connect to configured bootstrap peers (with DNS resolution),
	// falling back to well-known IPFS bootstrap peers if config is empty
	configConnected := 0
	bootstrapPeers := n.config.BootstrapPeers
	if len(bootstrapPeers) == 0 {
		bootstrapPeers = DefaultConfig().BootstrapPeers
		logger.Infow("no configured bootstrap peers, using well-known IPFS defaults",
			"count", len(bootstrapPeers))
	}
	if len(bootstrapPeers) > 0 {
		configConnected = n.connectToBootstrapPeersList(ctx, bootstrapPeers)
		logger.Infow("IPFS bootstrap stage 2: bootstrap peers",
			"connected", configConnected, "total", len(bootstrapPeers))
	}

	// Stage 3: Wait for DHT routing table to populate
	totalConnected := len(n.host.Network().Peers())
	if totalConnected > 0 {
		select {
		case <-n.dht.RefreshRoutingTable():
			// Routing table refreshed
		case <-ctx.Done():
			// Timeout — continue with what we have
		}
	}

	routingTableSize := len(n.dht.RoutingTable().ListPeers())
	connectedPeers := len(n.host.Network().Peers())

	if connectedPeers == 0 && routingTableSize == 0 {
		n.logConnectionDiagnostics()
	}

	// Mark complete — node can function even with limited routing
	n.ipfsBootstrapComplete.Store(true)
	logger.Infow("IPFS DHT bootstrap complete (transport layer)",
		"routing_table_size", routingTableSize,
		"connected_peers", connectedPeers,
		"stored_connected", storedConnected,
		"config_connected", configConnected,
		"duration", time.Since(startTime))
	return nil
}

// bootstrapBabylonDHT bootstraps to BABYLON DHT.
// This is the PROTOCOL LAYER bootstrap - used for:
// - All Babylon protocol operations
// - Messaging and groups
// - Identity document storage/retrieval
//
// Bootstrap stages:
// Stage 1: Try stored Babylon peers (fast path for returning nodes)
// Stage 2: Use PubSub discovery to find Babylon peers
// Stage 3: Defer - will be triggered by incoming messages (lazy bootstrap)
//
// This function runs asynchronously and sets babylonBootstrapComplete on success.
// Returns ErrBabylonBootstrapDeferred if bootstrap is deferred for lazy triggering.
func (n *Node) bootstrapBabylonDHT() error {
	logger.Info("bootstrapping Babylon DHT (protocol layer)...")

	// Check if Babylon DHT is initialized
	if n.babylonDHT == nil {
		return errors.New("Babylon DHT not initialized")
	}

	ctx, cancel := context.WithTimeout(n.ctx, DHTBootstrapTimeout)
	defer cancel()

	result := &BootstrapResult{}

	// ========== STAGE 1: Try stored Babylon peers ==========
	// Fast path for returning nodes with previously discovered peers
	storedBabylonPeers, err := n.loadStoredBabylonPeers()
	if err != nil {
		logger.Debugw("failed to load stored Babylon peers", "error", err)
	} else if len(storedBabylonPeers) > 0 {
		logger.Debugw("Babylon bootstrap stage 1: connecting to stored Babylon peers",
			"count", len(storedBabylonPeers))
		result.BabylonPeersAttempted = len(storedBabylonPeers)

		babylonCtx, babylonCancel := context.WithTimeout(ctx, 10*time.Second)
		result.BabylonPeersConnected = n.connectToPeersParallel(babylonCtx, storedBabylonPeers)
		babylonCancel()

		logger.Debugw("stored Babylon peer bootstrap complete",
			"connected", result.BabylonPeersConnected,
			"attempted", result.BabylonPeersAttempted)

		// Check if we have enough connections
		minBabylonPeers := DefaultMinBabylonPeersRequired
		if n.config.Bootstrap != nil {
			minBabylonPeers = n.config.Bootstrap.MinBabylonPeersRequired
		}

		if result.BabylonPeersConnected >= minBabylonPeers {
			logger.Infow("Babylon DHT bootstrap complete (stored peers)",
				"connected", result.BabylonPeersConnected)
			n.babylonBootstrapComplete.Store(true)
			n.babylonBootstrapDeferred.Store(false)
			return nil
		}
	}

	// ========== STAGE 2: Use DHT rendezvous discovery ==========
	// Discover other Babylon nodes via rendezvous on the IPFS DHT
	logger.Debugw("Babylon bootstrap stage 2: DHT rendezvous discovery")

	// Wait for IPFS DHT to be ready before rendezvous lookup
	if err := n.WaitForIPFSBootstrap(30 * time.Second); err != nil {
		logger.Debugw("IPFS bootstrap not ready for rendezvous", "error", err)
	}

	discoveredPeers := n.discoverBabylonPeers(ctx)
	result.PubSubDiscoveredCount = len(discoveredPeers) // reuse field

	if len(discoveredPeers) > 0 {
		logger.Debugw("rendezvous discovery found Babylon peers",
			"count", len(discoveredPeers))

		// Save discovered peers to storage
		n.saveDiscoveredBabylonPeers(discoveredPeers)

		// Connect to discovered peers
		connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
		result.BabylonPeersAttempted += len(discoveredPeers)
		result.BabylonPeersConnected += n.connectToPeersParallel(connectCtx, discoveredPeers)
		connectCancel()

		logger.Debugw("rendezvous discovered peer connections complete",
			"connected", result.BabylonPeersConnected,
			"attempted", result.BabylonPeersAttempted)
	}

	// ========== STAGE 3: Bootstrap Babylon DHT ==========
	// If we connected any peers, bootstrap the Babylon DHT routing table
	minBabylonPeers := 1
	if n.config.Bootstrap != nil {
		minBabylonPeers = n.config.Bootstrap.MinBabylonPeersRequired
	}

	if result.BabylonPeersConnected >= minBabylonPeers {
		// Add connected peers to Babylon DHT routing table
		for _, p := range discoveredPeers {
			if n.host.Network().Connectedness(p.ID) == network.Connected {
				n.babylonDHT.RoutingTable().TryAddPeer(p.ID, true, false)
			}
		}

		// Actually bootstrap the Babylon DHT
		if err := n.babylonDHT.Bootstrap(ctx); err != nil {
			logger.Warnw("Babylon DHT bootstrap call failed", "error", err)
		}

		logger.Infow("Babylon DHT bootstrap complete",
			"connected", result.BabylonPeersConnected)
		n.babylonBootstrapComplete.Store(true)
		n.babylonBootstrapDeferred.Store(false)
		return nil
	}

	// ========== STAGE 4: Defer for lazy bootstrap ==========
	// Not enough peers found - defer bootstrap until triggered by /connect
	logger.Info("Babylon DHT bootstrap deferred - waiting for connections")
	n.babylonBootstrapDeferred.Store(true)
	n.babylonBootstrapComplete.Store(false)

	return ErrBabylonBootstrapDeferred
}

// TriggerLazyBootstrap triggers Babylon bootstrap on-demand.
// Called when a peer is manually connected via /connect.
func (n *Node) TriggerLazyBootstrap() error {
	// Don't trigger if already complete
	if n.babylonBootstrapComplete.Load() {
		logger.Debugw("lazy bootstrap skipped - already complete")
		return nil
	}

	if n.babylonDHT == nil {
		return errors.New("Babylon DHT not initialized")
	}

	logger.Infow("lazy Babylon bootstrap triggered")

	// Try to add all currently connected peers to Babylon DHT routing table
	for _, pid := range n.host.Network().Peers() {
		n.babylonDHT.RoutingTable().TryAddPeer(pid, true, false)
	}

	// Bootstrap the Babylon DHT
	ctx, cancel := context.WithTimeout(n.ctx, 30*time.Second)
	defer cancel()

	if err := n.babylonDHT.Bootstrap(ctx); err != nil {
		logger.Warnw("Babylon DHT bootstrap call failed", "error", err)
	}

	// Mark as complete if we have peers in the routing table
	rtSize := len(n.babylonDHT.RoutingTable().ListPeers())
	if rtSize > 0 {
		logger.Infow("Babylon DHT bootstrap complete (lazy)",
			"routing_table_size", rtSize)
		n.babylonBootstrapComplete.Store(true)
		n.babylonBootstrapDeferred.Store(false)
	}

	return nil
}

// savePeerForLater saves a peer's address info for later connection attempts
func (n *Node) savePeerForLater(p peer.ID) {
	// Get peer's addresses
	peerInfo := n.host.Peerstore().PeerInfo(p)
	if len(peerInfo.Addrs) == 0 {
		return
	}

	// Convert to storage record
	record := &storage.PeerRecord{
		PeerID:        p.String(),
		Multiaddrs:    multiaddrsToStrings(peerInfo.Addrs),
		FirstSeen:     time.Now(),
		LastSeen:      time.Now(),
		LastConnected: time.Time{},
		ConnectCount:  0,
		FailCount:     0,
		Source:        storage.SourceBabylon,
		Protocols:     []string{},
		LatencyMs:     0,
	}

	// Save to storage
	if n.config.Storage != nil {
		if err := n.config.Storage.AddPeer(record); err != nil {
			logger.Debugw("failed to save peer for later", "peer", p, "error", err)
		} else {
			logger.Debugw("saved peer for later connection", "peer", p)
		}
	}
}

// saveDiscoveredBabylonPeers saves discovered Babylon peers to storage
func (n *Node) saveDiscoveredBabylonPeers(peers []peer.AddrInfo) {
	if n.config.Storage == nil {
		return
	}

	now := time.Now()
	saved := 0

	for _, peerInfo := range peers {
		if len(peerInfo.Addrs) == 0 {
			continue
		}

		addrs := make([]string, len(peerInfo.Addrs))
		for i, addr := range peerInfo.Addrs {
			addrs[i] = addr.String()
		}

		record := &storage.PeerRecord{
			PeerID:     peerInfo.ID.String(),
			Multiaddrs: addrs,
			FirstSeen:  now,
			LastSeen:   now,
			Source:     storage.SourceBabylon,
		}

		if err := n.config.Storage.AddPeer(record); err != nil {
			logger.Debugw("failed to save discovered Babylon peer",
				"peer", peerInfo.ID, "error", err)
			continue
		}
		saved++
	}

	if saved > 0 {
		logger.Debugw("saved discovered Babylon peers to storage", "count", saved)
	}
}

// multiaddrsToStrings converts multiaddrs to string slice
func multiaddrsToStrings(addrs []multiaddr.Multiaddr) []string {
	result := make([]string, len(addrs))
	for i, addr := range addrs {
		result[i] = addr.String()
	}
	return result
}


// loadStoredBabylonPeers loads previously discovered Babylon peers from storage
func (n *Node) loadStoredBabylonPeers() ([]peer.AddrInfo, error) {
	if n.config.Storage == nil {
		return nil, nil // No storage configured
	}

	// Get peers from storage with SourceBabylon
	peers, err := n.config.Storage.ListPeersBySource(storage.SourceBabylon)
	if err != nil {
		return nil, fmt.Errorf("failed to load stored Babylon peers: %w", err)
	}

	// Convert to AddrInfo
	var addrInfos []peer.AddrInfo
	storedPeerTimeoutSecs := DefaultStoredPeerTimeoutSecs
	if n.config.Bootstrap != nil {
		storedPeerTimeoutSecs = n.config.Bootstrap.StoredPeerTimeoutSecs
	}
	for _, record := range peers {
		// Skip stale peers (older than configured timeout)
		maxAge := time.Duration(storedPeerTimeoutSecs) * time.Second
		if record.IsStale(maxAge) {
			logger.Debugw("skipping stale Babylon peer",
				"peer", record.PeerID,
				"last_seen", record.LastSeen)
			continue
		}

		// Parse multiaddrs
		var addrs []multiaddr.Multiaddr
		for _, addrStr := range record.Multiaddrs {
			addr, err := multiaddr.NewMultiaddr(addrStr)
			if err != nil {
				logger.Debugw("failed to parse stored multiaddr",
					"peer", record.PeerID, "addr", addrStr, "error", err)
				continue
			}
			addrs = append(addrs, addr)
		}

		if len(addrs) > 0 {
			pid, err := peer.Decode(record.PeerID)
			if err != nil {
				logger.Debugw("failed to parse stored peer ID",
					"peer", record.PeerID, "error", err)
				continue
			}

			addrInfos = append(addrInfos, peer.AddrInfo{
				ID:    pid,
				Addrs: addrs,
			})
		}
	}

	logger.Debugw("loaded stored Babylon peers", "count", len(addrInfos))
	return addrInfos, nil
}

// loadStoredPeers loads all stored peers (legacy method for IPFS bootstrap peers)
func (n *Node) loadStoredPeers() ([]peer.AddrInfo, error) {
	// Use pre-loaded stored peers from config
	// These are loaded in main.go before node start
	if len(n.config.StoredPeers) > 0 {
		logger.Debugw("using pre-loaded stored peers", "count", len(n.config.StoredPeers))
		return n.config.StoredPeers, nil
	}
	return nil, nil
}

// isRetryableError determines if a connection error is worth retrying
// Returns false for errors that indicate the peer is unreachable or rate-limited
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()

	// Don't retry on these errors - they indicate fundamental issues
	nonRetryable := []string{
		"dial backoff",       // Rate limited by libp2p
		"peer identified",    // Peer ID mismatch
		"invalid peer",       // Bad peer ID
		"connection refused", // Port closed / service not running
		"no route to host",   // Network unreachable
		"i/o timeout",        // Network timeout (may be retryable but often indicates unreachable)
		"connection reset",   // Connection forcibly closed
	}

	for _, nr := range nonRetryable {
		if strings.Contains(errStr, nr) {
			return false
		}
	}

	return true
}

// connectToPeersParallel attempts parallel connections to multiple peers
// with improved exponential backoff and dial backoff handling
func (n *Node) connectToPeersParallel(ctx context.Context, peers []peer.AddrInfo) int {
	var wg sync.WaitGroup
	connected := atomic.Int32{}
	failed := atomic.Int32{}

	// Limit parallel connections to avoid resource exhaustion
	maxParallel := 8
	sem := make(chan struct{}, maxParallel)

	for _, peerInfo := range peers {
		if peerInfo.ID == n.host.ID() || len(peerInfo.Addrs) == 0 {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(pi peer.AddrInfo) {
			defer wg.Done()
			defer func() { <-sem }()

			// Check if peer is already connected
			if n.host.Network().Connectedness(pi.ID) == network.Connected {
				logger.Debugw("peer already connected", "peer", pi.ID)
				connected.Add(1)
				return
			}

			// Try with exponential backoff, but respect dial backoff
			var lastErr error
			maxAttempts := 3
			baseBackoff := 2 * time.Second

			for attempt := 0; attempt < maxAttempts; attempt++ {
				dialCtx, dialCancel := context.WithTimeout(ctx, DialTimeout)

				err := n.host.Connect(dialCtx, pi)
				dialCancel()

				if err == nil {
					connected.Add(1)
					logger.Debugw("connected to peer", "peer", pi.ID, "addrs", pi.Addrs)
					return
				}

				lastErr = err

				// Check if this is a dial backoff error - don't retry immediately
				if strings.Contains(err.Error(), "dial backoff") {
					logger.Debugw("dial backoff detected, skipping retries for this peer",
						"peer", pi.ID, "error", err)
					break
				}

				// Check if context is cancelled
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Only retry on certain error types
				if !isRetryableError(err) {
					logger.Debugw("non-retryable error, skipping retries",
						"peer", pi.ID, "error", err)
					break
				}

				// Exponential backoff with jitter: 2s, 4s, 8s
				backoff := baseBackoff * time.Duration(1<<uint(attempt))
				jitter := time.Duration(rand.Int63n(int64(backoff) / 4))
				totalBackoff := backoff + jitter

				logger.Debugw("connection attempt failed, backing off",
					"peer", pi.ID,
					"attempt", attempt+1,
					"max_attempts", maxAttempts,
					"backoff", totalBackoff,
					"error", err)

				select {
				case <-ctx.Done():
					return
				case <-time.After(totalBackoff):
					// Continue to next attempt
				}
			}

			// All attempts failed
			if lastErr != nil {
				failed.Add(1)
				if failed.Load() > int32(len(peers))/2 {
					logger.Debugw("connection attempts failed for peer",
						"peer", pi.ID,
						"error", lastErr,
						"total_failed", failed.Load())
				}
			}
		}(peerInfo)
	}

	// Wait for all connections with overall timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All connections attempted
	case <-ctx.Done():
		logger.Debug("bootstrap context cancelled")
	}

	connCount := int(connected.Load())
	failCount := int(failed.Load())
	logger.Debugw("peer connection attempts complete",
		"connected", connCount,
		"failed", failCount,
		"total", len(peers))

	return connCount
}

// connectToBootstrapPeersWithDNS connects to config bootstrap peers with DNS resolution
func (n *Node) connectToBootstrapPeersWithDNS(ctx context.Context) int {
	return n.connectToBootstrapPeersList(ctx, n.config.BootstrapPeers)
}

// connectToBootstrapPeersList connects to the given bootstrap peers with DNS resolution
func (n *Node) connectToBootstrapPeersList(ctx context.Context, peerAddrs []string) int {
	var wg sync.WaitGroup
	connected := atomic.Int32{}
	failed := atomic.Int32{}

	// Limit parallel connections
	maxParallel := 4
	sem := make(chan struct{}, maxParallel)

	for _, addrStr := range peerAddrs {
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			logger.Warnw("invalid bootstrap addr", "addr", addrStr, "error", err)
			continue
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(addr multiaddr.Multiaddr) {
			defer wg.Done()
			defer func() { <-sem }()

			// Resolve DNS addresses with timeout
			resolveCtx, resolveCancel := context.WithTimeout(ctx, 5*time.Second)
			resolvedAddrs, err := madns.Resolve(resolveCtx, addr)
			resolveCancel()

			if err != nil {
				logger.Debugw("failed to resolve bootstrap addr",
					"addr", addr, "error", err)
				failed.Add(1)
				return
			}

			// Try each resolved address
			var connectedTo bool
			for _, resolved := range resolvedAddrs {
				peerInfo, err := peer.AddrInfoFromP2pAddr(resolved)
				if err != nil {
					logger.Debugw("failed to parse resolved addr",
						"addr", resolved, "error", err)
					continue
				}

				if peerInfo.ID == n.host.ID() {
					continue
				}

				// Check if already connected
				if n.host.Network().Connectedness(peerInfo.ID) == network.Connected {
					logger.Debugw("already connected to bootstrap peer", "peer", peerInfo.ID)
					connected.Add(1)
					connectedTo = true
					break
				}

				dialCtx, dialCancel := context.WithTimeout(ctx, DialTimeout)
				err = n.host.Connect(dialCtx, *peerInfo)
				dialCancel()

				if err == nil {
					connected.Add(1)
					connectedTo = true
					logger.Debugw("connected to bootstrap peer",
						"peer", peerInfo.ID, "addr", resolved)
					break
				}

				if strings.Contains(err.Error(), "dial backoff") {
					logger.Debugw("bootstrap peer dial backoff",
						"peer", peerInfo.ID, "addr", resolved)
				} else {
					logger.Debugw("failed to connect to bootstrap peer",
						"peer", peerInfo.ID, "addr", resolved, "error", err)
				}
			}

			if !connectedTo {
				failed.Add(1)
			}
		}(ma)
	}

	// Wait for all connections with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		logger.Debug("bootstrap context cancelled")
	}

	connCount := int(connected.Load())
	failCount := int(failed.Load())
	logger.Debugw("bootstrap peer connection complete",
		"connected", connCount,
		"failed", failCount,
		"total", len(peerAddrs))

	return connCount
}


// logConnectionDiagnostics logs detailed connection diagnostics for troubleshooting
func (n *Node) logConnectionDiagnostics() {
	logger.Warn("=== CONNECTION DIAGNOSTICS ===")

	info := n.GetNetworkInfo()
	logger.Warnw("network status",
		"peer_id", info.PeerID,
		"connected_peers", info.ConnectedPeerCount,
		"listen_addrs", info.ListenAddrs)

	dhtInfo := n.GetDHTInfo()
	logger.Warnw("dht status",
		"routing_table_size", dhtInfo.RoutingTableSize,
		"mode", dhtInfo.Mode)

	for _, p := range info.ConnectedPeers {
		logger.Debugw("connected peer",
			"peer", p.ID,
			"addresses", p.Addresses,
			"protocols", p.Protocols)
	}

	mdnsStats := n.GetMDnsStats()
	logger.Warnw("mdns status",
		"total_discoveries", mdnsStats.TotalDiscoveries,
		"last_peer_found", mdnsStats.LastPeerFound)

	logger.Warnw("swarm dialer backoff",
		"note", "use /clearpeers command to clear backoffs")

	logger.Warn("=== END DIAGNOSTICS ===")
}

// ClearPeerBackoff clears the dial backoff for a specific peer
func (n *Node) ClearPeerBackoff(peerID peer.ID) {
	if swarm, ok := n.host.Network().(*swarm.Swarm); ok {
		swarm.Backoff().Clear(peerID)
		logger.Debugw("cleared dial backoff for peer", "peer", peerID)
	}
}

// ClearAllBackoffs clears all dial backoffs for connected peers
func (n *Node) ClearAllBackoffs() {
	count := 0
	if swarm, ok := n.host.Network().(*swarm.Swarm); ok {
		for _, peerID := range n.host.Network().Peers() {
			swarm.Backoff().Clear(peerID)
			count++
		}
		logger.Infow("cleared dial backoffs", "count", count)
	}
}

// verifyBootstrapPeers verifies that connected bootstrap peers are responsive
// using the libp2p ping protocol (lightweight liveness check)
func (n *Node) verifyBootstrapPeers(ctx context.Context) int {
	connectedPeers := n.host.Network().Peers()
	if len(connectedPeers) == 0 {
		return 0
	}

	logger.Debugw("verifying bootstrap peers with ping", "count", len(connectedPeers))

	verified := atomic.Int32{}
	var wg sync.WaitGroup

	for _, peerID := range connectedPeers {
		wg.Add(1)
		go func(pid peer.ID) {
			defer wg.Done()

			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			resultChan := ping.Ping(pingCtx, n.host, pid)

			select {
			case result := <-resultChan:
				if result.Error == nil {
					verified.Add(1)
					logger.Debugw("bootstrap peer verified via ping",
						"peer", pid, "rtt", result.RTT)
				} else {
					logger.Debugw("bootstrap peer ping failed",
						"peer", pid, "error", result.Error)
					n.updatePeerScore(pid.String(), false, true)
				}
			case <-pingCtx.Done():
				logger.Debugw("bootstrap peer ping timeout", "peer", pid)
				n.updatePeerScore(pid.String(), false, true)
			}
		}(peerID)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		logger.Debugw("verification context cancelled",
			"verified_so_far", verified.Load())
	}

	count := int(verified.Load())
	logger.Infow("bootstrap peer verification complete",
		"verified", count, "total", len(connectedPeers))
	return count
}

// ConnectToBootstrapPeers connects to all configured bootstrap peers
func (n *Node) ConnectToBootstrapPeers() error {
	if !n.isStarted {
		return ErrNodeNotStarted
	}

	ctx, cancel := context.WithTimeout(n.ctx, ConnectionTimeout)
	defer cancel()

	connected := 0
	for _, addr := range n.config.BootstrapPeers {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			logger.Warnw("invalid bootstrap addr", "addr", addr, "error", err)
			continue
		}

		peerInfo, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			logger.Warnw("failed to parse bootstrap addr", "addr", addr, "error", err)
			continue
		}

		if err := n.host.Connect(ctx, *peerInfo); err != nil {
			logger.Debugw("failed to connect to bootstrap peer", "peer", peerInfo.ID, "error", err)
		} else {
			connected++
			logger.Debugw("connected to bootstrap peer", "peer", peerInfo.ID)
		}
	}

	if connected == 0 {
		logger.Warnw("no bootstrap peers connected")
	} else {
		logger.Debugw("bootstrap complete", "connected_peers", connected)
	}

	return nil
}

// ConnectToLocalNode connects to another node on the same machine
func (n *Node) ConnectToLocalNode(multiaddrStr, peerID string) error {
	if !n.isStarted {
		return ErrNodeNotStarted
	}

	var fullAddr string
	if strings.Contains(multiaddrStr, "/p2p/") || strings.Contains(multiaddrStr, "/ipfs/") {
		fullAddr = multiaddrStr
	} else {
		fullAddr = fmt.Sprintf("%s/p2p/%s", multiaddrStr, peerID)
	}

	return n.ConnectToPeer(fullAddr)
}
