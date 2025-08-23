package ray

import (
    "context"
    "github.com/luxfi/ids"
)

// Chain defines a blockchain
type Chain interface {
    // GetBlock gets a block
    GetBlock(context.Context, ids.ID) (Block, error)
    
    // AddBlock adds a block
    AddBlock(Block) error
    
    // LastAccepted returns last accepted block
    LastAccepted() ids.ID
    
    // GetAncestor gets an ancestor
    GetAncestor(ids.ID, uint64) (ids.ID, error)
}

// Block defines a block
type Block interface {
    ID() ids.ID
    ParentID() ids.ID
    Height() uint64
    Bytes() []byte
    Verify(context.Context) error
    Accept(context.Context) error
    Reject(context.Context) error
}

// VM defines a chain VM
type VM interface {
    // ParseBlock parses a block
    ParseBlock(context.Context, []byte) (Block, error)
    
    // BuildBlock builds a block
    BuildBlock(context.Context) (Block, error)
    
    // GetBlock gets a block
    GetBlock(context.Context, ids.ID) (Block, error)
    
    // SetPreference sets preferred block
    SetPreference(context.Context, ids.ID) error
    
    // LastAccepted returns last accepted block ID
    LastAccepted(context.Context) (ids.ID, error)
}