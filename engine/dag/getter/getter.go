package getter

import (
    "context"
    "github.com/luxfi/ids"
)

// Getter gets vertices
type Getter interface {
    // Get gets a vertex
    Get(context.Context, ids.NodeID, uint32, ids.ID) error
    
    // GetAncestors gets ancestors
    GetAncestors(context.Context, ids.NodeID, uint32, ids.ID, int) error
    
    // Put puts a vertex
    Put(context.Context, ids.NodeID, uint32, []byte) error
    
    // PushQuery pushes a query
    PushQuery(context.Context, ids.NodeID, uint32, []byte) error
    
    // PullQuery pulls a query
    PullQuery(context.Context, ids.NodeID, uint32, ids.ID) error
}

// getter implementation
type getter struct{}

// New creates a new getter
func New() Getter {
    return &getter{}
}

// Get gets a vertex
func (g *getter) Get(ctx context.Context, nodeID ids.NodeID, requestID uint32, vertexID ids.ID) error {
    return nil
}

// GetAncestors gets ancestors
func (g *getter) GetAncestors(ctx context.Context, nodeID ids.NodeID, requestID uint32, vertexID ids.ID, maxVertices int) error {
    return nil
}

// Put puts a vertex
func (g *getter) Put(ctx context.Context, nodeID ids.NodeID, requestID uint32, vertex []byte) error {
    return nil
}

// PushQuery pushes a query
func (g *getter) PushQuery(ctx context.Context, nodeID ids.NodeID, requestID uint32, vertex []byte) error {
    return nil
}

// PullQuery pulls a query
func (g *getter) PullQuery(ctx context.Context, nodeID ids.NodeID, requestID uint32, vertexID ids.ID) error {
    return nil
}