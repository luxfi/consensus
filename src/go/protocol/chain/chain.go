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
