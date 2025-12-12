package router

import (
	"context"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/warp"
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

// Error is an alias for warp.Error
type Error = warp.Error

// HealthConfig configures health checks
type HealthConfig struct {
	Enabled              bool
	Interval             time.Duration
	Timeout              time.Duration
	MaxOutstandingChecks int
}
