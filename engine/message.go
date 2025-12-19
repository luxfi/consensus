// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package engine provides consensus engine implementations.
package engine

import "github.com/luxfi/vm"

// Type aliases imported from vm package for consensus internal use
type (
	Message     = vm.Message
	MessageType = vm.MessageType
	Fx          = vm.Fx
	FxLifecycle = vm.FxLifecycle
	State       = vm.State
)

// Re-export constants from vm package
const (
	PendingTxs    = vm.PendingTxs
	StateSyncDone = vm.StateSyncDone
	Unknown       = vm.Unknown
	Starting      = vm.Starting
	Syncing       = vm.Syncing
	Bootstrapping = vm.Bootstrapping
	Ready         = vm.Ready
	Degraded      = vm.Degraded
	Stopping      = vm.Stopping
	Stopped       = vm.Stopped
)
