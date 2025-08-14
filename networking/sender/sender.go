package sender

import "github.com/luxfi/ids"

// Sender sends consensus messages
type Sender interface {
    SendGetAcceptedFrontier(nodeID ids.NodeID, requestID uint32)
    SendAcceptedFrontier(nodeID ids.NodeID, requestID uint32, containerIDs []ids.ID)
    SendGetAccepted(nodeID ids.NodeID, requestID uint32, containerIDs []ids.ID)
    SendAccepted(nodeID ids.NodeID, requestID uint32, containerIDs []ids.ID)
}
