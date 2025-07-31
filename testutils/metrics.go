// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package testutils

import (
	"github.com/luxfi/consensus/core/interfaces"
)

// NoOpRegisterer is a no-op implementation of interfaces.Registerer for testing
type NoOpRegisterer struct{}

// Register is a no-op
func (n *NoOpRegisterer) Register(interface{}) error {
	return nil
}

// NewNoOpRegisterer returns a new no-op registerer
func NewNoOpRegisterer() interfaces.Registerer {
	return &NoOpRegisterer{}
}