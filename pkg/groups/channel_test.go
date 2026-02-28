package groups

import (
	"testing"

	"babylontower/pkg/storage"
	"golang.org/x/crypto/ed25519"
)

func setupTestChannelService(t *testing.T) (*ChannelService, ed25519.PublicKey, ed25519.PrivateKey) {
	// Generate identity keys
	pubKey, privKey, _ := ed25519.GenerateKey(nil)

	// Create storage
	stor := storage.NewMemoryStorage()

	// Create service
	service := NewChannelService(stor, pubKey, privKey)

	return service, pubKey, privKey
}

func TestCreatePrivateChannel(t *testing.T) {
	service, _, _ := setupTestChannelService(t)

	channel, err := service.CreateChannel("Test Channel", "A test private channel", PrivateChannel)
	if err != nil {
		t.Fatalf("Failed to create private channel: %v", err)
	}

	if channel.Type != PrivateChannel {
		t.Errorf("Expected channel type PrivateChannel, got %v", channel.Type)
	}

	if channel.Name != "Test Channel" {
		t.Errorf("Expected name 'Test Channel', got '%s'", channel.Name)
	}
}

func TestCreatePublicChannel(t *testing.T) {
	service, _, _ := setupTestChannelService(t)

	channel, err := service.CreateChannel("Public Channel", "A test public channel", PublicChannel)
	if err != nil {
		t.Fatalf("Failed to create public channel: %v", err)
	}

	if channel.Type != PublicChannel {
		t.Errorf("Expected channel type PublicChannel, got %v", channel.Type)
	}
}

func TestCreatePost(t *testing.T) {
	service, _, _ := setupTestChannelService(t)

	// Create channel
	channel, err := service.CreateChannel("Test Channel", "A test channel", PrivateChannel)
	if err != nil {
		t.Fatalf("Failed to create channel: %v", err)
	}

	// Create a post
	content := "Test post content"
	post, err := service.CreatePost(channel.ChannelID, content)
	if err != nil {
		t.Fatalf("Failed to create post: %v", err)
	}

	if post.ChannelID == nil {
		t.Error("Post channel ID is nil")
	}

	// First post has nil previous CID (no previous post)
	// This is expected behavior
}

func TestGetPosts(t *testing.T) {
	service, _, _ := setupTestChannelService(t)

	// Create channel
	channel, err := service.CreateChannel("Test Channel", "A test channel", PrivateChannel)
	if err != nil {
		t.Fatalf("Failed to create channel: %v", err)
	}

	// Create multiple posts
	for i := 0; i < 5; i++ {
		content := "Post content"
		_, err := service.CreatePost(channel.ChannelID, content)
		if err != nil {
			t.Fatalf("Failed to create post %d: %v", i, err)
		}
	}

	// Get posts
	posts, err := service.GetPosts(channel.ChannelID, 10, 0)
	if err != nil {
		t.Fatalf("Failed to get posts: %v", err)
	}

	if len(posts) != 5 {
		t.Errorf("Expected 5 posts, got %d", len(posts))
	}
}

func TestSubscribeUnsubscribe(t *testing.T) {
	service, _, _ := setupTestChannelService(t)

	// Create public channel
	channel, err := service.CreateChannel("Public Channel", "A public channel", PublicChannel)
	if err != nil {
		t.Fatalf("Failed to create channel: %v", err)
	}

	// Create subscriber
	subscriberPubKey, _, _ := ed25519.GenerateKey(nil)

	// Subscribe
	err = service.Subscribe(channel.ChannelID, subscriberPubKey)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Verify subscription
	if !service.IsSubscribed(channel.ChannelID, subscriberPubKey) {
		t.Error("Expected subscriber to be subscribed")
	}

	// Unsubscribe
	err = service.Unsubscribe(channel.ChannelID, subscriberPubKey)
	if err != nil {
		t.Fatalf("Failed to unsubscribe: %v", err)
	}

	// Verify unsubscription
	if service.IsSubscribed(channel.ChannelID, subscriberPubKey) {
		t.Error("Expected subscriber to be unsubscribed")
	}
}

