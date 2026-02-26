# Babylon Tower - Manual Testing Checklist

**Version:** 2.0 (Protocol v1)  
**Last Updated:** February 26, 2026  
**Phase:** 18 - Integration & Hardening

---

## Quick Start

```bash
# Run interactive test runner
./scripts/test/run-manual-tests.sh

# Or run individual scenario scripts
./scripts/test/scenario-basic.sh
./scripts/test/scenario-groups.sh
./scripts/test/scenario-multidevice.sh
```

---

## Test Environment Setup

### Prerequisites

- [ ] Go 1.25+ installed
- [ ] GNU Make installed
- [ ] Binary built: `make build`
- [ ] Test data directory clean: `rm -rf test-data/`

### System Requirements

- [ ] 500MB+ free RAM for multi-node tests
- [ ] 1GB+ free disk for IPFS repos
- [ ] Network access (localhost connections allowed)
- [ ] mDNS allowed (UDP 5353) for automatic discovery

---

## Test Category 1: Basic Messaging

### Test 1.1: Two-Instance Communication

**Objective:** Verify two instances can exchange encrypted messages.

**Setup:**
```bash
./scripts/test/launch-instance1.sh  # Terminal 1
./scripts/test/launch-instance2.sh  # Terminal 2
```

**Steps:**

1. [ ] Both instances launch successfully
2. [ ] Run `/myid` in both terminals
3. [ ] Identity fingerprints displayed correctly
4. [ ] Alice adds Bob: `/add <Bob_fingerprint> Bob`
5. [ ] Bob adds Alice: `/add <Alice_fingerprint> Alice`
6. [ ] Contacts appear in `/list`
7. [ ] Alice enters chat: `/chat 1`
8. [ ] Bob enters chat: `/chat 1`
9. [ ] Alice sends: "Hello Bob!"
10. [ ] Bob receives message
11. [ ] Bob replies: "Hi Alice!"
12. [ ] Alice receives reply

**Expected Results:**
- ✅ Messages encrypted with Double Ratchet
- ✅ Forward secrecy maintained
- ✅ Messages stored in BadgerDB
- ✅ Real-time delivery via PubSub

**Persistence Test:**

1. [ ] Both instances exit: `/exit`
2. [ ] Restart both instances
3. [ ] Run `/history Bob` in Alice's terminal
4. [ ] Run `/history Alice` in Bob's terminal
5. [ ] Previous messages displayed

**Acceptance Criteria:**
- [ ] Identity persists across restart
- [ ] Contacts persist across restart
- [ ] Message history persists
- [ ] Session state restored

---

## Test Category 2: Multi-Device

### Test 2.1: Device Registration

**Objective:** Verify device registration with same identity.

**Setup:**
```bash
./scripts/test/scenario-multidevice.sh
```

**Steps:**

1. [ ] Alice Device 1 loads identity
2. [ ] Alice Device 2 loads SAME identity (same mnemonic)
3. [ ] Both devices show same fingerprint
4. [ ] Device IDs are different
5. [ ] Device certificates generated

**Expected Results:**
- ✅ Same identity fingerprint
- ✅ Different device IDs (SHA256(DK_sign.pub)[:16])
- ✅ Device certificates signed by identity key

---

### Test 2.2: Message Fanout

**Objective:** Verify message delivery to multiple devices.

**Steps:**

1. [ ] Bob sends message to Alice
2. [ ] Alice Device 1 receives message
3. [ ] Alice Device 2 receives message
4. [ ] Both devices can decrypt
5. [ ] Bob sees messages from both devices

**Expected Results:**
- ✅ Message encrypted separately for each device
- ✅ All devices decrypt successfully
- ✅ Optimized mode for 5+ devices (symmetric key)

---

### Test 2.3: Cross-Device Sync

**Objective:** Verify state synchronization across devices.

**Steps:**

1. [ ] Alice Device 1 adds contact "Charlie"
2. [ ] Sync message published to `babylon-sync-` topic
3. [ ] Alice Device 2 receives sync message
4. [ ] Contact appears on Device 2
5. [ ] Vector clock prevents conflicts

**Expected Results:**
- ✅ Sync message encrypted with device-group key
- ✅ All devices converge to same state
- ✅ Sync doesn't block user input

---

## Test Category 3: Group Messaging

### Test 3.1: Private Group Creation

**Objective:** Verify private group creation with Sender Keys.

**Setup:**
```bash
./scripts/test/scenario-groups.sh
```

**Steps:**

1. [ ] Alice creates group: `/creategroup 'Project Team' Bob Carol`
2. [ ] Group ID generated
3. [ ] Epoch starts at 0
4. [ ] Alice added as owner
5. [ ] Bob and Carol added as members

**Expected Results:**
- ✅ GroupState created with hash chain
- ✅ Sender Keys generated
- ✅ Invitations sent to members

---

### Test 3.2: Group Message Encryption

**Objective:** Verify O(1) encryption with Sender Keys.

**Steps:**

1. [ ] Alice sends group message
2. [ ] Bob receives and decrypts
3. [ ] Carol receives and decrypts
4. [ ] Message encrypted once (O(1))

**Expected Results:**
- ✅ Sender Keys used for encryption
- ✅ All members decrypt successfully
- ✅ O(1) encryption cost

---

### Test 3.3: Member Removal

**Objective:** Verify member removal with key rotation.

**Steps:**

1. [ ] Alice removes Bob: `/removefromgroup Bob <group_id>`
2. [ ] Epoch increments
3. [ ] Sender Keys rotated
4. [ ] Bob cannot decrypt new messages
5. [ ] Carol can still decrypt

**Expected Results:**
- ✅ Member removed from group
- ✅ Epoch incremented
- ✅ Full key rotation
- ✅ Removed member excluded

