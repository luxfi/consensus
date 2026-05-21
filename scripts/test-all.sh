#!/bin/bash
# Test all consensus implementations
# Usage: ./test-all.sh

set -e

COLOR_RESET='\033[0m'
COLOR_RED='\033[0;31m'
COLOR_GREEN='\033[0;32m'
COLOR_YELLOW='\033[1;33m'
COLOR_BLUE='\033[0;34m'

FAILED=0
PASSED=0

echo -e "${COLOR_YELLOW}========================================${COLOR_RESET}"
echo -e "${COLOR_YELLOW}=== Testing All Consensus Implementations ===${COLOR_RESET}"
echo -e "${COLOR_YELLOW}========================================${COLOR_RESET}\n"

# Function to run test
run_test() {
    local name=$1
    local cmd=$2
    local dir=${3:-.}

    echo -e "\n${COLOR_BLUE}>>> Testing ${name}...${COLOR_RESET}"

    if (cd "$dir" && eval "$cmd"); then
        echo -e "${COLOR_GREEN}✅ ${name} PASSED${COLOR_RESET}"
        ((PASSED++))
        return 0
    else
        echo -e "${COLOR_RED}❌ ${name} FAILED${COLOR_RESET}"
        ((FAILED++))
        return 1
    fi
}

# 1. Test Go
run_test "Go" "go test -v ./..." "."

# 2. Test C
if [ -d "pkg/c" ]; then
    run_test "C" "make clean && make all && make test" "pkg/c"
fi

# 4. Test Rust
if [ -d "pkg/rust" ]; then
    if command -v cargo &> /dev/null; then
        # Build C library first (Rust depends on it)
        echo -e "${COLOR_BLUE}Building C library for Rust FFI...${COLOR_RESET}"
        (cd pkg/c && make all)
        run_test "Rust" "cargo test --release" "pkg/rust"
    else
        echo -e "${COLOR_YELLOW}⚠️  Rust skipped (cargo not found)${COLOR_RESET}"
    fi
fi

# 5. Test Python (optional - may need dependencies)
if [ -d "pkg/python" ]; then
    if command -v python3 &> /dev/null && python3 -c "import setuptools" 2>/dev/null; then
        # Build C library first (Python depends on it)
        echo -e "${COLOR_BLUE}Building C library for Python FFI...${COLOR_RESET}"
        (cd pkg/c && make all)
        run_test "Python" "python3 setup.py build_ext --inplace && python3 test_consensus_comprehensive.py" "pkg/python" || true
    else
        echo -e "${COLOR_YELLOW}⚠️  Python skipped (missing python3 or setuptools)${COLOR_RESET}"
    fi
fi

# Summary
echo -e "\n${COLOR_YELLOW}========================================${COLOR_RESET}"
echo -e "${COLOR_YELLOW}=== Test Summary ===${COLOR_RESET}"
echo -e "${COLOR_YELLOW}========================================${COLOR_RESET}"
echo -e "${COLOR_GREEN}Passed: ${PASSED}${COLOR_RESET}"
echo -e "${COLOR_RED}Failed: ${FAILED}${COLOR_RESET}"

if [ $FAILED -eq 0 ]; then
    echo -e "\n${COLOR_GREEN}🎉 ALL TESTS PASSED!${COLOR_RESET}"
    exit 0
else
    echo -e "\n${COLOR_RED}❌ SOME TESTS FAILED${COLOR_RESET}"
    exit 1
fi
