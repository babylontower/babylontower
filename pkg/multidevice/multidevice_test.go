package multidevice

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDeviceConfig generates a DeviceConfig with fresh random keys for testing.
func newTestDeviceConfig(t *testing.T, deviceName string) *DeviceConfig {
	t.Helper()

	signPub, signPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	dhPriv := make([]byte, 32)
	_, err = rand.Read(dhPriv)
	require.NoError(t, err)

	// Derive X25519 public key from private key using curve25519.Basepoint
	// For testing purposes a random 32-byte slice is sufficient as DH pub placeholder.
	dhPub := make([]byte, 32)
	_, err = rand.Read(dhPub)
	require.NoError(t, err)

	groupKey := make([]byte, 32)
	_, err = rand.Read(groupKey)
	require.NoError(t, err)

	return &DeviceConfig{
		IdentitySignPub:  signPub,
		IdentitySignPriv: signPriv,
		IdentityDHPub:    dhPub,
		IdentityDHPriv:   dhPriv,
		DeviceName:       deviceName,
		DeviceGroupKey:   groupKey,
	}
}

// newTestStorage creates an in-memory BadgerDB storage for testing.
func newTestStorage(t *testing.T) storage.Storage {
	t.Helper()
	store, err := storage.NewBadgerStorage(storage.Config{InMemory: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// ---------------------------------------------------------------------------
// Device registration and certificate verification
// ---------------------------------------------------------------------------

func TestRegisterNewDevice(t *testing.T) {
	cfg := newTestDeviceConfig(t, "test-device")
	dm := NewDeviceManager(cfg)

	cert, err := dm.RegisterNewDevice()
	require.NoError(t, err)
	require.NotNil(t, cert)

	// Certificate fields should be populated.
	assert.Len(t, cert.DeviceId, 16, "device ID should be 16 bytes")
	assert.Len(t, cert.DeviceSignPub, ed25519.PublicKeySize, "device sign pub should be 32 bytes")
	assert.Len(t, cert.DeviceDhPub, 32, "device DH pub should be 32 bytes")
	assert.Equal(t, "test-device", cert.DeviceName)
	assert.NotZero(t, cert.CreatedAt)
	assert.Equal(t, uint64(0), cert.ExpiresAt)
	assert.Equal(t, []byte(cfg.IdentitySignPub), cert.IdentityPub)
	assert.Len(t, cert.Signature, ed25519.SignatureSize, "signature should be 64 bytes")

	// Device ID should match SHA256(device sign pub)[:16].
	expectedID := ComputeDeviceID(cert.DeviceSignPub)
	assert.Equal(t, expectedID, cert.DeviceId)

	// After registration the manager should expose the new device keys.
	assert.Equal(t, cert.DeviceId, dm.GetDeviceID())
	assert.Equal(t, cert.DeviceSignPub, []byte(dm.GetDeviceSignPub()))
	assert.NotNil(t, dm.GetDeviceSignPriv())
	assert.Equal(t, cert.DeviceDhPub, dm.GetDeviceDHPub())
	assert.NotNil(t, dm.GetDeviceDHPriv())
	assert.Equal(t, cfg.DeviceGroupKey, dm.GetDeviceGroupKey())
}

func TestVerifyDeviceCertificate_Valid(t *testing.T) {
	cfg := newTestDeviceConfig(t, "alice-laptop")
	dm := NewDeviceManager(cfg)

	cert, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	err = VerifyDeviceCertificate(cert)
	assert.NoError(t, err, "valid certificate should pass verification")
}

func TestVerifyDeviceCertificate_TamperedSignature(t *testing.T) {
	cfg := newTestDeviceConfig(t, "tampered")
	dm := NewDeviceManager(cfg)

	cert, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	// Flip a byte in the signature.
	cert.Signature[0] ^= 0xFF

	err = VerifyDeviceCertificate(cert)
	assert.Error(t, err, "tampered signature should fail verification")
	assert.Contains(t, err.Error(), "invalid device certificate signature")
}

func TestVerifyDeviceCertificate_TamperedDeviceName(t *testing.T) {
	cfg := newTestDeviceConfig(t, "original")
	dm := NewDeviceManager(cfg)

	cert, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	// Tamper with the device name.
	cert.DeviceName = "evil-device"

	err = VerifyDeviceCertificate(cert)
	assert.Error(t, err, "tampered device name should fail verification")
}

func TestVerifyDeviceCertificate_WrongIdentityKey(t *testing.T) {
	cfg := newTestDeviceConfig(t, "wrong-key")
	dm := NewDeviceManager(cfg)

	cert, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	// Replace IdentityPub with a different key.
	otherPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cert.IdentityPub = otherPub

	err = VerifyDeviceCertificate(cert)
	assert.Error(t, err, "wrong identity pub should fail verification")
}

func TestRegisterMultipleDevices(t *testing.T) {
	cfg := newTestDeviceConfig(t, "device-1")
	dm := NewDeviceManager(cfg)

	cert1, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	// Register a second device (overwrites device keys on the same manager).
	dm.deviceName = "device-2"
	cert2, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	// Each registration should produce different device keys and IDs.
	assert.NotEqual(t, cert1.DeviceId, cert2.DeviceId)
	assert.NotEqual(t, cert1.DeviceSignPub, cert2.DeviceSignPub)

	// Both certificates should verify with the same identity key.
	assert.NoError(t, VerifyDeviceCertificate(cert1))
	assert.NoError(t, VerifyDeviceCertificate(cert2))
}

// ---------------------------------------------------------------------------
// Device ID computation and hex conversion
// ---------------------------------------------------------------------------

func TestComputeDeviceID(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	id := ComputeDeviceID(pub)
	assert.Len(t, id, 16)

	// Should be SHA256(pub)[:16].
	hash := sha256.Sum256(pub)
	assert.Equal(t, hash[:16], id)
}

func TestComputeDeviceID_Deterministic(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	id1 := ComputeDeviceID(pub)
	id2 := ComputeDeviceID(pub)
	assert.Equal(t, id1, id2, "same public key must produce same device ID")
}

func TestDeviceIDHexRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"zeros", make([]byte, 16)},
		{"ones", bytes.Repeat([]byte{0xFF}, 16)},
		{"random", func() []byte { b := make([]byte, 16); _, _ = rand.Read(b); return b }()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hexStr := DeviceIDToHex(tc.input)
			result, err := DeviceIDFromHex(hexStr)
			require.NoError(t, err)
			assert.Equal(t, tc.input, result)
		})
	}
}

