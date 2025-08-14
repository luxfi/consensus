// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import "github.com/luxfi/ids"

// BootstrapTracker tracks the bootstrapping state of chains
type BootstrapTracker interface {
	// IsBootstrapped returns true if the chains in this subnet are done bootstrapping
	IsBootstrapped() bool

	// Bootstrapped marks the chains in this subnet as done bootstrapping
	Bootstrapped(chainID ids.ID)

	// OnBootstrapCompleted registers a callback to be called when bootstrapping completes
	OnBootstrapCompleted(func()) chan struct{}
}