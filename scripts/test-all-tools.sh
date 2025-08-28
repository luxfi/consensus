#!/bin/bash

# Test all CLI tools comprehensively

set -e

echo "==========================================="
echo "     Lux Consensus Tools Test Suite"
echo "==========================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Test counter
TESTS_PASSED=0
TESTS_FAILED=0

# Test function
run_test() {
    local test_name="$1"
    local command="$2"
    
    echo -n "Testing $test_name... "
    if $command > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗${NC}"
        echo "  Failed command: $command"
        ((TESTS_FAILED++))
    fi
}

echo "1. Testing bench tool"
echo "------------------------"
run_test "bench help" "./bin/bench -help"
run_test "bench chain engine" "./bin/bench -engine chain -blocks 10 -duration 1s"
run_test "bench dag engine" "./bin/bench -engine dag -blocks 10 -duration 1s"
run_test "bench pq engine" "./bin/bench -engine pq -blocks 10 -duration 1s"
run_test "bench all engines" "./bin/bench -engine all -blocks 5 -duration 1s"
run_test "bench with parallel" "./bin/bench -engine chain -parallel 2 -blocks 10"
run_test "bench mainnet config" "./bin/bench -network mainnet -blocks 5"
run_test "bench testnet config" "./bin/bench -network testnet -blocks 5"
run_test "bench local config" "./bin/bench -network local -blocks 5"
echo ""

echo "2. Testing checker tool"
echo "------------------------"
run_test "checker help" "./bin/checker -help"
run_test "checker all engines" "./bin/checker -engine all"
run_test "checker chain engine" "./bin/checker -engine chain"
run_test "checker dag engine" "./bin/checker -engine dag"
run_test "checker pq engine" "./bin/checker -engine pq"
run_test "checker with timeout" "./bin/checker -timeout 10s"
run_test "checker verbose" "./bin/checker -verbose"
echo ""

echo "3. Testing sim tool"
echo "------------------------"
run_test "sim help" "./bin/sim -help"
run_test "sim default" "./bin/sim -nodes 10 -rounds 2"
run_test "sim local network" "./bin/sim -nodes 5 -rounds 2 -network local"
run_test "sim testnet" "./bin/sim -nodes 11 -rounds 2 -network testnet"
run_test "sim mainnet" "./bin/sim -nodes 21 -rounds 2 -network mainnet"
run_test "sim with failure" "./bin/sim -nodes 10 -rounds 2 -failure 0.3"
run_test "sim with latency" "./bin/sim -nodes 10 -rounds 2 -latency 100ms"
run_test "sim large scale" "./bin/sim -nodes 100 -rounds 1"
run_test "sim verbose" "./bin/sim -nodes 5 -rounds 1 -verbose"
echo ""

echo "4. Testing consensus tool"
echo "------------------------"
run_test "consensus help" "./bin/consensus -help"
run_test "consensus info chain" "./bin/consensus -engine chain -action info"
run_test "consensus info dag" "./bin/consensus -engine dag -action info"
run_test "consensus info pq" "./bin/consensus -engine pq -action info"
run_test "consensus test chain" "./bin/consensus -engine chain -action test"
run_test "consensus test dag" "./bin/consensus -engine dag -action test"
run_test "consensus test pq" "./bin/consensus -engine pq -action test"
run_test "consensus health chain" "./bin/consensus -engine chain -action health"
run_test "consensus health dag" "./bin/consensus -engine dag -action health"
run_test "consensus health pq" "./bin/consensus -engine pq -action health"
run_test "consensus mainnet" "./bin/consensus -network mainnet -action info"
run_test "consensus testnet" "./bin/consensus -network testnet -action info"
run_test "consensus local" "./bin/consensus -network local -action info"
echo ""

echo "5. Testing params tool"
echo "------------------------"
run_test "params help" "./bin/params -help"
run_test "params mainnet" "./bin/params -network mainnet"
run_test "params testnet" "./bin/params -network testnet"
run_test "params local" "./bin/params -network local"
run_test "params xchain" "./bin/params -network xchain"
run_test "params json mainnet" "./bin/params -network mainnet -json"
run_test "params json testnet" "./bin/params -network testnet -json"
run_test "params json local" "./bin/params -network local -json"
run_test "params json xchain" "./bin/params -network xchain -json"
echo ""

echo "==========================================="
echo "              TEST SUMMARY"
echo "==========================================="
echo -e "Tests Passed: ${GREEN}$TESTS_PASSED${NC}"
echo -e "Tests Failed: ${RED}$TESTS_FAILED${NC}"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}All tests passed successfully!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed. Please review the output above.${NC}"
    exit 1
fi