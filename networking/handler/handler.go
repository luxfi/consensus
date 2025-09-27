package handler

import (
	"context"

	"github.com/luxfi/ids"
)

// Handler handles network messages
type Handler interface {
	// HandleInbound handles inbound messages
	HandleInbound(context.Context, Message) error

	// HandleOutbound handles outbound messages
	HandleOutbound(context.Context, Message) error

	// Connected handles node connection
	Connected(context.Context, ids.NodeID) error

	// Disconnected handles node disconnection
	Disconnected(context.Context, ids.NodeID) error
}

// Message represents a network message
type Message struct {
	NodeID    ids.NodeID
	RequestID uint32
	Op        Op
	Message   []byte
}

// Op represents an operation
type Op byte

const (
	// GetAcceptedFrontier gets accepted frontier
	GetAcceptedFrontier Op = iota
	// AcceptedFrontier is accepted frontier response
	AcceptedFrontier
	// GetAccepted gets accepted
	GetAccepted
	// Accepted is accepted response
	Accepted
	// Get gets an item
	Get
	// Put puts an item
	Put
	// PushQuery pushes a query
	PushQuery
	// PullQuery pulls a query
	PullQuery
	// Chits is chits response
	Chits
)
