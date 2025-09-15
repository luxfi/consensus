// Package router is DEPRECATED.
// This package should be in the node repository as it's part of the P2P layer, not consensus.
//
// Migration:
//
//	OLD: import "github.com/luxfi/consensus/networking/router"
//	NEW: import "github.com/luxfi/node/network/router"
package router

import (
	"context"
	"errors"

	"github.com/luxfi/node/message"
)

var ErrDeprecated = errors.New("router package should be in github.com/luxfi/node/network/router")

// Deprecated: Implement in node repository
type Router interface {
	Deprecated()
}

// InboundHandler handles inbound messages
type InboundHandler interface {
	HandleInbound(ctx context.Context, msg message.InboundMessage)
}

// InboundHandlerFunc is a function that implements InboundHandler
type InboundHandlerFunc func(ctx context.Context, msg message.InboundMessage)

// HandleInbound implements InboundHandler
func (f InboundHandlerFunc) HandleInbound(ctx context.Context, msg message.InboundMessage) {
	f(ctx, msg)
}
