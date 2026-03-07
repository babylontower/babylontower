package ipfsnode

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestBootstrapAnnouncement_MarshalUnmarshal(t *testing.T) {
	// Test request message
	request := BootstrapAnnouncement{
		Type:      BootstrapRequestType,
		PeerID:    "QmTest123",
		Timestamp: 1234567890,
		RequestID: "req-abc-123",
	}

	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	var unmarshaled BootstrapAnnouncement
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal request: %v", err)
	}

	if unmarshaled.Type != request.Type {
		t.Errorf("Type mismatch: expected %s, got %s", request.Type, unmarshaled.Type)
	}
	if unmarshaled.PeerID != request.PeerID {
		t.Errorf("PeerID mismatch: expected %s, got %s", request.PeerID, unmarshaled.PeerID)
	}
	if unmarshaled.RequestID != request.RequestID {
		t.Errorf("RequestID mismatch: expected %s, got %s", request.RequestID, unmarshaled.RequestID)
	}

	// Test response message
	response := BootstrapAnnouncement{
		Type:      BootstrapResponseType,
		PeerID:    "QmTest456",
		Timestamp: 1234567891,
		UptimeSecs: 300,
		PeerCount:  5,
		BootstrapPeers: []string{
			"/ip4/127.0.0.1/tcp/4001/p2p/QmPeer1",
			"/ip4/127.0.0.1/tcp/4002/p2p/QmPeer2",
		},
	}

	data, err = json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if unmarshaled.Type != response.Type {
		t.Errorf("Response Type mismatch")
	}
	if unmarshaled.UptimeSecs != response.UptimeSecs {
		t.Errorf("UptimeSecs mismatch: expected %d, got %d", response.UptimeSecs, unmarshaled.UptimeSecs)
	}
	if len(unmarshaled.BootstrapPeers) != len(response.BootstrapPeers) {
		t.Errorf("BootstrapPeers length mismatch")
	}
}

func TestDefaultBootstrapPubSubConfig(t *testing.T) {
	config := DefaultBootstrapPubSubConfig()

	if config.PubSubTopic != DefaultBootstrapPubSubTopic {
		t.Errorf("Expected default PubSubTopic %s, got %s", DefaultBootstrapPubSubTopic, config.PubSubTopic)
	}
	if config.PubSubListenSecs != DefaultPubSubListenSecs {
		t.Errorf("Expected default PubSubListenSecs %d, got %d", DefaultPubSubListenSecs, config.PubSubListenSecs)
	}
	if config.MinBabylonPeersRequired != DefaultMinBabylonPeersRequired {
		t.Errorf("Expected default MinBabylonPeersRequired %d, got %d", DefaultMinBabylonPeersRequired, config.MinBabylonPeersRequired)
	}
}

func TestBootstrapPubSub_Creation(t *testing.T) {
	config := DefaultBootstrapPubSubConfig()
	rateLimiter, _ := NewBootstrapRateLimiter(DefaultBootstrapRateLimitConfig(), nil)

	bp, err := NewBootstrapPubSub(nil, config, rateLimiter)
	if err != nil {
		t.Fatalf("Failed to create BootstrapPubSub: %v", err)
	}

	if bp == nil {
		t.Fatal("Expected non-nil BootstrapPubSub")
	}
	if bp.config != config {
		t.Error("Config not set correctly")
	}
	if bp.rateLimiter != rateLimiter {
		t.Error("Rate limiter not set correctly")
	}
}

func TestBootstrapPubSub_HelperLifecycle(t *testing.T) {
	config := DefaultBootstrapPubSubConfig()
	rateLimiter, _ := NewBootstrapRateLimiter(DefaultBootstrapRateLimitConfig(), nil)

	bp, err := NewBootstrapPubSub(nil, config, rateLimiter)
	if err != nil {
		t.Fatalf("Failed to create BootstrapPubSub: %v", err)
	}

	// Initially not running
	if bp.IsHelperRunning() {
		t.Error("Expected helper to not be running initially")
	}

	// Can't start without topic initialized (would fail in real scenario)
	// For unit test, we just verify the state transitions
	bp.helperRunning = true
	if !bp.IsHelperRunning() {
		t.Error("Expected helper to be running after setting flag")
	}

	bp.StopHelper()
	if bp.IsHelperRunning() {
		t.Error("Expected helper to not be running after StopHelper")
	}
}

func TestBootstrapPubSub_Uptime(t *testing.T) {
	config := DefaultBootstrapPubSubConfig()
	rateLimiter, _ := NewBootstrapRateLimiter(DefaultBootstrapRateLimitConfig(), nil)

	bp, err := NewBootstrapPubSub(nil, config, rateLimiter)
	if err != nil {
		t.Fatalf("Failed to create BootstrapPubSub: %v", err)
	}

	// Uptime should be very small initially
	uptime := bp.GetUptime()
	if uptime < 0 {
		t.Error("Expected non-negative uptime")
	}

	// Wait a bit and check again
	time.Sleep(100 * time.Millisecond)
	uptime2 := bp.GetUptime()
	if uptime2 < uptime {
		t.Error("Expected uptime to increase")
	}
}

