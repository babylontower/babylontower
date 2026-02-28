package protocol

import (
	"bytes"
	"crypto/ed25519"
	"testing"

	"babylontower/pkg/identity"
	pb "babylontower/pkg/proto"

	"github.com/tyler-smith/go-bip39"
	"google.golang.org/protobuf/proto"
)

// generateTestIdentity creates a test identity
func generateTestIdentity(t *testing.T) *identity.IdentityV1 {
	entropy, _ := bip39.NewEntropy(128)
	mnemonic, _ := bip39.NewMnemonic(entropy)
	id, err := identity.NewIdentityV1(mnemonic, "Test Device")
	if err != nil {
		t.Fatalf("Failed to create test identity: %v", err)
	}
	return id
}

// TestEnvelopeBuilder tests envelope construction and signing
func TestEnvelopeBuilder(t *testing.T) {
	sender := generateTestIdentity(t)
	recipient := generateTestIdentity(t)

	payload := []byte("encrypted test payload")

	envelope, err := NewEnvelopeBuilder(
		sender.IKSignPub,
		sender.DeviceID,
		sender.IKSignPriv,
	).
		MessageType(pb.MessageType_DM_TEXT).
		Recipient(recipient.IKSignPub).
		Payload(payload).
		Build()

	if err != nil {
		t.Fatalf("Failed to build envelope: %v", err)
	}

	// Verify envelope fields
	if envelope.ProtocolVersion != ProtocolVersion1 {
		t.Errorf("Expected protocol version %d, got %d", ProtocolVersion1, envelope.ProtocolVersion)
	}
	if envelope.MessageType != pb.MessageType_DM_TEXT {
		t.Errorf("Expected message type DM_TEXT, got %v", envelope.MessageType)
	}
	if !bytes.Equal(envelope.SenderIdentity, sender.IKSignPub) {
		t.Error("Sender identity mismatch")
	}
	if !bytes.Equal(envelope.RecipientIdentity, recipient.IKSignPub) {
		t.Error("Recipient identity mismatch")
	}
	if !bytes.Equal(envelope.Payload, payload) {
		t.Error("Payload mismatch")
	}
	if len(envelope.MessageId) != 16 {
		t.Errorf("Expected message ID length 16, got %d", len(envelope.MessageId))
	}
	if len(envelope.Signature) != ed25519.SignatureSize {
		t.Errorf("Expected signature length %d, got %d", ed25519.SignatureSize, len(envelope.Signature))
	}
}

// TestVerifyEnvelope tests envelope signature verification
func TestVerifyEnvelope(t *testing.T) {
	sender := generateTestIdentity(t)
	recipient := generateTestIdentity(t)

	envelope, _ := NewEnvelopeBuilder(
		sender.IKSignPub,
		sender.DeviceID,
		sender.IKSignPriv,
	).
		MessageType(pb.MessageType_DM_TEXT).
		Recipient(recipient.IKSignPub).
		Payload([]byte("test")).
		Build()

	// Valid envelope should verify
	err := VerifyEnvelope(envelope)
	if err != nil {
		t.Errorf("Valid envelope verification failed: %v", err)
	}

	// Tampered payload should fail
	envelope.Payload[0] ^= 0xFF
	err = VerifyEnvelope(envelope)
	if err == nil {
		t.Error("Expected verification failure for tampered payload")
	}
}

// TestDeriveDMTopic tests DM topic derivation
func TestDeriveDMTopic(t *testing.T) {
	recipient := generateTestIdentity(t)

	topic := DeriveDMTopic(recipient.IKSignPub)

	// Verify format
	expectedPrefix := TopicDMPrefix
	if len(topic) <= len(expectedPrefix) {
		t.Errorf("Topic too short: %s", topic)
	}
	if topic[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Topic has wrong prefix: expected %s, got %s", expectedPrefix, topic[:len(expectedPrefix)])
	}

	// Verify determinism
	topic2 := DeriveDMTopic(recipient.IKSignPub)
	if topic != topic2 {
		t.Error("Topic derivation not deterministic")
	}

	// Verify different recipients produce different topics
	recipient2 := generateTestIdentity(t)
	topic3 := DeriveDMTopic(recipient2.IKSignPub)
	if topic == topic3 {
		t.Error("Different recipients produced same topic")
	}
}

