## 4.0 Bootstrap Discovery (Hybrid Approach)

Babylon Tower uses a hybrid bootstrap mechanism that combines public IPFS infrastructure with a custom PubSub-based discovery protocol for Babylon nodes to help bootstrap each other into the custom DHT network.

### 4.0.1 Architecture Overview

**Two-Bucket Peer Separation:**
- **IPFS peers** (SourceBootstrap, SourceDHT, SourceMDNS) - public infrastructure
- **Babylon peers** (SourceBabylon) - ALL Babylon nodes in single bucket, no subdivision

**Bootstrap Flow:**

```
┌─────────────────────────────────────────────────────────────────┐
│                    First Start (Cold Boot)                       │
├─────────────────────────────────────────────────────────────────┤
│ 1. Bootstrap from public IPFS DHT                               │
│ 2. Join /babylon/bootstrap PubSub topic                         │
│ 3. Send bootstrap_request message                               │
│ 4. Receive bootstrap_response from helper nodes                 │
│ 5. Save responders as SourceBabylon peers                       │
│ 6. Connect to custom DHT                                        │
│ 7. After 5 min uptime: become a helper itself                   │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                  Subsequent Starts (Warm Boot)                   │
├─────────────────────────────────────────────────────────────────┤
│ 1. Load SourceBabylon peers from storage                        │
│ 2. Try connecting (10s timeout, need ≥3 peers)                  │
│ 3. If failed: fall back to PubSub discovery                     │
│ 4. Connect to discovered peers                                  │
│ 5. Join custom DHT                                              │
└─────────────────────────────────────────────────────────────────┘
```

### 4.0.2 PubSub Bootstrap Protocol

**Topic:** `/babylon/bootstrap`

**Message Format:**

```json
{
  "type": "bootstrap_request" | "bootstrap_response",
  "peer_id": "<libp2p peer ID>",
  "timestamp": <unix timestamp>,
  "request_id": "<unique ID for deduplication>",  // requests only
  "uptime_secs": <node uptime>,                   // responses only
  "peer_count": <connected peers>,                // responses only
  "bootstrap_peers": ["<multiaddr>", ...]         // responses only
}
```

**Bootstrap Request:**
- Sent by nodes needing bootstrap assistance
- Contains unique `request_id` for deduplication
- Broadcast to `/babylon/bootstrap` topic

**Bootstrap Response:**
- Sent by helper nodes (nodes meeting criteria)
- Contains list of multiaddrs for connecting peers
- Includes node uptime and peer count for transparency

### 4.0.3 Rate Limiting

To prevent broadcast storms and ensure network stability, bootstrap responses are rate-limited:

| Parameter | Default | Description |
|-----------|---------|-------------|
| Response Probability | 50% | Probabilistic response to requests |
| Max Responses/Minute | 30 | Hard limit on responses |
| Request Dedup Window | 30s | Ignore duplicate requests |
| Seen Requests Cache | 1000 | LRU cache size for dedup |

**Deduplication:**
- Request IDs are cached for 30 seconds
- Duplicate requests within window are ignored
- LRU cache evicts oldest entries when full

### 4.0.4 Helper Node Criteria

A node qualifies as a bootstrap helper when it meets ALL of these criteria:

| Criterion | Minimum | Rationale |
|-----------|---------|-----------|
| Uptime | 5 minutes | Ensures node stability |
| Connected Peers | 3 | Has meaningful connectivity |
| DHT Routing Table | 10 peers | Integrated into DHT |

**Helper Lifecycle:**
1. Node bootstraps successfully
2. Waits 5 minutes to meet uptime requirement
3. Automatically starts responding to bootstrap requests
4. Continues helping as long as criteria are met
5. Stops helping if criteria are no longer met

### 4.0.5 Storage Schema

Discovered Babylon peers are stored with `SourceBabylon`:

```
PeerRecord:
  - peer_id: string
  - multiaddrs: []string
  - first_seen: timestamp
  - last_seen: timestamp
  - source: "babylon"
  - connect_count: int
  - fail_count: int
```

**Peer Timeout:**
- Stored peers are considered stale after 10 seconds of no contact
- Stale peers are skipped during bootstrap
- Successful connection refreshes the timestamp

### 4.0.6 Implementation Notes

**Multi-Stage Bootstrap:**

```go
func bootstrapDHT() error {
    // Stage 0: Try stored Babylon peers (10s timeout, need ≥3)
    storedPeers := loadStoredBabylonPeers()
    connected := connectToPeers(storedPeers, 10*time.Second)
    
    if connected < 3 {
        // Stage 1: PubSub discovery (listen 5s)
        discovered := pubSubDiscovery()
        saveDiscoveredPeers(discovered, SourceBabylon)
        connectToPeers(discovered, 30*time.Second)
    }
    
    // Stage 2: Wait for DHT routing table
    waitForDHT()
    
    // Stage 3: Verify connected peers
    verifyPeers()
    
    // On success: Start bootstrap helper (after 5 min)
    startBootstrapHelper()
}
```

**Security Considerations:**
- No reputation system in Phase 1 (all Babylon peers equal)
- Rate limiting prevents amplification attacks
- Helper criteria ensure only stable nodes respond
- Peer verification via ping protocol

### 4.0.7 Configuration

```yaml
bootstrap:
  pubsub_topic: "/babylon/bootstrap"
  response_probability: 0.5
  max_responses_per_minute: 30
  request_dedup_window_seconds: 30
  min_uptime_secs: 300        # 5 minutes
  min_peer_count: 3
  min_routing_table_size: 10
  stored_peer_timeout_seconds: 10
  pubsub_listen_seconds: 5
  min_babylon_peers_required: 3
```
