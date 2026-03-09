package messaging

import (
	"fmt"

	pb "babylontower/pkg/proto"
)

// ReceiveResult contains the result of processing a received message
type ReceiveResult struct {
	// Message is the decrypted plaintext message
	Message *pb.Message
	// SenderPubKey is the sender's Ed25519 public key
	SenderPubKey []byte
	// BabylonEnvelope is the original Protocol v1 envelope
	BabylonEnvelope *pb.BabylonEnvelope
}

// ReceiveMessage processes a raw envelope received from IPFS/PubSub.
// Delegates to processEnvelope which handles BabylonEnvelope parsing,
// signature verification, Double Ratchet decryption, and storage.
func (s *Service) ReceiveMessage(envelopeBytes []byte) (*ReceiveResult, error) {
	s.mu.RLock()
	if !s.isStarted {
		s.mu.RUnlock()
		return nil, ErrServiceNotStarted
	}
	s.mu.RUnlock()

	if err := s.processEnvelope(envelopeBytes); err != nil {
		return nil, fmt.Errorf("failed to process envelope: %w", err)
	}

	// The message was processed and emitted via messageChan.
	// Return a minimal result — callers that need the full message
	// should consume from Messages() channel instead.
	return &ReceiveResult{}, nil
}

// MessageWithMeta contains a message with metadata for display
type MessageWithMeta struct {
	Message    *pb.Message
	IsOutgoing bool
	Timestamp  uint64
}

// GetDecryptedMessagesWithMeta retrieves message history with direction metadata.
// Messages are stored as plaintext, so no decryption is needed at retrieval time.
func (s *Service) GetDecryptedMessagesWithMeta(contactPubKey []byte, limit, offset int) ([]*MessageWithMeta, error) {
	s.mu.RLock()
	if !s.isStarted {
		s.mu.RUnlock()
		return nil, ErrServiceNotStarted
	}
	s.mu.RUnlock()

	storedMsgs, err := s.storage.GetMessages(contactPubKey, limit, offset)
	if err != nil {
		return nil, err
	}

	messages := make([]*MessageWithMeta, 0, len(storedMsgs))
	for _, sm := range storedMsgs {
		messages = append(messages, &MessageWithMeta{
			Message: &pb.Message{
				Text:      sm.Text,
				Timestamp: sm.Timestamp,
			},
			IsOutgoing: sm.IsOutgoing,
			Timestamp:  sm.Timestamp,
		})
	}

	return messages, nil
}

// GetDecryptedMessages retrieves message history for a contact
// Deprecated: Use GetDecryptedMessagesWithMeta for proper direction handling
func (s *Service) GetDecryptedMessages(contactPubKey []byte, limit, offset int) ([]*pb.Message, error) {
	meta, err := s.GetDecryptedMessagesWithMeta(contactPubKey, limit, offset)
	if err != nil {
		return nil, err
	}

	messages := make([]*pb.Message, 0, len(meta))
	for _, m := range meta {
		messages = append(messages, m.Message)
	}
	return messages, nil
}
