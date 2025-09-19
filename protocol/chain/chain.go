package chain

import (
	"context"
	"time"

	"github.com/luxfi/ids"
)

// Block represents a blockchain block
type Block interface {
	ID() ids.ID
	Parent() ids.ID // Alias for ParentID for compatibility
	ParentID() ids.ID
	Height() uint64
	Timestamp() time.Time
	Bytes() []byte
	Status() uint8
	Accept(context.Context) error
	Reject(context.Context) error
	Verify(context.Context) error
}

// OracleBlock is a block that only has two valid children. The children should
// be returned in preferential order.
//
// This ordering does not need to be deterministically created from the chain
// state.
type OracleBlock interface {
	Block
	// Options returns the possible children of this block in the order this
	// validator prefers the blocks.
	Options(context.Context) ([2]Block, error)
}
