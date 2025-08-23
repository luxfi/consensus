package chain

import (
	"context"
	"github.com/luxfi/ids"
)

// Engine defines the chain consensus engine
type Engine interface {
	// Start starts the engine
	Start(context.Context, uint32) error

	// Stop stops the engine
	Stop(context.Context) error

	// HealthCheck performs a health check
	HealthCheck(context.Context) (interface{}, error)

	// IsBootstrapped returns whether the chain is bootstrapped
	IsBootstrapped() bool
}

// Transitive implements transitive chain consensus
type Transitive struct {
	bootstrapped bool
}

// Transport handles message transport for consensus
type Transport[ID comparable] interface {
	// Send sends a message
	Send(ctx context.Context, to string, msg interface{}) error

	// Receive receives messages
	Receive(ctx context.Context) (interface{}, error)
}

// New creates a new chain consensus engine
func New() *Transitive {
	return &Transitive{
		bootstrapped: false,
	}
}

// Start starts the engine
func (t *Transitive) Start(ctx context.Context, requestID uint32) error {
	t.bootstrapped = true
	return nil
}

// Stop stops the engine
func (t *Transitive) Stop(ctx context.Context) error {
	return nil
}

// HealthCheck performs a health check
func (t *Transitive) HealthCheck(ctx context.Context) (interface{}, error) {
	return map[string]interface{}{"healthy": true}, nil
}

// IsBootstrapped returns whether the chain is bootstrapped
func (t *Transitive) IsBootstrapped() bool {
	return t.bootstrapped
}

// GetBlock gets a block by ID
func (t *Transitive) GetBlock(ctx context.Context, nodeID ids.NodeID, requestID uint32, blockID ids.ID) error {
	return nil
}
