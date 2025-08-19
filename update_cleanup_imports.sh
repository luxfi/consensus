#!/bin/bash

echo "Updating imports after cleanup..."

# Update imports for moved modules
find . -name "*.go" -type f | while read -r file; do
    # Update poll imports
    sed -i.bak 's|"github.com/luxfi/consensus/poll"|"github.com/luxfi/consensus/protocol/poll"|g' "$file"
    
    # Update chain imports
    sed -i.bak 's|"github.com/luxfi/consensus/chain"|"github.com/luxfi/consensus/protocol/chain"|g' "$file"
    
    # Update choices imports
    sed -i.bak 's|"github.com/luxfi/consensus/choices"|"github.com/luxfi/consensus/protocol/choices"|g' "$file"
    
    # Update bootstrap imports
    sed -i.bak 's|"github.com/luxfi/consensus/bootstrap"|"github.com/luxfi/consensus/protocol/bootstrap"|g' "$file"
    
    # Update runtime imports
    sed -i.bak 's|"github.com/luxfi/consensus/runtimes/quasar"|"github.com/luxfi/consensus/engine/quasar"|g' "$file"
    
    # Update test imports
    sed -i.bak 's|"github.com/luxfi/consensus/consensustest"|"github.com/luxfi/consensus/tests/consensus"|g' "$file"
    sed -i.bak 's|"github.com/luxfi/consensus/snowtest"|"github.com/luxfi/consensus/tests/snow"|g' "$file"
    
    # Remove any references to deleted modules
    sed -i.bak 's|"github.com/luxfi/consensus/beam"|"github.com/luxfi/consensus/core/beam"|g' "$file"
    sed -i.bak 's|"github.com/luxfi/consensus/flare"|"github.com/luxfi/consensus/core/dag/flare"|g' "$file"
    sed -i.bak 's|"github.com/luxfi/consensus/horizon"|"github.com/luxfi/consensus/core/dag/horizon"|g' "$file"
    sed -i.bak 's|"github.com/luxfi/consensus/focus"|"github.com/luxfi/consensus/core/focus"|g' "$file"
    sed -i.bak 's|"github.com/luxfi/consensus/graph"|"github.com/luxfi/consensus/core/dag"|g' "$file"
    
    # Clean up backup files
    rm -f "${file}.bak"
done

echo "Import updates complete"
