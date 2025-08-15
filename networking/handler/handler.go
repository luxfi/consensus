package handler

import (
    "time"
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/consensus/core/interfaces"
    "github.com/luxfi/consensus/validators"
    "github.com/luxfi/consensus/engine/core/tracker"
    "github.com/luxfi/node/subnets"
    "github.com/luxfi/metric"
)

// Handler handles consensus messages
type Handler interface {
	HandleMessage(msg core.Message) error
}

// New creates a new handler
func New(
    ctx *interfaces.Runtime,
    cn interface{}, // changeNotifier
    subscription interface{},
    vdrs validators.Manager,
    frontierPollFrequency time.Duration,
    appConcurrency int,
    resourceTracker interface{},
    sb subnets.Subnet,
    connectedValidators tracker.Peers,
    peerTracker interface{},
    registerer metric.Registry,
) (Handler, error) {
    return &noOpHandler{}, nil
}

type noOpHandler struct{}

func (n *noOpHandler) HandleMessage(msg core.Message) error {
    return nil
}
