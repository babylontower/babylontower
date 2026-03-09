package ipfsnode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multihash"
)

// Add adds data to IPFS and returns the CID
// The data is stored in the node's blockstore
func (n *Node) Add(data []byte) (string, error) {
	if !n.isStarted.Load() {
		return "", ErrNodeNotStarted
	}

	// Create a CID from the data
	// Using SHA2-256 hash function
	mh, err := multihash.Sum(data, multihash.SHA2_256, -1)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrAddFailed, err)
	}

	c := cid.NewCidV1(cid.Raw, mh)
	cidStr := c.String()

	// Provide the CID to the DHT
	err = n.dht.Provide(n.ctx, c, true)
	if err != nil {
		// DHT provide often fails when node is starting or has few peers - log at debug level
		logger.Debugw("DHT provide failed (normal during startup)", "cid", cidStr, "error", err)
	}

	logger.Debugw("data added to IPFS", "cid", cidStr, "size", len(data))

	return cidStr, nil
}

// Get retrieves data from IPFS by CID
// Returns the raw bytes if successful
func (n *Node) Get(cidStr string) ([]byte, error) {
	if !n.isStarted.Load() {
		return nil, ErrNodeNotStarted
	}

	// Parse CID
	_, err := cid.Parse(cidStr)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid CID: %v", ErrGetFailed, err)
	}

	// For PoC, we don't have a full blockstore
	// In a real implementation, this would fetch from the network
	// For now, return an error indicating the limitation
	logger.Warnw("IPFS Get not fully implemented for PoC", "cid", cidStr)

	return nil, fmt.Errorf("%w: direct block fetch not implemented in PoC", ErrGetFailed)
}

// ConnectToPeer connects to a peer by multiaddr
func (n *Node) ConnectToPeer(maddr string) error {
	if !n.isStarted.Load() {
		return ErrNodeNotStarted
	}

	ma, err := multiaddr.NewMultiaddr(maddr)
	if err != nil {
		return fmt.Errorf("invalid multiaddr: %w", err)
	}

	peerInfo, err := peer.AddrInfoFromP2pAddr(ma)
	if err != nil {
		return fmt.Errorf("failed to parse peer addr: %w", err)
	}

	ctx, cancel := context.WithTimeout(n.ctx, ConnectionTimeout)
	defer cancel()

	if err := n.host.Connect(ctx, *peerInfo); err != nil {
		return fmt.Errorf("failed to connect to peer: %w", err)
	}

	logger.Infow("connected to peer", "peer", peerInfo.ID)

	// Save peer as Babylon peer for later reconnection
	// This ensures manually connected peers are counted as Babylon peers
	n.savePeerForLater(peerInfo.ID)

	// Trigger Babylon DHT bootstrap if not yet complete
	if !n.babylonBootstrapComplete.Load() {
		go func() {
			if err := n.TriggerLazyBootstrap(); err != nil {
				logger.Debugw("lazy bootstrap after /connect failed", "error", err)
			}
		}()
	}

	return nil
}

// FindPeer queries the DHT to find a peer by PeerID and returns their address info.
// It first tries direct FindPeer, then falls back to GetClosestPeers which returns
// peers closest to the target in DHT space (useful when target isn't directly advertised).
func (n *Node) FindPeer(peerID string) (*peer.AddrInfo, error) {
	if !n.isStarted.Load() {
		return nil, ErrNodeNotStarted
	}

	parsedID, err := peer.Decode(peerID)
	if err != nil {
		return nil, fmt.Errorf("invalid peer ID: %w", err)
	}

	ctx, cancel := context.WithTimeout(n.ctx, ConnectionTimeout)
	defer cancel()

	// Try direct FindPeer first (requires peer to have advertised themselves)
	peerInfo, err := n.dht.FindPeer(ctx, parsedID)
	if err == nil && len(peerInfo.Addrs) > 0 {
		logger.Debugw("found peer via DHT FindPeer", "peer", peerInfo.ID, "addrs", peerInfo.Addrs)
		return &peerInfo, nil
	}

	// Fallback: GetClosestPeers returns peers closest to target in DHT space
	// This is useful when the target peer hasn't explicitly advertised but
	// is reachable through the routing table
	logger.Debugw("FindPeer failed, trying GetClosestPeers fallback", "peer", parsedID, "error", err)

	closestPeers, err := n.dht.GetClosestPeers(ctx, string(parsedID))
	if err != nil {
		return nil, fmt.Errorf("DHT FindPeer and GetClosestPeers failed: %w", err)
	}

	if len(closestPeers) == 0 {
		return nil, errors.New("peer not found in DHT routing table")
	}

	// Return the closest peers - the first one is typically the best match
	// Note: These may not be the exact target, but peers near them in DHT space
	logger.Debugw("found closest peers via DHT", "target", parsedID, "count", len(closestPeers))

	// Get address info for the closest peer
	closestInfo := n.host.Peerstore().PeerInfo(closestPeers[0])
	if len(closestInfo.Addrs) > 0 {
		return &closestInfo, nil
	}

	return nil, errors.New("closest peer found but no addresses available")
}

