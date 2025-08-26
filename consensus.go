// Package consensus provides the Lux consensus implementation.
package consensus

import (
	"context"

	"github.com/luxfi/consensus/codec"
	"github.com/luxfi/consensus/config"
	consensuscontext "github.com/luxfi/consensus/context"
	"github.com/luxfi/consensus/engine/chain"
	"github.com/luxfi/consensus/engine/dag"
	"github.com/luxfi/consensus/engine/pq"
)

// LegacyEngine is the legacy consensus engine interface
type LegacyEngine interface {
	// Start starts the engine
	Start(context.Context, uint32) error

	// Stop stops the engine
	Stop(context.Context) error

	// HealthCheck performs a health check
	HealthCheck(context.Context) (interface{}, error)

	// IsBootstrapped returns whether the engine is bootstrapped
	IsBootstrapped() bool
}

// ModernEngine represents a modern consensus engine interface
type ModernEngine interface {
	// GetID returns the engine identifier  
	GetID() interface{}
}

// NewChainEngine creates a new chain consensus engine
func NewChainEngine() LegacyEngine {
	return chain.New()
}

// NewDAGEngine creates a new DAG consensus engine
func NewDAGEngine() LegacyEngine {
	return dag.New()
}

// NewPQEngine creates a new post-quantum consensus engine
func NewPQEngine() ModernEngine {
	return pq.New()
}

// Config returns default consensus parameters for different network sizes
func Config(nodes int) config.Parameters {
	switch {
	case nodes <= 5:
		return config.LocalParams()
	case nodes <= 11:
		return config.TestnetParams()
	case nodes <= 21:
		return config.MainnetParams()
	default:
		// For larger networks, use mainnet with adjusted K
		cfg := config.MainnetParams()
		cfg.K = nodes
		return cfg
	}
}

// Export types from sub-packages for convenience
type (
	// Context is the consensus context
	Context = consensuscontext.Context

	// ValidatorState is the validator state interface
	ValidatorState = consensuscontext.ValidatorState

	// CodecVersion is the codec version
	CodecVersion = codec.CodecVersion
)

// Export constants
const (
	// CurrentCodecVersion is the current codec version
	CurrentCodecVersion = codec.CurrentVersion
)

// Export variables
var (
	// Codec is the consensus codec
	Codec = codec.Codec
)

// Export functions from context
var (
	GetTimestamp       = consensuscontext.GetTimestamp
	GetChainID         = consensuscontext.GetChainID
	GetNetID           = consensuscontext.GetNetID
	GetValidatorState  = consensuscontext.GetValidatorState
	WithContext        = consensuscontext.WithContext
	FromContext        = consensuscontext.FromContext
	GetNodeID          = consensuscontext.GetNodeID
	WithIDs            = consensuscontext.WithIDs
	WithValidatorState = consensuscontext.WithValidatorState
)

// WithQuantumIDs adds quantum IDs to context
func WithQuantumIDs(ctx context.Context, qids *QuantumIDs) context.Context {
	if qids == nil {
		return ctx
	}
	ids := consensuscontext.IDs{
		QuantumID: qids.QuantumID,
		NetID:     qids.NetID,
		ChainID:   qids.ChainID,
		NodeID:    qids.NodeID,
	}
	return consensuscontext.WithIDs(ctx, ids)
}


// AppError represents an application error
type AppError struct {
	Code    int
	Message string
}
