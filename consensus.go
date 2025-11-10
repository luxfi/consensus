// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package consensus provides a high-performance, multi-consensus blockchain engine
// with support for Chain, DAG, and Post-Quantum consensus algorithms.
package consensus

import (
	"github.com/luxfi/consensus/core"
	"github.com/luxfi/consensus/engine/chain"
	"github.com/luxfi/consensus/engine/dag"
	"github.com/luxfi/consensus/engine/pq"
)

// Config represents consensus configuration
type Config = core.Config

// Engine is the consensus engine interface
type Engine = core.Engine

// Block represents a consensus block
type Block = core.Block

// DefaultConfig returns the default consensus configuration
func DefaultConfig() Config {
	return core.DefaultConfig()
}

// NewChain creates a new chain consensus engine
func NewChain(config Config) *chain.Engine {
	return chain.New(config)
}

// NewDAG creates a new DAG consensus engine
func NewDAG(config Config) *dag.Engine {
	return dag.New(config)
}

// NewPQ creates a new post-quantum consensus engine
func NewPQ(config Config) *pq.Engine {
	return pq.New(config)
}
