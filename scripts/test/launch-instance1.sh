#!/bin/bash

# Babylon Tower - Launch Instance 1 (Alice)
# This script launches the first instance for manual two-instance testing
#
# Usage:
#   ./scripts/test/launch-instance1.sh [data_dir]
#
# Arguments:
#   data_dir - Optional custom data directory (default: ./test-data/instance1)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
DEFAULT_DATA_DIR="$PROJECT_ROOT/test-data/instance1"
DATA_DIR="${1:-$DEFAULT_DATA_DIR}"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

show_banner() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║        Babylon Tower - Instance 1 (Alice)                 ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
}

# Check if binary exists
BINARY="$PROJECT_ROOT/bin/messenger"
if [ ! -f "$BINARY" ]; then
    log_info "Binary not found. Building..."
    cd "$PROJECT_ROOT"
    make build
fi

# Setup data directory
log_info "Setting up data directory: $DATA_DIR"
mkdir -p "$DATA_DIR"

show_banner

log_info "Starting Instance 1 (Alice)..."
log_info "Data directory: $DATA_DIR"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Instance 1 (Alice)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Next steps:"
echo "  1. Wait for the instance to start and generate identity"
echo "  2. Run '/myid' to get your public key"
echo "  3. Share your public key with Instance 2 (Bob)"
echo "  4. Add Bob as contact: /add <bob_public_key> Bob"
echo "  5. Start chat: /chat 1"
echo ""
echo "To launch Instance 2 (Bob), run in another terminal:"
echo "  ./scripts/test/launch-instance2.sh"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Set HOME to use custom data directory
# Babylon Tower stores identity in ~/.babylontower
export HOME="$DATA_DIR"

# Run the messenger
cd "$PROJECT_ROOT"
./bin/messenger
