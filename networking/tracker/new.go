package tracker

import (
	"time"

	"github.com/luxfi/ids"
)

// NewResourceTracker creates a new resource tracker
func NewResourceTracker(
	cpuHalflife time.Duration,
	diskHalflife time.Duration,
) (ResourceTracker, error) {
	return &resourceTracker{
		cpuUsage:  make(map[ids.NodeID]float64),
		diskUsage: make(map[ids.NodeID]float64),
	}, nil
}

type resourceTracker struct {
	cpuUsage  map[ids.NodeID]float64
	diskUsage map[ids.NodeID]float64
}

func (r *resourceTracker) StartProcessing(nodeID ids.NodeID, startTime time.Time) {
	// Track processing start
}

func (r *resourceTracker) StopProcessing(nodeID ids.NodeID, stopTime time.Time) {
	// Track processing stop
}

func (r *resourceTracker) CPUTracker() CPUTracker {
	return &cpuTracker{usage: r.cpuUsage}
}

func (r *resourceTracker) DiskTracker() DiskTracker {
	return &diskTracker{usage: r.diskUsage}
}

type cpuTracker struct {
	usage map[ids.NodeID]float64
}

func (t *cpuTracker) Usage(nodeID ids.NodeID, requestedTime time.Time) float64 {
	if usage, ok := t.usage[nodeID]; ok {
		return usage
	}
	return 0
}

func (t *cpuTracker) TimeUntilUsage(nodeID ids.NodeID, requestedTime time.Time, usage float64) time.Duration {
	return 0
}

func (t *cpuTracker) TotalUsage() float64 {
	var total float64
	for _, usage := range t.usage {
		total += usage
	}
	return total
}

type diskTracker struct {
	usage map[ids.NodeID]float64
}

func (t *diskTracker) Usage(nodeID ids.NodeID, requestedTime time.Time) float64 {
	if usage, ok := t.usage[nodeID]; ok {
		return usage
	}
	return 0
}

func (t *diskTracker) TimeUntilUsage(nodeID ids.NodeID, requestedTime time.Time, usage float64) time.Duration {
	return 0
}

func (t *diskTracker) TotalUsage() float64 {
	var total float64
	for _, usage := range t.usage {
		total += usage
	}
	return total
}
