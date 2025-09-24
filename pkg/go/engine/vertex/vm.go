// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"context"

	"github.com/luxfi/consensus/core"
	"github.com/luxfi/consensus/engine/dag"
	"github.com/luxfi/ids"
)

// DAGVM defines the interface for a DAG-based VM
type DAGVM interface {
	core.VM

	// PendingTxs returns transactions that are pending
	PendingTxs(context.Context) []dag.Transaction

	// ParseTx parses a transaction from bytes
	ParseTx(context.Context, []byte) (dag.Transaction, error)

	// GetTx gets a transaction by ID
	GetTx(context.Context, ids.ID) (dag.Transaction, error)
}

// LinearizableVMWithEngine combines a DAGVM with an Engine
type LinearizableVMWithEngine interface {
	DAGVM

	// GetEngine returns the DAG engine
	GetEngine() dag.Engine
}

// LinearizableVM is a VM that supports linearization
type LinearizableVM interface {
	DAGVM

	// Linearize attempts to linearize the DAG
	Linearize(context.Context, ids.ID, ids.ID) error
}
