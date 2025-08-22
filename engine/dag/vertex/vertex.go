package vertex

import (
    "context"
    "github.com/luxfi/ids"
)

// Vertex is a vertex in the DAG
type Vertex interface {
    ID() ids.ID
    ParentIDs() []ids.ID
    Height() uint64
    Epoch() uint32
    Bytes() []byte
    Verify(context.Context) error
    Accept(context.Context) error
    Reject(context.Context) error
}

// LinearizableVM defines a linearizable VM
type LinearizableVM interface {
    // ParseVertex parses a vertex
    ParseVertex(context.Context, []byte) (Vertex, error)
    
    // BuildVertex builds a vertex
    BuildVertex(context.Context) (Vertex, error)
    
    // GetVertex gets a vertex
    GetVertex(context.Context, ids.ID) (Vertex, error)
}

// Storage stores vertices
type Storage interface {
    // GetVertex gets a vertex
    GetVertex(ids.ID) (Vertex, error)
    
    // StoreVertex stores a vertex
    StoreVertex(Vertex) error
}

// Manager manages vertices
type Manager interface {
    Storage
    
    // ParseVertex parses a vertex
    ParseVertex(context.Context, []byte) (Vertex, error)
    
    // BuildVertex builds a vertex
    BuildVertex(context.Context) (Vertex, error)
}

// Builder builds vertices
type Builder interface {
    // BuildVertex builds a vertex
    BuildVertex(context.Context) (Vertex, error)
    
    // BuildStopVertex builds a stop vertex
    BuildStopVertex(context.Context) (Vertex, error)
}

// Parser parses vertices
type Parser interface {
    // ParseVertex parses a vertex from bytes
    ParseVertex([]byte) (Vertex, error)
}

// DAGVM defines a DAG VM
type DAGVM interface {
    LinearizableVM
    
    // PendingTxs returns pending transactions
    PendingTxs(context.Context) []ids.ID
}
