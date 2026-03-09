package app

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"

	pb "babylontower/pkg/proto"

	"github.com/mr-tron/base58"
)

// ContactInfo contains UI-friendly contact information.
type ContactInfo struct {
	// PublicKeyBase58 is the Ed25519 public key in base58 format
	PublicKeyBase58 string
	// PublicKeyHex is the Ed25519 public key in hex format
	PublicKeyHex string
	// X25519KeyBase58 is the X25519 public key in base58 (empty if not set)
	X25519KeyBase58 string
	// DisplayName is the user-set nickname
	DisplayName string
	// HasEncryptionKey indicates whether the X25519 key is available
	HasEncryptionKey bool
	// IsOnline indicates whether the contact is currently online
	IsOnline bool
	// Connected indicates whether we have a direct connection
	Connected bool
	// PeerID is the libp2p peer ID (empty if unknown)
	PeerID string
	// CreatedAt is when the contact was added
	CreatedAt time.Time
	// ContactLink is the btower:// link for this contact
	ContactLink string
}

// ContactManager provides high-level contact management for UI.
type ContactManager interface {
	// AddContact adds a contact by public key string (hex or base58).
	// x25519PubKeyStr is optional — pass empty string if unknown.
	AddContact(pubKeyStr, displayName, x25519PubKeyStr string) (*ContactInfo, error)

	// AddContactFromLink adds a contact from a btower:// exchange link.
	AddContactFromLink(link string) (*ContactInfo, error)

	// RemoveContact removes a contact by public key string (hex or base58).
	RemoveContact(pubKeyStr string) error

	// GetContact returns contact info by public key string.
	GetContact(pubKeyStr string) (*ContactInfo, error)

	// ListContacts returns all contacts with their current status.
	ListContacts() ([]*ContactInfo, error)

	// UpdateContactName updates a contact's display name.
	UpdateContactName(pubKeyStr, newName string) error

	// FindAndConnect discovers a contact via DHT and attempts connection.
	// Returns updated contact info with peer connection details.
	FindAndConnect(pubKeyStr string) (*ContactInfo, error)
}

// contactManager implements ContactManager.
type contactManager struct {
	app *application
}

