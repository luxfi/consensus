package ray

import (
    "context"
    "github.com/luxfi/consensus/types"
)

// Nova implements the Nova consensus protocol
type Nova struct {
    nodeID types.NodeID
    round  uint64
}

// New creates a new Nova instance
func New(nodeID types.NodeID) *Nova {
    return &Nova{
        nodeID: nodeID,
        round:  0,
    }
}

// Start starts the Nova protocol
func (n *Nova) Start(ctx context.Context) error {
    return nil
}

// Stop stops the Nova protocol
func (n *Nova) Stop(ctx context.Context) error {
    return nil
}

// Round returns the current round
func (n *Nova) Round() uint64 {
    return n.round
}

// Propose proposes a value
func (n *Nova) Propose(ctx context.Context, value []byte) error {
    n.round++
    return nil
}