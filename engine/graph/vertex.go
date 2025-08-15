package graph

import (
	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/ids"
)

// Vertex represents a vertex in the DAG
type Vertex interface {
	ID() ids.ID
	Bytes() []byte
	Status() choices.Status
	Parents() []ids.ID
	Txs() []Tx
	Accept() error
	Reject() error
	Verify() error
}
