package router

import (
    "context"
    "github.com/luxfi/consensus/engine/core"
)

// InboundHandler handles inbound messages
// The message parameter is an interface{} to avoid circular dependencies.
// In practice, it will be message.InboundMessage from the node package.
type InboundHandler interface {
    core.AppHandler
    HandleInbound(context.Context, interface{})
}