// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import (
	"context"
)

// Factory provides consensus creation capabilities
type Factory struct {
	ctx context.Context
}

// NewFactory creates a new factory
func NewFactory(ctx context.Context) *Factory {
	return &Factory{ctx: ctx}
}

// Context returns the factory's context
func (f *Factory) Context() context.Context {
	return f.ctx
}