func TestBootstrapAnnouncement_Types(t *testing.T) {
	// Verify type constants
	if BootstrapRequestType != "bootstrap_request" {
		t.Errorf("BootstrapRequestType mismatch: %s", BootstrapRequestType)
	}
	if BootstrapResponseType != "bootstrap_response" {
		t.Errorf("BootstrapResponseType mismatch: %s", BootstrapResponseType)
	}
	if DefaultBootstrapPubSubTopic != "/babylon/bootstrap" {
		t.Errorf("DefaultBootstrapPubSubTopic mismatch: %s", DefaultBootstrapPubSubTopic)
	}
}

func TestBootstrapPubSub_RequestBootstrap_ContextCancellation(t *testing.T) {
	config := DefaultBootstrapPubSubConfig()
	config.PubSubListenSecs = 1 // Short timeout for testing
	rateLimiter, _ := NewBootstrapRateLimiter(DefaultBootstrapRateLimitConfig(), nil)

	bp, err := NewBootstrapPubSub(nil, config, rateLimiter)
	if err != nil {
		t.Fatalf("Failed to create BootstrapPubSub: %v", err)
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should return empty list when context is cancelled
	peers := bp.RequestBootstrap(ctx)
	if peers != nil && len(peers) > 0 {
		t.Error("Expected empty peer list with cancelled context")
	}
}

func TestBootstrapConfig_Conversion(t *testing.T) {
	bootstrapConfig := &BootstrapConfig{
		PubSubTopic:             "/test/bootstrap",
		ResponseProbability:     0.75,
		MaxResponsesPerMinute:   50,
		RequestDedupWindowSecs:  60,
		MinUptimeSecs:           600,
		MinPeerCount:            5,
		MinRoutingTableSize:     20,
		StoredPeerTimeoutSecs:   30,
		PubSubListenSecs:        10,
		MinBabylonPeersRequired: 5,
	}

	// Test ToRateLimitConfig
	rateConfig := bootstrapConfig.ToRateLimitConfig()
	if rateConfig.ResponseProbability != 0.75 {
		t.Errorf("RateLimit ResponseProbability mismatch")
	}
	if rateConfig.MaxResponsesPerMinute != 50 {
		t.Errorf("RateLimit MaxResponsesPerMinute mismatch")
	}
	if rateConfig.RequestDedupWindow != 60*time.Second {
		t.Errorf("RateLimit RequestDedupWindow mismatch")
	}
	if rateConfig.MinUptimeSecs != 600 {
		t.Errorf("RateLimit MinUptimeSecs mismatch")
	}

	// Test ToPubSubConfig
	pubsubConfig := bootstrapConfig.ToPubSubConfig()
	if pubsubConfig.PubSubTopic != "/test/bootstrap" {
		t.Errorf("PubSubConfig PubSubTopic mismatch")
	}
	if pubsubConfig.PubSubListenSecs != 10 {
		t.Errorf("PubSubConfig PubSubListenSecs mismatch")
	}
	if pubsubConfig.MinBabylonPeersRequired != 5 {
		t.Errorf("PubSubConfig MinBabylonPeersRequired mismatch")
	}
}

func TestBootstrapConfig_NilConversion(t *testing.T) {
	var nilConfig *BootstrapConfig

	// Should return defaults when config is nil
	rateConfig := nilConfig.ToRateLimitConfig()
	if rateConfig == nil {
		t.Error("Expected non-nil rate config from nil BootstrapConfig")
	}

	pubsubConfig := nilConfig.ToPubSubConfig()
	if pubsubConfig == nil {
		t.Error("Expected non-nil pubsub config from nil BootstrapConfig")
	}
}

func TestDefaultBootstrapConfig(t *testing.T) {
	config := DefaultBootstrapConfig()

	if config.PubSubTopic != "/babylon/bootstrap" {
		t.Errorf("Default PubSubTopic mismatch")
	}
	if config.ResponseProbability != 0.5 {
		t.Errorf("Default ResponseProbability mismatch")
	}
	if config.MaxResponsesPerMinute != 30 {
		t.Errorf("Default MaxResponsesPerMinute mismatch")
	}
	if config.MinUptimeSecs != 300 {
		t.Errorf("Default MinUptimeSecs mismatch")
	}
	if config.MinPeerCount != 3 {
		t.Errorf("Default MinPeerCount mismatch")
	}
	if config.MinRoutingTableSize != 10 {
		t.Errorf("Default MinRoutingTableSize mismatch")
	}
}
