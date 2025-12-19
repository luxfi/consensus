// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package interfaces provides consensus engine interfaces.
package interfaces

import (
	"context"
	"net/http"

	"github.com/luxfi/vm"
)

// State is an alias to vm.State
type State = vm.State

// Re-export State constants from vm package
const (
	Unknown       = vm.Unknown
	Starting      = vm.Starting
	Syncing       = vm.Syncing
	Bootstrapping = vm.Bootstrapping
	Ready         = vm.Ready
	Degraded      = vm.Degraded
	Stopping      = vm.Stopping
	Stopped       = vm.Stopped
)

// Engine defines the consensus engine interface
type Engine interface {
	Start(context.Context, uint32) error
	Stop(context.Context) error
	HealthCheck(context.Context) (interface{}, error)
	IsBootstrapped() bool
}

// VM defines the common VM interface for consensus engines
type VM interface {
	// Initialize initializes this VM
	Initialize(
		ctx context.Context,
		chainCtx interface{},
		dbMgr interface{},
		genesisBytes []byte,
		upgradeBytes []byte,
		configBytes []byte,
		toEngine interface{},
		fxs []interface{},
		appSender interface{},
	) error

	// Shutdown is called when the node is shutting down
	Shutdown(context.Context) error

	// CreateHandlers returns HTTP handlers for this VM
	CreateHandlers(context.Context) (map[string]http.Handler, error)

	// CreateStaticHandlers returns static HTTP handlers for this VM
	CreateStaticHandlers(context.Context) (map[string]http.Handler, error)

	// HealthCheck returns the health of this VM
	HealthCheck(context.Context) (interface{}, error)

	// SetState sets the state of this VM
	SetState(context.Context, State) error

	// Version returns the version of this VM
	Version(context.Context) (string, error)
}
