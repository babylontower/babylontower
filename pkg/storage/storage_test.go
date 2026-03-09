package storage

import (
	"bytes"
	"os"
	"testing"
	"time"

	pb "babylontower/pkg/proto"
)

// testStorage creates an in-memory BadgerDB for testing
func testStorage(t *testing.T) *BadgerStorage {
	t.Helper()

	store, err := NewBadgerStorage(Config{InMemory: true})
	if err != nil {
		t.Fatalf("Failed to create test storage: %v", err)
	}

	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Failed to close storage: %v", err)
		}
	})

	return store
}

// createTestContact creates a test contact with the given public key
func createTestContact(pubKey []byte, displayName string) *pb.Contact {
	return &pb.Contact{
		PublicKey:   pubKey,
		DisplayName: displayName,
		CreatedAt:   uint64(time.Now().Unix()),
	}
}

// createTestMessage creates a test stored message
func createTestMessage(text string, senderPubKey []byte, isOutgoing bool) *StoredMessage {
	return &StoredMessage{
		Text:         text,
		Timestamp:    uint64(time.Now().Unix()),
		SenderPubKey: senderPubKey,
		IsOutgoing:   isOutgoing,
	}
}

func TestAddContact(t *testing.T) {
	store := testStorage(t)

	pubKey := []byte("test_public_key_32_bytes_longggg")
	contact := createTestContact(pubKey, "Test Contact")

	err := store.AddContact(contact)
	if err != nil {
		t.Fatalf("AddContact failed: %v", err)
	}
}

func TestGetContact(t *testing.T) {
	store := testStorage(t)

	pubKey := []byte("test_public_key_32_bytes_longggg")
	contact := createTestContact(pubKey, "Test Contact")

	// Add contact
	err := store.AddContact(contact)
	if err != nil {
		t.Fatalf("AddContact failed: %v", err)
	}

	// Get contact
	retrieved, err := store.GetContact(pubKey)
	if err != nil {
		t.Fatalf("GetContact failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetContact returned nil")
	}

	if !bytes.Equal(retrieved.PublicKey, pubKey) {
		t.Errorf("PublicKey mismatch: got %v, want %v", retrieved.PublicKey, pubKey)
	}

	if retrieved.DisplayName != "Test Contact" {
		t.Errorf("DisplayName mismatch: got %q, want %q", retrieved.DisplayName, "Test Contact")
	}
}

func TestGetContactNotFound(t *testing.T) {
	store := testStorage(t)

	pubKey := []byte("nonexistent_key_32_bytes_longg")

	contact, err := store.GetContact(pubKey)
	if err != nil {
		t.Fatalf("GetContact returned error: %v", err)
	}

	if contact != nil {
		t.Error("GetContact should return nil for non-existent contact")
	}
}

func TestGetContactByBase58(t *testing.T) {
	store := testStorage(t)

	pubKey := []byte("test_public_key_32_bytes_longggg")
	contact := createTestContact(pubKey, "Test Contact")

	// Add contact
	err := store.AddContact(contact)
	if err != nil {
		t.Fatalf("AddContact failed: %v", err)
	}

	// Get contact by base58
	pubKeyBase58 := ContactKeyToBase58(pubKey)
	retrieved, err := store.GetContactByBase58(pubKeyBase58)
	if err != nil {
		t.Fatalf("GetContactByBase58 failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetContactByBase58 returned nil")
	}

	if !bytes.Equal(retrieved.PublicKey, pubKey) {
		t.Errorf("PublicKey mismatch: got %v, want %v", retrieved.PublicKey, pubKey)
	}
}

func TestListContacts(t *testing.T) {
	store := testStorage(t)

	// Add multiple contacts
	contacts := []*pb.Contact{
		createTestContact([]byte("contact1_key_32_bytes_longgg"), "Contact 1"),
		createTestContact([]byte("contact2_key_32_bytes_longgg"), "Contact 2"),
		createTestContact([]byte("contact3_key_32_bytes_longgg"), "Contact 3"),
	}

	for _, c := range contacts {
		if err := store.AddContact(c); err != nil {
			t.Fatalf("AddContact failed: %v", err)
		}
	}

	// List contacts
	retrieved, err := store.ListContacts()
	if err != nil {
		t.Fatalf("ListContacts failed: %v", err)
	}

	if len(retrieved) != len(contacts) {
		t.Errorf("ListContacts returned %d contacts, want %d", len(retrieved), len(contacts))
	}
}

