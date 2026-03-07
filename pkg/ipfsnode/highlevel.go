// Package ipfsnode provides high-level convenience methods for common operations.
// This file wraps lower-level methods with simpler APIs for the application layer.
//
// Note: Many methods are defined in other files (info.go, operations.go, discovery.go).
// This file only contains truly new high-level orchestration methods.
package ipfsnode

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// ==================== High-Level Bootstrap Orchestration ====================

// BootstrapAndWait initiates bootstrap and waits for completion up to timeout.
// This is the main method for synchronous bootstrap with detailed results.
//
// Usage:
//   node, _ := NewNode(config)
//   node.Start()  // Starts async bootstrap
//   result, err := node.BootstrapAndWait(60 * time.Second)
//
// Returns:
//   - BootstrapResult with detailed statistics
//   - Error if timeout occurs or bootstrap fails
func (n *Node) BootstrapAndWait(timeout time.Duration) (*BootstrapResult, error) {
	if !n.isStarted {
		return nil, ErrNodeNotStarted
	}

	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	logger.Infow("bootstrap initiated, waiting for completion",
		"timeout", timeout,
		"start_time", startTime)

	// Wait for IPFS bootstrap (transport layer)
	ipfsTicker := time.NewTicker(500 * time.Millisecond)
	defer ipfsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Timeout - return partial result
			result := n.collectBootstrapResult(startTime)
			if !n.ipfsBootstrapComplete.Load() {
				return result, fmt.Errorf("bootstrap timeout: IPFS DHT not ready")
			}
			return result, fmt.Errorf("bootstrap timeout: Babylon DHT not ready")

		case <-ipfsTicker.C:
			if n.ipfsBootstrapComplete.Load() {
				logger.Info("IPFS DHT bootstrap complete")

				// Now wait for Babylon bootstrap
				babylonTicker := time.NewTicker(500 * time.Millisecond)
				for {
					select {
					case <-ctx.Done():
						babylonTicker.Stop()
						result := n.collectBootstrapResult(startTime)
						if !n.babylonBootstrapComplete.Load() {
							return result, fmt.Errorf("bootstrap timeout: Babylon DHT not ready")
						}
						return result, nil

					case <-babylonTicker.C:
						if n.babylonBootstrapComplete.Load() {
							babylonTicker.Stop()
							logger.Info("Babylon DHT bootstrap complete")
							result := n.collectBootstrapResult(startTime)
							return result, nil
						} else if n.babylonBootstrapDeferred.Load() {
							babylonTicker.Stop()
							logger.Info("Babylon DHT bootstrap deferred (lazy mode)")
							result := n.collectBootstrapResult(startTime)
							return result, nil
						}
					}
				}
			}
		}
	}
}

// collectBootstrapResult gathers current bootstrap statistics
func (n *Node) collectBootstrapResult(startTime time.Time) *BootstrapResult {
	result := &BootstrapResult{
		RoutingTableSize: len(n.dht.RoutingTable().ListPeers()),
		Duration:         time.Since(startTime),
		TotalConnected:   len(n.host.Network().Peers()),
	}

	// Count Babylon peers from storage
	if n.config.Storage != nil {
		babylonPeers, err := n.config.Storage.ListPeersBySource("babylon")
		if err == nil {
			result.BabylonPeersAttempted = len(babylonPeers)
			// Count connected Babylon peers
			for _, record := range babylonPeers {
				pid, err := peer.Decode(record.PeerID)
				if err == nil && n.host.Network().Connectedness(pid) == network.Connected {
					result.BabylonPeersConnected++
				}
			}
		}
	}

	return result
}

// IsBootstrapComplete returns true if both IPFS and Babylon bootstrap are complete
func (n *Node) IsBootstrapComplete() bool {
	return n.ipfsBootstrapComplete.Load() &&
		(n.babylonBootstrapComplete.Load() || n.babylonBootstrapDeferred.Load())
}

// ==================== High-Level Connection Methods ====================

// ConnectToPeers connects to multiple peers in parallel
// Returns the number of successful and failed connections
func (n *Node) ConnectToPeers(maddrs []string) (connected int, failed int) {
	if !n.isStarted || len(maddrs) == 0 {
		return 0, 0
	}

	ctx, cancel := context.WithTimeout(n.ctx, 30*time.Second)
	defer cancel()

	var addrInfos []peer.AddrInfo
	for _, maddr := range maddrs {
		addr, err := multiaddr.NewMultiaddr(maddr)
		if err != nil {
			logger.Debugw("invalid multiaddress", "addr", maddr, "error", err)
			failed++
			continue
		}

		// Extract peer ID from multiaddr
		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			logger.Debugw("invalid peer addr", "addr", maddr, "error", err)
			failed++
			continue
		}
		addrInfos = append(addrInfos, *peerInfo)
	}

	if len(addrInfos) == 0 {
		return 0, failed
	}

	connected = n.connectToPeersParallel(ctx, addrInfos)
	return connected, failed
}

// DisconnectFromPeer disconnects from a peer by peer ID
func (n *Node) DisconnectFromPeer(peerID string) error {
	if !n.isStarted {
		return ErrNodeNotStarted
	}

	pid, err := peer.Decode(peerID)
	if err != nil {
		return fmt.Errorf("invalid peer ID: %w", err)
	}

	return n.host.Network().ClosePeer(pid)
}

// ListConnectedPeers returns information about all connected peers
func (n *Node) ListConnectedPeers() []PeerInfo {
	if !n.isStarted {
		return nil
	}

	var peers []PeerInfo
	for _, p := range n.host.Network().Peers() {
		peerInfo := n.host.Peerstore().PeerInfo(p)
		protocols, _ := n.host.Peerstore().GetProtocols(p)

		protoStrings := make([]string, len(protocols))
		for i, proto := range protocols {
			protoStrings[i] = string(proto)
		}

		addrStrings := make([]string, len(peerInfo.Addrs))
		for i, addr := range peerInfo.Addrs {
			addrStrings[i] = addr.String()
		}

		peers = append(peers, PeerInfo{
			ID:        p.String(),
			Addresses: addrStrings,
			Protocols: protoStrings,
			Connected: true,
		})
	}

	return peers
}