func (cm *contactManager) AddContact(pubKeyStr, displayName, x25519PubKeyStr string) (*ContactInfo, error) {
	pubKey, err := decodePubKey(pubKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	if len(pubKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key length: expected %d bytes, got %d", ed25519.PublicKeySize, len(pubKey))
	}

	// Check for self-add
	if hex.EncodeToString(pubKey) == hex.EncodeToString(cm.app.identity.Ed25519PubKey) {
		return nil, fmt.Errorf("cannot add yourself as a contact")
	}

	// Check if already exists
	existing, err := cm.app.storage.GetContact(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing contact: %w", err)
	}
	if existing != nil {
		return contactInfoFromProto(existing), nil
	}

	// Parse optional X25519 key
	var x25519Key []byte
	if x25519PubKeyStr != "" {
		x25519Key, err = decodePubKey(x25519PubKeyStr)
		if err != nil {
			return nil, fmt.Errorf("invalid X25519 public key: %w", err)
		}
		if len(x25519Key) != 32 {
			return nil, fmt.Errorf("invalid X25519 key length: expected 32 bytes, got %d", len(x25519Key))
		}
	}

	contact := &pb.Contact{
		PublicKey:       pubKey,
		DisplayName:     displayName,
		CreatedAt:       uint64(time.Now().Unix()),
		X25519PublicKey: x25519Key,
	}

	if err := cm.app.storage.AddContact(contact); err != nil {
		return nil, fmt.Errorf("failed to add contact: %w", err)
	}

	return contactInfoFromProto(contact), nil
}

func (cm *contactManager) AddContactFromLink(link string) (*ContactInfo, error) {
	parsed, err := ParseContactLink(link)
	if err != nil {
		return nil, err
	}
	return cm.AddContact(parsed.PublicKeyBase58, parsed.DisplayName, parsed.X25519KeyBase58)
}

func (cm *contactManager) RemoveContact(pubKeyStr string) error {
	pubKey, err := decodePubKey(pubKeyStr)
	if err != nil {
		return fmt.Errorf("invalid public key: %w", err)
	}
	return cm.app.storage.DeleteContact(pubKey)
}

func (cm *contactManager) GetContact(pubKeyStr string) (*ContactInfo, error) {
	pubKey, err := decodePubKey(pubKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	contact, err := cm.app.storage.GetContact(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get contact: %w", err)
	}
	if contact == nil {
		return nil, fmt.Errorf("contact not found")
	}

	info := contactInfoFromProto(contact)

	// Enrich with online status from messaging service
	if cm.app.messaging != nil && cm.app.messaging.IsStarted() {
		status, err := cm.app.messaging.GetContactStatus(pubKey)
		if err == nil {
			info.IsOnline = status.IsOnline
			info.Connected = status.Connected
			info.PeerID = status.PeerID
		}
	}

	return info, nil
}

func (cm *contactManager) ListContacts() ([]*ContactInfo, error) {
	contacts, err := cm.app.storage.ListContacts()
	if err != nil {
		return nil, fmt.Errorf("failed to list contacts: %w", err)
	}

	// Get all contact statuses in one call
	var statusMap map[string]statusInfo
	if cm.app.messaging != nil && cm.app.messaging.IsStarted() {
		statuses, err := cm.app.messaging.GetAllContactStatuses()
		if err == nil {
			statusMap = make(map[string]statusInfo, len(statuses))
			for _, s := range statuses {
				key := hex.EncodeToString(s.PubKey)
				statusMap[key] = statusInfo{
					isOnline:  s.IsOnline,
					connected: s.Connected,
					peerID:    s.PeerID,
				}
			}
		}
	}

	result := make([]*ContactInfo, 0, len(contacts))
	for _, c := range contacts {
		info := contactInfoFromProto(c)
		if statusMap != nil {
			if s, ok := statusMap[info.PublicKeyHex]; ok {
				info.IsOnline = s.isOnline
				info.Connected = s.connected
				info.PeerID = s.peerID
			}
		}
		result = append(result, info)
	}
	return result, nil
}

func (cm *contactManager) UpdateContactName(pubKeyStr, newName string) error {
	pubKey, err := decodePubKey(pubKeyStr)
	if err != nil {
		return fmt.Errorf("invalid public key: %w", err)
	}

	contact, err := cm.app.storage.GetContact(pubKey)
	if err != nil {
		return fmt.Errorf("failed to get contact: %w", err)
	}
	if contact == nil {
		return fmt.Errorf("contact not found")
	}

	contact.DisplayName = newName

	// Delete and re-add to update (storage has no Update method)
	if err := cm.app.storage.DeleteContact(pubKey); err != nil {
		return fmt.Errorf("failed to update contact: %w", err)
	}
	if err := cm.app.storage.AddContact(contact); err != nil {
		return fmt.Errorf("failed to update contact: %w", err)
	}

	return nil
}

func (cm *contactManager) FindAndConnect(pubKeyStr string) (*ContactInfo, error) {
	pubKey, err := decodePubKey(pubKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	if cm.app.messaging == nil || !cm.app.messaging.IsStarted() {
		return nil, fmt.Errorf("messaging service not started")
	}

	result, err := cm.app.messaging.FindAndConnectToContact(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to find contact: %w", err)
	}

	// Get updated contact info
	contact, err := cm.app.storage.GetContact(pubKey)
	if err != nil || contact == nil {
		// Return basic info from discovery result
		return &ContactInfo{
			PublicKeyBase58: base58.Encode(pubKey),
			PublicKeyHex:    hex.EncodeToString(pubKey),
			PeerID:          result.PeerID,
			Connected:       true,
			IsOnline:        true,
		}, nil
	}

	info := contactInfoFromProto(contact)
	info.PeerID = result.PeerID
	info.Connected = true
	info.IsOnline = true
	return info, nil
}

// statusInfo is a helper for batch status lookup.
type statusInfo struct {
	isOnline  bool
	connected bool
	peerID    string
}

// decodePubKey decodes a public key from hex or base58.
func decodePubKey(s string) ([]byte, error) {
	// Try base58 first (more common in UI)
	if decoded, err := base58.Decode(s); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	// Try hex
	if decoded, err := hex.DecodeString(s); err == nil {
		return decoded, nil
	}
	return nil, fmt.Errorf("cannot decode key: not valid hex or base58")
}

// contactInfoFromProto converts a protobuf Contact to ContactInfo.
func contactInfoFromProto(c *pb.Contact) *ContactInfo {
	info := &ContactInfo{
		PublicKeyBase58:  base58.Encode(c.PublicKey),
		PublicKeyHex:     hex.EncodeToString(c.PublicKey),
		DisplayName:      c.DisplayName,
		HasEncryptionKey: len(c.X25519PublicKey) == 32,
		CreatedAt:        time.Unix(int64(c.CreatedAt), 0),
	}
	if len(c.X25519PublicKey) > 0 {
		info.X25519KeyBase58 = base58.Encode(c.X25519PublicKey)
	}
	info.ContactLink = GenerateContactLink(c.PublicKey, c.X25519PublicKey, c.DisplayName)
	return info
}
