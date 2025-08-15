package graph

import (
	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/ids"
)

// Tx represents a transaction in the DAG
type Tx interface {
	ID() ids.ID
	Bytes() []byte
	Status() choices.Status
	Accept() error
	Reject() error
	Verify() error
}
