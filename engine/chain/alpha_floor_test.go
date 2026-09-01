// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// alpha_floor_test.go — the accept predicate can never be satisfied by nothing.
//
// The engine accepts a block once `acceptVotes() >= c.alpha` (topological.go). With
// α=0 that predicate holds over an EMPTY accept set, and both decision predicates go
// off on nothing:
//
//   - ProcessVote: ANY arriving vote reaches the accept clause, so a single vote
//     AGAINST a block — leaving acceptVotes() == 0 — marks it accepted. The block ends
//     up flagged accepted AND rejected at once, and DrainAccepted hands the VM a block
//     that no validator voted for.
//   - Poll: `rejectVotes() >= alpha` also holds on an empty set, so every fresh block
//     is rejected on zero reject votes and dropped from the tips.
//
// Two partitioned nodes can therefore settle different blocks at the same height with
// no quorum and no cert. α=0 was reachable because Config.Validate existed but had no
// callers, and Parameters.Valid gated its α checks behind `AlphaPreference != 0` — so
// the one value that destroys safety was the one value that skipped validation.
//
// Three independent layers now close it, and each is asserted here:
//  1. config.Parameters.ValidQuorum rejects the params (config/quorum_test.go),
//  2. Transitive.Start refuses to run them,
//  3. NewChainConsensus — the single point where the running predicate is armed —
//     cannot install α < 1 at all, so even a path that skips both checks is safe.
package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// TestZeroAlphaDecidesNothingOnNoVotes is the direct regression, asserted on
// BEHAVIOR rather than on the stored α: build the engine the audit found reachable
// (α=0) and show neither decision predicate fires on an empty vote set.
func TestZeroAlphaDecidesNothingOnNoVotes(t *testing.T) {
	ctx := context.Background()
	for _, k := range []int{1, 4, 21} {
		c := NewChainConsensus(k, 0, 1) // α=0: the audit's construction

		// A single vote AGAINST the block leaves acceptVotes() == 0. Pre-fix,
		// `acceptVotes() >= alpha` held over that empty set and the block was
		// ACCEPTED — finality for a block no validator voted for.
		against := ids.GenerateTestID()
		blk := &Block{id: against, parentID: ids.GenerateTestID(), height: 1}
		if err := c.AddBlock(ctx, blk); err != nil {
			t.Fatalf("AddBlock: %v", err)
		}
		if err := c.ProcessVote(ctx, against, ids.GenerateTestNodeID(), false); err != nil {
			t.Fatalf("ProcessVote: %v", err)
		}
		if blk.acceptVotes() != 0 {
			t.Fatalf("K=%d: fixture is wrong — expected zero accept votes, got %d", k, blk.acceptVotes())
		}
		if c.IsAccepted(against) {
			t.Fatalf("K=%d: block ACCEPTED with ZERO accept votes — the accept predicate is satisfiable by nothing", k)
		}

		// And a block nobody has voted on at all stays undecided: pre-fix
		// `rejectVotes() >= alpha` also held on an empty set, so Poll rejected every
		// fresh block and dropped it from the tips.
		quiet := ids.GenerateTestID()
		if err := c.AddBlock(ctx, &Block{id: quiet, parentID: ids.GenerateTestID(), height: 1}); err != nil {
			t.Fatalf("AddBlock: %v", err)
		}
		if err := c.Poll(ctx, map[ids.ID]int{quiet: 0}); err != nil {
			t.Fatalf("Poll: %v", err)
		}
		if c.IsAccepted(quiet) || c.IsRejected(quiet) {
			t.Fatalf("K=%d: unvoted block decided (accepted=%v rejected=%v) on zero votes",
				k, c.IsAccepted(quiet), c.IsRejected(quiet))
		}

		if got := c.Alpha(); got < 1 || got > c.K() {
			t.Fatalf("K=%d: Alpha() = %d — outside [1, K=%d]", k, got, c.K())
		}
	}
}

// TestNewChainConsensusPinsQuorum: the one place the running predicate is armed can
// never hold a degenerate quorum, whatever it is handed.
func TestNewChainConsensusPinsQuorum(t *testing.T) {
	for _, tc := range []struct{ k, alpha, wantK, wantAlpha int }{
		{21, 0, 21, 1},  // the hole: zero quorum
		{21, -5, 21, 1}, // negative quorum
		{5, 9, 5, 5},    // unsatisfiable: more votes than members
		{0, 0, 1, 1},    // degenerate committee
		{1, 1, 1, 1},    // single validator — untouched
		{5, 4, 5, 4},    // a real quorum — untouched
		{3, 2, 3, 2},    // the local preset — untouched
	} {
		c := NewChainConsensus(tc.k, tc.alpha, 1)
		if c.K() != tc.wantK || c.Alpha() != tc.wantAlpha {
			t.Errorf("NewChainConsensus(%d,%d) armed (K=%d, α=%d); want (K=%d, α=%d)",
				tc.k, tc.alpha, c.K(), c.Alpha(), tc.wantK, tc.wantAlpha)
		}
	}
}

