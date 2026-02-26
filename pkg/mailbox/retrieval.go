package mailbox

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"

	pb "babylontower/pkg/proto"
	"babylontower/pkg/crypto"
	"babylontower/pkg/identity"
)

// RetrievalHandler handles message retrieval by recipients
type RetrievalHandler struct {
	host     host.Host
	identity *identity.Identity
	storage  *Storage
}

// NewRetrievalHandler creates a new retrieval handler
func NewRetrievalHandler(h host.Host, id *identity.Identity, storage *Storage) *RetrievalHandler {
	// Handler is integrated into DepositHandler's stream handler
	return &RetrievalHandler{
		host:     h,
		identity: id,
		storage:  storage,
	}
}

// handleRetrievalRequest processes a retrieval request
func (dh *DepositHandler) handleRetrievalRequest(ctx context.Context, reader *bufio.Reader, writer *bufio.Writer, s network.Stream) {
	// Read length prefix
	lengthBytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, lengthBytes); err != nil {
		return
	}
	length := binary.BigEndian.Uint32(lengthBytes)

	// Read retrieval request
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return
	}

	req := &pb.RetrievalRequest{}
	if err := proto.Unmarshal(data, req); err != nil {
		dh.writeRetrievalErrorResponse(writer, []byte{}, "invalid request format")
		return
	}

	// Validate request
	if err := dh.validateRetrievalRequest(req); err != nil {
		dh.writeRetrievalErrorResponse(writer, req.Nonce, err.Error())
		return
	}

	// Fetch messages from storage
	messages, err := dh.storage.ListMessages(req.RecipientPubkey)
	if err != nil {
		dh.writeRetrievalErrorResponse(writer, req.Nonce, fmt.Sprintf("storage error: %v", err))
		return
	}

	// Build response
	var messageIDs [][]byte
	var envelopes [][]byte
	for _, msg := range messages {
		messageIDs = append(messageIDs, msg.MessageID)
		envelopes = append(envelopes, msg.Envelope)
	}

	resp := &pb.RetrievalResponse{
		Nonce:      req.Nonce,
		MessageIds: messageIDs,
		Envelopes:  envelopes,
		Count:      uint64(len(messages)),
	}

	// Sign response
	signature, err := dh.signRetrievalResponse(resp)
	if err != nil {
		dh.writeRetrievalErrorResponse(writer, req.Nonce, "signing error")
		return
	}
	resp.MailboxSignature = signature

	// Send response
	if err := dh.writeResponse(writer, 0x02, resp); err != nil {
		return
	}
	if err := writer.Flush(); err != nil {
		return
	}
}

// handleAckRequest processes an acknowledgment request
func (dh *DepositHandler) handleAckRequest(ctx context.Context, reader *bufio.Reader, writer *bufio.Writer, s network.Stream) {
	// Read length prefix
	lengthBytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, lengthBytes); err != nil {
		return
	}
	length := binary.BigEndian.Uint32(lengthBytes)

	// Read ack request
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return
	}

	req := &pb.AcknowledgmentRequest{}
	if err := proto.Unmarshal(data, req); err != nil {
		dh.writeAckErrorResponse(writer, nil, "invalid request format")
		return
	}

	// Validate request
	if err := dh.validateAckRequest(req); err != nil {
		dh.writeAckErrorResponse(writer, req.MessageIds, err.Error())
		return
	}

	// Delete acknowledged messages
	if err := dh.storage.DeleteMessages(req.RecipientPubkey, req.MessageIds); err != nil {
		dh.writeAckErrorResponse(writer, req.MessageIds, fmt.Sprintf("deletion error: %v", err))
		return
	}

	// Build success response
	resp := &pb.AcknowledgmentResponse{
		MessageIds: req.MessageIds,
		Success:    true,
	}

	// Sign response
	signature, err := dh.signAckResponse(resp)
	if err != nil {
		dh.writeAckErrorResponse(writer, req.MessageIds, "signing error")
		return
	}
	resp.MailboxSignature = signature

	// Send response
	if err := dh.writeResponse(writer, 0x03, resp); err != nil {
		return
	}
	if err := writer.Flush(); err != nil {
		return
	}
}

// validateRetrievalRequest validates a retrieval request
func (dh *DepositHandler) validateRetrievalRequest(req *pb.RetrievalRequest) error {
	if len(req.RecipientPubkey) != 32 {
		return fmt.Errorf("invalid recipient pubkey length")
	}

	if len(req.Nonce) != 32 {
		return fmt.Errorf("invalid nonce length")
	}

	if len(req.RecipientSignature) == 0 {
		return fmt.Errorf("missing recipient signature")
	}

	// Verify signature
	canonical := &pb.RetrievalRequest{
		RecipientPubkey: req.RecipientPubkey,
		Nonce:           req.Nonce,
		Timestamp:       req.Timestamp,
	}
	_, err := proto.Marshal(canonical)
	if err != nil {
		return fmt.Errorf("failed to marshal for verification")
	}

	// Note: In PoC we skip actual signature verification
	// TODO: Verify Ed25519 signature against req.RecipientPubkey

	return nil
}

