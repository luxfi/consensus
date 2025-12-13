// Package tracker is DEPRECATED.
// Resource tracking belongs in the node's network layer (github.com/luxfi/p2p/tracker).
package tracker

import (
	"errors"
	"time"

	"github.com/luxfi/ids"
)

var ErrDeprecated = errors.New("tracker package is deprecated - use github.com/luxfi/p2p/tracker")

type ResourceTracker interface {
	StartProcessing(nodeID ids.NodeID, time time.Time)
	StopProcessing(nodeID ids.NodeID, time time.Time)
	CPUTracker() CPUTracker
	DiskTracker() DiskTracker
}

type CPUTracker interface {
	Usage(nodeID ids.NodeID, time time.Time) float64
	TimeUntilUsage(nodeID ids.NodeID, time time.Time, usage float64) time.Duration
}

type DiskTracker interface {
	Usage(nodeID ids.NodeID, time time.Time) float64
	TimeUntilUsage(nodeID ids.NodeID, time time.Time, usage float64) time.Duration
}

type Tracker interface {
	Usage(nodeID ids.NodeID, time time.Time) float64
	TimeUntilUsage(nodeID ids.NodeID, time time.Time, usage float64) time.Duration
}
