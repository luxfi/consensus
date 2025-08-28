#!/bin/bash

# Comprehensive validation script for Lux Consensus
# Ensures 100% passing status across all components

set -e

echo "======================================"
echo "   Lux Consensus Validation Suite    "
echo "======================================"
echo ""

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

# Track overall status
ALL_PASS=true

# Function to check status
check_status() {
    local name="$1"
    local command="$2"
    
    printf "%-30s" "$name"
    if eval "$command" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ PASS${NC}"
        return 0
    else
        echo -e "${RED}✗ FAIL${NC}"
        ALL_PASS=false
        return 1
    fi
}

echo "1. GO TESTS"
echo "----------------------------------------"
check_status "Running all tests..." "go test ./..."
TOTAL_PACKAGES=$(go list ./... | wc -l)
TESTED_PACKAGES=$(go test ./... 2>&1 | grep -c "^ok" || true)
echo "   Packages tested: $TESTED_PACKAGES"
echo "   Total packages: $TOTAL_PACKAGES"
echo ""

echo "2. LINTING"
echo "----------------------------------------"
check_status "Running golangci-lint..." "golangci-lint run --timeout=10m 2>&1 | grep -v '^level' | grep -v '^$' | test \$(wc -l) -eq 0"
echo ""

echo "3. BENCHMARKS"
echo "----------------------------------------"
check_status "Config benchmarks..." "go test -bench=. -benchtime=100ms -run=XXX ./config"
check_status "Engine PQ benchmarks..." "go test -bench=. -benchtime=100ms -run=XXX ./engine/pq"
check_status "Protocol benchmarks..." "go test -bench=. -benchtime=100ms -run=XXX ./protocol/field ./protocol/quasar"
check_status "QZMQ benchmarks..." "go test -bench=. -benchtime=100ms -run=XXX ./qzmq"
echo ""

echo "4. CLI TOOLS"
echo "----------------------------------------"
check_status "bench tool..." "./bin/bench -help"
check_status "checker tool..." "./bin/checker -help"
check_status "sim tool..." "./bin/sim -help"
check_status "consensus tool..." "./bin/consensus -help"
check_status "params tool..." "./bin/params -help"
echo ""

echo "5. TOOL FUNCTIONALITY"
echo "----------------------------------------"
check_status "bench chain engine..." "./bin/bench -engine chain -blocks 10 -duration 1s"
check_status "checker all engines..." "./bin/checker -engine all"
check_status "sim network..." "./bin/sim -nodes 5 -rounds 2"
check_status "consensus info..." "./bin/consensus -action info"
check_status "params mainnet..." "./bin/params -network mainnet"
echo ""

echo "6. BUILD VERIFICATION"
echo "----------------------------------------"
check_status "Clean build..." "make clean && make build"
echo ""

echo "======================================"
echo "           VALIDATION SUMMARY         "
echo "======================================"
echo ""

if [ "$ALL_PASS" = true ]; then
    echo -e "${GREEN}✓ 100% PASSING${NC}"
    echo ""
    echo "All components validated successfully:"
    echo "  • All Go tests pass"
    echo "  • No linting errors"
    echo "  • All benchmarks run"
    echo "  • All CLI tools functional"
    echo "  • Build system operational"
    exit 0
else
    echo -e "${RED}✗ VALIDATION FAILED${NC}"
    echo ""
    echo "Some components failed validation."
    echo "Please review the output above."
    exit 1
fi