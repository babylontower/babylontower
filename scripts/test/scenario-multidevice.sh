#!/bin/bash

# Babylon Tower - Multi-Device E2E Scenario
# This script automates the multi-device sync test scenario
#
# Usage:
#   ./scripts/test/scenario-multidevice.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
BINARY="$PROJECT_ROOT/bin/messenger"
TEST_DATA_DIR="$PROJECT_ROOT/test-data/scenario-multidevice"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

show_banner() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║    Babylon Tower - Multi-Device E2E Scenario              ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
}

cleanup() {
    log_info "Cleaning up..."
    rm -rf "$TEST_DATA_DIR"
    pkill -f "messenger.*scenario-multidevice" 2>/dev/null || true
}

setup() {
    show_banner
    log_info "Setting up multi-device test environment..."
    
    cleanup
    
    # Create test directories
    # Alice has 2 devices, Bob has 1
    mkdir -p "$TEST_DATA_DIR/alice-device1"
    mkdir -p "$TEST_DATA_DIR/alice-device2"
    mkdir -p "$TEST_DATA_DIR/bob"
    
    if [ ! -f "$BINARY" ]; then
        log_info "Building messenger..."
        cd "$PROJECT_ROOT"
        make build >/dev/null 2>&1
    fi
    
    log_success "Setup complete"
}

run_test() {
    show_banner
    log_info "Running Multi-Device E2E Test"
    echo ""
    
    echo "Test Scenario:"
    echo "  1. Alice loads same identity on Device 1 and Device 2"
    echo "  2. Bob sends message to Alice"
    echo "  3. Both Alice's devices receive the message"
    echo "  4. Alice adds contact on Device 1"
    echo "  5. Device 2 receives sync message"
    echo "  6. Contact appears on both devices"
    echo ""
    
    PIDS=()
    
    # Start all devices
    for device in alice-device1 alice-device2 bob; do
        log_info "Starting $device..."
        export HOME="$TEST_DATA_DIR/$device"
        "$BINARY" &
        PIDS+=($!)
        sleep 2
    done
    
    echo ""
    log_success "All instances running"
    echo "  Alice Device 1 PID: ${PIDS[0]}"
    echo "  Alice Device 2 PID: ${PIDS[1]}"
    echo "  Bob PID:            ${PIDS[2]}"
    echo ""
    
    log_header "Step 1: Load Same Identity"
    echo ""
    echo "On both Alice's devices, enter the SAME mnemonic:"
    echo ""
    echo "Device 1:"
    echo "  Enter mnemonic: <Alice's_mnemonic>"
    echo ""
    echo "Device 2:"
    echo "  Enter mnemonic: <Alice's_mnemonic> (same as Device 1)"
    echo ""
    echo "Verify both show same identity fingerprint:"
    echo "  >>> /myid"
    echo ""
    
    read -p "Press Enter after loading identity..."
    
    log_header "Step 2: Bob Sends Message"
    echo ""
    echo "Bob adds Alice (using fingerprint from either device):"
    echo "  >>> /add <Alice_fingerprint> Alice"
    echo ""
    echo "Bob sends message:"
    echo "  >>> /chat 1"
    echo "  >>> Hello Alice!"
    echo ""
    
    read -p "Press Enter after Bob sends message..."
    
    log_header "Step 3: Verify Fanout"
    echo ""
    echo "On both Alice's devices:"
    echo "  Verify message appears"
    echo "  >>> /chat 1"
    echo "  >>> Hi Bob! Got your message on <device_name>"
    echo ""
    echo "Bob should see both messages from Alice's devices."
    echo ""
    
    read -p "Press Enter after verifying fanout..."
    
    log_header "Step 4: Cross-Device Sync"
    echo ""
    echo "On Alice's Device 1:"
    echo "  >>> /add Charlie"
    echo "  ✓ Contact added: Charlie"
    echo ""
    echo "On Alice's Device 2:"
    echo "  Wait for sync message..."
    echo "  📬 Sync message received"
    echo "  ✓ Contact list updated: Charlie added"
    echo ""
    
    read -p "Press Enter after sync verification..."
    
    log_header "Step 5: Verify Convergence"
    echo ""
    echo "On both Alice's devices:"
    echo "  >>> /list"
    echo ""
    echo "Verify both show:"
    echo "  1. Bob"
    echo "  2. Charlie"
    echo ""
    
    read -p "Press Enter after verifying convergence..."
    
    # Cleanup
    cleanup
    
    echo ""
    log_success "Multi-device E2E test complete!"
}

log_header() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "$1"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

# Main
setup
run_test

echo ""
echo "Test completed. Review results above."
echo ""
