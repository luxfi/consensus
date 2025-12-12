// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

// Package core provides warp message types for cross-chain communication.
// All types are re-exported from github.com/luxfi/warp for convenience.
package core

import (
	"github.com/luxfi/warp"
)

// Sender sends warp messages with RingTail post-quantum signatures.
type Sender = warp.Sender

// Handler handles incoming warp messages.
type Handler = warp.Handler

// Error represents a warp error.
type Error = warp.Error

// SendConfig configures message sending parameters.
type SendConfig = warp.SendConfig

// FakeSender is a no-op sender for testing.
type FakeSender = warp.FakeSender

// AppSender sends application-level messages between nodes.
type AppSender = warp.AppSender

// AppError represents an application-level error.
type AppError = warp.AppError
