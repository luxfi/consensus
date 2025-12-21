// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

// Package consensus provides a clean, single-import interface to the Lux consensus system.
// This is the main SDK surface for applications using Lux consensus.
//
// For VM-related types (State, Message, Fx), use github.com/luxfi/vm
package consensus

import (
	"context"
	"errors"

	"github.com/luxfi/consensus/config"
	consensuscontext "github.com/luxfi/consensus/context"
	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/consensus/engine/interfaces"
	"github.com/luxfi/consensus/types"
)

// Type aliases for clean single-import experience
type (
	// Engine types
	Engine = engine.Engine
	Chain  = engine.Chain
	Config = types.Config

	// Context type
	Context = consensuscontext.Context

	// VM State type
	State = interfaces.State

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

	// VM states
	Unknown       = interfaces.Unknown
	Starting      = interfaces.Starting
	Syncing       = interfaces.Syncing
	Bootstrapping = interfaces.Bootstrapping
	Ready         = interfaces.Ready
	Degraded      = interfaces.Degraded
	Stopping      = interfaces.Stopping
	Stopped       = interfaces.Stopped
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
	ErrUnknownState   = errors.New("unknown state")
)

// Context accessor functions re-exported from context package
var (
	GetNetworkID      = consensuscontext.GetNetworkID
	GetValidatorState = consensuscontext.GetValidatorState
	WithContext       = consensuscontext.WithContext
	FromContext       = consensuscontext.FromContext
)

// DefaultConfig returns the default consensus configuration
func DefaultConfig() Config {
	return types.DefaultConfig()
}

// NewChain creates a new chain consensus engine
func NewChain(cfg Config) *Chain {
	return engine.NewChain(cfg)
}

// NewChainEngine creates a new chain consensus engine with default config
func NewChainEngine() Engine {
	return NewChain(DefaultConfig())
}

// NewDAG creates a new DAG consensus engine (placeholder for future)
func NewDAG(cfg Config) Engine {
	return engine.NewChain(cfg)
}

// NewDAGEngine creates a new DAG consensus engine with default config
func NewDAGEngine() Engine {
	return NewDAG(DefaultConfig())
}

// NewPQ creates a new post-quantum consensus engine (placeholder for future)
func NewPQ(cfg Config) Engine {
	return engine.NewChain(cfg)
}

// NewPQEngine creates a new PQ consensus engine with default config
func NewPQEngine() Engine {
	return NewPQ(DefaultConfig())
}

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

// GetConfig returns consensus parameters based on the number of nodes
func GetConfig(nodeCount int) config.Parameters {
	switch {
	case nodeCount == 1:
		return config.SingleValidatorParams()
	case nodeCount <= 5:
		return config.LocalParams()
	case nodeCount <= 11:
		return config.TestnetParams()
	case nodeCount <= 21:
		return config.MainnetParams()
	default:
		params := config.MainnetParams()
		params.K = nodeCount
		params.AlphaPreference = int(float64(nodeCount) * 0.69)
		params.AlphaConfidence = int(float64(nodeCount) * 0.69)
		params.BetaVirtuous = int(float64(nodeCount) * 0.69)
		params.BetaRogue = nodeCount
		params.Beta = uint32(int(float64(nodeCount) * 0.69))
		return params
	}
}

// Parameter presets for convenience
var (
	SingleValidatorParams = config.SingleValidatorParams
	LocalParams           = config.LocalParams
	TestnetParams         = config.TestnetParams
	MainnetParams         = config.MainnetParams
	DefaultParams         = config.DefaultParams
	XChainParams          = config.XChainParams
)
