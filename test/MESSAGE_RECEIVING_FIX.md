# Message Receiving - Root Cause Analysis

## Problem Statement

Messages were being sent successfully but not received by the other party.

---

## Root Cause

The message flow had a critical gap in the receiving path:

### Original Design (Broken)

```
Sender:
  1. Create encrypted envelope
  2. Add envelope to IPFS blockstore → Get CID
  3. Publish CID string via PubSub to recipient's topic

Receiver:
  1. Receive CID via PubSub
  2. Fetch envelope from IPFS using CID → ❌ NOT IMPLEMENTED
  3. Decrypt and display message
```

### The Problem

The IPFS `Get()` function was **not fully implemented** in the PoC:

```go
// pkg/ipfsnode/node.go
func (n *Node) Get(cidStr string) ([]byte, error) {
    // ... CID parsing ...
    
    // Get block from DAG service
    block, err := n.dag.Get(n.ctx, c)
    if err != nil {
        return nil, err
    }
    
    // TODO: Return block data
    // For PoC, this flow is not fully implemented
    return nil, fmt.Errorf("IPFS Get not implemented in PoC")
}
```

This meant:
- ✅ CID was successfully published via PubSub
- ❌ CID could not be resolved to the actual envelope data
- ❌ Message decryption never happened
- ❌ User never saw the message

---

## Solution

Instead of storing envelopes in IPFS and sending CIDs, we now send the **encrypted envelope directly** via PubSub:

### New Flow (Working)

```
Sender:
  1. Create encrypted envelope
  2. Publish envelope bytes directly via PubSub ✅

Receiver:
  1. Receive envelope bytes via PubSub ✅
  2. Parse and verify signature ✅
  3. Decrypt with X25519 private key ✅
  4. Display message ✅
```

### Code Changes

**Before** (`pkg/messaging/outgoing.go`):
```go
// Add to IPFS to get CID
cidStr, err := s.ipfsNode.Add(envelopeBytes)
if err != nil {
    return nil, fmt.Errorf("failed to add envelope to IPFS: %w", err)
}

// Publish CID via PubSub
cidBytes := []byte(cidStr)
if err := s.ipfsNode.PublishTo(recipientEd25519PubKey, cidBytes); err != nil {
    return nil, fmt.Errorf("failed to publish CID: %w", err)
}
```

**After** (`pkg/messaging/outgoing.go`):
```go
// Publish envelope directly via PubSub
if err := s.ipfsNode.PublishTo(recipientEd25519PubKey, envelopeBytes); err != nil {
    return nil, fmt.Errorf("failed to publish envelope: %w", err)
}
```

**Before** (`pkg/messaging/protocol.go`):
```go
func (s *Service) handlePubSubMessage(pubsubMsg *ipfsnode.Message) {
    cidStr := string(pubsubMsg.Data)
    
    // Fetch from IPFS - NOT IMPLEMENTED
    envelope, err := s.ipfsNode.Get(cidStr)
    if err != nil {
        return err
    }
    
    s.processEnvelope(envelope)
}
```

**After** (`pkg/messaging/protocol.go`):
```go
func (s *Service) handlePubSubMessage(pubsubMsg *ipfsnode.Message) {
    // Process envelope directly from PubSub message
    if err := s.processEnvelope(pubsubMsg.Data); err != nil {
        return err
    }
}
```

---

## Trade-offs

### Advantages of Direct PubSub

✅ **Works immediately** - No IPFS blockstore dependency
✅ **Simpler flow** - One less hop (no CID generation/fetch)
✅ **Real-time delivery** - Messages arrive instantly
✅ **PoC-friendly** - Focuses on encryption, not storage

### Disadvantages

❌ **No offline messages** - Recipient must be online
❌ **Larger messages** - Envelope sent to all subscribers vs. CID reference
❌ **No persistence** - Messages not stored in IPFS for later retrieval

### Future Enhancement

For production, we can implement a hybrid approach:

```
1. Store envelope in IPFS
2. Publish CID via PubSub
3. Recipient fetches from IPFS
4. If fetch fails, request retransmission via direct message
```

---

## Testing

### Verify Message Receiving Works

**Terminal 1 (Alice):**
```bash
./scripts/test/launch-instance1.sh
>>> /myid  # Copy both keys
>>> /add <bob_ed25519> Bob <bob_x25519>
>>> /chat 1
>>> Hello Bob!
```

**Terminal 2 (Bob):**
```bash
./scripts/test/launch-instance2.sh
>>> /myid  # Copy both keys
>>> /add <alice_ed25519> Alice <alice_x25519>
>>> /chat 1

# Should see:
📬 New message from Alice
[timestamp] Alice: Hello Bob!
```

### Debug Logging

Enable debug logging to see message flow:

```go
// In main.go or via environment
log.SetLogLevel("babylontower/messaging", "debug")
```

Expected log output:

**Sender:**
```
INFO messaging/messaging.go:XX message sent to <pubkey> text="Hello Bob!"
```

**Receiver:**
```
DEBUG messaging/protocol.go:XX received envelope via PubSub size=128 from=<peer_id>
INFO messaging/protocol.go:XX message received from <pubkey> text="Hello Bob!"
```

---

## Files Changed

| File | Lines Changed | Description |
|------|---------------|-------------|
| `pkg/messaging/outgoing.go` | ~30 | Send envelope directly |
| `pkg/messaging/protocol.go` | ~15 | Process envelope directly |
| `pkg/cli/commands.go` | ~5 | Use messaging service |

---

## Conclusion

The root cause was a **missing IPFS Get implementation** in the PoC. By sending envelopes directly via PubSub instead of CIDs, we bypass this limitation and enable bidirectional encrypted messaging.

This is appropriate for the PoC because:
- It demonstrates the encryption protocol
- It works in real-time between two online peers
- It can be extended later with IPFS storage for offline support

---

*Last updated: February 21, 2026*
*Phase 6 Bug Fix - Message Receiving*