func TestPrivateChannelPostAuthorization(t *testing.T) {
	service, _, _ := setupTestChannelService(t)

	// Create private channel
	channel, err := service.CreateChannel("Private Channel", "A private channel", PrivateChannel)
	if err != nil {
		t.Fatalf("Failed to create channel: %v", err)
	}

	// Try to create post as non-owner (simulate by changing identity)
	_, otherPrivKey, _ := ed25519.GenerateKey(nil)
	service.identitySignPriv = otherPrivKey
	service.identitySignPub = otherPrivKey.Public().(ed25519.PublicKey)

	_, err = service.CreatePost(channel.ChannelID, "Unauthorized post")
	if err != ErrNotChannelOwner {
		t.Errorf("Expected ErrNotChannelOwner, got %v", err)
	}

	// Note: In a real test, we would restore the original identity
	// For this test, we just skip the restoration
}

func TestChannelStateSignature(t *testing.T) {
	_, pubKey, privKey := setupTestChannelService(t)

	state := &ChannelState{
		ChannelID:   []byte("test-channel-id"),
		Name:        "Test Channel",
		Description: "Test description",
		Type:        PublicChannel,
		OwnerPubkey: pubKey,
	}

	// Sign the state
	err := state.Sign(privKey)
	if err != nil {
		t.Fatalf("Failed to sign state: %v", err)
	}

	// Verify the signature
	if !state.Verify(pubKey) {
		t.Error("Signature verification failed")
	}

	// Tamper with the state
	state.Name = "Tampered Channel"
	if state.Verify(pubKey) {
		t.Error("Signature should not verify after tampering")
	}
}

func TestChannelPostSignature(t *testing.T) {
	_, pubKey, privKey := setupTestChannelService(t)

	post := &ChannelPost{
		PostID:       []byte("test-post-id"),
		ChannelID:    []byte("test-channel-id"),
		AuthorPubkey: pubKey,
		Content:      "Test content",
	}

	// Sign the post
	err := post.Sign(privKey)
	if err != nil {
		t.Fatalf("Failed to sign post: %v", err)
	}

	// Verify the signature
	if !post.Verify(pubKey) {
		t.Error("Signature verification failed")
	}

	// Tamper with the post
	post.Content = "Tampered content"
	if post.Verify(pubKey) {
		t.Error("Signature should not verify after tampering")
	}
}

func TestChannelPersistence(t *testing.T) {
	stor := storage.NewMemoryStorage()
	pubKey, privKey, _ := ed25519.GenerateKey(nil)

	service := NewChannelService(stor, pubKey, privKey)

	// Create channel
	channel, err := service.CreateChannel("Persistent Channel", "A persistent channel", PublicChannel)
	if err != nil {
		t.Fatalf("Failed to create channel: %v", err)
	}

	// Retrieve from storage
	retrieved, err := stor.GetChannel(channel.ChannelID)
	if err != nil {
		t.Fatalf("Failed to retrieve channel: %v", err)
	}

	if string(retrieved.ChannelId) != string(channel.ChannelID) {
		t.Error("Retrieved channel ID doesn't match")
	}
}

func TestGetChannelTopic(t *testing.T) {
	channelID := []byte("test-channel-id-12345678901234567890")
	topic := GetChannelTopic(channelID)

	expectedPrefix := "babylon-ch-"
	if topic[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Expected topic prefix %s, got %s", expectedPrefix, topic)
	}
}

func TestListChannels(t *testing.T) {
	service, _, _ := setupTestChannelService(t)

	// Create multiple channels
	for i := 0; i < 3; i++ {
		_, err := service.CreateChannel("Channel", "Test channel", PublicChannel)
		if err != nil {
			t.Fatalf("Failed to create channel: %v", err)
		}
	}

	// List channels
	channels := service.ListChannels()
	if len(channels) != 3 {
		t.Errorf("Expected 3 channels, got %d", len(channels))
	}
}

func TestDeleteChannel(t *testing.T) {
	service, _, _ := setupTestChannelService(t)

	// Create channel
	channel, err := service.CreateChannel("Test Channel", "A test channel", PublicChannel)
	if err != nil {
		t.Fatalf("Failed to create channel: %v", err)
	}

	// Delete channel
	err = service.DeleteChannel(channel.ChannelID)
	if err != nil {
		t.Fatalf("Failed to delete channel: %v", err)
	}

	// Verify channel is deleted
	_, err = service.GetChannel(channel.ChannelID)
	if err != ErrChannelNotFound {
		t.Errorf("Expected ErrChannelNotFound, got %v", err)
	}
}