// TestStartRefusesUnsafeQuorum: an engine built from unsafe parameters does not run.
func TestStartRefusesUnsafeQuorum(t *testing.T) {
	base := func() config.Parameters {
		return config.Parameters{K: 21, Alpha: 0.69, Beta: 15, AlphaPreference: 15, AlphaConfidence: 15}
	}
	for _, tc := range []struct {
		name  string
		mutch func(*config.Parameters)
	}{
		{"zero preference", func(p *config.Parameters) { p.AlphaPreference = 0 }},
		{"zero confidence", func(p *config.Parameters) { p.AlphaConfidence = 0 }},
		{"confidence below preference", func(p *config.Parameters) { p.AlphaConfidence = 2 }},
		{"below the BFT floor", func(p *config.Parameters) { p.AlphaPreference, p.AlphaConfidence = 8, 8 }},
		{"alpha above K", func(p *config.Parameters) { p.AlphaPreference, p.AlphaConfidence = 22, 22 }},
	} {
		p := base()
		tc.mutch(&p)
		err := NewWithParams(p).Start(context.Background(), true)
		if !errors.Is(err, ErrInvalidParams) {
			t.Errorf("%s: Start() = %v; want ErrInvalidParams", tc.name, err)
		}
	}

	// The option path is covered too: WithParams runs AFTER a constructor, so
	// validating only in the constructor would leave it open.
	err := New(WithParams(config.Parameters{K: 21, Alpha: 0.69, Beta: 15})).Start(context.Background(), true)
	if !errors.Is(err, ErrInvalidParams) {
		t.Errorf("WithParams bypassed validation: Start() = %v; want ErrInvalidParams", err)
	}
}

// TestStartAcceptsSingleValidator guards the legitimate K=1/α=1 chain — the path
// bftCommittee preserves for a genuine single-validator network — against
// over-tightening. Its own accept IS the quorum, and it needs no vote verifier.
func TestStartAcceptsSingleValidator(t *testing.T) {
	p := config.Parameters{K: 1, Alpha: 1.0, Beta: 1, AlphaPreference: 1, AlphaConfidence: 1}
	e := NewWithParams(p)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("single-validator K=1/α=1 refused: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	if got := e.consensus.Alpha(); got != 1 {
		t.Fatalf("single-validator α = %d; want 1", got)
	}
}

// TestReclampRefusesWeakerQuorum (audit A9): Reclamp installs a quorum on a LIVE
// engine from a DERIVED validator count, so a mis-derivation is a running safety
// event. It used to accept any well-formed 1 ≤ α ≤ k — including k=4/α=2, where two
// disjoint 2-of-4 quorums overlap in nothing and can certify conflicting blocks.
func TestReclampRefusesWeakerQuorum(t *testing.T) {
	for _, tc := range []struct {
		name       string
		k, alpha   int
		wantK      int
		wantAlpha  int
		shouldTake bool
	}{
		{"weaker than the BFT floor", 4, 2, 1, 1, false},
		{"zero quorum", 4, 0, 1, 1, false},
		{"alpha above k", 4, 5, 1, 1, false},
		{"at the floor", 4, 3, 4, 3, true},
		{"the supermajority the sizer derives", 5, 4, 5, 4, true},
		{"single validator", 1, 1, 1, 1, true},
	} {
		c := NewChainConsensus(1, 1, 1)
		c.Reclamp(tc.k, tc.alpha)
		if c.K() != tc.wantK || c.Alpha() != tc.wantAlpha {
			t.Errorf("%s: Reclamp(%d,%d) left (K=%d, α=%d); want (K=%d, α=%d)",
				tc.name, tc.k, tc.alpha, c.K(), c.Alpha(), tc.wantK, tc.wantAlpha)
		}
	}
}

// TestReclampAcceptsEveryCommitteeTheSizerDerives: the floor must never refuse a
// legitimate re-clamp, or a growing validator set would wedge at its launch size.
func TestReclampAcceptsEveryCommitteeTheSizerDerives(t *testing.T) {
	for count := 1; count <= 64; count++ {
		k, alpha, clamped := bftCommittee(1000, count)
		if !clamped {
			continue
		}
		c := NewChainConsensus(1, 1, 1)
		c.Reclamp(k, alpha)
		if c.K() != k || c.Alpha() != alpha {
			t.Fatalf("count=%d: bftCommittee derived (K=%d, α=%d) but Reclamp refused it (left K=%d, α=%d)",
				count, k, alpha, c.K(), c.Alpha())
		}
	}
}
