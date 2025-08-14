package tracker

// Targeter provides target configuration
type Targeter interface {
    TargetUsage() uint64
}

// ResourceTracker tracks resource usage
type ResourceTracker interface {
    Tracker
    CPUTracker() Tracker
    DiskTracker() Tracker
}