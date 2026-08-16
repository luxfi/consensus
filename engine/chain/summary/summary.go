// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package summary decides which state summary — if any — a node adopts before it
// bootstraps, so a node whose gap is too deep to re-execute has a way back that is
// not a reseed.
//
// Two rounds, both weighed in stake, separated by what each establishes.
//
// DISCOVERY asks the connected beacons which summary each of them holds. Its only
// product is a list of candidate heights. It is not a safety boundary and carries no
// threshold: a wrong or hostile height costs one extra entry in the next request and
// nothing else, so gating it would only add a way to stall.
//
// RATIFICATION asks the WHOLE beacon set which of those heights it holds and sums the
// stake behind each summary id. A summary is adopted only when its stake strictly
// exceeds ⅔ of the stake that answered AND the stake that answered is itself a
// majority of the whole beacon set. Adopting a summary throws away every block below
// it — strictly more consequential than naming a frontier to sync toward — so it
// clears the bar the bootstrapper already demands before it will even name one, never
// a weaker bar. The majority-of-the-whole-set term is what stops an adversary who
// eclipses the heavy honest beacons and lets the light ones through: without it he can
// shrink the denominator until a sliver of real stake looks like a supermajority.
//
// Run returns an Outcome, not a decision about bootstrap. A node that adopts a summary
// still bootstraps the tail above it; a node that adopts nothing bootstraps from where
// it already stands. The only error is "the beacons could not be asked" — a split vote
// is an answer, and retrying an answer does not change it.
package summary

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
	"github.com/luxfi/math"
)

// Outcome is what the round established. The zero value is deliberately meaningless:
// the caller's next move — wait for the VM to finish fetching, or bootstrap from the
// local tip — is decided by exactly this value, so an uninitialized or out-of-range
// Outcome must never read as "a summary was adopted".
type Outcome int

const (
	// OutcomeInvalid is the zero value and is never returned deliberately. It
	// accompanies every error, where the outcome has no meaning.
	OutcomeInvalid Outcome = iota

	// OutcomeAdopted: the VM accepted a ratified summary. The history below it is
	// gone and the VM is fetching the state behind it. The caller waits for the VM to
	// signal that it finished, then bootstraps the tail from the summary's height —
	// which is where the chain now reports its last-accepted block, so the descent
	// spans frontier−H rather than frontier−genesis.
	OutcomeAdopted

	// OutcomeSkipped: nothing was adopted and nothing was destroyed. No beacon
	// answered, no summary cleared the bar, every candidate sat at or below the local
	// tip, or the VM declined the one that won. The caller bootstraps from the local
	// tip exactly as it would on a node that never had state sync at all.
	OutcomeSkipped
)

func (o Outcome) String() string {
	switch o {
	case OutcomeAdopted:
		return "adopted"
	case OutcomeSkipped:
		return "skipped"
	default:
		return "invalid"
	}
}

// Config wires an Adopter. Source and VM are required; Log defaults.
//
// There are no windows or deadlines here. Both live in the Source, where the
// transport that has to honour them lives.
type Config[S Summary] struct {
	Source Source
	VM     VM[S]
	Log    log.Logger
}

// Adopter runs the discovery and ratification rounds and, if the network stands
// behind a summary, hands it to the VM.
type Adopter[S Summary] struct {
	cfg Config[S]
}

// New builds an Adopter.
func New[S Summary](cfg Config[S]) *Adopter[S] {
	if cfg.Log == nil {
		cfg.Log = log.NewNoOpLogger()
	}
	return &Adopter[S]{cfg: cfg}
}

