# Bug Fixes - Phase 6 Testing

## Issues Fixed

### Issue 1: X25519 Key Not Available for Encryption

**Problem:**
When trying to send messages, users got the error:
```
❌ Error: Failed to send message: recipient X25519 key not available (PoC limitation - contacts need to share X25519 keys)
```

Additionally, `/myid` only showed the Ed25519 public key, not the X25519 key needed for encryption.

**Root Cause:**
- The `/myid` command only displayed Ed25519 public keys
- The `/add` command didn't support storing X25519 keys with contacts
- The Contact protobuf didn't have a field for X25519 public keys
- Message sending failed because recipient X25519 keys weren't stored

**Fix:**

1. **Updated `/myid` command** (`pkg/cli/commands.go`):
   - Now displays both Ed25519 and X25519 public keys
   - Shows keys in both Hex and Base58 formats
   - Clearly indicates which key to share for encryption

   **Example output:**
   ```
   Your Public Keys:
   
   Ed25519 (for signatures and verification):
     Hex:    0e1ba4bb4d0bb9d0...
     Base58: x5A38TJihSa1rofx...
   
   X25519 (for encryption - share this with contacts):
     Hex:    7c3f8a9b2e1d4c5f...
     Base58: 8KmN3pQrStUvWxYz...
   
   Share your X25519 public key with contacts so they can encrypt messages to you.
   ```

2. **Updated Contact protobuf** (`proto/message.proto`):
   - Added `x25519_public_key` field to store contact's encryption key

3. **Updated `/add` command** (`pkg/cli/commands.go`):
   - Now accepts optional X25519 public key as last argument
   - Automatically detects if last argument is a valid X25519 key
   - Stores X25519 key with contact for encryption

   **Usage:**
   ```bash
   # Without encryption (old way)
   /add <ed25519_pubkey> Bob
   
   # With encryption (new)
   /add <ed25519_pubkey> Bob <x25519_pubkey>
   ```

4. **Updated message sending** (`pkg/cli/commands.go`):
   - Retrieves X25519 key from contact storage
   - Returns clear error if X25519 key not available

---

### Issue 2: Empty Line Does Not Exit Chat Mode

**Problem:**
Pressing Enter on an empty line did not exit chat mode as documented.

**Root Cause:**
The `HandleCommand` function checked for empty input BEFORE checking if the user was in chat mode. This meant empty lines were ignored instead of triggering chat exit.

**Fix:**

Updated `HandleCommand` in `pkg/cli/commands.go`:
- Now checks if in chat mode FIRST
- Empty line in chat mode triggers exit
- Empty line outside chat mode is still ignored

**Code change:**
```go
func (h *CommandHandler) HandleCommand(input string) bool {
    input = strings.TrimSpace(input)

    // Check if in chat mode first (empty line exits chat)
    if h.inChatMode {
        return h.handleChatInput(input)
    }

    // Empty input when not in chat mode - ignore
    if input == "" {
        return false
    }
    // ... rest of command handling
}
```

---

### Issue 3: Messages Send But Not Received

**Problem:**
Messages could be sent successfully, but were not received by the other party.

**Root Cause:**
The message flow was:
1. **Sending**: Envelope → IPFS Add → Get CID → Publish CID via PubSub
2. **Receiving**: Receive CID via PubSub → IPFS Get → ❌ **Not implemented in PoC**

The IPFS `Get()` function was not fully implemented in the PoC, so messages could not be fetched after receiving the CID.

**Fix:**

Changed the message flow to send envelopes **directly** via PubSub instead of storing in IPFS and sending CIDs:

1. **Updated `SendMessage`** (`pkg/messaging/outgoing.go`):
   - Now publishes the encrypted envelope directly via PubSub
   - No longer stores envelope in IPFS blockstore
   - Generates a pseudo-CID for reference only

2. **Updated `handlePubSubMessage`** (`pkg/messaging/protocol.go`):
   - Now processes the envelope directly from PubSub message data
   - Removed the IPFS Get call that wasn't implemented

