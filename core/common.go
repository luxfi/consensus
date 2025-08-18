// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"context"
	"errors"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/core/appsender"
)

// Type aliases for convenience
type AppSender = appsender.AppSender

// Common errors
var (
	ErrUnknown = errors.New("unknown error")
)

// Message is a message that can be sent between consensus engines
type Message struct {
	// NodeID is the ID of the node that sent the message
	NodeID ids.NodeID
	// Op is the operation type
	Op Op
	// Get contains get request details
	Get *Get
	// GetFailed contains get failure details
	GetFailed *GetFailed
	// Put contains put request details
	Put *Put
	// PushQuery contains push query details
	PushQuery *PushQuery
	// PullQuery contains pull query details
	PullQuery *PullQuery
	// Chits contains chits details
	Chits *Chits
}

// Op represents a consensus operation type
type Op byte

const (
	// GetOp requests a container
	GetOp Op = iota
	// GetFailedOp indicates a get request failed
	GetFailedOp
	// PutOp provides a container
	PutOp
	// PushQueryOp sends a push query
	PushQueryOp
	// PullQueryOp sends a pull query
	PullQueryOp
	// ChitsOp sends chits (votes)
	ChitsOp
	// QueryFailedOp indicates a query failed
	QueryFailedOp
	// ConnectedOp indicates a peer connected
	ConnectedOp
	// DisconnectedOp indicates a peer disconnected
	DisconnectedOp
	// NotifyOp is a notification
	NotifyOp
	// GossipOp is for gossip messages
	GossipOp
	// TimeoutOp indicates a timeout
	TimeoutOp
)

// Get requests a container
type Get struct {
	RequestID   uint32
	ContainerID ids.ID
}

// GetFailed indicates a get request failed
type GetFailed struct {
	RequestID uint32
}

// Put provides a container
type Put struct {
	RequestID uint32
	Container []byte
}

// PushQuery sends a push query
type PushQuery struct {
	RequestID       uint32
	Container       []byte
	RequestedHeight uint64
}

// PullQuery sends a pull query
type PullQuery struct {
	RequestID       uint32
	ContainerID     ids.ID
	RequestedHeight uint64
}

// Chits sends votes
type Chits struct {
	RequestID           uint32
	PreferredID         ids.ID
	PreferredIDAtHeight ids.ID
	AcceptedID          ids.ID
}

// AllGetsServer defines the interface for handling get requests
type AllGetsServer interface {
	GetBlock(ctx context.Context, nodeID ids.NodeID, requestID uint32, blockID ids.ID) error
	GetStateSummaryFrontier(ctx context.Context, nodeID ids.NodeID, requestID uint32) error
	GetAcceptedStateSummary(ctx context.Context, nodeID ids.NodeID, requestID uint32, heights []uint64) error
	GetAcceptedFrontier(ctx context.Context, nodeID ids.NodeID, requestID uint32) error
	GetAccepted(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerIDs []ids.ID) error
	GetAncestors(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerID ids.ID) error
	Get(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerID ids.ID) error
	GetFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32) error
	AppRequest(ctx context.Context, nodeID ids.NodeID, requestID uint32, deadline int64, request []byte) error
}

// Engine defines the consensus engine interface
type Engine interface {
	// Start the engine
	Start(ctx context.Context, startReqID uint32) error

	// IsBootstrapped returns true if the engine is bootstrapped
	IsBootstrapped() (bool, error)

	// Timeout handles timeouts
	Timeout(ctx context.Context) error

	// Gossip handles gossip messages
	Gossip(ctx context.Context) error

	// Halt stops the engine
	Halt(ctx context.Context)

	// Shutdown the engine
	Shutdown(ctx context.Context) error

	// HealthCheck returns the health status
	HealthCheck(ctx context.Context) (interface{}, error)

	// Connected handles peer connections
	Connected(ctx context.Context, nodeID ids.NodeID, nodeVersion *version) error

	// Disconnected handles peer disconnections
	Disconnected(ctx context.Context, nodeID ids.NodeID) error
}

// version represents node version information
type version struct {
	Name  string
	Major int
	Minor int
	Patch int
}

// Sender sends consensus messages
type Sender interface {
	SendGetAncestors(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerID ids.ID) error
	SendGet(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerID ids.ID) error
	SendPut(ctx context.Context, nodeID ids.NodeID, requestID uint32, container []byte) error
	SendPushQuery(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, container []byte, requestedHeight uint64) error
	SendPullQuery(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, containerID ids.ID, requestedHeight uint64) error
	SendChits(ctx context.Context, nodeID ids.NodeID, requestID uint32, preferredID ids.ID, preferredIDAtHeight ids.ID, acceptedID ids.ID) error
	SendGossip(ctx context.Context, container []byte) error
}
