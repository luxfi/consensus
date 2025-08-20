package photon

import (
	"context"
	"sync"
	"time"

	"github.com/luxfi/consensus/types"
)

// Emitter chooses a K-sized committee from available nodes.
// The emission pattern can be uniform, weighted by stake, or optimized by latency.
type Emitter[T comparable] interface {
	// Emit returns K nodes/items for consensus participation
	Emit(ctx context.Context, k int, seed uint64) ([]T, error)
	// Report provides feedback on node performance
	Report(node T, success bool)
}

// EmitterOptions configures emission behavior
type EmitterOptions struct {
	MinPeers int
	MaxPeers int
	Stake    func(types.NodeID) float64
	Latency  func(types.NodeID) time.Duration
}

// UniformEmitter implements weighted K-of-N selection
type UniformEmitter struct {
	mu        sync.RWMutex
	peers     []types.NodeID
	opts      EmitterOptions
	luminance *Luminance // Track node brightness (lux)
}

// NewUniformEmitter creates a new uniform emitter with optional weighting
func NewUniformEmitter(peers []types.NodeID, opts EmitterOptions) *UniformEmitter {
	if opts.MinPeers == 0 {
		opts.MinPeers = 8
	}
	if opts.MaxPeers == 0 {
		opts.MaxPeers = 64
	}
	return &UniformEmitter{
		peers:     peers,
		opts:      opts,
		luminance: newLuminance(),
	}
}

// Emit selects K nodes using weighted selection based on stake, latency, and health
func (e *UniformEmitter) Emit(ctx context.Context, k int, seed uint64) ([]types.NodeID, error) {
	_ = ctx  // Context could be used for cancellation
	_ = seed // Seed could be used for deterministic randomness

	e.mu.RLock()
	defer e.mu.RUnlock()

	if k <= 0 {
		k = e.opts.MinPeers
	}
	if k > e.opts.MaxPeers {
		k = e.opts.MaxPeers
	}

	// Score each peer based on stake, latency, and brightness (lux)
	type photon struct {
		id         types.NodeID
		brightness float64 // Emission intensity in lux
	}

	candidates := make([]photon, 0, len(e.peers))
	for _, id := range e.peers {
		brightness := 1.0

		// Apply stake weighting if configured
		if e.opts.Stake != nil {
			brightness *= e.opts.Stake(id)
		}

		// Apply latency penalty if configured
		if e.opts.Latency != nil {
			if lat := e.opts.Latency(id); lat > 0 {
				brightness *= 1.0 / (1.0 + float64(lat.Milliseconds()))
			}
		}

		// Apply luminance (more votes = brighter emission)
		brightness *= e.luminance.brightness(id)

		candidates = append(candidates, photon{id, brightness})
	}

	// Select top K nodes by brightness (highest lux values)
	emitted := make([]types.NodeID, 0, k)
	for i := 0; i < k && len(candidates) > 0; i++ {
		brightest := 0
		for j := 1; j < len(candidates); j++ {
			if candidates[j].brightness > candidates[brightest].brightness {
				brightest = j
			}
		}
		emitted = append(emitted, candidates[brightest].id)
		// Dim selected node to avoid re-selection
		candidates[brightest].brightness *= 0.5
	}

	return emitted, nil
}

// Report updates luminance (brightness) tracking for a node
// Successful votes increase lux, failures dim the node
func (e *UniformEmitter) Report(node types.NodeID, success bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.luminance.illuminate(node, success)
}

// DefaultEmitterOptions returns standard emitter configuration
func DefaultEmitterOptions() EmitterOptions {
	return EmitterOptions{
		MinPeers: 8,
		MaxPeers: 64,
	}
}
