# Migration Guide: v1.21.x → v1.22.0

## Overview
Version 1.22.0 introduces a major simplification of the Lux Consensus API. The package structure has been flattened for easier imports, and the API has been streamlined to provide a single-import experience across all language SDKs.

## Breaking Changes

### Go SDK

#### Package Structure Changes
The deep nested package structure has been replaced with a shallow, obvious organization:

**Old Structure:**
```
consensus/
├── engine/core/       → engine/
├── core/types/        → types/
├── core/protocol/     → protocol/
├── engine/avalanche/  → avalanche/
└── engine/snowflake/  → snowflake/
```

#### Import Changes

**Before (v1.21.x):**
```go
import (
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/consensus/core/types"
    "github.com/luxfi/consensus/config"
    "github.com/luxfi/consensus/engine/avalanche"
)

// Multiple imports needed
cfg := config.DefaultParams()
engine := core.NewConsensusEngine(cfg)
block := types.NewBlock(...)
```

**After (v1.22.0):**
```go
import "github.com/luxfi/consensus" // Single import!

// Everything available from root package
cfg := consensus.DefaultConfig()
chain := consensus.NewChain(cfg)
block := &consensus.Block{...}
```

#### API Changes

| Old API | New API |
|---------|---------|
| `core.NewConsensusEngine()` | `consensus.NewChain()` |
| `engine.ConsensusEngine` | `consensus.Chain` |
| `types.Block` | `consensus.Block` |
| `types.Vote` | `consensus.Vote` |
| `config.Parameters` | `consensus.Config` |
| `config.DefaultParams()` | `consensus.DefaultConfig()` |

### Python SDK

**Before (v1.21.x):**
```python
from lux_consensus.engine import ConsensusEngine
from lux_consensus.types import Block, Vote
from lux_consensus.config import default_config

engine = ConsensusEngine(default_config())
```

**After (v1.22.0):**
```python
from lux_consensus import Chain, Block, Vote, default_config

chain = Chain(default_config())
```

### Rust SDK

**Before (v1.21.x):**
```rust
use lux_consensus::engine::core::ConsensusEngine;
use lux_consensus::types::{Block, Vote};
use lux_consensus::config::Config;

let engine = ConsensusEngine::new(Config::default());
```

**After (v1.22.0):**
```rust
use lux_consensus::{Chain, Block, Vote, Config};

let chain = Chain::new(Config::default());
```

### C SDK

**Before (v1.21.x):**
```c
lux_consensus_config_t config = {
    .k = 20,
    .alpha_preference = 15,
    .alpha_confidence = 15,
    .beta = 20,
    // ... many fields
};

lux_consensus_engine_t* engine;
lux_consensus_engine_create(&engine, &config);
```

**After (v1.22.0):**
```c
// Simple default chain
lux_chain_t* chain = lux_chain_new_default();

// Or with node count
lux_config_t config = {
    .node_count = 20
};
lux_chain_t* chain = lux_chain_new(&config);
```

### C++ SDK

**Before (v1.21.x):**
```cpp
auto params = lux::consensus::ConsensusParams{
    .k = 20,
    .alpha_preference = 15,
    // ... many fields
};
auto engine = lux::consensus::Consensus::create(
    lux::consensus::EngineType::Avalanche, 
    params
);
```

**After (v1.22.0):**
```cpp
#include <lux/consensus.hpp>
using namespace lux::consensus;

// Simple API with factory methods
auto chain = new_chain(Config::testnet());

// Or construct directly
Chain chain(Config::custom(20));
```

## Configuration Simplification

The configuration system has been dramatically simplified. Instead of managing 10+ parameters, you now primarily specify the network size:

**Old Way:**
- Set k, alpha_preference, alpha_confidence, beta, concurrent_polls, max_outstanding_items, timeout, etc.

**New Way:**
- Specify node_count, and optimal parameters are calculated automatically
- Or use preset configurations: single_validator(), local_network(), testnet(), mainnet()

## Helper Functions

New helper functions provide common configurations:

```go
// Go
consensus.GetConfig(nodeCount) // Auto-configures based on network size

// Python
config = get_config(node_count=20)

// Rust
let config = Config::for_network_size(20);

// C++
auto config = Config::custom(20);
```

## Removed Features

The following features have been removed or simplified:
- Complex factory patterns
- Deep nested package structures
- Redundant configuration parameters
- Multiple ways to achieve the same result

## Migration Checklist

- [ ] Update all import statements to use single root import
- [ ] Replace `ConsensusEngine` with `Chain`
- [ ] Update configuration to use simplified `Config` struct
- [ ] Replace factory method calls (`NewConsensusEngine` → `NewChain`)
- [ ] Update type references to use root package aliases
- [ ] Remove unnecessary nested imports
- [ ] Test with new simplified API

## Common Issues and Solutions

### Issue: "undefined: consensus.NewChainEngine"
**Solution:** Use `consensus.NewChain()` instead

### Issue: "cannot convert int to Config"
**Solution:** Use `consensus.GetConfig(nodeCount)` helper function

### Issue: "ConsensusEngine not found"
**Solution:** Replace with `Chain` type

### Issue: Import cycle errors
**Solution:** Use only the root `consensus` package import

## Benefits of v1.22.0

1. **Simpler Imports**: One import gives you everything
2. **Cleaner API**: Intuitive method names and structure
3. **Better Defaults**: Smart configuration based on network size
4. **Type Safety**: Stronger typing with clear ownership
5. **Consistency**: Same patterns across all language SDKs

## Example: Complete Migration

### Before (Complex, Multi-Import)
```go
package main

import (
    "context"
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/consensus/engine/avalanche"
    "github.com/luxfi/consensus/core/types"
    "github.com/luxfi/consensus/config"
    "github.com/luxfi/consensus/photon"
)

func main() {
    cfg := config.DefaultParams()
    cfg.K = 20
    cfg.AlphaPreference = 15
    
    emitter := photon.NewUniformEmitter(peers, photon.DefaultEmitterOptions())
    engine := avalanche.New(cfg, emitter, transport)
    
    block := types.NewBlock(id, parentID, height, data)
    engine.Add(context.Background(), block)
}
```

### After (Simple, Single-Import)
```go
package main

import (
    "context"
    "github.com/luxfi/consensus"
)

func main() {
    // Auto-configure for 20-node network
    chain := consensus.NewChain(consensus.GetConfig(20))
    
    ctx := context.Background()
    chain.Start(ctx)
    defer chain.Stop()
    
    block := &consensus.Block{
        ID:       consensus.NewID(),
        ParentID: consensus.GenesisID,
        Height:   1,
        Payload:  data,
    }
    chain.Add(ctx, block)
}
```

## Support

For questions about migration:
- Check the examples in `/examples/` directory
- Review updated tests for usage patterns
- Open an issue on GitHub with the `migration` label

## Version Compatibility

- v1.22.0 is NOT backward compatible with v1.21.x
- Node implementations must be updated to use new API
- All language SDKs should be updated simultaneously