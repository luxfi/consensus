# Consensus Migration Guide

## Overview
The consensus repository has been refactored to a minimal Wave-first architecture. Many components that were previously in consensus have been removed as they belong in the node repository.

## Components That Should Be in Node Repository

The following packages should be implemented in the node repository as they are node-specific, not consensus-specific:

### 1. Validators Package (`validators/`)
- Validator management
- Validator state tracking  
- Validator sets
- This is application-level, not consensus algorithm

### 2. Engine Chain Block (`engine/chain/block/`)
- Block structures
- Block verification
- This is blockchain-specific, not consensus

### 3. Networking Components
- `networking/router/` - Message routing
- `networking/tracker/` - Resource tracking
- `networking/benchlist/` - Peer benchmarking
- `networking/timeout/` - Timeout management
- These are P2P layer, not consensus

### 4. Uptime Package (`uptime/`)
- Uptime tracking
- Calculator implementations
- This is node monitoring, not consensus

### 5. Core Engine Interfaces
- The old `core/` package with engine interfaces
- These are application interfaces, not consensus primitives

## New Consensus API

The consensus repository now provides a clean, minimal API:

```go
import "github.com/luxfi/consensus"

// Create consensus engine
cfg := consensus.Config(21) // for 21 nodes
engine := consensus.NewChainEngine(cfg, peers, transport)

// Run consensus
engine.Tick(ctx, itemID)

// Check state
state, ok := engine.State(itemID)
if state.Decided {
    // Handle decision
}
```

## Migration Steps

### For Node Repository

1. **Move validator management to node**:
   ```go
   // OLD: github.com/luxfi/consensus/validators
   // NEW: github.com/luxfi/node/validators
   ```

2. **Move block structures to node**:
   ```go
   // OLD: github.com/luxfi/consensus/engine/chain/block
   // NEW: github.com/luxfi/node/chain/block
   ```

3. **Move networking components to node**:
   ```go
   // OLD: github.com/luxfi/consensus/networking/*
   // NEW: github.com/luxfi/node/network/*
   ```

4. **Use new consensus API**:
   ```go
   import "github.com/luxfi/consensus"
   
   // Create transport adapter
   type NodeTransport struct {
       // ... implement transport
   }
   
   // Create engine
   engine := consensus.NewChainEngine(cfg, validators, transport)
   ```

### For EVM Repository

1. **Import validators from node**:
   ```go
   // OLD: github.com/luxfi/consensus/validators
   // NEW: github.com/luxfi/node/validators
   ```

2. **Import block types from node**:
   ```go
   // OLD: github.com/luxfi/consensus/engine/chain/block
   // NEW: github.com/luxfi/node/chain/block
   ```

3. **Use consensus through node**:
   The EVM should not directly import consensus. It should use the node's consensus integration.

## What Consensus Provides

The consensus repository now provides only:

- **Core Algorithms**: Wave, Prism (sampling), DAG components
- **Consensus Engine**: Chain and DAG engines
- **Protocol Implementations**: Nova (finality), Nebula, Quasar
- **Types**: Basic consensus types (Decision, NodeID, etc.)

## What Consensus Does NOT Provide

- Validator management (belongs in node)
- Block structures (belongs in node/chain)
- Network transport (belongs in node)
- Uptime tracking (belongs in node)
- Resource tracking (belongs in node)
- Timeout management (belongs in node)

## Example Integration

```go
package node

import (
    "github.com/luxfi/consensus"
    "github.com/luxfi/node/validators"
    "github.com/luxfi/node/network"
)

type ConsensusAdapter struct {
    engine consensus.Engine[BlockID]
    vals   validators.Manager
    net    network.Network
}

func (c *ConsensusAdapter) ProcessBlock(block *Block) {
    // Convert to consensus item
    c.engine.Tick(ctx, block.ID())
    
    // Check if decided
    state, _ := c.engine.State(block.ID())
    if state.Decided {
        // Handle decision
    }
}
```

## Benefits of This Architecture

1. **Clean Separation**: Consensus algorithms separated from application logic
2. **Reusability**: Consensus can be used by any blockchain, not just Lux
3. **Maintainability**: Clear boundaries between consensus and node
4. **Testability**: Consensus can be tested independently
5. **Flexibility**: Node can implement its own validators, blocks, networking

## Action Items

1. **Node Team**: Move validators, blocks, networking packages to node repository
2. **EVM Team**: Update imports to use node packages instead of consensus
3. **Integration Team**: Create adapters between node and consensus
4. **Testing Team**: Update integration tests for new architecture

## Timeline

This is a breaking change that requires coordinated updates across repositories:

1. Phase 1: Node repository adds missing packages
2. Phase 2: Update import paths in node and EVM
3. Phase 3: Remove deprecated code
4. Phase 4: Update documentation and examples

## Support

For questions about the migration, please refer to:
- `/consensus.go` - Main API documentation
- `/examples/` - Example implementations
- GitHub Issues - For specific problems