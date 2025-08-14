package nova

import (
    "time"
    "github.com/luxfi/ids"
    "github.com/luxfi/consensus/core/interfaces"
)

// Block represents a Nova block
type Block interface {
    interfaces.Decidable
    Parent() ids.ID
    Height() uint64
    Timestamp() time.Time
    Bytes() []byte
}
