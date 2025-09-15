package sender

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
)

// Sender sends messages
type Sender interface {
	// Send sends a message
	Send(context.Context, Message) error

	// SendAppRequest sends an app request
	SendAppRequest(context.Context, set.Set[ids.NodeID], uint32, []byte) error

	// SendAppResponse sends an app response
	SendAppResponse(context.Context, ids.NodeID, uint32, []byte) error

	// SendAppGossip sends app gossip
	SendAppGossip(context.Context, set.Set[ids.NodeID], []byte) error
}

// Message represents a message to send
type Message struct {
	NodeIDs   set.Set[ids.NodeID]
	RequestID uint32
	Op        Op
	Bytes     []byte
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
