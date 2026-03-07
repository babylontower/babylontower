// Package ipfsnode provides an embedded IPFS node for decentralized communication.
// It wraps libp2p and IPFS components to provide a simple interface for the messaging layer.
//
// This package is organized into the following components:
//   - types.go: Core types, interfaces, constants, and errors
//   - lifecycle.go: Node creation, Start(), Stop(), and initialization
//   - operations.go: Core IPFS operations (Add, Get, ConnectToPeer, FindPeer)
//   - discovery.go: Peer discovery, mDNS, DHT maintenance
//   - bootstrap.go: DHT bootstrap, peer connection, verification
//   - peer_tracking.go: Peer scoring, connection health, peer pruning
//   - info.go: Network info, metrics, DHT info
//   - pubsub.go: PubSub subscriptions and publishing
//   - metrics.go: Metrics collection
package ipfsnode

import (
	"github.com/ipfs/go-log/v2"
)

// Logger for the ipfsnode package
var logger = log.Logger("babylontower/ipfsnode")