func TestDeviceIDFromHex_Invalid(t *testing.T) {
	_, err := DeviceIDFromHex("not-valid-hex!")
	assert.Error(t, err, "invalid hex should return error")
}

// ---------------------------------------------------------------------------
// Sync topic derivation
// ---------------------------------------------------------------------------

func TestGetSyncTopic(t *testing.T) {
	pub := make([]byte, 32)
	_, _ = rand.Read(pub)

	topic := GetSyncTopic(pub)

	// Should start with the prefix.
	assert.Contains(t, topic, "babylon-sync-")

	// Should be deterministic.
	assert.Equal(t, topic, GetSyncTopic(pub))

	// Should differ for different keys.
	pub2 := make([]byte, 32)
	_, _ = rand.Read(pub2)
	topic2 := GetSyncTopic(pub2)
	assert.NotEqual(t, topic, topic2)
}

func TestGetSyncTopic_Format(t *testing.T) {
	pub := make([]byte, 32)
	for i := range pub {
		pub[i] = byte(i)
	}

	topic := GetSyncTopic(pub)

	hash := sha256.Sum256(pub)
	expected := "babylon-sync-" + hex.EncodeToString(hash[:8])
	assert.Equal(t, expected, topic)
}

// ---------------------------------------------------------------------------
// Vector clock operations (via SyncManager)
// ---------------------------------------------------------------------------

func TestSyncManager_VectorClock_UpdateAndGet(t *testing.T) {
	cfg := newTestDeviceConfig(t, "vc-device")
	dm := NewDeviceManager(cfg)

	// Register to populate deviceID.
	_, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	sm := NewSyncManager(&SyncManagerConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
	})

	deviceHex := hex.EncodeToString(dm.GetDeviceID())

	// Initial clock should be empty.
	vc := sm.getVectorClock()
	assert.Empty(t, vc.Clocks)

	// Update increments the local counter.
	sm.updateVectorClock()
	vc = sm.getVectorClock()
	assert.Equal(t, uint64(1), vc.Clocks[deviceHex])

	sm.updateVectorClock()
	sm.updateVectorClock()
	vc = sm.getVectorClock()
	assert.Equal(t, uint64(3), vc.Clocks[deviceHex])
}

