// Package interfaces provides common consensus interfaces
package interfaces

// StateHolder holds state information
type StateHolder interface {
	// GetState returns the current state
	GetState() State
}

// State represents consensus state
type State uint8

const (
	// StateSyncing indicates the node is syncing state
	StateSyncing State = iota
	// Bootstrapping indicates the node is bootstrapping
	Bootstrapping
	// NormalOp indicates normal operation
	NormalOp
)
