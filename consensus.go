// Package consensus provides the Lux consensus implementation.
// This is a minimal, Wave-first consensus architecture designed for integration
// with node, EVM, and CoreTH.
package consensus

import (
	"context"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/prism"
	"github.com/luxfi/consensus/core/wave"
	"github.com/luxfi/consensus/engines/chain"
	"github.com/luxfi/consensus/protocol/nova"
	"github.com/luxfi/consensus/types"
)

// Engine is the main consensus engine interface
type Engine[ID comparable] interface {
	// Tick advances consensus for the given item
	Tick(ctx context.Context, id ID)
	
	// State returns the current consensus state for an item
	State(id ID) (wave.State[ID], bool)
}

// NewChainEngine creates a new chain consensus engine
func NewChainEngine[ID comparable](
	cfg config.Parameters,
	peers []types.NodeID,
	transport chain.Transport[ID],
) Engine[ID] {
	sampler := prism.New(peers, prism.DefaultOptions())
	return chain.New[ID](cfg, sampler, transport)
}

// Config returns default consensus parameters for different network sizes
func Config(nodes int) config.Parameters {
	cfg := config.DefaultParams()
	cfg.K = nodes
	
	// Adjust parameters based on network size
	switch {
	case nodes <= 5:
		cfg.Alpha = 0.6
		cfg.Beta = 3
	case nodes <= 11:
		cfg.Alpha = 0.7
		cfg.Beta = 6
	case nodes <= 21:
		cfg.Alpha = 0.8
		cfg.Beta = 15
	default:
		cfg.Alpha = 0.8
		cfg.Beta = 30
	}
	
	return cfg
}

// Finalizer provides classical finality for consensus decisions
type Finalizer[ID comparable] interface {
	OnDecide(id ID, decision types.Decision)
	Finalized(id ID) (bool, int)
}

// NewFinalizer creates a new finalizer for tracking finalized items
func NewFinalizer[ID comparable]() Finalizer[ID] {
	return nova.New[ID]()
}

// Decision represents a consensus decision
type Decision = types.Decision

const (
	// DecideAccept indicates the item was accepted
	DecideAccept = types.DecideAccept
	
	// DecideReject indicates the item was rejected
	DecideReject = types.DecideReject
	
	// DecideUndecided indicates no decision yet
	DecideUndecided = types.DecideUndecided
)

// NodeID represents a validator node identifier
type NodeID = types.NodeID

// VoteMsg represents a vote message in consensus
type VoteMsg[ID comparable] = wave.VoteMsg[ID]

// Transport defines the network transport interface
type Transport[ID comparable] interface {
	chain.Transport[ID]
}

// Sampler provides peer sampling for consensus
type Sampler[ID comparable] interface {
	prism.Sampler[ID]
}

// NewSampler creates a new peer sampler
func NewSampler[ID comparable](peers []NodeID, opts prism.Options) Sampler[ID] {
	return prism.New(peers, opts)
}

// DefaultSamplerOptions returns default sampler configuration
func DefaultSamplerOptions() prism.Options {
	return prism.DefaultOptions()
}