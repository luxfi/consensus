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
