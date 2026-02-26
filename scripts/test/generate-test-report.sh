#!/bin/bash

# Babylon Tower - Test Report Generator
# This script generates a markdown test report from test execution
#
# Usage:
#   ./scripts/test/generate-test-report.sh [output_file]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
OUTPUT_FILE="${1:-$PROJECT_ROOT/test-report.md}"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

show_banner() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║        Babylon Tower - Test Report Generator              ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
}

# Get current date
DATE=$(date '+%Y-%m-%d')
VERSION=$(grep '^VERSION=' Makefile 2>/dev/null | cut -d'=' -f2 || echo "unknown")

# Run unit tests and capture output
log_info "Running unit tests..."
cd "$PROJECT_ROOT"

UNIT_TEST_OUTPUT=$(make test 2>&1 || true)
UNIT_TEST_COVERAGE=$(make test-coverage 2>&1 | grep -E "^(ok|FAIL)" || true)

# Count test results
UNIT_TESTS_PASSED=$(echo "$UNIT_TEST_OUTPUT" | grep -c "--- PASS:" || echo "0")
UNIT_TESTS_FAILED=$(echo "$UNIT_TEST_OUTPUT" | grep -c "--- FAIL:" || echo "0")

# Run integration tests
log_info "Running integration tests..."
INTEGRATION_OUTPUT=$(go test -tags=integration ./pkg/ratchet/... ./pkg/multidevice/... ./pkg/groups/... ./pkg/mailbox/... ./pkg/rtc/... ./pkg/ipfsnode/... -timeout 10m 2>&1 || true)

INTEGRATION_PASSED=$(echo "$INTEGRATION_OUTPUT" | grep -c "PASS" || echo "0")
INTEGRATION_FAILED=$(echo "$INTEGRATION_OUTPUT" | grep -c "FAIL" || echo "0")

# Get coverage stats
log_info "Calculating coverage..."
COVERAGE_OUTPUT=$(go test -coverprofile=coverage.out ./... 2>&1 || true)
TOTAL_COVERAGE=$(go tool cover -func=coverage.out 2>/dev/null | grep "total:" | awk '{print $3}' || echo "N/A")

# Generate report
log_info "Generating test report..."

cat > "$OUTPUT_FILE" << EOF
# Babylon Tower - Test Execution Report

**Date:** $DATE  
**Tester:** Automated Test Runner  
**Version:** v$VERSION  
**Commit:** $(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

---

## Executive Summary

| Metric | Result |
|--------|--------|
| Overall Status | $(if [ "$UNIT_TESTS_FAILED" -eq 0 ] && [ "$INTEGRATION_FAILED" -eq 0 ]; then echo "✅ PASS"; else echo "❌ FAIL"; fi) |
| Unit Tests | $UNIT_TESTS_PASSED passed, $UNIT_TESTS_FAILED failed |
| Integration Tests | $INTEGRATION_PASSED passed, $INTEGRATION_FAILED failed |
| Total Coverage | $TOTAL_COVERAGE |

---

## Unit Test Results

\`\`\`
$UNIT_TEST_OUTPUT
\`\`\`

### Coverage by Package

\`\`\`
$UNIT_TEST_COVERAGE
\`\`\`

---

## Integration Test Results

\`\`\`
$INTEGRATION_OUTPUT
\`\`\`

---

## Coverage Details

### Overall Coverage

Total coverage: **$TOTAL_COVERAGE**

### Coverage by Module

\`\`\`bash
$(go tool cover -func=coverage.out 2>/dev/null | head -30 || echo "Coverage details not available")
\`\`\`

---

## Performance Benchmarks

\`\`\`bash
$(cd "$PROJECT_ROOT" && go test -bench=. ./pkg/ratchet/... -benchmem 2>/dev/null | head -20 || echo "Benchmarks not available")
\`\`\`

---

## Issues Found

$(if [ "$UNIT_TESTS_FAILED" -gt 0 ] || [ "$INTEGRATION_FAILED" -gt 0 ]; then
    echo "### Failed Tests"
    echo ""
    echo "$UNIT_TEST_OUTPUT" | grep -A 5 "FAIL:" || echo "No failed tests details available"
    echo ""
    echo "$INTEGRATION_OUTPUT" | grep -A 5 "FAIL" || echo "No integration test failures details available"
else
    echo "No issues found. All tests passed."
fi)

---

## Known Limitations

1. **Local storage encryption:** Not encrypted in PoC (Phase 19+)
2. **NAT traversal:** Limited; symmetric NATs may have connectivity issues
3. **Metadata privacy:** Network observers can see communication patterns
4. **Contact verification:** No safety number comparison yet

See [specs/limitations-roadmap.md](specs/limitations-roadmap.md) for comprehensive list.

---

## Environment

### System Information

- **OS:** $(uname -s) $(uname -r)
- **Go Version:** $(go version 2>/dev/null || echo "unknown")
- **CPU:** $(nproc 2>/dev/null || echo "unknown") cores
- **Memory:** $(free -h 2>/dev/null | grep Mem | awk '{print $2}' || echo "unknown")

### Test Configuration

- **Unit Tests:** \`make test\`
- **Integration Tests:** \`go test -tags=integration ./...\`
- **Coverage:** \`go test -coverprofile=coverage.out ./...\`

---

## Recommendation

$(if [ "$UNIT_TESTS_FAILED" -eq 0 ] && [ "$INTEGRATION_FAILED" -eq 0 ]; then
    echo "✅ **Ready for release** - All tests passed"
elif [ "$UNIT_TESTS_FAILED" -lt 3 ] && [ "$INTEGRATION_FAILED" -lt 2 ]; then
    echo "⚠️ **Needs fixes** - Minor test failures detected"
else
    echo "❌ **Needs more testing** - Significant test failures detected"
fi)

---

## Appendix

### Test Commands Used

\`\`\`bash
# Unit tests
make test

# Integration tests
go test -tags=integration ./pkg/ratchet/... ./pkg/multidevice/... ./pkg/groups/... ./pkg/mailbox/... ./pkg/rtc/... ./pkg/ipfsnode/... -timeout 10m

# Coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Benchmarks
go test -bench=. ./pkg/ratchet/... -benchmem
\`\`\`

### Files Generated

- Test report: $OUTPUT_FILE
- Coverage profile: coverage.out
- Coverage HTML: coverage.html

---

*Report generated automatically by scripts/test/generate-test-report.sh*
EOF

# Generate coverage HTML
go tool cover -html=coverage.out -o coverage.html 2>/dev/null || true

echo ""
log_success "Test report generated: $OUTPUT_FILE"
log_success "Coverage HTML generated: coverage.html"
echo ""
log_info "Summary:"
echo "  Unit Tests: $UNIT_TESTS_PASSED passed, $UNIT_TESTS_FAILED failed"
echo "  Integration Tests: $INTEGRATION_PASSED passed, $INTEGRATION_FAILED failed"
echo "  Total Coverage: $TOTAL_COVERAGE"
echo ""
