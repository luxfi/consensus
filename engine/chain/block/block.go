package block

import (
    "context"
    "time"
    "github.com/luxfi/ids"
    "github.com/luxfi/consensus/chain"
    "github.com/luxfi/consensus/choices"
)

// Block extends chain.Block for engine use
type Block interface {
    chain.Block
}

// ChainVM defines the VM interface for blockchains
type ChainVM interface {
    BuildBlock(context.Context) (Block, error)
    ParseBlock(context.Context, []byte) (Block, error)
    GetBlock(context.Context, ids.ID) (Block, error)
    SetPreference(context.Context, ids.ID) error
    LastAccepted(context.Context) (ids.ID, error)
}

// BuildBlockWithContextChainVM extends ChainVM with context support
type BuildBlockWithContextChainVM interface {
    ChainVM
    BuildBlockWithContext(context.Context, *Context) (Block, error)
}

// Context provides block building context
type Context struct {
    PChainHeight uint64
}

// BatchedChainVM supports batched operations
type BatchedChainVM interface {
    ChainVM
    GetAncestors(ctx context.Context, blkID ids.ID, maxBlocksNum int, maxBlocksSize int, maxBlocksRetrivalTime time.Duration) ([][]byte, error)
}

// StateSyncableVM supports state sync
type StateSyncableVM interface {
    StateSyncEnabled(context.Context) (bool, error)
    GetOngoingSyncStateSummary(context.Context) (StateSummary, error)
    GetLastStateSummary(context.Context) (StateSummary, error)
    ParseStateSummary(context.Context, []byte) (StateSummary, error)
    GetStateSummary(context.Context, uint64) (StateSummary, error)
}

// StateSummary represents a state summary
type StateSummary interface {
    ID() ids.ID
    Height() uint64
    Bytes() []byte
    Accept(context.Context) (StateSyncMode, error)
}

// StateSyncMode defines state sync modes
type StateSyncMode int

const (
    StateSyncSkipped StateSyncMode = iota
    StateSyncStatic
    StateSyncDynamic
)

// WithVerifyContext adds context verification
type WithVerifyContext interface {
    ShouldVerifyWithContext(context.Context) (bool, error)
    VerifyWithContext(context.Context, *Context) error
}

var ErrStateSyncableVMNotImplemented = errNotImplemented{}

type errNotImplemented struct{}
func (errNotImplemented) Error() string { return "state syncable VM not implemented" }
