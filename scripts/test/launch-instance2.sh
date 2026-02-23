#!/bin/bash

# Babylon Tower - Launch Instance 2 (Bob)
# This script launches the second instance for manual two-instance testing
#
# Usage:
#   ./scripts/test/launch-instance2.sh [data_dir]
#
# Arguments:
#   data_dir - Optional custom data directory (default: ./test-data/instance2)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
DEFAULT_DATA_DIR="$PROJECT_ROOT/test-data/instance2"
DATA_DIR="${1:-$DEFAULT_DATA_DIR}"

# Detect current platform
detect_platform() {
    case "$(uname -s)" in
        Linux*)     echo "linux" ;;
        Darwin*)    echo "darwin" ;;
        MINGW*|CYGWIN*|MSYS*) echo "windows" ;;
        *)          echo "linux" ;;  # Default to linux
    esac
}

PLATFORM=$(detect_platform)

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
    echo "║        Babylon Tower - Instance 2 (Bob)                   ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
}

# Check if binary exists (check multiple locations)
# Priority: 1) platform-specific, 2) standard build location
BINARY=""

# First check platform-specific binary
PLATFORM_BINARY="$PROJECT_ROOT/bin/platform/$PLATFORM/messenger"
if [ "$PLATFORM" = "windows" ]; then
    PLATFORM_BINARY="$PROJECT_ROOT/bin/platform/$PLATFORM/messenger.exe"
fi

# Second check standard build location
STANDARD_BINARY="$PROJECT_ROOT/bin/messenger"
if [ "$PLATFORM" = "windows" ]; then
    STANDARD_BINARY="$PROJECT_ROOT/bin/messenger.exe"
fi

# Use platform binary if exists, otherwise try standard location
if [ -f "$PLATFORM_BINARY" ]; then
    BINARY="$PLATFORM_BINARY"
    log_info "Using platform binary: $BINARY"
elif [ -f "$STANDARD_BINARY" ]; then
    BINARY="$STANDARD_BINARY"
    log_info "Using standard binary: $BINARY"
else
    log_info "Binary not found. Building for current platform ($PLATFORM)..."
    cd "$PROJECT_ROOT"
    case "$PLATFORM" in
        linux)   make build-linux ;;
        darwin)  make build-darwin ;;
        windows) make build-windows ;;
        *)       make build ;;
    esac
    
    # Update binary path after build (prefer platform binary)
    if [ -f "$PLATFORM_BINARY" ]; then
        BINARY="$PLATFORM_BINARY"
    else
        BINARY="$STANDARD_BINARY"
    fi
fi

# Setup data directory
log_info "Setting up data directory: $DATA_DIR"
log_info "Using binary: $BINARY"
log_info "Platform: $PLATFORM"
mkdir -p "$DATA_DIR"

show_banner

log_info "Starting Instance 2 (Bob)..."
log_info "Data directory: $DATA_DIR"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Instance 2 (Bob)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Next steps:"
echo "  1. Wait for the instance to start and generate identity"
echo "  2. Run '/myid' to get your public key"
echo "  3. Share your public key with Instance 1 (Alice)"
echo "  4. Add Alice as contact: /add <alice_public_key> Alice"
echo "  5. Start chat: /chat 1"
echo ""
echo "To launch Instance 1 (Alice), run in another terminal:"
echo "  ./scripts/test/launch-instance1.sh"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Set HOME to use custom data directory
# Babylon Tower stores identity in ~/.babylontower
export HOME="$DATA_DIR"

# Run the messenger
cd "$PROJECT_ROOT"
"$BINARY"