// WaitForDHT blocks until the DHT routing table is populated with at least one peer.
// This should be called after Start() before attempting DHT operations.
// Returns an error if the timeout expires before any peers are found.
func (n *Node) WaitForDHT(timeout time.Duration) error {
	if !n.isStarted.Load() {
		return ErrNodeNotStarted
	}

	// Fast path: routing table already populated
	if len(n.dht.RoutingTable().ListPeers()) > 0 {
		return nil
	}

	logger.Debugw("waiting for DHT bootstrap...", "timeout", timeout)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	// Wait for IPFS bootstrap to complete first (event-driven),
	// then check routing table
	select {
	case <-n.ipfsBootstrapDone:
		// Bootstrap finished — check routing table
		routingTableSize := len(n.dht.RoutingTable().ListPeers())
		if routingTableSize > 0 {
			logger.Debugw("DHT bootstrap complete", "routing_table_size", routingTableSize)
			return nil
		}
		return fmt.Errorf("DHT bootstrap completed but routing table empty (0 peers)")
	case <-timer.C:
		routingTableSize := len(n.dht.RoutingTable().ListPeers())
		return fmt.Errorf("DHT bootstrap timeout after %s (routing table has %d peers)", timeout, routingTableSize)
	case <-n.ctx.Done():
		return n.ctx.Err()
	}
}

// PutToDHT stores a value in the DHT with the given key and TTL
// This implements the DHTClient interface for identity document publication
// For Babylon protocol keys (/bt/id/, /bt/prekeys/), uses Babylon DHT
// For other keys, uses the default IPFS DHT
func (n *Node) PutToDHT(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if !n.isStarted.Load() {
		return ErrNodeNotStarted
	}

	// Determine which DHT to use based on key namespace
	useBabylonDHT := n.shouldUseBabylonDHT(key)

	// Select appropriate DHT
	dhtToUse := n.dht
	if useBabylonDHT {
		if n.babylonDHT == nil {
			return errors.New("Babylon DHT not initialized - cannot publish Babylon protocol data")
		}
		// Check if Babylon DHT is ready (bootstrap complete or deferred for lazy triggering)
		if !n.IsBabylonDHTReady() {
			logger.Warnw("Babylon DHT not ready, publication may fail or be delayed",
				"key", key,
				"bootstrap_complete", n.babylonBootstrapComplete.Load(),
				"bootstrap_deferred", n.babylonBootstrapDeferred.Load(),
				"routing_table_size", len(n.babylonDHT.RoutingTable().ListPeers()),
				"connected_peers", len(n.host.Network().Peers()))
			// Continue anyway - the put may still succeed if we have any peers
		}
		dhtToUse = n.babylonDHT
	}

	if dhtToUse == nil {
		return errors.New("DHT not initialized")
	}

	// Log which DHT namespace we're publishing to
	namespace := "unknown"
	if len(key) > 0 {
		parts := strings.SplitN(key, "/", 3)
		if len(parts) >= 3 {
			namespace = "/" + parts[1] + "/" + parts[2]
		}
	}

	logger.Infow("putting value to DHT",
		"key", key,
		"namespace", namespace,
		"dht_type", map[bool]string{true: "babylon", false: "ipfs"}[useBabylonDHT],
		"size", len(value),
		"ttl", ttl,
		"routing_table_size", len(dhtToUse.RoutingTable().ListPeers()),
		"connected_peers", len(n.host.Network().Peers()))

	// Store the value in the DHT using PutValue
	// The TTL is handled internally by the DHT (default expiration)
	err := dhtToUse.PutValue(ctx, key, value)
	if err != nil {
		logger.Errorw("DHT PutValue failed",
			"key", key,
			"namespace", namespace,
			"dht_type", map[bool]string{true: "babylon", false: "ipfs"}[useBabylonDHT],
			"error", err)
		return fmt.Errorf("failed to put value to DHT: %w", err)
	}

	logger.Infow("stored value in DHT successfully",
		"key", key,
		"namespace", namespace,
		"dht_type", map[bool]string{true: "babylon", false: "ipfs"}[useBabylonDHT],
		"size", len(value),
		"ttl", ttl)
	return nil
}

