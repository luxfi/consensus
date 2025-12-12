package block

import (
	"context"
	"errors"
	"time"

	consensuscontext "github.com/luxfi/consensus/context"
	"github.com/luxfi/database/manager"
	"github.com/luxfi/ids"
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
	*consensuscontext.Context
}

// DBManager manages databases
type DBManager = manager.Manager

// Message represents a message to the VM
type Message struct {
	Type uint32
	Data []byte
}

// Fx represents a feature extension
type Fx struct {
	ID ids.ID
	Fx interface{}
}
