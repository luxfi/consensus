// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

// Package consensus provides a clean, single-import interface to the Lux consensus system.
// This is the main SDK surface for applications using Lux consensus.
package consensus

import (
	"context"

	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/consensus/types"
)

// Type aliases for clean single-import experience
type (
	// Engine types
	Engine = engine.Engine
	Chain  = engine.Chain
	Config = types.Config

	// Core types
	Block       = types.Block
	Vote        = types.Vote
	Certificate = types.Certificate
	ID          = types.ID
	NodeID      = types.NodeID
	Hash        = types.Hash
	Status      = types.Status
	Decision    = types.Decision
	VoteType    = types.VoteType
)

// Constants re-exported for convenience
const (
	// Decision outcomes
	DecideUndecided = types.DecideUndecided
	DecideAccept    = types.DecideAccept
	DecideReject    = types.DecideReject

	// Vote types
	VotePreference = types.VotePreference
	VoteCommit     = types.VoteCommit
	VoteCancel     = types.VoteCancel

	// Block status
	StatusUnknown    = types.StatusUnknown
	StatusProcessing = types.StatusProcessing
	StatusRejected   = types.StatusRejected
	StatusAccepted   = types.StatusAccepted
)

// Variables re-exported for convenience
var (
	// GenesisID is the ID of the genesis block
	GenesisID = types.GenesisID

	// Common errors
	ErrBlockNotFound  = types.ErrBlockNotFound
	ErrInvalidBlock   = types.ErrInvalidBlock
	ErrInvalidVote    = types.ErrInvalidVote
	ErrNoQuorum       = types.ErrNoQuorum
	ErrAlreadyVoted   = types.ErrAlreadyVoted
	ErrNotValidator   = types.ErrNotValidator
	ErrTimeout        = types.ErrTimeout
	ErrNotInitialized = types.ErrNotInitialized
)

// DefaultConfig returns the default consensus configuration
func DefaultConfig() Config {
	return types.DefaultConfig()
}

// NewChain creates a new chain consensus engine
func NewChain(cfg Config) *Chain {
	return engine.NewChain(cfg)
}

// NewDAG creates a new DAG consensus engine (placeholder for future)
func NewDAG(cfg Config) Engine {
	// TODO: Implement DAG engine
	return engine.NewChain(cfg)
}

// NewPQ creates a new post-quantum consensus engine (placeholder for future)
func NewPQ(cfg Config) Engine {
	// TODO: Implement PQ engine
	return engine.NewChain(cfg)
}

// Helper functions

// NewBlock creates a new block with default values
func NewBlock(id ID, parentID ID, height uint64, payload []byte) *Block {
	return &Block{
		ID:       id,
		ParentID: parentID,
		Height:   height,
		Payload:  payload,
	}
}

// NewVote creates a new vote
func NewVote(blockID ID, voteType VoteType, voter NodeID) *Vote {
	return &Vote{
		BlockID:  blockID,
		VoteType: voteType,
		Voter:    voter,
	}
}

// QuickStart initializes and starts a consensus engine with default config
func QuickStart(ctx context.Context) (*Chain, error) {
	cfg := DefaultConfig()
	chain := NewChain(cfg)
	if err := chain.Start(ctx); err != nil {
		return nil, err
	}
	return chain, nil
}