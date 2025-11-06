package nova

import (
	"context"
	"time"

	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/consensus/protocol/ray"
	"github.com/luxfi/consensus/protocol/wave"
)

// Nova implements linear blockchain consensus using the internal Ray engine
type Nova[T comparable] struct {
	rayEngine *ray.Driver[T]
	config    Config
}

// Config holds configuration for Nova consensus mode
type Config struct {
	SampleSize  int           // k parameter for sampling
	Alpha       float64       // threshold ratio
	Beta        uint32        // confidence threshold
	RoundTO     time.Duration // round timeout
	GenesisHash [32]byte      // genesis block hash
}

// NewNova creates a new Nova instance with Ray engine
func NewNova[T comparable](cfg Config, cut prism.Cut[T], tx wave.Transport[T], source ray.Source[T], sink ray.Sink[T]) *Nova[T] {
	rayConfig := ray.Config{
		PollSize: cfg.SampleSize,
		Alpha:    cfg.Alpha,
		Beta:     cfg.Beta,
		RoundTO:  cfg.RoundTO,
	}

	return &Nova[T]{
		rayEngine: ray.NewDriver(rayConfig, cut, tx, source, sink),
		config:    cfg,
	}
}

// Start begins Nova consensus operation
func (n *Nova[T]) Start(ctx context.Context) error {
	return n.rayEngine.Start(ctx)
}

// Stop ends Nova consensus operation
func (n *Nova[T]) Stop(ctx context.Context) error {
	return n.rayEngine.Stop(ctx)
}

// ProposeBlock proposes a new block for the current height
func (n *Nova[T]) ProposeBlock(ctx context.Context, block T) error {
	return n.rayEngine.Propose(ctx, block)
}

// Tick performs one consensus round for linear chain progression
func (n *Nova[T]) Tick(ctx context.Context) error {
	return n.rayEngine.Tick(ctx)
}

// GetPreference returns the current preferred block
func (n *Nova[T]) GetPreference() (T, bool) {
	return n.rayEngine.GetPreference()
}

// IsFinalized checks if a block is finalized
func (n *Nova[T]) IsFinalized(block T) bool {
	return n.rayEngine.IsFinalized(block)
}

// Height returns the current blockchain height
func (n *Nova[T]) Height() uint64 {
	return n.rayEngine.Height()
}
