#!/bin/bash

# Final consensus module reorganization to achieve clean architecture:
# - Pure consensus protocols (no I/O)
# - Thin engine orchestrators (transport-free)
# - Clear separation of concerns

set -e

echo "Starting final consensus module reorganization..."
cd /Users/z/work/lux/consensus

# 1. Create the clean directory structure
echo "Creating clean directory structure..."

# Core interfaces and utils
mkdir -p core/interfaces
mkdir -p core/utils

# Protocol directories (already mostly there)
mkdir -p protocols/{photon,pulse,wave,nova,nebula,prism,quasar}
mkdir -p protocols/nebula/{bootstrap,vertex}

# Engine orchestrators
mkdir -p engine/{core,chain,dag,pq}

# Supporting directories
mkdir -p config
mkdir -p types
mkdir -p utils/{bag,set,sampler,constants,math}
mkdir -p validator

# 2. Move core interfaces
echo "Setting up core interfaces..."

cat > core/interfaces/acceptor.go << 'EOF'
package interfaces

import "github.com/luxfi/ids"

// Acceptor defines the interface for accepting decisions
type Acceptor interface {
    Accept() error
    Reject() error
    ID() ids.ID
}
EOF

cat > core/interfaces/consensus.go << 'EOF'
package interfaces

import (
    "context"
    "github.com/luxfi/ids"
)

// Consensus defines the core consensus interface
type Consensus interface {
    Parameters() Parameters
    IsVirtuous(ID ids.ID) bool
    Add(Decidable) error
    RecordPoll(votes []ids.ID) error
    Finalized() bool
}

// Parameters defines consensus parameters
type Parameters interface {
    K() int
    AlphaPreference() int
    AlphaConfidence() int
    Beta() int
}
EOF

cat > core/interfaces/decidable.go << 'EOF'
package interfaces

import "github.com/luxfi/ids"

// Decidable represents an item that can be decided by consensus
type Decidable interface {
    ID() ids.ID
    Accept() error
    Reject() error
    Status() Status
}
EOF

cat > core/interfaces/status.go << 'EOF'
package interfaces

// Status represents the status of a decidable item
type Status uint8

const (
    Unknown Status = iota
    Processing
    Rejected
    Accepted
)

func (s Status) String() string {
    switch s {
    case Unknown:
        return "Unknown"
    case Processing:
        return "Processing"
    case Rejected:
        return "Rejected"
    case Accepted:
        return "Accepted"
    default:
        return "Invalid"
    }
}
EOF

# 3. Create engine core interfaces (transport-free)
echo "Creating engine core interfaces..."

cat > engine/core/interfaces.go << 'EOF'
// Package core provides transport-free interfaces for consensus engines
package core

import (
    "context"
    "github.com/luxfi/consensus/types"
    "github.com/luxfi/consensus/validator"
)

// Logger provides logging capabilities
type Logger interface {
    Debug(msg string, kv ...any)
    Info(msg string, kv ...any)
    Warn(msg string, kv ...any)
    Error(msg string, kv ...any)
}

// Clock provides time-related operations
type Clock interface {
    NowUnixMillis() int64
    After(dMs int64, f func())
}

// Storage provides persistent storage operations
type Storage interface {
    View(fn func(Tx) error) error
    Update(fn func(Tx) error) error
}

// Tx represents a storage transaction
type Tx interface {
    Get(bucket, key []byte) ([]byte, error)
    Put(bucket, key, val []byte) error
    Delete(bucket, key []byte) error
}

// Backend provides consensus backend operations
type Backend interface {
    Validators() validator.Set
    BroadcastVote(ctx context.Context, msg []byte) error // abstract; node wires to P2P
    SubmitBlock(ctx context.Context, blk types.Block) error
}

// Metrics provides instrumentation capabilities
type Metrics interface {
    ObserveLatency(name string, ms int64)
    IncCounter(name string, delta int64)
    SetGauge(name string, val float64)
}

// Params defines consensus parameters
type Params struct {
    K                 int
    AlphaPreference   int
    AlphaConfidence   int
    Beta              int
    SlotMillis        int
    PQEnabled         bool
}

// Deps provides dependencies for consensus engines
type Deps struct {
    Log     Logger
    Clock   Clock
    Store   Storage
    Back    Backend
    Metrics Metrics
}
EOF

