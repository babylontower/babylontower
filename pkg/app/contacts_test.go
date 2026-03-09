package app

import (
	"encoding/hex"
	"testing"

	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestApp(t *testing.T) *application {
	t.Helper()

	ident, err := GenerateNewIdentity()
	require.NoError(t, err)

	store := storage.NewMemoryStorage()

	return &application{
		config:   DefaultAppConfig(),
		identity: ident.Identity,
		storage:  store,
	}
}

func TestContactManager_AddContact(t *testing.T) {
	app := newTestApp(t)
	cm := &contactManager{app: app}

	// Generate a contact identity
	contact, err := GenerateNewIdentity()
	require.NoError(t, err)

	// Add by base58
	info, err := cm.AddContact(contact.PublicKeyBase58, "Alice", contact.X25519KeyBase58)
	require.NoError(t, err)

	assert.Equal(t, "Alice", info.DisplayName)
	assert.Equal(t, contact.PublicKeyBase58, info.PublicKeyBase58)
	assert.True(t, info.HasEncryptionKey)
	assert.NotEmpty(t, info.ContactLink)
}

func TestContactManager_AddContact_Hex(t *testing.T) {
	app := newTestApp(t)
	cm := &contactManager{app: app}

	contact, err := GenerateNewIdentity()
	require.NoError(t, err)

	hexKey := hex.EncodeToString(contact.Identity.Ed25519PubKey)
	info, err := cm.AddContact(hexKey, "Bob", "")
	require.NoError(t, err)

	assert.Equal(t, "Bob", info.DisplayName)
	assert.False(t, info.HasEncryptionKey)
}

func TestContactManager_AddContact_Duplicate(t *testing.T) {
	app := newTestApp(t)
	cm := &contactManager{app: app}

	contact, err := GenerateNewIdentity()
	require.NoError(t, err)

	_, err = cm.AddContact(contact.PublicKeyBase58, "Alice", "")
	require.NoError(t, err)

	// Adding again returns existing
	info, err := cm.AddContact(contact.PublicKeyBase58, "Different Name", "")
	require.NoError(t, err)
	assert.Equal(t, "Alice", info.DisplayName) // Original name kept
}

func TestContactManager_AddContact_Self(t *testing.T) {
	app := newTestApp(t)
	cm := &contactManager{app: app}

	selfKey := base58.Encode(app.identity.Ed25519PubKey)
	_, err := cm.AddContact(selfKey, "Me", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot add yourself")
}

func TestContactManager_AddContact_InvalidKey(t *testing.T) {
	app := newTestApp(t)
	cm := &contactManager{app: app}

	_, err := cm.AddContact("not-a-key!!!", "Bad", "")
	assert.Error(t, err)
}

func TestContactManager_GetContact(t *testing.T) {
	app := newTestApp(t)
	cm := &contactManager{app: app}

	contact, err := GenerateNewIdentity()
	require.NoError(t, err)

	_, err = cm.AddContact(contact.PublicKeyBase58, "Alice", "")
	require.NoError(t, err)

	info, err := cm.GetContact(contact.PublicKeyBase58)
	require.NoError(t, err)
	assert.Equal(t, "Alice", info.DisplayName)
}

func TestContactManager_GetContact_NotFound(t *testing.T) {
	app := newTestApp(t)
	cm := &contactManager{app: app}

	contact, err := GenerateNewIdentity()
	require.NoError(t, err)

	_, err = cm.GetContact(contact.PublicKeyBase58)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestContactManager_ListContacts(t *testing.T) {
	app := newTestApp(t)
	cm := &contactManager{app: app}

	// Add two contacts
	c1, err := GenerateNewIdentity()
	require.NoError(t, err)
	c2, err := GenerateNewIdentity()
	require.NoError(t, err)

	_, err = cm.AddContact(c1.PublicKeyBase58, "Alice", "")
	require.NoError(t, err)
	_, err = cm.AddContact(c2.PublicKeyBase58, "Bob", "")
	require.NoError(t, err)

	list, err := cm.ListContacts()
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestContactManager_RemoveContact(t *testing.T) {
	app := newTestApp(t)
	cm := &contactManager{app: app}

	contact, err := GenerateNewIdentity()
	require.NoError(t, err)

	_, err = cm.AddContact(contact.PublicKeyBase58, "Alice", "")
	require.NoError(t, err)

	err = cm.RemoveContact(contact.PublicKeyBase58)
	require.NoError(t, err)

	_, err = cm.GetContact(contact.PublicKeyBase58)
	assert.Error(t, err)
}

func TestContactManager_UpdateContactName(t *testing.T) {
	app := newTestApp(t)
	cm := &contactManager{app: app}

	contact, err := GenerateNewIdentity()
	require.NoError(t, err)

	_, err = cm.AddContact(contact.PublicKeyBase58, "Alice", "")
	require.NoError(t, err)

	err = cm.UpdateContactName(contact.PublicKeyBase58, "Alice Smith")
	require.NoError(t, err)

	info, err := cm.GetContact(contact.PublicKeyBase58)
	require.NoError(t, err)
	assert.Equal(t, "Alice Smith", info.DisplayName)
}

func TestContactManager_AddContactFromLink(t *testing.T) {
	app := newTestApp(t)
	cm := &contactManager{app: app}

	contact, err := GenerateNewIdentity()
	require.NoError(t, err)

	link := GenerateContactLink(contact.Identity.Ed25519PubKey, contact.Identity.X25519PubKey, "Alice")
	info, err := cm.AddContactFromLink(link)
	require.NoError(t, err)

	assert.Equal(t, "Alice", info.DisplayName)
	assert.Equal(t, contact.PublicKeyBase58, info.PublicKeyBase58)
	assert.True(t, info.HasEncryptionKey)
}

func TestContactInfoFromProto(t *testing.T) {
	contact, err := GenerateNewIdentity()
	require.NoError(t, err)

	proto := &pb.Contact{
		PublicKey:       contact.Identity.Ed25519PubKey,
		X25519PublicKey: contact.Identity.X25519PubKey,
		DisplayName:     "Test",
		CreatedAt:       1000,
	}

	info := contactInfoFromProto(proto)
	assert.Equal(t, "Test", info.DisplayName)
	assert.True(t, info.HasEncryptionKey)
	assert.NotEmpty(t, info.ContactLink)
	assert.NotEmpty(t, info.PublicKeyBase58)
	assert.NotEmpty(t, info.PublicKeyHex)
	assert.NotEmpty(t, info.X25519KeyBase58)
}

func TestDecodePubKey(t *testing.T) {
	contact, err := GenerateNewIdentity()
	require.NoError(t, err)

	// Base58
	decoded, err := decodePubKey(contact.PublicKeyBase58)
	require.NoError(t, err)
	assert.Equal(t, contact.Identity.Ed25519PubKey, decoded)

	// Hex
	hexKey := hex.EncodeToString(contact.Identity.Ed25519PubKey)
	decoded2, err := decodePubKey(hexKey)
	require.NoError(t, err)
	assert.Equal(t, contact.Identity.Ed25519PubKey, decoded2)

	// Invalid
	_, err = decodePubKey("!!!invalid!!!")
	assert.Error(t, err)
}
