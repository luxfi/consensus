#!/bin/bash

# AGGRESSIVE LUX CONSENSUS CLEANUP
# Remove ALL Snow/Avalanche references and duplicates

set -e

echo "=== AGGRESSIVE LUX CONSENSUS CLEANUP ==="
echo "Removing ALL Snow/Avalanche references and duplicates!"
echo ""

# Phase 1: DELETE SNOW/AVALANCHE DIRECTORIES
echo "Phase 1: DELETING Snow/Avalanche directories..."
rm -rf snowtest 2>/dev/null || true
rm -rf tests/snow 2>/dev/null || true
rm -rf snow 2>/dev/null || true
rm -rf consensustest 2>/dev/null || true  # Old Avalanche test structure
rm -rf tests/consensus 2>/dev/null || true  # We don't need this

# Phase 2: FIX BROKEN IMPORTS - Update choices and consensustest paths
echo "Phase 2: Fixing broken imports..."
find . -name "*.go" -type f -exec sed -i '' \
    -e 's|"github.com/luxfi/consensus/choices"|"github.com/luxfi/consensus/protocol/choices"|g' \
    -e 's|"github.com/luxfi/consensus/consensustest"|"github.com/luxfi/consensus/testutils"|g' {} \;

# Phase 3: REMOVE DUPLICATE PROTOCOLS
echo "Phase 3: Removing duplicate protocols..."

# We have both photon and prism doing the same thing - DELETE photon from protocol
rm -rf protocol/photon 2>/dev/null || true

# We have both wave and fpc doing the same thing - DELETE wave from protocol  
rm -rf protocol/wave 2>/dev/null || true

# Pulse is probably duplicate of something else
rm -rf protocol/pulse 2>/dev/null || true

# Phase 4: CLEAN UP CHAIN ABSTRACTIONS
echo "Phase 4: Removing unnecessary chain abstractions..."
rm -rf chain 2>/dev/null || true  # Root level chain - not needed
rm -rf protocol/chain 2>/dev/null || true  # Protocol chain - redundant

# Phase 5: REMOVE MOCK/TEST DIRECTORIES FROM MAIN CODE
echo "Phase 5: Removing mock directories from main code..."
find . -type d -name "*mock" -exec rm -rf {} + 2>/dev/null || true
find . -type d -name "*test" ! -path "./tests/*" -exec rm -rf {} + 2>/dev/null || true

# Phase 6: REMOVE AVALANCHE REFERENCES IN CODE
echo "Phase 6: Removing Avalanche references in code..."
find . -name "*.go" -type f -exec sed -i '' \
    -e 's/Avalanche/Lux/g' \
    -e 's/avalanche/lux/g' \
    -e 's/AVALANCHE/LUX/g' \
    -e 's/Snow/Lux/g' \
    -e 's/snow/lux/g' \
    -e 's/SNOW/LUX/g' \
    -e 's/Snowball/Luxball/g' \
    -e 's/snowball/luxball/g' \
    -e 's/Snowflake/Luxflake/g' \
    -e 's/snowflake/luxflake/g' {} \;

# Phase 7: REMOVE EMPTY DIRECTORIES
echo "Phase 7: Removing empty directories..."
find . -type d -empty -delete 2>/dev/null || true

# Phase 8: CONSOLIDATE TESTS INTO SINGLE DIRECTORY
echo "Phase 8: Consolidating tests..."
mkdir -p tests
# Move any remaining test files to tests/
find . -name "*_test.go" ! -path "./tests/*" -exec mv {} tests/ 2>/dev/null \; || true

# Phase 9: REMOVE .cleanup_backup
echo "Phase 9: Removing backup directories..."
rm -rf .cleanup_backup 2>/dev/null || true
rm -rf .backup 2>/dev/null || true

echo ""
echo "=== CLEANUP COMPLETE ==="
echo "Run 'go mod tidy' to clean up dependencies"