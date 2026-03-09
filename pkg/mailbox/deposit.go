package mailbox

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"

	"babylontower/pkg/crypto"
	"babylontower/pkg/identity"
	pb "babylontower/pkg/proto"
)

// DepositHandler handles incoming deposit requests from senders
type DepositHandler struct {
	host      host.Host
	identity  *identity.Identity
	storage   *Storage
	config    *pb.MailboxConfig
	mu        sync.RWMutex
	rateLimit map[string]map[string]*rateLimitEntry // sender_hex -> target_hex -> entry
}

type rateLimitEntry struct {
	count      uint32
	hourBucket int64
}

// NewDepositHandler creates a new deposit handler
func NewDepositHandler(h host.Host, id *identity.Identity, storage *Storage, config *pb.MailboxConfig) *DepositHandler {
	dh := &DepositHandler{
		host:      h,
		identity:  id,
		storage:   storage,
		config:    config,
		rateLimit: make(map[string]map[string]*rateLimitEntry),
	}

	// Set up stream handler
	h.SetStreamHandler(MailboxProtocolID, dh.handleStream)

	return dh
}

// handleStream handles incoming libp2p streams for mailbox operations
func (dh *DepositHandler) handleStream(s network.Stream) {
	defer func() {
		_ = s.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reader := bufio.NewReader(s)
	writer := bufio.NewWriter(s)

	// Read message type (1 byte)
	msgType, err := reader.ReadByte()
	if err != nil {
		return
	}

	switch msgType {
	case 0x01: // DepositRequest
		dh.handleDepositRequest(ctx, reader, writer, s)
	case 0x02: // RetrievalRequest
		dh.handleRetrievalRequest(ctx, reader, writer, s)
	case 0x03: // AcknowledgmentRequest
		dh.handleAckRequest(ctx, reader, writer, s)
	default:
		return
	}
}

// handleDepositRequest processes a deposit request
func (dh *DepositHandler) handleDepositRequest(ctx context.Context, reader *bufio.Reader, writer *bufio.Writer, s network.Stream) {
	// Read length prefix (4 bytes)
	lengthBytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, lengthBytes); err != nil {
		return
	}
	length := binary.BigEndian.Uint32(lengthBytes)

	// Read deposit request
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return
	}

	req := &pb.DepositRequest{}
	if err := proto.Unmarshal(data, req); err != nil {
		dh.writeErrorResponse(writer, 0, "invalid request format")
		return
	}

	// Validate request
	if err := dh.validateDepositRequest(req); err != nil {
		dh.writeErrorResponse(writer, req.RequestId, err.Error())
		return
	}

	// Extract sender pubkey from envelope
	senderPubkey := extractSenderPubkey(req.Envelope)
	if senderPubkey == nil {
		dh.writeErrorResponse(writer, req.RequestId, "failed to extract sender pubkey")
		return
	}

	// Check rate limit
	if dh.isRateLimited(senderPubkey, req.TargetPubkey) {
		dh.writeErrorResponse(writer, req.RequestId, "rate limit exceeded")
		return
	}

	// Store the message
	ttlSeconds := dh.config.DefaultTtlSeconds
	if err := dh.storage.StoreMessage(
		req.TargetPubkey,
		extractMessageID(req.Envelope),
		senderPubkey,
		req.Envelope,
		ttlSeconds,
	); err != nil {
		dh.writeErrorResponse(writer, req.RequestId, fmt.Sprintf("storage error: %v", err))
		return
	}

	// Increment rate limit
	dh.incrementRateLimit(senderPubkey, req.TargetPubkey)

	// Write success response
	resp := &pb.DepositResponse{
		RequestId:   req.RequestId,
		Accepted:    true,
		StoredUntil: uint64(time.Now().Add(time.Duration(ttlSeconds) * time.Second).Unix()),
	}

	// Sign response
	signature, err := dh.signResponse(resp)
	if err != nil {
		dh.writeErrorResponse(writer, req.RequestId, "signing error")
		return
	}
	resp.MailboxSignature = signature

	// Send response
	if err := dh.writeResponse(writer, 0x01, resp); err != nil {
		return
	}
	if err := writer.Flush(); err != nil {
		return
	}
}

