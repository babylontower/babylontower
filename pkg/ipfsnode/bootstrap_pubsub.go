package ipfsnode

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"babylontower/pkg/storage"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

const (
	// DefaultBootstrapPubSubTopic is the default PubSub topic for bootstrap discovery
	DefaultBootstrapPubSubTopic = "/babylon/bootstrap"

	// BootstrapRequestType is the message type for bootstrap requests
	BootstrapRequestType = "bootstrap_request"

	// BootstrapResponseType is the message type for bootstrap responses
	BootstrapResponseType = "bootstrap_response"

	// DefaultPubSubListenSecs is the default time to listen for bootstrap responses
	DefaultPubSubListenSecs = 5

	// DefaultMinBabylonPeersRequired is the minimum number of Babylon peers needed
	DefaultMinBabylonPeersRequired = 3

	// DefaultStoredPeerTimeoutSecs is the default timeout for stored peers
	DefaultStoredPeerTimeoutSecs = 10
)

// BootstrapAnnouncement represents a bootstrap discovery message
type BootstrapAnnouncement struct {
	// Type is either "bootstrap_request" or "bootstrap_response"
	Type string `json:"type"`

	// PeerID is the libp2p peer ID of the sender
	PeerID string `json:"peer_id"`

	// Timestamp is the Unix timestamp of the message
	Timestamp uint64 `json:"timestamp"`

	// UptimeSecs is the node uptime in seconds (only for helpers responding)
	UptimeSecs uint64 `json:"uptime_secs,omitempty"`

	// PeerCount is the number of connected peers (only for helpers responding)
	PeerCount int `json:"peer_count,omitempty"`

	// BootstrapPeers is a list of multiaddr strings (only in responses)
	BootstrapPeers []string `json:"bootstrap_peers,omitempty"`

	// RequestID is a unique identifier for deduplication (only in requests)
	RequestID string `json:"request_id,omitempty"`
}

// BootstrapPubSubConfig holds configuration for PubSub bootstrap discovery
type BootstrapPubSubConfig struct {
	// PubSubTopic is the topic name for bootstrap discovery
	PubSubTopic string `yaml:"pubsub_topic"`

	// PubSubListenSecs is how long to listen for responses during bootstrap
	PubSubListenSecs int `yaml:"pubsub_listen_seconds"`

	// MinBabylonPeersRequired is the minimum number of Babylon peers needed
	MinBabylonPeersRequired int `yaml:"min_babylon_peers_required"`
}

// DefaultBootstrapPubSubConfig returns a BootstrapPubSubConfig with sensible defaults
func DefaultBootstrapPubSubConfig() *BootstrapPubSubConfig {
	return &BootstrapPubSubConfig{
		PubSubTopic:             DefaultBootstrapPubSubTopic,
		PubSubListenSecs:        DefaultPubSubListenSecs,
		MinBabylonPeersRequired: DefaultMinBabylonPeersRequired,
	}
}

// BootstrapPubSub handles PubSub-based bootstrap discovery
type BootstrapPubSub struct {
	mu sync.RWMutex

	node   *Node
	config *BootstrapPubSubConfig

	// Rate limiter for responses
	rateLimiter *BootstrapRateLimiter

	// Subscription to bootstrap topic
	sub *pubsub.Subscription

	// Topic handle
	topic *pubsub.Topic

	// Channel for discovered peers
	peerChan chan peer.AddrInfo

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Start time for uptime calculation
	startTime time.Time

	// Whether the helper goroutine is running
	helperRunning bool
}

