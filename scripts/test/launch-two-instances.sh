#!/bin/bash

# Babylon Tower - Two-Instance Test Launcher
# This script launches two instances of Babylon Tower for manual testing
#
# Usage:
#   ./scripts/test/launch-two-instances.sh [mode]
#
# Modes:
#   local    - Launch two instances locally (default)
#   docker   - Launch two instances in Docker containers
#   clean    - Clean test data and exit
#   help     - Show this help message

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
TEST_DATA_DIR="$PROJECT_ROOT/test-data"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
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
    echo "║         Babylon Tower - Two-Instance Test Setup           ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
}

clean_test_data() {
    log_info "Cleaning test data directory..."
    if [ -d "$TEST_DATA_DIR" ]; then
        rm -rf "$TEST_DATA_DIR"
        log_success "Test data cleaned"
    else
        log_info "No test data to clean"
    fi
}

setup_test_data() {
    log_info "Setting up test data directories..."
    mkdir -p "$TEST_DATA_DIR/instance1"
    mkdir -p "$TEST_DATA_DIR/instance2"
    log_success "Test data directories created"
}

launch_local() {
    show_banner
    log_info "Launching two instances locally..."
    echo ""
    
    # Check if binary exists
    BINARY="$PROJECT_ROOT/bin/messenger"
    if [ ! -f "$BINARY" ]; then
        log_warn "Binary not found. Building..."
        cd "$PROJECT_ROOT"
        make build
    fi
    
    setup_test_data
    
    log_info "Starting Instance 1 (Alice) in terminal..."
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Instance 1 (Alice) - Data: $TEST_DATA_DIR/instance1"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    # Launch first instance in a new terminal window (if available)
    if command -v gnome-terminal &> /dev/null; then
        # GNOME Terminal
        gnome-terminal -- bash -c "
            export HOME=$TEST_DATA_DIR/instance1
            cd '$PROJECT_ROOT'
            echo '=== Instance 1 (Alice) ==='
            echo 'Data directory: $TEST_DATA_DIR/instance1'
            echo ''
            ./bin/messenger
            exec bash
        " &
        PID1=$!
        log_success "Instance 1 launched (PID: $PID1)"
    elif command -v x-terminal-emulator &> /dev/null; then
        # Generic X terminal
        x-terminal-emulator -e bash -c "
            export HOME=$TEST_DATA_DIR/instance1
            cd '$PROJECT_ROOT'
            echo '=== Instance 1 (Alice) ==='
            echo 'Data directory: $TEST_DATA_DIR/instance1'
            echo ''
            ./bin/messenger
            exec bash
        " &
        PID1=$!
        log_success "Instance 1 launched (PID: $PID1)"
    elif command -v tmux &> /dev/null; then
        # tmux
        tmux new-session -d -s babylon-test-1
        tmux send-keys -t babylon-test-1 "export HOME=$TEST_DATA_DIR/instance1" C-m
        tmux send-keys -t babylon-test-1 "cd '$PROJECT_ROOT'" C-m
        tmux send-keys -t babylon-test-1 "echo '=== Instance 1 (Alice) ==='" C-m
        tmux send-keys -t babylon-test-1 "./bin/messenger" C-m
        PID1=$!
        log_success "Instance 1 launched in tmux session: babylon-test-1"
    else
        # Fallback: run in background
        export HOME="$TEST_DATA_DIR/instance1"
        cd "$PROJECT_ROOT"
        ./bin/messenger &
        PID1=$!
        log_success "Instance 1 launched (PID: $PID1)"
        log_warn "Running in background - use 'kill $PID1' to stop"
    fi
    
    sleep 2
    
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Instance 2 (Bob) - Data: $TEST_DATA_DIR/instance2"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    # Launch second instance
    if command -v gnome-terminal &> /dev/null; then
        gnome-terminal -- bash -c "
            export HOME=$TEST_DATA_DIR/instance2
            cd '$PROJECT_ROOT'
            echo '=== Instance 2 (Bob) ==='
            echo 'Data directory: $TEST_DATA_DIR/instance2'
            echo ''
            ./bin/messenger
            exec bash
        " &
        PID2=$!
        log_success "Instance 2 launched (PID: $PID2)"
    elif command -v x-terminal-emulator &> /dev/null; then
        x-terminal-emulator -e bash -c "
            export HOME=$TEST_DATA_DIR/instance2
            cd '$PROJECT_ROOT'
            echo '=== Instance 2 (Bob) ==='
            echo 'Data directory: $TEST_DATA_DIR/instance2'
            echo ''
            ./bin/messenger
            exec bash
        " &
        PID2=$!
        log_success "Instance 2 launched (PID: $PID2)"
    elif command -v tmux &> /dev/null; then
        tmux new-session -d -s babylon-test-2
        tmux send-keys -t babylon-test-2 "export HOME=$TEST_DATA_DIR/instance2" C-m
        tmux send-keys -t babylon-test-2 "cd '$PROJECT_ROOT'" C-m
        tmux send-keys -t babylon-test-2 "echo '=== Instance 2 (Bob) ==='" C-m
        tmux send-keys -t babylon-test-2 "./bin/messenger" C-m
        PID2=$!
        log_success "Instance 2 launched in tmux session: babylon-test-2"
    else
        export HOME="$TEST_DATA_DIR/instance2"
        cd "$PROJECT_ROOT"
        ./bin/messenger &
        PID2=$!
        log_success "Instance 2 launched (PID: $PID2)"
        log_warn "Running in background - use 'kill $PID2' to stop"
    fi
    
    echo ""
    log_success "Both instances launched!"
    echo ""
    echo "═══════════════════════════════════════════════════════════"
    echo "Testing Instructions:"
    echo "═══════════════════════════════════════════════════════════"
    echo "1. In Instance 1 (Alice), run: /myid"
    echo "2. In Instance 2 (Bob), run: /myid"
    echo "3. Exchange public keys between instances"
    echo "4. Add contacts: /add <public_key> <nickname>"
    echo "5. Start chat: /chat <contact_number>"
    echo "6. Send messages and verify encryption works"
    echo "═══════════════════════════════════════════════════════════"
    echo ""
    log_info "To stop instances: kill $PID1 $PID2"
    log_info "To clean test data: $0 clean"
}

