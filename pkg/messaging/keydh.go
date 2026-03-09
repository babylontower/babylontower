package messaging

import (
	"encoding/binary"
	"errors"
	"fmt"

	pb "babylontower/pkg/proto"

	"google.golang.org/protobuf/proto"
)

// encodeRatchetPayload encodes a RatchetHeader + ciphertext into the payload
// wire format per Protocol v1 §3.3:
//
//	[2 bytes: ratchet_header_length (big-endian uint16)]
//	[N bytes: serialized RatchetHeader]
//	[remaining: ciphertext]
func encodeRatchetPayload(ratchetHeader *pb.RatchetHeader, ciphertext []byte) ([]byte, error) {
	headerBytes, err := proto.Marshal(ratchetHeader)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal RatchetHeader: %w", err)
	}
	if len(headerBytes) > 0xFFFF {
		return nil, errors.New("RatchetHeader too large")
	}

	payload := make([]byte, 2+len(headerBytes)+len(ciphertext))
	binary.BigEndian.PutUint16(payload[0:2], uint16(len(headerBytes)))
	copy(payload[2:2+len(headerBytes)], headerBytes)
	copy(payload[2+len(headerBytes):], ciphertext)
	return payload, nil
}

// decodeRatchetPayload extracts the RatchetHeader and ciphertext from a
// Protocol v1 payload.
func decodeRatchetPayload(payload []byte) (*pb.RatchetHeader, []byte, error) {
	if len(payload) < 2 {
		return nil, nil, errors.New("payload too short for header length prefix")
	}

	headerLen := int(binary.BigEndian.Uint16(payload[0:2]))
	if 2+headerLen > len(payload) {
		return nil, nil, fmt.Errorf("payload too short: need %d, have %d", 2+headerLen, len(payload))
	}

	var header pb.RatchetHeader
	if err := proto.Unmarshal(payload[2:2+headerLen], &header); err != nil {
		return nil, nil, fmt.Errorf("failed to parse RatchetHeader: %w", err)
	}

	ciphertext := payload[2+headerLen:]
	return &header, ciphertext, nil
}

// isLegacyPayload detects the old payload format (nonce + ciphertext in payload,
// RatchetHeader in x3dh_header field). Used for backward compatibility during
// transition to Protocol v1 compliant format.
//
// Heuristic: old format has x3dh_header containing a RatchetHeader (small,
// typically <40 bytes with valid protobuf), and payload starting with a 24-byte
// nonce. New format has a 2-byte length prefix in payload.
func isLegacyPayload(x3dhHeader, payload []byte) bool {
	if len(x3dhHeader) == 0 {
		return false
	}
	// In the new format, x3dh_header contains X3DHHeader (has 32-byte
	// initiator_identity_dh_pub at field 1 and 32-byte ephemeral_pub at
	// field 2, so typically >= 66 bytes) or is empty for non-init messages.
	// Legacy format puts RatchetHeader (typically <40 bytes) in x3dh_header.
	// Try to parse as X3DHHeader — if ephemeral_pub is present, it's new format.
	var x3dh pb.X3DHHeader
	if err := proto.Unmarshal(x3dhHeader, &x3dh); err == nil && len(x3dh.EphemeralPub) == 32 {
		return false // Valid X3DHHeader = new format
	}
	return true // Assume legacy
}