func TestSyncManager_VectorClock_Merge(t *testing.T) {
	cfg := newTestDeviceConfig(t, "merge-device")
	dm := NewDeviceManager(cfg)
	_, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	sm := NewSyncManager(&SyncManagerConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
	})

	localHex := hex.EncodeToString(dm.GetDeviceID())

	// Set local counter.
	sm.updateVectorClock()
	sm.updateVectorClock()

	// Merge a remote clock.
	remote := &pb.VectorClock{
		Clocks: map[string]uint64{
			"remote-device-1": 5,
			"remote-device-2": 3,
			localHex:          1, // Remote has smaller value for local.
		},
	}
	sm.mergeVectorClock(remote)

	vc := sm.getVectorClock()
	// Local counter should keep the larger value.
	assert.Equal(t, uint64(2), vc.Clocks[localHex])
	// Remote counters should be adopted.
	assert.Equal(t, uint64(5), vc.Clocks["remote-device-1"])
	assert.Equal(t, uint64(3), vc.Clocks["remote-device-2"])
}

func TestSyncManager_VectorClock_MergeNil(t *testing.T) {
	cfg := newTestDeviceConfig(t, "nil-merge")
	dm := NewDeviceManager(cfg)
	_, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	sm := NewSyncManager(&SyncManagerConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
	})

	sm.updateVectorClock()

	// Merging nil should be a no-op.
	sm.mergeVectorClock(nil)

	vc := sm.getVectorClock()
	assert.Len(t, vc.Clocks, 1, "nil merge should not change clock")
}

func TestSyncManager_VectorClock_MergeTakesMax(t *testing.T) {
	cfg := newTestDeviceConfig(t, "max-device")
	dm := NewDeviceManager(cfg)
	_, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	sm := NewSyncManager(&SyncManagerConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
	})

	// Set initial remote value.
	sm.mergeVectorClock(&pb.VectorClock{Clocks: map[string]uint64{"dev-a": 10}})

	// Merge a smaller value for same key.
	sm.mergeVectorClock(&pb.VectorClock{Clocks: map[string]uint64{"dev-a": 5}})

	vc := sm.getVectorClock()
	assert.Equal(t, uint64(10), vc.Clocks["dev-a"], "merge should keep the max")
}

// ---------------------------------------------------------------------------
// BroadcastSync
// ---------------------------------------------------------------------------

func TestSyncManager_BroadcastSync(t *testing.T) {
	cfg := newTestDeviceConfig(t, "broadcast-device")
	dm := NewDeviceManager(cfg)
	_, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	sm := NewSyncManager(&SyncManagerConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
	})

	// Start the sync manager (needed for topic subscription setup).
	err = sm.Start(cfg.IdentitySignPub)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Stop() })

	contact := CreateContactSync(
		[]byte("contact-pub-key"),
		[]byte("x25519-key"),
		"Alice",
		"peer-id-123",
		[]string{"/ip4/127.0.0.1/tcp/4001"},
		false,
	)

	// BroadcastSync encrypts and serializes the sync message.
	// Note: actual publishing to IPFS is a no-op (data is discarded).
	err = sm.BroadcastSync(SyncTypeContactAdded, contact)
	assert.NoError(t, err, "BroadcastSync should succeed")
}

// ---------------------------------------------------------------------------
// Sync helper constructors
// ---------------------------------------------------------------------------

func TestCreateContactSync(t *testing.T) {
	cs := CreateContactSync(
		[]byte("pub"), []byte("x25519"), "Bob", "peer1",
		[]string{"/ip4/1.2.3.4/tcp/80"}, true,
	)
	assert.Equal(t, []byte("pub"), cs.ContactPubkey)
	assert.Equal(t, "Bob", cs.DisplayName)
	assert.Equal(t, []byte("x25519"), cs.X25519Pubkey)
	assert.Equal(t, "peer1", cs.PeerId)
	assert.Equal(t, []string{"/ip4/1.2.3.4/tcp/80"}, cs.Multiaddrs)
	assert.True(t, cs.IsRemoved)
	assert.NotZero(t, cs.CreatedAt)
}

