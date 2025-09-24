package timeout

import (
	"context"
	"time"

	"github.com/luxfi/ids"
)

// Manager manages timeouts
type Manager interface {
	// RegisterTimeout registers a timeout
	RegisterTimeout(time.Duration) func(context.Context, ids.ID) error

	// RegisterRequest registers a request
	RegisterRequest(ids.NodeID, ids.ID, bool, uint32, func())

	// RegisterResponse registers a response
	RegisterResponse(ids.NodeID, ids.ID, uint32, Op) (bool, func())

	// TimeoutDuration returns timeout duration
	TimeoutDuration() time.Duration
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

// manager implementation
type manager struct {
	duration time.Duration
}

// NewManager creates a new timeout manager
func NewManager(duration time.Duration) Manager {
	return &manager{
		duration: duration,
	}
}

// RegisterTimeout registers a timeout
func (m *manager) RegisterTimeout(duration time.Duration) func(context.Context, ids.ID) error {
	return func(ctx context.Context, id ids.ID) error {
		return nil
	}
}

// RegisterRequest registers a request
func (m *manager) RegisterRequest(nodeID ids.NodeID, requestID ids.ID, critical bool, uniqueRequestID uint32, callback func()) {
	// Implementation
}

// RegisterResponse registers a response
func (m *manager) RegisterResponse(nodeID ids.NodeID, requestID ids.ID, uniqueRequestID uint32, op Op) (bool, func()) {
	return false, func() {}
}

// TimeoutDuration returns timeout duration
func (m *manager) TimeoutDuration() time.Duration {
	return m.duration
}
