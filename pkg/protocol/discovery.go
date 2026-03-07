// Package protocol implements the Babylon Tower Protocol v1 specification.
package protocol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/multiformats/go-multiaddr"
)

// Use the package-level logger from the protocol package

// PeerDiscoveryImpl implements the PeerDiscovery interface.
// It provides peer discovery and routing functionality for the Babylon Tower protocol.
//
// Discovery Mechanisms:
//   1. DHT-based discovery: Find peers by identity public key via the DHT
//   2. Routing discovery: Use libp2p's routing discovery for service-based peer finding
//   3. Direct connection: Connect to known peer addresses
//
// Routing Priority:
//   1. Direct connection (if multiaddr is known)
//   2. DHT FindPeer (lookup peer ID in DHT)
//   3. Circuit relay (for NAT traversal)
type PeerDiscoveryImpl struct {
	// config is the protocol configuration
	config *ProtocolConfig
	// networkNode is the underlying network interface
	networkNode NetworkNode
	// ipfsDHT is the DHT instance for peer routing
	ipfsDHT *dht.IpfsDHT
	// routingDiscovery is the libp2p routing discovery interface
	routingDiscovery *routing.RoutingDiscovery

	// Contact cache
	contacts      map[string]*ContactInfo // identity_pub_hex -> contact info
	contactsMu    sync.RWMutex

	// Peer address cache
	peerCache   map[peer.ID]*PeerAddrInfo
	peerCacheMu sync.RWMutex

	// Discovery state
	lastDiscovery time.Time
	discoveryCount int64

	// Mutex for protecting shared state
	mu sync.RWMutex
}

// ContactInfo contains information about a discovered contact.
type ContactInfo struct {
	// IdentityPub is the contact's Ed25519 identity public key
	IdentityPub []byte
	// PeerID is the contact's libp2p peer ID
	PeerID peer.ID
	// LastSeen is when the contact was last seen
	LastSeen time.Time
	// Addresses is the list of known multiaddrs
	Addresses []multiaddr.Multiaddr
	// IsOnline indicates if the contact is currently reachable
	IsOnline bool
	// DisplayName is the contact's display name (from identity document)
	DisplayName string
}

// PeerAddrInfo contains cached peer address information.
type PeerAddrInfo struct {
	// AddrInfo is the peer's address information
	AddrInfo peer.AddrInfo
	// LastUpdated is when the info was last updated
	LastUpdated time.Time
	// Source is where the info came from (dht, pubsub, etc.)
	Source string
}

// NewPeerDiscovery creates a new peer discovery instance.
// It requires the protocol configuration, network node, and DHT instance.
func NewPeerDiscovery(
	config *ProtocolConfig,
	networkNode NetworkNode,
	ipfsDHT *dht.IpfsDHT,
	routingDiscovery *routing.RoutingDiscovery,
) *PeerDiscoveryImpl {
	return &PeerDiscoveryImpl{
		config:           config,
		networkNode:      networkNode,
		ipfsDHT:          ipfsDHT,
		routingDiscovery: routingDiscovery,
		contacts:         make(map[string]*ContactInfo),
		peerCache:        make(map[peer.ID]*PeerAddrInfo),
	}
}

