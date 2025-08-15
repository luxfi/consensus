package nova

import (
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/ids"
	"time"
)

// Block represents a Nova block
type Block interface {
	interfaces.Decidable
	Parent() ids.ID
	Height() uint64
	Timestamp() time.Time
	Bytes() []byte
}