// TestDeriveGroupTopic tests group topic derivation
func TestDeriveGroupTopic(t *testing.T) {
	groupID := []byte("test-group-id-1234567890123456")

	topic := DeriveGroupTopic(groupID)

	expectedPrefix := TopicGroupPrefix
	if topic[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Group topic has wrong prefix: expected %s, got %s", expectedPrefix, topic[:len(expectedPrefix)])
	}

	// Verify determinism
	topic2 := DeriveGroupTopic(groupID)
	if topic != topic2 {
		t.Error("Group topic derivation not deterministic")
	}
}

// TestDeriveChannelTopic tests channel topic derivation
func TestDeriveChannelTopic(t *testing.T) {
	channelID := []byte("test-channel-id-123456789012345")

	topic := DeriveChannelTopic(channelID)

	expectedPrefix := TopicChannelPrefix
	if topic[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Channel topic has wrong prefix")
	}
}

// TestDeriveRevocationTopic tests revocation topic derivation
func TestDeriveRevocationTopic(t *testing.T) {
	identity := generateTestIdentity(t)

	topic := DeriveRevocationTopic(identity.IKSignPub)

	expectedPrefix := TopicRevocationPrefix
	if topic[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Revocation topic has wrong prefix")
	}
}

// TestDeriveSyncTopic tests device sync topic derivation
func TestDeriveSyncTopic(t *testing.T) {
	identity := generateTestIdentity(t)

	topic := DeriveSyncTopic(identity.IKSignPub)

	expectedPrefix := TopicSyncPrefix
	if topic[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Sync topic has wrong prefix")
	}
}

// TestParseDMPayload tests DM payload parsing
func TestParseDMPayload(t *testing.T) {
	textMsg := &pb.TextMessage{
		Text: "Hello, World!",
	}

	payload := &pb.DMPayload{
		RatchetHeader: &pb.RatchetHeader{
			DhRatchetPub:        make([]byte, 32),
			PreviousChainLength: 0,
			MessageNumber:       0,
		},
		Content: &pb.DMPayload_Text{
			Text: textMsg,
		},
	}

	payloadBytes, _ := proto.Marshal(payload)

	parsed, err := ParseDMPayload(payloadBytes)
	if err != nil {
		t.Fatalf("Failed to parse DM payload: %v", err)
	}

	if parsed.GetText() == nil {
		t.Error("Parsed payload text is nil")
	}
}

// TestParseX3DHHeader tests X3DH header parsing
func TestParseX3DHHeader(t *testing.T) {
	header := &pb.X3DHHeader{
		InitiatorIdentityDhPub: make([]byte, 32),
		EphemeralPub:           make([]byte, 32),
		SignedPrekeyId:         1,
		OneTimePrekeyId:        100,
		CipherSuiteId:          DefaultCipherSuite,
	}

	headerBytes, _ := proto.Marshal(header)

	parsed, err := ParseX3DHHeader(headerBytes)
	if err != nil {
		t.Fatalf("Failed to parse X3DH header: %v", err)
	}

	if parsed.SignedPrekeyId != 1 {
		t.Errorf("Expected SPK ID 1, got %d", parsed.SignedPrekeyId)
	}
	if parsed.OneTimePrekeyId != 100 {
		t.Errorf("Expected OPK ID 100, got %d", parsed.OneTimePrekeyId)
	}
}

