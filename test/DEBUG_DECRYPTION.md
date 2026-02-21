# Debugging Message Decryption Issues

## Current Status

Messages are being sent and received, but decryption shows "[Encrypted message received]" placeholder.

## Debug Logging Enabled

Debug logging has been enabled in the following areas:

### Encryption (Sender side)
```
DEBUG messaging: building envelope recipient_pub=6e44261d... plaintext_len=23
DEBUG messaging: computed shared secret secret=abc123...
DEBUG messaging: envelope built ephemeral_pub=def456... nonce_len=24 ciphertext_len=55
```

### Decryption (Receiver side)
```
DEBUG messaging: received envelope via PubSub size=128 from=12D3KooW...
DEBUG messaging: decrypting envelope ephemeral_pub=def456... nonce_len=24 ciphertext_len=55
DEBUG messaging: computed shared secret secret=xyz789...
DEBUG messaging: decryption successful plaintext_len=23
INFO messaging: message event emitted from=840b6097... text="Hi Bob!"
```

## How to Test

### 1. Start Both Instances with Logging

**Terminal 1 (Alice):**
```bash
./scripts/test/launch-instance1.sh 2>&1 | tee alice.log
```

**Terminal 2 (Bob):**
```bash
./scripts/test/launch-instance2.sh 2>&1 | tee bob.log
```

### 2. Connect Nodes

In Bob's terminal:
```bash
>>> /myid
# Copy Alice's multiaddr

>>> /connect /ip4/127.0.0.1/tcp/PORT/p2p/PEER_ID
```

### 3. Add Contacts with X25519 Keys

Both users exchange keys via `/myid` and add each other:
```bash
>>> /add <ed25519_key> <name> <x25519_key>
```

### 4. Send Test Message

In Alice's terminal:
```bash
>>> /chat 1
>>> Test message 123
```

### 5. Check Logs

**In Bob's log, look for:**

✅ **Success pattern:**
```
DEBUG messaging: received envelope via PubSub
DEBUG messaging: decrypting envelope
DEBUG messaging: computed shared secret
DEBUG messaging: decryption successful plaintext_len=15
INFO messaging: message event emitted text="Test message 123"
```

❌ **Failure patterns:**

1. **Shared secret mismatch:**
   ```
   DEBUG messaging: computed shared secret secret=DIFFERENT_FROM_SENDER
   ERROR messaging: failed to process envelope error="decryption failed"
   ```

2. **Wrong X25519 key used:**
   ```
   DEBUG messaging: building envelope recipient_pub=KEY1
   DEBUG messaging: decrypting envelope (uses KEY2's private key)
   ERROR messaging: decryption failed: MAC verification failed
   ```

3. **Message event not emitted:**
   ```
   DEBUG messaging: decryption successful
   (no "message event emitted" log)
   ```

## Common Issues and Solutions

### Issue: "decryption failed: MAC verification failed"

**Cause:** The X25519 keys don't match between sender and receiver.

**Solution:**
1. Verify you added the contact with the CORRECT X25519 public key
2. The X25519 key shown in `/myid` must be shared with your contact
3. Re-add the contact with the correct key:
   ```bash
   >>> /add <ed25519_key> <name> <CORRECT_x25519_key>
   ```

### Issue: "failed to compute shared secret"

**Cause:** Invalid key format or length.

**Solution:**
1. Ensure X25519 key is 32 bytes (64 hex chars or ~44 base58 chars)
2. Re-copy the key from `/myid` output

### Issue: Message shows "[Encrypted message received]"

**Cause:** Decryption failed silently or message event not processed.

**Solution:**
1. Check Bob's logs for "decryption successful"
2. If decryption successful but message not shown, check CLI handler
3. Verify the message event is being received by CLI

## Key Verification

To verify keys are correct:

**Alice's X25519 keys:**
```
Public (shared with Bob):  8RS7qsXFkREGqvcLfs1iMMVUyw4x8TLQQiLizJsSpNvz
Private (never shared):    (stored in identity.json)
```

**Bob's X25519 keys:**
```
Public (shared with Alice): FSHhNrfai3h8HCti29RoRREcHCWBdGnxrWvzMDf6VVX6
Private (never shared):     (stored in identity.json)
```

**When Alice sends to Bob:**
- Alice uses: Bob's X25519 PUBLIC key for encryption
- Bob uses: His own X25519 PRIVATE key for decryption

**When Bob sends to Alice:**
- Bob uses: Alice's X25519 PUBLIC key for encryption
- Alice uses: Her own X25519 PRIVATE key for decryption

## Quick Test

```bash
# Alice's terminal
>>> /myid
# Note the X25519 public key hex

# Bob's terminal  
>>> /myid
# Note the X25519 public key hex

# Alice adds Bob with Bob's X25519 key
>>> /add <bob_ed25519> Bob <BOB_X25519_HEX>

# Bob adds Alice with Alice's X25519 key
>>> /add <alice_ed25519> Alice <ALICE_X25519_HEX>

# Connect nodes
>>> /connect <alice_multiaddr>

# Test
>>> /chat 1
>>> decryption test
```

Check logs on both sides - you should see matching shared secrets computed.

---

*Last updated: February 21, 2026*
*Debugging Guide for Message Decryption*
