package photon

import (
	"context"

	"github.com/luxfi/consensus/types"
)

// Sampler is a compatibility alias for Emitter during migration
// Deprecated: Use Emitter instead
type Sampler[T comparable] interface {
	Emitter[T]
	Sample(ctx context.Context, k int, topic types.Topic) []T
}

// SamplerAdapter wraps an Emitter to provide backward compatibility
type SamplerAdapter struct {
	*UniformEmitter
}

// Sample provides backward compatibility with the old Sampler interface
func (s *SamplerAdapter) Sample(ctx context.Context, k int, topic types.Topic) []types.NodeID {
	// Convert topic to seed for deterministic selection
	seed := uint64(0)
	if len(topic) > 0 {
		for _, b := range topic {
			seed = seed*31 + uint64(b)
		}
	}

	nodes, _ := s.Emit(ctx, k, seed)
	return nodes
}

// Report provides feedback on node performance
func (s *SamplerAdapter) Report(node types.NodeID, probe types.Probe) {
	s.UniformEmitter.Report(node, probe == types.ProbeGood)
}

// Allow always returns true for backward compatibility
func (s *SamplerAdapter) Allow(topic types.Topic) bool {
	return true
}

// NewSampler creates a backward-compatible sampler
// Deprecated: Use NewUniformEmitter instead
func NewSampler(peers []types.NodeID, opts EmitterOptions) *SamplerAdapter {
	return &SamplerAdapter{
		UniformEmitter: NewUniformEmitter(peers, opts),
	}
}
