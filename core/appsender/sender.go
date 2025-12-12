// Copyright (C) 2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

// Package appsender provides the AppSender interface for cross-chain communication.
// Deprecated: Use github.com/luxfi/warp instead.
package appsender

import (
	"github.com/luxfi/warp"
)

// Sender is the warp Sender interface.
type Sender = warp.Sender

// Deprecated: Use Sender instead.
type AppSender = warp.Sender
