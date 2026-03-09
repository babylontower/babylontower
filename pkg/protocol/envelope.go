// Package protocol implements Protocol v1 wire format handling
package protocol

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	pb "babylontower/pkg/proto"

	"google.golang.org/protobuf/proto"
)

// Protocol version constants
const (
	ProtocolVersion1   = 1
	DefaultCipherSuite = 0x0001
)

// Topic prefixes for Protocol v1 routing
const (
	TopicDMPrefix          = "babylon-dm-"
	TopicGroupPrefix       = "babylon-grp-"
	TopicPublicGroupPrefix = "babylon-pub-"
	TopicChannelPrefix     = "babylon-ch-"
	TopicRevocationPrefix  = "babylon-rev-"
	TopicSyncPrefix        = "babylon-sync-"
)

// EnvelopeBuilder helps construct BabylonEnvelope messages
type EnvelopeBuilder struct {
	messageType       pb.MessageType
	senderIdentity    ed25519.PublicKey
	senderDeviceID    []byte
	senderPrivKey     ed25519.PrivateKey
	recipientIdentity ed25519.PublicKey
	groupID           []byte
	channelID         []byte
	payload           []byte
	x3dhHeader        *pb.X3DHHeader
	cipherSuiteID     uint32
}

// NewEnvelopeBuilder creates a new envelope builder
func NewEnvelopeBuilder(
	senderIdentity ed25519.PublicKey,
	senderDeviceID []byte,
	senderPrivKey ed25519.PrivateKey,
) *EnvelopeBuilder {
	return &EnvelopeBuilder{
		senderIdentity: senderIdentity,
		senderDeviceID: senderDeviceID,
		senderPrivKey:  senderPrivKey,
		cipherSuiteID:  DefaultCipherSuite,
	}
}

// MessageType sets the message type
func (b *EnvelopeBuilder) MessageType(mt pb.MessageType) *EnvelopeBuilder {
	b.messageType = mt
	return b
}

// Recipient sets the recipient identity
func (b *EnvelopeBuilder) Recipient(recipientIdentity ed25519.PublicKey) *EnvelopeBuilder {
	b.recipientIdentity = recipientIdentity
	return b
}

// Group sets the group ID for group messages
func (b *EnvelopeBuilder) Group(groupID []byte) *EnvelopeBuilder {
	b.groupID = groupID
	return b
}

// Channel sets the channel ID for channel messages
func (b *EnvelopeBuilder) Channel(channelID []byte) *EnvelopeBuilder {
	b.channelID = channelID
	return b
}

// Payload sets the encrypted payload
func (b *EnvelopeBuilder) Payload(payload []byte) *EnvelopeBuilder {
	b.payload = payload
	return b
}

// X3DHHeader sets the X3DH header for session initialization
func (b *EnvelopeBuilder) X3DHHeader(header *pb.X3DHHeader) *EnvelopeBuilder {
	b.x3dhHeader = header
	return b
}

// CipherSuite sets the cipher suite ID
func (b *EnvelopeBuilder) CipherSuite(suiteID uint32) *EnvelopeBuilder {
	b.cipherSuiteID = suiteID
	return b
}

// NegotiateCipherSuite selects the highest mutually supported cipher suite
// Per spec section 2.1: max(intersect(own_supported, recipient_supported))
// Returns the negotiated cipher suite ID or DefaultCipherSuite if no intersection
func NegotiateCipherSuite(ownSupported, recipientSupported []string) uint32 {
	// For protocol v1, we only have one mandatory cipher suite
	// This function is future-proof for when we add more suites
	
	// Define cipher suite priority (higher = better)
	cipherPriority := map[string]uint32{
		"BT-X25519-XChaCha20Poly1305-SHA256": 0x0001,
		"BT-X25519-AES256GCM-SHA256":         0x0002,
	}
	
	// Build set of recipient supported suites
	recipientSet := make(map[string]bool)
	for _, suite := range recipientSupported {
		recipientSet[suite] = true
	}
	
	// Find highest priority mutually supported suite
	var bestSuite uint32 = 0
	var bestPriority int = -1
	
	for _, suite := range ownSupported {
		if recipientSet[suite] {
			priority, ok := cipherPriority[suite]
			if ok && int(priority) > bestPriority {
				bestPriority = int(priority)
				bestSuite = priority
			}
		}
	}
	
	if bestSuite == 0 {
		// Fall back to default if no intersection
		return DefaultCipherSuite
	}
	
	return bestSuite
}

