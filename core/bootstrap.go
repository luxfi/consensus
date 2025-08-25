// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

// BootstrapTracker tracks the progress of bootstrapping
type BootstrapTracker interface {
	// OnBootstrapStarted is called when bootstrapping starts
	OnBootstrapStarted() error
	// OnBootstrapCompleted is called when bootstrapping completes
	OnBootstrapCompleted() error
	// IsBootstrapped returns whether the node has finished bootstrapping
	IsBootstrapped() bool
}