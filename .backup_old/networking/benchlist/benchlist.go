package benchlist

import (
	"time"

	"github.com/luxfi/ids"
)

// Benchlist tracks misbehaving nodes
type Benchlist interface {
	IsBenched(nodeID ids.NodeID) bool
	Bench(nodeID ids.NodeID, duration time.Duration)
}