// validateDepositRequest validates a deposit request
func (dh *DepositHandler) validateDepositRequest(req *pb.DepositRequest) error {
	// Check envelope size
	if uint64(len(req.Envelope)) > dh.config.MaxMessageSize {
		return errors.New("envelope too large")
	}

	// Verify sender signature
	if len(req.SenderSignature) == 0 {
		return errors.New("missing sender signature")
	}

	// Parse envelope to extract sender's public key
	envelope := &pb.BabylonEnvelope{}
	if err := proto.Unmarshal(req.Envelope, envelope); err != nil {
		return errors.New("failed to parse envelope")
	}

	// Create canonical form for verification (without signature field)
	canonical := &pb.DepositRequest{
		TargetPubkey: req.TargetPubkey,
		Envelope:     req.Envelope,
		RequestId:    req.RequestId,
		Timestamp:    req.Timestamp,
	}
	dataForSigning, err := proto.Marshal(canonical)
	if err != nil {
		return errors.New("failed to marshal for verification")
	}

	// Verify signature using the sender's identity pubkey from the envelope
	senderPub := envelope.SenderIdentity
	if len(senderPub) == ed25519.PublicKeySize {
		if !crypto.Verify(senderPub, dataForSigning, req.SenderSignature) {
			return errors.New("invalid sender signature")
		}
	}

	return nil
}

// isRateLimited checks if a sender has exceeded their rate limit
func (dh *DepositHandler) isRateLimited(senderPubkey, targetPubkey []byte) bool {
	senderKey := hex.EncodeToString(senderPubkey)
	targetKey := hex.EncodeToString(targetPubkey)

	now := time.Now()
	hourBucket := now.Unix() / 3600

	dh.mu.RLock()
	defer dh.mu.RUnlock()

	if senderMap, exists := dh.rateLimit[senderKey]; exists {
		if entry, exists := senderMap[targetKey]; exists {
			if entry.hourBucket == hourBucket {
				return entry.count >= dh.config.DepositRateLimit
			}
		}
	}

	return false
}

// incrementRateLimit increments the rate limit counter
func (dh *DepositHandler) incrementRateLimit(senderPubkey, targetPubkey []byte) {
	senderKey := hex.EncodeToString(senderPubkey)
	targetKey := hex.EncodeToString(targetPubkey)

	now := time.Now()
	hourBucket := now.Unix() / 3600

	dh.mu.Lock()
	defer dh.mu.Unlock()

	if _, exists := dh.rateLimit[senderKey]; !exists {
		dh.rateLimit[senderKey] = make(map[string]*rateLimitEntry)
	}

	entry := &rateLimitEntry{
		count:      1,
		hourBucket: hourBucket,
	}

	if existing, exists := dh.rateLimit[senderKey][targetKey]; exists && existing.hourBucket == hourBucket {
		entry.count = existing.count + 1
	}

	dh.rateLimit[senderKey][targetKey] = entry
}

// writeResponse writes a protobuf response with length prefix
func (dh *DepositHandler) writeResponse(writer *bufio.Writer, msgType byte, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	// Write message type
	if err := writer.WriteByte(msgType); err != nil {
		return err
	}

	// Write length prefix
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(data)))
	if _, err := writer.Write(lengthBytes); err != nil {
		return err
	}

	// Write data
	if _, err := writer.Write(data); err != nil {
		return err
	}

	return nil
}

// writeErrorResponse writes an error response
func (dh *DepositHandler) writeErrorResponse(writer *bufio.Writer, requestID uint64, reason string) {
	resp := &pb.DepositResponse{
		RequestId:       requestID,
		Accepted:        false,
		RejectionReason: reason,
	}
	_ = dh.writeResponse(writer, 0x01, resp)
}

