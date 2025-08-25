// Package vertex provides DAG vertex functionality
package vertex

import (
	"context"
	"github.com/luxfi/consensus/engine/chain"
	"github.com/luxfi/ids"
)

// Vertex represents a vertex in the DAG
type Vertex interface {
	ID() ids.ID
	ParentIDs() []ids.ID
	Height() uint64
	Epoch() uint32
	Bytes() []byte
	Verify(context.Context) error
	Accept(context.Context) error
	Reject(context.Context) error
}

// LinearizableVM provides linearizable VM operations
type LinearizableVM interface {
	// Linearize linearizes vertices
	Linearize(context.Context, []ids.ID) error
}

// LinearizableVMWithEngine extends LinearizableVM with engine support
type LinearizableVMWithEngine interface {
	LinearizableVM
	// GetEngine returns the chain engine
	GetEngine() chain.Engine
}