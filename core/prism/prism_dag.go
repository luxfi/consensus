package prism

import "github.com/luxfi/consensus/types"

// Cut represents a frontier in the DAG partial order
type Cut struct {
    Height   uint64
    Frontier []types.NodeID
}

// Refract analyzes light paths through the DAG structure
// to determine optimal ordering and conflict resolution
type Refractor interface {
    // ComputeCut returns the current frontier of the DAG
    ComputeCut() Cut
    
    // RefractPath determines the optimal path through conflicting vertices
    RefractPath(from, to types.NodeID) []types.NodeID
    
    // Interference checks if two vertices conflict
    Interference(a, b types.NodeID) bool
}