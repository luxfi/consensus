package interfaces

import (
    "context"
    "github.com/luxfi/ids"
)

// Engine defines the consensus engine interface
type Engine interface {
    // Start starts the engine
    Start(context.Context, uint32) error
    
    // Stop stops the engine
    Stop(context.Context) error
    
    // HealthCheck performs health check
    HealthCheck(context.Context) (interface{}, error)
    
    // IsBootstrapped checks if bootstrapped
    IsBootstrapped() bool
}

// VM defines a virtual machine
type VM interface {
    // Initialize initializes the VM
    Initialize(context.Context, *VMConfig) error
    
    // Shutdown shuts down the VM
    Shutdown(context.Context) error
    
    // Version returns VM version
    Version(context.Context) (string, error)
}

// VMConfig defines VM configuration
type VMConfig struct {
    ChainID     ids.ID
    NetworkID   uint32
    NodeID      ids.NodeID
    PublicKey   []byte
}