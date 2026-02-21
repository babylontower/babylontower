package ipfsnode

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Message represents a PubSub message with metadata
type Message struct {
	// Data is the message payload
	Data []byte
	// From is the peer ID of the sender
	From peer.ID
	// Topic is the topic the message was received on
	Topic string
	// SeqNo is the sequence number of the message (as bytes)
	SeqNo []byte
}

// Subscription represents a subscription to a PubSub topic
type Subscription struct {
	topic   string
	psSub   *pubsub.Subscription
	cancel  context.CancelFunc
	mu      sync.RWMutex
	closed  bool
	msgChan chan *Message
	errChan chan error
}

// topicCache manages topic handles to avoid re-joining
type topicCache struct {
	mu     sync.RWMutex
	topics map[string]*pubsub.Topic
}

func newTopicCache() *topicCache {
	return &topicCache{
		topics: make(map[string]*pubsub.Topic),
	}
}

func (tc *topicCache) getOrJoin(ps *pubsub.PubSub, topic string) (*pubsub.Topic, error) {
	tc.mu.RLock()
	t, ok := tc.topics[topic]
	tc.mu.RUnlock()

	if ok {
		return t, nil
	}

	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Double-check after acquiring write lock
	if t, ok = tc.topics[topic]; ok {
		return t, nil
	}

	t, err := ps.Join(topic)
	if err != nil {
		return nil, err
	}

	tc.topics[topic] = t
	return t, nil
}

func (tc *topicCache) closeAll() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	for topic, t := range tc.topics {
		if err := t.Close(); err != nil {
			logger.Warnw("failed to close topic", "topic", topic, "error", err)
		}
		delete(tc.topics, topic)
	}
}

// Subscribe subscribes to a topic and returns a subscription
// The subscription receives messages via the Messages channel
func (n *Node) Subscribe(topic string) (*Subscription, error) {
	if !n.isStarted {
		return nil, ErrNodeNotStarted
	}

	if n.pubsub == nil {
		return nil, fmt.Errorf("PubSub not initialized")
	}

	// Join the topic (or get existing handle)
	t, err := n.topicCache.getOrJoin(n.pubsub, topic)
	if err != nil {
		return nil, fmt.Errorf("failed to join topic: %w", err)
	}

	// Subscribe to the topic
	sub, err := t.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	ctx, cancel := context.WithCancel(n.ctx)

	s := &Subscription{
		topic:   topic,
		psSub:   sub,
		cancel:  cancel,
		msgChan: make(chan *Message, 32),
		errChan: make(chan error, 1),
	}

	// Start message processing goroutine
	go s.processMessages(ctx)

	logger.Infow("subscribed to topic", "topic", topic)

	return s, nil
}

// Publish publishes data to a topic
// Returns the sequence number of the published message
func (n *Node) Publish(topic string, data []byte) error {
	if !n.isStarted {
		return ErrNodeNotStarted
	}

	if n.pubsub == nil {
		return fmt.Errorf("PubSub not initialized")
	}

	// Join the topic (or get existing handle)
	t, err := n.topicCache.getOrJoin(n.pubsub, topic)
	if err != nil {
		return fmt.Errorf("failed to join topic: %w", err)
	}

	// Publish the data
	if err := t.Publish(n.ctx, data); err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}

	logger.Debugw("published to topic", "topic", topic, "size", len(data))

	return nil
}

// PublishTo publishes data to a topic derived from a public key
// This is a convenience method for the messaging protocol
func (n *Node) PublishTo(pubKey []byte, data []byte) error {
	topic := TopicFromPublicKey(pubKey)
	return n.Publish(topic, data)
}

// ListTopics returns a list of currently joined topics
func (n *Node) ListTopics() []string {
	if n.pubsub == nil {
		return nil
	}
	return n.pubsub.GetTopics()
}

// ListPeers returns peers subscribed to a topic
func (n *Node) ListPeers(topic string) []peer.ID {
	if n.pubsub == nil {
		return nil
	}
	return n.pubsub.ListPeers(topic)
}

// processMessages reads messages from the PubSub subscription
// and forwards them to the message channel
func (s *Subscription) processMessages(ctx context.Context) {
	defer func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.msgChan)
		close(s.errChan)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := s.psSub.Next(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				select {
				case s.errChan <- err:
				default:
				}
				continue
			}

			// Skip messages from self (optional, can be disabled)
			// if msg.ReceivedFrom == s.psSub.Topic().

			message := &Message{
				Data:  msg.Data,
				From:  msg.ReceivedFrom,
				Topic: msg.GetTopic(),
				SeqNo: msg.Seqno,
			}

			select {
			case s.msgChan <- message:
			case <-ctx.Done():
				return
			}
		}
	}
}

// Messages returns the channel for receiving messages
func (s *Subscription) Messages() <-chan *Message {
	return s.msgChan
}

// Errors returns the channel for receiving errors
func (s *Subscription) Errors() <-chan error {
	return s.errChan
}

// Topic returns the topic name
func (s *Subscription) Topic() string {
	return s.topic
}

// Close unsubscribes from the topic and closes channels
func (s *Subscription) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	s.cancel()

	if s.psSub != nil {
		s.psSub.Cancel()
	}

	logger.Infow("unsubscribed from topic", "topic", s.topic)

	return nil
}

// IsClosed returns true if the subscription is closed
func (s *Subscription) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// MarshalJSON implements json.Marshaler for Message
func (m *Message) MarshalJSON() ([]byte, error) {
	seqNoStr := ""
	if len(m.SeqNo) > 0 {
		seqNoStr = hex.EncodeToString(m.SeqNo)
	}
	return json.Marshal(map[string]interface{}{
		"data":  m.Data,
		"from":  m.From.String(),
		"topic": m.Topic,
		"seqno": seqNoStr,
	})
}

// String returns a string representation of the message
func (m *Message) String() string {
	return fmt.Sprintf("Message{topic=%s, from=%s, len=%d}",
		m.Topic, m.From.String(), len(m.Data))
}
