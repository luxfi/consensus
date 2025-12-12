// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"github.com/luxfi/warp"
)

// Sender is the warp Sender interface.
type Sender = warp.Sender

// Handler handles warp messages.
type Handler = warp.Handler

// Error represents a warp error.
type Error = warp.Error

// Deprecated: Use Sender instead.
type WarpSender = warp.Sender

// Deprecated: Use Handler instead.
type WarpHandler = warp.Handler

// Deprecated: Use Error instead.
type WarpError = warp.Error

// Deprecated: Use Sender instead.
type AppSender = warp.Sender

// Deprecated: Use Handler instead.
type AppHandler = warp.Handler

// Deprecated: Use Error instead.
type AppError = warp.Error

// SendConfig configures message sending.
type SendConfig struct {
	NodeIDs       []interface{}
	Validators    int
	NonValidators int
	Peers         int
}
