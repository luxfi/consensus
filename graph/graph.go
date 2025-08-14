package graph

import (
	"context"

	"github.com/luxfi/ids"
)

// Tx represents a transaction in the graph
type Tx interface {
	ID() ids.ID
	Verify(context.Context) error
	Accept(context.Context) error
	Reject(context.Context) error
}
