#!/bin/bash

# Babylon Tower - Clean Test Data
# This script removes all test data from previous test runs
#
# Usage:
#   ./scripts/test/clean-test-data.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
TEST_DATA_DIR="$PROJECT_ROOT/test-data"

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

show_banner() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║        Babylon Tower - Clean Test Data                    ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
}

show_banner()

if [ ! -d "$TEST_DATA_DIR" ]; then
    log_info "No test data directory found at: $TEST_DATA_DIR"
    log_success "Nothing to clean"
    exit 0
fi

log_warn "This will remove all test data from:"
echo "  $TEST_DATA_DIR"
echo ""

# Check if running interactively
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
echo ""
echo "You can now start fresh with:"
echo "  ./scripts/test/launch-instance1.sh"
echo "  ./scripts/test/launch-instance2.sh"
