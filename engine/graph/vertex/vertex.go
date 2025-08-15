package vertex

import (
	"context"
	"github.com/luxfi/ids"
)

// LinearizableVM defines the interface for linearizable vertex VM
type LinearizableVM interface {
	// Linearize is called after the DAG has been finalized and needs to be
	// linearized into a chain.
	Linearize(ctx context.Context, stopVertexID ids.ID) error
}