// TestEnvelopeWithX3DHHeader tests envelope with X3DH header
func TestEnvelopeWithX3DHHeader(t *testing.T) {
	sender := generateTestIdentity(t)
	recipient := generateTestIdentity(t)

	x3dhHeader := &pb.X3DHHeader{
		InitiatorIdentityDhPub: sender.IKDHPub[:],
		EphemeralPub:           make([]byte, 32),
		SignedPrekeyId:         1,
		OneTimePrekeyId:        100,
		CipherSuiteId:          DefaultCipherSuite,
	}

	envelope, err := NewEnvelopeBuilder(
		sender.IKSignPub,
		sender.DeviceID,
		sender.IKSignPriv,
	).
		MessageType(pb.MessageType_CTRL_X3DH_INITIAL).
		Recipient(recipient.IKSignPub).
		X3DHHeader(x3dhHeader).
		Payload([]byte("encrypted initial message")).
		Build()

	if err != nil {
		t.Fatalf("Failed to build envelope with X3DH header: %v", err)
	}

	if len(envelope.X3DhHeader) == 0 {
		t.Error("X3DH header not set")
	}

	// Verify and parse header
	err = VerifyEnvelope(envelope)
	if err != nil {
		t.Errorf("Envelope verification failed: %v", err)
	}

	parsedHeader, err := ParseX3DHHeader(envelope.X3DhHeader)
	if err != nil {
		t.Fatalf("Failed to parse X3DH header: %v", err)
	}

	if parsedHeader.OneTimePrekeyId != 100 {
		t.Errorf("OPK ID mismatch: expected 100, got %d", parsedHeader.OneTimePrekeyId)
	}
}

// TestDeriveDMTopic_EmptyPubkey tests topic derivation with empty pubkey
func TestDeriveDMTopic_EmptyPubkey(t *testing.T) {
	// Should not panic, returns a deterministic topic for empty input
	topic := DeriveDMTopic(nil)
	if topic[:len(TopicDMPrefix)] != TopicDMPrefix {
		t.Errorf("Expected DM prefix, got %s", topic)
	}

	// Empty and nil should produce the same result
	topic2 := DeriveDMTopic([]byte{})
	if topic != topic2 {
		t.Error("nil and empty byte slice should produce same topic")
	}
}

// TestVerifyEnvelope_InvalidSenderLength tests verification with wrong key length
func TestVerifyEnvelope_InvalidSenderLength(t *testing.T) {
	env := &pb.BabylonEnvelope{
		SenderIdentity: []byte("short"),
		Signature:      make([]byte, 64),
	}

	err := VerifyEnvelope(env)
	if err == nil {
		t.Error("Expected error for invalid sender identity length")
	}
}

// TestEnvelopeBuilder_AllMessageTypes tests building envelopes with various message types
func TestEnvelopeBuilder_AllMessageTypes(t *testing.T) {
	sender := generateTestIdentity(t)
	recipient := generateTestIdentity(t)

	messageTypes := []pb.MessageType{
		pb.MessageType_DM_TEXT,
		pb.MessageType_CTRL_X3DH_INITIAL,
		pb.MessageType_GROUP_TEXT,
		pb.MessageType_CHANNEL_POST,
	}

	for _, mt := range messageTypes {
		t.Run(mt.String(), func(t *testing.T) {
			builder := NewEnvelopeBuilder(sender.IKSignPub, sender.DeviceID, sender.IKSignPriv).
				MessageType(mt).
				Payload([]byte("test"))

			if mt == pb.MessageType_GROUP_TEXT {
				builder.Group([]byte("group-id-12345678901234567890123"))
			} else {
				builder.Recipient(recipient.IKSignPub)
			}

			env, err := builder.Build()
			if err != nil {
				t.Fatalf("Build failed for %v: %v", mt, err)
			}
			if env.MessageType != mt {
				t.Errorf("Expected message type %v, got %v", mt, env.MessageType)
			}
			if err := VerifyEnvelope(env); err != nil {
				t.Errorf("Verify failed for %v: %v", mt, err)
			}
		})
	}
}

