#!/bin/bash

# Update imports after consensus reorganization
set -e

echo "=== Updating imports after reorganization ==="

# Find all Go files and update imports
find . -name "*.go" -type f | while read -r file; do
    # Skip vendor and .git
    if [[ "$file" == *"/vendor/"* ]] || [[ "$file" == *"/.git/"* ]]; then
        continue
    fi
    
    # Update protocol imports (remove nested protocol/)
    sed -i.bak 's|protocols/protocol/|protocols/|g' "$file"
    
    # Update engine imports
    sed -i.bak 's|engine/graph/|engine/dag/|g' "$file"
    sed -i.bak 's|engine/snowman/|engine/chain/|g' "$file"
    sed -i.bak 's|engine/common/|engine/core/|g' "$file"
    
    # Update nova imports (collapsed from nova/nova)
    sed -i.bak 's|protocols/nova/nova/|protocols/nova/|g' "$file"
    
    # Update prism file imports (renamed files)
    sed -i.bak 's|prism/sampler|prism/splitter|g' "$file"
    sed -i.bak 's|prism/early_term_traversal|prism/refract|g' "$file"
    sed -i.bak 's|prism/threshold|prism/cut|g' "$file"
    
    # Remove backup files
    rm -f "${file}.bak"
done

echo "Import updates complete!"