func TestCreateReadReceiptSync(t *testing.T) {
	ids := [][]byte{[]byte("msg1"), []byte("msg2")}
	rr := CreateReadReceiptSync([]byte("contact"), ids)
	assert.Equal(t, []byte("contact"), rr.ContactPubkey)
	assert.Len(t, rr.MessageIds, 2)
	assert.NotZero(t, rr.ReadAt)
}

func TestCreateGroupSync(t *testing.T) {
	gs := CreateGroupSync([]byte("gid"), "Test Group", 5, true)
	assert.Equal(t, []byte("gid"), gs.GroupId)
	assert.Equal(t, "Test Group", gs.Name)
	assert.Equal(t, uint64(5), gs.Epoch)
	assert.True(t, gs.Joined)
	assert.NotZero(t, gs.Timestamp)
}

func TestCreateSettingsSync(t *testing.T) {
	ss := CreateSettingsSync("theme", []byte(`"dark"`))
	assert.Equal(t, "theme", ss.Key)
	assert.Equal(t, []byte(`"dark"`), ss.Value)
	assert.NotZero(t, ss.UpdatedAt)
}

// ---------------------------------------------------------------------------
// Revocation: create, verify, and verify-tampered
// ---------------------------------------------------------------------------

func TestRevokeDevice(t *testing.T) {
	cfg := newTestDeviceConfig(t, "revoke-device")
	dm := NewDeviceManager(cfg)
	_, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	rm := NewRevocationManager(&RevocationConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
	})

	deviceID := dm.GetDeviceID()
	cert, err := rm.RevokeDevice(deviceID, "compromised")
	require.NoError(t, err)
	require.NotNil(t, cert)

	assert.Equal(t, deviceID, cert.RevokedKey)
	assert.Equal(t, "device", cert.RevocationType)
	assert.Equal(t, "compromised", cert.Reason)
	assert.NotZero(t, cert.RevokedAt)
	assert.Len(t, cert.Signature, ed25519.SignatureSize)
}

func TestVerifyRevocation_Valid(t *testing.T) {
	cfg := newTestDeviceConfig(t, "verify-rev")
	dm := NewDeviceManager(cfg)
	_, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	rm := NewRevocationManager(&RevocationConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
	})

	cert, err := rm.RevokeDevice(dm.GetDeviceID(), "replaced")
	require.NoError(t, err)

	err = VerifyRevocation(cert, cfg.IdentitySignPub)
	assert.NoError(t, err)
}

func TestVerifyRevocation_Tampered(t *testing.T) {
	cfg := newTestDeviceConfig(t, "tamper-rev")
	dm := NewDeviceManager(cfg)
	_, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	rm := NewRevocationManager(&RevocationConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
	})

	cert, err := rm.RevokeDevice(dm.GetDeviceID(), "expired")
	require.NoError(t, err)

	// Tamper with the reason.
	cert.Reason = "definitely-not-expired"

	err = VerifyRevocation(cert, cfg.IdentitySignPub)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid revocation signature")
}

func TestVerifyRevocation_WrongKey(t *testing.T) {
	cfg := newTestDeviceConfig(t, "wrong-rev")
	dm := NewDeviceManager(cfg)
	_, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	rm := NewRevocationManager(&RevocationConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
	})

	cert, err := rm.RevokeDevice(dm.GetDeviceID(), "lost")
	require.NoError(t, err)

	// Verify with a different identity key.
	otherPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	err = VerifyRevocation(cert, otherPub)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// IsDeviceRevoked and GetActiveDevices
// ---------------------------------------------------------------------------

