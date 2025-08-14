package chain

import (
    "time"
    "github.com/luxfi/ids"
    "github.com/luxfi/consensus/core/interfaces"
)

// Block represents a blockchain block
type Block interface {
    ID() ids.ID
    Parent() ids.ID
    Height() uint64
    Timestamp() time.Time
    Bytes() []byte
    Status() interfaces.Status
    Accept() error
    Reject() error
    Verify() error
}
