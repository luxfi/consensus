#!/bin/bash

# Consensus Module Reorganization Script
set -e

echo "=== Starting Consensus Module Reorganization ==="

echo "Step 1: Flattening nested protocol directories..."
# Move nova/nova contents up one level
if [ -d "protocols/nova/nova" ]; then
    echo "  - Flattening protocols/nova/nova"
    mv protocols/nova/nova/* protocols/nova/ 2>/dev/null || true
    rm -rf protocols/nova/nova
fi

echo "Step 2: Clean up old structures..."
# Remove snowman 
[ -d "protocols/snowman" ] && rm -rf protocols/snowman

echo "Step 3: Create clean engine/core interfaces..."
mkdir -p engine/core
cat > engine/core/interfaces.go << 'EOFI'
package core

import (
    "context"
    "github.com/luxfi/consensus/types"
    "github.com/luxfi/consensus/validator"
)

type Logger interface {
    Debug(msg string, kv ...any)
    Info(msg string, kv ...any)
    Warn(msg string, kv ...any)
    Error(msg string, kv ...any)
}

type Clock interface {
    NowUnixMillis() int64
    After(dMs int64, f func())
}

type Storage interface {
    View(fn func(Tx) error) error
    Update(fn func(Tx) error) error
}

type Tx interface {
    Get(bucket, key []byte) ([]byte, error)
    Put(bucket, key, val []byte) error
    Delete(bucket, key []byte) error
}

type Backend interface {
    Validators() validator.Set
    BroadcastVote(ctx context.Context, msg []byte) error
    SubmitBlock(ctx context.Context, blk types.Block) error
}

type Metrics interface {
    ObserveLatency(name string, ms int64)
    IncCounter(name string, delta int64)
    SetGauge(name string, val float64)
}

type Params struct {
    K               int
    AlphaPreference int
    AlphaConfidence int
    Beta            int
    SlotMillis      int
    PQEnabled       bool
}

type Deps struct {
    Log     Logger
    Clock   Clock
    Store   Storage
    Back    Backend
    Metrics Metrics
}
EOFI

echo "Step 4: Create chain engine..."
mkdir -p engine/chain
cat > engine/chain/engine.go << 'EOFC'
package chain

import (
    "context"
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/consensus/protocols/nova"
)

type Engine struct {
    params core.Params
    deps   core.Deps
    nova   *nova.Topological
}

func New(params core.Params, deps core.Deps) *Engine {
    return &Engine{
        params: params,
        deps:   deps,
    }
}

func (e *Engine) Start(ctx context.Context) error {
    e.deps.Log.Info("Starting chain engine", "k", e.params.K)
    return nil
}

func (e *Engine) Stop() error {
    e.deps.Log.Info("Stopping chain engine")
    return nil
}
EOFC

echo "Step 5: Create PQ wrapper engine..."
mkdir -p engine/pq
cat > engine/pq/engine.go << 'EOFP'
package pq

import (
    "context"
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/consensus/protocols/quasar"
)

type Engine struct {
    inner   interface{}
    quasar  *quasar.Engine
    params  core.Params
    deps    core.Deps
}

func Wrap(inner interface{}, params core.Params, deps core.Deps) *Engine {
    return &Engine{
        inner:  inner,
        params: params,
        deps:   deps,
    }
}

func (e *Engine) Start(ctx context.Context) error {
    e.deps.Log.Info("Starting PQ wrapper", "enabled", e.params.PQEnabled)
    return nil
}
EOFP

echo "Step 6: Create utils structure..."
mkdir -p utils/bag utils/set utils/sampler utils/constants utils/math

# Move bag if exists
[ -f "core/utils/bag.go" ] && cp core/utils/bag.go utils/bag/

echo "=== Reorganization Complete ==="
