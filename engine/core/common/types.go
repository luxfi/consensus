package common

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/warp"
)

// Fx represents a feature extension
type Fx struct {
	ID ids.ID
	Fx interface{}
}

// Sender sends warp messages.
type Sender = warp.Sender

// Handler handles warp messages.
type Handler = warp.Handler

// AppSender sends application-level messages between nodes.
type AppSender = warp.AppSender

// AppError represents an application-level error.
type AppError = warp.AppError

// SendConfig configures message sending.
type SendConfig = warp.SendConfig
