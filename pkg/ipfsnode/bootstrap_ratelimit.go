package ipfsnode

import (
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

// BootstrapRateLimitConfig holds configuration for bootstrap response rate limiting
type BootstrapRateLimitConfig struct {
	// ResponseProbability is the probability of responding to a bootstrap request (0.0-1.0)
	// Default: 0.5 (50% of requests get a response)
	ResponseProbability float64 `yaml:"response_probability"`

	// MaxResponsesPerMinute is the maximum number of responses allowed per minute
	// Default: 30
	MaxResponsesPerMinute int `yaml:"max_responses_per_minute"`

	// RequestDedupWindow is the time window for request deduplication
	// Duplicate requests within this window are ignored
	// Default: 30s
	RequestDedupWindow time.Duration `yaml:"request_dedup_window"`

	// SeenRequestsCacheSize is the size of the LRU cache for deduplication
	// Default: 1000
	SeenRequestsCacheSize int `yaml:"seen_requests_cache_size"`

	// MinUptimeSecs is the minimum uptime required to qualify as a helper node
	// Default: 300 (5 minutes)
	MinUptimeSecs int `yaml:"min_uptime_secs"`

	// MinPeerCount is the minimum number of connected peers required to qualify as a helper
	// Default: 3
	MinPeerCount int `yaml:"min_peer_count"`

	// MinRoutingTableSize is the minimum DHT routing table size required to qualify as a helper
	// Default: 10
	MinRoutingTableSize int `yaml:"min_routing_table_size"`
}

// DefaultBootstrapRateLimitConfig returns a BootstrapRateLimitConfig with sensible defaults
func DefaultBootstrapRateLimitConfig() *BootstrapRateLimitConfig {
	return &BootstrapRateLimitConfig{
		ResponseProbability:   0.5,
		MaxResponsesPerMinute: 30,
		RequestDedupWindow:    30 * time.Second,
		SeenRequestsCacheSize: 1000,
		MinUptimeSecs:         300, // 5 minutes
		MinPeerCount:          3,
		MinRoutingTableSize:   10,
	}
}

// BootstrapRateLimiter controls the rate of bootstrap responses to prevent broadcast storms
// and ensure only stable nodes participate as helpers
type BootstrapRateLimiter struct {
	mu sync.RWMutex

	// LRU cache for request deduplication (requestID -> first seen time)
	seenRequests *lru.Cache[string, time.Time]

	// Timestamps of recent responses for rate limiting
	responseTimes []time.Time

	// Configuration
	config *BootstrapRateLimitConfig

	// Reference to parent node for checking helper criteria
	node *Node
}

// NewBootstrapRateLimiter creates a new rate limiter with the given configuration
func NewBootstrapRateLimiter(config *BootstrapRateLimitConfig, node *Node) (*BootstrapRateLimiter, error) {
	if config == nil {
		config = DefaultBootstrapRateLimitConfig()
	}

	// Create LRU cache for deduplication
	cache, err := lru.New[string, time.Time](config.SeenRequestsCacheSize)
	if err != nil {
		return nil, err
	}

	return &BootstrapRateLimiter{
		seenRequests:  cache,
		responseTimes: make([]time.Time, 0, config.MaxResponsesPerMinute),
		config:        config,
		node:          node,
	}, nil
}

// ShouldRespond determines if we should respond to a bootstrap request
// Returns true if:
// - Request is not a duplicate within the dedup window
// - We haven't exceeded the rate limit
// - Probabilistic check passes
// - Node qualifies as a helper (uptime, peers, routing table)
func (rl *BootstrapRateLimiter) ShouldRespond(requestID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Check if node qualifies as a helper
	if !rl.qualifiesAsHelper() {
		logger.Debugw("not responding to bootstrap request - node does not qualify as helper",
			"request_id", requestID)
		return false
	}

	// Check for duplicate request
	if rl.isDuplicate(requestID, now) {
		logger.Debugw("not responding to bootstrap request - duplicate",
			"request_id", requestID)
		return false
	}

	// Check rate limit
	if !rl.withinRateLimit(now) {
		logger.Debugw("not responding to bootstrap request - rate limited",
			"request_id", requestID)
		return false
	}

	// Probabilistic response
	if !rl.passesProbabilityCheck() {
		logger.Debugw("not responding to bootstrap request - probability check failed",
			"request_id", requestID)
		return false
	}

	// Record this request and response
	rl.recordRequest(requestID, now)
	rl.recordResponse(now)

	logger.Debugw("responding to bootstrap request",
		"request_id", requestID)
	return true
}

// isDuplicate checks if this request was seen within the dedup window
func (rl *BootstrapRateLimiter) isDuplicate(requestID string, now time.Time) bool {
	if firstSeen, ok := rl.seenRequests.Get(requestID); ok {
		if now.Sub(firstSeen) < rl.config.RequestDedupWindow {
			return true
		}
	}
	return false
}

// withinRateLimit checks if we're under the response rate limit
func (rl *BootstrapRateLimiter) withinRateLimit(now time.Time) bool {
	// Clean old response times (older than 1 minute)
	cutoff := now.Add(-time.Minute)
	validTimes := make([]time.Time, 0, len(rl.responseTimes))
	for _, t := range rl.responseTimes {
		if t.After(cutoff) {
			validTimes = append(validTimes, t)
		}
	}
	rl.responseTimes = validTimes

	// Check if under limit
	return len(rl.responseTimes) < rl.config.MaxResponsesPerMinute
}

// passesProbabilityCheck performs probabilistic response selection
func (rl *BootstrapRateLimiter) passesProbabilityCheck() bool {
	// Use crypto/rand for better randomness in production
	// For now, use time-based pseudo-random for simplicity
	now := time.Now()
	seed := now.UnixNano() % 1000
	prob := float64(seed) / 1000.0
	return prob < rl.config.ResponseProbability
}

// recordRequest records a request in the dedup cache
func (rl *BootstrapRateLimiter) recordRequest(requestID string, now time.Time) {
	rl.seenRequests.Add(requestID, now)
}

// recordResponse records a response timestamp for rate limiting
func (rl *BootstrapRateLimiter) recordResponse(now time.Time) {
	rl.responseTimes = append(rl.responseTimes, now)
}

// qualifiesAsHelper checks if the node meets the criteria to be a bootstrap helper
// Criteria:
// - Uptime >= MinUptimeSecs (if MinUptimeSecs > 0)
// - Connected peers >= MinPeerCount (if MinPeerCount > 0)
// - DHT routing table size >= MinRoutingTableSize (if MinRoutingTableSize > 0)
//
// NEWCOMER GRACE PERIOD: Nodes that started recently (< 2 minutes ago) can still
// help other newcomers with relaxed criteria. This solves the chicken-and-egg problem
// where two new nodes need to help each other bootstrap.
func (rl *BootstrapRateLimiter) qualifiesAsHelper() bool {
	if rl.node == nil {
		return false
	}

	startTime := rl.node.GetStartTime()
	if startTime.IsZero() {
		return false
	}

	uptime := time.Since(startTime)
	isNewcomer := uptime < 2*time.Minute

	// If this is a newcomer, use relaxed criteria
	// Newcomers can help other newcomers with just 1 peer and basic connectivity
	if isNewcomer {
		// Newcomers can always respond during their first 2 minutes
		// This enables mutual bootstrap between new nodes
		return true
	}

	// For established nodes, check full criteria

	// Check uptime (only if MinUptimeSecs > 0)
	if rl.config.MinUptimeSecs > 0 {
		if int(uptime.Seconds()) < rl.config.MinUptimeSecs {
			return false
		}
	}

	// Check connected peers (only if MinPeerCount > 0)
	if rl.config.MinPeerCount > 0 {
		peerCount := len(rl.node.host.Network().Peers())
		if peerCount < rl.config.MinPeerCount {
			return false
		}
	}

	// Check DHT routing table size (only if MinRoutingTableSize > 0)
	if rl.config.MinRoutingTableSize > 0 {
		if rl.node.dht == nil {
			return false
		}
		routingTableSize := len(rl.node.dht.RoutingTable().ListPeers())
		if routingTableSize < rl.config.MinRoutingTableSize {
			return false
		}
	}

	return true
}

// GetStats returns current rate limiter statistics
func (rl *BootstrapRateLimiter) GetStats() BootstrapRateLimiterStats {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-time.Minute)
	validResponses := 0
	for _, t := range rl.responseTimes {
		if t.After(cutoff) {
			validResponses++
		}
	}

	return BootstrapRateLimiterStats{
		ResponsesLastMinute: validResponses,
		SeenRequestsCount:   rl.seenRequests.Len(),
		QualifiesAsHelper:   rl.qualifiesAsHelper(),
	}
}

// BootstrapRateLimiterStats contains statistics about the rate limiter
type BootstrapRateLimiterStats struct {
	ResponsesLastMinute int  `json:"responses_last_minute"`
	SeenRequestsCount   int  `json:"seen_requests_count"`
	QualifiesAsHelper   bool `json:"qualifies_as_helper"`
}

// Reset clears all rate limiter state
// Useful for testing or manual recovery
func (rl *BootstrapRateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.seenRequests.Purge()
	rl.responseTimes = make([]time.Time, 0, rl.config.MaxResponsesPerMinute)
}
