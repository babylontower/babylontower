#!/bin/bash

# Babylon Tower - Multi-Node Test Launcher
# This script launches N nodes for scale testing (Phase 6)
#
# Usage:
#   ./scripts/test/launch-multi-node.sh [node_count] [mode]
#
# Arguments:
#   node_count - Number of nodes to launch (default: 5)
#   mode       - Launch mode: "background", "tmux", or "screen" (default: background)
#
# Examples:
#   ./scripts/test/launch-multi-node.sh 5           # Launch 5 nodes in background
#   ./scripts/test/launch-multi-node.sh 10 tmux     # Launch 10 nodes in tmux panes
#   ./scripts/test/launch-multi-node.sh clean       # Clean all test data

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
TEST_DATA_DIR="$PROJECT_ROOT/test-data"
DEFAULT_NODE_COUNT=5

# Detect current platform
detect_platform() {
    case "$(uname -s)" in
        Linux*)     echo "linux" ;;
        Darwin*)    echo "darwin" ;;
        MINGW*|CYGWIN*|MSYS*) echo "windows" ;;
        *)          echo "linux" ;;
    esac
}

PLATFORM=$(detect_platform)

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

show_banner() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║     Babylon Tower - Multi-Node Test Launcher              ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
}

show_help() {
    show_banner
    echo "Usage:"
    echo "  $0 [node_count] [mode]"
    echo ""
    echo "Arguments:"
    echo "  node_count - Number of nodes to launch (default: $DEFAULT_NODE_COUNT)"
    echo "  mode       - Launch mode: background, tmux, or screen (default: background)"
    echo ""
    echo "Examples:"
    echo "  $0 5              # Launch 5 nodes in background"
    echo "  $0 10 tmux        # Launch 10 nodes in tmux panes"
    echo "  $0 clean          # Clean all test data"
    echo ""
    echo "Modes:"
    echo "  background - Run nodes in background (good for automated tests)"
    echo "  tmux       - Run nodes in tmux panes (good for monitoring)"
    echo "  screen     - Run nodes in screen windows (alternative to tmux)"
    echo ""
}

# Clean test data
clean_test_data() {
    log_warn "This will remove all test data from:"
    echo "  $TEST_DATA_DIR"
    echo ""
    
    if [ -t 0 ]; then
        read -p "Are you sure you want to continue? [y/N] " -n 1 -r
        echo ""
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Cancelled"
            exit 0
        fi
    fi
    
    log_info "Removing test data..."
    rm -rf "$TEST_DATA_DIR"
    log_success "Test data cleaned successfully"
}

# Check if binary exists
check_binary() {
    BINARY=""
    
    # Check platform-specific binary
    PLATFORM_BINARY="$PROJECT_ROOT/bin/platform/$PLATFORM/messenger"
    if [ "$PLATFORM" = "windows" ]; then
        PLATFORM_BINARY="$PROJECT_ROOT/bin/platform/$PLATFORM/messenger.exe"
    fi
    
    # Check standard binary
    STANDARD_BINARY="$PROJECT_ROOT/bin/messenger"
    if [ "$PLATFORM" = "windows" ]; then
        STANDARD_BINARY="$PROJECT_ROOT/bin/messenger.exe"
    fi
    
    if [ -f "$PLATFORM_BINARY" ]; then
        BINARY="$PLATFORM_BINARY"
        log_info "Using platform binary: $BINARY"
    elif [ -f "$STANDARD_BINARY" ]; then
        BINARY="$STANDARD_BINARY"
        log_info "Using standard binary: $BINARY"
    else
        log_info "Binary not found. Building..."
        cd "$PROJECT_ROOT"
        make build >/dev/null 2>&1
        BINARY="$STANDARD_BINARY"
    fi
}

