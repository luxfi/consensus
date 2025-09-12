#!/bin/bash

echo "🔍 COMPREHENSIVE VERIFICATION REPORT"
echo "===================================="
echo ""

# Track results
PASS=0
FAIL=0

# Function to test command
test_cmd() {
    local desc=$1
    local cmd=$2
    echo -n "Testing $desc... "
    if eval "$cmd" > /dev/null 2>&1; then
        echo "✅ PASS"
        ((PASS++))
    else
        echo "❌ FAIL"
        ((FAIL++))
    fi
}

echo "📦 CI/CD PIPELINE"
echo "-----------------"
LATEST_RUN=$(gh run list --repo luxfi/consensus --limit 1 2>/dev/null | grep -E "completed\s+success")
if [ -n "$LATEST_RUN" ]; then
    echo "✅ CI Pipeline: SUCCESS"
    ((PASS++))
else
    echo "❌ CI Pipeline: FAILED"
    ((FAIL++))
fi

echo ""
echo "🛠️ CLI TOOLS"
echo "------------"
test_cmd "consensus -engine chain" "./bin/consensus -engine chain -action test"
test_cmd "consensus -engine dag" "./bin/consensus -engine dag -action test"
test_cmd "consensus -engine pq" "./bin/consensus -engine pq -action test"
test_cmd "params -network mainnet" "./bin/params -network mainnet"
test_cmd "params -network testnet" "./bin/params -network testnet"
test_cmd "params -network local" "./bin/params -network local"
test_cmd "params -json output" "./bin/params -json"
test_cmd "checker -engine all" "./bin/checker -engine all"
test_cmd "sim -nodes 5" "./bin/sim -nodes 5 -rounds 3"
test_cmd "bench -engine chain" "./bin/bench -engine chain -blocks 10"

echo ""
echo "🧪 UNIT TESTS"
echo "-------------"
test_cmd "consensus tests" "go test github.com/luxfi/consensus"
test_cmd "config tests" "go test github.com/luxfi/consensus/config"
test_cmd "engine/chain tests" "go test github.com/luxfi/consensus/engine/chain"
test_cmd "engine/dag tests" "go test github.com/luxfi/consensus/engine/dag"
test_cmd "engine/pq tests" "go test github.com/luxfi/consensus/engine/pq"

echo ""
echo "⚡ BENCHMARKS"
echo "-------------"
test_cmd "config benchmarks" "go test -bench=. -run=XXX github.com/luxfi/consensus/config"
test_cmd "engine/pq benchmarks" "go test -bench=. -run=XXX github.com/luxfi/consensus/engine/pq"

echo ""
echo "🔐 RACE DETECTION"
echo "-----------------"
test_cmd "race detection" "go test -race github.com/luxfi/consensus"

echo ""
echo "📊 FINAL REPORT"
echo "==============="
echo "✅ Passed: $PASS"
echo "❌ Failed: $FAIL"
echo ""
if [ $FAIL -eq 0 ]; then
    echo "🎉 100% SUCCESS RATE - ALL SYSTEMS OPERATIONAL!"
else
    echo "⚠️ Some tests failed. Please investigate."
fi
