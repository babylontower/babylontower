#!/bin/bash
# Helper script to rebuild devcontainer and build the Gio UI

set -e

echo "=========================================="
echo "Babylon Tower UI - DevContainer Setup"
echo "=========================================="
echo ""

# Check if running in VS Code devcontainer
if [ "$CODESPACES" = "true" ] || [ -n "$DEVCONTAINER_ID" ]; then
    echo "✓ Running inside a devcontainer"
    echo ""
    echo "To build the UI, run:"
    echo "  make build-ui"
    echo ""
    echo "To run the UI (requires display):"
    echo "  make run-ui"
    exit 0
fi

# Check if running in workspace
if [ ! -f ".devcontainer/devcontainer.json" ]; then
    echo "✗ Error: Not in Babylon Tower workspace"
    echo "  Please run this script from the project root"
    exit 1
fi

echo "This script will help you rebuild the devcontainer with GUI libraries."
echo ""
echo "Steps:"
echo "1. Open VS Code"
echo "2. Press Ctrl+Shift+P (or Cmd+Shift+P on macOS)"
echo "3. Select 'Dev Containers: Rebuild Container'"
echo "4. Wait for the container to rebuild"
echo "5. Run 'make build-ui' to build the UI"
echo ""

# Check if VS Code is available
if command -v code &> /dev/null; then
    echo "VS Code found at: $(which code)"
    echo ""
    read -p "Open VS Code now? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        code .
        echo ""
        echo "✓ VS Code opened"
        echo "  Now rebuild the devcontainer from the command palette"
    fi
else
    echo "Note: VS Code 'code' command not found in PATH"
    echo "  Please open VS Code manually and rebuild the devcontainer"
fi

echo ""
echo "=========================================="
echo "After rebuild, run:"
echo "  make build-ui"
echo "  make run-ui"
echo "=========================================="