// ParseCipherSuiteFromID converts a cipher suite ID to its string representation
func ParseCipherSuiteFromID(suiteID uint32) string {
	switch suiteID {
	case 0x0001:
		return "BT-X25519-XChaCha20Poly1305-SHA256"
	case 0x0002:
		return "BT-X25519-AES256GCM-SHA256"
	default:
		return ""
	}
}

// GetCipherSuiteID converts a cipher suite string to its ID
func GetCipherSuiteID(suiteName string) uint32 {
	switch suiteName {
	case "BT-X25519-XChaCha20Poly1305-SHA256":
		return 0x0001
	case "BT-X25519-AES256GCM-SHA256":
		return 0x0002
	default:
		return DefaultCipherSuite
	}
}

// Build constructs and signs the BabylonEnvelope
func (b *EnvelopeBuilder) Build() (*pb.BabylonEnvelope, error) {
	// Generate message ID
	messageID := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, messageID); err != nil {
		return nil, fmt.Errorf("failed to generate message ID: %w", err)
	}

	// Serialize X3DH header if present
	var x3dhHeaderBytes []byte
	if b.x3dhHeader != nil {
		var err error
		x3dhHeaderBytes, err = proto.Marshal(b.x3dhHeader)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal X3DH header: %w", err)
		}
	}

	envelope := &pb.BabylonEnvelope{
		ProtocolVersion:   ProtocolVersion1,
		MessageType:       b.messageType,
		SenderIdentity:    b.senderIdentity,
		RecipientIdentity: b.recipientIdentity,
		Timestamp:         uint64(time.Now().Unix()),
		MessageId:         messageID,
		GroupId:           b.groupID,
		ChannelId:         b.channelID,
		Payload:           b.payload,
		SenderDeviceId:    b.senderDeviceID,
		X3DhHeader:        x3dhHeaderBytes,
		CipherSuiteId:     b.cipherSuiteID,
	}

	// Sign the envelope (fields 1-10)
	signature, err := b.signEnvelope(envelope)
	if err != nil {
		return nil, fmt.Errorf("failed to sign envelope: %w", err)
	}
	envelope.Signature = signature

	return envelope, nil
}

// signEnvelope signs the envelope fields 1-10
func (b *EnvelopeBuilder) signEnvelope(env *pb.BabylonEnvelope) ([]byte, error) {
	data, err := SerializeEnvelopeForSigning(env)
	if err != nil {
		return nil, err
	}
	signature := ed25519.Sign(b.senderPrivKey, data)
	return signature, nil
}

// SerializeEnvelopeForSigning serializes BabylonEnvelope fields 1-10 for signing.
// This is the canonical serialization used by both signing and verification.
func SerializeEnvelopeForSigning(env *pb.BabylonEnvelope) ([]byte, error) {
	data := make([]byte, 0, 256)

	// Field 1: protocol_version (4 bytes)
	verBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(verBytes, env.ProtocolVersion)
	data = append(data, verBytes...)

	// Field 2: message_type (4 bytes)
	typeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(typeBytes, uint32(env.MessageType))
	data = append(data, typeBytes...)

	// Field 3: sender_identity (32 bytes)
	data = append(data, env.SenderIdentity...)

	// Field 4: recipient_identity (32 bytes if present)
	if len(env.RecipientIdentity) > 0 {
		data = append(data, env.RecipientIdentity...)
	}

	// Field 5: timestamp (8 bytes)
	tsBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(tsBytes, env.Timestamp)
	data = append(data, tsBytes...)

	// Field 6: message_id (16 bytes)
	data = append(data, env.MessageId...)

	// Field 7: group_id (32 bytes if present)
	if len(env.GroupId) > 0 {
		data = append(data, env.GroupId...)
	}

	// Field 8: channel_id (32 bytes if present)
	if len(env.ChannelId) > 0 {
		data = append(data, env.ChannelId...)
	}

	// Field 9: payload (prefixed with length)
	if len(env.Payload) > 0 {
		lenBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBytes, uint32(len(env.Payload)))
		data = append(data, lenBytes...)
		data = append(data, env.Payload...)
	}

	// Field 10: sender_device_id (16 bytes)
	data = append(data, env.SenderDeviceId...)

	return data, nil
}

