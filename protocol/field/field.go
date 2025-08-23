package field

import (
	"context"
	"time"

	"github.com/luxfi/consensus/prism"
	"github.com/luxfi/consensus/wave"
	"github.com/luxfi/consensus/types"
)

type VID interface{ comparable } // vertex id

// BlockView is enough for order-theory (parents + metadata).
type BlockView[V VID] interface {
	ID() V
	Parents() []V
	Author() types.NodeID
	Round() uint64
}

// Store exposes local DAG state for cert/skip scans and causal reads.
type Store[V VID] interface {
	Head() []V
	Get(V) (BlockView[V], bool)
	Children(V) []V
}

// Proposer builds a new vertex from frontier + sidecar votes, etc.
type Proposer[V VID] interface {
	Propose(ctx context.Context, parents []V) (V, error)
}

// Committer applies an ordered prefix decided by Field.
type Committer[V VID] interface {
	Commit(ctx context.Context, ordered []V) error
}

type Config struct {
	PollSize int
	Alpha    float64
	Beta     uint32
	RoundTO  time.Duration
}

type Driver[V VID] struct {
	cfg  Config
	wv   wave.Wave[V]
	cut  prism.Cut[V]
	str  Store[V]
	prop Proposer[V]
	com  Committer[V]
}

func NewDriver[V VID](cfg Config, cut prism.Cut[V], tx wave.Transport[V], store Store[V], prop Proposer[V], com Committer[V]) *Driver[V] {
	if cfg.PollSize == 0 { cfg.PollSize = 20 }
	if cfg.Alpha == 0 { cfg.Alpha = 0.8 }
	if cfg.Beta == 0 { cfg.Beta = 15 }
	if cfg.RoundTO == 0 { cfg.RoundTO = 250 * time.Millisecond }

	return &Driver[V]{
		cfg: cfg,
		wv:  wave.New[V](wave.Config{K: cfg.PollSize, Alpha: cfg.Alpha, Beta: cfg.Beta, RoundTO: cfg.RoundTO}, cut, tx),
		cut: cut, str: store, prop: prop, com: com,
	}
}

// OnObserve should be called by your networking layer as new vertices arrive.
// You can also plug DAG fast-path voting (flare) here if you embed it in vertex payloads.
func (d *Driver[V]) OnObserve(ctx context.Context, v V) {
	// optional: run local checks, update sampler health, etc.
	_ = ctx; _ = v
}

// Tick runs one poll round over DAG heads, looks for cert/skip and commits the safe prefix.
func (d *Driver[V]) Tick(ctx context.Context) error {
	frontier := d.str.Head()
	if len(frontier) == 0 { return nil }

	// Drive thresholding on frontier candidates
	for _, v := range frontier {
		d.wv.Tick(ctx, v)
	}

	// Use DAG order theory to derive commit/skip (cert/skip patterns)
	// TODO: Implement proper type conversion or interface alignment
	ordered := []V{} // dag.ComputeSafePrefix(d.str, frontier) // implement using horizon+flare
	if len(ordered) > 0 {
		if err := d.com.Commit(ctx, ordered); err != nil { return err }
	}

	// Optionally propose a new vertex extending the frontier
	// TODO: Implement proper type conversion or interface alignment
	// parents := dag.ChooseFrontier(frontier) // e.g., 2f+1 parents or all-recent
	// _, _ = d.prop.Propose(ctx, parents)

	return nil
}

// Start begins the Field engine operation
func (d *Driver[V]) Start(ctx context.Context) error {
	// Field engine is stateless and starts immediately
	return nil
}

// Stop ends the Field engine operation
func (d *Driver[V]) Stop(ctx context.Context) error {
	// Field engine is stateless and stops immediately
	return nil
}

// Propose proposes a new vertex with given parents (Nebula will use this)
func (d *Driver[V]) Propose(ctx context.Context, parents []V) (V, error) {
	return d.prop.Propose(ctx, parents)
}

// GetFrontier returns the current DAG frontier (tips)
func (d *Driver[V]) GetFrontier() []V {
	return d.str.Head()
}

// IsFinalized checks if a vertex is finalized
func (d *Driver[V]) IsFinalized(vertex V) bool {
	if state, exists := d.wv.State(vertex); exists {
		return state.Decided && state.Result == types.DecideAccept
	}
	return false
}

// GetCommittedVertices returns vertices that have been committed in order
func (d *Driver[V]) GetCommittedVertices() []V {
	// TODO: Implement committed vertex tracking
	// This would require maintaining a list of committed vertices from the Committer
	return []V{}
}