package core

import (
	consensus_core "github.com/luxfi/consensus/core"
)

// WarpSender is a type alias for the core WarpSender
type WarpSender = consensus_core.WarpSender

// FakeSender is a type alias for the core FakeSender
type FakeSender = consensus_core.FakeSender

// SenderTest is a test implementation of WarpSender
type SenderTest struct {
	FakeSender
}

// SendConfig is a type alias for the core SendConfig
type SendConfig = consensus_core.SendConfig

// Deprecated: Use WarpSender instead
type AppSender = consensus_core.WarpSender
