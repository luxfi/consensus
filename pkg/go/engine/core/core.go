package core

import (
	consensus_core "github.com/luxfi/consensus/core"
	"github.com/luxfi/consensus/engine/core/common"
	"github.com/luxfi/ids"
)

// Fx represents a feature extension
type Fx struct {
	ID ids.ID
	Fx interface{}
}

// AppError represents an application error
type AppError = consensus_core.AppError

// AppHandler handles application messages
type AppHandler = consensus_core.AppHandler

// MessageType represents the type of message
type MessageType = common.MessageType

// Message constants
const (
	PendingTxs = common.PendingTxs
)