// NewBootstrapPubSub creates a new PubSub bootstrap handler
func NewBootstrapPubSub(node *Node, config *BootstrapPubSubConfig, rateLimiter *BootstrapRateLimiter) (*BootstrapPubSub, error) {
	if config == nil {
		config = DefaultBootstrapPubSubConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	bp := &BootstrapPubSub{
		node:        node,
		config:      config,
		rateLimiter: rateLimiter,
		peerChan:    make(chan peer.AddrInfo, 100),
		ctx:         ctx,
		cancel:      cancel,
		startTime:   time.Now(),
		helperRunning: false,
	}

	return bp, nil
}

// Start initializes the PubSub subscription and starts listening for bootstrap messages
func (bp *BootstrapPubSub) Start() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.node.pubsub == nil {
		return ErrNodeNotStarted
	}

	// Join the bootstrap topic
	topic, err := bp.node.pubsub.Join(bp.config.PubSubTopic)
	if err != nil {
		return fmt.Errorf("failed to join bootstrap topic: %w", err)
	}
	bp.topic = topic

	// Subscribe to the topic
	sub, err := topic.Subscribe()
	if err != nil {
		topic.Close()
		return fmt.Errorf("failed to subscribe to bootstrap topic: %w", err)
	}
	bp.sub = sub

	// Start message handler goroutine
	go bp.handleBootstrapAnnouncements()

	// Start periodic bootstrap requests to discover peers
	// This is critical for new nodes to find each other
	go bp.periodicBootstrapRequests()

	logger.Infow("bootstrap PubSub handler started",
		"topic", bp.config.PubSubTopic)

	return nil
}

// periodicBootstrapRequests sends periodic bootstrap requests to discover peers
// This helps new nodes find each other even if they start at the same time
func (bp *BootstrapPubSub) periodicBootstrapRequests() {
	// Wait for topic to fully join mesh first
	time.Sleep(3 * time.Second)

	// Send initial bootstrap request
	bp.sendBootstrapRequest()

	// Continue with periodic requests (every 10 seconds for first minute, then every 30 seconds)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	requestCount := 0
	for {
		select {
		case <-bp.ctx.Done():
			return
		case <-ticker.C:
			requestCount++
			
			// Send request more frequently in the first minute
			if requestCount > 6 {
				ticker.Reset(30 * time.Second)
			}
			
			bp.sendBootstrapRequest()
		}
	}
}

// sendBootstrapRequest sends a single bootstrap request and attempts to connect to responders
func (bp *BootstrapPubSub) sendBootstrapRequest() {
	ctx, cancel := context.WithTimeout(bp.ctx, 10*time.Second)
	defer cancel()

	discoveredPeers := bp.RequestBootstrap(ctx)
	
	if len(discoveredPeers) > 0 {
		logger.Infow("bootstrap request discovered peers", "count", len(discoveredPeers))
		
		// Save discovered peers
		if err := bp.saveDiscoveredPeers(discoveredPeers); err != nil {
			logger.Debugw("failed to save discovered peers", "error", err)
		}
		
		// Connect to discovered peers asynchronously
		go func() {
			connectCtx, connectCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer connectCancel()
			
			connected := bp.node.connectToPeersParallel(connectCtx, discoveredPeers)
			logger.Infow("connected to discovered peers", "connected", connected, "total", len(discoveredPeers))
		}()
	}
}

// Stop gracefully shuts down the PubSub bootstrap handler
func (bp *BootstrapPubSub) Stop() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Cancel context to stop goroutines
	bp.cancel()

	// Cancel subscription
	if bp.sub != nil {
		bp.sub.Cancel()
		bp.sub = nil
	}

	// Close topic
	if bp.topic != nil {
		if err := bp.topic.Close(); err != nil {
			logger.Warnw("failed to close bootstrap topic", "error", err)
		}
		bp.topic = nil
	}

	// Close peer channel
	close(bp.peerChan)

	logger.Info("bootstrap PubSub handler stopped")

	return nil
}