// shouldUseBabylonDHT determines if a key should use the Babylon DHT
// Babylon protocol keys: /bt/id/, /bt/prekeys/, /bt/username/, etc.
func (n *Node) shouldUseBabylonDHT(key string) bool {
	// Babylon protocol namespaces that require the Babylon DHT
	babylonNamespaces := []string{
		"/bt/id/",
		"/bt/prekeys/",
		"/bt/username/",
		"/bt/pubgroup/",
		"/bt/chanhead/",
		"/bt/mailbox/",
		"/bt/rep/",
	}

	for _, ns := range babylonNamespaces {
		if strings.HasPrefix(key, ns) {
			return true
		}
	}
	return false
}

// GetFromDHT retrieves a value from the DHT by key
// This implements the DHTClient interface for identity document retrieval
// For Babylon protocol keys (/bt/id/, /bt/prekeys/), uses Babylon DHT
// For other keys, uses the default IPFS DHT
func (n *Node) GetFromDHT(ctx context.Context, key string) ([]byte, error) {
	if !n.isStarted.Load() {
		return nil, ErrNodeNotStarted
	}

	// Determine which DHT to use based on key namespace
	useBabylonDHT := n.shouldUseBabylonDHT(key)
	
	// Select appropriate DHT
	dhtToUse := n.dht
	if useBabylonDHT {
		if n.babylonDHT == nil {
			return nil, errors.New("Babylon DHT not initialized - cannot fetch Babylon protocol data")
		}
		dhtToUse = n.babylonDHT
	}

	if dhtToUse == nil {
		return nil, errors.New("DHT not initialized")
	}

	// Retrieve the value from the DHT
	value, err := dhtToUse.GetValue(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get value from DHT: %w", err)
	}

	logger.Debugw("retrieved value from DHT",
		"key", key,
		"dht_type", map[bool]string{true: "babylon", false: "ipfs"}[useBabylonDHT],
		"size", len(value))
	return value, nil
}

// GetClosestPeers finds peers closest to a given key in the DHT
// This implements the DHTClient interface
func (n *Node) GetClosestPeers(ctx context.Context, key string) ([]string, error) {
	if !n.isStarted.Load() {
		return nil, ErrNodeNotStarted
	}
	if n.dht == nil {
		return nil, errors.New("DHT not initialized")
	}

	// Query the DHT for closest peers
	peers, err := n.dht.GetClosestPeers(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get closest peers: %w", err)
	}

	// Convert to string slice
	peerStrings := make([]string, len(peers))
	for i, p := range peers {
		peerStrings[i] = p.String()
	}

	logger.Debugw("found closest peers", "key", key, "count", len(peers))
	return peerStrings, nil
}

// ConnectedPeers returns information about currently connected peers
func (n *Node) ConnectedPeers() []PeerInfo {
	if n.host == nil {
		return nil
	}

	peers := n.host.Network().Peers()
	peerInfos := make([]PeerInfo, 0, len(peers))

	for _, p := range peers {
		info := PeerInfo{
			ID:        p.String(),
			Connected: true,
		}

		// Get peer addresses
		conn := n.host.Network().ConnsToPeer(p)
		if len(conn) > 0 {
			info.Addresses = make([]string, len(conn))
			for i, c := range conn {
				info.Addresses[i] = c.RemoteMultiaddr().String()
			}
		}

		// Get peer protocols
		protocols, err := n.host.Peerstore().GetProtocols(p)
		if err == nil && len(protocols) > 0 {
			info.Protocols = make([]string, len(protocols))
			for i, proto := range protocols {
				info.Protocols[i] = string(proto)
			}
		}

		peerInfos = append(peerInfos, info)
	}

	return peerInfos
}
