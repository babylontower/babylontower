# Babylon Tower - Security & Censorship Resistance Roadmap

## Overview

This document outlines the security and censorship resistance roadmap for Babylon Tower. Unlike the main implementation roadmap, this focuses specifically on making the network resilient against blocking, surveillance, and attacks while **maintaining the core principle of node equality**.

**Core Principle:** No special nodes, no privileged infrastructure, no partnerships required. Every node is equal and can perform any function.

---

## Relationship to Other Roadmaps

### This Roadmap (Security & Censorship Resistance)
- **Focus:** Blocking resistance, surveillance protection, attack mitigation
- **Goal:** Make network resilient against adversaries (ISP, nation-state)
- **Principle:** Maintain node equality - no special infrastructure

### Main Roadmap (`roadmap.md`)
- **Focus:** Protocol features, functionality, UX improvements
- **Goal:** Complete Protocol v1 implementation and beyond
- **Phases:** 1-18 (implementation), 19+ (future features)

### Limitations Roadmap (`limitations-roadmap.md`)
- **Focus:** Known limitations, technical debt, UX gaps
- **Goal:** Track what needs improvement for usability
- **Examples:** Storage encryption, contact verification, message search, GUI

### Overlap & Collaboration

Some limitations require work across multiple roadmaps:

| Limitation | Security Roadmap | Limitations Roadmap |
|------------|-----------------|---------------------|
| **M1: mDNS in containers** | Phase 1 (configurable bootstrap) | UX improvement |
| **C2: Metadata privacy** | Phase 3, 5 (obfuscation, cover traffic) | Privacy UX features |
| **L3: Push notifications** | Phase 8 (mesh networking) | Desktop notifications |

---

## Threat Model

### Adversaries

| Adversary | Capability | Goal |
|-----------|------------|------|
| **ISP (Basic)** | DNS blocking, IP blocking | Prevent access |
| **ISP (Advanced)** | DPI, traffic analysis | Identify and block protocol |
| **Nation-State** | Full DPI, honeypot nodes, eclipse attacks | Isolate users, identify dissidents |
| **Passive Surveillance** | Traffic monitoring | Map social graph |
| **Active Attacker** | Run many nodes, Sybil attack | Control network view |

### Attack Vectors

1. **Bootstrap Blocking:** Prevent new nodes from joining network
2. **Protocol Fingerprinting:** Identify Babylon Tower traffic via DPI
3. **Traffic Analysis:** Determine who communicates with whom
4. **Eclipse Attack:** Isolate target node with attacker-controlled peers
5. **DHT Poisoning:** Flood DHT with false information
6. **App Distribution Blocking:** Prevent download of application

---

## Phase 1: Configurable Bootstrap

**Goal:** Eliminate hardcoded bootstrap nodes, enable multiple discovery channels.

**Status:** ⏹️ Pending  
**Effort:** 3-5 days  
**Impact:** HIGH - Defeats DNS/IP blocking of bootstrap nodes

### Tasks

#### 1.1 Environment Variable Bootstrap

```go
// pkg/ipfsnode/bootstrap_env.go

// User sets: export BABYLON_BOOTSTRAP_PEERS="/ip4/x.x.x.x/tcp/443/wss/p2p/Qm..."
func FromEnvironment() ([]multiaddr.Multiaddr, error)
```

**Acceptance Criteria:**
- ✅ Application reads bootstrap peers from environment variable
- ✅ Multiple peers supported (newline or comma-separated)
- ✅ Invalid multiaddrs rejected with clear error

---

#### 1.2 File-Based Bootstrap

```go
// pkg/ipfsnode/bootstrap_file.go

// File: ~/.babylontower/bootstrap.conf
// Format: One multiaddr per line
func FromFile(path string) ([]multiaddr.Multiaddr, error)
```

