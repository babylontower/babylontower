package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestStorage(t *testing.T) *BadgerStorage {
	dir := t.TempDir()
	cfg := Config{
		Path:     dir,
		InMemory: false,
	}

	storage, err := NewBadgerStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	return storage
}

func TestAddPeer(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	peer := &PeerRecord{
		PeerID:        "QmTest123",
		Multiaddrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		FirstSeen:     time.Now(),
		LastSeen:      time.Now(),
		LastConnected: time.Now(),
		ConnectCount:  5,
		FailCount:     1,
		Source:        SourceBootstrap,
		Protocols:     []string{"/babylontower/1.0.0"},
		LatencyMs:     50,
	}

	err := storage.AddPeer(peer)
	if err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}
}

func TestGetPeer(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	peer := &PeerRecord{
		PeerID:        "QmTestGetPeer",
		Multiaddrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		FirstSeen:     time.Now(),
		LastSeen:      time.Now(),
		LastConnected: time.Now(),
		ConnectCount:  3,
		FailCount:     0,
		Source:        SourceDHT,
		Protocols:     []string{"/babylontower/1.0.0"},
		LatencyMs:     30,
	}

	// Add peer
	err := storage.AddPeer(peer)
	if err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	// Get peer
	retrieved, err := storage.GetPeer(peer.PeerID)
	if err != nil {
		t.Fatalf("GetPeer failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetPeer returned nil")
	}

	// Verify fields
	if retrieved.PeerID != peer.PeerID {
		t.Errorf("PeerID mismatch: got %s, want %s", retrieved.PeerID, peer.PeerID)
	}
	if retrieved.Source != peer.Source {
		t.Errorf("Source mismatch: got %s, want %s", retrieved.Source, peer.Source)
	}
	if retrieved.ConnectCount != peer.ConnectCount {
		t.Errorf("ConnectCount mismatch: got %d, want %d", retrieved.ConnectCount, peer.ConnectCount)
	}
}

func TestGetPeerNotFound(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	peer, err := storage.GetPeer("NonExistentPeer")
	if err != nil {
		t.Fatalf("GetPeer should not return error for non-existent peer: %v", err)
	}
	if peer != nil {
		t.Error("GetPeer should return nil for non-existent peer")
	}
}

func TestListPeers(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Add multiple peers
	peers := []*PeerRecord{
		{PeerID: "QmPeer1", Source: SourceBootstrap, LastSeen: time.Now()},
		{PeerID: "QmPeer2", Source: SourceDHT, LastSeen: time.Now()},
		{PeerID: "QmPeer3", Source: SourceMDNS, LastSeen: time.Now()},
	}

	for _, peer := range peers {
		peer.FirstSeen = time.Now()
		peer.LastConnected = time.Now()
		if err := storage.AddPeer(peer); err != nil {
			t.Fatalf("AddPeer failed: %v", err)
		}
	}

	// List all peers
	listed, err := storage.ListPeers(0)
	if err != nil {
		t.Fatalf("ListPeers failed: %v", err)
	}
	if len(listed) != 3 {
		t.Errorf("Expected 3 peers, got %d", len(listed))
	}

	// List with limit
	listed, err = storage.ListPeers(2)
	if err != nil {
		t.Fatalf("ListPeers with limit failed: %v", err)
	}
	if len(listed) != 2 {
		t.Errorf("Expected 2 peers with limit, got %d", len(listed))
	}
}

func TestListPeersBySource(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Add peers with different sources
	peers := []*PeerRecord{
		{PeerID: "QmBootstrap1", Source: SourceBootstrap, LastSeen: time.Now()},
		{PeerID: "QmBootstrap2", Source: SourceBootstrap, LastSeen: time.Now()},
		{PeerID: "QmDHT1", Source: SourceDHT, LastSeen: time.Now()},
	}

	for _, peer := range peers {
		peer.FirstSeen = time.Now()
		peer.LastConnected = time.Now()
		if err := storage.AddPeer(peer); err != nil {
			t.Fatalf("AddPeer failed: %v", err)
		}
	}

	// Filter by bootstrap source
	listed, err := storage.ListPeersBySource(SourceBootstrap)
	if err != nil {
		t.Fatalf("ListPeersBySource failed: %v", err)
	}
	if len(listed) != 2 {
		t.Errorf("Expected 2 bootstrap peers, got %d", len(listed))
	}

	// Filter by DHT source
	listed, err = storage.ListPeersBySource(SourceDHT)
	if err != nil {
		t.Fatalf("ListPeersBySource failed: %v", err)
	}
	if len(listed) != 1 {
		t.Errorf("Expected 1 DHT peer, got %d", len(listed))
	}
}

func TestDeletePeer(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	peer := &PeerRecord{
		PeerID:   "QmToDelete",
		Source:   SourceBootstrap,
		LastSeen: time.Now(),
	}
	peer.FirstSeen = time.Now()
	peer.LastConnected = time.Now()

	// Add peer
	err := storage.AddPeer(peer)
	if err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	// Delete peer
	err = storage.DeletePeer(peer.PeerID)
	if err != nil {
		t.Fatalf("DeletePeer failed: %v", err)
	}

	// Verify deletion
	retrieved, err := storage.GetPeer(peer.PeerID)
	if err != nil {
		t.Fatalf("GetPeer after delete failed: %v", err)
	}
	if retrieved != nil {
		t.Error("GetPeer should return nil after delete")
	}
}

