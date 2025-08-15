package chain

import (
	"context"
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/ids"
	"time"
)

// Block represents a blockchain block
type Block interface {
	ID() ids.ID
	Parent() ids.ID
	Height() uint64
	Timestamp() time.Time
	Bytes() []byte
	Status() interfaces.Status
	Accept(ctx context.Context) error
	Reject(ctx context.Context) error
	Verify(ctx context.Context) error

	// FPC (Fast Path Consensus) methods
	FPCVotes() [][]byte // Embedded fast-path vote references
	EpochBit() bool     // Epoch fence bit for FPC
}
