package tracker

import (
    "time"
    "github.com/luxfi/ids"
)

// Targeter provides target configuration
type Targeter interface {
    TargetUsage() uint64
}

// DiskTracker tracks disk usage
type DiskTracker interface {
    Tracker
    AvailableDiskBytes() uint64
}

// ResourceTracker tracks resource usage
type ResourceTracker interface {
    CPUTracker() Tracker
    DiskTracker() DiskTracker
    // Registers that the given node started processing at the given time.
    StartProcessing(ids.NodeID, time.Time)
    // Registers that the given node stopped processing at the given time.
    StopProcessing(ids.NodeID, time.Time)
}