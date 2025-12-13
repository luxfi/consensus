package handler

import (
	"context"

	"github.com/luxfi/consensus/core/router"
	"github.com/luxfi/ids"
)

// Handler handles network messages
type Handler interface {
	// HandleInbound handles inbound messages
	HandleInbound(context.Context, Message) error

	// HandleOutbound handles outbound messages
	HandleOutbound(context.Context, Message) error

	// Connected handles node connection
	Connected(context.Context, ids.NodeID) error

	// Disconnected handles node disconnection
	Disconnected(context.Context, ids.NodeID) error
}

// Message represents a network message
type Message struct {
	NodeID    ids.NodeID
	RequestID uint32
	Op        Op
	Message   []byte
}

// Op re-exports from core/router for consistency
type Op = router.Op

// Op constants re-exported from core/router
const (
	GetAcceptedFrontier = router.GetAcceptedFrontier
	AcceptedFrontier    = router.AcceptedFrontier
	GetAccepted         = router.GetAccepted
	Accepted            = router.Accepted
	Get                 = router.Get
	Put                 = router.Put
	PushQuery           = router.PushQuery
	PullQuery           = router.PullQuery
	Chits               = router.Chits
)
