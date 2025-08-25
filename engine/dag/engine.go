// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package dag

import (
	"context"

	"github.com/luxfi/ids"
)

// Transaction represents a DAG transaction
type Transaction interface {
	ID() ids.ID
	Parent() ids.ID
	Height() uint64
	Bytes() []byte
	Verify(context.Context) error
	Accept(context.Context) error
	Reject(context.Context) error
}

// Engine defines the DAG consensus engine interface
type Engine interface {
	// GetVtx gets a vertex by ID
	GetVtx(context.Context, ids.ID) (Transaction, error)

	// BuildVtx builds a new vertex
	BuildVtx(context.Context) (Transaction, error)

	// ParseVtx parses a vertex from bytes
	ParseVtx(context.Context, []byte) (Transaction, error)

	// Start starts the engine
	Start(context.Context, uint32) error

	// Shutdown shuts down the engine
	Shutdown(context.Context) error
}

// dagEngine implements the Engine interface
type dagEngine struct{}

// New creates a new DAG engine
func New() Engine {
	return &dagEngine{}
}

func (e *dagEngine) GetVtx(ctx context.Context, id ids.ID) (Transaction, error) {
	return nil, nil
}

func (e *dagEngine) BuildVtx(ctx context.Context) (Transaction, error) {
	return nil, nil
}

func (e *dagEngine) ParseVtx(ctx context.Context, b []byte) (Transaction, error) {
	return nil, nil
}

func (e *dagEngine) Start(ctx context.Context, requestID uint32) error {
	return nil
}

func (e *dagEngine) Shutdown(ctx context.Context) error {
	return nil
}