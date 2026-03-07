// Package protocol implements the Babylon Tower Protocol v1 specification.
package protocol

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ipfs/go-log/v2"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

// Logger for the protocol package
var logger = log.Logger("babylontower/protocol")

// BootstrapOrchestrator handles the multi-stage bootstrap process.
// It separates IPFS DHT bootstrap (transport layer) from Babylon DHT bootstrap
// (protocol layer), providing fine-grained control and status tracking.
//
// Bootstrap Stages:
//   1. IPFS DHT Bootstrap: Connect to public IPFS bootstrap peers, join the
//      global DHT for peer routing and content addressing.
//   2. Babylon DHT Bootstrap: Discover Babylon Tower peers via PubSub, build
//      the protocol-specific overlay network.
//   3. Deferred Bootstrap: If insufficient Babylon peers are found, act as a
//      bootstrap helper for other nodes while waiting for peers.
type BootstrapOrchestrator struct {
	// config is the protocol configuration
	config *ProtocolConfig
	// networkNode is the underlying network interface
	networkNode NetworkNode
	// ipfsDHT is the IPFS DHT instance (transport layer)
	ipfsDHT *dht.IpfsDHT
	// babylonDHT is the Babylon DHT instance (protocol layer)
	// Note: In the current architecture, this shares the IPFS DHT but uses
	// custom validators and a separate namespace
	babylonDHT *dht.IpfsDHT
	// routingDiscovery is the libp2p routing discovery interface
	routingDiscovery *routing.RoutingDiscovery

	// Bootstrap state tracking (atomic for thread-safety)
	ipfsBootstrapComplete   atomic.Bool
	babylonBootstrapComplete atomic.Bool
	babylonBootstrapDeferred atomic.Bool
	rendezvousActive atomic.Bool

	// Bootstrap timing
	startTime        time.Time
	lastAttempt      time.Time
	consecutiveFails int

	// Mutex for protecting shared state
	mu sync.RWMutex

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// BootstrapResult contains the result of a bootstrap operation
type BootstrapResult struct {
	// Success indicates if bootstrap completed successfully
	Success bool
	// IPFSBootstrapComplete indicates if IPFS DHT bootstrap succeeded
	IPFSBootstrapComplete bool
	// BabylonBootstrapComplete indicates if Babylon DHT bootstrap succeeded
	BabylonBootstrapComplete bool
	// BabylonBootstrapDeferred indicates if Babylon bootstrap was deferred
	BabylonBootstrapDeferred bool
	// IPFSRoutingTableSize is the size of the IPFS DHT routing table
	IPFSRoutingTableSize int
	// BabylonPeersDiscovered is the number of Babylon peers discovered
	BabylonPeersDiscovered int
	// BabylonPeersConnected is the number of Babylon peers connected
	BabylonPeersConnected int
	// Duration is how long bootstrap took
	Duration time.Duration
	// Error contains any error that occurred
	Error error
}

// BootstrapStatus contains the current bootstrap status
type BootstrapStatus struct {
	// IPFS DHT (Transport Layer) Status
	IPFSBootstrapComplete bool
	IPFSRoutingTableSize  int

	// Babylon DHT (Protocol Layer) Status
	BabylonBootstrapComplete bool
	BabylonPeersStored       int
	BabylonPeersConnected    int
	BabylonBootstrapDeferred bool

	// Rendezvous Discovery Status
	RendezvousActive bool

	// Connection Summary
	TotalConnectedPeers int

	// Timing
	StartTime        time.Time
	LastAttempt      time.Time
	ConsecutiveFails int
}

// NewBootstrapOrchestrator creates a new bootstrap orchestrator.
// It requires the network node and configuration to perform bootstrap operations.
func NewBootstrapOrchestrator(
	networkNode NetworkNode,
	config *ProtocolConfig,
	ipfsDHT *dht.IpfsDHT,
	routingDiscovery *routing.RoutingDiscovery,
) *BootstrapOrchestrator {
	ctx, cancel := context.WithCancel(context.Background())

	orchestrator := &BootstrapOrchestrator{
		config:           config,
		networkNode:      networkNode,
		ipfsDHT:          ipfsDHT,
		babylonDHT:       ipfsDHT, // Share IPFS DHT for now, use namespace separation
		routingDiscovery: routingDiscovery,
		ctx:              ctx,
		cancel:           cancel,
		startTime:        time.Now(),
	}

	return orchestrator
}

// Bootstrap performs the complete multi-stage bootstrap process.
// It returns a BootstrapResult with detailed status information.
//
// The bootstrap process:
//   1. Waits for IPFS DHT to be ready (if not already complete)
//   2. Discovers Babylon Tower peers via PubSub
//   3. Connects to discovered Babylon peers
//   4. Validates sufficient peers are connected
//   5. Either completes or defers bootstrap based on peer count
func (b *BootstrapOrchestrator) Bootstrap(ctx context.Context) (*BootstrapResult, error) {
	b.mu.Lock()
	b.startTime = time.Now()
	b.lastAttempt = time.Now()
	b.mu.Unlock()

	logger.Info("Starting Babylon Tower protocol bootstrap")

	result := &BootstrapResult{
		Success: false,
	}

	// Stage 1: Ensure IPFS DHT bootstrap is complete
	ipfsComplete, err := b.ensureIPFSBootstrap(ctx)
	if err != nil {
		logger.Warnw("IPFS bootstrap failed", "error", err)
		result.Error = fmt.Errorf("IPFS bootstrap failed: %w", err)
		return result, err
	}
	result.IPFSBootstrapComplete = ipfsComplete
	result.IPFSRoutingTableSize = b.getIPFSRoutingTableSize()

	if !ipfsComplete {
		logger.Warn("IPFS bootstrap incomplete, deferring Babylon bootstrap")
		result.BabylonBootstrapDeferred = true
		b.babylonBootstrapDeferred.Store(true)
		return result, ErrBootstrapIncomplete
	}

	logger.Infow("IPFS DHT bootstrap complete", "routing_table_size", result.IPFSRoutingTableSize)

	// Stage 2: Babylon DHT bootstrap via PubSub discovery
	babylonComplete, babylonPeers, err := b.babylonBootstrap(ctx)
	if err != nil {
		logger.Warnw("Babylon bootstrap failed", "error", err)
		result.Error = fmt.Errorf("Babylon bootstrap failed: %w", err)
		b.consecutiveFails++
		return result, err
	}

	result.BabylonPeersDiscovered = babylonPeers
	result.BabylonPeersConnected = b.getBabylonConnectedPeers()
	result.BabylonBootstrapComplete = babylonComplete

	if babylonComplete {
		logger.Infow("Babylon DHT bootstrap complete",
			"peers_discovered", babylonPeers,
			"peers_connected", result.BabylonPeersConnected)

		b.ipfsBootstrapComplete.Store(true)
		b.babylonBootstrapComplete.Store(true)
		b.babylonBootstrapDeferred.Store(false)
		b.consecutiveFails = 0
		result.Success = true
	} else {
		logger.Warnw("Babylon bootstrap deferred - insufficient peers",
			"peers_discovered", babylonPeers,
			"min_required", b.config.MinBabylonPeersRequired)

		b.babylonBootstrapDeferred.Store(true)
		result.BabylonBootstrapDeferred = true
		b.consecutiveFails++
	}

	result.Duration = time.Since(b.startTime)
	return result, nil
}

// ensureIPFSBootstrap waits for the IPFS DHT bootstrap to complete.
// It checks the network node's bootstrap state and waits if necessary.
func (b *BootstrapOrchestrator) ensureIPFSBootstrap(ctx context.Context) (bool, error) {
	// Check if already complete
	if b.networkNode.IsIPFSBootstrapComplete() {
		logger.Debug("IPFS bootstrap already complete")
		return true, nil
	}

	// Wait for IPFS DHT to be ready
	timeout := b.config.DHTBootstrapTimeout
	if timeout <= 0 {
		timeout = DefaultBootstrapTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Wait for IPFS DHT to be ready by polling
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, timeout)
	defer timeoutCancel()

	for {
		select {
		case <-timeoutCtx.Done():
			logger.Warn("IPFS bootstrap timeout")
			return false, timeoutCtx.Err()
		case <-ticker.C:
			if b.networkNode.IsIPFSBootstrapComplete() {
				return true, nil
			}
		}
	}
}

// babylonBootstrap performs the Babylon DHT bootstrap via PubSub discovery.
// It discovers Babylon Tower peers and connects to them.
func (b *BootstrapOrchestrator) babylonBootstrap(ctx context.Context) (bool, int, error) {
	logger.Info("Starting Babylon DHT bootstrap via PubSub discovery")

	// Discover peers via PubSub
	peersDiscovered := 0

	// Use routing discovery to find Babylon peers
	if b.routingDiscovery != nil {
		// Advertise our presence
		if _, err := b.routingDiscovery.Advertise(ctx, "babylon"); err != nil {
			logger.Warnw("Failed to advertise Babylon service", "error", err)
		}

		// Find peers
		peerChan, err := b.routingDiscovery.FindPeers(ctx, "babylon")
		if err != nil {
			logger.Warnw("Failed to find Babylon peers", "error", err)
			return false, 0, err
		}

		// Connect to discovered peers
		for peerInfo := range peerChan {
			if peerInfo.ID == b.networkNode.Host().ID() {
				continue // Skip ourselves
			}

			if err := b.connectToPeer(ctx, peerInfo); err != nil {
				logger.Debugw("Failed to connect to Babylon peer",
					"peer", peerInfo.ID,
					"error", err)
				continue
			}

			peersDiscovered++
			logger.Debugw("Connected to Babylon peer",
				"peer", peerInfo.ID,
				"total", peersDiscovered)

			// Check if we have enough peers
			if peersDiscovered >= b.config.MinBabylonPeersRequired {
				break
			}
		}
	}

	// Also try direct DHT peer discovery
	dhtPeers := b.discoverPeersFromDHT(ctx)
	for _, peerInfo := range dhtPeers {
		if peersDiscovered >= b.config.MinBabylonPeersRequired {
			break
		}

		if err := b.connectToPeer(ctx, peerInfo); err != nil {
			continue
		}
		peersDiscovered++
	}

	// Check if we have enough peers
	enoughPeers := peersDiscovered >= b.config.MinBabylonPeersRequired
	return enoughPeers, peersDiscovered, nil
}

// discoverPeersFromDHT discovers peers from the DHT routing table.
func (b *BootstrapOrchestrator) discoverPeersFromDHT(ctx context.Context) []peer.AddrInfo {
	var peers []peer.AddrInfo

	if b.ipfsDHT == nil {
		return peers
	}

	// Get peers from routing table
	routingTable := b.ipfsDHT.RoutingTable()
	if routingTable == nil {
		return peers
	}

	peerIDs := routingTable.ListPeers()
	for _, pid := range peerIDs {
		if pid == b.networkNode.Host().ID() {
			continue
		}

		addrInfo := b.networkNode.Host().Peerstore().PeerInfo(pid)
		if len(addrInfo.Addrs) > 0 {
			peers = append(peers, addrInfo)
		}
	}

	return peers
}

// connectToPeer attempts to connect to a peer.
func (b *BootstrapOrchestrator) connectToPeer(ctx context.Context, peerInfo peer.AddrInfo) error {
	timeout := b.config.PeerConnectTimeout
	if timeout <= 0 {
		timeout = DefaultPeerConnectTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return b.networkNode.Host().Connect(ctx, peerInfo)
}

// GetStatus returns the current bootstrap status.
func (b *BootstrapOrchestrator) GetStatus() *BootstrapStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()

	status := &BootstrapStatus{
		IPFSBootstrapComplete: b.ipfsBootstrapComplete.Load(),
		IPFSRoutingTableSize:  b.getIPFSRoutingTableSize(),

		BabylonBootstrapComplete: b.babylonBootstrapComplete.Load(),
		BabylonPeersStored:       b.getBabylonStoredPeers(),
		BabylonPeersConnected:    b.getBabylonConnectedPeers(),
		BabylonBootstrapDeferred: b.babylonBootstrapDeferred.Load(),

		RendezvousActive: b.rendezvousActive.Load(),

		TotalConnectedPeers: len(b.networkNode.Host().Network().Peers()),

		StartTime:        b.startTime,
		LastAttempt:      b.lastAttempt,
		ConsecutiveFails: b.consecutiveFails,
	}

	return status
}

// IsComplete returns true if both IPFS and Babylon bootstrap are complete.
func (b *BootstrapOrchestrator) IsComplete() bool {
	return b.ipfsBootstrapComplete.Load() && b.babylonBootstrapComplete.Load()
}

// IsIPFSComplete returns true if IPFS DHT bootstrap is complete.
func (b *BootstrapOrchestrator) IsIPFSComplete() bool {
	return b.ipfsBootstrapComplete.Load()
}

// IsBabylonComplete returns true if Babylon DHT bootstrap is complete.
func (b *BootstrapOrchestrator) IsBabylonComplete() bool {
	return b.babylonBootstrapComplete.Load()
}

// IsDeferred returns true if Babylon bootstrap is deferred.
func (b *BootstrapOrchestrator) IsDeferred() bool {
	return b.babylonBootstrapDeferred.Load()
}

// IsRendezvousActive returns true if rendezvous discovery is active.
func (b *BootstrapOrchestrator) IsRendezvousActive() bool {
	return b.rendezvousActive.Load()
}

// TriggerBootstrap triggers a bootstrap attempt.
// It can be called to retry bootstrap after failure or deferral.
func (b *BootstrapOrchestrator) TriggerBootstrap() (int, error) {
	if b.ctx.Err() != nil {
		return 0, b.ctx.Err()
	}

	peersDiscovered := 0

	// Try to discover more peers
	if b.routingDiscovery != nil {
		peerChan, err := b.routingDiscovery.FindPeers(b.ctx, "babylon")
		if err != nil {
			return 0, err
		}

		for peerInfo := range peerChan {
			if peerInfo.ID == b.networkNode.Host().ID() {
				continue
			}

			if err := b.connectToPeer(b.ctx, peerInfo); err != nil {
				continue
			}

			peersDiscovered++
		}
	}

	return peersDiscovered, nil
}

// getIPFSRoutingTableSize returns the size of the IPFS DHT routing table.
func (b *BootstrapOrchestrator) getIPFSRoutingTableSize() int {
	if b.ipfsDHT == nil || b.ipfsDHT.RoutingTable() == nil {
		return 0
	}
	return b.ipfsDHT.RoutingTable().Size()
}

// getBabylonStoredPeers returns the number of Babylon peers stored.
func (b *BootstrapOrchestrator) getBabylonStoredPeers() int {
	// This would query the peerstore for Babylon-specific peers
	// For now, return the routing table size as an approximation
	return b.getIPFSRoutingTableSize()
}

// getBabylonConnectedPeers returns the number of Babylon peers connected.
func (b *BootstrapOrchestrator) getBabylonConnectedPeers() int {
	// Count connected peers that support Babylon protocol
	connectedPeers := b.networkNode.Host().Network().Peers()
	count := 0

	for _, pid := range connectedPeers {
		// Check if peer supports Babylon protocol
		protocols, err := b.networkNode.Host().Peerstore().GetProtocols(pid)
		if err != nil {
			continue
		}

		for _, proto := range protocols {
			if proto == ProtocolID {
				count++
				break
			}
		}
	}

	return count
}

// Close shuts down the bootstrap orchestrator.
func (b *BootstrapOrchestrator) Close() error {
	b.cancel()
	return nil
}
