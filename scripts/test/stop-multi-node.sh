#!/bin/bash

# Babylon Tower - Stop Multi-Node Test
# This script stops all nodes launched by launch-multi-node.sh
#
# Usage:
#   ./scripts/test/stop-multi-node.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
TEST_DATA_DIR="$PROJECT_ROOT/test-data"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

show_banner() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║     Babylon Tower - Stop Multi-Node Test                  ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
}

show_banner

if [ ! -d "$TEST_DATA_DIR" ]; then
    log_info "No test data directory found"
    exit 0
fi

# Find and stop all nodes
stopped=0
for pid_file in "$TEST_DATA_DIR"/node-*/pid; do
    if [ -f "$pid_file" ]; then
        pid=$(cat "$pid_file")
        node_dir=$(dirname "$pid_file")
        node_name=$(basename "$node_dir")
        
        if ps -p $pid > /dev/null 2>&1; then
            log_info "Stopping $node_name (PID: $pid)..."
            kill $pid 2>/dev/null || true
            ((stopped++)) || true
        else
            log_info "$node_name not running"
        fi
        
        rm -f "$pid_file"
    fi
done

# Also try to kill any remaining messenger processes in test-data
pkill -f "HOME=$TEST_DATA_DIR.*messenger" 2>/dev/null || true

if [ $stopped -gt 0 ]; then
    log_success "Stopped $stopped node(s)"
else
    log_info "No running nodes found"
fi

echo ""
log_info "To clean test data, run:"
echo "  ./scripts/test/clean-test-data.sh"
echo ""
