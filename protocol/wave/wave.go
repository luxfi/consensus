package wave

import (
	"context"
	"time"

	"github.com/luxfi/consensus/core/types"
	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/consensus/protocol/wave/fpc"
)

// Photon represents a vote message in the consensus protocol
type Photon[T comparable] struct {
	Item      T
	Prefer    bool
	Sender    types.NodeID
	Timestamp time.Time
}

// Transport handles network communication for voting
type Transport[T comparable] interface {
	RequestVotes(ctx context.Context, peers []types.NodeID, item T) <-chan Photon[T]
	MakeLocalPhoton(item T, prefer bool) Photon[T]
}

// Config holds configuration for wave consensus
type Config struct {
	K         int           // Sample size
	Alpha     float64       // Threshold ratio (used when FPC disabled)
	Beta      uint32        // Confidence threshold
	RoundTO   time.Duration // Round timeout
	EnableFPC bool          // Enable Fast Probabilistic Consensus
	ThetaMin  float64       // FPC minimum threshold (default: 0.5)
	ThetaMax  float64       // FPC maximum threshold (default: 0.8)
}

// WaveState represents the state of an item in wave consensus
type WaveState struct {
	Decided bool
	Result  types.Decision
	Count   uint32
}

// Wave manages threshold voting and confidence building
type Wave[T comparable] struct {
	cfg Config
	cut prism.Cut[T]
	tx  Transport[T]

	// FPC support
	fpcSelector *fpc.Selector
	phase       uint64 // Current phase for FPC threshold selection

	// State tracking
	states map[T]*WaveState
	prefs  map[T]bool // current preferences
}

// New creates a new Wave instance
func New[T comparable](cfg Config, cut prism.Cut[T], tx Transport[T]) Wave[T] {
	// Initialize FPC selector if enabled
	var fpcSel *fpc.Selector
	if cfg.EnableFPC {
		thetaMin := cfg.ThetaMin
		if thetaMin == 0 {
			thetaMin = 0.5 // Default
		}
		thetaMax := cfg.ThetaMax
		if thetaMax == 0 {
			thetaMax = 0.8 // Default
		}
		fpcSel = fpc.NewSelector(thetaMin, thetaMax, nil)
	}

	return Wave[T]{
		cfg:         cfg,
		cut:         cut,
		tx:          tx,
		fpcSelector: fpcSel,
		phase:       0,
		states:      make(map[T]*WaveState),
		prefs:       make(map[T]bool),
	}
}

// Tick performs one round of sampling and threshold checking for an item
func (w *Wave[T]) Tick(ctx context.Context, item T) {
	// Get current state or create new one
	state, exists := w.states[item]
	if !exists {
		state = &WaveState{Decided: false, Result: types.DecideUndecided, Count: 0}
		w.states[item] = state
	}

	// Skip if already decided
	if state.Decided {
		return
	}

	// Cut light rays (sample peers) and request votes
	peers := w.cut.Sample(w.cfg.K)
	votes := w.tx.RequestVotes(ctx, peers, item)

	// Count votes
	yesVotes := 0
	totalVotes := 0

	// Collect votes with timeout
	timeout := time.After(w.cfg.RoundTO)
	for {
		select {
		case vote := <-votes:
			totalVotes++
			if vote.Prefer {
				yesVotes++
			}
			// Break if we have enough votes
			if totalVotes >= w.cfg.K {
				goto countVotes
			}
		case <-timeout:
			goto countVotes
		case <-ctx.Done():
			return
		}
	}

countVotes:
	if totalVotes == 0 {
		return
	}

	// Increment phase for FPC
	w.phase++

	// Calculate threshold using FPC or fixed Alpha
	var threshold int
	if w.fpcSelector != nil {
		threshold = w.fpcSelector.SelectThreshold(w.phase, w.cfg.K)
	} else {
		threshold = int(float64(w.cfg.K) * w.cfg.Alpha)
	}

	currentPref := w.prefs[item]

	if yesVotes >= threshold {
		// Strong preference for yes
		w.prefs[item] = true
		if currentPref {
			// Consecutive confirmation
			state.Count++
		} else {
			// Preference switch
			state.Count = 1
		}
	} else if (totalVotes - yesVotes) >= threshold {
		// Strong preference for no
		w.prefs[item] = false
		if !currentPref {
			// Consecutive confirmation
			state.Count++
		} else {
			// Preference switch
			state.Count = 1
		}
	} else {
		// No strong preference, reset count
		state.Count = 0
	}

	// Check for decision
	if state.Count >= w.cfg.Beta {
		state.Decided = true
		if w.prefs[item] {
			state.Result = types.DecideAccept
		} else {
			state.Result = types.DecideReject
		}
	}
}

// State returns the current state of an item
func (w *Wave[T]) State(item T) (*WaveState, bool) {
	state, exists := w.states[item]
	return state, exists
}

// Preference returns the current preference for an item
func (w *Wave[T]) Preference(item T) bool {
	return w.prefs[item]
}
