// Package ipfsnode provides an embedded IPFS node for decentralized communication.
// It wraps libp2p and IPFS components to provide a simple interface for the messaging layer.
package ipfsnode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"babylontower/pkg/storage"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

const (
	// DefaultRepoDir is the default directory for IPFS repo
	DefaultRepoDir = "~/.babylontower/ipfs"
	// DefaultProtocolID is the protocol ID for Babylon Tower
	DefaultProtocolID = "/babylontower/1.0.0"
	// ConnectionTimeout is the timeout for peer connections
	ConnectionTimeout = 30 * time.Second
	// DialTimeout is the timeout for individual dial attempts
	DialTimeout = 20 * time.Second // Increased from 15s for better NAT traversal
	// DHTBootstrapTimeout is the timeout for DHT bootstrap
	DHTBootstrapTimeout = 60 * time.Second
	// DHTRefreshInterval is how often to refresh the DHT routing table
	DHTRefreshInterval = 2 * time.Minute
	// MDnsAnnounceInterval is how often mDNS announces our presence
	MDnsAnnounceInterval = 5 * time.Second
	// DialBackoffDuration is how long to wait before retrying a failed peer
	DialBackoffDuration = 2 * time.Minute // Reduced from default 5min for faster recovery
	// DefaultMinBabylonPeersRequired is the minimum number of Babylon peers needed for bootstrap
	DefaultMinBabylonPeersRequired = 1
	// DefaultStoredPeerTimeoutSecs is the maximum age of stored peers in seconds (24 hours)
	DefaultStoredPeerTimeoutSecs = 86400
	// BabylonRendezvousNS is the DHT rendezvous namespace for discovering Babylon nodes
	BabylonRendezvousNS = "babylon/rendezvous/v1"
	// RendezvousAdvertiseInterval is how often to re-advertise on the rendezvous namespace
	RendezvousAdvertiseInterval = 4 * time.Hour
	// RendezvousDiscoveryInterval is how often to search for new Babylon peers via rendezvous
	RendezvousDiscoveryInterval = 30 * time.Second
	// RendezvousDiscoverySlowInterval is the interval after Babylon bootstrap completes
	RendezvousDiscoverySlowInterval = 5 * time.Minute
)

var (
	// ErrNodeNotStarted is returned when operations are attempted on a stopped node
	ErrNodeNotStarted = errors.New("IPFS node not started")
	// ErrAddFailed is returned when adding data to IPFS fails
	ErrAddFailed = errors.New("failed to add data to IPFS")
	// ErrGetFailed is returned when getting data from IPFS fails
	ErrGetFailed = errors.New("failed to get data from IPFS")
	// ErrBabylonBootstrapDeferred is returned when Babylon bootstrap is deferred
	ErrBabylonBootstrapDeferred = errors.New("Babylon bootstrap deferred - waiting for messages")
)

// Config holds configuration for the IPFS node
type Config struct {
	// RepoDir is the directory for IPFS repo
	RepoDir string
	// ProtocolID is the protocol ID for libp2p
	ProtocolID string
	// BootstrapPeers is a list of multiaddr strings for bootstrap peers
	BootstrapPeers []string
	// StoredPeers is a list of pre-loaded peer addresses from storage
	StoredPeers []peer.AddrInfo
	// EnableRelay enables circuit relay for NAT traversal
	EnableRelay bool
	// EnableHolePunching enables hole punching for direct NAT connections (default: true)
	EnableHolePunching bool
	// DHTMode is the DHT mode: "auto", "client", or "server"
	DHTMode string
	// Bootstrap holds the bootstrap-specific configuration
	Bootstrap *BootstrapConfig
	// Storage is the storage interface for persisting peers
	Storage storage.Storage
}

// BootstrapConfig holds configuration for the DHT rendezvous bootstrap mechanism
type BootstrapConfig struct {
	// StoredPeerTimeoutSecs is the maximum age of stored peers before they're considered stale
	StoredPeerTimeoutSecs int `yaml:"stored_peer_timeout_seconds"`
	// MinBabylonPeersRequired is the minimum number of Babylon peers needed
	MinBabylonPeersRequired int `yaml:"min_babylon_peers_required"`
}

// DefaultBootstrapConfig returns a BootstrapConfig with sensible defaults
func DefaultBootstrapConfig() *BootstrapConfig {
	return &BootstrapConfig{
		StoredPeerTimeoutSecs:   86400, // 24 hours — match DHT record TTL
		MinBabylonPeersRequired: 1,     // one peer is enough to form a DHT
	}
}

// NetworkNode is the interface for network operations.
type NetworkNode interface {
	// Lifecycle
	Start() error
	Stop() error

	// Network operations
	ConnectToPeer(maddr string) error
	FindPeer(peerID string) (*peer.AddrInfo, error)
	Subscribe(topic string) (*Subscription, error)
	AdvertiseSelf(ctx context.Context) error
	WaitForDHT(timeout time.Duration) error

	// Identity
	PeerID() string
	Multiaddrs() []string
	IsStarted() bool
	Context() context.Context

	// Additional methods used by messaging service
	Host() host.Host
	DHT() *dht.IpfsDHT
	PubSub() *pubsub.PubSub
	GetNetworkInfo() *NetworkInfo
	PublishTo(pubKey []byte, data []byte) error

	// Diagnostics and metrics (used by CLI)
	GetDHTInfo() *DHTInfo
	ClearAllBackoffs()
	GetMDnsStats() MDnsStats
	GetMetricsFull() *MetricsFull

	// Babylon DHT and bootstrap status (used by CLI)
	GetBabylonDHTInfo() *BabylonDHTInfo
	GetPeerCountsBySource() *PeerCountsBySource
	GetBootstrapStatus() *BootstrapStatus
	TriggerRendezvousDiscovery() int

	// Bootstrap state queries (for decoupled architecture)
	IsIPFSBootstrapComplete() bool
	IsBabylonBootstrapComplete() bool
	IsBabylonBootstrapDeferred() bool
	IsRendezvousActive() bool
	TriggerLazyBootstrap() error
}

