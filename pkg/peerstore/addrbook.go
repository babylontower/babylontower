// Package peerstore provides peer address book functionality for persistent peer storage.
// It stores peer IDs, multiaddresses, and identity public keys for later connection.
package peerstore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-multiaddr"

	"babylontower/pkg/ipfsnode"
)

var logger = log.Logger("babylontower/peerstore")

const (
	// AddrBookFile is the filename for the peer address book
	AddrBookFile = "peers.json"
	// MaxPeers is the maximum number of peers to store in the address book
	MaxPeers = 1000
)

// PeerRecord contains information about a known peer
type PeerRecord struct {
	PublicKey  string   `json:"public_key"`  // Identity public key (hex encoded)
	PeerID     string   `json:"peer_id"`     // libp2p PeerID
	Addresses  []string `json:"addresses"`   // Multiaddresses
	LastSeen   int64    `json:"last_seen"`   // Unix timestamp
	Connected  bool     `json:"connected"`   // Currently connected
}

// AddrBook manages a persistent store of peer addresses
type AddrBook struct {
	repoDir string
	file    string
	mu      sync.RWMutex
	peers   map[string]*PeerRecord // key: hex(public_key)
}

// NewAddrBook creates a new address book
func NewAddrBook(repoDir string) (*AddrBook, error) {
	ab := &AddrBook{
		repoDir: repoDir,
		file:    filepath.Join(repoDir, AddrBookFile),
		peers:   make(map[string]*PeerRecord),
	}

	// Load existing address book if present
	if err := ab.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load address book: %w", err)
	}

	return ab, nil
}

// load reads the address book from disk
func (ab *AddrBook) load() error {
	data, err := os.ReadFile(ab.file)
	if err != nil {
		return err
	}

	var peers []*PeerRecord
	if err := json.Unmarshal(data, &peers); err != nil {
		return fmt.Errorf("failed to parse address book: %w", err)
	}

	ab.mu.Lock()
	defer ab.mu.Unlock()

	ab.peers = make(map[string]*PeerRecord, len(peers))
	for _, p := range peers {
		ab.peers[p.PublicKey] = p
	}

	return nil
}

// save writes the address book to disk
func (ab *AddrBook) save() error {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	// Ensure directory exists
	if err := os.MkdirAll(ab.repoDir, 0700); err != nil {
		return fmt.Errorf("failed to create repo directory: %w", err)
	}

	// Convert map to slice for JSON
	peers := make([]*PeerRecord, 0, len(ab.peers))
	for _, p := range ab.peers {
		peers = append(peers, p)
	}

	data, err := json.MarshalIndent(peers, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal address book: %w", err)
	}

	if err := os.WriteFile(ab.file, data, 0600); err != nil {
		return fmt.Errorf("failed to write address book: %w", err)
	}

	return nil
}

// AddContact adds or updates a contact in the address book
func (ab *AddrBook) AddContact(pubKey []byte, peerID string, addrs []multiaddr.Multiaddr) error {
	pubKeyHex := hex.EncodeToString(pubKey)

	ab.mu.Lock()
	defer ab.mu.Unlock()

	// Convert addresses to strings
	addrStrs := make([]string, len(addrs))
	for i, addr := range addrs {
		addrStrs[i] = addr.String()
	}

	// Check if we already have this peer
	if existing, ok := ab.peers[pubKeyHex]; ok {
		// Update existing record
		existing.PeerID = peerID
		existing.Addresses = addrStrs
		existing.LastSeen = time.Now().Unix()
	} else {
		// Create new record
		ab.peers[pubKeyHex] = &PeerRecord{
			PublicKey: pubKeyHex,
			PeerID:    peerID,
			Addresses: addrStrs,
			LastSeen:  time.Now().Unix(),
			Connected: false,
		}

		// Enforce max peers limit
		if len(ab.peers) > MaxPeers {
			ab.removeOldestPeer()
		}
	}

	// Save to disk
	if err := ab.save(); err != nil {
		return fmt.Errorf("failed to save address book: %w", err)
	}

	return nil
}

