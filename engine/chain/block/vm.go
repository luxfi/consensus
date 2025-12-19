package block

import (
	"context"
	"errors"
	"time"

	consensuscontext "github.com/luxfi/consensus/context"
	"github.com/luxfi/consensus/engine"
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

// AppSender sends application-level messages
type AppSender interface {
	// SendAppRequest sends an application-level request to the given nodes
	SendAppRequest(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, appRequestBytes []byte) error

	// SendAppResponse sends an application-level response to the given node
	SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error

	// SendAppError sends an application-level error to the given node
	SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error

	// SendAppGossip sends an application-level gossip to peers
	SendAppGossip(ctx context.Context, nodeIDs []ids.NodeID, appGossipBytes []byte) error

	// SendAppGossipSpecific sends an application-level gossip to specific peers
	SendAppGossipSpecific(ctx context.Context, nodeIDs []ids.NodeID, appGossipBytes []byte) error
}
