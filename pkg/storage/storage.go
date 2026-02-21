package storage

import (
	"github.com/mr-tron/base58"
	pb "babylontower/pkg/proto"
)

// Storage defines the interface for persistent storage
type Storage interface {
	// Contact operations
	AddContact(contact *pb.Contact) error
	GetContact(pubKey []byte) (*pb.Contact, error)
	GetContactByBase58(pubKeyBase58 string) (*pb.Contact, error)
	ListContacts() ([]*pb.Contact, error)
	DeleteContact(pubKey []byte) error

	// Message operations
	AddMessage(contactPubKey []byte, envelope *pb.SignedEnvelope) error
	GetMessages(contactPubKey []byte, limit, offset int) ([]*pb.SignedEnvelope, error)
	DeleteMessages(contactPubKey []byte) error

	// Lifecycle
	Close() error
}

// ContactKeyToBase58 converts a public key to base58 string for display
func ContactKeyToBase58(pubKey []byte) string {
	return base58.Encode(pubKey)
}

// ContactKeyFromBase58 converts a base58 string back to public key bytes
func ContactKeyFromBase58(pubKeyBase58 string) ([]byte, error) {
	return base58.Decode(pubKeyBase58)
}
