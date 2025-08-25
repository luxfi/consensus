// Package dag provides DAG consensus functionality
package dag

import (
	"context"
	"github.com/luxfi/ids"
)

// Tx represents a transaction in the DAG
type Tx interface {
	ID() ids.ID
	ParentIDs() []ids.ID
	Bytes() []byte
	Verify(context.Context) error
	Accept(context.Context) error
	Reject(context.Context) error
}