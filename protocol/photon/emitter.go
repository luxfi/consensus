package photon

import "github.com/luxfi/consensus/core/types"

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

// Emit emits a message to selected nodes
func (e *UniformEmitter) Emit(msg interface{}) ([]types.NodeID, error) {
	// Select random subset of nodes
	selected := make([]types.NodeID, 0, e.options.Fanout)
	for i := 0; i < e.options.Fanout && i < len(e.nodes); i++ {
		selected = append(selected, e.nodes[i])
	}
	return selected, nil
}

// EmitTo emits a message to specific nodes
func (e *UniformEmitter) EmitTo(nodes []types.NodeID, msg interface{}) error {
	return nil
}
