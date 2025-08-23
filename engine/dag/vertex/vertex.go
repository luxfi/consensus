package vertex

import (
	"context"
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/choices"
)

// Vertex represents a vertex in the DAG
type Vertex interface {
	ID() ids.ID
	Bytes() []byte
	Height() uint64
	Epoch() uint32
	Parents() []ids.ID
	Txs() []ids.ID
	Status() choices.Status
	Accept(context.Context) error
	Reject(context.Context) error
	Verify(context.Context) error
}

// DAGVM represents a DAG-based VM
type DAGVM interface {
	ParseVertex(context.Context, []byte) (Vertex, error)
	BuildVertex(context.Context) (Vertex, error)
}

// Manager manages vertices
type Manager interface {
	GetVertex(ids.ID) (Vertex, error)
	AddVertex(Vertex) error
}

// Parser parses vertices
type Parser interface {
	ParseVertex([]byte) (Vertex, error)
}

// Storage stores vertices
type Storage interface {
	GetVertex(ids.ID) (Vertex, error)
	PutVertex(Vertex) error
}
