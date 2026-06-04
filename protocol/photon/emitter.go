package photon

import (
	"crypto/rand"
	"math/big"

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
// [0, max) — uniformly distributed via crypto/rand.Int. Closes
// BLOCKERS.md CR-13: prior implementation used
// `binary.LittleEndian.Uint64(buf[:]) % uint64(max)` which introduces
// modulo bias for non-power-of-2 max. Under the nation-state grinding
// threat model, that bias was a structural exploit on committee
// sampling (Pinkas-Reiter style).
//
// crypto/rand.Int implements constant-time rejection sampling
// internally; we rely on the standard library instead of hand-rolling
// the rejection loop.
func cryptoRandInt(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		// crypto/rand.Reader.Read failing is an unrecoverable runtime
		// condition; biased fallback would defeat CR-13.
		panic("photon: crypto/rand.Int failed: " + err.Error())
	}
	return int(n.Int64())
}

// EmitTo emits a message to specific nodes
func (e *UniformEmitter) EmitTo(nodes []types.NodeID, msg interface{}) error {
	return nil
}
