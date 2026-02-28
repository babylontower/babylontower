#!/bin/bash

# Babylon Tower - Connection Verifier
# This script verifies connections between nodes in a multi-node test
#
# Usage:
#   ./scripts/test/verify-connections.sh [node_count]
#
# Arguments:
#   node_count - Number of nodes to check (default: 5)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
TEST_DATA_DIR="$PROJECT_ROOT/test-data"
DEFAULT_NODE_COUNT=5

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[!]${NC} $1"
}

show_banner() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║     Babylon Tower - Connection Verifier                   ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
}

show_banner

NODE_COUNT="${1:-$DEFAULT_NODE_COUNT}"

if [ ! -d "$TEST_DATA_DIR" ]; then
    log_error "Test data directory not found: $TEST_DATA_DIR"
    log_info "Launch nodes first with: ./scripts/test/launch-multi-node.sh"
    exit 1
fi

log_info "Verifying connections for $NODE_COUNT nodes..."
echo ""

# Connection matrix
declare -A connection_matrix
total_connections=0
total_dht_peers=0
nodes_with_connections=0

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Node Status:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

for i in $(seq 1 $NODE_COUNT); do
    DATA_DIR="$TEST_DATA_DIR/node-$i"
    
    if [ ! -d "$DATA_DIR" ]; then
        log_error "Node $i data directory not found"
        continue
    fi
    
    # Check if node is running
    if [ -f "$DATA_DIR/pid" ]; then
        pid=$(cat "$DATA_DIR/pid")
        if ! ps -p $pid > /dev/null 2>&1; then
            log_error "Node $i: Not running (PID: $pid)"
            continue
        fi
    else
        log_warn "Node $i: PID file not found"
    fi
    
    # Read output log to extract connection info
    OUTPUT_LOG="$DATA_DIR/output.log"
    
    if [ ! -f "$OUTPUT_LOG" ]; then
        log_warn "Node $i: Output log not found"
        continue
    fi
    
    # Extract peer count from output (look for "Connected peers:" pattern)
    peer_count=$(grep -oP "Connected peers: \K\d+" "$OUTPUT_LOG" 2>/dev/null | tail -1 || echo "0")
    
    # Extract DHT routing table size
    dht_peers=$(grep -oP "Routing Table Size: \K\d+" "$OUTPUT_LOG" 2>/dev/null | tail -1 || echo "0")
    
    # Extract bootstrap status
    if grep -q "DHT bootstrap completed" "$OUTPUT_LOG" 2>/dev/null; then
        bootstrap_status="✓"
    elif grep -q "Bootstrap.*failed\|Not connected to any peers" "$OUTPUT_LOG" 2>/dev/null; then
        bootstrap_status="✗"
    else
        bootstrap_status="?"
    fi
    
    # Display status
    if [ "$peer_count" -gt 0 ] 2>/dev/null; then
        log_success "Node $i: Connected to $peer_count peers, DHT: $dht_peers peers $bootstrap_status"
        ((total_connections += peer_count)) || true
        ((total_dht_peers += dht_peers)) || true
        ((nodes_with_connections++)) || true
    else
        log_warn "Node $i: No connections yet, DHT: $dht_peers peers $bootstrap_status"
    fi
done

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Summary:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

avg_connections=0
avg_dht_peers=0

if [ $nodes_with_connections -gt 0 ]; then
    avg_connections=$((total_connections / nodes_with_connections))
    avg_dht_peers=$((total_dht_peers / nodes_with_connections))
fi

echo "  Nodes checked:        $NODE_COUNT"
echo "  Nodes connected:      $nodes_with_connections"
echo "  Total connections:    $total_connections"
echo "  Total DHT peers:      $total_dht_peers"
echo "  Avg connections/node: $avg_connections"
echo "  Avg DHT peers/node:   $avg_dht_peers"
echo ""

# Phase 6 acceptance criteria
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 6 Acceptance Criteria:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Check criteria
criteria_passed=0
criteria_total=4

if [ $nodes_with_connections -eq $NODE_COUNT ]; then
    log_success "All nodes running: PASS"
    ((criteria_passed++)) || true
else
    log_error "All nodes running: FAIL ($nodes_with_connections/$NODE_COUNT)"
fi

if [ $avg_connections -ge 2 ]; then
    log_success "Avg connections ≥2: PASS ($avg_connections)"
    ((criteria_passed++)) || true
else
    log_error "Avg connections ≥2: FAIL ($avg_connections)"
fi

if [ $avg_dht_peers -ge 3 ]; then
    log_success "Avg DHT peers ≥3: PASS ($avg_dht_peers)"
    ((criteria_passed++)) || true
else
    log_error "Avg DHT peers ≥3: FAIL ($avg_dht_peers)"
fi

if [ $criteria_passed -eq $criteria_total ]; then
    log_success "Network formation: PASS"
    ((criteria_passed++)) || true
else
    log_warn "Network formation: PARTIAL ($criteria_passed/$criteria_total criteria)"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Overall: $criteria_passed/$criteria_total criteria passed"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if [ $criteria_passed -eq $criteria_total ]; then
    log_success "Multi-node network test PASSED!"
    exit 0
else
    log_warn "Multi-node network test NEEDS ATTENTION"
    log_info "Wait longer for network formation or check node logs:"
    echo "  tail -f $TEST_DATA_DIR/node-1/output.log"
    exit 1
fi