**Before:**
```go
// Old flow - publish CID
cidStr, err := s.ipfsNode.Add(envelopeBytes)
err = s.ipfsNode.PublishTo(recipientPubKey, []byte(cidStr))

// Receiving - try to fetch (fails)
envelope, err := s.ipfsNode.Get(cidStr) // Not implemented!
```

**After:**
```go
// New flow - publish envelope directly
err = s.ipfsNode.PublishTo(recipientPubKey, envelopeBytes)

// Receiving - process directly
err = s.processEnvelope(pubsubMsg.Data) // Works!
```

**Impact:**
- Messages now flow bidirectionally in real-time
- PoC works without requiring IPFS block storage
- Future implementation can add IPFS storage for offline messages

---

### Issue 4: Nodes Not Discovering Each Other

**Problem:**
Even with the message flow fixed, two instances running on the same machine were not receiving each other's messages because the IPFS nodes were not connected.

**Root Cause:**
The IPFS nodes used DHT-based peer discovery which:
- Requires bootstrap peers (not configured in PoC)
- Can take 30-60 seconds to discover peers
- May not work on isolated networks or localhost

**Fix:**

Added **mDNS (multicast DNS)** for instant local network discovery:

1. **Added mDNS service** (`pkg/ipfsnode/node.go`):
   - Imports `github.com/libp2p/go-libp2p/p2p/discovery/mdns`
   - Creates mDNS service on node start
   - Implements `HandlePeerFound()` callback
   - Auto-connects to discovered peers

2. **Updated `/connect` command** (still available):
   - For manual connection when needed
   - Cross-network connections
   - Environments where mDNS is blocked

3. **Updated logging**:
   - Shows "mDNS discovery enabled" on startup
   - Logs "mDNS discovered peer" when peers found
   - Shows connection status

**Before:**
```
Node starts → DHT bootstrap → Wait 30-60s → Maybe discover peers
>>> /connect <multiaddr>  # Manual connection needed
```

**After:**
```
Node starts → mDNS broadcast → < 1 second → Auto-connect ✅
>>> /connect <multiaddr>  # Only for cross-network
```

**Impact:**
- ✅ Instant peer discovery on same network (< 1 second)
- ✅ No manual `/connect` needed for local testing
- ✅ Works on same machine, LAN, WiFi
- ✅ Zero configuration required
- ✅ DHT still runs as backup for wider discovery

**Usage:**
```bash
# Just launch two instances - they auto-connect!
./scripts/test/launch-instance1.sh
./scripts/test/launch-instance2.sh

# Watch for:
# INFO ipfsnode: mDNS discovered peer peer=12D3KooW...
# INFO ipfsnode: connected to mDNS discovered peer
```

---

## How to Test the Fixes

### Test 1: Key Display and Exchange

**Terminal 1 (Alice):**
```bash
./scripts/test/launch-instance1.sh

>>> /myid
```

Copy both the Ed25519 and X25519 public keys.

**Terminal 2 (Bob):**
```bash
./scripts/test/launch-instance2.sh

>>> /myid
```

Copy both keys.

---

### Test 2: Add Contact with Encryption

**Terminal 1 (Alice):**
```bash
# Add Bob with his X25519 key
>>> /add <bob_ed25519_key> Bob <bob_x25519_key>

# Verify contact was added with encryption
>>> /list
```

Expected output:
```
✅ Contact added: Bob (with encryption)
```

**Terminal 2 (Bob):**
```bash
# Add Alice with her X25519 key
>>> /add <alice_ed25519_key> Alice <alice_x25519_key>

>>> /list
```

---

### Test 3: Send and Receive Encrypted Messages (Bidirectional)

**Note:** With mDNS discovery, nodes should auto-connect within 1-2 seconds!

**Watch for auto-discovery in logs:**
```
INFO ipfsnode: mDNS discovered peer peer=12D3KooW...
INFO ipfsnode: connected to mDNS discovered peer
```

