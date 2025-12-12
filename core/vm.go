// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"context"
	"fmt"
	"net/http"

	consensuscontext "github.com/luxfi/consensus/context"
	"github.com/luxfi/database/manager"
	"github.com/luxfi/ids"
	"github.com/luxfi/warp"
)

// VMState represents the state of a VM
type VMState uint8

const (
	// VMInitializing is the state of a VM that is initializing
	VMInitializing VMState = iota
	// VMStateSyncing is the state of a VM that is syncing state
	VMStateSyncing
	// VMBootstrapping is the state of a VM that is bootstrapping
	VMBootstrapping
	// VMNormalOp is the state of a VM that is in normal operation
	VMNormalOp
)

// VM defines the interface that all VMs must implement
type VM interface {
	// Initialize initializes the VM
	Initialize(
		ctx context.Context,
		chainCtx *consensuscontext.Context,
		dbManager manager.Manager,
		genesisBytes []byte,
		upgradeBytes []byte,
		configBytes []byte,
		toEngine chan<- Message,
		fxs []*Fx,
		warpSender interface{}, // WarpSender interface from warp package
	) error

	// SetState sets the state of the VM
	SetState(ctx context.Context, state VMState) error

	// Shutdown shuts down the VM
	Shutdown(ctx context.Context) error

	// Version returns the version of the VM
	Version(ctx context.Context) (string, error)

	// HealthCheck returns nil if the VM is healthy
	HealthCheck(ctx context.Context) (interface{}, error)

	// CreateHandlers returns the HTTP handlers for the VM
	CreateHandlers(ctx context.Context) (map[string]http.Handler, error)

	// CreateStaticHandlers returns the static HTTP handlers for the VM
	CreateStaticHandlers(ctx context.Context) (map[string]http.Handler, error)

	// NewHTTPHandler returns a new HTTP handler for the VM
	NewHTTPHandler(ctx context.Context) (http.Handler, error)
}

// Message defines a message that can be sent to the consensus engine
type Message struct {
	Type    MessageType
	NodeID  ids.NodeID
	Content []byte
}

// MessageType defines the type of a message
type MessageType uint32

// String returns the string representation of the message type
func (m MessageType) String() string {
	switch m {
	case PendingTxs:
		return "PendingTxs"
	case PutBlock:
		return "PutBlock"
	case GetBlock:
		return "GetBlock"
	case GetAccepted:
		return "GetAccepted"
	case Accepted:
		return "Accepted"
	case GetAncestors:
		return "GetAncestors"
	case MultiPut:
		return "MultiPut"
	case GetFailed:
		return "GetFailed"
	case QueryFailed:
		return "QueryFailed"
	case Chits:
		return "Chits"
	case ChitsV2:
		return "ChitsV2"
	case GetAcceptedFrontier:
		return "GetAcceptedFrontier"
	case AcceptedFrontier:
		return "AcceptedFrontier"
	case GetAcceptedFrontierFailed:
		return "GetAcceptedFrontierFailed"
	case WarpRequest:
		return "WarpRequest"
	case WarpResponse:
		return "WarpResponse"
	case WarpGossip:
		return "WarpGossip"
	default:
		return "Unknown"
	}
}

const (
	// PendingTxs indicates pending transactions
	PendingTxs MessageType = iota
	// PutBlock indicates a block to be added
	PutBlock
	// GetBlock indicates a request for a block
	GetBlock
	// GetAccepted indicates a request for accepted blocks
	GetAccepted
	// Accepted indicates an accepted block
	Accepted
	// GetAncestors indicates a request for ancestors
	GetAncestors
	// MultiPut indicates multiple blocks
	MultiPut
	// GetFailed indicates a failed get request
	GetFailed
	// QueryFailed indicates a failed query
	QueryFailed
	// Chits indicates chits
	Chits
	// ChitsV2 indicates chits v2
	ChitsV2
	// GetAcceptedFrontier indicates a request for the accepted frontier
	GetAcceptedFrontier
	// AcceptedFrontier indicates the accepted frontier
	AcceptedFrontier
	// GetAcceptedFrontierFailed indicates a failed frontier request
	GetAcceptedFrontierFailed
	// WarpRequest indicates a warp request
	WarpRequest
	// WarpResponse indicates a warp response
	WarpResponse
	// WarpGossip indicates warp gossip
	WarpGossip
)

// Fx defines a feature extension
type Fx struct {
	ID ids.ID
	Fx interface{}
}

// AppError represents an application-level error for peer messaging
type AppError struct {
	Code    int32
	Message string
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("app error %d: %s", e.Code, e.Message)
}

// SendConfig is an alias for warp.SendConfig for backwards compatibility
type SendConfig = warp.SendConfig

// Sender is an alias for warp.Sender for backwards compatibility
type Sender = warp.Sender

// Handler is an alias for warp.Handler for backwards compatibility
type Handler = warp.Handler

// Error is an alias for warp.Error for backwards compatibility
type Error = warp.Error
