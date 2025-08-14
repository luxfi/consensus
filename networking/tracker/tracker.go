package tracker

import (
    "time"
    "github.com/luxfi/ids"
)

// Tracker tracks network metrics
type Tracker interface {
    // Returns the current usage for the given node.
    Usage(nodeID ids.NodeID, now time.Time) float64
    // Returns the current usage by all nodes.
    TotalUsage() float64
    // Returns the duration between [now] and when the usage of [nodeID] reaches
    // [value], assuming that the node uses no more resources.
    // If the node's usage isn't known, or is already <= [value], returns the
    // zero duration.
    TimeUntilUsage(nodeID ids.NodeID, now time.Time, value float64) time.Duration
}
