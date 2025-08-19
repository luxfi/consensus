#!/bin/bash

# Lux Consensus Repository Refactoring Script
# This script refactors the consensus repository structure according to the proposed design

set -e

REPO_ROOT="/Users/z/work/lux/consensus"
cd "$REPO_ROOT"

echo "=== Starting Lux Consensus Refactoring ==="
echo "Repository root: $REPO_ROOT"

# Backup current structure
echo "Creating backup of current structure..."
if [ ! -d ".backup" ]; then
    mkdir -p .backup
    cp -r protocol .backup/protocol_backup 2>/dev/null || true
    cp -r core .backup/core_backup 2>/dev/null || true
    cp -r engine .backup/engine_backup 2>/dev/null || true
fi

# Phase 1: Create new directory structure
echo "Phase 1: Creating new directory structure..."

# Create main directories if they don't exist
mkdir -p protocol/nova
mkdir -p protocol/nebula
mkdir -p protocol/quasar

mkdir -p core/prism
mkdir -p core/fpc
mkdir -p core/focus
mkdir -p core/beam
mkdir -p core/dag/flare
mkdir -p core/dag/horizon

mkdir -p engine/runner
mkdir -p witness
mkdir -p config
mkdir -p utils
mkdir -p types
mkdir -p validators
mkdir -p cmd
mkdir -p tests

echo "✓ Directory structure created"

# Phase 2: Resolve naming conflicts
echo "Phase 2: Resolving naming conflicts..."

# First, rename the existing protocol/prism to protocol/compat to avoid conflicts
if [ -d "protocol/prism" ]; then
    echo "  - Moving protocol/prism → protocol/compat (to avoid naming conflict)"
    mv protocol/prism protocol/compat 2>/dev/null || echo "    (already moved or doesn't exist)"
fi

# Phase 3: Move and rename core consensus modules
echo "Phase 3: Moving and renaming core consensus modules..."

