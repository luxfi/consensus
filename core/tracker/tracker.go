package tracker

import (
    "github.com/luxfi/ids"
    "github.com/luxfi/consensus/validators"
    metric "github.com/luxfi/metric"
)

// Tracker tracks consensus progress
type Tracker interface {
    IsProcessing(id interface{}) bool
    Add(id interface{})
    Remove(id interface{})
}

// Peers tracks connected validators
type Peers interface {
    validators.SetCallbackListener
    Connected(nodeID ids.NodeID)
    Disconnected(nodeID ids.NodeID)
}

// NewMeteredPeers creates a new metered peers tracker
func NewMeteredPeers(registry metric.Registry) (Peers, error) {
    return &noOpPeers{}, nil
}

// NewPeers creates a new peers tracker
func NewPeers() Peers {
    return &noOpPeers{}
}

// Startup tracks startup progress
type Startup interface {
    OnValidatorAdded(nodeID ids.NodeID)
    OnValidatorRemoved(nodeID ids.NodeID)
}

// NewStartup creates a new startup tracker
func NewStartup(peers Peers, startupAlpha float64) Startup {
    return &noOpStartup{}
}

type noOpStartup struct{}

func (n *noOpStartup) OnValidatorAdded(nodeID ids.NodeID) {}
func (n *noOpStartup) OnValidatorRemoved(nodeID ids.NodeID) {}

type noOpPeers struct{}

func (n *noOpPeers) OnValidatorAdded(nodeID ids.NodeID, weight uint64) {}
func (n *noOpPeers) OnValidatorRemoved(nodeID ids.NodeID, weight uint64) {}
func (n *noOpPeers) OnValidatorWeightChanged(nodeID ids.NodeID, oldWeight, newWeight uint64) {}
func (n *noOpPeers) Connected(nodeID ids.NodeID) {}
func (n *noOpPeers) Disconnected(nodeID ids.NodeID) {}
