// Copyright (C) 2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"github.com/luxfi/warp"
)

// Sender is the warp Sender interface.
type Sender = warp.Sender

// FakeSender is the warp FakeSender for testing.
type FakeSender = warp.FakeSender

// SenderTest is a test implementation of Sender.
type SenderTest struct {
	FakeSender
}

// Deprecated: Use Sender instead.
type WarpSender = warp.Sender

// Deprecated: Use Sender instead.
type AppSender = warp.Sender