func TestIsDeviceRevoked(t *testing.T) {
	cfg := newTestDeviceConfig(t, "revoked-check")
	dm := NewDeviceManager(cfg)
	_, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	rm := NewRevocationManager(&RevocationConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
	})

	deviceA := []byte("device-aaaaaaaaaa")[:16]
	deviceB := []byte("device-bbbbbbbbbb")[:16]
	deviceC := []byte("device-cccccccccc")[:16]

	doc := &pb.IdentityDocument{
		Devices: []*pb.DeviceCertificate{
			{DeviceId: deviceA},
			{DeviceId: deviceB},
		},
		Revocations: []*pb.RevocationCertificate{
			{RevokedKey: deviceA},
		},
	}

	tests := []struct {
		name     string
		deviceID []byte
		revoked  bool
	}{
		{"revoked device in revocation list", deviceA, true},
		{"active device not in revocation list", deviceB, false},
		{"unknown device not in device list", deviceC, true}, // implicitly revoked
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := rm.IsDeviceRevoked(doc, tc.deviceID)
			assert.Equal(t, tc.revoked, result)
		})
	}
}

func TestGetActiveDevices(t *testing.T) {
	deviceA := []byte("aaaaaaaaaaaaaaaa")
	deviceB := []byte("bbbbbbbbbbbbbbbb")
	deviceC := []byte("cccccccccccccccc")

	doc := &pb.IdentityDocument{
		Devices: []*pb.DeviceCertificate{
			{DeviceId: deviceA, DeviceName: "A"},
			{DeviceId: deviceB, DeviceName: "B"},
			{DeviceId: deviceC, DeviceName: "C"},
		},
		Revocations: []*pb.RevocationCertificate{
			{RevokedKey: deviceB},
		},
	}

	active := GetActiveDevices(doc)
	assert.Len(t, active, 2)

	names := make([]string, len(active))
	for i, d := range active {
		names[i] = d.DeviceName
	}
	assert.Contains(t, names, "A")
	assert.Contains(t, names, "C")
	assert.NotContains(t, names, "B")
}

func TestGetActiveDevices_NoRevocations(t *testing.T) {
	doc := &pb.IdentityDocument{
		Devices: []*pb.DeviceCertificate{
			{DeviceId: []byte("aaaaaaaaaaaaaaaa")},
			{DeviceId: []byte("bbbbbbbbbbbbbbbb")},
		},
	}

	active := GetActiveDevices(doc)
	assert.Len(t, active, 2, "all devices should be active when no revocations")
}

func TestGetActiveDevices_AllRevoked(t *testing.T) {
	id := []byte("aaaaaaaaaaaaaaaa")
	doc := &pb.IdentityDocument{
		Devices: []*pb.DeviceCertificate{
			{DeviceId: id},
		},
		Revocations: []*pb.RevocationCertificate{
			{RevokedKey: id},
		},
	}

	active := GetActiveDevices(doc)
	assert.Empty(t, active, "all devices revoked should yield empty list")
}

// ---------------------------------------------------------------------------
// Fanout manager creation
// ---------------------------------------------------------------------------

func TestNewFanoutManager(t *testing.T) {
	cfg := newTestDeviceConfig(t, "fanout-device")
	dm := NewDeviceManager(cfg)

	fm := NewFanoutManager(&FanoutConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
		IPFSNode:      nil, // No real IPFS node in unit tests.
	})

	require.NotNil(t, fm)
	assert.NotNil(t, fm.sessions)
	assert.NotNil(t, fm.symmetricKeys)

	// Stop should not panic.
	err := fm.Stop()
	assert.NoError(t, err)
}

func TestFanoutManager_SendMessageToIdentity_NoDevices(t *testing.T) {
	cfg := newTestDeviceConfig(t, "fanout-no-dev")
	dm := NewDeviceManager(cfg)
	_, err := dm.RegisterNewDevice()
	require.NoError(t, err)

	fm := NewFanoutManager(&FanoutConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
		IPFSNode:      nil,
	})

	result, err := fm.SendMessageToIdentity("hello", []byte("recipient"), nil)
	assert.Error(t, err, "sending to zero devices should fail")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no recipient devices")
}