// Ensure Node implements NetworkNode interface
var _ NetworkNode = (*Node)(nil)

// Node represents an embedded IPFS node
type Node struct {
	config     *Config
	host       host.Host
	dht        *dht.IpfsDHT  // IPFS DHT for transport layer
	babylonDHT *dht.IpfsDHT  // Babylon DHT for protocol layer (identity, prekeys, etc.)
	pubsub     *pubsub.PubSub
	discovery  *routing.RoutingDiscovery
	mdns       mdns.Service
	ctx        context.Context
	cancel     context.CancelFunc
	peerChan   <-chan peer.AddrInfo
	isStarted  bool
	topicCache *topicCache

	// Peer tracking
	peerCount     atomic.Int32
	peerMu        sync.RWMutex
	mdnsCount     atomic.Int32
	lastPeerFound time.Time

	// Event subscriptions
	peerEventSub interface{ Close() error }

	// DHT maintenance
	dhtMaintenanceDone chan struct{}

	// Peer scoring for connection management
	peerScores  map[string]*PeerScore // peer.ID -> score
	peerScoreMu sync.RWMutex

	// Connection health tracking
	healthCheckInterval time.Duration
	lastHealthCheck     time.Time

	// Network metrics
	metrics *MetricsCollector

	// Start time for uptime tracking
	startTime time.Time

	// === DECOUPLED BOOTSTRAP STATE FLAGS ===
	// IPFS DHT bootstrap state (transport layer)
	ipfsBootstrapComplete atomic.Bool

	// Babylon DHT bootstrap state (protocol layer)
	babylonBootstrapComplete atomic.Bool
	babylonBootstrapDeferred atomic.Bool

	// Rendezvous discovery state
	rendezvousActive atomic.Bool
}

// PeerInfo contains information about a peer
type PeerInfo struct {
	ID        string
	Addresses []string
	Protocols []string
	Connected bool
}

// NetworkInfo contains network status information
type NetworkInfo struct {
	PeerID             string
	Multiaddrs         []string
	ListenAddrs        []string
	ConnectedPeers     []PeerInfo
	ConnectedPeerCount int
	IsStarted          bool
}

// DHTInfo contains DHT routing table and connection information
type DHTInfo struct {
	IsStarted              bool
	Mode                   string
	RoutingTableSize       int
	RoutingTablePeers      []string
	ConnectedPeerCount     int
	HasBootstrapConnection bool
}

// MetricsFull contains comprehensive network metrics
type MetricsFull struct {
	NetworkMetrics
	DHTInfo               *DHTInfo
	MDnsStats             MDnsStats
	DiscoveryBySource     map[string]int64
	ConnectionSuccessRate float64
	MessageSuccessRate    float64
}

// MDnsStats contains mDNS discovery statistics
type MDnsStats struct {
	TotalDiscoveries int32
	LastPeerFound    time.Time
}

// BabylonDHTInfo contains information about Babylon DHT peers
type BabylonDHTInfo struct {
	StoredBabylonPeers    int
	ConnectedBabylonPeers int
	BabylonPeerIDs        []string
	RendezvousActive      bool
}

// PeerCountsBySource contains peer counts grouped by discovery source
type PeerCountsBySource struct {
	Babylon        int // Peers from Babylon bootstrap
	IPFSBootstrap  int // Config bootstrap peers
	IPFSDiscovery  int // Discovered via public IPFS DHT
	MDNS           int // Local mDNS discovery
	ConnectedTotal int // Currently connected peers
}

// BootstrapStatus contains the status of the bootstrap process
// Updated for decoupled architecture with separate IPFS and Babylon DHT states
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
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		RepoDir:    DefaultRepoDir,
		ProtocolID: DefaultProtocolID,
		BootstrapPeers: []string{
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiRNN6vEC9qmL9egu92p",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
			"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
			"/ip4/104.236.179.241/tcp/4001/p2p/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
			"/ip4/128.199.219.111/tcp/4001/p2p/QmSoLV4Bbm51jM9C4gDYZQ9Cy3U6aXMJDAbzgu2fzaDs64",
			"/ip4/104.236.76.40/tcp/4001/p2p/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
			"/ip4/178.62.158.147/tcp/4001/p2p/QmSoLer265NRgSp2LA3dPaeykiS1J6DifTC88f5uVQKNAd",
		},
		EnableRelay:        true,
		EnableHolePunching: true,
		DHTMode:            "auto",
		Bootstrap:          DefaultBootstrapConfig(),
	}
}

// TopicFromPublicKey derives a PubSub topic from a public key.
// Per protocol spec §4.3: DM topic = "babylon-dm-" + hex(SHA256(recipient.pub)[:8])
func TopicFromPublicKey(pubKey []byte) string {
	hash := sha256.Sum256(pubKey)
	return "babylon-dm-" + hex.EncodeToString(hash[:8])
}
