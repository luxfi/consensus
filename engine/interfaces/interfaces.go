// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package interfaces provides consensus engine interfaces.
package interfaces

import "context"

// Engine defines the consensus engine interface
type Engine interface {
	Start(context.Context, uint32) error
	Stop(context.Context) error
	HealthCheck(context.Context) (interface{}, error)
	IsBootstrapped() bool
}
