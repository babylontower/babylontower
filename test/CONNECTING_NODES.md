# Connecting Nodes - User Guide

## Problem

When running two Babylon Tower instances on the same machine or local network, they may not automatically discover each other. This causes messages to be sent but not received.

## Solution

Use the `/connect` command to manually connect the nodes.

---

## Step-by-Step Guide

### Step 1: Launch Both Instances

**Terminal 1 (Alice):**
```bash
./scripts/test/launch-instance1.sh
```

**Terminal 2 (Bob):**
```bash
./scripts/test/launch-instance2.sh
```

---

### Step 2: Get Multiaddr from Each Instance

**In Alice's terminal:**
```bash
>>> /myid
```

Look for the multiaddr output:
```
Your Node Multiaddr (for /connect command):
  /ip4/127.0.0.1/tcp/45678/p2p/12D3KooWAbc123...
  /ip4/192.168.1.100/tcp/45679/p2p/12D3KooWAbc123...
```

**In Bob's terminal:**
```bash
>>> /myid
```

Bob will see similar output with different addresses and peer ID.

---

### Step 3: Connect the Nodes

**Option A: Connect Alice to Bob**

In Alice's terminal, copy one of Bob's multiaddrs and run:
```bash
>>> /connect /ip4/127.0.0.1/tcp/4001/p2p/12D3KooWBobPeerID...
✅ Connected to peer!
```

**Option B: Connect Bob to Alice**

In Bob's terminal, copy one of Alice's multiaddrs and run:
```bash
>>> /connect /ip4/127.0.0.1/tcp/4000/p2p/12D3KooWAlicePeerID...
✅ Connected to peer!
```

**Note:** You only need to connect in ONE direction. Once connected, communication is bidirectional.

---

### Step 4: Verify Connection

After connecting, you can verify by checking the logs:

**Alice's logs should show:**
```
INFO ipfsnode: connected to peer peer=12D3KooWBobPeerID...
```

**Bob's logs should show:**
```
INFO ipfsnode: connected to discovered peer peer=12D3KooWAlicePeerID...
```

---

### Step 5: Add Contacts and Chat

Now that nodes are connected, proceed with normal operation:

```bash
>>> /add <ed25519_key> <nickname> <x25519_key>
>>> /chat 1
>>> Hello!
```

Messages should now be delivered in both directions!

---

## Multiaddr Formats

### Localhost (same machine)
```
/ip4/127.0.0.1/tcp/PORT/p2p/PEER_ID
```

### Local Network (different machines)
```
/ip4/192.168.1.100/tcp/PORT/p2p/PEER_ID
```

### IPv6
```
/ip6/::1/tcp/PORT/p2p/PEER_ID
```

---

## Troubleshooting

### "Failed to connect: invalid multiaddr"

**Cause:** Multiaddr format is incorrect.

**Solution:** Copy the multiaddr exactly as shown in `/myid` output.

---

### "Failed to connect: context deadline exceeded"

**Cause:** Cannot reach the peer (wrong address, firewall, peer not running).

**Solution:**
1. Verify the peer instance is running
2. Check you're using the correct IP address
3. Try localhost (`127.0.0.1`) if on same machine
4. Check firewall settings

---

### Messages still not received after connecting

**Cause:** May need to wait a moment for connection to establish.

**Solution:**
1. Wait 5-10 seconds after connecting
2. Try sending a test message
3. Check logs for connection confirmation

---

## Quick Reference

| Command | Description |
|---------|-------------|
| `/myid` | Show your multiaddrs |
| `/connect <addr>` | Connect to peer |
| `/add <key> <name> <x25519>` | Add contact |
| `/chat <index>` | Start chat |

---

## Example Session

### Alice's Terminal
```bash
$ ./scripts/test/launch-instance1.sh

>>> /myid
Your Public Keys:
  ...

Your Node Multiaddr (for /connect command):
  /ip4/127.0.0.1/tcp/45678/p2p/12D3KooWAlice...

# Alice shares her multiaddr with Bob

# Bob connects to Alice (in Bob's terminal)
# Alice sees in logs:
INFO ipfsnode: connected to discovered peer peer=12D3KooWBob...

>>> /add <bob_key> Bob <bob_x25519>
✅ Contact added: Bob (with encryption)

>>> /chat 1
>>> Hi Bob!
[2026-02-21 10:30:00] You: Hi Bob!
```

### Bob's Terminal
```bash
$ ./scripts/test/launch-instance2.sh

>>> /myid
Your Public Keys:
  ...

Your Node Multiaddr (for /connect command):
  /ip4/127.0.0.1/tcp/45679/p2p/12D3KooWBob...

# Bob copies Alice's multiaddr and connects:
>>> /connect /ip4/127.0.0.1/tcp/45678/p2p/12D3KooWAlice...
✅ Connected to peer!

>>> /add <alice_key> Alice <alice_x25519>
✅ Contact added: Alice (with encryption)

# Bob sees Alice's message:
📬 New message from Alice
[2026-02-21 10:30:00] Alice: Hi Bob!

>>> /chat 1
>>> Hello Alice!
```

---

## Why Manual Connection?

The PoC uses DHT-based peer discovery which:
- Requires bootstrap peers (not configured in PoC)
- Can take 30+ seconds to discover peers
- May not work on isolated networks

The `/connect` command provides:
- ✅ Immediate connection
- ✅ Works on localhost and LAN
- ✅ No configuration needed
- ✅ Explicit control over connections

**Future Enhancement:** Add mDNS/bonjour for automatic local network discovery.

---

*Last updated: February 21, 2026*
*Phase 6 - Node Connection Guide*
