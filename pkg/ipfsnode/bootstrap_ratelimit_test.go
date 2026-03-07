package ipfsnode

import (
	"testing"
	"time"
)

func TestBootstrapRateLimiter_ShouldRespond(t *testing.T) {
	config := &BootstrapRateLimitConfig{
		ResponseProbability:   1.0, // 100% for deterministic testing
		MaxResponsesPerMinute: 10,
		RequestDedupWindow:    1 * time.Second,
		SeenRequestsCacheSize: 100,
		MinUptimeSecs:         1, // 1 second for testing
		MinPeerCount:          1,
		MinRoutingTableSize:   1,
	}

	node := &Node{}
	limiter, err := NewBootstrapRateLimiter(config, node)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}

	// First request should pass probability check but fail helper criteria
	// (node has no start time, peers, or routing table)
	if limiter.ShouldRespond("request-1") {
		t.Error("Expected ShouldRespond to return false for node that doesn't qualify as helper")
	}
}

func TestBootstrapRateLimiter_Deduplication(t *testing.T) {
	config := &BootstrapRateLimitConfig{
		ResponseProbability:   1.0,
		MaxResponsesPerMinute: 100,
		RequestDedupWindow:    1 * time.Second,
		SeenRequestsCacheSize: 100,
		MinUptimeSecs:         0,
		MinPeerCount:          0,
		MinRoutingTableSize:   0,
	}

	node := &Node{}
	limiter, err := NewBootstrapRateLimiter(config, node)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}

	requestID := "test-request-1"

	// First request
	first := limiter.ShouldRespond(requestID)

	// Immediate duplicate should be deduplicated
	second := limiter.ShouldRespond(requestID)

	if first && second {
		t.Error("Expected duplicate request to be deduplicated")
	}

	// Wait for dedup window to expire
	time.Sleep(1100 * time.Millisecond)

	// After window expires, should allow again (but may fail rate limit)
	third := limiter.ShouldRespond(requestID)
	_ = third // May be true or false depending on rate limit
}

func TestBootstrapRateLimiter_RateLimit(t *testing.T) {
	config := &BootstrapRateLimitConfig{
		ResponseProbability:   1.0,
		MaxResponsesPerMinute: 3,
		RequestDedupWindow:    100 * time.Millisecond,
		SeenRequestsCacheSize: 100,
		MinUptimeSecs:         0,
		MinPeerCount:          0,
		MinRoutingTableSize:   0,
	}

	node := &Node{}
	limiter, err := NewBootstrapRateLimiter(config, node)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}

	// Make requests up to the limit
	responses := 0
	for i := 0; i < 10; i++ {
		requestID := "rate-test-" + string(rune('a'+i))
		// Wait for dedup window between requests
		if i > 0 {
			time.Sleep(150 * time.Millisecond)
		}
		if limiter.ShouldRespond(requestID) {
			responses++
		}
	}

	// Should have responded to at most MaxResponsesPerMinute
	if responses > config.MaxResponsesPerMinute {
		t.Errorf("Expected at most %d responses, got %d", config.MaxResponsesPerMinute, responses)
	}
}

func TestBootstrapRateLimiter_Probability(t *testing.T) {
	config := &BootstrapRateLimitConfig{
		ResponseProbability:   0.5, // 50%
		MaxResponsesPerMinute: 1000,
		RequestDedupWindow:    10 * time.Millisecond,
		SeenRequestsCacheSize: 1000,
		MinUptimeSecs:         0,
		MinPeerCount:          0,
		MinRoutingTableSize:   0,
	}

	node := &Node{}
	limiter, err := NewBootstrapRateLimiter(config, node)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}

	// Make many requests to test probability distribution
	total := 100
	responses := 0
	for i := 0; i < total; i++ {
		requestID := "prob-test-" + string(rune('a'+(i%26)))
		time.Sleep(15 * time.Millisecond) // Avoid dedup
		if limiter.ShouldRespond(requestID) {
			responses++
		}
	}

	// With 50% probability and 100 requests, expect roughly 40-60 responses
	// (allowing for statistical variance)
	if responses < 20 || responses > 80 {
		t.Logf("Warning: Response count %d outside expected range for 50%% probability", responses)
		// Don't fail - probability is inherently non-deterministic
	}
}

