// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package interfaces provides consensus engine interfaces.
package interfaces

import (
	"context"
	"net/http"
)

// State represents the operational state of a VM or consensus engine.
type State uint8

// State constants for VM lifecycle
const (
	Unknown       State = iota // Unknown state
	Starting                   // VM is starting up
	Syncing                    // VM is syncing state
	Bootstrapping              // VM is bootstrapping
	Ready                      // VM is ready for normal operation
	Degraded                   // VM is degraded but operational
	Stopping                   // VM is shutting down
	Stopped                    // VM has stopped
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
