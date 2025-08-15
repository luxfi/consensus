package sender

import (
    "github.com/luxfi/ids"
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/consensus/core/interfaces"
    "github.com/luxfi/node/message"
    "github.com/luxfi/node/subnets"
    "github.com/luxfi/node/utils/set"
    "github.com/luxfi/trace"
)

// Sender sends consensus messages
type Sender interface {
    SendGetAcceptedFrontier(nodeID ids.NodeID, requestID uint32)
    SendAcceptedFrontier(nodeID ids.NodeID, requestID uint32, containerIDs []ids.ID)
    SendGetAccepted(nodeID ids.NodeID, requestID uint32, containerIDs []ids.ID)
    SendAccepted(nodeID ids.NodeID, requestID uint32, containerIDs []ids.ID)
}

// ExternalSender sends messages to peers
type ExternalSender interface {
    Send(msg message.OutboundMessage, config core.SendConfig, subnetID ids.ID, allower subnets.Allower) set.Set[ids.NodeID]
}

// New creates a new sender
func New(
    ctx *interfaces.Runtime,
    msgCreator message.OutboundMsgBuilder,
    timeouts interface{},
    engineType interface{},
    sb subnets.Subnet,
) (Sender, error) {
    // Return a no-op sender for now
    return &noOpSender{}, nil
}

// Trace wraps a sender with tracing
func Trace(sender Sender, tracer trace.Tracer) Sender {
    // Just return the original sender for now
    return sender
}

type noOpSender struct{}

func (n *noOpSender) SendGetAcceptedFrontier(nodeID ids.NodeID, requestID uint32) {}
func (n *noOpSender) SendAcceptedFrontier(nodeID ids.NodeID, requestID uint32, containerIDs []ids.ID) {}
func (n *noOpSender) SendGetAccepted(nodeID ids.NodeID, requestID uint32, containerIDs []ids.ID) {}
func (n *noOpSender) SendAccepted(nodeID ids.NodeID, requestID uint32, containerIDs []ids.ID) {}
