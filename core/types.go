// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"github.com/luxfi/warp"
)

// Sender sends warp messages.
type Sender = warp.Sender

// Handler handles warp messages.
type Handler = warp.Handler

// Error represents a warp error.
type Error = warp.Error

// SendConfig configures message sending.
type SendConfig = warp.SendConfig

// FakeSender is a no-op sender for testing.
type FakeSender = warp.FakeSender
