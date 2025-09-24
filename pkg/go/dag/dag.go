package dag

import "github.com/luxfi/ids"

// Tx represents a transaction in the DAG
type Tx interface {
	ID() ids.ID
	Bytes() []byte
}

// Vertex represents a vertex in the DAG
type Vertex interface {
	ID() ids.ID
	Bytes() []byte
	Height() uint64
	Txs() []Tx
}
