// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"github.com/luxfi/warp"
)

// WarpSender is the primary interface for warp messaging
type WarpSender = warp.Sender

// WarpHandler handles warp messages
type WarpHandler = warp.Handler

// WarpError represents a warp error
type WarpError = warp.Error

// Deprecated: Use WarpSender instead
type AppSender = warp.Sender

// Deprecated: Use WarpHandler instead
type AppHandler = warp.Handler

// Deprecated: Use WarpError instead
type AppError = warp.Error

// SendConfig configures message sending
type SendConfig struct {
	NodeIDs       []interface{}
	Validators    int
	NonValidators int
	Peers         int
}
