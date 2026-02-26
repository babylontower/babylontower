#!/bin/bash

# Babylon Tower - Verify HOME Environment Variable Handling
# This script verifies that multiple instances use different peer keys

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
TEST_DIR="$PROJECT_ROOT/test-data/home-env-test"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
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

echo ""
echo "╔═══════════════════════════════════════════════════════════╗"
echo "║   Babylon Tower - HOME Environment Variable Test          ║"
echo "╚═══════════════════════════════════════════════════════════╝"
echo ""

# Clean up previous test
log_info "Cleaning up previous test data..."
rm -rf "$TEST_DIR"
mkdir -p "$TEST_DIR/instance1" "$TEST_DIR/instance2"

# Detect platform
PLATFORM="$(uname -s)"
BINARY=""

if [ "$PLATFORM" = "MINGW"* ] || [ "$PLATFORM" = "CYGWIN"* ] || [ "$PLATFORM" = "MSYS"* ]; then
    PLATFORM="windows"
    BINARY="$PROJECT_ROOT/bin/platform/windows/messenger.exe"
elif [ "$PLATFORM" = "Darwin" ]; then
    PLATFORM="darwin"
    BINARY="$PROJECT_ROOT/bin/platform/darwin/messenger"
else
    PLATFORM="linux"
    BINARY="$PROJECT_ROOT/bin/platform/linux/messenger"
fi

# Check if binary exists
if [ ! -f "$BINARY" ]; then
    log_info "Binary not found, building..."
    cd "$PROJECT_ROOT"
    make build
    BINARY="$PROJECT_ROOT/bin/messenger"
fi

log_info "Using binary: $BINARY"
log_info "Test directory: $TEST_DIR"
echo ""

# Test 1: Verify different HOME dirs produce different peer keys
log_info "Test 1: Creating identity in instance1..."
export HOME="$TEST_DIR/instance1"
PEER_ID_1=$("$BINARY" -data-dir "$TEST_DIR/instance1" 2>&1 | grep -oP 'peer_id.*' | head -1 || true)

# Wait a moment for file to be written
sleep 1

log_info "Test 2: Creating identity in instance2..."
export HOME="$TEST_DIR/instance2"
PEER_ID_2=$("$BINARY" -data-dir "$TEST_DIR/instance2" 2>&1 | grep -oP 'peer_id.*' | head -1 || true)

echo ""
log_info "Instance 1 peer ID: $PEER_ID_1"
log_info "Instance 2 peer ID: $PEER_ID_2"

# Check if peer IDs are different
if [ -n "$PEER_ID_1" ] && [ -n "$PEER_ID_2" ] && [ "$PEER_ID_1" != "$PEER_ID_2" ]; then
    log_success "Peer IDs are different - HOME environment variable is respected!"
    exit 0
else
    log_error "Peer IDs are the same or could not be determined"
    log_error "This indicates HOME environment variable is not being respected"
    exit 1
fi
