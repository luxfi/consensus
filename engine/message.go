// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package engine provides consensus engine implementations.
package engine

import "github.com/luxfi/consensus/core"

// Type aliases from core package for consensus internal use
type (
	Message     = core.Message
	MessageType = core.MessageType
	Fx          = core.Fx
	FxLifecycle = core.FxLifecycle
	State       = core.State
)

// Re-export constants from core package
const (
	PendingTxs    = core.PendingTxs
	StateSyncDone = core.StateSyncDone
	Unknown       = core.Unknown
	Starting      = core.Starting
	Syncing       = core.Syncing
	Bootstrapping = core.Bootstrapping
	Ready         = core.Ready
	Degraded      = core.Degraded
	Stopping      = core.Stopping
	Stopped       = core.Stopped
)
