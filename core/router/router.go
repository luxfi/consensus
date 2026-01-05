package router

import (
	"context"
	"time"

	"github.com/luxfi/ids"
)

// Message represents a network message
type Message interface {
	NodeID() ids.NodeID
	Op() Op
	Get(Field) interface{}
	Bytes() []byte
	BytesSavedCompression() int
	AddRef()
	DecRef()
	IsProposal() bool
}

// Op represents a message operation type
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
	// Vote is the authenticated preference signal returned by polling
	Vote
	// GetContext requests the verification context (parent chain) for a block
	GetContext
	// Context is the response containing prerequisite blocks needed to verify/attach a tip
	Context
)


// Field represents a message field
type Field byte

// InboundHandler handles inbound messages
type InboundHandler interface {
	HandleInbound(context.Context, Message) error
}

// ExternalHandler handles messages from external chains
type ExternalHandler interface {
	Connected(nodeID ids.NodeID, nodeVersion interface{}, subnetID ids.ID)
	Disconnected(nodeID ids.NodeID)
	HandleInbound(context.Context, Message) error
}

// HealthConfig configures health checks
type HealthConfig struct {
	Enabled              bool
	Interval             time.Duration
	Timeout              time.Duration
	MaxOutstandingChecks int
}
