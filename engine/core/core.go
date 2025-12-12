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

// AppSender sends application messages
type AppSender = consensus_core.AppSender

// AppHandler handles application messages
type AppHandler = consensus_core.AppHandler

// SendConfig configures message sending.
type SendConfig = consensus_core.SendConfig

// MessageType represents the type of message
type MessageType = common.MessageType

// Message constants
const (
	PendingTxs = common.PendingTxs
)
