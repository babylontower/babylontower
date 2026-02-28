# Babylon Tower - Quick Start Testing Guide

**5-Minute Setup for New Contributors**

---

## Prerequisites

- Go 1.25+ installed
- GNU Make installed
- Git cloned repository

---

## Quick Start (5 Minutes)

### Step 1: Build the Application (1 min)

```bash
cd /workspaces/babylontower
make build
```

Expected output:
```
Building messenger...
✅ Binary created: bin/messenger
```

---

### Step 2: Run Unit Tests (2 min)

```bash
make test
```

Expected output:
```
Running tests...
ok   babylontower/pkg/crypto    0.5s
ok   babylontower/pkg/identity  0.8s
ok   babylontower/pkg/ratchet   1.2s
...
```

---

### Step 3: Launch Two Instances (2 min)

**Terminal 1:**
```bash
./scripts/test/launch-instance1.sh
```

**Terminal 2:**
```bash
./scripts/test/launch-instance2.sh
```

---

### Step 4: Test Messaging (5 min)

In **Terminal 1** (Alice):
```
>>> /myid
# Copy the identity fingerprint

>>> /add <Bob's_fingerprint> Bob
>>> /chat 1
>>> Hello Bob! This is a test.
```

In **Terminal 2** (Bob):
```
>>> /myid
# Copy the identity fingerprint

>>> /add <Alice's_fingerprint> Alice
>>> /chat 1
# You should see Alice's message
>>> Hi Alice! Received your message.
```

---

## What You Just Tested

✅ **X3DH Key Exchange** - Secure session establishment  
✅ **Double Ratchet** - Forward-secret message encryption  
✅ **IPFS PubSub** - Decentralized message delivery  
✅ **BadgerDB** - Local message persistence  
✅ **Ed25519 Signatures** - Message authentication  

---

## Next Steps

### Run Integration Tests

```bash
# X3DH & Double Ratchet
go test -tags=integration -v ./pkg/ratchet/...

# Groups
go test -tags=integration -v ./pkg/groups/...

# Network
go test -tags=integration -v ./pkg/ipfsnode/...
```

### Run Scenario Tests

```bash
# Basic messaging
./scripts/test/scenario-basic.sh

# Groups
./scripts/test/scenario-groups.sh

# Multi-device
./scripts/test/scenario-multidevice.sh
```

### Interactive Test Runner

```bash
./scripts/test/run-manual-tests.sh
```

---

## Common Issues

### Issue: Binary not found

**Solution:**
```bash
make build
```

### Issue: Port already in use

**Solution:**
```bash
# Kill existing processes
pkill -f messenger

# Clean test data
./scripts/test/clean-test-data.sh
```

### Issue: Tests timeout

**Solution:**
```bash
# Increase timeout
go test -tags=integration -timeout 10m ./...
```

---

## Test Coverage Report

```bash
make test-coverage
# Opens coverage.html in browser
```

---

## Learn More

- **Full Testing Spec:** [`specs/testing.md`](specs/testing.md)
- **Manual Test Checklist:** [`scripts/test/MANUAL-TEST-CHECKLIST.md`](scripts/test/MANUAL-TEST-CHECKLIST.md)
- **Test Scripts README:** [`scripts/test/README.md`](scripts/test/README.md)

---

## Get Help

- Check existing issues on GitHub
- Review error logs in `test-data/`
- Ask in project chat/channel

---

*Last updated: February 26, 2026*  
*Version: 2.0 (Protocol v1)*