**Acceptance Criteria:**
- ✅ Reads from configurable file path
- ✅ Comments supported (lines starting with #)
- ✅ Auto-created with example on first run

---

#### 1.3 HTTPS URL Bootstrap

```go
// pkg/ipfsnode/bootstrap_url.go

// Fetch from any HTTPS URL (GitHub Gist, personal website, etc.)
func FromURL(url string, timeout time.Duration) ([]multiaddr.Multiaddr, error)
```

**Acceptance Criteria:**
- ✅ Fetches plain text file from URL
- ✅ Timeout handling (5 second default)
- ✅ Certificate validation (standard HTTPS)
- ✅ Multiple URLs supported

---

#### 1.4 DNS TXT Record Bootstrap

```go
// pkg/ipfsnode/bootstrap_dns.go

// Query TXT records from any domain
// Domain owner adds: TXT record "babylon-peers=/ip4/.../p2p/Qm..."
func FromDNS(domain string) ([]multiaddr.Multiaddr, error)

// Also support DNS-over-HTTPS for censored regions
func FromDoH(provider, domain string) ([]multiaddr.Multiaddr, error)
```

**Acceptance Criteria:**
- ✅ Queries standard DNS TXT records
- ✅ Supports DoH providers (Cloudflare, Google, Quad9)
- ✅ Parses multiaddrs from TXT record value

---

#### 1.5 Email-Based Bootstrap

```go
// pkg/ipfsnode/bootstrap_email.go

// Send empty email to any address, auto-reply contains peer list
// Server is stateless - just returns same text file
func FromEmail(server, recipient string) ([]multiaddr.Multiaddr, error)
```

**Acceptance Criteria:**
- ✅ Sends minimal email (empty body)
- ✅ Parses auto-reply for peer list
- ✅ Works with any email provider

---

#### 1.6 QR Code Bootstrap

```go
// pkg/ipfsnode/bootstrap_qr.go

// Generate QR code from peer list
func GenerateQR(peers []multiaddr.Multiaddr) (image.Image, error)

// Scan QR code from another user's device
func FromQR(imagePath string) ([]multiaddr.Multiaddr, error)
```

**Acceptance Criteria:**
- ✅ Generates scannable QR code
- ✅ Parses QR code from image file
- ✅ CLI command: `/exportpeers` and `/importpeers <qr-image>`

---

#### 1.7 Bootstrap Configuration CLI

```bash
# Show current bootstrap configuration
>>> /bootstrap config
Current bootstrap sources:
  - Environment: 0 peers
  - File: 3 peers
  - URL: 5 peers (https://example.com/peers.txt)
  - DNS: 2 peers (babylontower.io)
  - Storage: 15 peers (from last run)

# Add new bootstrap URL
>>> /bootstrap add-url https://example.com/peers.txt

# Remove bootstrap source
>>> /bootstrap remove-url https://example.com/peers.txt

# Force refresh bootstrap
>>> /bootstrap refresh
Fetching from all sources...
✅ Discovered 25 unique peers
✅ Connected to 12 peers

# Export bootstrap config to QR
>>> /bootstrap export-qr bootstrap.png
```

---

### Deliverables

- ✅ All bootstrap sources implemented
- ✅ CLI commands for management
- ✅ Documentation for users on setting up bootstrap
- ✅ Example bootstrap hosting (GitHub Gist guide)

---

## Phase 2: Peer Exchange Protocol

**Goal:** Network becomes self-sustaining through peer exchange.

**Status:** ⏹️ Pending  
**Effort:** 2-3 days  
**Impact:** HIGH - No ongoing bootstrap dependency

### Tasks

#### 2.1 Peer Request/Response Protocol

```go
// pkg/discovery/peer_exchange.go

// Custom libp2p protocol: /babylon/peer-exchange/1.0.0
type PeerRequest struct {
    Count int  // Number of peers requested (default: 10)
}

type PeerResponse struct {
    Peers []SignedPeerInfo  // Peers from routing table
}

// Every node responds to peer requests with random peers from routing table
func HandlePeerRequest(stream network.Stream)
```

**Acceptance Criteria:**
- ✅ New protocol registered on all nodes
- ✅ Responds with 10-20 random peers from routing table
- ✅ Rate limited (1 request per minute per peer)

---

#### 2.2 Proactive Peer Gossip

```go
// pkg/discovery/peer_gossip.go

// Every 5 minutes, tell 3 random peers about 5 peers you know
func GossipPeers() {
    peers := selectRandomPeers(3)
    for _, p := range peers {
        knownPeers := selectRandomPeers(5)
        sendPeerList(p, knownPeers)
    }
}
```

**Acceptance Criteria:**
- ✅ Gossip runs every 5 minutes
- ✅ Prevents gossip loops (don't gossip back to source)
- ✅ Tracks gossip history (don't repeat same peers)

---

#### 2.3 Peer Exchange CLI

```bash
# Request peers from current connections
>>> /peerex request 20
Requesting 20 peers from 5 connected peers...
✅ Received 87 unique peers
✅ Added 45 new peers to address book

# Show peer exchange stats
>>> /peerex stats
Peer Exchange Statistics:
  Requests sent: 15
  Requests received: 23
  Peers discovered: 156
  Peers connected: 34
```

---

### Deliverables

- ✅ Peer exchange protocol implemented
- ✅ Gossip mechanism working
- ✅ CLI commands for manual peer exchange
- ✅ Statistics tracking

---

## Phase 3: Transport Obfuscation

**Goal:** Make protocol unidentifiable via DPI.

**Status:** ⏹️ Pending  
**Effort:** 1-2 weeks  
**Impact:** VERY HIGH - Defeats protocol fingerprinting

### Tasks

#### 3.1 WebSocket Transport on Port 443

```go
// pkg/ipfsnode/config.go

// Default listen addresses
config.ListenAddrs = []string{
    "/ip4/0.0.0.0/tcp/443/wss",
    "/ip4/0.0.0.0/tcp/8443/wss",
    "/ip4/0.0.0.0/udp/443/quic-v1",
}
```

**Acceptance Criteria:**
- ✅ Listens on port 443 (requires root/cap_net_bind_service)
- ✅ Falls back to 8443 if 443 unavailable
- ✅ QUIC transport on port 443 as alternative

---

#### 3.2 Obfs4-Style Obfuscation

```go
// pkg/transport/obfs4.go

// Obfuscated transport - looks like random noise
// Any user can enable this (no special bridge status)
type Obfs4Transport struct {
    // 4-way handshake with padding
    // No identifiable signature
    // Shared secret established during handshake
}

// User configuration
type TransportConfig struct {
    EnableObfs4 bool
    Obfs4Port   int  // Default: 9443
}
```

**Acceptance Criteria:**
- ✅ Handshake indistinguishable from random noise
- ✅ Any node can enable obfs4 transport
- ✅ Advertised in peer multiaddrs as `/obfs4`
- ✅ Performance overhead <20%

---

#### 3.3 Transport Negotiation

```go
// pkg/transport/negotiate.go

// Try transports in order based on censorship level
func NegotiateTransport(peer peer.ID) (transport.Transport, error) {
    // Try standard WebSocket first (fastest)
    // If blocked, try obfs4
    // If blocked, try QUIC
    // Return first working transport
}
```

**Acceptance Criteria:**
- ✅ Automatic fallback on failure
- ✅ Remembers which transports work per peer
- ✅ Configurable transport priority

---

#### 3.4 Transport CLI

```bash
# Show available transports
>>> /transport list
Available Transports:
  [✓] /tcp/443/wss (enabled)
  [✓] /tcp/8443/wss (enabled)
  [✓] /udp/443/quic-v1 (enabled)
  [ ] /tcp/9443/obfs4 (disabled)

# Enable obfs4 transport
>>> /transport enable obfs4
✅ Obfs4 transport enabled on port 9443
⚠️  Requires restart to take effect

# Test transport to peer
>>> /transport test QmPeerID
Testing transports to QmPeerID...
  /tcp/443/wss: ✅ 45ms
  /udp/443/quic-v1: ✅ 38ms
  /tcp/9443/obfs4: ⏹️ Not tested (disabled)
```

---

### Deliverables

- ✅ WebSocket transport on port 443
- ✅ Obfs4 obfuscation implemented
- ✅ Transport negotiation working
- ✅ CLI commands for transport management

---

## Phase 4: Peer Scoring & Anti-Eclipse

**Goal:** Detect and avoid eclipse attacks.

**Status:** ⏹️ Pending  
**Effort:** 3-5 days  
**Impact:** HIGH - Defeats Sybil/eclipse attacks

### Tasks

#### 4.1 Peer Score Computation

```go
// pkg/discovery/scoring.go

type PeerScore struct {
    PeerID peer.ID
    
    // How long known (older = more trusted)
    Age time.Duration
    
    // Success rate of connections
    Reliability float64
    
    // Number of useful peers introduced
    IntroductionQuality float64
    
    // Diversity score (ASN, country)
    Diversity int
    
    // Suspicious behavior flags
    Flags BehaviorFlags
}

// Locally computed - no central scoring
func ComputeScore(peer peer.ID) *PeerScore
```

**Acceptance Criteria:**
- ✅ Scores computed locally per node
- ✅ Age factor (peers known longer score higher)
- ✅ Reliability tracked (connection success rate)
- ✅ Introduction quality measured (do they provide useful peers?)

---

#### 4.2 Eclipse Detection

```go
// pkg/discovery/eclipse_detect.go

type EclipseDetector struct {
    currentPeers map[peer.ID]PeerInfo
}

// Detect potential eclipse attack
func (d *EclipseDetector) DetectEclipse() EclipseAlert {
    // All peers from same IP range?
    // All peers introduced by same node?
    // Sudden drop in peer diversity?
    // All peers have similar characteristics?
    
    // Return alert level: NONE, LOW, MEDIUM, HIGH
}
```

**Acceptance Criteria:**
- ✅ Detects IP range concentration
- ✅ Detects single-introducer concentration
- ✅ Detects sudden diversity drop
- ✅ Alerts user via CLI

---

#### 4.3 Diversity Enforcement

```go
// pkg/discovery/diversity.go

// Ensure connections to diverse peers
func EnsureDiversity(targetDiversity int) {
    // If all peers from same ASN, force connection to different ASN
    // If all peers from same country, force connection to different country
    // Use GeoIP + ASN lookup
}
```

**Acceptance Criteria:**
- ✅ ASN lookup for each peer
- ✅ Country lookup for each peer
- ✅ Forces diverse connections when needed
- ✅ Configurable diversity requirements

---

#### 4.4 Scoring CLI

```bash
# Show peer scores
>>> /peerscore list
Peer Scores:
  QmPeer1...  Score: 85  Age: 7d  Reliability: 95%  Diversity: 12
  QmPeer2...  Score: 72  Age: 3d  Reliability: 88%  Diversity: 8
  QmPeer3...  Score: 45  Age: 1d  Reliability: 60%  Diversity: 2  ⚠️ Low diversity

# Check for eclipse attack
>>> /peerscore check-eclipse
✅ No eclipse attack detected
  Peer diversity: 15 ASNs, 8 countries
  Introducer diversity: 12 unique introducers

# Force diversity refresh
>>> /peerscore refresh-diversity
Finding peers from different ASNs...
✅ Connected to 3 new peers from different ASNs
```

---

### Deliverables

- ✅ Peer scoring system implemented
- ✅ Eclipse detection working
- ✅ Diversity enforcement active
- ✅ CLI commands for monitoring

---

## Phase 5: DHT Privacy

**Goal:** Obscure DHT queries to hide communication patterns.

**Status:** ⏹️ Pending  
**Effort:** 1 week  
**Impact:** MEDIUM-HIGH - Defeats traffic analysis

### Tasks

#### 5.1 Cover Traffic

```go
// pkg/ipfsnode/dht_cover.go

// Add dummy queries to hide real queries
func FindPeerWithCover(target peer.ID) {
    // Launch real query
    realQuery := dht.FindPeer(target)
    
    // Launch 5-10 dummy queries to random peer IDs
    for i := 0; i < rand.Intn(5, 10); i++ {
        randomPeer := generateRandomPeerID()
        go dht.FindPeer(randomPeer)
    }
    
    // Wait for real query result
    return realQuery
}
```

**Acceptance Criteria:**
- ✅ Configurable cover traffic intensity (0-10 dummy queries)
- ✅ Dummy queries indistinguishable from real queries
- ✅ Performance impact acceptable (<2x query time)

---

#### 5.2 Query Timing Obfuscation

```go
// pkg/ipfsnode/dht_timing.go

// Add random delays to queries
func ObfuscatedQuery(query func()) {
    // Wait random 0-5 seconds before sending
    time.Sleep(time.Duration(rand.Intn(5000)) * time.Millisecond)
    
    // Send query
    query()
}
```

**Acceptance Criteria:**
- ✅ Random delays added to queries
- ✅ Configurable delay range
- ✅ Prevents timing correlation

---

#### 5.3 Privacy CLI

```bash
# Configure DHT privacy level
>>> /dhtprivacy set-level medium
✅ DHT privacy level set to MEDIUM
  - Cover traffic: 5 dummy queries per real query
  - Timing obfuscation: 0-3 second delays

# Show privacy stats
>>> /dhtprivacy stats
DHT Privacy Statistics:
  Real queries: 45
  Cover queries: 312
  Privacy ratio: 6.9x (cover/real)
```

---

### Deliverables

- ✅ Cover traffic implemented
- ✅ Timing obfuscation working
- ✅ CLI commands for configuration

---

## Phase 6: User-Hosted Peer Lists

**Goal:** Distributed peer list hosting (no central authority).

**Status:** ⏹️ Pending  
**Effort:** 2-3 days  
**Impact:** HIGH - Distributed bootstrap infrastructure

### Tasks

#### 6.1 Peer List Format Specification

```yaml
# Peer List Format v1
# File: peers.txt

version: 1
timestamp: 2026-02-26T10:00:00Z
 curator: "QmCuratorPeerID..."  # Optional
 peers:
   - "/ip4/192.168.1.100/tcp/443/wss/p2p/QmPeer1"
   - "/ip4/example.com/tcp/9443/wss/p2p/QmPeer2"
   - "/dns/peer3.example.com/tcp/443/wss/p2p/QmPeer3"
```

**Acceptance Criteria:**
- ✅ Simple text format
- ✅ Version field for future updates
- ✅ Timestamp for freshness
- ✅ Optional curator signature

---

#### 6.2 Hosting Guide

**Documentation:**
- How to host on GitHub Gist
- How to host on personal website
- How to host on IPFS
- How to host on any static file server

**Acceptance Criteria:**
- ✅ Step-by-step guide for each platform
- ✅ Example peer list files
- ✅ Update instructions

---

#### 6.3 Peer List Discovery

```go
// pkg/discovery/peer_lists.go

// Curated list of public peer lists (user can modify)
var DefaultPeerLists = []string{
    // Community-maintained lists (anyone can host)
    "https://raw.githubusercontent.com/.../peers.txt",
    // IPFS-hosted lists
    "ipfs://QmPeerListCID/peers.txt",
}

// User can add their own lists
func AddPeerList(url string) error
```

**Acceptance Criteria:**
- ✅ Default lists configurable
- ✅ User can add/remove lists
- ✅ Lists fetched periodically

---

#### 6.4 Peer List CLI

```bash
# List configured peer list URLs
>>> /peerlists list
Configured Peer Lists:
  [✓] https://github.com/.../peers.txt (updated 2h ago, 25 peers)
  [✓] ipfs://QmPeerListCID (updated 1d ago, 18 peers)
  [ ] https://example.com/peers.txt (failed: timeout)

# Add new peer list
>>> /peerlists add https://example.com/peers.txt
✅ Added peer list
Fetching...
✅ Retrieved 32 peers

# Remove peer list
>>> /peerlists remove https://example.com/peers.txt
✅ Removed peer list

# Force update all lists
>>> /peerlists refresh
Refreshing all peer lists...
✅ Updated 3 lists, 2 failed
```

---

### Deliverables

- ✅ Peer list format specified
- ✅ Hosting guide published
- ✅ CLI commands for management
- ✅ Automatic refresh mechanism

---

## Phase 7: Social Network Bootstrap

**Goal:** Use existing social networks for peer discovery.

**Status:** ⏹️ Pending  
**Effort:** 3-5 days  
**Impact:** MEDIUM-HIGH - Uses existing infrastructure

### Tasks

#### 7.1 Twitter/X Integration

```go
// pkg/discovery/twitter.go

// Fetch tweets with #BabylonPeers hashtag
func FromTwitterHashtag(hashtag string) ([]multiaddr.Multiaddr, error)

// Fetch from specific user's tweets
func FromTwitterUser(username string) ([]multiaddr.Multiaddr, error)
```

**Acceptance Criteria:**
- ✅ Uses public Twitter API (or scraping as fallback)
- ✅ Parses multiaddrs from tweet text
- ✅ Rate limited to avoid API limits

---

#### 7.2 Mastodon Integration

```go
// pkg/discovery/mastodon.go

// Fetch public toots with hashtag
func FromMastodonHashtag(instance, hashtag string) ([]multiaddr.Multiaddr, error)
```

**Acceptance Criteria:**
- ✅ Uses Mastodon public API
- ✅ Works with any instance
- ✅ Parses multiaddrs from toots

---

#### 7.3 Social Bootstrap CLI

```bash
# Fetch peers from Twitter hashtag
>>> /social twitter #BabylonPeers
Fetching from Twitter...
✅ Found 15 peers in 23 tweets

# Fetch peers from Mastodon
>>> /social mastodon mastodon.social #BabylonPeers
Fetching from Mastodon...
✅ Found 8 peers in 12 toots

# Post your peer info to Twitter
>>> /social twitter-post
🐦 Posting your peer info to Twitter...
✅ Posted: "Babylon Tower peers: /ip4/x.x.x.x/tcp/443/wss/p2p/Qm..."
```

---

### Deliverables

- ✅ Twitter integration working
- ✅ Mastodon integration working
- ✅ CLI commands for social bootstrap
- ✅ Documentation on hashtag usage

---

## Phase 8: Mesh Networking

**Goal:** Local communication without internet.

**Status:** ⏹️ Pending  
**Effort:** 3-4 weeks  
**Impact:** HIGH - Works during internet shutdown

### Tasks

#### 8.1 Bluetooth LE Mesh

```go
// pkg/mesh/bluetooth.go

// Advertise presence via Bluetooth LE
func AdvertiseBluetooth() error

// Scan for nearby Babylon nodes
func ScanBluetooth() ([]peer.ID, error)

// Exchange messages via Bluetooth
func SendBluetooth(peer peer.ID, msg []byte) error
```

**Acceptance Criteria:**
- ✅ Bluetooth LE advertising
- ✅ Peer discovery via scan
- ✅ Message exchange
- ✅ Multi-hop support (messages relay through intermediate nodes)

---

#### 8.2 WiFi Direct Mesh

```go
// pkg/mesh/wifi.go

// Create WiFi Direct group
func CreateWiFiGroup() (string, error)

// Join WiFi Direct group
func JoinWiFiGroup(groupID string) error
```

**Acceptance Criteria:**
- ✅ WiFi Direct group creation
- ✅ Peer discovery within group
- ✅ Message broadcast

---

#### 8.3 Internet Bridge Mode

```go
// pkg/mesh/bridge.go

// If one mesh node has internet, bridge messages to wider network
type BridgeNode struct {
    meshConn net.Conn  // Bluetooth/WiFi
    internetConn net.Conn  // Regular IPFS
}

func (b *BridgeNode) RelayMessages() {
    // Receive from mesh, send to internet
    // Receive from internet, send to mesh
}
```

**Acceptance Criteria:**
- ✅ Any node can become bridge (user-enabled)
- ✅ No special status - just relays messages
- ✅ Messages encrypted end-to-end (bridge can't read)

---

#### 8.4 Mesh CLI

```bash
# Show mesh status
>>> /mesh status
Mesh Network:
  Mode: Bluetooth + WiFi Direct
  Connected peers: 5 (local)
  Internet bridge: Yes (via QmBridgePeer...)
  Messages relayed: 23

# Enable mesh mode
>>> /mesh enable
✅ Mesh networking enabled
  Scanning for nearby peers...
  ✅ Found 3 peers via Bluetooth
  ✅ Found 2 peers via WiFi Direct

# Disable mesh mode
>>> /mesh disable
✅ Mesh networking disabled
```

---

### Deliverables

- ✅ Bluetooth mesh working
- ✅ WiFi Direct mesh working
- ✅ Internet bridge mode implemented
- ✅ CLI commands for mesh management

---

## Success Criteria

### Phase 1-4 (Basic Resistance)
- ✅ No hardcoded bootstrap nodes
- ✅ 5+ bootstrap discovery channels
- ✅ Protocol unidentifiable via basic DPI
- ✅ Eclipse attacks detected and avoided
- ✅ Network self-sustaining via peer exchange

### Phase 5-8 (Advanced Resistance)
- ✅ DHT queries obscured with cover traffic
- ✅ Distributed peer list hosting (no central authority)
- ✅ Social network bootstrap working
- ✅ Local mesh networking functional
- ✅ Works during partial internet shutdown

---

## Testing

### Censorship Simulation Tests

```bash
# Test bootstrap with blocked DNS
go test -tags=censorship ./pkg/discovery/... -run TestDNSBlocked

# Test bootstrap with blocked IPs
go test -tags=censorship ./pkg/ipfsnode/... -run TestIPsBlocked

# Test with DPI simulation
go test -tags=censorship ./pkg/transport/... -run TestDPISimulation

# Test eclipse attack scenario
go test -tags=censorship ./pkg/discovery/... -run TestEclipseAttack

# Test mesh networking without internet
go test -tags=censorship ./pkg/mesh/... -run TestOfflineMesh
```

---

## Maintenance

### Peer List Rotation

- Peer lists refreshed every 24 hours
- Stale peers removed after 7 days
- Emergency discovery triggered if <5 working peers

### Transport Updates

- New transports added as plugins
- Obsolete transports deprecated
- Community maintains transport implementations

### Documentation Updates

- Bootstrap hosting guide updated quarterly
- Social media hashtag monitored
- Mesh networking guide expanded

---

## Comparison: Before vs After

| Feature | Before | After Phase 4 | After Phase 8 |
|---------|--------|---------------|---------------|
| **Bootstrap Blocking** | Vulnerable | Resistant | Very Resistant |
| **DPI Detection** | Vulnerable | Resistant | Very Resistant |
| **Eclipse Attack** | Vulnerable | Resistant | Very Resistant |
| **Traffic Analysis** | Vulnerable | Partially Resistant | Resistant |
| **Internet Shutdown** | No | No | Yes (mesh mode) |
| **Node Equality** | ✅ | ✅ | ✅ |

---

## Timeline

| Phase | Duration | Cumulative |
|-------|----------|------------|
| Phase 1: Configurable Bootstrap | 3-5 days | Day 5 |
| Phase 2: Peer Exchange | 2-3 days | Day 8 |
| Phase 3: Transport Obfuscation | 1-2 weeks | Day 18 |
| Phase 4: Peer Scoring | 3-5 days | Day 23 |
| Phase 5: DHT Privacy | 1 week | Day 30 |
| Phase 6: Peer List Hosting | 2-3 days | Day 33 |
| Phase 7: Social Bootstrap | 3-5 days | Day 38 |
| Phase 8: Mesh Networking | 3-4 weeks | Day 66 |

**Total:** ~66 working days (9-10 weeks)

---

## Phase 9: User-Run Relay Infrastructure

**Goal:** Enable users to voluntarily run relay nodes for NAT traversal, without creating special node classes.

**Status:** ⏹️ Pending  
**Effort:** 3-5 days  
**Impact:** HIGH - Helps users behind symmetric NATs

### Tasks

#### 9.1 Voluntary Relay Service

```go
// pkg/relay/service.go

// Any user can enable relay service
type RelayService struct {
    config *RelayConfig
}

type RelayConfig struct {
    // User enables relay to help others
    EnableRelayService bool
    
    // User-set bandwidth limits (bytes/day)
    MaxBandwidth int64
    
    // Optional: only relay for specific peers
    AllowedPeers []peer.ID
    
    // Optional: only relay for peers from certain regions
    AllowedCountries []string
    
    // Track usage
    bandwidthUsed int64
    lastReset time.Time
}

// Relay encrypted traffic without reading content
func (r *RelayService) RelayStream(src, dst network.Stream) {
    // Blindly forward encrypted data
    // Cannot decrypt (end-to-end encryption)
    // Track bandwidth usage
}
```

**Acceptance Criteria:**
- ✅ Any node can enable relay service
- ✅ Bandwidth limits enforced (user-configured)
- ✅ Optional peer whitelist (help friends only)
- ✅ Relay node cannot read message content

---

#### 9.2 Auto-Relay for NATed Nodes

```go
// pkg/relay/auto_relay.go

// Nodes behind NAT automatically find relays
type AutoRelayFinder struct {
    host host.Host
}

func (a *AutoRelayFinder) FindRelays() ([]peer.ID, error) {
    // Query DHT for nodes advertising relay service
    // Test connectivity to each
    // Select 2-3 reliable relays
    // Maintain relay connections
}

// Advertise relay availability in DHT
func AdvertiseRelayService() {
    // Add "relay" protocol to peer record
    // Other nodes can discover via DHT
}
```

**Acceptance Criteria:**
- ✅ NATed nodes automatically find relays
- ✅ Maintains 2-3 backup relays
- ✅ Relay peers advertised in DHT
- ✅ No central relay registry

---

#### 9.3 TURN Server for WebRTC

```go
// pkg/rtc/turn.go

// User-run TURN servers for WebRTC calls
// Similar to relay, but specific to RTC

type TURNConfig struct {
    // User can run TURN server
    EnableTURN bool
    
    // TURN server address (if running)
    ServerAddr string
    
    // Credentials (long-term auth)
    Username string
    Password string
    
    // Or use public/community TURN servers
    PublicServers []string
}

// Generate TURN credentials for call
func GenerateTURNCredentials(callID string, expiry time.Time) (string, string)
```

**Acceptance Criteria:**
- ✅ Users can run private TURN servers
- ✅ Community TURN servers (volunteer-run)
- ✅ Credentials generated per-call
- ✅ No dependency on centralized TURN

---

#### 9.4 Community Coordination (No Central Registry)

```go
// pkg/relay/community.go

// Wiki-based coordination (out-of-band)
// No protocol-level registry

// Community-maintained list of public relays
var CommunityRelays = []string{
    // Users volunteer via GitHub PR
    "/dns/relay1.example.com/tcp/443/wss/p2p/Qm...",
    "/dns/relay2.example.com/tcp/443/wss/p2p/Qm...",
}

// Fetch latest community relay list
func FetchCommunityRelays() ([]multiaddr.Multiaddr, error) {
    // GitHub raw file
    // IPFS-hosted list
    // Any static URL
}
```

**Acceptance Criteria:**
- ✅ Community relay list maintained via GitHub
- ✅ List hosted on multiple mirrors (no single point of failure)
- ✅ Users can volunteer relay capacity
- ✅ No protocol-level special status

---

#### 9.5 Relay CLI

```bash
# Show relay status
>>> /relay status
Relay Service:
  Status: Enabled (helping community)
  Bandwidth limit: 10 GB/day
  Bandwidth used: 3.2 GB today
  Peers helped: 15 (last 24h)
  Uptime: 2 days, 5 hours

# Configure relay service
>>> /relay config set --bandwidth 20GB --public
✅ Relay service configured
  Bandwidth: 20 GB/day
  Visibility: Public (advertised in DHT)

# Disable relay service
>>> /relay disable
✅ Relay service disabled
  You will no longer relay traffic for others
  You can still use community relays

# List community relays
>>> /relay list-community
Community Relays:
  [✓] relay1.example.com (15ms, 99.5% uptime)
  [✓] relay2.example.com (28ms, 98.2% uptime)
  [⚠️] relay3.example.com (45ms, 95.1% uptime)
```

---

### Deliverables

- ✅ Voluntary relay service implemented
- ✅ Auto-relay finder for NATed nodes
- ✅ TURN server support for WebRTC
- ✅ Community coordination (wiki-based)
- ✅ CLI commands for relay management

---

## Phase 10: Private Relay Networks (Optional)

**Goal:** Allow users to create private relay networks for trusted peers.

**Status:** ⏹️ Pending  
**Effort:** 2-3 days  
**Impact:** MEDIUM - Trusted infrastructure for sensitive users

### Tasks

#### 10.1 Private Relay Groups

```go
// pkg/relay/private.go

// User creates private relay group
type PrivateRelayGroup struct {
    GroupID string
    Members []peer.ID  // Trusted peers only
    Relays  []peer.ID  // Designated relay nodes
}

// Only relay traffic for group members
func (p *PrivateRelayGroup) CanRelay(peer peer.ID) bool {
    return contains(p.Members, peer)
}
```

**Acceptance Criteria:**
- ✅ User creates private relay group
- ✅ Only trusted peers can use relays
- ✅ Multiple private groups supported

---

#### 10.2 Family/Friend Relay Networks

```bash
# Create private relay network
>>> /relay create-private "Family Network"
✅ Private relay group created: grp_family
  Add trusted peers: /relay add-peer grp_family QmPeerID

# Add peer to private network
>>> /relay add-peer grp_family QmPeerID
✅ Peer added to Family Network
  Peer can now use your relay nodes

# List private relay networks
>>> /relay list-private
Private Relay Networks:
  [1] Family Network - 5 peers, 2 relay nodes
  [2] Activist Group - 12 peers, 4 relay nodes
```

---

### Deliverables

- ✅ Private relay group creation
- ✅ Peer management for private groups
- ✅ CLI commands for private networks

---

## Updated Timeline

| Phase | Duration | Cumulative |
|-------|----------|------------|
| Phase 1: Configurable Bootstrap | 3-5 days | Day 5 |
| Phase 2: Peer Exchange | 2-3 days | Day 8 |
| Phase 3: Transport Obfuscation | 1-2 weeks | Day 18 |
| Phase 4: Peer Scoring | 3-5 days | Day 23 |
| Phase 5: DHT Privacy | 1 week | Day 30 |
| Phase 6: Peer List Hosting | 2-3 days | Day 33 |
| Phase 7: Social Bootstrap | 3-5 days | Day 38 |
| Phase 8: Mesh Networking | 3-4 weeks | Day 66 |
| **Phase 9: User-Run Relays** | **3-5 days** | **Day 71** |
| **Phase 10: Private Relays** | **2-3 days** | **Day 74** |

**Total:** ~74 working days (10-11 weeks)

---

*Last updated: February 26, 2026*
*Version: 1.0*
