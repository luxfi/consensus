package interfaces

import (
	"context"

	"github.com/luxfi/ids"
)

// Engine defines the consensus engine interface
type Engine interface {
	Start(context.Context, uint32) error
	Stop(context.Context) error
	HealthCheck(context.Context) (interface{}, error)
	IsBootstrapped() bool
}

// VM defines a virtual machine
type VM interface {
	Initialize(context.Context, *VMConfig) error
	Shutdown(context.Context) error
	Version(context.Context) (string, error)
}

// VMConfig defines VM configuration
type VMConfig struct {
	ChainID   ids.ID
	NetworkID uint32
	NodeID    ids.NodeID
	PublicKey []byte
}

// Fx is a feature extension
type Fx interface {
	Initialize(vm interface{}) error
	Bootstrapping() error
	Bootstrapped() error
}