// handleBootstrapAnnouncements processes incoming bootstrap messages
func (bp *BootstrapPubSub) handleBootstrapAnnouncements() {
	for {
		select {
		case <-bp.ctx.Done():
			return
		default:
			msg, err := bp.sub.Next(bp.ctx)
			if err != nil {
				if bp.ctx.Err() != nil {
					return
				}
				logger.Debugw("error receiving bootstrap message", "error", err)
				continue
			}

			// Parse the announcement
			var announcement BootstrapAnnouncement
			if err := json.Unmarshal(msg.Data, &announcement); err != nil {
				logger.Debugw("failed to parse bootstrap announcement", "error", err)
				continue
			}

			// Skip messages from self
			if announcement.PeerID == bp.node.host.ID().String() {
				continue
			}

			// Handle based on message type
			switch announcement.Type {
			case BootstrapRequestType:
				bp.handleBootstrapRequest(&announcement, msg.ReceivedFrom)
			case BootstrapResponseType:
				bp.handleBootstrapResponse(&announcement)
			default:
				logger.Debugw("unknown bootstrap message type", "type", announcement.Type)
			}
		}
	}
}

// handleBootstrapRequest processes a bootstrap request and potentially responds
// CRITICAL: This is where lazy bootstrap is triggered for incoming requests
func (bp *BootstrapPubSub) handleBootstrapRequest(announcement *BootstrapAnnouncement, sender peer.ID) {
	logger.Debugw("received bootstrap request",
		"from", announcement.PeerID,
		"request_id", announcement.RequestID)

	// Check if we should respond (rate limited)
	if bp.rateLimiter == nil || !bp.rateLimiter.ShouldRespond(announcement.RequestID) {
		logger.Debugw("skipping bootstrap request (rate limited)",
			"from", announcement.PeerID,
			"request_id", announcement.RequestID)
		return
	}

	// === LAZY BOOTSTRAP TRIGGER ===
	// If our Babylon bootstrap is incomplete, use this request as trigger
	// to bootstrap ourselves. This is the "help first, then help ourselves" pattern.
	if !bp.node.IsBabylonBootstrapComplete() {
		logger.Infow("lazy Babylon bootstrap triggered by incoming bootstrap request",
			"requester", announcement.PeerID,
			"our_bootstrap_complete", bp.node.IsBabylonBootstrapComplete(),
			"our_bootstrap_deferred", bp.node.IsBabylonBootstrapDeferred())

		// Save requester as potential Babylon peer for later connection
		bp.savePeerForLater(sender)

		// Trigger our own bootstrap using this peer as a starting point
		// Run in goroutine to avoid blocking the response
		go func() {
			if err := bp.node.TriggerLazyBootstrap(); err != nil {
				logger.Debugw("lazy bootstrap trigger failed",
					"requester", announcement.PeerID,
					"error", err)
			}
		}()
	}

	// Send response with our peer multiaddrs
	// We respond even if our bootstrap is incomplete - this is the "help first" principle
	bp.sendBootstrapResponse(announcement.PeerID)
}

// handleBootstrapResponse processes a bootstrap response and extracts peer info
func (bp *BootstrapPubSub) handleBootstrapResponse(announcement *BootstrapAnnouncement) {
	logger.Infow("received bootstrap response",
		"from", announcement.PeerID,
		"peer_count", len(announcement.BootstrapPeers),
		"uptime_secs", announcement.UptimeSecs)

	// Parse multiaddrs and send to peer channel
	for _, addrStr := range announcement.BootstrapPeers {
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			logger.Debugw("failed to parse multiaddr from bootstrap response",
				"addr", addrStr, "error", err)
			continue
		}

		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			logger.Debugw("failed to parse peer info from bootstrap response",
				"addr", addrStr, "error", err)
			continue
		}

		// Skip self
		if peerInfo.ID == bp.node.host.ID() {
			continue
		}

		// Send to peer channel (non-blocking)
		select {
		case bp.peerChan <- *peerInfo:
			logger.Debugw("queued discovered peer for connection",
				"peer", peerInfo.ID, "addrs", len(peerInfo.Addrs))
		default:
			logger.Debugw("bootstrap peer channel full, dropping peer",
				"peer", peerInfo.ID)
		}
	}
}

