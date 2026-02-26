#!/bin/bash

# Babylon Tower - Basic Messaging E2E Scenario
# This script automates the basic messaging test scenario
#
# Usage:
#   ./scripts/test/scenario-basic.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
BINARY="$PROJECT_ROOT/bin/messenger"
TEST_DATA_DIR="$PROJECT_ROOT/test-data/scenario-basic"

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

log_error() {
    echo -e "${RED}[✗]${NC} $1"
}

show_banner() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║     Babylon Tower - Basic Messaging E2E Scenario          ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
}

cleanup() {
    log_info "Cleaning up..."
    rm -rf "$TEST_DATA_DIR"
    pkill -f "messenger.*scenario-basic" 2>/dev/null || true
}

setup() {
    show_banner
    log_info "Setting up test environment..."
    
    # Clean previous test data
    cleanup
    
    # Create test directories
    mkdir -p "$TEST_DATA_DIR/alice"
    mkdir -p "$TEST_DATA_DIR/bob"
    
    # Check binary
    if [ ! -f "$BINARY" ]; then
        log_info "Building messenger..."
        cd "$PROJECT_ROOT"
        make build >/dev/null 2>&1
    fi
    
    log_success "Setup complete"
}

run_test() {
    show_banner
    log_info "Running Basic Messaging E2E Test"
    echo ""
    
    echo "Test Scenario:"
    echo "  1. Launch Alice and Bob instances"
    echo "  2. Exchange identity fingerprints"
    echo "  3. Add contacts"
    echo "  4. Establish secure session"
    echo "  5. Exchange encrypted messages"
    echo "  6. Verify persistence"
    echo ""
    
    # Start Alice
    log_info "Starting Alice..."
    export HOME="$TEST_DATA_DIR/alice"
    "$BINARY" &
    ALICE_PID=$!
    sleep 3
    
    # Start Bob
    log_info "Starting Bob..."
    export HOME="$TEST_DATA_DIR/bob"
    "$BINARY" &
    BOB_PID=$!
    sleep 3
    
    echo ""
    log_success "Both instances running"
    echo "  Alice PID: $ALICE_PID"
    echo "  Bob PID: $BOB_PID"
    echo ""
    
    # Wait for user to complete manual steps
    log_info "Manual verification required:"
    echo ""
    echo "In Alice's terminal window:"
    echo "  1. Run: /myid"
    echo "  2. Note the identity fingerprint"
    echo ""
    echo "In Bob's terminal window:"
    echo "  1. Run: /myid"
    echo "  2. Note the identity fingerprint"
    echo ""
    echo "Then add each other as contacts:"
    echo "  Alice: /add <Bob's_fingerprint> Bob"
    echo "  Bob: /add <Alice's_fingerprint> Alice"
    echo ""
    
    read -p "Press Enter when contacts are added..."
    
    # Verify session establishment
    echo ""
    log_info "Verifying session establishment..."
    
    echo "In Alice's terminal:"
    echo "  /chat 1"
    echo "  Hello Bob! This is an encrypted test message."
    echo ""
    
    echo "In Bob's terminal:"
    echo "  /chat 1"
    echo "  Hi Alice! Message received successfully."
    echo ""
    
    read -p "Press Enter after exchanging messages..."
    
    # Verify persistence
    echo ""
    log_info "Testing persistence..."
    
    echo "Exit both instances (/exit) and restart them."
    echo "Then run /history to verify messages persisted."
    echo ""
    
    read -p "Press Enter after verifying persistence..."
    
    # Cleanup
    cleanup
    
    echo ""
    log_success "Basic messaging E2E test complete!"
}

# Main
setup
run_test

echo ""
echo "Test completed. Review results above."
echo ""
echo "To clean test data manually:"
echo "  rm -rf $TEST_DATA_DIR"
echo ""
