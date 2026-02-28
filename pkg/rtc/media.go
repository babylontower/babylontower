package rtc

import (
	"context"
	"errors"
	"sync"
)

// logger is declared in session.go for this package

var (
	// ErrMediaNotStarted is returned when media operations are attempted on a stopped service
	ErrMediaNotStarted = errors.New("media service not started")
	// ErrNoMediaKey is returned when trying to start media without a derived media key
	ErrNoMediaKey = errors.New("no media key available")
)

// MediaService handles RTC media transport over libp2p streams
// This is a stub implementation for Phase 15 - actual WebRTC integration would go here
type MediaService struct {
	session *CallSession
	stats   *MediaStats

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex

	// Callbacks
	onMediaStarted func()
	onMediaStopped func()
}

// MediaStats holds media transmission statistics
type MediaStats struct {
	mu sync.RWMutex

	// Audio stats
	AudioPacketsSent     uint64
	AudioPacketsReceived uint64
	AudioBytesSent       uint64
	AudioBytesReceived   uint64

	// Video stats
	VideoPacketsSent     uint64
	VideoPacketsReceived uint64
	VideoBytesSent       uint64
	VideoBytesReceived   uint64

	// Quality metrics
	JitterMs       float64
	PacketLossRate float64
	RTTMs          float64
}

// NewMediaService creates a new media service
func NewMediaService() *MediaService {
	ctx, cancel := context.WithCancel(context.Background())

	return &MediaService{
		ctx:   ctx,
		cancel: cancel,
		stats: &MediaStats{},
	}
}

// Start begins the media service
func (m *MediaService) Start() error {
	logger.Info("Media service started (stub)")
	return nil
}

// Stop gracefully stops the media service
func (m *MediaService) Stop() {
	m.cancel()
	m.wg.Wait()
	logger.Info("Media service stopped")
}

// StartOutgoing initiates an outgoing media stream to a peer
// This is a stub - actual implementation would use WebRTC
func (m *MediaService) StartOutgoing(session *CallSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session.MediaKey == nil {
		return ErrNoMediaKey
	}

	m.session = session

	logger.Info("Media stream established (stub)")

	if m.onMediaStarted != nil {
		m.onMediaStarted()
	}

	return nil
}

// SendAudio sends audio data over the media stream
func (m *MediaService) SendAudio(data []byte) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.session == nil {
		return 0, ErrMediaNotStarted
	}

	// Track statistics
	m.stats.mu.Lock()
	m.stats.AudioPacketsSent++
	m.stats.AudioBytesSent += uint64(len(data))
	m.stats.mu.Unlock()

	return len(data), nil
}

// SendVideo sends video data over the media stream
func (m *MediaService) SendVideo(data []byte) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.session == nil {
		return 0, ErrMediaNotStarted
	}

	// Track statistics
	m.stats.mu.Lock()
	m.stats.VideoPacketsSent++
	m.stats.VideoBytesSent += uint64(len(data))
	m.stats.mu.Unlock()

	return len(data), nil
}

// SendRTCP sends an RTCP control packet
func (m *MediaService) SendRTCP(packet []byte) error {
	if m.session == nil {
		return ErrMediaNotStarted
	}

	logger.Debugw("sending RTCP packet", "size", len(packet))
	return nil
}

// GetStats returns media statistics
func (m *MediaService) GetStats() *MediaStats {
	m.stats.mu.RLock()
	defer m.stats.mu.RUnlock()

	return &MediaStats{
		AudioPacketsSent:     m.stats.AudioPacketsSent,
		AudioPacketsReceived: m.stats.AudioPacketsReceived,
		AudioBytesSent:       m.stats.AudioBytesSent,
		AudioBytesReceived:   m.stats.AudioBytesReceived,
		VideoPacketsSent:     m.stats.VideoPacketsSent,
		VideoPacketsReceived: m.stats.VideoPacketsReceived,
		VideoBytesSent:       m.stats.VideoBytesSent,
		VideoBytesReceived:   m.stats.VideoBytesReceived,
		JitterMs:             m.stats.JitterMs,
		PacketLossRate:       m.stats.PacketLossRate,
		RTTMs:                m.stats.RTTMs,
	}
}

// SetOnMediaStarted sets the callback for when media starts
func (m *MediaService) SetOnMediaStarted(callback func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onMediaStarted = callback
}

// SetOnMediaStopped sets the callback for when media stops
func (m *MediaService) SetOnMediaStopped(callback func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onMediaStopped = callback
}

// UpdateStats updates media statistics
func (m *MediaService) UpdateStats(jitter, packetLoss, rtt float64) {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	m.stats.JitterMs = jitter
	m.stats.PacketLossRate = packetLoss
	m.stats.RTTMs = rtt
}

// IsStreaming returns true if media is currently being transmitted
func (m *MediaService) IsStreaming() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.session != nil
}

// GetSession returns the current call session
func (m *MediaService) GetSession() *CallSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.session
}

// SetSession sets the call session for this media stream
func (m *MediaService) SetSession(session *CallSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session = session
}