// sendBootstrapResponse sends a bootstrap response to a requesting peer
func (bp *BootstrapPubSub) sendBootstrapResponse(requesterPeerID string) {
	// Get our listen multiaddrs
	addrs := bp.node.host.Addrs()
	peerID := bp.node.host.ID().String()

	// Build multiaddr strings with our peer ID
	bootstrapPeers := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		// Only include addresses that can be dialed by others
		// Skip localhost-only addresses unless in development
		fullAddr := fmt.Sprintf("%s/p2p/%s", addr.String(), peerID)
		bootstrapPeers = append(bootstrapPeers, fullAddr)
	}

	// Calculate uptime
	uptime := uint64(time.Since(bp.startTime).Seconds())

	// Get peer count
	peerCount := len(bp.node.host.Network().Peers())

	// Build response
	response := BootstrapAnnouncement{
		Type:           BootstrapResponseType,
		PeerID:         peerID,
		Timestamp:      uint64(time.Now().Unix()),
		UptimeSecs:     uptime,
		PeerCount:      peerCount,
		BootstrapPeers: bootstrapPeers,
	}

	// Marshal to JSON
	data, err := json.Marshal(response)
	if err != nil {
		logger.Warnw("failed to marshal bootstrap response", "error", err)
		return
	}

	// Publish to topic
	if err := bp.topic.Publish(bp.ctx, data); err != nil {
		logger.Debugw("failed to publish bootstrap response", "error", err)
	} else {
		logger.Debugw("sent bootstrap response",
			"requester", requesterPeerID,
			"peer_count", len(bootstrapPeers))
	}
}

// RequestBootstrap sends a bootstrap request and collects responses
// Returns a list of discovered peer addresses
func (bp *BootstrapPubSub) RequestBootstrap(ctx context.Context) []peer.AddrInfo {
	// Generate unique request ID (handle nil node for testing)
	var requestID string
	if bp.node != nil && bp.node.host != nil {
		requestID = fmt.Sprintf("%s-%d", bp.node.host.ID().String(), time.Now().UnixNano())
	} else {
		requestID = fmt.Sprintf("anon-%d", time.Now().UnixNano())
	}

	// Build request (handle nil node for testing)
	var peerIDStr string
	if bp.node != nil && bp.node.host != nil {
		peerIDStr = bp.node.host.ID().String()
	} else {
		peerIDStr = "anon"
	}

	request := BootstrapAnnouncement{
		Type:      BootstrapRequestType,
		PeerID:    peerIDStr,
		Timestamp: uint64(time.Now().Unix()),
		RequestID: requestID,
	}

	// Marshal to JSON
	data, err := json.Marshal(request)
	if err != nil {
		logger.Warnw("failed to marshal bootstrap request", "error", err)
		return nil
	}

	// Publish request (handle nil topic for testing)
	if bp.topic == nil {
		logger.Debugw("topic is nil, skipping publish (test mode)")
		return nil
	}

	if err := bp.topic.Publish(ctx, data); err != nil {
		logger.Warnw("failed to publish bootstrap request", "error", err)
		return nil
	}

	logger.Infow("sent bootstrap request", "request_id", requestID, "topic", bp.config.PubSubTopic)

	// Collect responses with timeout
	var discoveredPeers []peer.AddrInfo
	listenDuration := time.Duration(bp.config.PubSubListenSecs) * time.Second

	logger.Debugw("listening for bootstrap responses", "duration_secs", bp.config.PubSubListenSecs)

	select {
	case <-ctx.Done():
		return discoveredPeers
	case <-time.After(listenDuration):
		// Timeout - collect what we have
	}

	// Drain peer channel
	for {
		select {
		case peerInfo := <-bp.peerChan:
			discoveredPeers = append(discoveredPeers, peerInfo)
		default:
			return discoveredPeers
		}
	}
}

// StartHelper starts the bootstrap helper goroutine
// This makes our node respond to bootstrap requests from other nodes
func (bp *BootstrapPubSub) StartHelper() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.helperRunning {
		return nil // Already running
	}

	if bp.topic == nil {
		return fmt.Errorf("bootstrap topic not initialized")
	}

	bp.helperRunning = true
	bp.startTime = time.Now()

	logger.Info("bootstrap helper started - node will respond to bootstrap requests")

	return nil
}