launch_docker() {
    show_banner
    log_info "Launching two instances in Docker..."
    echo ""
    
    # Check if Docker is available
    if ! command -v docker &> /dev/null; then
        log_error "Docker not installed"
        exit 1
    fi
    
    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        log_error "docker-compose not installed"
        exit 1
    fi
    
    setup_test_data
    
    cd "$PROJECT_ROOT/test"
    
    # Stop any existing containers
    log_info "Stopping existing containers..."
    if command -v docker-compose &> /dev/null; then
        docker-compose down 2>/dev/null || true
    else
        docker compose down 2>/dev/null || true
    fi
    
    # Clean old test data
    rm -rf "$TEST_DATA_DIR/instance1" "$TEST_DATA_DIR/instance2"
    mkdir -p "$TEST_DATA_DIR/instance1" "$TEST_DATA_DIR/instance2"
    
    log_info "Starting containers..."
    if command -v docker-compose &> /dev/null; then
        docker-compose up -d
    else
        docker compose up -d
    fi
    
    echo ""
    log_success "Docker containers launched!"
    echo ""
    echo "Container details:"
    echo "  - Alice: babylon-alice (ports 4001, 4002)"
    echo "  - Bob:   babylon-bob   (ports 4011, 4012)"
    echo ""
    echo "To attach to Alice's instance:"
    echo "  docker exec -it babylon-alice /app/messenger"
    echo ""
    echo "To attach to Bob's instance:"
    echo "  docker exec -it babylon-bob /app/messenger"
    echo ""
    echo "To view logs:"
    echo "  docker logs -f babylon-alice"
    echo "  docker logs -f babylon-bob"
    echo ""
    echo "To stop containers:"
    echo "  make docker-stop"
    echo ""
    echo "To clean up:"
    echo "  make docker-clean"
}

show_help() {
    show_banner
    echo "Usage: $0 [mode]"
    echo ""
    echo "Modes:"
    echo "  local    Launch two instances locally (default)"
    echo "  docker   Launch two instances in Docker containers"
    echo "  clean    Clean test data and exit"
    echo "  help     Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 local     # Launch locally"
    echo "  $0 docker    # Launch in Docker"
    echo "  $0 clean     # Clean test data"
    echo ""
    echo "Manual Testing Steps:"
    echo "  1. Launch two instances"
    echo "  2. In each instance, run: /myid"
    echo "  3. Exchange public keys"
    echo "  4. Add contacts: /add <pubkey> <nickname>"
    echo "  5. Start chat: /chat <contact_number>"
    echo "  6. Send messages"
    echo ""
}

# Main
case "${1:-local}" in
    local)
        launch_local
        ;;
    docker)
        launch_docker
        ;;
    clean)
        show_banner
        clean_test_data
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        log_error "Unknown mode: $1"
        echo ""
        show_help
        exit 1
        ;;
esac
