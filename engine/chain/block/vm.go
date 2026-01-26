package block

import (
	"context"
	"errors"
	"time"

	"github.com/luxfi/database/manager"
	"github.com/luxfi/ids"
	"github.com/luxfi/p2p"
	"github.com/luxfi/runtime"
)

var (
	// ErrRemoteVMNotImplemented is returned when the remote VM is not implemented
	ErrRemoteVMNotImplemented = errors.New("remote VM not implemented")
	// ErrStateSyncableVMNotImplemented is returned when state syncable VM is not implemented
	ErrStateSyncableVMNotImplemented = errors.New("state syncable VM not implemented")
)

// MessageType represents the type of VM message
type MessageType uint8

// Message type constants
const (
	PendingTxs MessageType = iota + 1
	StateSyncDone
)

// String returns the string representation of the message type
func (m MessageType) String() string {
	switch m {
	case PendingTxs:
		return "PendingTxs"
	case StateSyncDone:
		return "StateSyncDone"
	default:
		return "Unknown"
	}
}

// Message is sent from the VM to the consensus engine
type Message struct {
	Type    MessageType
	NodeID  ids.NodeID
	Content []byte
}

// Fx defines a feature extension to a VM
type Fx struct {
	ID ids.ID
	Fx interface{}
}

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

// AppSender is an alias for p2p.Sender for backwards compatibility
// The node passes a p2p.Sender to the VM via RPC
type AppSender = p2p.Sender
