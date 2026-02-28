// Package peerstore provides peer address book functionality for persistent peer storage.
// It stores peer IDs, multiaddresses, and identity public keys for later connection.
package peerstore

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	bterrors "babylontower/pkg/errors"
	"babylontower/pkg/ipfsnode"
	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"
	"github.com/multiformats/go-multiaddr"
)

// ContactTracker manages contact peer information and online status
type ContactTracker struct {
	storage storage.Storage
	ipfsNode *ipfsnode.Node
	mu      sync.RWMutex
	// Cache of contact peer IDs for quick lookup (key: hex-encoded Ed25519 pubkey)
	contactPeers map[string]*ContactPeerInfo
	// Cache refresh interval
	refreshInterval time.Duration
	// Last refresh time
	lastRefresh time.Time
}

// ContactPeerInfo contains information about a contact's peer presence
type ContactPeerInfo struct {
	Ed25519PubKey []byte
	PeerID        string
	Multiaddrs    []multiaddr.Multiaddr
	IsOnline      bool
	LastSeen      time.Time
	ConnectedByUs bool
}

// NewContactTracker creates a new contact tracker
func NewContactTracker(storage storage.Storage, ipfsNode *ipfsnode.Node) *ContactTracker {
	return &ContactTracker{
		storage:         storage,
		ipfsNode:        ipfsNode,
		contactPeers:    make(map[string]*ContactPeerInfo),
		refreshInterval: 30 * time.Second,
	}
}

// LoadContacts loads all contacts from storage and initializes the cache
func (ct *ContactTracker) LoadContacts() error {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	contacts, err := ct.storage.ListContacts()
	if err != nil {
		return fmt.Errorf("failed to list contacts: %w", err)
	}

	ct.contactPeers = make(map[string]*ContactPeerInfo, len(contacts))

	for _, contact := range contacts {
		pubKeyHex := hex.EncodeToString(contact.PublicKey)
		info := &ContactPeerInfo{
			Ed25519PubKey: contact.PublicKey,
			PeerID:        contact.PeerId,
			IsOnline:      false,
			ConnectedByUs: false,
		}

		// Parse multiaddrs if available
		if len(contact.Multiaddrs) > 0 {
			info.Multiaddrs = make([]multiaddr.Multiaddr, 0, len(contact.Multiaddrs))
			for _, addrStr := range contact.Multiaddrs {
				addr, err := multiaddr.NewMultiaddr(addrStr)
				if err == nil {
					info.Multiaddrs = append(info.Multiaddrs, addr)
				}
			}
		}

		// Set last seen from contact record
		if contact.LastSeen > 0 {
			info.LastSeen = time.Unix(int64(contact.LastSeen), 0)
		}

		ct.contactPeers[pubKeyHex] = info
	}

	return nil
}

// UpdateContact updates or adds a contact's peer information
func (ct *ContactTracker) UpdateContact(contact *pb.Contact) error {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	pubKeyHex := hex.EncodeToString(contact.PublicKey)

	// Parse multiaddrs
	var addrs []multiaddr.Multiaddr
	if len(contact.Multiaddrs) > 0 {
		addrs = make([]multiaddr.Multiaddr, 0, len(contact.Multiaddrs))
		for _, addrStr := range contact.Multiaddrs {
			addr, err := multiaddr.NewMultiaddr(addrStr)
			if err == nil {
				addrs = append(addrs, addr)
			}
		}
	}

	info := &ContactPeerInfo{
		Ed25519PubKey: contact.PublicKey,
		PeerID:        contact.PeerId,
		Multiaddrs:    addrs,
		IsOnline:      false,
		ConnectedByUs: false,
	}

	if contact.LastSeen > 0 {
		info.LastSeen = time.Unix(int64(contact.LastSeen), 0)
	}

	ct.contactPeers[pubKeyHex] = info

	return nil
}

// GetContactInfo retrieves peer info for a contact
func (ct *ContactTracker) GetContactInfo(pubKey []byte) (*ContactPeerInfo, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	pubKeyHex := hex.EncodeToString(pubKey)
	info, ok := ct.contactPeers[pubKeyHex]
	if !ok {
		return nil, false
	}

	// Return a copy to avoid race conditions
	infoCopy := *info
	if len(info.Multiaddrs) > 0 {
		infoCopy.Multiaddrs = make([]multiaddr.Multiaddr, len(info.Multiaddrs))
		copy(infoCopy.Multiaddrs, info.Multiaddrs)
	}
	return &infoCopy, true
}

// GetContactPeerID retrieves the PeerID for a contact
func (ct *ContactTracker) GetContactPeerID(pubKey []byte) (string, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	pubKeyHex := hex.EncodeToString(pubKey)
	info, ok := ct.contactPeers[pubKeyHex]
	if !ok || info.PeerID == "" {
		return "", false
	}
	return info.PeerID, true
}

// SetContactOnline marks a contact as online
func (ct *ContactTracker) SetContactOnline(pubKey []byte, peerID string, addrs []multiaddr.Multiaddr) error {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	pubKeyHex := hex.EncodeToString(pubKey)

	info, exists := ct.contactPeers[pubKeyHex]
	if !exists {
		// Create new entry for unknown contact
		info = &ContactPeerInfo{
			Ed25519PubKey: pubKey,
			PeerID:        peerID,
		}
		ct.contactPeers[pubKeyHex] = info
	}

	info.PeerID = peerID
	info.Multiaddrs = addrs
	info.IsOnline = true
	info.LastSeen = time.Now()

	// Update storage
	if err := ct.updateContactInStorage(pubKey, peerID, addrs); err != nil {
		return fmt.Errorf("failed to update contact in storage: %w", err)
	}

	return nil
}

