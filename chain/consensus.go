package chain

import (
    "github.com/luxfi/trace"
)

// Consensus defines the consensus algorithm
type Consensus interface {
    // Initialize consensus
    Initialize() error
}

// Topological is a topological consensus implementation
type Topological struct{}

func (t *Topological) Initialize() error {
    return nil
}

// Trace wraps a consensus with tracing
func Trace(consensus Consensus, tracer trace.Tracer) Consensus {
    // Just return the original consensus for now
    return consensus
}