// validateAckRequest validates an acknowledgment request
func (dh *DepositHandler) validateAckRequest(req *pb.AcknowledgmentRequest) error {
	if len(req.RecipientPubkey) != 32 {
		return fmt.Errorf("invalid recipient pubkey length")
	}

	if len(req.RecipientSignature) == 0 {
		return fmt.Errorf("missing recipient signature")
	}

	// Verify signature
	canonical := &pb.AcknowledgmentRequest{
		RecipientPubkey: req.RecipientPubkey,
		MessageIds:      req.MessageIds,
		Timestamp:       req.Timestamp,
	}
	_, err := proto.Marshal(canonical)
	if err != nil {
		return fmt.Errorf("failed to marshal for verification")
	}

	// Note: In PoC we skip actual signature verification
	// TODO: Verify Ed25519 signature against req.RecipientPubkey

	return nil
}

// writeRetrievalErrorResponse writes an error response for retrieval
func (dh *DepositHandler) writeRetrievalErrorResponse(writer *bufio.Writer, nonce []byte, reason string) {
	resp := &pb.RetrievalResponse{
		Nonce: nonce,
		Count: 0,
	}
	_ = dh.writeResponse(writer, 0x02, resp)
}

// writeAckErrorResponse writes an error response for acknowledgment
func (dh *DepositHandler) writeAckErrorResponse(writer *bufio.Writer, messageIDs [][]byte, reason string) {
	resp := &pb.AcknowledgmentResponse{
		MessageIds:    messageIDs,
		Success:       false,
		FailureReason: reason,
	}
	_ = dh.writeResponse(writer, 0x03, resp)
}

// signRetrievalResponse signs a retrieval response
func (dh *DepositHandler) signRetrievalResponse(resp *pb.RetrievalResponse) ([]byte, error) {
	canonical := &pb.RetrievalResponse{
		Nonce: resp.Nonce,
		Count: resp.Count,
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

// signAckResponse signs an acknowledgment response
func (dh *DepositHandler) signAckResponse(resp *pb.AcknowledgmentResponse) ([]byte, error) {
	canonical := &pb.AcknowledgmentResponse{
		MessageIds: resp.MessageIds,
		Success:    resp.Success,
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

// RetrieveFromMailbox retrieves messages from a remote mailbox node
func RetrieveFromMailbox(ctx context.Context, h host.Host, mailboxPeerID string, recipientIdentity *identity.Identity) (*pb.RetrievalResponse, error) {
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
	defer s.Close()

	writer := bufio.NewWriter(s)
	reader := bufio.NewReader(s)

	// Generate random nonce for challenge
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Create retrieval request
	req := &pb.RetrievalRequest{
		RecipientPubkey: recipientIdentity.Ed25519PubKey,
		Nonce:           nonce,
		Timestamp:       uint64(time.Now().Unix()),
	}

	// Sign request
	signature, err := crypto.Sign(recipientIdentity.Ed25519PrivKey, mustMarshal(req))
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}
	req.RecipientSignature = signature

	// Send request
	if err := writeRequest(writer, 0x02, req); err != nil {
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

	if msgType != 0x02 {
		return nil, fmt.Errorf("unexpected response type")
	}

	resp := &pb.RetrievalResponse{}
	if err := readResponse(reader, resp); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return resp, nil
}

// AcknowledgeMessages acknowledges and deletes messages from a mailbox
func AcknowledgeMessages(ctx context.Context, h host.Host, mailboxPeerID string, recipientIdentity *identity.Identity, messageIDs [][]byte) (*pb.AcknowledgmentResponse, error) {
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
	defer s.Close()

	writer := bufio.NewWriter(s)
	reader := bufio.NewReader(s)

	// Create ack request
	req := &pb.AcknowledgmentRequest{
		RecipientPubkey: recipientIdentity.Ed25519PubKey,
		MessageIds:      messageIDs,
		Timestamp:       uint64(time.Now().Unix()),
	}

	// Sign request
	signature, err := crypto.Sign(recipientIdentity.Ed25519PrivKey, mustMarshal(req))
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}
	req.RecipientSignature = signature

	// Send request
	if err := writeRequest(writer, 0x03, req); err != nil {
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

	if msgType != 0x03 {
		return nil, fmt.Errorf("unexpected response type")
	}

	resp := &pb.AcknowledgmentResponse{}
	if err := readResponse(reader, resp); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return resp, nil
}
