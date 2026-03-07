#!/bin/bash
#
# Cross-platform build script for Babylon UI
# Supports building for Linux, Windows, and macOS
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Directories
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_ROOT/bin/platform/ui"

# Version info
VERSION="${VERSION:-0.0.1}"
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go build flags
LDFLAGS="-X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME -X main.GitCommit=$GIT_COMMIT"

echo -e "${GREEN}Babylon UI Cross-Platform Build Script${NC}"
echo "================================================"
echo "Version: $VERSION"
echo "Build Time: $BUILD_TIME"
echo "Git Commit: $GIT_COMMIT"
echo ""

# Function to build for a specific platform
build_platform() {
    local os=$1
    local arch=$2
    local ext=""
    local cc=""
    local cgo=1
    
    if [ "$os" = "windows" ]; then
        ext=".exe"
        cc="x86_64-w64-mingw32-gcc"
        cgo=1
    elif [ "$os" = "darwin" ]; then
        cgo=0
    fi

    local output_name="${os}/${BINARY_UI_NAME:-babylon-ui}${ext}"
    local output_path="$BUILD_DIR/$output_name"

    echo -e "${YELLOW}Building for $os ($arch)...${NC}"

    # Create build directory
    mkdir -p "$BUILD_DIR/$os"
    
    # Set environment variables and build
    export GOOS="$os"
    export GOARCH="$arch"
    export CGO_ENABLED="$cgo"
    
    if [ -n "$cc" ]; then
        export CC="$cc"
    fi
    
    # Build
    if go build -buildvcs=false -ldflags "$LDFLAGS" -o "$output_path" "$PROJECT_ROOT/cmd/babylon-ui"; then
        echo -e "${GREEN}✓ Build successful: $output_path${NC}"
    else
        echo -e "${RED}✗ Build failed for $os ($arch)${NC}"
        return 1
    fi
    
    # Unset CC if it was set
    if [ -n "$cc" ]; then
        unset CC
    fi
}

# Parse command line arguments
PLATFORM="${1:-all}"

case "$PLATFORM" in
    linux)
        build_platform "linux" "amd64"
        ;;
    windows)
        build_platform "windows" "amd64"
        ;;
    darwin)
        build_platform "darwin" "amd64"
        ;;
    all)
        echo "Building for all platforms..."
        echo ""
        
        # Build for Linux (native)
        build_platform "linux" "amd64"
        echo ""
        
        # Build for Windows (cross-compile with mingw)
        if command -v x86_64-w64-mingw32-gcc &> /dev/null; then
            build_platform "windows" "amd64"
        else
            echo -e "${YELLOW}⚠ Skipping Windows build (mingw-w64 not installed)${NC}"
            echo "  Install with: sudo apt-get install mingw-w64"
        fi
        echo ""
        
        # Build for macOS (without CGO)
        echo -e "${YELLOW}Note: macOS GUI apps cannot be cross-compiled. Skipping macOS build.${NC}"
        echo "  For macOS, build natively on macOS with: make build-ui"
        # build_platform "darwin" "amd64"
        ;;
    *)
        echo -e "${RED}Unknown platform: $PLATFORM${NC}"
        echo ""
        echo "Usage: $0 [linux|windows|darwin|all]"
        echo ""
        echo "  linux   - Build for Linux (amd64)"
        echo "  windows - Build for Windows (amd64, requires mingw-w64)"
        echo "  darwin  - Build for macOS (amd64, limited CGO)"
        echo "  all     - Build for all platforms (default)"
        exit 1
        ;;
esac

echo ""
echo -e "${GREEN}Build complete!${NC}"
echo ""
echo "Binaries are located in: $BUILD_DIR"
echo ""
echo "Available binaries:"
ls -lh "$BUILD_DIR"/*/babylon-ui* 2>/dev/null || echo "  (no binaries found)"
echo ""
echo -e "${YELLOW}Note:${NC}"
echo "  - Linux binary: Run with $BUILD_DIR/linux/babylon-ui"
echo "  - Windows binary: Copy to Windows and run $BUILD_DIR/windows/babylon-ui.exe"
echo "  - macOS binary: Build natively on macOS with: make build-ui"
echo "    For best results, build natively on each target platform."