func TestPrunePeers(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	now := time.Now()
	oldTime := now.Add(-48 * time.Hour) // 2 days ago

	// Add peers with different ages
	peers := []*PeerRecord{
		{PeerID: "QmRecent1", Source: SourceBootstrap, LastSeen: now},
		{PeerID: "QmRecent2", Source: SourceDHT, LastSeen: now.Add(-1 * time.Hour)},
		{PeerID: "QmOld1", Source: SourceMDNS, LastSeen: oldTime},
		{PeerID: "QmOld2", Source: SourcePeerExchange, LastSeen: oldTime.Add(-24 * time.Hour)},
	}

	for _, peer := range peers {
		peer.FirstSeen = peer.LastSeen
		peer.LastConnected = peer.LastSeen
		if err := storage.AddPeer(peer); err != nil {
			t.Fatalf("AddPeer failed: %v", err)
		}
	}

	// Prune peers older than 1 day
	err := storage.PrunePeers(1, 0)
	if err != nil {
		t.Fatalf("PrunePeers failed: %v", err)
	}

	// Verify only recent peers remain
	listed, err := storage.ListPeers(0)
	if err != nil {
		t.Fatalf("ListPeers after prune failed: %v", err)
	}
	if len(listed) != 2 {
		t.Errorf("Expected 2 peers after pruning, got %d", len(listed))
		for _, p := range listed {
			t.Logf("Remaining peer: %s, LastSeen: %v", p.PeerID, p.LastSeen)
		}
	}
}

func TestPrunePeersWithKeepCount(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	now := time.Now()

	// Add 5 recent peers
	for i := 0; i < 5; i++ {
		peer := &PeerRecord{
			PeerID:        "QmPeer",
			Source:        SourceBootstrap,
			LastSeen:      now,
			FirstSeen:     now,
			LastConnected: now,
			ConnectCount:  i,
			FailCount:     0,
		}
		peer.PeerID = "QmPeer" + string(rune('0'+i))
		if err := storage.AddPeer(peer); err != nil {
			t.Fatalf("AddPeer failed: %v", err)
		}
	}

	// Prune with keepCount=3 (should keep 3 best peers)
	err := storage.PrunePeers(30, 3)
	if err != nil {
		t.Fatalf("PrunePeers failed: %v", err)
	}

	// Verify only 3 peers remain
	listed, err := storage.ListPeers(0)
	if err != nil {
		t.Fatalf("ListPeers after prune failed: %v", err)
	}
	if len(listed) != 3 {
		t.Errorf("Expected 3 peers after pruning with keepCount, got %d", len(listed))
	}
}

func TestPeerRecordSuccessRate(t *testing.T) {
	tests := []struct {
		name         string
		connectCount int
		failCount    int
		want         float64
	}{
		{"no attempts", 0, 0, 0.0},
		{"all success", 10, 0, 1.0},
		{"all fail", 0, 10, 0.0},
		{"50-50", 5, 5, 0.5},
		{"75% success", 75, 25, 0.75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer := &PeerRecord{
				ConnectCount: tt.connectCount,
				FailCount:    tt.failCount,
			}
			got := peer.SuccessRate()
			if got != tt.want {
				t.Errorf("SuccessRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPeerRecordIsStale(t *testing.T) {
	now := time.Now()
	oldTime := now.Add(-48 * time.Hour)

	tests := []struct {
		name    string
		lastSeen time.Time
		maxAge  time.Duration
		want    bool
	}{
		{"recent peer", now, 24 * time.Hour, false},
		{"old peer", oldTime, 24 * time.Hour, true},
		{"just over threshold", now.Add(-25 * time.Hour), 24 * time.Hour, true},
		{"well within threshold", now.Add(-12 * time.Hour), 24 * time.Hour, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer := &PeerRecord{LastSeen: tt.lastSeen}
			got := peer.IsStale(tt.maxAge)
			if got != tt.want {
				t.Errorf("IsStale() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStorageConcurrent(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Concurrent peer additions
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			peer := &PeerRecord{
				PeerID:        "QmConcurrent" + string(rune('0'+id)),
				Source:        SourceBootstrap,
				LastSeen:      time.Now(),
				FirstSeen:     time.Now(),
				LastConnected: time.Now(),
			}
			if err := storage.AddPeer(peer); err != nil {
				t.Errorf("AddPeer failed: %v", err)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all peers were added
	listed, err := storage.ListPeers(0)
	if err != nil {
		t.Fatalf("ListPeers failed: %v", err)
	}
	if len(listed) != 10 {
		t.Errorf("Expected 10 concurrent peers, got %d", len(listed))
	}
}

func TestBadgerStoragePeerPersistence(t *testing.T) {
	// Create temp directory
	dir, err := os.MkdirTemp("", "badger-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("RemoveAll failed: %v", err)
		}
	}()

	// Create storage and add peer
	cfg := Config{Path: filepath.Join(dir, "badger")}
	storage, err := NewBadgerStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	peer := &PeerRecord{
		PeerID:        "QmPersistent",
		Multiaddrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		Source:        SourceBootstrap,
		LastSeen:      time.Now(),
		FirstSeen:     time.Now(),
		LastConnected: time.Now(),
		ConnectCount:  5,
	}

	if err := storage.AddPeer(peer); err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	// Close storage
	if err := storage.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Reopen storage
	storage2, err := NewBadgerStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to reopen storage: %v", err)
	}
	defer func() {
		if err := storage2.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Verify peer persisted
	retrieved, err := storage2.GetPeer(peer.PeerID)
	if err != nil {
		t.Fatalf("GetPeer after reopen failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Peer did not persist after storage reopen")
	}
	if retrieved.PeerID != peer.PeerID {
		t.Errorf("PeerID mismatch after reopen: got %s, want %s", retrieved.PeerID, peer.PeerID)
	}
}