func TestFanoutManager_SessionCreatesRatchet(t *testing.T) {
	cfg := newTestDeviceConfig(t, "ratchet-session")
	dm := NewDeviceManager(cfg)

	fm := NewFanoutManager(&FanoutConfig{
		DeviceManager: dm,
	})

	// Create a mock device certificate with valid X25519 key
	deviceDHPub := make([]byte, 32)
	deviceDHPub[0] = 9 // valid X25519 point
	deviceSignPub := make([]byte, 32)

	device := &pb.DeviceCertificate{
		DeviceId:      []byte("test-device-id00"),
		DeviceSignPub: deviceSignPub,
		DeviceDhPub:   deviceDHPub,
	}

	recipientIdentity := make([]byte, 32)
	recipientIdentity[0] = 1

	// Creating a session should establish X3DH and Double Ratchet
	session, err := fm.getOrCreateSession(recipientIdentity, device)
	assert.NoError(t, err, "session creation should succeed")
	assert.NotNil(t, session)
	assert.NotEqual(t, make([]byte, 32), session.SessionKey, "session key should not be zero-filled")

	// Verify ratchet state was created
	identityHex := hex.EncodeToString(recipientIdentity)
	deviceIDHex := hex.EncodeToString(device.DeviceId)
	ratchetState, ok := fm.getRatchetState(identityHex, deviceIDHex)
	assert.True(t, ok, "ratchet state should exist")
	assert.NotNil(t, ratchetState)

	// Getting the same session should return cached version
	session2, err := fm.getOrCreateSession(recipientIdentity, device)
	assert.NoError(t, err)
	assert.Equal(t, session, session2, "should return cached session")
}

// ---------------------------------------------------------------------------
// SyncManager creation and lifecycle
// ---------------------------------------------------------------------------

func TestNewSyncManager(t *testing.T) {
	cfg := newTestDeviceConfig(t, "sm-device")
	dm := NewDeviceManager(cfg)

	sm := NewSyncManager(&SyncManagerConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
	})

	require.NotNil(t, sm)
	assert.NotNil(t, sm.vectorClock)
	assert.NotNil(t, sm.eventChan)
	assert.NotNil(t, sm.historyRequests)
}

func TestSyncManager_StartStop(t *testing.T) {
	cfg := newTestDeviceConfig(t, "lifecycle")
	dm := NewDeviceManager(cfg)

	sm := NewSyncManager(&SyncManagerConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
	})

	err := sm.Start(cfg.IdentitySignPub)
	assert.NoError(t, err)

	err = sm.Stop()
	assert.NoError(t, err)
}

func TestSyncManager_EventsChannel(t *testing.T) {
	cfg := newTestDeviceConfig(t, "events")
	dm := NewDeviceManager(cfg)

	sm := NewSyncManager(&SyncManagerConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
	})

	ch := sm.Events()
	assert.NotNil(t, ch)
}

// ---------------------------------------------------------------------------
// RevocationManager creation
// ---------------------------------------------------------------------------

func TestNewRevocationManager(t *testing.T) {
	cfg := newTestDeviceConfig(t, "rm-device")
	dm := NewDeviceManager(cfg)

	rm := NewRevocationManager(&RevocationConfig{
		DeviceManager: dm,
		Storage:       newTestStorage(t),
		IPFSNode:      nil,
	})

	require.NotNil(t, rm)
}

// ---------------------------------------------------------------------------
// DeviceManager getters on unregistered manager
// ---------------------------------------------------------------------------

func TestDeviceManager_GettersBeforeRegistration(t *testing.T) {
	cfg := newTestDeviceConfig(t, "pre-reg")
	dm := NewDeviceManager(cfg)

	// Before registration, device-specific fields should be nil/empty.
	assert.Nil(t, dm.GetDeviceID())
	assert.Nil(t, dm.GetDeviceSignPub())
	assert.Nil(t, dm.GetDeviceSignPriv())
	assert.Nil(t, dm.GetDeviceDHPub())
	assert.Nil(t, dm.GetDeviceDHPriv())

	// Group key is set at construction time.
	assert.Equal(t, cfg.DeviceGroupKey, dm.GetDeviceGroupKey())
}

// ---------------------------------------------------------------------------
// Table-driven: DeviceIDToHex
// ---------------------------------------------------------------------------

func TestDeviceIDToHex_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{"empty", nil, ""},
		{"single byte", []byte{0xAB}, "ab"},
		{"two bytes", []byte{0x01, 0x02}, "0102"},
		{"16 bytes zeros", make([]byte, 16), "00000000000000000000000000000000"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, DeviceIDToHex(tc.input))
		})
	}
}
