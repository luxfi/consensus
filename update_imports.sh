#!/bin/bash

# Script to update imports throughout the codebase

echo "Updating imports in Go files..."

# Function to update imports in a file
update_file_imports() {
    local file=$1
    
    # Core consensus stages
    sed -i.bak 's|"github.com/luxfi/consensus/protocol/photon"|"github.com/luxfi/consensus/core/prism"|g' "$file"
    sed -i.bak 's|"github.com/luxfi/consensus/protocol/wave"|"github.com/luxfi/consensus/core/fpc"|g' "$file"
    sed -i.bak 's|"github.com/luxfi/consensus/focus"|"github.com/luxfi/consensus/core/focus"|g' "$file"
    sed -i.bak 's|"github.com/luxfi/consensus/beam"|"github.com/luxfi/consensus/core/beam"|g' "$file"
    
    # DAG components
    sed -i.bak 's|"github.com/luxfi/consensus/flare"|"github.com/luxfi/consensus/core/dag/flare"|g' "$file"
    sed -i.bak 's|"github.com/luxfi/consensus/horizon"|"github.com/luxfi/consensus/core/dag/horizon"|g' "$file"
    sed -i.bak 's|"github.com/luxfi/consensus/graph"|"github.com/luxfi/consensus/core/dag"|g' "$file"
    
    # Protocol compatibility rename
    sed -i.bak 's|"github.com/luxfi/consensus/protocol/prism"|"github.com/luxfi/consensus/protocol/compat"|g' "$file"
    
    # Clean up backup files
    rm -f "${file}.bak"
}

# Find all Go files and update imports
find . -name "*.go" -type f | while read -r file; do
    update_file_imports "$file"
done

echo "✓ Imports updated"
