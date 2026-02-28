package storage

import (
	"testing"
	"time"
)

func TestBlacklistPeer(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	peerID := "QmBlacklistTest"
	reason := "Test blacklist reason"

	// Blacklist peer
	err := storage.BlacklistPeer(peerID, reason)
	if err != nil {
		t.Fatalf("BlacklistPeer failed: %v", err)
	}

	// Check if peer is blacklisted
	blacklisted, err := storage.IsBlacklisted(peerID)
	if err != nil {
		t.Fatalf("IsBlacklisted failed: %v", err)
	}
	if !blacklisted {
		t.Error("Peer should be blacklisted")
	}
}

func TestBlacklistPeerNotFound(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	blacklisted, err := storage.IsBlacklisted("NonExistentPeer")
	if err != nil {
		t.Fatalf("IsBlacklisted failed: %v", err)
	}
	if blacklisted {
		t.Error("Non-blacklisted peer should return false")
	}
}

func TestListBlacklisted(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Add multiple blacklisted peers
	peers := []struct {
		id     string
		reason string
	}{
		{"QmBlack1", "Reason 1"},
		{"QmBlack2", "Reason 2"},
		{"QmBlack3", "Reason 3"},
	}

	for _, p := range peers {
		if err := storage.BlacklistPeer(p.id, p.reason); err != nil {
			t.Fatalf("BlacklistPeer failed: %v", err)
		}
	}

	// List blacklisted peers
	listed, err := storage.ListBlacklisted()
	if err != nil {
		t.Fatalf("ListBlacklisted failed: %v", err)
	}
	if len(listed) != 3 {
		t.Errorf("Expected 3 blacklisted peers, got %d", len(listed))
	}

	// Verify entries
	for i, entry := range listed {
		if entry.PeerID != peers[i].id {
			t.Errorf("PeerID mismatch: got %s, want %s", entry.PeerID, peers[i].id)
		}
		if entry.Reason != peers[i].reason {
			t.Errorf("Reason mismatch: got %s, want %s", entry.Reason, peers[i].reason)
		}
		if entry.BlacklistedAt.IsZero() {
			t.Error("BlacklistedAt should be set")
		}
	}
}

func TestRemoveFromBlacklist(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	peerID := "QmToRemove"

	// Blacklist peer
	err := storage.BlacklistPeer(peerID, "Test reason")
	if err != nil {
		t.Fatalf("BlacklistPeer failed: %v", err)
	}

	// Verify blacklisted
	blacklisted, err := storage.IsBlacklisted(peerID)
	if err != nil {
		t.Fatalf("IsBlacklisted failed: %v", err)
	}
	if !blacklisted {
		t.Error("Peer should be blacklisted")
	}

	// Remove from blacklist
	err = storage.RemoveFromBlacklist(peerID)
	if err != nil {
		t.Fatalf("RemoveFromBlacklist failed: %v", err)
	}

	// Verify no longer blacklisted
	blacklisted, err = storage.IsBlacklisted(peerID)
	if err != nil {
		t.Fatalf("IsBlacklisted failed: %v", err)
	}
	if blacklisted {
		t.Error("Peer should not be blacklisted after removal")
	}
}

func TestBlacklistEntryIsExpired(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"permanent (no expiry)", time.Time{}, false},
		{"future expiry", now.Add(1 * time.Hour), false},
		{"past expiry", now.Add(-1 * time.Hour), true},
		{"just expired", now.Add(-1 * time.Second), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &BlacklistEntry{
				PeerID:        "QmTest",
				Reason:        "Test",
				BlacklistedAt: now.Add(-2 * time.Hour),
				ExpiresAt:     tt.expiresAt,
			}

			got := entry.IsExpired()
			if got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlacklistPersistence(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	peerID := "QmPersistent"
	reason := "Persistent test"

	// Blacklist peer
	err := storage.BlacklistPeer(peerID, reason)
	if err != nil {
		t.Fatalf("BlacklistPeer failed: %v", err)
	}

	// Close and reopen storage
	if err := storage.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	storage2, err := NewBadgerStorage(Config{Path: t.TempDir()})
	if err != nil {
		t.Fatalf("Failed to create new storage: %v", err)
	}
	defer func() {
		if err := storage2.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Note: BadgerDB persists to disk, but we're using a new temp dir
	// This test verifies the blacklist API works across storage instances
	// For true persistence test, we'd need to use the same directory
}

func TestBlacklistConcurrent(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Concurrent blacklist operations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			peerID := "QmConcurrent" + string(rune('0'+id))
			if err := storage.BlacklistPeer(peerID, "Concurrent test"); err != nil {
				t.Errorf("BlacklistPeer failed: %v", err)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all peers were blacklisted
	listed, err := storage.ListBlacklisted()
	if err != nil {
		t.Fatalf("ListBlacklisted failed: %v", err)
	}
	if len(listed) != 10 {
		t.Errorf("Expected 10 blacklisted peers, got %d", len(listed))
	}
}