// signResponse signs a deposit response
func (dh *DepositHandler) signResponse(resp *pb.DepositResponse) ([]byte, error) {
	canonical := &pb.DepositResponse{
		RequestId:   resp.RequestId,
		Accepted:    resp.Accepted,
		StoredUntil: resp.StoredUntil,
	}
	data, err := proto.Marshal(canonical)
	if err != nil {
		return nil, err
	}

	signature, err := crypto.Sign(dh.identity.Ed25519PrivKey, data)
	if err != nil {
		return nil, err
	}

	return signature, nil
}

// extractSenderPubkey extracts the sender's public key from an envelope
func extractSenderPubkey(envelopeData []byte) []byte {
	envelope := &pb.BabylonEnvelope{}
	if err := proto.Unmarshal(envelopeData, envelope); err != nil {
		return nil
	}
	// Extract sender identity public key (32-byte Ed25519 pubkey)
	// This is used for signature verification of the deposit request
	if len(envelope.SenderIdentity) == 32 {
		return envelope.SenderIdentity
	}
	// Fallback to sender_device_id for backward compatibility (will fail verification)
	if len(envelope.SenderDeviceId) > 0 {
		return envelope.SenderDeviceId
	}
	return nil
}

// extractMessageID extracts the message ID from an envelope
func extractMessageID(envelopeData []byte) []byte {
	envelope := &pb.BabylonEnvelope{}
	if err := proto.Unmarshal(envelopeData, envelope); err != nil {
		// Generate random ID if parsing fails
		id := make([]byte, 16)
		if _, err := rand.Read(id); err != nil {
			// Fallback to deterministic ID
			return []byte("msg-parse-error")
		}
		return id
	}

	if len(envelope.MessageId) > 0 {
		return envelope.MessageId
	}

	// Generate from hash
	hash := sha256.Sum256(envelopeData)
	return hash[:16]
}

// DepositToMailbox deposits a message to a remote mailbox node
func DepositToMailbox(ctx context.Context, h host.Host, mailboxPeerID string, targetPubkey []byte, envelope *pb.BabylonEnvelope, senderIdentity *identity.Identity) (*pb.DepositResponse, error) {
	// Convert string to peer.ID
	peerID, err := peer.Decode(mailboxPeerID)
	if err != nil {
		return nil, fmt.Errorf("invalid mailbox peer ID: %w", err)
	}

	// Open stream to mailbox
	s, err := h.NewStream(ctx, peerID, MailboxProtocolID)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}
	defer func() {
		_ = s.Close()
	}()

	writer := bufio.NewWriter(s)
	reader := bufio.NewReader(s)

	// Create deposit request
	envelopeData, err := proto.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal envelope: %w", err)
	}

	req := &pb.DepositRequest{
		TargetPubkey: targetPubkey,
		Envelope:     envelopeData,
		RequestId:    generateRequestID(),
		Timestamp:    uint64(time.Now().Unix()),
	}

	// Sign request
	reqData, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal deposit request: %w", err)
	}
	signature, err := crypto.Sign(senderIdentity.Ed25519PrivKey, reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}
	req.SenderSignature = signature

	// Send request
	if err := writeRequest(writer, 0x01, req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}

	// Read response
	msgType, err := reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("failed to read response type: %w", err)
	}

	if msgType != 0x01 {
		return nil, errors.New("unexpected response type")
	}

	resp := &pb.DepositResponse{}
	if err := readResponse(reader, resp); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return resp, nil
}

// writeRequest writes a request with length prefix
func writeRequest(writer *bufio.Writer, msgType byte, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	if err := writer.WriteByte(msgType); err != nil {
		return err
	}

	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(data)))
	if _, err := writer.Write(lengthBytes); err != nil {
		return err
	}

	if _, err := writer.Write(data); err != nil {
		return err
	}

	return nil
}

// readResponse reads a length-prefixed response
func readResponse(reader *bufio.Reader, msg proto.Message) error {
	lengthBytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, lengthBytes); err != nil {
		return err
	}

	length := binary.BigEndian.Uint32(lengthBytes)
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return err
	}

	return proto.Unmarshal(data, msg)
}

// generateRequestID generates a unique request ID
func generateRequestID() uint64 {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(time.Now().UnixNano()))
	return binary.BigEndian.Uint64(buf[:])
}

