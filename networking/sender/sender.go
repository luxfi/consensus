package sender

import (
    "github.com/luxfi/ids"
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/node/message"
    "github.com/luxfi/node/subnets"
    "github.com/luxfi/node/utils/set"
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