// VerifyEnvelope verifies an envelope signature
func VerifyEnvelope(env *pb.BabylonEnvelope) error {
	if len(env.SenderIdentity) != ed25519.PublicKeySize {
		return errors.New("invalid sender identity length")
	}

	data, err := SerializeEnvelopeForSigning(env)
	if err != nil {
		return fmt.Errorf("failed to serialize for verification: %w", err)
	}

	if !ed25519.Verify(env.SenderIdentity, data, env.Signature) {
		return errors.New("invalid envelope signature")
	}

	return nil
}

// DeriveDMTopic derives the DM topic from recipient identity
func DeriveDMTopic(recipientIdentity ed25519.PublicKey) string {
	hash := sha256.Sum256(recipientIdentity)
	hexPrefix := hex.EncodeToString(hash[:8])
	return TopicDMPrefix + hexPrefix
}

// DeriveGroupTopic derives the private group topic from group ID
func DeriveGroupTopic(groupID []byte) string {
	hash := sha256.Sum256(groupID)
	hexPrefix := hex.EncodeToString(hash[:8])
	return TopicGroupPrefix + hexPrefix
}

// DerivePublicGroupTopic derives the public group topic from group ID
// Per §4.3: Public groups MUST use "babylon-pub-" prefix (distinct from "babylon-grp-")
func DerivePublicGroupTopic(groupID []byte) string {
	hash := sha256.Sum256(groupID)
	hexPrefix := hex.EncodeToString(hash[:8])
	return TopicPublicGroupPrefix + hexPrefix
}

// DeriveChannelTopic derives the channel topic from channel ID
func DeriveChannelTopic(channelID []byte) string {
	hash := sha256.Sum256(channelID)
	hexPrefix := hex.EncodeToString(hash[:8])
	return TopicChannelPrefix + hexPrefix
}

// DeriveRevocationTopic derives the revocation topic from identity
func DeriveRevocationTopic(identityPub ed25519.PublicKey) string {
	hash := sha256.Sum256(identityPub)
	hexPrefix := hex.EncodeToString(hash[:8])
	return TopicRevocationPrefix + hexPrefix
}

// DeriveSyncTopic derives the device sync topic from root identity
func DeriveSyncTopic(rootIdentityPub ed25519.PublicKey) string {
	hash := sha256.Sum256(rootIdentityPub)
	hexPrefix := hex.EncodeToString(hash[:8])
	return TopicSyncPrefix + hexPrefix
}

// ParseDMPayload parses a DM payload
func ParseDMPayload(payloadBytes []byte) (*pb.DMPayload, error) {
	var payload pb.DMPayload
	if err := proto.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal DM payload: %w", err)
	}
	return &payload, nil
}

// ParseGroupPayload parses a group payload
func ParseGroupPayload(payloadBytes []byte) (*pb.GroupPayload, error) {
	var payload pb.GroupPayload
	if err := proto.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal group payload: %w", err)
	}
	return &payload, nil
}

// ParseX3DHHeader parses an X3DH header
func ParseX3DHHeader(headerBytes []byte) (*pb.X3DHHeader, error) {
	var header pb.X3DHHeader
	if err := proto.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("failed to unmarshal X3DH header: %w", err)
	}
	return &header, nil
}
