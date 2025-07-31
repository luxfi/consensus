// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import (
	"github.com/luxfi/consensus/core/interfaces"
)

// Factory provides consensus creation capabilities
type Factory struct {
	ctx *interfaces.Context
}

// NewFactory creates a new factory
func NewFactory(ctx *interfaces.Context) *Factory {
	return &Factory{ctx: ctx}
}

// Context returns the factory's context
func (f *Factory) Context() *interfaces.Context {
	return f.ctx
}