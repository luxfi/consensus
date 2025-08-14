package router

import (
    "context"
    "github.com/luxfi/ids"
)

// InboundMessage represents an inbound message
type InboundMessage interface {
    Op() interface{}
    OnFinishedHandling()
}

// Router routes messages between chains
type Router interface {
    AddChain(chainID ids.ID, handler interface{})
    RemoveChain(chainID ids.ID)
    HandleInbound(ctx context.Context, msg InboundMessage)
}
