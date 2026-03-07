#!/bin/bash
# Test script for running two Babylon Tower nodes
# This script starts two nodes in separate directories for testing

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MESSENGER="${SCRIPT_DIR}/bin/messenger"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}╔════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║     Babylon Tower - Two Node Test Script              ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════════════════════╝${NC}"
echo ""

# Check if messenger binary exists
if [ ! -f "$MESSENGER" ]; then
    echo -e "${RED}Error: Messenger binary not found at $MESSENGER${NC}"
    echo "Please run 'make build' first"
    exit 1
fi

# Create test directories
NODE1_DIR="$HOME/.babylontower/test_node1"
NODE2_DIR="$HOME/.babylontower/test_node2"

echo -e "${YELLOW}Setting up test directories...${NC}"
mkdir -p "$NODE1_DIR" "$NODE2_DIR"

# Clean previous test data (optional)
if [ "$1" == "--clean" ]; then
    echo -e "${YELLOW}Cleaning previous test data...${NC}"
    rm -rf "$NODE1_DIR"/* "$NODE2_DIR"/*
fi

echo ""
echo -e "${GREEN}Test directories created:${NC}"
echo "  Node 1: $NODE1_DIR"
echo "  Node 2: $NODE2_DIR"
echo ""

# Function to cleanup on exit
cleanup() {
    echo ""
    echo -e "${YELLOW}Stopping nodes...${NC}"
    if [ ! -z "$PID1" ]; then
        kill $PID1 2>/dev/null || true
        echo "  Node 1 stopped"
    fi
    if [ ! -z "$PID2" ]; then
        kill $PID2 2>/dev/null || true
        echo "  Node 2 stopped"
    fi
    echo -e "${GREEN}Cleanup complete${NC}"
}

trap cleanup EXIT

# Start Node 1
echo -e "${YELLOW}Starting Node 1...${NC}"
"$MESSENGER" -data-dir "$NODE1_DIR" -log-level info &
PID1=$!
echo "  Node 1 PID: $PID1"

# Wait for Node 1 to start
sleep 3

# Start Node 2
echo -e "${YELLOW}Starting Node 2...${NC}"
"$MESSENGER" -data-dir "$NODE2_DIR" -log-level info &
PID2=$!
echo "  Node 2 PID: $PID2"

echo ""
echo -e "${GREEN}╔════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║  Both nodes are now running!                          ║${NC}"
echo -e "${GREEN}╠════════════════════════════════════════════════════════╣${NC}"
echo -e "${GREEN}║  Watch the logs above for:                            ║${NC}"
echo -e "${GREEN}║  ✓ mDNS peer discovery                                ║${NC}"
echo -e "${GREEN}║  ✓ Peer connection established                        ║${NC}"
echo -e "${GREEN}║  ✓ Bootstrap completion                               ║${NC}"
echo -e "${GREEN}╠════════════════════════════════════════════════════════╣${NC}"
echo -e "${GREEN}║  To test messaging:                                   ║${NC}"
echo -e "${GREEN}║  1. Get Node 1's public key from logs                 ║${NC}"
echo -e "${GREEN}║  2. In Node 2: /contact add <pubkey>                  ║${NC}"
echo -e "${GREEN}║  3. In Node 1: /contact add <pubkey>                  ║${NC}"
echo -e "${GREEN}║  4. Send messages with /send <pubkey> <message>       ║${NC}"
echo -e "${GREEN}╠════════════════════════════════════════════════════════╣${NC}"
echo -e "${GREEN}║  Press Ctrl+C to stop both nodes                      ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════════════════════╝${NC}"
echo ""

# Wait for both processes
wait $PID1 $PID2