# 4. Create chain engine orchestrator
echo "Creating chain engine orchestrator..."

cat > engine/chain/engine.go << 'EOF'
// Package chain provides the Nova linear chain engine orchestrator
package chain

import (
    "context"
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/consensus/protocols/nova"
    "github.com/luxfi/consensus/protocols/photon"
    "github.com/luxfi/consensus/protocols/pulse"
    "github.com/luxfi/consensus/protocols/wave"
)

// Engine orchestrates Nova consensus pipeline (Photon→Pulse/Wave→Nova)
type Engine struct {
    params core.Params
    deps   core.Deps
    nova   *nova.Consensus
    photon *photon.Photon
    pulse  *pulse.Pulse
    wave   *wave.Wave
}

// New creates a new chain engine
func New(params core.Params, deps core.Deps) *Engine {
    return &Engine{
        params: params,
        deps:   deps,
        // Initialize protocols here
    }
}

// Start begins the consensus engine
func (e *Engine) Start(ctx context.Context) error {
    e.deps.Log.Info("Starting chain engine", "k", e.params.K)
    // Wire protocols together
    return nil
}

// Stop halts the consensus engine
func (e *Engine) Stop() error {
    e.deps.Log.Info("Stopping chain engine")
    return nil
}
EOF

# 5. Create DAG engine orchestrator
echo "Creating DAG engine orchestrator..."

cat > engine/dag/engine.go << 'EOF'
// Package dag provides the Nebula DAG engine orchestrator
package dag

import (
    "context"
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/consensus/protocols/nebula"
)

// Engine orchestrates Nebula DAG consensus
type Engine struct {
    params core.Params
    deps   core.Deps
    nebula *nebula.Nebula
}

// New creates a new DAG engine
func New(params core.Params, deps core.Deps) *Engine {
    return &Engine{
        params: params,
        deps:   deps,
        // Initialize Nebula here
    }
}

// Start begins the consensus engine
func (e *Engine) Start(ctx context.Context) error {
    e.deps.Log.Info("Starting DAG engine", "k", e.params.K)
    // Wire Nebula protocol
    return nil
}

// Stop halts the consensus engine
func (e *Engine) Stop() error {
    e.deps.Log.Info("Stopping DAG engine")
    return nil
}
EOF

# 6. Create PQ engine wrapper
echo "Creating PQ engine wrapper..."

cat > engine/pq/engine.go << 'EOF'
// Package pq provides the Quasar post-quantum finality wrapper
package pq

import (
    "context"
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/consensus/protocols/quasar"
)

// Engine wraps chain/dag engines with PQ certificate flow
type Engine struct {
    params   core.Params
    deps     core.Deps
    wrapped  interface{} // Either chain.Engine or dag.Engine
    quasar   *quasar.Engine
}

// WrapChain wraps a chain engine with PQ finality
func WrapChain(chain interface{}, params core.Params, deps core.Deps) *Engine {
    return &Engine{
        params:  params,
        deps:    deps,
        wrapped: chain,
        // Initialize Quasar wrapper
    }
}

// WrapDAG wraps a DAG engine with PQ finality  
func WrapDAG(dag interface{}, params core.Params, deps core.Deps) *Engine {
    return &Engine{
        params:  params,
        deps:    deps,
        wrapped: dag,
        // Initialize Quasar wrapper
    }
}

// Start begins the PQ-wrapped consensus engine
func (e *Engine) Start(ctx context.Context) error {
    e.deps.Log.Info("Starting PQ engine wrapper")
    if !e.params.PQEnabled {
        e.deps.Log.Info("PQ disabled, delegating to wrapped engine")
        // Delegate to wrapped engine
    }
    // Initialize Quasar flow
    return nil
}
EOF

# 7. Clean up Prism naming
echo "Renaming Prism components to final names..."

# Create clean Prism implementations
cat > protocols/prism/splitter.go << 'EOF'
// Package prism provides shared voting primitives
package prism

// Splitter handles sampling operations
type Splitter struct {
    // Sampling logic
}
EOF

cat > protocols/prism/refract.go << 'EOF'
package prism

// Refract handles traversal operations
type Refract struct {
    // Traversal logic
}
EOF

