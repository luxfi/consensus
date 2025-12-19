package interfaces

import (
	"context"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
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
	ChainID   ids.ID
	NetworkID uint32
	NodeID    ids.NodeID
	PublicKey []byte
}

// AppError represents an application-level error
type AppError struct {
	Code    int32
	Message string
}

// Error implements error interface
func (e *AppError) Error() string {
	return e.Message
}

// AppSender sends application-level messages.
type AppSender interface {
	// SendAppRequest sends an application-level request to the given nodes.
	SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, appRequestBytes []byte) error

	// SendAppResponse sends an application-level response to the given node.
	SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error

	// SendAppError sends an application-level error to the given node.
	SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error

	// SendAppGossip sends an application-level gossip to all connected nodes.
	SendAppGossip(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error

	// SendAppGossipSpecific sends an application-level gossip to specific nodes.
	SendAppGossipSpecific(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error
}

// AppHandler handles application-level messages from nodes.
type AppHandler interface {
	// AppRequest handles an application-level request from a node.
	AppRequest(ctx context.Context, nodeID ids.NodeID, requestID uint32, deadline time.Time, request []byte) error

	// AppRequestFailed handles a failed application-level request.
	AppRequestFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32, appErr *AppError) error

	// AppResponse handles an application-level response from a node.
	AppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error

	// AppGossip handles an application-level gossip from a node.
	AppGossip(ctx context.Context, nodeID ids.NodeID, msg []byte) error
}

// FxInterface is a feature extension interface
type FxInterface interface {
	// Initialize initializes the Fx
	Initialize(vm interface{}) error
	// Bootstrapping is called when the chain is bootstrapping
	Bootstrapping() error
	// Bootstrapped is called when the chain is done bootstrapping
	Bootstrapped() error
}

// Fx is a feature extension wrapper with ID
type Fx struct {
	ID ids.ID
	Fx FxInterface
}
