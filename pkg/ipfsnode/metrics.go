// Package ipfsnode provides metrics collection for network health monitoring.
package ipfsnode

import (
	"sync"
	"time"
)

// NetworkMetrics contains comprehensive network health statistics
type NetworkMetrics struct {
	// Node info
	PeerID              string `json:"peer_id"`
	
	// Connection metrics
	TotalConnections    int64 `json:"total_connections"`
	TotalDisconnections int64 `json:"total_disconnections"`
	CurrentConnections  int32 `json:"current_connections"`
	
	// Discovery metrics
	DHTDiscoveries      int64 `json:"dht_discoveries"`
	MDNSDiscoveries     int64 `json:"mdns_discoveries"`
	PeerExchangeDiscoveries int64 `json:"peer_exchange_discoveries"`
	
	// Bootstrap metrics
	BootstrapAttempts   int64 `json:"bootstrap_attempts"`
	BootstrapSuccesses  int64 `json:"bootstrap_successes"`
	LastBootstrapTime   time.Time `json:"last_bootstrap_time"`
	
	// Connection quality metrics
	AverageLatencyMs    int64 `json:"average_latency_ms"`
	SuccessfulMessages  int64 `json:"successful_messages"`
	FailedMessages      int64 `json:"failed_messages"`
	
	// Timing
	StartTime           time.Time `json:"start_time"`
	UptimeSeconds       int64 `json:"uptime_seconds"`
}

// MetricsCollector collects and aggregates network metrics
type MetricsCollector struct {
	mu sync.RWMutex
	metrics NetworkMetrics
	
	// Connection tracking
	connectionHistory []ConnectionEvent
	maxHistorySize    int
	
	// Discovery source tracking
	discoveryBySource map[string]int64
}

// ConnectionEvent tracks a connection state change
type ConnectionEvent struct {
	PeerID    string
	Connected bool
	Timestamp time.Time
	LatencyMs int64
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		metrics: NetworkMetrics{
			StartTime: time.Now(),
		},
		connectionHistory: make([]ConnectionEvent, 0),
		maxHistorySize:    100,
		discoveryBySource: make(map[string]int64),
	}
}

// RecordConnection records a new connection
func (mc *MetricsCollector) RecordConnection(peerID string, latencyMs int64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	mc.metrics.TotalConnections++
	mc.metrics.CurrentConnections++
	mc.metrics.UptimeSeconds = int64(time.Since(mc.metrics.StartTime).Seconds())
	
	// Record connection event
	event := ConnectionEvent{
		PeerID:    peerID,
		Connected: true,
		Timestamp: time.Now(),
		LatencyMs: latencyMs,
	}
	mc.addConnectionEvent(event)
	
	// Update average latency
	mc.updateAverageLatency(latencyMs)
}

// RecordDisconnection records a disconnection
func (mc *MetricsCollector) RecordDisconnection(peerID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	mc.metrics.TotalDisconnections++
	if mc.metrics.CurrentConnections > 0 {
		mc.metrics.CurrentConnections--
	}
	mc.metrics.UptimeSeconds = int64(time.Since(mc.metrics.StartTime).Seconds())
	
	// Record disconnection event
	event := ConnectionEvent{
		PeerID:    peerID,
		Connected: false,
		Timestamp: time.Now(),
	}
	mc.addConnectionEvent(event)
}

// RecordDiscovery records a peer discovery from a specific source
func (mc *MetricsCollector) RecordDiscovery(source string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	switch source {
	case "dht":
		mc.metrics.DHTDiscoveries++
		mc.discoveryBySource["dht"]++
	case "mdns":
		mc.metrics.MDNSDiscoveries++
		mc.discoveryBySource["mdns"]++
	case "peer_exchange":
		mc.metrics.PeerExchangeDiscoveries++
		mc.discoveryBySource["peer_exchange"]++
	default:
		mc.discoveryBySource[source]++
	}
}

// RecordBootstrapAttempt records a bootstrap attempt
func (mc *MetricsCollector) RecordBootstrapAttempt() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.metrics.BootstrapAttempts++
}

// RecordBootstrapSuccess records a successful bootstrap
func (mc *MetricsCollector) RecordBootstrapSuccess() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.metrics.BootstrapSuccesses++
	mc.metrics.LastBootstrapTime = time.Now()
}

// RecordMessageSuccess records a successful message send
func (mc *MetricsCollector) RecordMessageSuccess() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.metrics.SuccessfulMessages++
}

// RecordMessageFailure records a failed message send
func (mc *MetricsCollector) RecordMessageFailure() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.metrics.FailedMessages++
}

// GetMetrics returns a copy of current metrics
func (mc *MetricsCollector) GetMetrics() NetworkMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	metrics := mc.metrics
	metrics.UptimeSeconds = int64(time.Since(mc.metrics.StartTime).Seconds())
	return metrics
}

// GetDiscoveryBySource returns discovery counts by source
func (mc *MetricsCollector) GetDiscoveryBySource() map[string]int64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	result := make(map[string]int64)
	for k, v := range mc.discoveryBySource {
		result[k] = v
	}
	return result
}

// GetConnectionHistory returns recent connection events
func (mc *MetricsCollector) GetConnectionHistory() []ConnectionEvent {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	result := make([]ConnectionEvent, len(mc.connectionHistory))
	copy(result, mc.connectionHistory)
	return result
}

// addConnectionEvent adds a connection event to history
func (mc *MetricsCollector) addConnectionEvent(event ConnectionEvent) {
	mc.connectionHistory = append(mc.connectionHistory, event)
	
	// Trim history if needed
	if len(mc.connectionHistory) > mc.maxHistorySize {
		mc.connectionHistory = mc.connectionHistory[len(mc.connectionHistory)-mc.maxHistorySize:]
	}
}

// updateAverageLatency updates the running average latency
func (mc *MetricsCollector) updateAverageLatency(newLatency int64) {
	total := mc.metrics.TotalConnections
	if total == 1 {
		mc.metrics.AverageLatencyMs = newLatency
		return
	}
	
	// Calculate new average
	oldTotal := mc.metrics.AverageLatencyMs * (total - 1)
	mc.metrics.AverageLatencyMs = (oldTotal + newLatency) / total
}

// GetConnectionSuccessRate returns the connection success rate
func (mc *MetricsCollector) GetConnectionSuccessRate() float64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	total := mc.metrics.BootstrapAttempts
	if total == 0 {
		return 0.0
	}
	return float64(mc.metrics.BootstrapSuccesses) / float64(total)
}

// GetMessageSuccessRate returns the message success rate
func (mc *MetricsCollector) GetMessageSuccessRate() float64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	total := mc.metrics.SuccessfulMessages + mc.metrics.FailedMessages
	if total == 0 {
		return 1.0 // Default to 100% if no messages sent
	}
	return float64(mc.metrics.SuccessfulMessages) / float64(total)
}
