package nebula

import (
	"context"
	"time"
	
	"github.com/luxfi/consensus/protocol/field"
	"github.com/luxfi/consensus/prism"
	"github.com/luxfi/consensus/wave"
)

// VID represents a vertex identifier in the DAG
type VID interface{ comparable }

// Nebula implements DAG consensus using the internal Field engine
type Nebula[V VID] struct {
	fieldEngine *field.Driver[V]
	config      Config
}

// Config holds configuration for Nebula consensus mode
type Config struct {
	PollSize   int           // sample size for voting
	Alpha      float64       // threshold ratio
	Beta       uint32        // confidence threshold
	RoundTO    time.Duration // round timeout
	GenesisSet []byte        // genesis vertex set
}

// NewNebula creates a new Nebula instance with Field engine
func NewNebula[V VID](cfg Config, cut prism.Cut[V], tx wave.Transport[V], store field.Store[V], prop field.Proposer[V], com field.Committer[V]) *Nebula[V] {
	fieldConfig := field.Config{
		PollSize: cfg.PollSize,
		Alpha:    cfg.Alpha,
		Beta:     cfg.Beta,
		RoundTO:  cfg.RoundTO,
	}
	
	return &Nebula[V]{
		fieldEngine: field.NewDriver(fieldConfig, cut, tx, store, prop, com),
		config:      cfg,
	}
}

// Start begins Nebula DAG consensus operation  
func (n *Nebula[V]) Start(ctx context.Context) error {
	return n.fieldEngine.Start(ctx)
}

// Stop ends Nebula DAG consensus operation
func (n *Nebula[V]) Stop(ctx context.Context) error {
	return n.fieldEngine.Stop(ctx)
}

// ProposeVertex proposes a new vertex to the DAG
func (n *Nebula[V]) ProposeVertex(ctx context.Context, parents []V) (V, error) {
	return n.fieldEngine.Propose(ctx, parents)
}

// Tick performs one consensus round for DAG progression
func (n *Nebula[V]) Tick(ctx context.Context) error {
	return n.fieldEngine.Tick(ctx)
}

// OnObserve should be called when observing new vertices from the network
func (n *Nebula[V]) OnObserve(ctx context.Context, vertex V) {
	n.fieldEngine.OnObserve(ctx, vertex)
}

// GetFrontier returns the current DAG frontier (tips)
func (n *Nebula[V]) GetFrontier() []V {
	return n.fieldEngine.GetFrontier()
}

// IsFinalized checks if a vertex is finalized in the DAG
func (n *Nebula[V]) IsFinalized(vertex V) bool {
	return n.fieldEngine.IsFinalized(vertex)
}

// GetCommittedVertices returns vertices that have been committed in order
func (n *Nebula[V]) GetCommittedVertices() []V {
	return n.fieldEngine.GetCommittedVertices()
}