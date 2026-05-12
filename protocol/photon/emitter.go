package photon

import (
	"crypto/rand"
	"encoding/binary"

	"github.com/luxfi/consensus/core/types"
)

// Emitter emits consensus messages
type Emitter interface {
	// Emit emits a message to selected nodes
	Emit(msg interface{}) ([]types.NodeID, error)

	// EmitTo emits a message to specific nodes
	EmitTo(nodes []types.NodeID, msg interface{}) error
}

// DefaultEmitterOptions returns default emitter options
func DefaultEmitterOptions() EmitterOptions {
	return EmitterOptions{
		K:       20,
		Fanout:  4,
		Timeout: 1000,
	}
}

// EmitterOptions defines emitter options
type EmitterOptions struct {
	K       int // Committee size
	Fanout  int // Number of nodes to emit to
	Timeout int // Timeout in milliseconds
}

// UniformEmitter implements uniform random emission
type UniformEmitter struct {
	nodes   []types.NodeID
	options EmitterOptions
}

// NewUniformEmitter creates a new uniform emitter
func NewUniformEmitter(nodes []types.NodeID, options EmitterOptions) *UniformEmitter {
	return &UniformEmitter{
		nodes:   nodes,
		options: options,
	}
}

// Emit selects a uniform random subset of nodes using Fisher-Yates shuffle
// with crypto/rand (same algorithm as prism.UniformCut.Sample).
func (e *UniformEmitter) Emit(msg interface{}) ([]types.NodeID, error) {
	n := len(e.nodes)
	k := e.options.Fanout
	if k >= n {
		return e.nodes, nil
	}

	// Shuffle a copy so we don't mutate the original slice order.
	shuffled := make([]types.NodeID, n)
	copy(shuffled, e.nodes)

	for i := 0; i < k; i++ {
		j := i + cryptoRandInt(n-i)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	return shuffled[:k], nil
}

// cryptoRandInt returns a cryptographically secure random integer in
// [0, max) — uniformly distributed. Closes BLOCKERS.md CR-13: prior
// implementation used `binary.LittleEndian.Uint64(buf[:]) % uint64(max)`
// which introduces modulo bias for non-power-of-2 max. Under the
// nation-state grinding threat model, that bias was a structural
// exploit on committee sampling (Pinkas-Reiter style).
//
// This implementation uses rejection sampling: read 8 bytes, reject
// values in the high partial bucket, retry until the sample falls in
// the perfectly-uniform range. Expected loops ~1.0–2.0 (never more
// than 2.0 in expectation for any positive max).
func cryptoRandInt(max int) int {
	if max <= 0 {
		return 0
	}
	// Bound to refuse: largest multiple of max <= 2^64. Values above
	// this are biased and must be rejected.
	limit := (^uint64(0) / uint64(max)) * uint64(max)
	var buf [8]byte
	for {
		_, _ = rand.Read(buf[:])
		v := binary.LittleEndian.Uint64(buf[:])
		if v < limit {
			return int(v % uint64(max))
		}
		// Bias-region hit; resample.
	}
}

// EmitTo emits a message to specific nodes
func (e *UniformEmitter) EmitTo(nodes []types.NodeID, msg interface{}) error {
	return nil
}
