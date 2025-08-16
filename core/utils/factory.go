// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import (
	"github.com/luxfi/consensus/core/interfaces"
)

// Factory provides consensus creation capabilities
type Factory struct {
	ctx *interfaces.Runtime
}

// NewFactory creates a new factory
func NewFactory(ctx *interfaces.Runtime) *Factory {
	return &Factory{ctx: ctx}
}

// Context returns the factory's context
func (f *Factory) Context() *interfaces.Runtime {
	return f.ctx
}