# Move photon → core/prism
if [ -d "protocol/photon" ]; then
    echo "  - Moving protocol/photon → core/prism"
    cp -r protocol/photon/* core/prism/ 2>/dev/null || true
    # We'll delete the old directory after updating imports
fi

# Move wave → core/fpc
if [ -d "protocol/wave" ]; then
    echo "  - Moving protocol/wave → core/fpc"
    cp -r protocol/wave/* core/fpc/ 2>/dev/null || true
    # We'll delete the old directory after updating imports
fi

# Move focus module if it exists
if [ -d "focus" ]; then
    echo "  - Moving focus → core/focus"
    cp -r focus/* core/focus/ 2>/dev/null || true
fi

# Move beam module
if [ -d "beam" ]; then
    echo "  - Moving beam → core/beam"
    cp -r beam/* core/beam/ 2>/dev/null || true
fi

echo "✓ Core consensus modules moved"

# Phase 4: Consolidate DAG logic
echo "Phase 4: Consolidating DAG logic..."

# Move flare to core/dag/flare
if [ -d "flare" ]; then
    echo "  - Moving flare → core/dag/flare"
    cp -r flare/* core/dag/flare/ 2>/dev/null || true
fi

# Move horizon to core/dag/horizon
if [ -f "horizon/horizon.go" ]; then
    echo "  - Moving horizon → core/dag/horizon"
    cp horizon/* core/dag/horizon/ 2>/dev/null || true
fi

# Move graph utilities to core/dag
if [ -f "graph/graph.go" ]; then
    echo "  - Moving graph → core/dag/"
    cp graph/graph.go core/dag/ 2>/dev/null || true
fi

# Also consolidate DAG logic from engine/dag
if [ -d "engine/dag" ]; then
    echo "  - Consolidating engine/dag components"
    # Keep engine/dag for now but mark for integration
fi

echo "✓ DAG logic consolidated"

# Phase 5: Organize protocol hierarchy
echo "Phase 5: Organizing protocol hierarchy..."

# Ensure protocol modules are in place
if [ -d "protocol/nova" ] && [ -z "$(ls -A protocol/nova)" ]; then
    echo "  - Setting up protocol/nova"
    # Nova is already in the right place, just ensure it has content
fi

if [ -d "protocol/nebula" ] && [ -z "$(ls -A protocol/nebula)" ]; then
    echo "  - Setting up protocol/nebula"
    # Nebula is already in the right place
fi

if [ -d "protocol/quasar" ] && [ -z "$(ls -A protocol/quasar)" ]; then
    echo "  - Setting up protocol/quasar"
    # Quasar is already in the right place
fi

echo "✓ Protocol hierarchy organized"

# Phase 6: Setup witness module
echo "Phase 6: Setting up witness module..."

if [ -d "witness" ] && [ -z "$(ls -A witness)" ]; then
    echo "  - Initializing witness module for Verkle integration"
    cat > witness/witness.go << 'EOF'
package witness

// Package witness provides Verkle trie witness verification for stateless clients
// This module integrates with the consensus to verify state proofs during block processing

import (
    "errors"
)

// Verifier interface for state witness verification
type Verifier interface {
    // VerifyBlock verifies the state witness for a block
    VerifyBlock(block interface{}) error
    
    // VerifyProof verifies a single Verkle proof
    VerifyProof(root []byte, key []byte, value []byte, proof []byte) error
}

// DefaultVerifier implements the Verifier interface
type DefaultVerifier struct {
    // TODO: Add Verkle tree implementation
}

// NewVerifier creates a new witness verifier
func NewVerifier() Verifier {
    return &DefaultVerifier{}
}

// VerifyBlock verifies the state witness for a block
func (v *DefaultVerifier) VerifyBlock(block interface{}) error {
    // TODO: Implement Verkle witness verification
    return nil
}

// VerifyProof verifies a single Verkle proof
func (v *DefaultVerifier) VerifyProof(root []byte, key []byte, value []byte, proof []byte) error {
    // TODO: Implement single proof verification
    return nil
}
EOF
fi

echo "✓ Witness module setup complete"

# Phase 7: Create migration mapping file
echo "Phase 7: Creating import migration mapping..."

cat > migration_map.txt << 'EOF'
# Import Migration Map for Lux Consensus Refactoring
# Old Path → New Path

# Core consensus stages
github.com/luxfi/consensus/protocol/photon → github.com/luxfi/consensus/core/prism
github.com/luxfi/consensus/protocol/wave → github.com/luxfi/consensus/core/fpc
github.com/luxfi/consensus/focus → github.com/luxfi/consensus/core/focus
github.com/luxfi/consensus/beam → github.com/luxfi/consensus/core/beam

# DAG components
github.com/luxfi/consensus/flare → github.com/luxfi/consensus/core/dag/flare
github.com/luxfi/consensus/horizon → github.com/luxfi/consensus/core/dag/horizon
github.com/luxfi/consensus/graph → github.com/luxfi/consensus/core/dag

# Protocol modules remain mostly the same
github.com/luxfi/consensus/protocol/nova → github.com/luxfi/consensus/protocol/nova
github.com/luxfi/consensus/protocol/nebula → github.com/luxfi/consensus/protocol/nebula
github.com/luxfi/consensus/protocol/quasar → github.com/luxfi/consensus/protocol/quasar

# Renamed modules
github.com/luxfi/consensus/protocol/prism → github.com/luxfi/consensus/protocol/compat

# New modules
# → github.com/luxfi/consensus/witness (for Verkle proofs)
# → github.com/luxfi/consensus/engine/runner (unified engine)
EOF

echo "✓ Migration map created"

# Phase 8: Create update script for imports
echo "Phase 8: Creating import update script..."

cat > update_imports.sh << 'SCRIPT'
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
SCRIPT

chmod +x update_imports.sh

echo "✓ Import update script created"

# Phase 9: Create README for new structure
echo "Phase 9: Creating updated README..."

cat > README_REFACTORED.md << 'EOF'
# Lux Quasar Consensus - Refactored Architecture

## 🏗️ Repository Structure

```
lux-consensus/
├── protocol/              # High-level consensus protocols and finality layers
│   ├── nova/             # Classical finality protocol (DAG consensus)
│   ├── nebula/           # Extended finality layer
│   └── quasar/           # Quantum finality overlay and unified coordinator
│
├── core/                  # Core consensus stages
│   ├── prism/            # Sampling stage (peer sampling for votes)
│   ├── fpc/              # Fast Probabilistic Consensus thresholding
│   ├── focus/            # Confidence accumulation stage
│   ├── beam/             # Linear chain finalizer (optional)
│   └── dag/              # DAG-specific utilities
│       ├── flare/        # DAG ordering algorithm
│       └── horizon/      # DAG ancestry tracking
│
├── engine/               # Node engine integration
│   └── runner/          # Unified engine runner
│
├── witness/             # Verkle trie witness verification
├── networking/          # P2P networking abstractions
├── validators/          # Validator set management
├── config/             # Consensus configuration
├── utils/              # Shared utilities
├── types/              # Common type definitions
├── cmd/                # CLI tools
└── tests/              # Test suites
```

## 🔄 Migration Status

### ✅ Completed
- [x] Created new directory structure
- [x] Resolved naming conflicts (protocol/prism → protocol/compat)
- [x] Set up core consensus stages directories
- [x] Consolidated DAG logic structure
- [x] Created witness module for Verkle integration
- [x] Generated migration mapping

### 🚧 In Progress
- [ ] Moving module contents
- [ ] Updating imports throughout codebase
- [ ] Testing refactored structure
- [ ] Updating documentation

## 📦 Module Mappings

| Old Location | New Location | Description |
|-------------|--------------|-------------|
| `protocol/photon` | `core/prism` | Sampling stage |
| `protocol/wave` | `core/fpc` | FPC thresholding |
| `focus` | `core/focus` | Confidence accumulation |
| `beam` | `core/beam` | Linear finalizer |
| `flare` | `core/dag/flare` | DAG ordering |
| `horizon` | `core/dag/horizon` | DAG ancestry |
| `protocol/prism` | `protocol/compat` | Compatibility layer |

## 🚀 Next Steps

1. Run `./update_imports.sh` to update all import paths
2. Run tests to verify functionality: `go test ./...`
3. Clean up old directories after verification
4. Update external documentation

## 📚 Architecture Overview

The refactored architecture follows a clear layered approach:

1. **Core Layer** (`core/`): Fundamental consensus mechanisms
   - Prism: Sampling peers for votes
   - FPC: Applying vote thresholds
   - Focus: Building confidence over rounds
   - DAG: Managing DAG-specific operations

2. **Protocol Layer** (`protocol/`): High-level finality protocols
   - Nova: Classical finality
   - Nebula: Extended/cross-chain finality
   - Quasar: Quantum-secure finality

3. **Engine Layer** (`engine/`): Integration with node
   - Runner: Unified engine for all chain types

4. **Support Modules**: Networking, validation, configuration

This structure ensures better maintainability, clearer module boundaries, and easier future extensions.
EOF

echo "✓ README created"

echo ""
echo "=== Refactoring Structure Complete ==="
echo ""
echo "Next steps:"
echo "1. Review the changes in the new directory structure"
echo "2. Run ./update_imports.sh to update all import paths"
echo "3. Run tests: go test ./..."
echo "4. Clean up old directories after verification"
echo ""
echo "Migration map saved to: migration_map.txt"
echo "New README saved to: README_REFACTORED.md"