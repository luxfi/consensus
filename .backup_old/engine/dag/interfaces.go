package dag

import (
	"context"

	"github.com/luxfi/consensus/protocol/choices"
	"github.com/luxfi/ids"
)

// Tx represents a transaction in the DAG
type Tx interface {
	// ID returns the unique identifier of the transaction
	ID() ids.ID

	// Bytes returns the byte representation of the transaction
	Bytes() []byte

	// Status returns the current status of the transaction
	Status() choices.Status

	// Verify verifies the validity of the transaction
	Verify(ctx context.Context) error

	// Accept marks the transaction as accepted
	Accept(ctx context.Context) error

	// Reject marks the transaction as rejected
	Reject(ctx context.Context) error
}
