package tracker

import (
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/node/utils/resource"
	"github.com/prometheus/client_golang/prometheus"
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

// NewResourceTracker creates a new resource tracker
func NewResourceTracker(
	reg prometheus.Registerer,
	manager resource.Manager,
	halflife time.Duration,
) (ResourceTracker, error) {
	return &resourceTracker{
		cpu:  &basicTracker{},
		disk: &diskTracker{basicTracker: &basicTracker{}},
	}, nil
}

type resourceTracker struct {
	cpu  Tracker
	disk DiskTracker
}

func (r *resourceTracker) CPUTracker() Tracker                   { return r.cpu }
func (r *resourceTracker) DiskTracker() DiskTracker              { return r.disk }
func (r *resourceTracker) StartProcessing(ids.NodeID, time.Time) {}
func (r *resourceTracker) StopProcessing(ids.NodeID, time.Time)  {}

type basicTracker struct{}

func (t *basicTracker) UtilizationTarget() float64                                  { return 0.8 }
func (t *basicTracker) CurrentUsage() uint64                                        { return 0 }
func (t *basicTracker) TotalUsage() float64                                         { return 0 }
func (t *basicTracker) Usage(ids.NodeID, time.Time) float64                         { return 0 }
func (t *basicTracker) TimeUntilUsage(ids.NodeID, time.Time, float64) time.Duration { return 0 }

type diskTracker struct {
	*basicTracker
}

func (d *diskTracker) AvailableDiskBytes() uint64 { return 1 << 30 } // 1GB

// TargeterConfig configures a targeter
type TargeterConfig struct {
	VdrAlloc           float64
	MaxNonVdrUsage     float64
	MaxNonVdrNodeUsage float64
}

// NewTargeter creates a new targeter
func NewTargeter(
	logger interface{}, // Accepting interface{} to avoid circular dependency
	config *TargeterConfig,
	validators interface{}, // validators.Manager
	tracker Tracker,
) Targeter {
	return &targeter{targetUsage: uint64(config.MaxNonVdrUsage)}
}

type targeter struct {
	targetUsage uint64
}

func (t *targeter) TargetUsage() uint64 { return t.targetUsage }
