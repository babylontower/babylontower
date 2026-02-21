package storage

import (
	"fmt"
	"sync"

	pb "babylontower/pkg/proto"
	"google.golang.org/protobuf/proto"
)

// MemoryStorage is an in-memory implementation of Storage for testing
type MemoryStorage struct {
	mu       sync.RWMutex
	contacts map[string]*pb.Contact
	messages map[string][]*pb.SignedEnvelope
}

// NewMemoryStorage creates a new in-memory storage
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		contacts: make(map[string]*pb.Contact),
		messages: make(map[string][]*pb.SignedEnvelope),
	}
}

// AddContact stores a contact in memory
func (s *MemoryStorage) AddContact(contact *pb.Contact) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(contact.PublicKey)
	s.contacts[key] = contact
	return nil
}

// GetContact retrieves a contact by public key
func (s *MemoryStorage) GetContact(pubKey []byte) (*pb.Contact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	contact, ok := s.contacts[string(pubKey)]
	if !ok {
		return nil, nil
	}
	return contact, nil
}

// GetContactByBase58 retrieves a contact by base58-encoded public key
func (s *MemoryStorage) GetContactByBase58(pubKeyBase58 string) (*pb.Contact, error) {
	pubKey, err := ContactKeyFromBase58(pubKeyBase58)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base58 public key: %w", err)
	}
	return s.GetContact(pubKey)
}

// ListContacts returns all contacts
func (s *MemoryStorage) ListContacts() ([]*pb.Contact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	contacts := make([]*pb.Contact, 0, len(s.contacts))
	for _, contact := range s.contacts {
		contacts = append(contacts, contact)
	}
	return contacts, nil
}

// DeleteContact removes a contact from memory
func (s *MemoryStorage) DeleteContact(pubKey []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(pubKey)
	delete(s.contacts, key)
	return nil
}

// AddMessage stores a message for a contact
func (s *MemoryStorage) AddMessage(contactPubKey []byte, envelope *pb.SignedEnvelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages[string(contactPubKey)] = append(s.messages[string(contactPubKey)], envelope)
	return nil
}

// GetMessages retrieves messages for a contact
// limit specifies maximum number of messages (0 = no limit)
// offset specifies number of messages to skip
func (s *MemoryStorage) GetMessages(contactPubKey []byte, limit, offset int) ([]*pb.SignedEnvelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allMessages, ok := s.messages[string(contactPubKey)]
	if !ok {
		return []*pb.SignedEnvelope{}, nil
	}

	// Apply offset
	if offset >= len(allMessages) {
		return []*pb.SignedEnvelope{}, nil
	}
	start := offset

	// Apply limit
	end := len(allMessages)
	if limit > 0 && start+limit < end {
		end = start + limit
	}

	result := make([]*pb.SignedEnvelope, end-start)
	copy(result, allMessages[start:end])
	return result, nil
}

// DeleteMessages removes all messages for a contact
func (s *MemoryStorage) DeleteMessages(contactPubKey []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(contactPubKey)
	delete(s.messages, key)
	return nil
}

// Close is a no-op for in-memory storage
func (s *MemoryStorage) Close() error {
	return nil
}

// Clear removes all data from storage
func (s *MemoryStorage) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contacts = make(map[string]*pb.Contact)
	s.messages = make(map[string][]*pb.SignedEnvelope)
}

// ContactCount returns the number of contacts
func (s *MemoryStorage) ContactCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.contacts)
}

// MessageCount returns the number of messages for a contact
func (s *MemoryStorage) MessageCount(contactPubKey []byte) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages[string(contactPubKey)])
}

// Clone creates a deep copy of the storage (for testing)
func (s *MemoryStorage) Clone() *MemoryStorage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clone := &MemoryStorage{
		contacts: make(map[string]*pb.Contact),
		messages: make(map[string][]*pb.SignedEnvelope),
	}

	for k, v := range s.contacts {
		clone.contacts[k] = proto.Clone(v).(*pb.Contact)
	}

	for k, msgs := range s.messages {
		clone.messages[k] = make([]*pb.SignedEnvelope, len(msgs))
		for i, msg := range msgs {
			clone.messages[k][i] = proto.Clone(msg).(*pb.SignedEnvelope)
		}
	}

	return clone
}