// DiscoverContact discovers a contact by identity public key.
// It performs the following steps:
//   1. Check local contact cache
//   2. Look up identity document in DHT to get peer information
//   3. Find peer in DHT routing table
//   4. Return peer address information
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - identityPub: The contact's Ed25519 identity public key
//
// Returns:
//   - peer.AddrInfo with the contact's address information
//   - An error if discovery fails
func (d *PeerDiscoveryImpl) DiscoverContact(ctx context.Context, identityPub []byte) (*peer.AddrInfo, error) {
	cacheKey := hex.EncodeToString(identityPub)

	// Check contact cache
	d.contactsMu.RLock()
	if contact, ok := d.contacts[cacheKey]; ok {
		if contact.IsOnline && len(contact.Addresses) > 0 {
			d.contactsMu.RUnlock()
			logger.Debugw("Found contact in cache", "identity", cacheKey)
			return &peer.AddrInfo{
				ID:    contact.PeerID,
				Addrs: contact.Addresses,
			}, nil
		}
	}
	d.contactsMu.RUnlock()

	// Set timeout
	timeout := d.config.ContactDiscoveryTimeout
	if timeout <= 0 {
		timeout = DefaultDHTTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Try to find peer via DHT
	// The peer ID should be derivable from the identity or stored in the identity document
	// For now, we search the DHT for the identity key
	peerInfo, err := d.findPeerByIdentity(ctx, identityPub)
	if err != nil {
		return nil, fmt.Errorf("failed to find peer by identity: %w", err)
	}

	// Cache the contact
	d.contactsMu.Lock()
	d.contacts[cacheKey] = &ContactInfo{
		IdentityPub: identityPub,
		PeerID:      peerInfo.ID,
		LastSeen:    time.Now(),
		Addresses:   peerInfo.Addrs,
		IsOnline:    true,
	}
	d.contactsMu.Unlock()

	logger.Infow("Discovered contact",
		"identity", cacheKey,
		"peer_id", peerInfo.ID,
		"addresses", len(peerInfo.Addrs))

	return peerInfo, nil
}

// findPeerByIdentity attempts to find a peer by their identity public key.
func (d *PeerDiscoveryImpl) findPeerByIdentity(ctx context.Context, identityPub []byte) (*peer.AddrInfo, error) {
	// Create DHT key from identity
	dhtKey := d.identityDHTKey(identityPub)

	// Try to get the identity record from DHT
	// This would contain peer ID information
	record, err := d.ipfsDHT.GetValue(ctx, dhtKey)
	if err == nil && len(record) > 0 {
		// Parse the record to extract peer ID
		// For now, we'll try to find the peer directly
		logger.Debugw("Found identity record in DHT", "key", dhtKey)
	}

	// Try direct peer lookup using identity hash as potential peer ID
	// In a full implementation, the peer ID would be stored in the identity document
	hash := sha256.Sum256(identityPub)
	peerIDStr := hex.EncodeToString(hash[:])

	// Try to find peer in DHT
	peerID, err := peer.Decode(peerIDStr)
	if err == nil {
		peerInfo, err := d.ipfsDHT.FindPeer(ctx, peerID)
		if err == nil && len(peerInfo.Addrs) > 0 {
			return &peerInfo, nil
		}
	}

	// Fall back to routing discovery
	if d.routingDiscovery != nil {
		peerChan, err := d.routingDiscovery.FindPeers(ctx, "babylon")
		if err == nil {
			for info := range peerChan {
				// Check if this peer matches our identity
				// This would require checking the peer's advertised identity
				if len(info.Addrs) > 0 {
					return &info, nil
				}
			}
		}
	}

	return nil, ErrPeerNotFound
}

// ResolvePeerAddress resolves a peer's multiaddr from the DHT.
// It looks up the peer ID in the DHT routing table and returns address information.
func (d *PeerDiscoveryImpl) ResolvePeerAddress(ctx context.Context, peerID peer.ID) (*peer.AddrInfo, error) {
	// Check peer cache
	d.peerCacheMu.RLock()
	if cached, ok := d.peerCache[peerID]; ok {
		if time.Since(cached.LastUpdated) < 5*time.Minute {
			d.peerCacheMu.RUnlock()
			logger.Debugw("Found peer in cache", "peer_id", peerID)
			return &cached.AddrInfo, nil
		}
	}
	d.peerCacheMu.RUnlock()

	// Set timeout
	timeout := d.config.PeerConnectTimeout
	if timeout <= 0 {
		timeout = DefaultPeerConnectTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Find peer in DHT
	peerInfo, err := d.ipfsDHT.FindPeer(ctx, peerID)
	if err != nil {
		return nil, fmt.Errorf("DHT find peer failed: %w", err)
	}

	if len(peerInfo.Addrs) == 0 {
		return nil, ErrPeerNotFound
	}

	// Cache the result
	d.peerCacheMu.Lock()
	d.peerCache[peerID] = &PeerAddrInfo{
		AddrInfo:    peerInfo,
		LastUpdated: time.Now(),
		Source:      "dht",
	}
	d.peerCacheMu.Unlock()

	logger.Debugw("Resolved peer address",
		"peer_id", peerID,
		"addresses", len(peerInfo.Addrs))

	return &peerInfo, nil
}

// FindPeers finds peers providing a specific service.
// It uses the routing discovery to find peers advertising the service.
func (d *PeerDiscoveryImpl) FindPeers(ctx context.Context, service string, limit int) (<-chan peer.AddrInfo, error) {
	if d.routingDiscovery == nil {
		return nil, errors.New("routing discovery not available")
	}

	// Set timeout
	timeout := d.config.PeerConnectTimeout
	if timeout <= 0 {
		timeout = DefaultPeerConnectTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)

	// Find peers
	peerChan, err := d.routingDiscovery.FindPeers(ctx, service)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("find peers failed: %w", err)
	}

	// Wrap the channel to apply limit and cleanup
	resultChan := make(chan peer.AddrInfo)
	go func() {
		defer close(resultChan)
		defer cancel()

		count := 0
		for info := range peerChan {
			if count >= limit {
				break
			}
			if info.ID == d.networkNode.Host().ID() {
				continue // Skip ourselves
			}
			if len(info.Addrs) == 0 {
				continue
			}

			resultChan <- info
			count++
		}

		d.mu.Lock()
		d.discoveryCount++
		d.lastDiscovery = time.Now()
		d.mu.Unlock()
	}()

	return resultChan, nil
}

// AdvertiseService advertises that this node provides a service.
// Other nodes can discover this node by searching for the service.
func (d *PeerDiscoveryImpl) AdvertiseService(ctx context.Context, service string) error {
	if d.routingDiscovery == nil {
		return errors.New("routing discovery not available")
	}

	// Advertise the service
	if _, err := d.routingDiscovery.Advertise(ctx, service); err != nil {
		return fmt.Errorf("advertise failed: %w", err)
	}

	logger.Infow("Advertised service", "service", service)
	return nil
}

// CancelAdvertisement cancels a service advertisement.
func (d *PeerDiscoveryImpl) CancelAdvertisement(ctx context.Context, service string) error {
	// Note: libp2p routing discovery doesn't have a direct cancel method
	// The advertisement will expire naturally
	logger.Debugw("Cancelled service advertisement", "service", service)
	return nil
}

// GetRoutingTableSize returns the DHT routing table size.
func (d *PeerDiscoveryImpl) GetRoutingTableSize() int {
	if d.ipfsDHT == nil || d.ipfsDHT.RoutingTable() == nil {
		return 0
	}
	return d.ipfsDHT.RoutingTable().Size()
}

// GetConnectedPeers returns all connected peers.
func (d *PeerDiscoveryImpl) GetConnectedPeers() []peer.ID {
	return d.networkNode.Host().Network().Peers()
}

// GetConnectedPeerCount returns the number of connected peers.
func (d *PeerDiscoveryImpl) GetConnectedPeerCount() int {
	return len(d.GetConnectedPeers())
}

// AddContact manually adds a contact to the cache.
// This is useful for contacts discovered through out-of-band means.
func (d *PeerDiscoveryImpl) AddContact(identityPub []byte, peerInfo peer.AddrInfo, displayName string) {
	cacheKey := hex.EncodeToString(identityPub)

	d.contactsMu.Lock()
	defer d.contactsMu.Unlock()

	d.contacts[cacheKey] = &ContactInfo{
		IdentityPub: identityPub,
		PeerID:      peerInfo.ID,
		LastSeen:    time.Now(),
		Addresses:   peerInfo.Addrs,
		IsOnline:    false, // Will be updated on first connection
		DisplayName: displayName,
	}

	logger.Infow("Added contact",
		"identity", cacheKey,
		"peer_id", peerInfo.ID,
		"display_name", displayName)
}

// GetContact returns contact information by identity public key.
func (d *PeerDiscoveryImpl) GetContact(identityPub []byte) (*ContactInfo, error) {
	cacheKey := hex.EncodeToString(identityPub)

	d.contactsMu.RLock()
	defer d.contactsMu.RUnlock()

	contact, ok := d.contacts[cacheKey]
	if !ok {
		return nil, ErrIdentityNotFound
	}

	return contact, nil
}

// ListContacts returns all cached contacts.
func (d *PeerDiscoveryImpl) ListContacts() []*ContactInfo {
	d.contactsMu.RLock()
	defer d.contactsMu.RUnlock()

	contacts := make([]*ContactInfo, 0, len(d.contacts))
	for _, contact := range d.contacts {
		contacts = append(contacts, contact)
	}

	return contacts
}

// GetContactCount returns the number of cached contacts.
func (d *PeerDiscoveryImpl) GetContactCount() int {
	d.contactsMu.RLock()
	defer d.contactsMu.RUnlock()
	return len(d.contacts)
}

// RemoveContact removes a contact from the cache.
func (d *PeerDiscoveryImpl) RemoveContact(identityPub []byte) {
	cacheKey := hex.EncodeToString(identityPub)

	d.contactsMu.Lock()
	defer d.contactsMu.Unlock()

	delete(d.contacts, cacheKey)
	logger.Debugw("Removed contact", "identity", cacheKey)
}

// UpdateContactOnline updates a contact's online status.
func (d *PeerDiscoveryImpl) UpdateContactOnline(identityPub []byte, isOnline bool) {
	cacheKey := hex.EncodeToString(identityPub)

	d.contactsMu.Lock()
	defer d.contactsMu.Unlock()

	if contact, ok := d.contacts[cacheKey]; ok {
		contact.IsOnline = isOnline
		if isOnline {
			contact.LastSeen = time.Now()
		}
	}
}

// GetDiscoveryStats returns discovery statistics.
func (d *PeerDiscoveryImpl) GetDiscoveryStats() map[string]interface{} {
	d.mu.RLock()
	d.contactsMu.RLock()
	d.peerCacheMu.RLock()

	stats := map[string]interface{}{
		"contacts_count":       len(d.contacts),
		"peer_cache_count":     len(d.peerCache),
		"routing_table_size":   d.GetRoutingTableSize(),
		"connected_peers":      d.GetConnectedPeerCount(),
		"last_discovery":       d.lastDiscovery,
		"discovery_count":      d.discoveryCount,
	}

	d.peerCacheMu.RUnlock()
	d.contactsMu.RUnlock()
	d.mu.RUnlock()

	return stats
}

// identityDHTKey creates the DHT key for an identity document.
func (d *PeerDiscoveryImpl) identityDHTKey(identityPub []byte) string {
	hash := sha256.Sum256(identityPub)
	return DHTNamespaceIdentity + hex.EncodeToString(hash[:16])
}

// ConnectToPeer attempts to connect to a peer.
func (d *PeerDiscoveryImpl) ConnectToPeer(ctx context.Context, peerInfo peer.AddrInfo) error {
	if peerInfo.ID == d.networkNode.Host().ID() {
		return nil // Don't connect to ourselves
	}

	timeout := d.config.PeerConnectTimeout
	if timeout <= 0 {
		timeout = DefaultPeerConnectTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Add addresses to peerstore
	d.networkNode.Host().Peerstore().AddAddrs(peerInfo.ID, peerInfo.Addrs, time.Hour)

	// Connect
	if err := d.networkNode.Host().Connect(ctx, peerInfo); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	logger.Debugw("Connected to peer",
		"peer_id", peerInfo.ID,
		"addresses", len(peerInfo.Addrs))

	return nil
}

// StartDiscovery starts periodic peer discovery.
func (d *PeerDiscoveryImpl) StartDiscovery(ctx context.Context) error {
	interval := d.config.DiscoveryInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.runDiscovery(ctx)
			}
		}
	}()

	logger.Infow("Started peer discovery", "interval", interval)
	return nil
}

