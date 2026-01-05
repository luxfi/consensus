package timeout

import (
	"context"
	"time"

	"github.com/luxfi/consensus/core/router"
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

// Op re-exports from core/router for consistency
type Op = router.Op

// Op constants re-exported from core/router
const (
	GetAcceptedFrontier = router.GetAcceptedFrontier
	AcceptedFrontier    = router.AcceptedFrontier
	GetAccepted         = router.GetAccepted
	Accepted            = router.Accepted
	Get                 = router.Get
	Put                 = router.Put
	PushQuery           = router.PushQuery
	PullQuery           = router.PullQuery
	Vote                = router.Vote
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