---

## Test Category 4: Offline Delivery

### Test 4.1: Mailbox Storage

**Objective:** Verify message storage for offline delivery.

**Setup:** Requires relay node running

**Steps:**

1. [ ] Bob goes offline: `/exit`
2. [ ] Alice sends message to Bob
3. [ ] Message stored in relay node mailbox
4. [ ] Message encrypted for Bob's devices
5. [ ] CID published to Bob's mailbox topic

**Expected Results:**
- ✅ Message encrypted for offline delivery
- ✅ Stored in relay node
- ✅ Mailbox authentication ready

---

### Test 4.2: Mailbox Retrieval

**Objective:** Verify message retrieval on reconnect.

**Steps:**

1. [ ] Bob comes online
2. [ ] Bob discovers relay nodes
3. [ ] Bob subscribes to mailbox topic
4. [ ] Bob retrieves messages
5. [ ] Bob decrypts messages
6. [ ] OPKs replenished from mailbox

**Expected Results:**
- ✅ Messages retrieved from relay
- ✅ All messages decrypted
- ✅ OPKs replenished

---

## Test Category 5: Network Formation

### Test 5.1: Two-Node PubSub

**Objective:** Verify publish/subscribe between two nodes.

**Run:**
```bash
go test -tags=integration -v ./pkg/ipfsnode/... -run TestTwoNodePubSub
```

**Steps:**

1. [ ] Node 1 (Alice) starts
2. [ ] Node 2 (Bob) starts
3. [ ] Nodes connect
4. [ ] Node 1 subscribes to topic
5. [ ] Node 2 publishes message
6. [ ] Node 1 receives message

**Expected Results:**
- ✅ Both nodes start successfully
- ✅ Nodes connect
- ✅ Message delivered
- ✅ Content matches

---

### Test 5.2: Multi-Node Network

**Objective:** Verify 5+ nodes form stable mesh network.

**Run:**
```bash
go test -tags=integration -v ./pkg/ipfsnode/... -run TestMultiNodeNetworkFormation
```

**Acceptance Criteria:**
- [ ] 5 nodes form mesh network within 30 seconds
- [ ] Each node has ≥2 peer connections
- [ ] Each node's DHT routing table has ≥3 peers
- [ ] No node crashes or panics
- [ ] Average connections ≥2
- [ ] Average DHT peers ≥3

---

## Test Category 6: X3DH & Double Ratchet

### Test 6.1: X3DH Session Establishment

**Run:**
```bash
go test -tags=integration -v ./pkg/ratchet/... -run TestX3DHSessionEstablishment
```

**Steps:**

1. [ ] Alice fetches Bob's IdentityDocument from DHT
2. [ ] Alice retrieves Bob's prekey bundle (SPK + OPKs)
3. [ ] Alice performs X3DH, computes shared secret SK
4. [ ] Alice initializes Double Ratchet as initiator
5. [ ] Bob receives message, performs X3DH as responder
6. [ ] Bob initializes Double Ratchet, decrypts message

**Expected Results:**
- ✅ Both parties compute same shared secret SK
- ✅ First message decrypts successfully
- ✅ OPK marked as consumed
- ✅ Ratchet state initialized correctly

---

### Test 6.2: Double Ratchet Message Exchange

**Run:**
```bash
go test -tags=integration -v ./pkg/ratchet/... -run TestDoubleRatchetMessageExchange
```

**Steps:**

1. [ ] Alice and Bob have established session
2. [ ] Alice sends 3 messages to Bob
3. [ ] Bob receives and decrypts all 3
4. [ ] Bob sends 2 messages to Alice
5. [ ] Alice receives and decrypts both

**Expected Results:**
- ✅ All messages decrypt in correct order
- ✅ Ratchet state advances with each message
- ✅ Skipped message keys cached correctly
- ✅ Session state persists across restarts

---

## Test Category 7: Voice/Video Calls

### Test 7.1: WebRTC Call Setup

**Run:**
```bash
go test -tags=integration -v ./pkg/rtc/... -run TestWebRTCOfferAnswerExchange
```

**Steps:**

1. [ ] Alice initiates call to Bob
2. [ ] Alice sends offer via messaging
3. [ ] Bob receives offer, sends answer
4. [ ] ICE candidates exchanged
5. [ ] Call established, media flows

**Expected Results:**
- ✅ SDP offer/answer exchange works
- ✅ ICE candidates delivered
- ✅ Call state transitions correct
- ✅ Media stream established

---

## Test Results Summary

### Pass/Fail Summary

| Category | Tests | Passed | Failed | Skipped |
|----------|-------|--------|--------|---------|
| Basic Messaging | 2 | ___ | ___ | ___ |
| Multi-Device | 3 | ___ | ___ | ___ |
| Group Messaging | 3 | ___ | ___ | ___ |
| Offline Delivery | 2 | ___ | ___ | ___ |
| Network Formation | 2 | ___ | ___ | ___ |
| X3DH & Ratchet | 2 | ___ | ___ | ___ |
| Voice/Video | 1 | ___ | ___ | ___ |
| **Total** | **15** | ___ | ___ | ___ |

### Issues Found

| ID | Description | Severity | Status |
|----|-------------|----------|--------|
| 1  |             |          |        |
| 2  |             |          |        |
| 3  |             |          |        |

### Known Limitations

1. 
2. 
3. 

---

## Sign-Off

**Tester:** ________________________  
**Date:** ________________________  
**Version:** ________________________  

**Recommendation:**
- [ ] Ready for release
- [ ] Needs fixes
- [ ] Needs more testing

**Comments:**

---

*For detailed test scenarios, see `specs/testing.md`*
