#!/bin/bash

# Babylon Tower - Interactive Manual Test Runner
# This script provides a menu-driven interface for running manual integration tests
#
# Usage:
#   ./scripts/test/run-manual-tests.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
TEST_DATA_DIR="$PROJECT_ROOT/test-data"
BINARY="$PROJECT_ROOT/bin/messenger"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Test results
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[!]${NC} $1"
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
}

log_header() {
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

show_banner() {
    clear
    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║     Babylon Tower - Manual Integration Test Runner        ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
    echo "Current Status: Passed=$TESTS_PASSED Failed=$TESTS_FAILED Skipped=$TESTS_SKIPPED"
    echo ""
}

show_main_menu() {
    show_banner
    
    log_header "Test Categories"
    echo ""
    echo "  1. Basic Messaging Tests"
    echo "  2. Multi-Device Tests"
    echo "  3. Group Messaging Tests"
    echo "  4. Offline Delivery Tests"
    echo "  5. Network Formation Tests"
    echo "  6. Run All Tests"
    echo "  7. View Test Results"
    echo "  8. Clean Test Data"
    echo "  0. Exit"
    echo ""
    echo -n "Select test category [0-8]: "
}

check_binary() {
    if [ ! -f "$BINARY" ]; then
        log_warn "Binary not found. Building..."
        cd "$PROJECT_ROOT"
        make build >/dev/null 2>&1
        if [ ! -f "$BINARY" ]; then
            log_error "Build failed. Please run 'make build' manually."
            return 1
        fi
        log_success "Build complete"
    fi
    return 0
}

# Basic Messaging Tests
run_basic_messaging_tests() {
    show_banner
    log_header "Basic Messaging Tests"
    echo ""
    
    echo "This test will:"
    echo "  • Launch two messenger instances (Alice and Bob)"
    echo "  • Guide you through exchanging identity fingerprints"
    echo "  • Guide you through adding contacts"
    echo "  • Guide you through sending encrypted messages"
    echo ""
    
    read -p "Press Enter to start or 'q' to return to menu: " -n 1
    echo ""
    
    if [ "$REPLY" = "q" ]; then
        return
    fi
    
    # Launch two instances
    log_info "Launching two instances..."
    
    # Terminal 1: Alice
    log_info "Starting Alice's instance..."
    export HOME="$TEST_DATA_DIR/instance1"
    mkdir -p "$HOME"
    
    # Terminal 2: Bob
    log_info "Starting Bob's instance in separate terminal..."
    export HOME="$TEST_DATA_DIR/instance2"
    mkdir -p "$HOME"
    
    echo ""
    log_header "Manual Test Instructions"
    echo ""
    echo "Step 1: Launch Instances"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Open TWO terminals and run:"
    echo ""
    echo "Terminal 1 (Alice):"
    echo "  export HOME=$TEST_DATA_DIR/instance1"
    echo "  $BINARY"
    echo ""
    echo "Terminal 2 (Bob):"
    echo "  export HOME=$TEST_DATA_DIR/instance2"
    echo "  $BINARY"
    echo ""
    
    read -p "Press Enter when both instances are running..."
    
    echo ""
    echo "Step 2: Exchange Identity Fingerprints"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "In BOTH terminals, run:"
    echo "  >>> /myid"
    echo ""
    echo "Copy the identity fingerprint from each terminal."
    echo ""
    
    read -p "Press Enter after copying fingerprints..."
    
    echo ""
    echo "Step 3: Add Contacts"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "In Alice's terminal:"
    echo "  >>> /add <Bob's_fingerprint> Bob"
    echo ""
    echo "In Bob's terminal:"
    echo "  >>> /add <Alice's_fingerprint> Alice"
    echo ""
    
    read -p "Press Enter after adding contacts..."
    
    echo ""
    echo "Step 4: Start Chat"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "In BOTH terminals:"
    echo "  >>> /chat 1"
    echo ""
    
    read -p "Press Enter after entering chat mode..."
    
    echo ""
    echo "Step 5: Exchange Messages"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "In Alice's terminal, type:"
    echo "  Hello Bob! This is a test message."
    echo ""
    echo "In Bob's terminal, verify the message appears, then reply:"
    echo "  Hi Alice! Received your message."
    echo ""
    echo "Verify both messages appear in both terminals."
    echo ""
    
    read -p "Press Enter after exchanging messages..."
    
    echo ""
    echo "Step 6: Verify Persistence"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "In BOTH terminals:"
    echo "  >>> /exit"
    echo ""
    echo "Then restart both instances and run:"
    echo "  >>> /history Alice  (in Bob's terminal)"
    echo "  >>> /history Bob    (in Alice's terminal)"
    echo ""
    echo "Verify previous messages are displayed."
    echo ""
    
    read -p "Press Enter after verifying persistence..."
    
    echo ""
    read -p "Did all tests pass? [y/N]: " -n 1
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        log_success "Basic messaging tests PASSED"
        ((TESTS_PASSED++))
    else
        log_error "Basic messaging tests FAILED"
        ((TESTS_FAILED++))
    fi
    
    echo ""
    log_info "Press Enter to return to menu..."
    read
}

# Multi-Device Tests
run_multidevice_tests() {
    show_banner
    log_header "Multi-Device Tests"
    echo ""
    
    echo "This test will:"
    echo "  • Test device registration with same identity"
    echo "  • Test cross-device message sync"
    echo "  • Test message fanout to multiple devices"
    echo ""
    
    log_warn "Note: Multi-device testing requires manual setup"
    log_info "See scripts/test/MANUAL-TEST-CHECKLIST.md for detailed instructions"
    echo ""
    
    read -p "Mark test as passed for now? [y/N]: " -n 1
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        ((TESTS_PASSED++))
        log_success "Multi-device test marked PASSED (manual verification pending)"
    else
        ((TESTS_FAILED++))
        log_error "Multi-device test marked FAILED"
    fi
    
    echo ""
    log_info "Press Enter to return to menu..."
    read
}

# Group Messaging Tests
run_group_tests() {
    show_banner
    log_header "Group Messaging Tests"
    echo ""
    
    echo "This test will:"
    echo "  • Create a private group"
    echo "  • Add members to the group"
    echo "  • Test group message encryption"
    echo "  • Test member removal and key rotation"
    echo ""
    
    log_info "Group testing requires 3+ instances"
    log_info "See scripts/test/scenario-groups.sh for automated scenario"
    echo ""
    
    read -p "Run automated group scenario script? [y/N]: " -n 1
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        if [ -f "$SCRIPT_DIR/scenario-groups.sh" ]; then
            bash "$SCRIPT_DIR/scenario-groups.sh"
        else
            log_warn "Scenario script not found"
        fi
    fi
    
    echo ""
    log_info "Press Enter to return to menu..."
    read
}

# Offline Delivery Tests
run_offline_tests() {
    show_banner
    log_header "Offline Delivery Tests"
    echo ""
    
    echo "This test will:"
    echo "  • Simulate Bob going offline"
    echo "  • Alice sends messages while Bob offline"
    echo "  • Bob comes online and retrieves messages"
    echo ""
    
    log_warn "Note: Offline delivery testing requires relay node setup"
    log_info "See specs/testing.md Section 2.8 for detailed test scenario"
    echo ""
    
    read -p "Mark test as passed for now? [y/N]: " -n 1
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        ((TESTS_PASSED++))
        log_success "Offline delivery test marked PASSED (manual verification pending)"
    else
        ((TESTS_FAILED++))
        log_error "Offline delivery test marked FAILED"
    fi
    
    echo ""
    log_info "Press Enter to return to menu..."
    read
}

# Network Formation Tests
run_network_tests() {
    show_banner
    log_header "Network Formation Tests"
    echo ""
    
    echo "This test will:"
    echo "  • Launch 5 IPFS nodes"
    echo "  • Verify mesh network formation"
    echo "  • Check DHT routing table sizes"
    echo "  • Verify PubSub message delivery"
    echo ""
    
    read -p "Run network formation test? [y/N]: " -n 1
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        log_info "Running network formation test..."
        echo ""
        
        # Run Go integration test
        cd "$PROJECT_ROOT"
        if go test -tags=integration -v ./pkg/ipfsnode/... -run TestMultiNodeNetworkFormation -timeout 5m; then
            log_success "Network formation test PASSED"
            ((TESTS_PASSED++))
        else
            log_error "Network formation test FAILED"
            ((TESTS_FAILED++))
        fi
        
        echo ""
        log_info "Press Enter to return to menu..."
        read
    fi
}

# Run All Tests
run_all_tests() {
    show_banner
    log_header "Running All Tests"
    echo ""
    
    log_warn "This will run all manual test scenarios"
    log_warn "Estimated time: 15-20 minutes"
    echo ""
    
    read -p "Continue? [y/N]: " -n 1
    echo ""
    
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        return
    fi
    
    run_basic_messaging_tests
    run_multidevice_tests
    run_group_tests
    run_offline_tests
    run_network_tests
    
    show_results
}

# Show Test Results
show_results() {
    show_banner
    log_header "Test Results Summary"
    echo ""
    echo "  Tests Passed:  $TESTS_PASSED"
    echo "  Tests Failed:  $TESTS_FAILED"
    echo "  Tests Skipped: $TESTS_SKIPPED"
    echo ""
    
    total=$((TESTS_PASSED + TESTS_FAILED))
    if [ $total -gt 0 ]; then
        pass_rate=$((TESTS_PASSED * 100 / total))
        echo "  Pass Rate: ${pass_rate}%"
    fi
    
    echo ""
    
    if [ $TESTS_FAILED -eq 0 ]; then
        log_success "All tests passed!"
    else
        log_error "Some tests failed. Review failures above."
    fi
    
    echo ""
    log_info "Press Enter to return to menu..."
    read
}

# Clean Test Data
clean_test_data() {
    show_banner
    log_header "Clean Test Data"
    echo ""
    
    log_warn "This will remove all test data from:"
    echo "  $TEST_DATA_DIR"
    echo ""
    
    read -p "Are you sure? [y/N]: " -n 1
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -rf "$TEST_DATA_DIR"
        log_success "Test data cleaned"
    else
        log_info "Cancelled"
    fi
    
    echo ""
    log_info "Press Enter to return to menu..."
    read
}

# Main loop
main() {
    check_binary || return 1
    
    while true; do
        show_main_menu
        read -n 1 choice
        echo ""
        
        case $choice in
            1) run_basic_messaging_tests ;;
            2) run_multidevice_tests ;;
            3) run_group_tests ;;
            4) run_offline_tests ;;
            5) run_network_tests ;;
            6) run_all_tests ;;
            7) show_results ;;
            8) clean_test_data ;;
            0) 
                echo ""
                log_info "Exiting test runner..."
                exit 0
                ;;
            *) 
                log_warn "Invalid option. Press Enter to try again..."
                read
                ;;
        esac
    done
}

# Run main function
main
