package ray

import (
	"context"
	"time"

	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/luxfi/consensus/types"
)

// ID is your item identifier (block hash, tx id, etc.).
type ID interface{ comparable }

// Source lets Ray pull next candidates to decide in linear order.
type Source[T ID] interface {
	NextPending(ctx context.Context, n int) []T
}

// Sink receives final decisions in order.
type Sink[T ID] interface {
	Decide(ctx context.Context, items []T, d types.Decision) error
}

// Transport bridges network vote requests <-> Photons the Wave consumes.
type Transport[T ID] interface {
	RequestVotes(ctx context.Context, peers []types.NodeID, item T) <-chan wave.Photon[T]
	MakeLocalPhoton(item T, prefer bool) wave.Photon[T]
}

type Config struct {
	PollSize int
	Alpha    float64
	Beta     uint32
	RoundTO  time.Duration
	MaxBatch int
}

type Driver[T ID] struct {
	wv            wave.Wave[T]
	cut           prism.Cut[T]
	tx            Transport[T]
	src           Source[T]
	out           Sink[T]
	cfg           Config
	height        uint64
	preference    T
	hasPreference bool
}

func NewDriver[T ID](cfg Config, cut prism.Cut[T], tx Transport[T], src Source[T], out Sink[T]) *Driver[T] {
	if cfg.PollSize == 0 {
		cfg.PollSize = 20
	}
	if cfg.Alpha == 0 {
		cfg.Alpha = 0.8
	}
	if cfg.Beta == 0 {
		cfg.Beta = 15
	}
	if cfg.RoundTO == 0 {
		cfg.RoundTO = 250 * time.Millisecond
	}
	if cfg.MaxBatch == 0 {
		cfg.MaxBatch = 64
	}

	return &Driver[T]{
		wv:  wave.New[T](wave.Config{K: cfg.PollSize, Alpha: cfg.Alpha, Beta: cfg.Beta, RoundTO: cfg.RoundTO}, cut, tx),
		cut: cut, tx: tx, src: src, out: out, cfg: cfg,
		height:        0,
		hasPreference: false,
	}
}

// Tick drives one sampling round across up to MaxBatch pending items.
// Any that reach β are emitted to Sink in input order with their decision.
func (d *Driver[T]) Tick(ctx context.Context) error {
	items := d.src.NextPending(ctx, d.cfg.MaxBatch)
	if len(items) == 0 {
		return nil
	}

	var decided []T
	for _, it := range items {
		d.wv.Tick(ctx, it)
		if st, ok := d.wv.State(it); ok && st.Decided {
			if st.Result == types.DecideAccept {
				decided = append(decided, it)
			}
		}
	}
	if len(decided) > 0 {
		if len(decided) > 0 {
			d.preference = decided[0] // Update preference to latest decided
			d.hasPreference = true
			d.height++
		}
		return d.out.Decide(ctx, decided, types.DecideAccept)
	}
	return nil
}

// Start begins the Ray engine operation
func (d *Driver[T]) Start(ctx context.Context) error {
	// Ray engine is stateless and starts immediately
	return nil
}

// Stop ends the Ray engine operation
func (d *Driver[T]) Stop(ctx context.Context) error {
	// Ray engine is stateless and stops immediately
	return nil
}

// Propose proposes an item for consensus (Nova will use this for blocks)
func (d *Driver[T]) Propose(ctx context.Context, item T) error {
	// In Ray, proposals are handled through the Source interface
	// This is a convenience method for direct proposals
	d.wv.Tick(ctx, item)
	return nil
}

// GetPreference returns the current preferred item
func (d *Driver[T]) GetPreference() (T, bool) {
	return d.preference, d.hasPreference
}

// IsFinalized checks if an item is finalized
func (d *Driver[T]) IsFinalized(item T) bool {
	if state, exists := d.wv.State(item); exists {
		return state.Decided && state.Result == types.DecideAccept
	}
	return false
}

// Height returns the current consensus height (for linear chains)
func (d *Driver[T]) Height() uint64 {
	return d.height
}
