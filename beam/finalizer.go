// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beam

import "github.com/luxfi/consensus/types"

// Block interface for linear chain blocks
type Block interface {
	ID() types.BlockID
	Parent() types.BlockID
}

// Finalizer manages linear chain finalization
type Finalizer interface {
	Ready(head types.BlockID) bool
	Commit(b Block) error
}

// linear implements linear chain finalization
type linear struct{}

// New creates a new Beam finalizer
func New() Finalizer {
	return &linear{}
}

// Ready checks if a block is ready for finalization
func (l *linear) Ready(head types.BlockID) bool {
	return true
}

// Commit finalizes a block
func (l *linear) Commit(b Block) error {
	return nil
}