// Run asks the network which summary to adopt and, if one is ratified, adopts it.
//
// It returns an error only when the beacons could not be asked or the VM could not
// answer for itself. Every other ending — nobody answered, nobody agreed, the VM said
// no — is OutcomeSkipped, because the caller's fallback is bootstrap and a node whose
// gap fits the descent window needs nothing more than that.
func (a *Adopter[S]) Run(ctx context.Context) (Outcome, error) {
	tip, err := a.tip(ctx)
	if err != nil {
		return OutcomeInvalid, err
	}

	// A sync cut short by a restart is worth resuming only if the network still stands
	// behind the summary it was fetching, so it enters the round as an ordinary
	// candidate and is preferred only among the summaries that survive ratification.
	// Entering it here is also what lets it be ratified at all: the beacons that
	// happen to answer discovery may not include one whose newest summary is ours.
	resume, haveResume, err := a.resumable(ctx)
	if err != nil {
		return OutcomeInvalid, err
	}

	offers, err := a.cfg.Source.Offers(ctx)
	if err != nil {
		return OutcomeInvalid, fmt.Errorf("summary: the beacons could not be asked what they hold: %w", err)
	}

	candidates := make(map[ids.ID]S, len(offers)+1)
	if haveResume && resume.Height() > tip {
		candidates[resume.ID()] = resume
	}
	for _, offer := range offers {
		s, perr := a.cfg.VM.ParseStateSummary(ctx, offer.Bytes)
		if perr != nil {
			// A beacon that answers with bytes the VM cannot read contributes no
			// candidate, and that is the entire consequence. Discovery has no threshold
			// to fall below, so one unreadable reply cannot end the round for everyone.
			a.cfg.Log.Debug("summary: unreadable reply — dropping the summary, not the round",
				log.Stringer("nodeID", offer.NodeID), log.Err(perr))
			continue
		}
		// Everything at or below the local tip is history this node already has.
		// Refusing it here, rather than trusting the VM to refuse it inside the
		// state-destroying call, is what keeps a majority that happens to be behind
		// from dragging the node backwards.
		if s.Height() <= tip {
			continue
		}
		candidates[s.ID()] = s
	}
	if len(candidates) == 0 {
		a.cfg.Log.Info("summary: no beacon offered a summary above the local tip — bootstrapping from here",
			log.Int("offers", len(offers)), log.Uint64("tip", tip))
		return OutcomeSkipped, nil
	}

	ballots, total, err := a.cfg.Source.Ballots(ctx, heights(candidates))
	if err != nil {
		return OutcomeInvalid, fmt.Errorf("summary: the beacons could not be asked to ratify: %w", err)
	}

	chosen, elected := a.elect(candidates, resume, haveResume, ballots, total)
	if !elected {
		return OutcomeSkipped, nil
	}
	return a.adopt(ctx, chosen)
}

// tip reads the height this node already stands at — the floor under every candidate.
func (a *Adopter[S]) tip(ctx context.Context) (uint64, error) {
	h, err := a.cfg.VM.Tip(ctx)
	if err != nil {
		return 0, fmt.Errorf("summary: reading the local tip failed: %w", err)
	}
	return h, nil
}

// resumable returns the summary an interrupted sync was fetching, or nil when there is
// none. Having no interrupted sync is the ordinary case, not a failure.
func (a *Adopter[S]) resumable(ctx context.Context) (S, bool, error) {
	s, err := a.cfg.VM.GetOngoingSyncStateSummary(ctx)
	var zero S
	switch {
	case errors.Is(err, database.ErrNotFound):
		return zero, false, nil
	case err != nil:
		return zero, false, fmt.Errorf("summary: reading the interrupted sync failed: %w", err)
	}
	return s, true, nil
}