func TestDeleteContact(t *testing.T) {
	store := testStorage(t)

	pubKey := []byte("test_public_key_32_bytes_longggg")
	contact := createTestContact(pubKey, "Test Contact")

	// Add contact
	err := store.AddContact(contact)
	if err != nil {
		t.Fatalf("AddContact failed: %v", err)
	}

	// Delete contact
	err = store.DeleteContact(pubKey)
	if err != nil {
		t.Fatalf("DeleteContact failed: %v", err)
	}

	// Verify contact is deleted
	retrieved, err := store.GetContact(pubKey)
	if err != nil {
		t.Fatalf("GetContact failed: %v", err)
	}

	if retrieved != nil {
		t.Error("GetContact should return nil after deletion")
	}
}

func TestAddMessage(t *testing.T) {
	store := testStorage(t)

	contactPubKey := []byte("contact_pub_key_32_bytes_longg")
	msg := createTestMessage("Hello, World!", []byte("sender_pub_key_32_bytes_longgg"), false)

	err := store.AddMessage(contactPubKey, msg)
	if err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}
}

func TestGetMessages(t *testing.T) {
	store := testStorage(t)

	contactPubKey := []byte("contact_pub_key_32_bytes_longg")
	senderPubKey := []byte("sender_pub_key_32_bytes_longgg")

	// Add multiple messages
	texts := []string{"Message 1", "Message 2", "Message 3"}
	for _, text := range texts {
		msg := createTestMessage(text, senderPubKey, false)
		if err := store.AddMessage(contactPubKey, msg); err != nil {
			t.Fatalf("AddMessage failed: %v", err)
		}
	}

	// Get all messages
	retrieved, err := store.GetMessages(contactPubKey, 0, 0)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(retrieved) != len(texts) {
		t.Errorf("GetMessages returned %d messages, want %d", len(retrieved), len(texts))
	}
}

func TestGetMessagesWithLimit(t *testing.T) {
	store := testStorage(t)

	contactPubKey := []byte("contact_pub_key_32_bytes_longg")
	senderPubKey := []byte("sender_pub_key_32_bytes_longgg")

	// Add multiple messages
	for i := 0; i < 5; i++ {
		msg := createTestMessage("Message", senderPubKey, false)
		msg.Timestamp = uint64(time.Now().Unix()) + uint64(i)
		if err := store.AddMessage(contactPubKey, msg); err != nil {
			t.Fatalf("AddMessage failed: %v", err)
		}
	}

	// Get messages with limit
	retrieved, err := store.GetMessages(contactPubKey, 2, 0)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(retrieved) != 2 {
		t.Errorf("GetMessages with limit returned %d messages, want 2", len(retrieved))
	}
}

func TestGetMessagesWithOffset(t *testing.T) {
	store := testStorage(t)

	contactPubKey := []byte("contact_pub_key_32_bytes_longg")
	senderPubKey := []byte("sender_pub_key_32_bytes_longgg")

	// Add multiple messages
	for i := 0; i < 5; i++ {
		msg := createTestMessage("Message", senderPubKey, false)
		msg.Timestamp = uint64(time.Now().Unix()) + uint64(i)
		if err := store.AddMessage(contactPubKey, msg); err != nil {
			t.Fatalf("AddMessage failed: %v", err)
		}
	}

	// Get messages with offset
	retrieved, err := store.GetMessages(contactPubKey, 0, 2)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(retrieved) != 3 {
		t.Errorf("GetMessages with offset returned %d messages, want 3", len(retrieved))
	}
}

func TestGetMessagesEmptyContact(t *testing.T) {
	store := testStorage(t)

	contactPubKey := []byte("nonexistent_contact_key_longg")

	retrieved, err := store.GetMessages(contactPubKey, 0, 0)
	if err != nil {
		t.Fatalf("GetMessages returned error: %v", err)
	}

	if len(retrieved) != 0 {
		t.Errorf("GetMessages returned %d messages, want 0", len(retrieved))
	}
}