// SetContactOffline marks a contact as offline
func (ct *ContactTracker) SetContactOffline(pubKey []byte) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	pubKeyHex := hex.EncodeToString(pubKey)
	if info, exists := ct.contactPeers[pubKeyHex]; exists {
		info.IsOnline = false
	}
}

// SetConnected marks whether we're connected to a contact's peer
func (ct *ContactTracker) SetConnected(pubKey []byte, connected bool) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	pubKeyHex := hex.EncodeToString(pubKey)
	if info, exists := ct.contactPeers[pubKeyHex]; exists {
		info.ConnectedByUs = connected
		if connected {
			info.LastSeen = time.Now()
		}
	}
}

// RefreshOnlineStatus checks the online status of all contacts with known PeerIDs
func (ct *ContactTracker) RefreshOnlineStatus() error {
	if ct.ipfsNode == nil || !ct.ipfsNode.IsStarted() {
		return fmt.Errorf("IPFS node not started")
	}

	ct.mu.Lock()
	contacts := make([]*ContactPeerInfo, 0, len(ct.contactPeers))
	for _, info := range ct.contactPeers {
		if info.PeerID != "" {
			contacts = append(contacts, info)
		}
	}
	ct.mu.Unlock()

	for _, info := range contacts {
		// Try to find peer in DHT
		peerInfo, err := ct.ipfsNode.FindPeer(info.PeerID)
		if err == nil && len(peerInfo.Addrs) > 0 {
			// Peer found - update info
			ct.mu.Lock()
			info.IsOnline = true
			info.LastSeen = time.Now()
			info.Multiaddrs = peerInfo.Addrs
			ct.mu.Unlock()

			// Update storage
			if err := ct.updateContactInStorage(info.Ed25519PubKey, info.PeerID, peerInfo.Addrs); err != nil {
				logger.Debugw("failed to update contact in storage", "peer", info.PeerID, "error", err)
			}
		} else {
			// Peer not found - mark as offline
			ct.mu.Lock()
			info.IsOnline = false
			ct.mu.Unlock()
		}
	}

	ct.mu.Lock()
	ct.lastRefresh = time.Now()
	ct.mu.Unlock()

	return nil
}

// updateContactInStorage updates the contact record with peer information
func (ct *ContactTracker) updateContactInStorage(pubKey []byte, peerID string, addrs []multiaddr.Multiaddr) error {
	contact, err := ct.storage.GetContact(pubKey)
	if err != nil {
		return err
	}
	if contact == nil {
		return nil // Contact doesn't exist, nothing to update
	}

	// Update contact fields
	contact.PeerId = peerID
	contact.LastSeen = uint64(time.Now().Unix())

	// Update multiaddrs
	addrStrs := make([]string, len(addrs))
	for i, addr := range addrs {
		addrStrs[i] = addr.String()
	}
	contact.Multiaddrs = addrStrs

	return ct.storage.AddContact(contact)
}

// GetOnlineContacts returns all contacts that are currently online
func (ct *ContactTracker) GetOnlineContacts() []*ContactPeerInfo {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	online := make([]*ContactPeerInfo, 0)
	for _, info := range ct.contactPeers {
		if info.IsOnline {
			infoCopy := *info
			if len(info.Multiaddrs) > 0 {
				infoCopy.Multiaddrs = make([]multiaddr.Multiaddr, len(info.Multiaddrs))
				copy(infoCopy.Multiaddrs, info.Multiaddrs)
			}
			online = append(online, &infoCopy)
		}
	}
	return online
}

// GetContactByPeerID finds a contact by their PeerID
func (ct *ContactTracker) GetContactByPeerID(peerID string) (*ContactPeerInfo, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	for _, info := range ct.contactPeers {
		if info.PeerID == peerID {
			infoCopy := *info
			if len(info.Multiaddrs) > 0 {
				infoCopy.Multiaddrs = make([]multiaddr.Multiaddr, len(info.Multiaddrs))
				copy(infoCopy.Multiaddrs, info.Multiaddrs)
			}
			return &infoCopy, true
		}
	}
	return nil, false
}

// StartPeriodicRefresh starts a goroutine that periodically refreshes contact status
func (ct *ContactTracker) StartPeriodicRefresh(ctx context.Context) {
	ticker := time.NewTicker(ct.refreshInterval)
	bterrors.SafeGo("contact-tracker-refresh", func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := ct.RefreshOnlineStatus(); err != nil {
					logger.Debugw("failed to refresh contact status", "error", err)
				}
			}
		}
	})
}

// GetStats returns statistics about tracked contacts
func (ct *ContactTracker) GetStats() ContactTrackerStats {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	stats := ContactTrackerStats{
		TotalContacts:   len(ct.contactPeers),
		OnlineContacts:  0,
		ConnectedPeers:  0,
		WithPeerID:      0,
		LastRefresh:     ct.lastRefresh,
	}

	for _, info := range ct.contactPeers {
		if info.IsOnline {
			stats.OnlineContacts++
		}
		if info.ConnectedByUs {
			stats.ConnectedPeers++
		}
		if info.PeerID != "" {
			stats.WithPeerID++
		}
	}

	return stats
}

// ContactTrackerStats contains statistics about contact tracking
type ContactTrackerStats struct {
	TotalContacts   int
	OnlineContacts  int
	ConnectedPeers  int
	WithPeerID      int
	LastRefresh     time.Time
}