# Launch nodes in background mode
launch_background() {
    local node_count=$1
    local pids=()
    
    log_info "Launching $node_count nodes in background mode..."
    echo ""
    
    for i in $(seq 1 $node_count); do
        DATA_DIR="$TEST_DATA_DIR/node-$i"
        mkdir -p "$DATA_DIR"
        
        log_info "Starting node $i..."
        export HOME="$DATA_DIR"
        
        # Start node in background
        cd "$PROJECT_ROOT"
        "$BINARY" > "$DATA_DIR/output.log" 2>&1 &
        pid=$!
        pids+=($pid)
        
        echo "$pid" > "$DATA_DIR/pid"
        log_info "Node $i started (PID: $pid, Data: $DATA_DIR)"
    done
    
    echo ""
    log_success "All $node_count nodes launched!"
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Node Status:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    for i in $(seq 1 $node_count); do
        DATA_DIR="$TEST_DATA_DIR/node-$i"
        if [ -f "$DATA_DIR/pid" ]; then
            pid=$(cat "$DATA_DIR/pid")
            if ps -p $pid > /dev/null 2>&1; then
                echo "  Node $i: ✓ Running (PID: $pid)"
            else
                echo "  Node $i: ✗ Stopped"
            fi
        fi
    done
    
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Next Steps:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  1. Wait 30 seconds for network formation"
    echo "  2. Check node output: tail -f $TEST_DATA_DIR/node-1/output.log"
    echo "  3. Verify connections: ./scripts/test/verify-connections.sh"
    echo "  4. Stop all nodes: ./scripts/test/stop-multi-node.sh"
    echo "  5. Clean data: $0 clean"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}

# Launch nodes in tmux mode
launch_tmux() {
    local node_count=$1
    
    # Check if tmux is available
    if ! command -v tmux &> /dev/null; then
        log_error "tmux not found. Please install tmux or use 'background' mode."
        exit 1
    fi
    
    local session_name="babylon-test-$$"
    
    log_info "Launching $node_count nodes in tmux session: $session_name"
    echo ""
    
    # Create tmux session
    tmux new-session -d -s "$session_name" -n "node-1"
    
    for i in $(seq 1 $node_count); do
        DATA_DIR="$TEST_DATA_DIR/node-$i"
        mkdir -p "$DATA_DIR"
        
        if [ $i -gt 1 ]; then
            tmux split-window -t "$session_name"
        fi
        
        tmux select-layout -t "$session_name" tiled
        
        export HOME="$DATA_DIR"
        tmux send-keys -t "$session_name" "cd $PROJECT_ROOT && HOME=$DATA_DIR $BINARY" C-m
        
        log_info "Node $i started in tmux pane"
    done
    
    # Attach to session if running interactively
    if [ -t 0 ]; then
        log_info "Attaching to tmux session. Press Ctrl+B, then D to detach."
        tmux attach-session -t "$session_name"
    else
        log_info "Nodes launched in tmux session: $session_name"
        log_info "Attach with: tmux attach -t $session_name"
    fi
}

# Main logic
show_banner

# Handle special commands
if [ "$1" = "clean" ]; then
    clean_test_data
    exit 0
fi

if [ "$1" = "help" ] || [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
    show_help
    exit 0
fi

# Parse arguments
NODE_COUNT="${1:-$DEFAULT_NODE_COUNT}"
MODE="${2:-background}"

# Validate node count
if ! [[ "$NODE_COUNT" =~ ^[0-9]+$ ]] || [ "$NODE_COUNT" -lt 1 ] || [ "$NODE_COUNT" -gt 50 ]; then
    log_error "Invalid node count: $NODE_COUNT (must be 1-50)"
    exit 1
fi

# Validate mode
case "$MODE" in
    background|bg)
        MODE="background"
        ;;
    tmux)
        MODE="tmux"
        ;;
    screen)
        log_warn "Screen mode not yet implemented, using background mode"
        MODE="background"
        ;;
    *)
        log_error "Unknown mode: $MODE (use: background, tmux, or screen)"
        exit 1
        ;;
esac

# Check binary
check_binary

# Launch nodes
case "$MODE" in
    background)
        launch_background "$NODE_COUNT"
        ;;
    tmux)
        launch_tmux "$NODE_COUNT"
        ;;
esac

log_success "Multi-node test environment ready!"
