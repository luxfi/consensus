package prism

import "github.com/luxfi/consensus/core/types"

// Frontier represents a cut/frontier in the DAG partial order
type Frontier struct {
	Height   uint64
	Vertices []types.NodeID
}

// Refractor analyzes light paths through the DAG structure
// to determine optimal ordering and conflict resolution
type Refractor interface {
	// ComputeFrontier returns the current frontier of the DAG
	ComputeFrontier() Frontier

	// RefractPath determines the optimal path through conflicting vertices
	RefractPath(from, to types.NodeID) []types.NodeID

	// Interference checks if two vertices conflict
	Interference(a, b types.NodeID) bool
}
