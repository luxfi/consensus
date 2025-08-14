package tracker

import (
    "time"
    "github.com/luxfi/ids"
)

// Tracker tracks network metrics
type Tracker interface {
    Connected(nodeID ids.NodeID)
    Disconnected(nodeID ids.NodeID)
    Usage(nodeID ids.NodeID, currentTime time.Time) uint64
    TimeUntilUsage(nodeID ids.NodeID, currentTime time.Time, usage uint64) time.Duration
}
