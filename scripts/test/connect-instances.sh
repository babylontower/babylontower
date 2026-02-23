#!/bin/bash

# Babylon Tower - Connect Two Instances
# This script helps connect two running instances
#
# Usage:
#   ./scripts/test/connect-instances.sh [mode] [instance1_addr] [instance2_addr]
#
# Modes:
#   manual - Show connection instructions (default)
#   auto   - Automatically send connect commands (requires tmux)
#
# Examples:
#   ./scripts/test/connect-instances.sh
#   ./scripts/test/connect-instances.sh manual
#   ./scripts/test/connect-instances.sh auto "/ip4/.../p2p/QmAlice" "/ip4/.../p2p/QmBob"

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
TEST_DATA_DIR="$PROJECT_ROOT/test-data"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

show_help() {
    echo "Babylon Tower - Connect Two Instances"
    echo ""
    echo "Usage:"
    echo "  $0 [mode] [instance1_multiaddr] [instance2_multiaddr]"
    echo ""
    echo "Modes:"
    echo "  manual - Show connection instructions (default)"
    echo "  auto   - Automatically send connect commands via tmux"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Interactive mode"
    echo "  $0 manual                             # Show instructions"
    echo "  $0 auto /ip4/.../p2p/QmAlice /ip4/... # Auto-connect"
    echo ""
    echo "Manual Steps:"
    echo "  1. Start two instances:"
    echo "     ./scripts/test/launch-instance1.sh"
    echo "     ./scripts/test/launch-instance2.sh"
    echo "  2. In each instance, run: /myaddr"
    echo "  3. Copy the multiaddrs"
    echo "  4. Run this script with the multiaddrs"
    echo "  5. In each instance, run: /connect <multiaddr>"
    echo "  6. Verify with: /peers"
    echo ""
    echo "Auto Mode (requires tmux):"
    echo "  Automatically sends /connect commands to running instances"
    echo ""
}

# Auto-connect mode using tmux
auto_connect() {
    local addr1="$1"
    local addr2="$2"
    
    if ! command -v tmux &> /dev/null; then
        log_error "tmux not found. Auto mode requires tmux."
        log_info "Install tmux or use manual mode: $0 manual"
        exit 1
    fi
    
    log_info "Auto-connect mode using tmux..."
    echo ""
    
    # Find tmux sessions for instances
    session1=$(tmux list-sessions 2>/dev/null | grep -i "instance1\|alice" | cut -d: -f1 | head -1)
    session2=$(tmux list-sessions 2>/dev/null | grep -i "instance2\|bob" | cut -d: -f1 | head -1)
    
    if [ -z "$session1" ] || [ -z "$session2" ]; then
        log_warn "Could not find tmux sessions for instances"
        log_info "Falling back to manual instructions..."
        echo ""
        show_manual_instructions "$addr1" "$addr2"
        return
    fi
    
    log_info "Found sessions: $session1, $session2"
    log_info "Sending connect commands..."
    
    # Send connect commands
    tmux send-keys -t "$session1" "/connect $addr2" C-m
    tmux send-keys -t "$session2" "/connect $addr1" C-m
    
    sleep 2
    
    # Verify connections
    log_info "Verifying connections..."
    tmux send-keys -t "$session1" "/peers" C-m
    tmux send-keys -t "$session2" "/peers" C-m
    
    echo ""
    log_success "Connect commands sent!"
    log_info "Check the tmux sessions to verify connections:"
    echo "  tmux attach -t $session1"
    echo "  tmux attach -t $session2"
}

show_manual_instructions() {
    local addr1="$1"
    local addr2="$2"
    
    log_info "Connecting two instances..."
    echo ""
    log_info "Instance 1 should connect to:"
    echo "  $addr2"
    echo ""
    log_info "Instance 2 should connect to:"
    echo "  $addr1"
    echo ""
    log_info "In each instance, run:"
    echo "  /connect <multiaddr>"
    echo ""
    log_info "Then verify with:"
    echo "  /peers"
    echo ""
    log_info "Alternative: Use DHT discovery"
    echo "  1. Ensure both instances are connected to bootstrap peers"
    echo "  2. Run /waitdht on both instances"
    echo "  3. Use /find <peer_id> to discover each other"
    echo ""
    log_success "Ready to connect!"
}

# Main logic

# Check for help flag
if [ "$1" = "help" ] || [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
    show_help
    exit 0
fi

# Parse mode
MODE="${1:-manual}"
ADDR1="$2"
ADDR2="$3"

case "$MODE" in
    manual)
        if [ -z "$ADDR1" ] || [ -z "$ADDR2" ]; then
            show_help
            exit 1
        fi
        show_manual_instructions "$ADDR1" "$ADDR2"
        ;;
    auto)
        if [ -z "$ADDR1" ] || [ -z "$ADDR2" ]; then
            log_error "Auto mode requires two multiaddrs"
            echo ""
            show_help
            exit 1
        fi
        auto_connect "$ADDR1" "$ADDR2"
        ;;
    *)
        log_error "Unknown mode: $MODE"
        echo ""
        show_help
        exit 1
        ;;
esac