// heights lists the candidate heights, deduplicated and ordered, so the same candidate
// set always produces the same request no matter what order the map yields.
func heights[S Summary](candidates map[ids.ID]S) []uint64 {
	seen := make(map[uint64]struct{}, len(candidates))
	out := make([]uint64, 0, len(candidates))
	for _, s := range candidates {
		h := s.Height()
		if _, dup := seen[h]; dup {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// elect sums the stake behind each candidate and returns the summary the network
// stands behind, or nil when none of them clears the bar.
func (a *Adopter[S]) elect(
	candidates map[ids.ID]S,
	resume S,
	haveResume bool,
	ballots []Ballot,
	total uint64,
) (S, bool) {
	var answered uint64
	stake := make(map[ids.ID]uint64, len(candidates))
	spoke := make(map[ids.NodeID]struct{}, len(ballots))

	for _, b := range ballots {
		if _, again := spoke[b.NodeID]; again {
			// One beacon, one voice. A repeated ballot is a duplicated reply or a beacon
			// buying weight; either way the second one is not stake.
			continue
		}
		spoke[b.NodeID] = struct{}{}

		sum, err := math.Add(answered, b.Weight)
		if err != nil {
			// Real stake cannot overflow a uint64, so this is arithmetic gone wrong
			// upstream. Saturating would turn a fault into an infinitely trusted summary;
			// refusing costs a bootstrap.
			a.cfg.Log.Warn("summary: responder stake overflowed — adopting nothing this round")
			var zero S
			return zero, false
		}
		answered = sum

		named := make(map[ids.ID]struct{}, len(b.Held))
		for _, id := range b.Held {
			if _, known := candidates[id]; !known {
				// Selection ranges over the candidates, so an id nobody offered could
				// never win anyway. Dropping it here keeps a hostile ballot from growing
				// this map with names of its own invention.
				continue
			}
			if _, again := named[id]; again {
				continue // naming a summary twice does not double a beacon's stake
			}
			named[id] = struct{}{}

			// No overflow check: stake[id] only ever sums a subset of the ballots
			// answered already sums, so stake[id] <= answered holds throughout, and
			// answered was checked one line per ballot earlier. A second check here
			// could never fire.
			stake[id] += b.Weight
		}
	}

	// The set that answered must itself be a stake majority of the beacon set before
	// its ⅔ means anything. Otherwise the denominator is whatever the network let
	// through, and an adversary who eclipses the heavy honest beacons while passing
	// enough light ones can shrink it until a sliver of real stake clears a
	// ⅔-of-responders test — for the price of one round, a whole state.
	if answered <= config.HalfStakeFloor(total) {
		a.cfg.Log.Info("summary: too little beacon stake answered to judge a summary — bootstrapping instead",
			log.Uint64("answered", answered), log.Uint64("total", total))
		var zero S
		return zero, false
	}

	// Ranging over the candidates, not over the stake, is what makes "a beacon cannot
	// introduce a candidate during the vote" structural: a summary nobody offered has
	// nothing to be selected, however much stake names it.
	floor := config.TwoThirdsStakeFloor(answered)
	var best S
	haveBest := false
	for id, s := range candidates {
		if stake[id] <= floor {
			continue
		}
		if haveResume && id == resume.ID() {
			// The trie under this one is already partly on disk. Ratified, it beats a
			// taller summary this node would have to start from nothing.
			a.cfg.Log.Info("summary: resuming the interrupted sync — the network still stands behind it",
				log.Stringer("summary", id), log.Uint64("height", s.Height()))
			return s, true
		}
		if !haveBest || taller(s, best) {
			best, haveBest = s, true
		}
	}
	if !haveBest {
		a.cfg.Log.Info("summary: the beacons answered but split below the ⅔ bar — bootstrapping instead",
			log.Uint64("answered", answered), log.Uint64("total", total), log.Int("candidates", len(candidates)))
	}
	return best, haveBest
}

// taller orders two ratified summaries: the higher one wins, and equal heights fall
// back to the id so the choice never depends on map order.
func taller[S Summary](s, than S) bool {
	if h, t := s.Height(), than.Height(); h != t {
		return h > t
	}
	sid, tid := s.ID(), than.ID()
	return sid.Compare(tid) < 0
}

// adopt hands the ratified summary to the VM and reports what the caller should do
// next.
func (a *Adopter[S]) adopt(ctx context.Context, s S) (Outcome, error) {
	mode, err := s.Accept(ctx)
	if err != nil {
		// This is the call that discards local history. If it failed we cannot say how
		// much of the old state survived it, so falling quietly through to bootstrap
		// would re-execute against a tip we can no longer vouch for. Surface it.
		return OutcomeInvalid, fmt.Errorf("summary: the VM failed to adopt the ratified summary at height %d: %w", s.Height(), err)
	}

	switch mode {
	case block.StateSyncSkipped:
		a.cfg.Log.Info("summary: the VM declined the ratified summary — bootstrapping from the local tip",
			log.Stringer("summary", s.ID()), log.Uint64("height", s.Height()))
		return OutcomeSkipped, nil

	case block.StateSyncStatic, block.StateSyncDynamic:
		// Both wait. A VM that means to fetch in the background still leaves the state
		// below the summary incomplete, and bootstrap's first move is to re-execute the
		// blocks above it against exactly that state — those executions fail for reasons
		// that have nothing to do with the network. One path, so there is no second one
		// to rot; a VM that genuinely wants to fetch in the background has to gate its
		// own execution readiness.
		a.cfg.Log.Info("summary: adopted — waiting for the VM to finish fetching the state behind it",
			log.Stringer("summary", s.ID()), log.Uint64("height", s.Height()))
		return OutcomeAdopted, nil

	default:
		// The state is already gone, but we cannot know whether this VM will ever signal
		// that it finished. Bootstrapping is bounded; waiting on a signal that may never
		// come is not.
		a.cfg.Log.Warn("summary: the VM returned a mode this engine does not know — bootstrapping rather than waiting",
			log.Int("mode", int(mode)), log.Stringer("summary", s.ID()))
		return OutcomeSkipped, nil
	}
}