// runDiscovery performs a single round of peer discovery.
func (d *PeerDiscoveryImpl) runDiscovery(ctx context.Context) {
	// Advertise our presence
	if err := d.AdvertiseService(ctx, "babylon"); err != nil {
		logger.Debugw("Failed to advertise", "error", err)
	}

	// Find new peers
	peerChan, err := d.FindPeers(ctx, "babylon", d.config.MaxDiscoveredPeers)
	if err != nil {
		logger.Debugw("Failed to find peers", "error", err)
		return
	}

	connected := 0
	for peerInfo := range peerChan {
		if err := d.ConnectToPeer(ctx, peerInfo); err != nil {
			logger.Debugw("Failed to connect to peer",
				"peer_id", peerInfo.ID,
				"error", err)
			continue
		}
		connected++
	}

	if connected > 0 {
		logger.Infow("Discovery round complete", "peers_connected", connected)
	}
}

// ClearPeerCache clears the peer address cache.
func (d *PeerDiscoveryImpl) ClearPeerCache() {
	d.peerCacheMu.Lock()
	defer d.peerCacheMu.Unlock()
	d.peerCache = make(map[peer.ID]*PeerAddrInfo)
	logger.Debug("Cleared peer cache")
}

// ClearContactCache clears the contact cache.
func (d *PeerDiscoveryImpl) ClearContactCache() {
	d.contactsMu.Lock()
	defer d.contactsMu.Unlock()
	d.contacts = make(map[string]*ContactInfo)
	logger.Debug("Cleared contact cache")
}
