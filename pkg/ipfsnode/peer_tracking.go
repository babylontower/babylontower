package ipfsnode

import (
	"context"
	"time"

	bterrors "babylontower/pkg/errors"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// PeerScore tracks connection quality for a peer
type PeerScore struct {
	PeerID           string
	ConnectCount     int
	DisconnectCount  int
	FailCount        int
	SuccessCount     int
	LastConnected    time.Time
	LastDisconnected time.Time
	LatencyMs        int64
	Score            float64 // Computed score (0.0 - 1.0)
}

// computeScore calculates the peer's score based on connection history
func (p *PeerScore) computeScore() float64 {
	total := p.ConnectCount + p.FailCount
	if total == 0 {
		return 0.5 // Default score for new peers
	}

	// Base score is success rate
	successRate := float64(p.SuccessCount) / float64(total)

	// Penalty for frequent disconnections
	disconnectPenalty := 0.0
	if p.DisconnectCount > 5 {
		disconnectPenalty = 0.1 * float64(p.DisconnectCount-5)
		if disconnectPenalty > 0.3 {
			disconnectPenalty = 0.3
		}
	}

	// Bonus for recent connections
	recencyBonus := 0.0
	if !p.LastConnected.IsZero() {
		timeSinceConnect := time.Since(p.LastConnected)
		if timeSinceConnect < time.Hour {
			recencyBonus = 0.1
		} else if timeSinceConnect < 24*time.Hour {
			recencyBonus = 0.05
		}
	}

	score := successRate - disconnectPenalty + recencyBonus
	if score < 0.0 {
		score = 0.0
	}
	if score > 1.0 {
		score = 1.0
	}

	p.Score = score
	return score
}

// updatePeerScore updates the score for a peer based on connection events
func (n *Node) updatePeerScore(peerID string, connected, failed bool) {
	n.peerScoreMu.Lock()
	defer n.peerScoreMu.Unlock()

	score, exists := n.peerScores[peerID]
	if !exists {
		score = &PeerScore{
			PeerID: peerID,
		}
		n.peerScores[peerID] = score
	}

	if connected && !failed {
		score.ConnectCount++
		score.SuccessCount++
		score.LastConnected = time.Now()
	} else if failed {
		score.FailCount++
	} else {
		score.DisconnectCount++
		score.LastDisconnected = time.Now()
	}

	score.computeScore()

	logger.Debugw("peer score updated",
		"peer", peerID,
		"score", score.Score,
		"success", score.SuccessCount,
		"fail", score.FailCount,
		"disconnect", score.DisconnectCount)
}

// getPeerScore returns the score for a peer
func (n *Node) getPeerScore(peerID string) float64 {
	n.peerScoreMu.RLock()
	defer n.peerScoreMu.RUnlock()

	if score, exists := n.peerScores[peerID]; exists {
		return score.Score
	}
	return 0.5 // Default score for unknown peers
}

// getLowScorePeers returns peers with scores below threshold for pruning
func (n *Node) getLowScorePeers(threshold float64) []string {
	n.peerScoreMu.RLock()
	defer n.peerScoreMu.RUnlock()

	var lowScorePeers []string
	for peerID, score := range n.peerScores {
		if score.Score < threshold {
			lowScorePeers = append(lowScorePeers, peerID)
		}
	}

	return lowScorePeers
}

// subscribePeerEvents subscribes to libp2p peer connection events for tracking
func (n *Node) subscribePeerEvents() {
	// Subscribe to peer connectedness changes (covers both connect and disconnect)
	sub, err := n.host.EventBus().Subscribe(new(event.EvtPeerConnectednessChanged))
	if err != nil {
		logger.Warnw("failed to subscribe to peer connectedness events", "error", err)
		return
	}
	n.peerEventSub = sub

	bterrors.SafeGo("ipfs-peer-events", func() {
		for {
			select {
			case <-n.ctx.Done():
				return
			case e := <-sub.Out():
				if evt, ok := e.(event.EvtPeerConnectednessChanged); ok {
					peerID := evt.Peer.String()

					// Track connected/disconnected state
					switch evt.Connectedness {
					case network.Connected:
						n.peerCount.Add(1)
						n.updatePeerScore(peerID, true, false)
						n.metrics.RecordConnection(peerID, 0) // Latency can be added later
						logger.Debugw("peer connected",
							"peer", peerID,
							"total_peers", n.peerCount.Load())
					case network.NotConnected:
						n.peerCount.Add(-1)
						n.updatePeerScore(peerID, false, false)
						n.metrics.RecordDisconnection(peerID)
						logger.Debugw("peer disconnected",
							"peer", peerID,
							"total_peers", n.peerCount.Load())
					}
				}
			}
		}
	})
}

// performHealthCheck checks connection health and reconnects if needed
// Includes connection pruning for low-quality peers (Task 2.3.4)
func (n *Node) performHealthCheck() {
	n.lastHealthCheck = time.Now()

	// Get current connected peers
	connectedPeers := n.host.Network().Peers()
	peerCount := len(connectedPeers)

	logger.Debugw("connection health check",
		"connected_peers", peerCount,
		"tracked_peers", len(n.peerScores))

	// Check if we have enough connections
	if peerCount < 5 {
		logger.Debugw("low peer count, attempting reconnection to stored peers")
		n.reconnectToLowScorePeers()
	}

	// Prune low-quality peers (Task 2.3.4)
	// Pruning thresholds based on peer count:
	// - >100 peers: aggressive pruning (score < 0.3)
	// - >50 peers: moderate pruning (score < 0.25)
	// - >20 peers: light pruning (score < 0.2)
	var pruneThreshold float64
	var maxPruneCount int

	switch {
	case peerCount > 100:
		pruneThreshold = 0.3
		maxPruneCount = 20
	case peerCount > 50:
		pruneThreshold = 0.25
		maxPruneCount = 10
	case peerCount > 20:
		pruneThreshold = 0.2
		maxPruneCount = 5
	default:
		// Don't prune if we have few connections
		return
	}

	lowScorePeers := n.getLowScorePeers(pruneThreshold)
	if len(lowScorePeers) > 0 {
		logger.Debugw("pruning low-quality peers",
			"count", min(maxPruneCount, len(lowScorePeers)),
			"threshold", pruneThreshold,
			"total_low_score", len(lowScorePeers))

		pruned := 0
		for _, peerID := range lowScorePeers {
			if pruned >= maxPruneCount {
				break
			}

			pid, err := peer.Decode(peerID)
			if err != nil {
				continue
			}

			// Skip if already disconnected
			if n.host.Network().Connectedness(pid) != network.Connected {
				continue
			}

			score := n.getPeerScore(peerID)
			logger.Debugw("pruning low-score peer", "peer", peerID, "score", score)
			if err := n.host.Network().ClosePeer(pid); err != nil {
				logger.Debugw("failed to close peer", "peer", peerID, "error", err)
			}
			pruned++

			// Record disconnection in metrics
			n.metrics.RecordDisconnection(peerID)
		}

		logger.Debugw("pruning complete", "pruned", pruned)
	}
}

// reconnectToLowScorePeers attempts to reconnect to peers with low scores
func (n *Node) reconnectToLowScorePeers() {
	// Get peers with scores below 0.4
	lowScorePeers := n.getLowScorePeers(0.4)

	if len(lowScorePeers) == 0 {
		logger.Debug("no low-score peers to reconnect to")
		return
	}

	logger.Debugw("attempting reconnection to low-score peers", "count", len(lowScorePeers))

	// Try to reconnect to up to 5 peers
	for i, peerID := range lowScorePeers {
		if i >= 5 {
			break
		}

		pid, err := peer.Decode(peerID)
		if err != nil {
			logger.Debugw("invalid peer ID", "peer", peerID, "error", err)
			continue
		}

		// Skip if already connected
		if n.host.Network().Connectedness(pid) == network.Connected {
			continue
		}

		// Get peer info from peerstore
		peerInfo := n.host.Peerstore().PeerInfo(pid)
		if len(peerInfo.Addrs) == 0 {
			logger.Debugw("no addresses for peer", "peer", peerID)
			continue
		}

		ctx, cancel := context.WithTimeout(n.ctx, DialTimeout)
		err = n.host.Connect(ctx, peerInfo)
		cancel()

		if err != nil {
			logger.Debugw("reconnection failed", "peer", peerID, "error", err)
			n.updatePeerScore(peerID, false, true)
		} else {
			logger.Debugw("reconnected to peer", "peer", peerID)
		}
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
