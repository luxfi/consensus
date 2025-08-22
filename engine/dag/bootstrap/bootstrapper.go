package bootstrap

import (
    "context"
    "github.com/luxfi/ids"
)

// Version represents protocol version
type Version uint32

// Bootstrapper bootstraps a DAG
type Bootstrapper interface {
    // Start starts bootstrapping
    Start(context.Context, uint32) error
    
    // Connected notifies the bootstrapper of a connected validator
    Connected(context.Context, ids.NodeID, Version) error
    
    // Disconnected notifies the bootstrapper of a disconnected validator
    Disconnected(context.Context, ids.NodeID) error
    
    // HealthCheck performs a health check
    HealthCheck(context.Context) (interface{}, error)
}

// bootstrapper implementation
type bootstrapper struct {
    started bool
}

// New creates a new bootstrapper
func New() Bootstrapper {
    return &bootstrapper{}
}

// Start starts bootstrapping
func (b *bootstrapper) Start(ctx context.Context, requestID uint32) error {
    b.started = true
    return nil
}

// Connected notifies the bootstrapper of a connected validator
func (b *bootstrapper) Connected(ctx context.Context, nodeID ids.NodeID, version Version) error {
    return nil
}

// Disconnected notifies the bootstrapper of a disconnected validator
func (b *bootstrapper) Disconnected(ctx context.Context, nodeID ids.NodeID) error {
    return nil
}

// HealthCheck performs a health check
func (b *bootstrapper) HealthCheck(ctx context.Context) (interface{}, error) {
    return map[string]interface{}{"started": b.started}, nil
}