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

// AppError represents an application-level error
type AppError struct {
	Code    int32
	Message string
}

// HealthConfig configures health checks
type HealthConfig struct {
	Enabled              bool
	Interval             time.Duration
	Timeout              time.Duration
	MaxOutstandingChecks int
}

// InboundHandler handles inbound messages
type InboundHandler interface {
	HandleInbound(context.Context, Message) error
}