// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"context"
	"net/http"

	consensusContext "github.com/luxfi/consensus/context"
	"github.com/luxfi/consensus/interfaces"
	"github.com/luxfi/database/manager"
	"github.com/luxfi/ids"
)

// VM defines the interface that all VMs must implement
type VM interface {
	// Initialize initializes the VM
	Initialize(
		ctx context.Context,
		chainCtx *consensusContext.Context,
		dbManager manager.Manager,
		genesisBytes []byte,
		upgradeBytes []byte,
		configBytes []byte,
		toEngine chan<- Message,
		fxs []*Fx,
		appSender interface{}, // AppSender interface from appsender package
	) error

	// SetState sets the state of the VM
	SetState(ctx context.Context, state interfaces.State) error

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
	case AppRequest:
		return "AppRequest"
	case AppResponse:
		return "AppResponse"
	case AppGossip:
		return "AppGossip"
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
	// AppRequest indicates an app request
	AppRequest
	// AppResponse indicates an app response
	AppResponse
	// AppGossip indicates app gossip
	AppGossip
)

// Fx defines a feature extension
type Fx struct {
	ID ids.ID
	Fx interface{}
}

// Re-export State constants for convenience
const (
	// Bootstrapping means the VM is currently bootstrapping
	Bootstrapping = interfaces.Bootstrapping
	// NormalOp means the VM is operating normally
	NormalOp = interfaces.NormalOp
)

