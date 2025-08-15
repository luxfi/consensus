// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package fpc

import (
	"context"
	"sync/atomic"

	"github.com/luxfi/consensus/types"
)

// Status represents FPC transaction status
type Status int

const (
	StatusPending Status = iota
	StatusExecutable
	StatusFinal
)

// Config for FPC
type Config struct {
	Enable            bool
	VoteLimitPerBlock int
	EpochFence        bool
}

// DAGTap provides DAG ancestry queries
type DAGTap interface {
	InAncestry(block types.BlockID, tx types.TxRef) bool
}

// Engine manages fast path consensus
type Engine interface {
	NextVotes(budget int) []types.TxRef
	OnBlockObserved(ctx context.Context, b *Block)
	OnBlockAccepted(ctx context.Context, b *Block)
	Status(tx types.TxRef) (Status, Proof)
	OnEpochCloseStart()
	OnEpochClosed()
}

// Proof contains optional voting proof
type Proof struct {
	VoterBitmap []byte
}

// Block contains FPC-relevant block data
type Block struct {
	Author  types.NodeID
	ID      types.BlockID
	Payload struct {
		FPCVotes [][]byte
		EpochBit bool
	}
}

// engine implements FPC Engine
type engine struct {
	cfg   Config
	dag   DAGTap
	fence atomic.Bool
}

// New creates a new FPC engine
func New(cfg Config, dag DAGTap) Engine {
	return &engine{cfg: cfg, dag: dag}
}

func (e *engine) NextVotes(budget int) []types.TxRef { return nil }
func (e *engine) OnBlockObserved(ctx context.Context, b *Block) {}
func (e *engine) OnBlockAccepted(ctx context.Context, b *Block) {}
func (e *engine) Status(tx types.TxRef) (Status, Proof) { return StatusPending, Proof{} }
func (e *engine) OnEpochCloseStart() { e.fence.Store(true) }
func (e *engine) OnEpochClosed() { e.fence.Store(false) }