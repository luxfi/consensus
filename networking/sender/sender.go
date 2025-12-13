package sender

import (
	"context"

	"github.com/luxfi/consensus/core/router"
	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
	"github.com/luxfi/warp"
)

// Sender sends messages
type Sender interface {
	// Send sends a message
	Send(context.Context, Message) error

	// SendRequest sends a warp request
	SendRequest(context.Context, set.Set[ids.NodeID], uint32, []byte) error

	// SendResponse sends a warp response
	SendResponse(context.Context, ids.NodeID, uint32, []byte) error

	// SendGossip sends warp gossip
	SendGossip(context.Context, warp.SendConfig, []byte) error
}

// Message represents a message to send
type Message struct {
	NodeIDs   set.Set[ids.NodeID]
	RequestID uint32
	Op        Op
	Bytes     []byte
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