// StopHelper stops the bootstrap helper goroutine
func (bp *BootstrapPubSub) StopHelper() {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.helperRunning = false
	logger.Info("bootstrap helper stopped")
}

// IsHelperRunning returns true if the helper is currently active
func (bp *BootstrapPubSub) IsHelperRunning() bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.helperRunning
}

// GetUptime returns the node uptime since helper started
func (bp *BootstrapPubSub) GetUptime() time.Duration {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return time.Since(bp.startTime)
}

// saveDiscoveredPeers saves discovered Babylon peers to storage
func (bp *BootstrapPubSub) saveDiscoveredPeers(peers []peer.AddrInfo) error {
	if bp.node.config.Storage == nil {
		return nil // No storage configured
	}

	now := time.Now()
	saved := 0

	for _, peerInfo := range peers {
		if len(peerInfo.Addrs) == 0 {
			continue
		}

		// Convert multiaddrs to strings
		addrs := make([]string, len(peerInfo.Addrs))
		for i, addr := range peerInfo.Addrs {
			addrs[i] = addr.String()
		}

		// Create peer record with SourceBabylon
		record := &storage.PeerRecord{
			PeerID:        peerInfo.ID.String(),
			Multiaddrs:    addrs,
			FirstSeen:     now,
			LastSeen:      now,
			LastConnected: time.Time{}, // Not connected yet
			ConnectCount:  0,
			FailCount:     0,
			Source:        storage.SourceBabylon,
			Protocols:     []string{}, // Will be populated on connection
			LatencyMs:     0,
		}

		if err := bp.node.config.Storage.AddPeer(record); err != nil {
			logger.Debugw("failed to save discovered Babylon peer",
				"peer", peerInfo.ID, "error", err)
			continue
		}
		saved++
	}

	if saved > 0 {
		logger.Infow("saved discovered Babylon peers to storage",
			"count", saved, "total", len(peers))
	}

	return nil
}

// savePeerForLater saves a peer's address info for later connection attempts
// This is used when we receive a bootstrap request and want to remember the requester
func (bp *BootstrapPubSub) savePeerForLater(p peer.ID) {
	// Get peer's addresses from peerstore
	peerInfo := bp.node.host.Peerstore().PeerInfo(p)
	if len(peerInfo.Addrs) == 0 {
		// No addresses available, use the sender address from the message
		// This is a fallback - typically the peerstore will have the address
		logger.Debugw("no addresses in peerstore for peer", "peer", p)
		return
	}

	// Convert multiaddrs to strings
	addrs := make([]string, len(peerInfo.Addrs))
	for i, addr := range peerInfo.Addrs {
		addrs[i] = addr.String()
	}

	// Create peer record with SourceBabylon
	record := &storage.PeerRecord{
		PeerID:        p.String(),
		Multiaddrs:    addrs,
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
	if err := bp.node.config.Storage.AddPeer(record); err != nil {
		logger.Debugw("failed to save peer for later", "peer", p, "error", err)
	} else {
		logger.Debugw("saved peer for later connection", "peer", p, "addrs", len(addrs))
	}
}

// LoadStoredBabylonPeers loads previously discovered Babylon peers from storage
func (bp *BootstrapPubSub) LoadStoredBabylonPeers() ([]peer.AddrInfo, error) {
	if bp.node.config.Storage == nil {
		return nil, nil // No storage configured
	}

	// Get peers from storage with SourceBabylon
	peers, err := bp.node.config.Storage.ListPeersBySource(storage.SourceBabylon)
	if err != nil {
		return nil, fmt.Errorf("failed to load stored Babylon peers: %w", err)
	}

	// Convert to AddrInfo
	var addrInfos []peer.AddrInfo
	for _, record := range peers {
		// Skip stale peers (older than configured timeout)
		maxAge := time.Duration(bp.node.config.Bootstrap.StoredPeerTimeoutSecs) * time.Second
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
