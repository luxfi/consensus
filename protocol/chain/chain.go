package chain

import (
	"context"
	"github.com/luxfi/ids"
)

// Block represents a blockchain block
type Block interface {
	ID() ids.ID
	ParentID() ids.ID
	Height() uint64
	Timestamp() int64
	Bytes() []byte
	Status() uint8
	Accept(context.Context) error
	Reject(context.Context) error
	Verify(context.Context) error
}
