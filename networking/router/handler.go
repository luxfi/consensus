package router

import (
    "context"
    "github.com/luxfi/ids"
    "github.com/luxfi/consensus/engine/core"
)

// InboundHandler handles inbound messages
type InboundHandler interface {
    core.AppHandler
    HandleInbound(context.Context, ids.NodeID, []byte) error
}