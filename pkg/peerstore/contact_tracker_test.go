package peerstore

import (
	"encoding/hex"
	"testing"

	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTracker(t *testing.T) (*ContactTracker, storage.Storage) {
	t.Helper()
	store := storage.NewMemoryStorage()
	ct := NewContactTracker(store, nil)
	return ct, store
}

func addTestContact(t *testing.T, store storage.Storage, id byte, peerID string) *pb.Contact {
	t.Helper()
	pk := makePubKey(id)
	c := &pb.Contact{
		PublicKey:    pk,
		DisplayName:  "test-" + peerID,
		PeerId:      peerID,
		Multiaddrs:  []string{"/ip4/127.0.0.1/tcp/4001"},
		LastSeen:    1000,
	}
	require.NoError(t, store.AddContact(c))
	return c
}

func TestNewContactTracker(t *testing.T) {
	ct, _ := newTestTracker(t)
	assert.NotNil(t, ct)
	assert.Empty(t, ct.contactPeers)
}

func TestLoadContacts(t *testing.T) {
	ct, store := newTestTracker(t)
	addTestContact(t, store, 1, "peer1")
	addTestContact(t, store, 2, "peer2")

	require.NoError(t, ct.LoadContacts())

	stats := ct.GetStats()
	assert.Equal(t, 2, stats.TotalContacts)
	assert.Equal(t, 2, stats.WithPeerID)
	assert.Equal(t, 0, stats.OnlineContacts)
}

func TestUpdateContact(t *testing.T) {
	ct, _ := newTestTracker(t)

	c := &pb.Contact{
		PublicKey:    makePubKey(1),
		DisplayName:  "Alice",
		PeerId:      "peer-alice",
		Multiaddrs:  []string{"/ip4/10.0.0.1/tcp/5000"},
		LastSeen:    2000,
	}

	require.NoError(t, ct.UpdateContact(c))

	info, ok := ct.GetContactInfo(makePubKey(1))
	require.True(t, ok)
	assert.Equal(t, "peer-alice", info.PeerID)
	assert.False(t, info.IsOnline)
}

func TestGetContactInfo_NotFound(t *testing.T) {
	ct, _ := newTestTracker(t)
	_, ok := ct.GetContactInfo(makePubKey(99))
	assert.False(t, ok)
}

func TestGetContactPeerID(t *testing.T) {
	ct, _ := newTestTracker(t)

	require.NoError(t, ct.UpdateContact(&pb.Contact{
		PublicKey: makePubKey(1),
		PeerId:   "peer-1",
	}))

	pid, ok := ct.GetContactPeerID(makePubKey(1))
	assert.True(t, ok)
	assert.Equal(t, "peer-1", pid)

	// Empty PeerID returns false
	require.NoError(t, ct.UpdateContact(&pb.Contact{
		PublicKey: makePubKey(2),
		PeerId:   "",
	}))
	_, ok = ct.GetContactPeerID(makePubKey(2))
	assert.False(t, ok)

	// Unknown contact
	_, ok = ct.GetContactPeerID(makePubKey(99))
	assert.False(t, ok)
}

func TestSetContactOnline(t *testing.T) {
	ct, store := newTestTracker(t)
	addTestContact(t, store, 1, "peer1")
	require.NoError(t, ct.LoadContacts())

	pk := makePubKey(1)
	addr, _ := multiaddr.NewMultiaddr("/ip4/10.0.0.1/tcp/9000")
	require.NoError(t, ct.SetContactOnline(pk, "peer1", []multiaddr.Multiaddr{addr}))

	info, ok := ct.GetContactInfo(pk)
	require.True(t, ok)
	assert.True(t, info.IsOnline)
	assert.Equal(t, "peer1", info.PeerID)
	assert.Len(t, info.Multiaddrs, 1)
}

func TestSetContactOnline_UnknownContact(t *testing.T) {
	ct, _ := newTestTracker(t)

	pk := makePubKey(42)
	// SetContactOnline creates a new entry for unknown contacts
	require.NoError(t, ct.SetContactOnline(pk, "new-peer", nil))

	info, ok := ct.GetContactInfo(pk)
	require.True(t, ok)
	assert.True(t, info.IsOnline)
}

func TestSetContactOffline(t *testing.T) {
	ct, _ := newTestTracker(t)
	pk := makePubKey(1)

	require.NoError(t, ct.UpdateContact(&pb.Contact{PublicKey: pk, PeerId: "p1"}))
	require.NoError(t, ct.SetContactOnline(pk, "p1", nil))

	ct.SetContactOffline(pk)
	info, ok := ct.GetContactInfo(pk)
	require.True(t, ok)
	assert.False(t, info.IsOnline)

	// Offline on unknown is a no-op
	ct.SetContactOffline(makePubKey(99))
}

func TestSetConnectedTracker(t *testing.T) {
	ct, _ := newTestTracker(t)
	pk := makePubKey(1)

	require.NoError(t, ct.UpdateContact(&pb.Contact{PublicKey: pk, PeerId: "p1"}))
	ct.SetConnected(pk, true)

	info, ok := ct.GetContactInfo(pk)
	require.True(t, ok)
	assert.True(t, info.ConnectedByUs)

	ct.SetConnected(pk, false)
	info, _ = ct.GetContactInfo(pk)
	assert.False(t, info.ConnectedByUs)
}

func TestGetOnlineContacts(t *testing.T) {
	ct, _ := newTestTracker(t)

	require.NoError(t, ct.UpdateContact(&pb.Contact{PublicKey: makePubKey(1), PeerId: "p1"}))
	require.NoError(t, ct.UpdateContact(&pb.Contact{PublicKey: makePubKey(2), PeerId: "p2"}))
	require.NoError(t, ct.UpdateContact(&pb.Contact{PublicKey: makePubKey(3), PeerId: "p3"}))

	require.NoError(t, ct.SetContactOnline(makePubKey(1), "p1", nil))
	require.NoError(t, ct.SetContactOnline(makePubKey(3), "p3", nil))

	online := ct.GetOnlineContacts()
	assert.Len(t, online, 2)

	// Verify returned are copies
	peerIDs := map[string]bool{}
	for _, o := range online {
		peerIDs[o.PeerID] = true
	}
	assert.True(t, peerIDs["p1"])
	assert.True(t, peerIDs["p3"])
}

func TestGetContactByPeerIDTracker(t *testing.T) {
	ct, _ := newTestTracker(t)

	require.NoError(t, ct.UpdateContact(&pb.Contact{PublicKey: makePubKey(1), PeerId: "unique-peer"}))

	info, ok := ct.GetContactByPeerID("unique-peer")
	assert.True(t, ok)
	assert.Equal(t, hex.EncodeToString(makePubKey(1)), hex.EncodeToString(info.Ed25519PubKey))

	_, ok = ct.GetContactByPeerID("nonexistent")
	assert.False(t, ok)
}

func TestGetStats(t *testing.T) {
	ct, _ := newTestTracker(t)

	require.NoError(t, ct.UpdateContact(&pb.Contact{PublicKey: makePubKey(1), PeerId: "p1"}))
	require.NoError(t, ct.UpdateContact(&pb.Contact{PublicKey: makePubKey(2), PeerId: ""}))
	require.NoError(t, ct.SetContactOnline(makePubKey(1), "p1", nil))
	ct.SetConnected(makePubKey(1), true)

	stats := ct.GetStats()
	assert.Equal(t, 2, stats.TotalContacts)
	assert.Equal(t, 1, stats.OnlineContacts)
	assert.Equal(t, 1, stats.ConnectedPeers)
	assert.Equal(t, 1, stats.WithPeerID)
}

func TestGetContactInfoReturnsCopy(t *testing.T) {
	ct, _ := newTestTracker(t)
	pk := makePubKey(1)

	require.NoError(t, ct.UpdateContact(&pb.Contact{PublicKey: pk, PeerId: "p1"}))

	info1, _ := ct.GetContactInfo(pk)
	info1.PeerID = "mutated"

	info2, _ := ct.GetContactInfo(pk)
	assert.Equal(t, "p1", info2.PeerID)
}
