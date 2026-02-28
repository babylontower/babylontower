#!/bin/bash

# Babylon Tower - Groups Messaging E2E Scenario
# This script automates the groups messaging test scenario
#
# Usage:
#   ./scripts/test/scenario-groups.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
BINARY="$PROJECT_ROOT/bin/messenger"
TEST_DATA_DIR="$PROJECT_ROOT/test-data/scenario-groups"

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
    echo "║     Babylon Tower - Groups Messaging E2E Scenario         ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
}

cleanup() {
    log_info "Cleaning up..."
    rm -rf "$TEST_DATA_DIR"
    pkill -f "messenger.*scenario-groups" 2>/dev/null || true
}

setup() {
    show_banner
    log_info "Setting up groups test environment..."
    
    cleanup
    
    # Create test directories for 3 participants
    mkdir -p "$TEST_DATA_DIR/alice"
    mkdir -p "$TEST_DATA_DIR/bob"
    mkdir -p "$TEST_DATA_DIR/carol"
    
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
    log_info "Running Groups Messaging E2E Test"
    echo ""
    
    echo "Test Scenario:"
    echo "  1. Launch Alice, Bob, and Carol instances"
    echo "  2. Alice creates a private group 'Project Team'"
    echo "  3. Alice adds Bob and Carol to the group"
    echo "  4. All members exchange group messages"
    echo "  5. Alice removes Bob from the group"
    echo "  6. Verify Bob cannot decrypt new messages"
    echo "  7. Carol continues messaging with Alice"
    echo ""
    
    PIDS=()
    
    # Start all participants
    for participant in alice bob carol; do
        log_info "Starting $participant..."
        export HOME="$TEST_DATA_DIR/$participant"
        "$BINARY" &
        PIDS+=($!)
        sleep 2
    done
    
    echo ""
    log_success "All instances running"
    echo "  Alice PID: ${PIDS[0]}"
    echo "  Bob PID:   ${PIDS[1]}"
    echo "  Carol PID: ${PIDS[2]}"
    echo ""
    
    # Manual test steps
    log_header "Step 1: Add Contacts"
    echo ""
    echo "All participants add each other as contacts:"
    echo ""
    echo "In each terminal:"
    echo "  >>> /myid"
    echo "  (Note the fingerprint)"
    echo ""
    echo "Then add others:"
    echo "  >>> /add <fingerprint> <name>"
    echo ""
    
    read -p "Press Enter when contacts are added..."
    
    log_header "Step 2: Create Group"
    echo ""
    echo "In Alice's terminal:"
    echo "  >>> /creategroup 'Project Team' Bob Carol"
    echo ""
    echo "Expected output:"
    echo "  ✓ Group created: grp_<id>"
    echo "  ✓ Members added: Bob, Carol"
    echo ""
    
    read -p "Press Enter after group creation..."
    
    log_header "Step 3: Group Messaging"
    echo ""
    echo "In Alice's terminal:"
    echo "  >>> /groupchat <group_id>"
    echo "  >>> Welcome to the team!"
    echo ""
    echo "In Bob's and Carol's terminals:"
    echo "  Verify message appears"
    echo "  >>> /groupchat <group_id>"
    echo "  >>> Thanks for adding me!"
    echo ""
    
    read -p "Press Enter after group messaging..."
    
    log_header "Step 4: Remove Member"
    echo ""
    echo "In Alice's terminal:"
    echo "  >>> /removefromgroup Bob <group_id>"
    echo ""
    echo "Expected:"
    echo "  ✓ Bob removed from group"
    echo "  ✓ Sender Keys rotated (epoch++)"
    echo ""
    
    read -p "Press Enter after removing Bob..."
    
    log_header "Step 5: Verify Removal"
    echo ""
    echo "In Bob's terminal:"
    echo "  Try to send group message:"
    echo "  >>> /groupchat <group_id>"
    echo "  >>> Can you still see this?"
    echo ""
    echo "Expected:"
    echo "  ✗ Error: You are no longer a member"
    echo ""
    echo "In Alice's and Carol's terminals:"
    echo "  Send new message:"
    echo "  >>> Message after Bob removed"
    echo ""
    echo "In Bob's terminal:"
    echo "  Verify message NOT decrypted"
    echo ""
    
    read -p "Press Enter after verification..."
    
    # Cleanup
    cleanup
    
    echo ""
    log_success "Groups messaging E2E test complete!"
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