cat > protocols/prism/cut.go << 'EOF'
package prism

// Cut handles α/β threshold operations
type Cut struct {
    AlphaPreference int
    AlphaConfidence int
    Beta           int
}
EOF

# 8. Move existing code to proper locations
echo "Moving existing code to clean structure..."

# Move core types
if [ -d "chain" ]; then
    cp -r chain/*.go types/ 2>/dev/null || true
fi

# Move validator logic (keeping only logic, no gRPC)
if [ -d "validators" ]; then
    # Copy only the pure logic files, not gvalidators
    cp validators/state.go validator/ 2>/dev/null || true
    cp validators/set.go validator/ 2>/dev/null || true
    cp validators/manager.go validator/ 2>/dev/null || true
fi

# 9. Create clean config
echo "Creating configuration structure..."

cat > config/parameters.go << 'EOF'
package config

// Parameters defines consensus configuration
type Parameters struct {
    K                     int
    AlphaPreference       int
    AlphaConfidence       int
    Beta                  int
    MaxItemProcessingTime int64 // milliseconds
}

// DefaultParameters provides default consensus parameters
var DefaultParameters = Parameters{
    K:                     21,
    AlphaPreference:       13,
    AlphaConfidence:       18,
    Beta:                  8,
    MaxItemProcessingTime: 9630,
}

// TestnetParameters provides testnet consensus parameters
var TestnetParameters = Parameters{
    K:                     11,
    AlphaPreference:       7,
    AlphaConfidence:       9,
    Beta:                  6,
    MaxItemProcessingTime: 6300,
}

// LocalParameters provides local development parameters
var LocalParameters = Parameters{
    K:                     5,
    AlphaPreference:       3,
    AlphaConfidence:       4,
    Beta:                  2,
    MaxItemProcessingTime: 3690,
}
EOF

# 10. Clean go.mod to remove transport dependencies
echo "Cleaning go.mod..."

cat > go.mod << 'EOF'
module github.com/luxfi/consensus

go 1.21

require (
    github.com/luxfi/ids v0.1.0
    github.com/luxfi/crypto v0.1.0
    github.com/luxfi/utils v0.1.0
)

// No gRPC, ZMQ, or HTTP dependencies in pure consensus
EOF

# 11. Create a migration list for luxfi/node
echo "Creating migration list for transport code..."

cat > MOVE_TO_NODE.md << 'EOF'
# Components to Move to luxfi/node

The following components contain transport/networking code and should be moved to luxfi/node:

## To Move:
- networking/* - All networking code
- proto/* - All protobuf definitions
- validators/gvalidators/* - gRPC validator services
- utils/transport/* - Transport utilities
- utils/networking/* - Networking utilities
- cmd/* that starts servers or does RPC
- Any remaining gRPC/ZMQ/HTTP dependencies

## Node-side Adapters to Create:
- node/adapters/storage/pebble - Implements consensus/engine/core.Storage
- node/adapters/storage/badger - Implements consensus/engine/core.Storage
- node/adapters/network/p2p - Implements consensus/engine/core.Backend
- node/adapters/clock - Implements consensus/engine/core.Clock
- node/adapters/metrics/prometheus - Implements consensus/engine/core.Metrics
- node/adapters/logger - Implements consensus/engine/core.Logger

## Wiring Example:
```go
// In luxfi/node
deps := engine.Deps{
    Log:     nodeLogger,
    Clock:   nodeClock,
    Store:   badgerAdapter,
    Back:    p2pBackend,
    Metrics: promAdapter,
}
params := engine.Params{
    K:               21,
    AlphaPreference: 15,
    AlphaConfidence: 18,
    Beta:            8,
    SlotMillis:      200,
    PQEnabled:       true,
}

chain := chainengine.New(params, deps)      // linear Nova
dag   := dagenengine.New(params, deps)      // Nebula
pq    := pqengine.WrapChain(chain, params, deps) // add Quasar on top
```
EOF

echo "Final reorganization complete!"
echo ""
echo "Next steps:"
echo "1. Review MOVE_TO_NODE.md for components to migrate"
echo "2. Update imports in existing code"
echo "3. Move tests next to their packages"
echo "4. Verify pure consensus has no transport dependencies"