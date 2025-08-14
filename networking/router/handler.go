package router

import (
    "context"
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/ids"
    "github.com/luxfi/node/version"
)

// InboundHandler handles inbound messages
// The message parameter is an interface{} to avoid circular dependencies.
// In practice, it will be message.InboundMessage from the node package.
type InboundHandler interface {
    core.AppHandler
    HandleInbound(context.Context, interface{})
}

// ExternalHandler handles messages from external sources  
type ExternalHandler interface {
    InboundHandler
    
    // Connected is called when a peer connects
    Connected(nodeID ids.NodeID, nodeVersion *version.Application, subnetID ids.ID)
    
    // Disconnected is called when a peer disconnects
    Disconnected(nodeID ids.NodeID)
}