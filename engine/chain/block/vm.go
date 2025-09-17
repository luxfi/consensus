package block

import (
	"context"
	"errors"
	"time"

	consensus "github.com/luxfi/consensus/context"
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

// ConsensusContext provides consensus context
type ConsensusContext struct {
	ValidatorState consensus.ValidatorState
	Metrics        consensus.Metrics
}

// ChainContext provides chain context
type ChainContext struct {
	*ConsensusContext
	*consensus.Context
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

// AppSender sends application messages
type AppSender interface {
	SendAppRequest(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, appRequestBytes []byte) error
	SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error
	SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error
	SendAppGossip(ctx context.Context, nodeIDs []ids.NodeID, appGossipBytes []byte) error
}