func TestDeleteMessages(t *testing.T) {
	store := testStorage(t)

	contactPubKey := []byte("contact_pub_key_32_bytes_longg")
	senderPubKey := []byte("sender_pub_key_32_bytes_longgg")

	// Add messages
	for i := 0; i < 3; i++ {
		msg := createTestMessage("Message", senderPubKey, false)
		msg.Timestamp = uint64(time.Now().Unix()) + uint64(i)
		if err := store.AddMessage(contactPubKey, msg); err != nil {
			t.Fatalf("AddMessage failed: %v", err)
		}
	}

	// Delete messages
	err := store.DeleteMessages(contactPubKey)
	if err != nil {
		t.Fatalf("DeleteMessages failed: %v", err)
	}

	// Verify messages are deleted
	retrieved, err := store.GetMessages(contactPubKey, 0, 0)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(retrieved) != 0 {
		t.Errorf("GetMessages returned %d messages after deletion, want 0", len(retrieved))
	}
}

func TestMessageOrdering(t *testing.T) {
	store := testStorage(t)

	contactPubKey := []byte("contact_pub_key_32_bytes_longg")
	senderPubKey := []byte("sender_pub_key_32_bytes_longgg")

	baseTime := uint64(time.Now().Unix())

	// Add messages with explicit timestamps to ensure ordering
	for i := 0; i < 5; i++ {
		msg := &StoredMessage{
			Text:         "Message",
			Timestamp:    baseTime + uint64(i),
			SenderPubKey: senderPubKey,
			IsOutgoing:   false,
		}
		if err := store.AddMessage(contactPubKey, msg); err != nil {
			t.Fatalf("AddMessage failed: %v", err)
		}
	}

	// Get messages - they should be ordered by timestamp
	retrieved, err := store.GetMessages(contactPubKey, 0, 0)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(retrieved) != 5 {
		t.Errorf("GetMessages returned %d messages, want 5", len(retrieved))
	}

	// Verify ordering
	for i := 1; i < len(retrieved); i++ {
		if retrieved[i].Timestamp < retrieved[i-1].Timestamp {
			t.Errorf("messages not ordered: timestamp %d < %d at index %d",
				retrieved[i].Timestamp, retrieved[i-1].Timestamp, i)
		}
	}
}

func TestStorageClose(t *testing.T) {
	store, err := NewBadgerStorage(Config{InMemory: true})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	err = store.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestContactKeyConversion(t *testing.T) {
	pubKey := []byte("test_public_key_32_bytes_longggg")

	base58Str := ContactKeyToBase58(pubKey)
	decoded, err := ContactKeyFromBase58(base58Str)
	if err != nil {
		t.Fatalf("ContactKeyFromBase58 failed: %v", err)
	}

	if !bytes.Equal(decoded, pubKey) {
		t.Error("Decoded key doesn't match original")
	}
}

func TestInvalidBase58Key(t *testing.T) {
	_, err := ContactKeyFromBase58("invalid_base58!!!")
	if err == nil {
		t.Error("Expected error for invalid base58 string")
	}
}

// TestPersistence tests that data persists across storage restarts
// This test uses a temporary directory instead of in-memory storage
func TestPersistence(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "badger-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	pubKey := []byte("test_public_key_32_bytes_longggg")
	contact := createTestContact(pubKey, "Persistent Contact")

	// Create storage and add contact
	store1, err := NewBadgerStorage(Config{Path: tmpDir})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	if err := store1.AddContact(contact); err != nil {
		_ = store1.Close()
		t.Fatalf("AddContact failed: %v", err)
	}

	if err := store1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Reopen storage and verify contact persists
	store2, err := NewBadgerStorage(Config{Path: tmpDir})
	if err != nil {
		t.Fatalf("Failed to reopen storage: %v", err)
	}
	defer func() {
		_ = store2.Close()
	}()

	retrieved, err := store2.GetContact(pubKey)
	if err != nil {
		t.Fatalf("GetContact failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Contact did not persist across storage restart")
	}

	if retrieved.DisplayName != "Persistent Contact" {
		t.Errorf("DisplayName mismatch after restart: got %q, want %q", retrieved.DisplayName, "Persistent Contact")
	}
}
