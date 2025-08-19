#!/bin/bash

# Verification script for Lux Consensus Refactoring

set -e

echo "=== Verifying Lux Consensus Refactoring ==="
echo ""

# Color codes for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Function to check if directory exists
check_dir() {
    if [ -d "$1" ]; then
        echo -e "${GREEN}✓${NC} Directory exists: $1"
        return 0
    else
        echo -e "${RED}✗${NC} Directory missing: $1"
        return 1
    fi
}

# Function to check if file exists
check_file() {
    if [ -f "$1" ]; then
        echo -e "${GREEN}✓${NC} File exists: $1"
        return 0
    else
        echo -e "${RED}✗${NC} File missing: $1"
        return 1
    fi
}

# Function to check imports
check_import() {
    local file=$1
    local import=$2
    if grep -q "$import" "$file" 2>/dev/null; then
        echo -e "${GREEN}✓${NC} Import found in $file: $import"
        return 0
    else
        echo -e "${RED}✗${NC} Import missing in $file: $import"
        return 1
    fi
}

echo "1. Checking new directory structure..."
echo "----------------------------------------"

# Check core directories
check_dir "core/prism"
check_dir "core/fpc"
check_dir "core/focus"
check_dir "core/beam"
check_dir "core/dag"
check_dir "core/dag/flare"
check_dir "core/dag/horizon"

# Check protocol directories
check_dir "protocol/nova"
check_dir "protocol/nebula"
check_dir "protocol/quasar"
check_dir "protocol/compat"

# Check new modules
check_dir "witness"
check_dir "engine/runner"

echo ""
echo "2. Checking key files..."
echo "-------------------------"

check_file "witness/verkle.go"
check_file "migration_map.txt"
check_file "README_REFACTORED.md"
check_file "update_imports.sh"

echo ""
echo "3. Verifying import updates..."
echo "-------------------------------"

# Check a sample of updated imports
if [ -f "flare/flare.go" ]; then
    check_import "flare/flare.go" "github.com/luxfi/consensus/core/fpc"
    check_import "flare/flare.go" "github.com/luxfi/consensus/core/dag/horizon"
fi

echo ""
echo "4. Running tests..."
echo "-------------------"

# Run tests for refactored modules
echo "Testing core/prism..."
if go test ./core/prism/... > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC} core/prism tests pass"
else
    echo -e "${RED}✗${NC} core/prism tests fail"
fi

echo "Testing core/fpc..."
if go test ./core/fpc/... > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC} core/fpc tests pass"
else
    echo -e "${RED}✗${NC} core/fpc tests fail"
fi

echo "Testing core/beam..."
if go test ./core/beam/... > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC} core/beam tests pass"
else
    echo -e "${RED}✗${NC} core/beam tests fail"
fi

echo ""
echo "5. Building project..."
echo "---------------------"

if make build > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC} Build successful"
else
    echo -e "${RED}✗${NC} Build failed"
fi

echo ""
echo "6. Checking for compilation errors..."
echo "-------------------------------------"

if go build ./... > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC} No compilation errors"
else
    echo -e "${RED}✗${NC} Compilation errors found"
fi

echo ""
echo "7. Running comprehensive tests..."
echo "---------------------------------"

# Count passing tests
PASS_COUNT=$(go test ./... 2>&1 | grep -c "^ok" || true)
FAIL_COUNT=$(go test ./... 2>&1 | grep -c "^FAIL" || true)

echo "Test Results:"
echo -e "  ${GREEN}Passing packages: $PASS_COUNT${NC}"
echo -e "  ${RED}Failing packages: $FAIL_COUNT${NC}"

if [ "$FAIL_COUNT" -eq 0 ]; then
    echo -e "${GREEN}✓${NC} All tests passing!"
else
    echo -e "${RED}✗${NC} Some tests failing"
fi

echo ""
echo "8. Checking test coverage..."
echo "----------------------------"

# Get coverage for key modules
echo "Core module coverage:"
go test -cover ./core/prism/... 2>&1 | grep coverage || true
go test -cover ./core/fpc/... 2>&1 | grep coverage || true
go test -cover ./core/beam/... 2>&1 | grep coverage || true

echo ""
echo "=== Verification Summary ==="
echo "============================"

# Final summary
if [ "$FAIL_COUNT" -eq 0 ]; then
    echo -e "${GREEN}✅ REFACTORING SUCCESSFUL!${NC}"
    echo "All directories created, imports updated, and tests passing."
else
    echo -e "${RED}⚠️  REFACTORING NEEDS ATTENTION${NC}"
    echo "Some tests are failing. Please review and fix."
fi

echo ""
echo "Next steps:"
echo "1. Review the changes in detail"
echo "2. Update any external dependencies"
echo "3. Run performance benchmarks"
echo "4. Update documentation if needed"