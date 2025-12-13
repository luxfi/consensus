// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package tracker

import (
	"time"

	"github.com/luxfi/ids"
)

// Tracker tracks system resource usage caused by each peer
type Tracker interface {
	// Usage returns the current usage for a given node at the given time
	Usage(nodeID ids.NodeID, now time.Time) float64
	// TimeUntilUsage returns the duration until the usage drops to the given value
	TimeUntilUsage(nodeID ids.NodeID, now time.Time, value float64) time.Duration
	// TotalUsage returns the total usage across all nodes
	TotalUsage() float64
}

// CPUTracker tracks CPU resource usage per peer
type CPUTracker interface {
	// Usage returns the current CPU usage for a given node at the given time
	Usage(nodeID ids.NodeID, now time.Time) float64
	// TimeUntilUsage returns the duration until CPU usage drops to the given value
	TimeUntilUsage(nodeID ids.NodeID, now time.Time, value float64) time.Duration
}

// DiskTracker tracks disk resource usage per peer
type DiskTracker interface {
	// Usage returns the current disk usage for a given node at the given time
	Usage(nodeID ids.NodeID, now time.Time) float64
	// TimeUntilUsage returns the duration until disk usage drops to the given value
	TimeUntilUsage(nodeID ids.NodeID, now time.Time, value float64) time.Duration
}

// Targeter determines target resource usage thresholds
type Targeter interface {
	// TargetUsage returns the target usage threshold
	TargetUsage() uint64
}

// ResourceTracker manages CPU and disk resource tracking
type ResourceTracker interface {
	// CPUTracker returns the CPU usage tracker
	CPUTracker() CPUTracker
	// DiskTracker returns the disk usage tracker
	DiskTracker() DiskTracker
}
