package ipfsnode

import (
	"context"
	"fmt"
	"time"

	"babylontower/pkg/storage"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// PeerID returns the node's peer ID
func (n *Node) PeerID() string {
	if n.host == nil {
		return ""
	}
	return n.host.ID().String()
}

// Multiaddrs returns the node's multiaddresses
func (n *Node) Multiaddrs() []string {
	if n.host == nil {
		return nil
	}
	addrs := make([]string, len(n.host.Addrs()))
	for i, addr := range n.host.Addrs() {
		addrs[i] = addr.String()
	}
	return addrs
}

// Host returns the underlying libp2p host
func (n *Node) Host() host.Host {
	return n.host
}

// PubSub returns the PubSub instance
func (n *Node) PubSub() *pubsub.PubSub {
	return n.pubsub
}

// DHT returns the DHT instance for peer routing and discovery
func (n *Node) DHT() *dht.IpfsDHT {
	return n.dht
}

// Context returns the node's context
func (n *Node) Context() context.Context {
	return n.ctx
}

// IsStarted returns true if the node is running
func (n *Node) IsStarted() bool {
	return n.isStarted.Load()
}

// IsIPFSBootstrapComplete returns true if IPFS DHT bootstrap is complete
// IPFS DHT is the transport layer - used for PubSub connectivity
func (n *Node) IsIPFSBootstrapComplete() bool {
	return n.ipfsBootstrapComplete.Load()
}

// IsBabylonBootstrapComplete returns true if Babylon DHT bootstrap is complete
// Babylon DHT is the protocol layer - used for messaging and groups
func (n *Node) IsBabylonBootstrapComplete() bool {
	return n.babylonBootstrapComplete.Load()
}

// IsBabylonBootstrapDeferred returns true if Babylon bootstrap is deferred
// This means bootstrap is waiting to be triggered by incoming messages
func (n *Node) IsBabylonBootstrapDeferred() bool {
	return n.babylonBootstrapDeferred.Load()
}

// IsBabylonDHTReady returns true if Babylon DHT is initialized and ready
// This is used by the identity layer to check if protocol operations can proceed
func (n *Node) IsBabylonDHTReady() bool {
	return n.babylonDHT != nil && (n.babylonBootstrapComplete.Load() || n.babylonBootstrapDeferred.Load())
}

// closeBabylonBootstrapDone safely closes the babylonBootstrapDone channel (idempotent).
func (n *Node) closeBabylonBootstrapDone() {
	select {
	case <-n.babylonBootstrapDone:
		// already closed
	default:
		close(n.babylonBootstrapDone)
	}
}

// WaitForBabylonDHT waits for Babylon DHT to be ready for protocol operations.
// Returns nil if Babylon DHT is ready (either bootstrap complete or deferred for lazy triggering).
// Returns context.DeadlineExceeded if timeout expires before DHT is ready.
func (n *Node) WaitForBabylonDHT(timeout time.Duration) error {
	// Fast path: already ready
	if n.babylonDHT != nil && (n.babylonBootstrapComplete.Load() || n.babylonBootstrapDeferred.Load()) {
		return nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-n.babylonBootstrapDone:
		if n.babylonBootstrapComplete.Load() {
			logger.Debug("Babylon DHT bootstrap complete")
		} else if n.babylonBootstrapDeferred.Load() {
			logger.Debug("Babylon DHT bootstrap deferred (lazy triggering enabled)")
		}
		return nil
	case <-timer.C:
		return fmt.Errorf("Babylon DHT not ready after %v (bootstrap neither complete nor deferred)", timeout)
	case <-n.ctx.Done():
		return n.ctx.Err()
	}
}

// IsRendezvousActive returns true if the node is advertising on the rendezvous namespace
func (n *Node) IsRendezvousActive() bool {
	return n.rendezvousActive.Load()
}

// GetNetworkInfo returns network status information
func (n *Node) GetNetworkInfo() *NetworkInfo {
	info := &NetworkInfo{
		PeerID:             n.PeerID(),
		IsStarted:          n.isStarted.Load(),
		ConnectedPeerCount: 0,
	}

	if n.host != nil {
		info.Multiaddrs = n.Multiaddrs()
		info.ConnectedPeers = n.ConnectedPeers()
		info.ConnectedPeerCount = len(info.ConnectedPeers)
		info.ListenAddrs = make([]string, len(n.host.Addrs()))
		for i, addr := range n.host.Addrs() {
			info.ListenAddrs[i] = addr.String()
		}
	}

	return info
}

// GetMetrics returns network health metrics
func (n *Node) GetMetrics() *NetworkMetrics {
	if n.metrics == nil {
		return &NetworkMetrics{}
	}
	return &n.metrics.metrics
}

// GetMetricsFull returns comprehensive metrics including discovery and connection history
func (n *Node) GetMetricsFull() *MetricsFull {
	if n.metrics == nil {
		return &MetricsFull{}
	}

	n.peerMu.RLock()
	lastPeerFound := n.lastPeerFound
	mdnsCount := n.mdnsCount.Load()
	n.peerMu.RUnlock()

	metrics := n.metrics.GetMetrics()
	metrics.PeerID = n.PeerID()

	return &MetricsFull{
		NetworkMetrics: metrics,
		DHTInfo:        n.GetDHTInfo(),
		MDnsStats: MDnsStats{
			TotalDiscoveries: mdnsCount,
			LastPeerFound:    lastPeerFound,
		},
		DiscoveryBySource:     n.metrics.GetDiscoveryBySource(),
		ConnectionSuccessRate: n.metrics.GetConnectionSuccessRate(),
		MessageSuccessRate:    n.metrics.GetMessageSuccessRate(),
	}
}

// GetDHTInfo returns detailed information about the DHT state
func (n *Node) GetDHTInfo() *DHTInfo {
	info := &DHTInfo{
		IsStarted: n.isStarted.Load(),
	}

	if !n.isStarted.Load() || n.dht == nil {
		return info
	}

	// Get routing table peers
	routingTable := n.dht.RoutingTable()
	peers := routingTable.ListPeers()

	info.RoutingTableSize = len(peers)
	info.RoutingTablePeers = make([]string, 0, len(peers))
	for _, p := range peers {
		info.RoutingTablePeers = append(info.RoutingTablePeers, p.String())
	}

	// Get connection count
	info.ConnectedPeerCount = len(n.host.Network().Peers())

	// Check if we're connected to bootstrap peers
	info.HasBootstrapConnection = info.ConnectedPeerCount > 0

	// Get DHT mode (simplified - just show if it's auto mode)
	info.Mode = "auto"

	return info
}

// GetMDnsStats returns mDNS discovery statistics
func (n *Node) GetMDnsStats() MDnsStats {
	n.peerMu.RLock()
	lastPeerFound := n.lastPeerFound
	n.peerMu.RUnlock()

	return MDnsStats{
		TotalDiscoveries: n.mdnsCount.Load(),
		LastPeerFound:    lastPeerFound,
	}
}

// GetBabylonDHTInfo returns information about Babylon DHT peers
// This shows peers discovered via the Babylon bootstrap mechanism
func (n *Node) GetBabylonDHTInfo() *BabylonDHTInfo {
	// Handle nil config or storage (during early startup)
	if n.config == nil || n.config.Storage == nil {
		return &BabylonDHTInfo{
			StoredBabylonPeers:    0,
			ConnectedBabylonPeers: 0,
			BabylonPeerIDs:        []string{},
			RendezvousActive:      n.rendezvousActive.Load(),
		}
	}

	babylonPeers, _ := n.config.Storage.ListPeersBySource(storage.SourceBabylon)

	var connectedBabylon int
	var babylonPeerIDs []string
	for _, record := range babylonPeers {
		pid, err := peer.Decode(record.PeerID)
		if err != nil {
			continue
		}
		if n.host.Network().Connectedness(pid) == network.Connected {
			connectedBabylon++
		}
		babylonPeerIDs = append(babylonPeerIDs, record.PeerID)
	}

	return &BabylonDHTInfo{
		StoredBabylonPeers:    len(babylonPeers),
		ConnectedBabylonPeers: connectedBabylon,
		BabylonPeerIDs:        babylonPeerIDs,
		RendezvousActive:      n.rendezvousActive.Load(),
	}
}

// GetPeerCountsBySource returns peer counts grouped by discovery source
func (n *Node) GetPeerCountsBySource() *PeerCountsBySource {
	counts := &PeerCountsBySource{}

	// Handle nil config or storage (during early startup)
	if n.config == nil || n.config.Storage == nil {
		counts.ConnectedTotal = len(n.host.Network().Peers())
		return counts
	}

	if sources, err := n.config.Storage.ListPeersBySource(storage.SourceBabylon); err == nil {
		counts.Babylon = len(sources)
	}
	if sources, err := n.config.Storage.ListPeersBySource(storage.SourceBootstrap); err == nil {
		counts.IPFSBootstrap = len(sources)
	}
	if sources, err := n.config.Storage.ListPeersBySource(storage.SourceDHT); err == nil {
		counts.IPFSDiscovery = len(sources)
	}
	if sources, err := n.config.Storage.ListPeersBySource(storage.SourceMDNS); err == nil {
		counts.MDNS = len(sources)
	}

	counts.ConnectedTotal = len(n.host.Network().Peers())

	return counts
}

// GetBootstrapStatus returns detailed bootstrap status for the decoupled architecture.
// This shows the status of both IPFS DHT (transport) and Babylon DHT (protocol).
func (n *Node) GetBootstrapStatus() *BootstrapStatus {
	status := &BootstrapStatus{
		// IPFS DHT (Transport Layer) Status
		IPFSBootstrapComplete: n.ipfsBootstrapComplete.Load(),
		IPFSRoutingTableSize:  0,

		// Babylon DHT (Protocol Layer) Status
		BabylonBootstrapComplete: n.babylonBootstrapComplete.Load(),
		BabylonPeersStored:       0,
		BabylonPeersConnected:    0,
		BabylonBootstrapDeferred: n.babylonBootstrapDeferred.Load(),

		// Rendezvous Discovery Status
		RendezvousActive: n.rendezvousActive.Load(),

		// Connection Summary
		TotalConnectedPeers: len(n.host.Network().Peers()),
	}

	// Get IPFS DHT routing table size
	if n.dht != nil {
		status.IPFSRoutingTableSize = len(n.dht.RoutingTable().ListPeers())
	}

	// Get Babylon peer counts
	if n.config.Storage != nil {
		babylonPeers, _ := n.config.Storage.ListPeersBySource(storage.SourceBabylon)
		status.BabylonPeersStored = len(babylonPeers)

		for _, record := range babylonPeers {
			pid, err := peer.Decode(record.PeerID)
			if err != nil {
				continue
			}
			if n.host.Network().Connectedness(pid) == network.Connected {
				status.BabylonPeersConnected++
			}
		}
	}

	return status
}

// TriggerRendezvousDiscovery triggers DHT rendezvous discovery for Babylon peers
// Returns the number of peers discovered
func (n *Node) TriggerRendezvousDiscovery() int {
	if n.discovery == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(n.ctx, 30*time.Second)
	defer cancel()
	peers := n.discoverBabylonPeers(ctx)
	return len(peers)
}

// WaitForIPFSBootstrap waits for IPFS DHT bootstrap to complete
func (n *Node) WaitForIPFSBootstrap(timeout time.Duration) error {
	// Fast path
	if n.ipfsBootstrapComplete.Load() {
		return nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-n.ipfsBootstrapDone:
		return nil
	case <-timer.C:
		return context.DeadlineExceeded
	case <-n.ctx.Done():
		return n.ctx.Err()
	}
}

// WaitForBabylonBootstrap waits for Babylon DHT bootstrap to complete
func (n *Node) WaitForBabylonBootstrap(timeout time.Duration) error {
	// Fast path
	if n.babylonBootstrapComplete.Load() || n.babylonBootstrapDeferred.Load() {
		return nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-n.babylonBootstrapDone:
		return nil
	case <-timer.C:
		return context.DeadlineExceeded
	case <-n.ctx.Done():
		return n.ctx.Err()
	}
}
