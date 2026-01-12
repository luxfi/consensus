package block

import (
	"context"
	"errors"
	"time"

	"github.com/luxfi/consensus/runtime"
	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/database/manager"
	"github.com/luxfi/ids"
	"github.com/luxfi/p2p"
)

var (
	// ErrRemoteVMNotImplemented is returned when the remote VM is not implemented
	ErrRemoteVMNotImplemented = errors.New("remote VM not implemented")
	// ErrStateSyncableVMNotImplemented is returned when state syncable VM is not implemented
	ErrStateSyncableVMNotImplemented = errors.New("state syncable VM not implemented")
)

// BatchedChainVM extends ChainVM with batch operations
type BatchedChainVM interface {
	ChainVM
	GetAncestors(
		ctx context.Context,
		blkID ids.ID,
		maxBlocksNum int,
		maxBlocksSize int,
		maxBlocksRetrievalTime time.Duration,
	) ([][]byte, error)
	BatchedParseBlock(ctx context.Context, blks [][]byte) ([]Block, error)
}

// BuildBlockWithContextChainVM extends ChainVM with context-aware block building
type BuildBlockWithContextChainVM interface {
	ChainVM
	BuildBlockWithContext(ctx context.Context, blockCtx *Context) (Block, error)
}

// ChainContext provides chain context
type ChainContext struct {
	*runtime.Runtime
}

// DBManager manages databases
type DBManager = manager.Manager

// Re-export engine types
type (
	MessageType = engine.MessageType
	Message     = engine.Message
	Fx          = engine.Fx
)

// Message type constants
const (
	PendingTxs    = engine.PendingTxs
	StateSyncDone = engine.StateSyncDone
)

// AppSender is an alias for p2p.Sender for backwards compatibility
// The node passes a p2p.Sender to the VM via RPC
type AppSender = p2p.Sender
