package benchlist

import (
	"github.com/luxfi/ids"
	"time"
)

// Benchlist tracks misbehaving nodes
type Benchlist interface {
	IsBenched(nodeID ids.NodeID) bool
	Bench(nodeID ids.NodeID, duration time.Duration)
}