// TestEnvelopeBuilder_ChannelMessage tests envelope with channel ID
func TestEnvelopeBuilder_ChannelMessage(t *testing.T) {
	sender := generateTestIdentity(t)
	channelID := []byte("test-channel-id-12345678901234567")

	env, err := NewEnvelopeBuilder(sender.IKSignPub, sender.DeviceID, sender.IKSignPriv).
		MessageType(pb.MessageType_CHANNEL_POST).
		Channel(channelID).
		Payload([]byte("channel post")).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if !bytes.Equal(env.ChannelId, channelID) {
		t.Error("Channel ID mismatch")
	}
	if err := VerifyEnvelope(env); err != nil {
		t.Errorf("Verify failed: %v", err)
	}
}

// TestEnvelopeBuilder_EmptyPayload tests building an envelope with no payload
func TestEnvelopeBuilder_EmptyPayload(t *testing.T) {
	sender := generateTestIdentity(t)
	recipient := generateTestIdentity(t)

	env, err := NewEnvelopeBuilder(sender.IKSignPub, sender.DeviceID, sender.IKSignPriv).
		MessageType(pb.MessageType_DM_TEXT).
		Recipient(recipient.IKSignPub).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if len(env.Payload) != 0 {
		t.Error("Expected empty payload")
	}
	if err := VerifyEnvelope(env); err != nil {
		t.Errorf("Verify failed: %v", err)
	}
}

// TestParseDMPayload_Invalid tests parsing invalid DM payload
func TestParseDMPayload_Invalid(t *testing.T) {
	_, err := ParseDMPayload([]byte("not-protobuf"))
	if err == nil {
		t.Error("Expected error for invalid DM payload")
	}
}

// TestParseGroupPayload tests parsing a group payload
func TestParseGroupPayload(t *testing.T) {
	payload := &pb.GroupPayload{
		Epoch:      2,
		ChainIndex: 5,
		Content:    &pb.GroupPayload_Text{Text: &pb.TextMessage{Text: "hello group"}},
	}

	data, _ := proto.Marshal(payload)
	parsed, err := ParseGroupPayload(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if parsed.Epoch != 2 {
		t.Errorf("Expected epoch 2, got %d", parsed.Epoch)
	}
	if parsed.ChainIndex != 5 {
		t.Errorf("Expected chain_index 5, got %d", parsed.ChainIndex)
	}
}

// TestParseGroupPayload_Invalid tests parsing invalid group payload
func TestParseGroupPayload_Invalid(t *testing.T) {
	_, err := ParseGroupPayload([]byte("invalid"))
	if err == nil {
		t.Error("Expected error for invalid group payload")
	}
}

// TestParseX3DHHeader_Invalid tests parsing invalid X3DH header
func TestParseX3DHHeader_Invalid(t *testing.T) {
	_, err := ParseX3DHHeader([]byte("bad"))
	if err == nil {
		t.Error("Expected error for invalid X3DH header")
	}
}

// TestEnvelopeGroupMessage tests group envelope construction
func TestEnvelopeGroupMessage(t *testing.T) {
	sender := generateTestIdentity(t)
	groupID := []byte("test-group-123456789012345678901")

	groupPayload := &pb.GroupPayload{
		Epoch:      1,
		ChainIndex: 0,
		Content:    &pb.GroupPayload_Text{Text: &pb.TextMessage{Text: "Group message"}},
	}

	payloadBytes, _ := proto.Marshal(groupPayload)

	envelope, err := NewEnvelopeBuilder(
		sender.IKSignPub,
		sender.DeviceID,
		sender.IKSignPriv,
	).
		MessageType(pb.MessageType_GROUP_TEXT).
		Group(groupID).
		Payload(payloadBytes).
		Build()

	if err != nil {
		t.Fatalf("Failed to build group envelope: %v", err)
	}

	if !bytes.Equal(envelope.GroupId, groupID) {
		t.Error("Group ID mismatch")
	}
	if len(envelope.RecipientIdentity) != 0 {
		t.Error("Group message should not have recipient identity")
	}
}
