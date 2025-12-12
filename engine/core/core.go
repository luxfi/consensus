package core

import (
	"github.com/luxfi/consensus/engine/core/common"
	"github.com/luxfi/ids"
	"github.com/luxfi/warp"
)

// Fx represents a feature extension
type Fx struct {
	ID ids.ID
	Fx interface{}
}

// Sender sends warp messages with RingTail post-quantum signatures.
type Sender = warp.Sender

// Handler handles incoming warp messages.
type Handler = warp.Handler

// Error represents a warp error.
type Error = warp.Error

// SendConfig configures message sending parameters.
type SendConfig = warp.SendConfig

// MessageType represents the type of message
type MessageType = common.MessageType

// Message constants
const (
	PendingTxs = common.PendingTxs
)