// GetContact retrieves a contact by public key
func (ab *AddrBook) GetContact(pubKey []byte) (*PeerRecord, error) {
	pubKeyHex := hex.EncodeToString(pubKey)

	ab.mu.RLock()
	defer ab.mu.RUnlock()

	record, ok := ab.peers[pubKeyHex]
	if !ok {
		return nil, ErrPeerNotFound
	}

	// Return a copy to avoid race conditions
	recordCopy := *record
	return &recordCopy, nil
}

// GetContactByPeerID retrieves a contact by PeerID
func (ab *AddrBook) GetContactByPeerID(peerID string) (*PeerRecord, error) {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	for _, record := range ab.peers {
		if record.PeerID == peerID {
			recordCopy := *record
			return &recordCopy, nil
		}
	}

	return nil, ErrPeerNotFound
}

// UpdateAddresses updates the addresses for a known peer
func (ab *AddrBook) UpdateAddresses(pubKey []byte, addrs []multiaddr.Multiaddr) error {
	pubKeyHex := hex.EncodeToString(pubKey)

	ab.mu.Lock()
	defer ab.mu.Unlock()

	record, ok := ab.peers[pubKeyHex]
	if !ok {
		return ErrPeerNotFound
	}

	// Convert addresses to strings
	addrStrs := make([]string, len(addrs))
	for i, addr := range addrs {
		addrStrs[i] = addr.String()
	}

	record.Addresses = addrStrs
	record.LastSeen = time.Now().Unix()

	// Save to disk
	if err := ab.save(); err != nil {
		return fmt.Errorf("failed to save address book: %w", err)
	}

	return nil
}

// SetConnected updates the connected status for a peer
func (ab *AddrBook) SetConnected(pubKey []byte, connected bool) error {
	pubKeyHex := hex.EncodeToString(pubKey)

	ab.mu.Lock()
	defer ab.mu.Unlock()

	record, ok := ab.peers[pubKeyHex]
	if !ok {
		return ErrPeerNotFound
	}

	record.Connected = connected

	// Don't save on every connection change - too frequent
	// Will be saved on next AddContact or periodic save
	return nil
}

// GetAllContacts returns all contacts in the address book
func (ab *AddrBook) GetAllContacts() ([]*PeerRecord, error) {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	contacts := make([]*PeerRecord, 0, len(ab.peers))
	for _, record := range ab.peers {
		recordCopy := *record
		contacts = append(contacts, &recordCopy)
	}

	return contacts, nil
}

// ConnectToAll attempts to connect to all known peers
func (ab *AddrBook) ConnectToAll(ctx context.Context, node *ipfsnode.Node) error {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	connected := 0
	failed := 0

	for _, record := range ab.peers {
		if len(record.Addresses) == 0 {
			continue
		}

		// Try first address
		addr := record.Addresses[0]
		if err := node.ConnectToPeer(addr); err != nil {
			failed++
			continue
		}

		record.Connected = true
		connected++
	}

	if connected > 0 {
		// Save connection status
		if err := ab.save(); err != nil {
			return fmt.Errorf("failed to save address book: %w", err)
		}
	}

	return nil
}

// removeOldestPeer removes the peer with the oldest last_seen timestamp
// Must be called with lock held
func (ab *AddrBook) removeOldestPeer() {
	var oldestKey string
	var oldestTime int64

	for key, record := range ab.peers {
		if oldestKey == "" || record.LastSeen < oldestTime {
			oldestKey = key
			oldestTime = record.LastSeen
		}
	}

	if oldestKey != "" {
		delete(ab.peers, oldestKey)
	}
}

// DeleteContact removes a contact from the address book
func (ab *AddrBook) DeleteContact(pubKey []byte) error {
	pubKeyHex := hex.EncodeToString(pubKey)

	ab.mu.Lock()
	defer ab.mu.Unlock()

	if _, ok := ab.peers[pubKeyHex]; !ok {
		return ErrPeerNotFound
	}

	delete(ab.peers, pubKeyHex)

	if err := ab.save(); err != nil {
		return fmt.Errorf("failed to save address book: %w", err)
	}

	return nil
}

// Count returns the number of peers in the address book
func (ab *AddrBook) Count() int {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	return len(ab.peers)
}

// Errors
var (
	ErrPeerNotFound = fmt.Errorf("peer not found in address book")
)
