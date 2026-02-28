package peerstore

import (
	"encoding/hex"
	"testing"

	bterrors "babylontower/pkg/errors"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makePubKey(id byte) []byte {
	key := make([]byte, 32)
	key[0] = id
	return key
}

func makeMultiaddrs(t *testing.T, strs ...string) []multiaddr.Multiaddr {
	t.Helper()
	addrs := make([]multiaddr.Multiaddr, len(strs))
	for i, s := range strs {
		ma, err := multiaddr.NewMultiaddr(s)
		require.NoError(t, err)
		addrs[i] = ma
	}
	return addrs
}

func TestNewAddrBook(t *testing.T) {
	ab, err := NewAddrBook(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, 0, ab.Count())
}

func TestNewAddrBook_LoadExisting(t *testing.T) {
	dir := t.TempDir()

	// Create and populate an address book
	ab1, err := NewAddrBook(dir)
	require.NoError(t, err)
	addrs := makeMultiaddrs(t, "/ip4/127.0.0.1/tcp/4001")
	require.NoError(t, ab1.AddContact(makePubKey(1), "peer1", addrs))

	// Create a new address book from the same directory — should load the saved data
	ab2, err := NewAddrBook(dir)
	require.NoError(t, err)
	assert.Equal(t, 1, ab2.Count())
}

func TestAddContact_NewAndUpdate(t *testing.T) {
	ab, err := NewAddrBook(t.TempDir())
	require.NoError(t, err)

	pk := makePubKey(1)
	addrs := makeMultiaddrs(t, "/ip4/127.0.0.1/tcp/4001")

	// Add new contact
	require.NoError(t, ab.AddContact(pk, "peer1", addrs))
	assert.Equal(t, 1, ab.Count())

	rec, err := ab.GetContact(pk)
	require.NoError(t, err)
	assert.Equal(t, "peer1", rec.PeerID)
	assert.Equal(t, []string{"/ip4/127.0.0.1/tcp/4001"}, rec.Addresses)

	// Update existing contact
	addrs2 := makeMultiaddrs(t, "/ip4/192.168.1.1/tcp/5001")
	require.NoError(t, ab.AddContact(pk, "peer1-new", addrs2))
	assert.Equal(t, 1, ab.Count()) // still one contact

	rec, err = ab.GetContact(pk)
	require.NoError(t, err)
	assert.Equal(t, "peer1-new", rec.PeerID)
	assert.Equal(t, []string{"/ip4/192.168.1.1/tcp/5001"}, rec.Addresses)
}

func TestGetContact_NotFound(t *testing.T) {
	ab, err := NewAddrBook(t.TempDir())
	require.NoError(t, err)

	_, err = ab.GetContact(makePubKey(99))
	assert.ErrorIs(t, err, bterrors.ErrPeerNotFound)
}

func TestGetContactByPeerID(t *testing.T) {
	ab, err := NewAddrBook(t.TempDir())
	require.NoError(t, err)

	pk := makePubKey(1)
	addrs := makeMultiaddrs(t, "/ip4/127.0.0.1/tcp/4001")
	require.NoError(t, ab.AddContact(pk, "peer1", addrs))

	rec, err := ab.GetContactByPeerID("peer1")
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(pk), rec.PublicKey)

	_, err = ab.GetContactByPeerID("nonexistent")
	assert.ErrorIs(t, err, bterrors.ErrPeerNotFound)
}

func TestUpdateAddresses(t *testing.T) {
	ab, err := NewAddrBook(t.TempDir())
	require.NoError(t, err)

	pk := makePubKey(1)
	addrs := makeMultiaddrs(t, "/ip4/127.0.0.1/tcp/4001")
	require.NoError(t, ab.AddContact(pk, "peer1", addrs))

	// Update addresses
	newAddrs := makeMultiaddrs(t, "/ip4/10.0.0.1/tcp/9000", "/ip4/10.0.0.2/tcp/9001")
	require.NoError(t, ab.UpdateAddresses(pk, newAddrs))

	rec, err := ab.GetContact(pk)
	require.NoError(t, err)
	assert.Len(t, rec.Addresses, 2)

	// Update for unknown peer
	err = ab.UpdateAddresses(makePubKey(99), newAddrs)
	assert.ErrorIs(t, err, bterrors.ErrPeerNotFound)
}

func TestSetConnected(t *testing.T) {
	ab, err := NewAddrBook(t.TempDir())
	require.NoError(t, err)

	pk := makePubKey(1)
	addrs := makeMultiaddrs(t, "/ip4/127.0.0.1/tcp/4001")
	require.NoError(t, ab.AddContact(pk, "peer1", addrs))

	require.NoError(t, ab.SetConnected(pk, true))
	rec, err := ab.GetContact(pk)
	require.NoError(t, err)
	assert.True(t, rec.Connected)

	require.NoError(t, ab.SetConnected(pk, false))
	rec, err = ab.GetContact(pk)
	require.NoError(t, err)
	assert.False(t, rec.Connected)

	// Unknown peer
	err = ab.SetConnected(makePubKey(99), true)
	assert.ErrorIs(t, err, bterrors.ErrPeerNotFound)
}

func TestGetAllContacts(t *testing.T) {
	ab, err := NewAddrBook(t.TempDir())
	require.NoError(t, err)

	addrs := makeMultiaddrs(t, "/ip4/127.0.0.1/tcp/4001")
	require.NoError(t, ab.AddContact(makePubKey(1), "peer1", addrs))
	require.NoError(t, ab.AddContact(makePubKey(2), "peer2", addrs))

	contacts, err := ab.GetAllContacts()
	require.NoError(t, err)
	assert.Len(t, contacts, 2)
}

func TestDeleteContact(t *testing.T) {
	ab, err := NewAddrBook(t.TempDir())
	require.NoError(t, err)

	pk := makePubKey(1)
	addrs := makeMultiaddrs(t, "/ip4/127.0.0.1/tcp/4001")
	require.NoError(t, ab.AddContact(pk, "peer1", addrs))
	assert.Equal(t, 1, ab.Count())

	require.NoError(t, ab.DeleteContact(pk))
	assert.Equal(t, 0, ab.Count())

	// Delete unknown
	err = ab.DeleteContact(makePubKey(99))
	assert.ErrorIs(t, err, bterrors.ErrPeerNotFound)
}

func TestLRUEviction(t *testing.T) {
	ab, err := NewAddrBook(t.TempDir())
	require.NoError(t, err)

	addrs := makeMultiaddrs(t, "/ip4/127.0.0.1/tcp/4001")

	// Fill to MaxPeers + 1
	for i := 0; i <= MaxPeers; i++ {
		pk := make([]byte, 32)
		pk[0] = byte(i >> 8)
		pk[1] = byte(i & 0xff)
		require.NoError(t, ab.AddContact(pk, "peer", addrs))
	}

	// Should have evicted the oldest, so count stays at MaxPeers
	assert.Equal(t, MaxPeers, ab.Count())
}

func TestGetContactReturnsCopy(t *testing.T) {
	ab, err := NewAddrBook(t.TempDir())
	require.NoError(t, err)

	pk := makePubKey(1)
	addrs := makeMultiaddrs(t, "/ip4/127.0.0.1/tcp/4001")
	require.NoError(t, ab.AddContact(pk, "peer1", addrs))

	rec1, err := ab.GetContact(pk)
	require.NoError(t, err)
	rec1.PeerID = "mutated"

	rec2, err := ab.GetContact(pk)
	require.NoError(t, err)
	assert.Equal(t, "peer1", rec2.PeerID) // original unmodified
}
