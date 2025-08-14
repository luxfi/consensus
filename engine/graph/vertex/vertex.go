package vertex

import "github.com/luxfi/ids"

// Vertex represents a DAG vertex
type Vertex interface {
    ID() ids.ID
    Parents() []ids.ID
    Height() uint64
    Bytes() []byte
}