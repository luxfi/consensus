package interfaces

import (
	"context"

	"github.com/luxfi/ids"
)

// Decidable represents an item that can be decided
type Decidable interface {
	ID() ids.ID
	Status() Status
	Accept(ctx context.Context) error
	Reject(ctx context.Context) error
}
