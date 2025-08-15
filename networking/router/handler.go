package router

import (
    "context"
    "time"
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

// InboundHandlerFunc is an adapter to allow using ordinary functions as InboundHandlers
type InboundHandlerFunc func(context.Context, interface{})

func (f InboundHandlerFunc) HandleInbound(ctx context.Context, msg interface{}) {
    f(ctx, msg)
}

func (f InboundHandlerFunc) AppRequest(ctx context.Context, nodeID ids.NodeID, requestID uint32, deadline time.Time, appRequestBytes []byte) error {
    return nil
}

func (f InboundHandlerFunc) AppRequestFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32, appErr *core.AppError) error {
    return nil
}

func (f InboundHandlerFunc) AppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error {
    return nil
}

func (f InboundHandlerFunc) AppGossip(ctx context.Context, nodeID ids.NodeID, appGossipBytes []byte) error {
    return nil
}