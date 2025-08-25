// Package tracker is DEPRECATED.
// Resource tracking belongs in the node's network layer.
//
// Migration:
//
//	OLD: import "github.com/luxfi/consensus/networking/tracker"
//	NEW: import "github.com/luxfi/node/network/tracker"
package tracker

import (
	"errors"
	"time"
	
	"github.com/luxfi/ids"
)

var ErrDeprecated = errors.New("tracker package should be in github.com/luxfi/node/network/tracker")

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
