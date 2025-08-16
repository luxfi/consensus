package block

import (
	"context"
	"time"

	"github.com/luxfi/consensus/protocol/chain"
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
)

// Block extends chain.Block for engine use
type Block interface {
	chain.Block
}

// ChainContext provides chain-specific context
type ChainContext = interfaces.Runtime

// DBManager manages database operations
type DBManager interface {
	Current() database.Database
	Get(version uint64) (database.Database, error)
	Close() error
}

// Message represents engine messages
type Message interface{}

// Fx represents a feature extension
type Fx struct {
	ID ids.ID
}

// AppSender sends application messages
type AppSender interface {
	SendAppRequest(ctx context.Context, nodeID ids.NodeID, requestID uint32, appRequestBytes []byte) error
	SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error
	SendAppGossip(ctx context.Context, appGossipBytes []byte) error
}

// ChainVM defines the VM interface for blockchains
type ChainVM interface {
	Initialize(ctx context.Context, chainCtx *ChainContext, dbManager DBManager, genesisBytes []byte, upgradeBytes []byte, configBytes []byte, toEngine chan<- Message, fxs []*Fx, appSender AppSender) error
	BuildBlock(context.Context) (Block, error)
	ParseBlock(context.Context, []byte) (Block, error)
	GetBlock(context.Context, ids.ID) (Block, error)
	SetPreference(context.Context, ids.ID) error
	LastAccepted(context.Context) (ids.ID, error)
	GetBlockIDAtHeight(context.Context, uint64) (ids.ID, error)
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
	BatchedParseBlock(ctx context.Context, blks [][]byte) ([]Block, error)
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