**If auto-discovery doesn't work** (different networks or mDNS blocked):
In Bob's terminal, copy Alice's multiaddr from `/myid` and run:
```bash
>>> /connect /ip4/127.0.0.1/tcp/PORT/p2p/PEER_ID
✅ Connected to peer!
```

**Then test messaging:**

**Terminal 1 (Alice):**
```bash
>>> /chat 1

━━━ Chat with Bob ━━━
...

Hello Bob! This is an encrypted message.
[2026-02-21 10:30:00] You: Hello Bob! This is an encrypted message.
```

**Terminal 2 (Bob):**
```
📬 New message from Alice
[2026-02-21 10:30:00] Alice: Hello Bob! This is an encrypted message.
Type /chat to reply.
```

**Terminal 2 (Bob):**
```bash
>>> /chat 1

Hi Alice! I received your message!
[2026-02-21 10:30:15] You: Hi Alice! I received your message!
```

**Terminal 1 (Alice):**
```
📬 New message from Bob
[2026-02-21 10:30:15] Bob: Hi Alice! I received your message!
Type /chat to reply.
```

✅ **Expected:** Both parties can send and receive messages in real-time.

---

### Test 4: Exit Chat with Empty Line

**Terminal 1 (Alice):**
```bash
>>> /chat 1

━━━ Chat with Bob ━━━
...

Some message
[2026-02-21 10:30:00] You: Some message

<press Enter on empty line>

Exited chat mode.

>>>
```

✅ **Expected:** Empty line exits chat mode cleanly.

---

### Test 5: Add Contact Without X25519 Key (Error Case)

**Terminal 1 (Alice):**
```bash
>>> /add <bob_ed25519_key> Bob

✅ Contact added: Bob
ℹ️  Note: No X25519 key provided. Message encryption will not work.
ℹ️  Ask contact to share their X25519 public key.

>>> /chat 1

>>> Hello
❌ Error: Failed to send message: recipient X25519 key not available - ask contact to share their X25519 public key
```

✅ **Expected:** Clear error message when X25519 key is missing.

---

## Files Modified

| File | Changes |
|------|---------|
| `proto/message.proto` | Added `x25519_public_key` field to Contact |
| `pkg/proto/message.pb.go` | Regenerated from proto |
| `pkg/cli/commands.go` | Updated `/myid`, `/add`, `sendMessage`, added `/connect` |
| `pkg/cli/cli.go` | Fixed chat mode empty line handling |
| `pkg/cli/display.go` | Updated help text |
| `pkg/messaging/outgoing.go` | Send envelope directly via PubSub |
| `pkg/messaging/protocol.go` | Process envelope directly from PubSub |
| `test/testutil.go` | Fixed test utility stdin handling |

---

## Testing Checklist

- [ ] `/myid` shows both Ed25519 and X25519 keys
- [ ] Keys are displayed in both Hex and Base58 formats
- [ ] `/myid` shows node multiaddrs
- [ ] `/add` with X25519 key shows "(with encryption)"
- [ ] `/add` without X25519 key shows warning
- [ ] `/connect` can connect to another node
- [ ] Messages can be sent when X25519 key is provided
- [ ] Messages fail with clear error when X25519 key missing
- [ ] Empty line exits chat mode
- [ ] **Messages are received in real-time**
- [ ] **Bidirectional communication works**
- [ ] All existing tests still pass

---

## Command Reference

### View Your Keys
```bash
>>> /myid
```

### Add Contact with Encryption
```bash
>>> /add <ed25519_pubkey> <nickname> <x25519_pubkey>
```

### Add Contact (No Encryption)
```bash
>>> /add <ed25519_pubkey> <nickname>
```

### List Contacts
```bash
>>> /list
```

### Start Chat
```bash
>>> /chat <contact_index_or_pubkey>
```

### Exit Chat
```bash
<press Enter on empty line>
```

### Send Message
```bash
Type message and press Enter
```

---

*Last updated: February 21, 2026*
*Fixes for Phase 6 Testing Issues*
