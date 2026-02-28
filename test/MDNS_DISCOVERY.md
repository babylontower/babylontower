# mDNS Discovery - Fast Local Network Peer Discovery

## What Changed

Babylon Tower now uses **mDNS (multicast DNS)** for automatic peer discovery on local networks.

### Before (DHT-only Discovery)
```
Node starts → Join DHT → Wait 30-60 seconds → Maybe discover peers
```

### After (mDNS + DHT Discovery)
```
Node starts → Broadcast via mDNS → Discover peers in < 1 second → Connect automatically
```

---

## How It Works

### mDNS (Local Network)
1. Node A starts and broadcasts: "I'm here! Protocol: /babylontower/1.0.0"
2. Node B hears the broadcast on the same network
3. Node B automatically connects to Node A
4. Connection established in **< 1 second**

### DHT (Internet/Backup)
- Still runs in background for wider network discovery
- Used as fallback if mDNS unavailable
- Connects to bootstrap peers if configured

---

## Benefits

✅ **Instant Discovery**: Peers find each other in < 1 second
✅ **Zero Configuration**: No manual `/connect` needed on same network
✅ **Works Everywhere**: LAN, WiFi, same machine
✅ **Automatic Reconnection**: Handles network changes
✅ **No Bootstrap Peers**: Works on isolated networks

---

## Usage

### Same Machine Testing

Just launch two instances - they will auto-connect!

```bash
# Terminal 1
./scripts/test/launch-instance1.sh

# Terminal 2 (start within 5 seconds)
./scripts/test/launch-instance2.sh
```

**Watch the logs:**
```
INFO ipfsnode: mDNS discovery enabled for local network
INFO ipfsnode: IPFS node started successfully mDNS=enabled DHT=enabled
INFO ipfsnode: mDNS discovered peer peer=12D3KooW... addrs=[/ip4/127.0.0.1/tcp/4001]
INFO ipfsnode: connected to mDNS discovered peer peer=12D3KooW...
```

### Different Machines (Same Network)

Both machines on same WiFi/LAN will auto-discover:

```bash
# Machine 1 (Alice)
./bin/messenger

# Machine 2 (Bob)
./bin/messenger

# They will automatically connect within 1-2 seconds!
```

---

## When Manual Connect is Still Needed

### Different Networks
If nodes are on different networks (not same LAN):
```bash
>>> /connect /ip4/203.0.113.100/tcp/4001/p2p/QmPeerID
```

### mDNS Blocked
Some networks block multicast traffic:
- Corporate networks
- Some cloud environments
- Docker networks without host networking

In these cases, use manual `/connect` or configure bootstrap peers.

---

## Logs to Watch For

### ✅ Success - Auto Discovery
```
[INFO] mDNS discovery enabled for local network
[INFO] IPFS node started successfully
[INFO] mDNS discovered peer peer=12D3KooWAbc123... addrs=[/ip4/192.168.1.100/tcp/4001]
[INFO] connected to mDNS discovered peer peer=12D3KooWAbc123...
```

### ⚠️ Fallback - DHT Discovery
```
[INFO] IPFS node started successfully
[WARN] DHT bootstrap failed (no bootstrap peers)
[INFO] mDNS discovery enabled for local network
(wait 30-60 seconds for DHT to populate routing table)
```

### ❌ Manual Connect Needed
```
[INFO] IPFS node started successfully
(no "mDNS discovered peer" messages)
# Use /connect command
```

---

## Technical Details

### Protocol
- **mDNS Service**: `github.com/libp2p/go-libp2p/p2p/discovery/mdns`
- **Service Name**: `/babylontower/1.0.0` (same as protocol ID)
- **Port**: Uses libp2p host's listening port
- **Interface**: All network interfaces (WiFi, Ethernet, loopback)

### Discovery Flow
```
1. Node creates libp2p host
2. mDNS service attaches to host
3. mDNS broadcasts service announcement
4. Other nodes receive announcement
5. mDNS calls HandlePeerFound()
6. Node initiates connection
7. PubSub topics sync
8. Messages can flow
```

### Interface with Existing Code
```go
// In node.go Start():
n.mdns = mdns.NewMdnsService(n.host, n.config.ProtocolID, n)

// HandlePeerFound implements mdns.PeerNotif interface:
func (n *Node) HandlePeerFound(peerInfo peer.AddrInfo) {
    logger.Infow("mDNS discovered peer", "peer", peerInfo.ID)
    n.host.Connect(ctx, peerInfo)  // Auto-connect
}
```

---

## Troubleshooting

### Issue: Peers Not Auto-Connecting

**Check:**
1. Both nodes running with mDNS enabled (check logs)
2. On same network/subnet
3. Firewall not blocking multicast (UDP 5353)
4. Wait 5 seconds after both nodes start

**Solution:**
```bash
# Check if mDNS is enabled in logs
grep "mDNS discovery enabled" alice.log bob.log

# Check if peers discovered
grep "mDNS discovered peer" alice.log bob.log

# If not working, use manual connect
>>> /connect <multiaddr>
```

### Issue: "mDNS discovery enabled" but No Peers Found

**Possible causes:**
- Network switches don't forward multicast
- WiFi client isolation enabled
- Docker network isolation

**Solutions:**
1. Use manual `/connect`
2. Configure bootstrap peers
3. Run on host network (Docker: `--network host`)

---

## Comparison: mDNS vs DHT

| Feature | mDNS | DHT |
|---------|------|-----|
| Discovery Speed | < 1 second | 30-60 seconds |
| Network Scope | Local (LAN/WiFi) | Global (Internet) |
| Configuration | Zero | Bootstrap peers needed |
| Bandwidth | Minimal | Moderate |
| NAT Traversal | No | Yes (with relay) |
| Isolated Networks | ✅ Works | ❌ Needs bootstrap |

---

## Future Enhancements

### Planned Improvements
1. **Bootstrap Peer Configuration**: Pre-configured peers for internet discovery
2. **Peer Persistence**: Remember peers across restarts
3. **Connection Pooling**: Maintain multiple peer connections
4. **Relay Nodes**: For NAT traversal

### Configuration Options (Future)
```go
Config{
    EnableMDNS: true,      // Local discovery
    EnableDHT: true,       // Global discovery  
    BootstrapPeers: [...], // Initial peers
    EnableRelay: true,     // NAT traversal
}
```

---

## Testing

### Quick Test
```bash
# Terminal 1
./scripts/test/launch-instance1.sh
# Wait for "mDNS discovery enabled"

# Terminal 2 (within 10 seconds)
./scripts/test/launch-instance2.sh
# Should see "mDNS discovered peer" in both logs

# Test messaging
>>> /myid
>>> /add <peer_key> Peer <x25519_key>
>>> /chat 1
>>> Hello! (should be received)
```

### Verify Connection
```bash
# Check connected peers in logs
grep "connected to" alice.log
grep "connected to" bob.log

# Should see mutual connection within 1-2 seconds
```

---

*Last updated: February 21, 2026*
*mDNS Discovery Implementation*
