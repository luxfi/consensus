package tracker

import (
	"github.com/luxfi/ids"
)

// Tracker tracks consensus state
type Tracker interface {
	// IsBootstrapped checks if bootstrapped
	IsBootstrapped() bool

	// Bootstrapped marks as bootstrapped
	Bootstrapped(ids.ID)

	// OnBootstrapCompleted called when bootstrap completes
	OnBootstrapCompleted() chan ids.ID
}

// tracker implementation
type tracker struct {
	bootstrapped bool
	completed    chan ids.ID
}

// NewTracker creates a new tracker
func NewTracker() Tracker {
	return &tracker{
		completed: make(chan ids.ID, 1),
	}
}

// IsBootstrapped checks if bootstrapped
func (t *tracker) IsBootstrapped() bool {
	return t.bootstrapped
}

// Bootstrapped marks as bootstrapped
func (t *tracker) Bootstrapped(chainID ids.ID) {
	t.bootstrapped = true
	select {
	case t.completed <- chainID:
	default:
	}
}

// OnBootstrapCompleted returns completion channel
func (t *tracker) OnBootstrapCompleted() chan ids.ID {
	return t.completed
}
