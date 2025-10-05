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

// Engine is the main consensus engine interface
type Engine interface {
	// Start starts the engine
	Start(context.Context, uint32) error

	// Stop stops the engine
	Stop(context.Context) error

	// HealthCheck performs a health check
	HealthCheck(context.Context) (interface{}, error)

	// IsBootstrapped returns whether the engine is bootstrapped
	IsBootstrapped() bool
}

// NewChainEngine creates a new chain consensus engine
func NewChainEngine() Engine {
	return chain.New()
}

// NewDAGEngine creates a new DAG consensus engine
func NewDAGEngine() Engine {
	return &dagEngineWrapper{dag.New()}
}

// dagEngineWrapper wraps dag.Engine to implement consensus.Engine
type dagEngineWrapper struct {
	dag.Engine
}

func (d *dagEngineWrapper) Stop(ctx context.Context) error {
	return d.Shutdown(ctx)
}

func (d *dagEngineWrapper) HealthCheck(ctx context.Context) (interface{}, error) {
	return map[string]string{"status": "healthy"}, nil
}

func (d *dagEngineWrapper) IsBootstrapped() bool {
	return true
}

// NewPQEngine creates a new post-quantum consensus engine
func NewPQEngine() Engine {
	return pq.New()
}

// Config returns default consensus parameters for different network sizes
func Config(nodes int) config.Parameters {
	switch {
	case nodes == 1:
		// Single validator mode for POA mainnet
		return config.SingleValidatorParams()
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

	// ValidatorState provides validator information
	ValidatorState = consensuscontext.ValidatorState

	// IDs holds consensus IDs
	IDs = consensuscontext.IDs

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
	GetNetworkID       = consensuscontext.GetNetworkID
	GetValidatorState  = consensuscontext.GetValidatorState
	GetSubnetID        = consensuscontext.GetNetID // GetSubnetID is an alias for GetNetID for backward compatibility
	WithContext        = consensuscontext.WithContext
	FromContext        = consensuscontext.FromContext
	GetNodeID          = consensuscontext.GetNodeID
	WithIDs            = consensuscontext.WithIDs
	WithValidatorState = consensuscontext.WithValidatorState
)

// AppError represents an application error
type AppError struct {
	Code    int
	Message string
}