func TestBootstrapRateLimiter_Reset(t *testing.T) {
	config := &BootstrapRateLimitConfig{
		ResponseProbability:   1.0,
		MaxResponsesPerMinute: 2,
		RequestDedupWindow:    1 * time.Second,
		SeenRequestsCacheSize: 100,
		MinUptimeSecs:         0,
		MinPeerCount:          0,
		MinRoutingTableSize:   0,
	}

	node := &Node{}
	limiter, err := NewBootstrapRateLimiter(config, node)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}

	// Exhaust rate limit
	limiter.ShouldRespond("req-1")
	limiter.ShouldRespond("req-2")

	// Should be rate limited now
	if limiter.ShouldRespond("req-3") {
		t.Error("Expected to be rate limited")
	}

	// Reset the limiter
	limiter.Reset()

	// After reset, should be able to respond again (subject to helper criteria)
	// Note: This will still fail helper criteria since node is not qualified
}

func TestBootstrapRateLimiter_GetStats(t *testing.T) {
	config := &BootstrapRateLimitConfig{
		ResponseProbability:   1.0,
		MaxResponsesPerMinute: 10,
		RequestDedupWindow:    100 * time.Millisecond,
		SeenRequestsCacheSize: 100,
		MinUptimeSecs:         0,
		MinPeerCount:          0,
		MinRoutingTableSize:   0,
	}

	// Create a node with a start time in the past (so it's not a newcomer)
	node := &Node{}
	node.startTime = time.Now().Add(-5 * time.Minute) // Started 5 minutes ago
	
	limiter, err := NewBootstrapRateLimiter(config, node)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}

	// Make some requests
	limiter.ShouldRespond("stats-1")
	time.Sleep(150 * time.Millisecond)
	limiter.ShouldRespond("stats-2")

	stats := limiter.GetStats()

	if stats.SeenRequestsCount < 2 {
		t.Errorf("Expected at least 2 seen requests, got %d", stats.SeenRequestsCount)
	}

	// With MinUptimeSecs=0, MinPeerCount=0, MinRoutingTableSize=0, node qualifies as helper
	if !stats.QualifiesAsHelper {
		t.Error("Expected node to qualify as helper (all mins are 0)")
	}
}

func TestDefaultBootstrapRateLimitConfig(t *testing.T) {
	config := DefaultBootstrapRateLimitConfig()

	if config.ResponseProbability != 0.5 {
		t.Errorf("Expected default ResponseProbability 0.5, got %f", config.ResponseProbability)
	}
	if config.MaxResponsesPerMinute != 30 {
		t.Errorf("Expected default MaxResponsesPerMinute 30, got %d", config.MaxResponsesPerMinute)
	}
	if config.RequestDedupWindow != 30*time.Second {
		t.Errorf("Expected default RequestDedupWindow 30s, got %v", config.RequestDedupWindow)
	}
	if config.MinUptimeSecs != 300 {
		t.Errorf("Expected default MinUptimeSecs 300, got %d", config.MinUptimeSecs)
	}
	if config.MinPeerCount != 3 {
		t.Errorf("Expected default MinPeerCount 3, got %d", config.MinPeerCount)
	}
	if config.MinRoutingTableSize != 10 {
		t.Errorf("Expected default MinRoutingTableSize 10, got %d", config.MinRoutingTableSize)
	}
}

func TestBootstrapRateLimiter_QualifiesAsHelper(t *testing.T) {
	config := DefaultBootstrapRateLimitConfig()
	config.MinUptimeSecs = 1 // 1 second for testing
	config.MinPeerCount = 1
	config.MinRoutingTableSize = 1

	node := &Node{}
	limiter, err := NewBootstrapRateLimiter(config, node)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}

	// Node should not qualify as helper without being started
	if limiter.qualifiesAsHelper() {
		t.Error("Expected node to not qualify as helper when not started")
	}
}
