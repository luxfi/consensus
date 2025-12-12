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

// Error represents a warp error.
type Error = warp.Error

// Handler handles warp messages.
type Handler = warp.Handler

// MessageType represents the type of message
type MessageType = common.MessageType

// Message constants
const (
	PendingTxs = common.PendingTxs
)
