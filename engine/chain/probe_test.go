// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// zz_red_integration_probe_test.go — INDEPENDENT Red adversarial probes for the
// integration/finality-fix-complete branch. These are NOT part of Blue's suite;
// they target the SEAM the three combined changes create that no single existing
// test exercises:
//
//   - the L-1/buffer + double-count work is tested COUNT-ONLY (no StakeSource);
//   - the weighted (⅔-stake) path is tested only via a PRE-ASSEMBLED cert
//     (HandleIncomingCert);
//   - the f143010ec Gossip routing enables HandleIncomingVote as a NEW live-vote
//     ingestion entry.
//
// The unproven composition: feed a low-stake (count≥α, stake<⅔) coalition's votes
// through the LIVE vote path (channel + HandleIncomingVote) into a STAKE-WEIGHTED
// engine, and assert the proposer-side assemble→TryAccept finality road refuses
// to finalize. If assembleCertLocked's VerifyWeighted gate were bypassable from
// the count/Gossip path, THIS is where it would show.
//
// Each test either stays GREEN (the gate holds) or, if it goes red, is a concrete
// safety finding. They drive only the public/engine API.
package chain

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// newStakeWeightedRuntime builds a started Runtime for `self` with BOTH a vote
// verifier AND a StakeSource wired (so finality is the ⅔-by-stake predicate, not
// a raw count) plus a Catchup transport — the full production-shaped wiring the
// existing weighted tests (bare engine) do not assemble.
func newStakeWeightedRuntime(t *testing.T, vs *testValidatorSet, self int, stake StakeSource, cu Catchup) (*Runtime, ids.ID) {
	t.Helper()
	chainID := ids.GenerateTestID()
	rt := NewRuntime(NetworkConfig{
		ChainID:      chainID,
		NetworkID:    ids.GenerateTestID(),
		NodeID:       vs.nodeID(self),
		Logger:       log.Noop(),
		Params:       ptrParams(params5()),
		VoteVerifier: vs,
		VoteSigner:   vs.signerFor(self),
		Gossiper:     &certQuorumGossiper{rec: &recordingGossiper{}},
		StakeSource:  stake,
		Catchup:      cu,
	})
	if err := rt.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = rt.Stop(context.Background()) })
	return rt, chainID
}

// RED-INT-1 — STAKE GATE HOLDS ACROSS THE LIVE (CHANNEL) VOTE PATH.
//
// 5 validators, stake {96,1,1,1,1} (total 100). The four 1-stake nodes {1,2,3,4}
// are an α=3 COUNT supermajority but hold only 4/100 of stake. They each send a
// GENUINE signed accept via the live channel path (ReceiveVote → handleVote →
// recordCertVoteLocked → ProcessVote → tryFinalizeBlock → assembleCertLocked).
// The block is locally tracked. acceptVotes climbs ≥ α, so the COUNT predicate
// (HasEnoughResponsesForRetry) flips true and TryAccept is REACHED — but
// assembleCertLocked runs VerifyWeighted, which must REFUSE (4% < ⅔). The block
// MUST NOT finalize. self(0), the 96-stake node, deliberately does NOT vote, so
// no honest supermajority forms.
func TestRED_INT_LowStakeCoalition_LiveVotePath_DoesNotFinalize(t *testing.T) {
	vs := newTestValidatorSet(5)
	stake := newStakeMap(vs, 96, 1, 1, 1, 1) // total 100; {1,2,3,4} = 4
	cu := &deliveringCatchup{store: map[ids.ID]*verifyOnceBlock{}}
	rt, chainID := newStakeWeightedRuntime(t, vs, 0, stake, cu)
	cu.mu.Lock()
	cu.rt = rt
	cu.mu.Unlock()

	blk := newTestBlock(1, ids.Empty, "lowstake-live")
	// Track the block locally WITHOUT casting self's vote (trackVerifiedBlock does
	// not self-vote), so the only votes are the low-stake coalition's.
	trackVerifiedBlock(rt, blk, 0)
	pos := posFor(chainID, blk)

	// The α=3 COUNT is met (4 voters) but the STAKE is 4/100. Each vote is genuine.
	for _, i := range []int{1, 2, 3, 4} {
		rt.ReceiveVote(vs.signedVote(i, pos))
	}

	// The COUNT predicate must be reachable (proving we actually exercised the
	// finality road, not a vacuous "never had enough votes" pass).
	if !waitFor(2*time.Second, func() bool { return rt.HasEnoughResponsesForRetry(blk.id) }) {
		t.Fatalf("precondition: the α-of-K COUNT predicate never tripped — the live votes did "+
			"not reach the engine, so this test would be vacuous (count=%d)", blk.AcceptCalled())
	}

	// SAFETY: despite count ≥ α, the ⅔-STAKE gate in assembleCertLocked must refuse.
	if waitFor(800*time.Millisecond, func() bool { return rt.IsAccepted(blk.id) }) {
		t.Fatalf("SAFETY VIOLATION: a 4%%-stake coalition finalized a block via the LIVE vote path "+
			"(VM.Accept=%d) — assembleCertLocked's VerifyWeighted gate was bypassed from the "+
			"count/Gossip ingestion road", blk.AcceptCalled())
	}
	if got := blk.AcceptCalled(); got != 0 {
		t.Fatalf("VM.Accept must not run for a sub-⅔-stake coalition, got %d", got)
	}

	// LIVENESS CONTROL (anti-vacuity): once the 96-stake node ALSO votes, stake is
	// 100/100 > ⅔ and the SAME block finalizes — so the refusal above was the stake
	// gate, not a stuck engine.
	rt.ReceiveVote(vs.signedVote(0, pos))
	if !waitFor(2*time.Second, func() bool { return rt.IsAccepted(blk.id) }) {
		t.Fatalf("liveness: block did not finalize once the 96-stake node voted (stake>⅔) — "+
			"VerifyWeighted is over-strict (Accept=%d)", blk.AcceptCalled())
	}
}

// RED-INT-2 — STAKE GATE HOLDS THROUGH THE GOSSIP HandleIncomingVote ENTRY.
//
// The f143010ec routing wires inbound app-Gossip to HandleIncomingVote. Drive the
// low-stake coalition's votes through THAT entry (the new ingestion surface) into
// a stake-weighted engine and assert the same refusal. HandleIncomingVote
// verifies the sig and forwards to ReceiveVote; the finality decision is still the
// proposer-side stake-weighted assemble. A bypass here would mean the newly-routed
// Gossip path is a count-only finality road.
func TestRED_INT_LowStakeCoalition_GossipVoteEntry_DoesNotFinalize(t *testing.T) {
	vs := newTestValidatorSet(5)
	stake := newStakeMap(vs, 96, 1, 1, 1, 1)
	cu := &deliveringCatchup{store: map[ids.ID]*verifyOnceBlock{}}
	rt, chainID := newStakeWeightedRuntime(t, vs, 0, stake, cu)
	cu.mu.Lock()
	cu.rt = rt
	cu.mu.Unlock()

	blk := newTestBlock(1, ids.Empty, "lowstake-gossip")
	trackVerifiedBlock(rt, blk, 0)
	pos := posFor(chainID, blk)

	// Encode each coalition vote as the Gossip envelope payload and feed it through
	// the demux entry the router now reaches (HandleIncomingVote). Each returns true
	// (sig verifies) — we are NOT testing the sig gate here, we are testing that a
	// verified-but-low-stake set still cannot finalize.
	accepted := 0
	for _, i := range []int{1, 2, 3, 4} {
		voteBytes, err := encodeSignedVote(vs.nodeID(i), vs.sign(i, pos))
		if err != nil {
			t.Fatalf("encode vote %d: %v", i, err)
		}
		if rt.HandleIncomingVote(blk.id, voteBytes) {
			accepted++
		}
	}
	if accepted != 4 {
		t.Fatalf("precondition: all 4 genuine coalition votes should verify+count via the Gossip "+
			"entry, got %d — probe wiring is wrong (would be vacuous)", accepted)
	}

	if !waitFor(2*time.Second, func() bool { return rt.HasEnoughResponsesForRetry(blk.id) }) {
		t.Fatalf("precondition: COUNT predicate never tripped via the Gossip vote entry (vacuous)")
	}
	if waitFor(800*time.Millisecond, func() bool { return rt.IsAccepted(blk.id) }) {
		t.Fatalf("SAFETY VIOLATION: a 4%%-stake coalition finalized via the Gossip HandleIncomingVote "+
			"entry (VM.Accept=%d) — the newly-routed Gossip path is a count-only finality road",
			blk.AcceptCalled())
	}

	// Anti-vacuity: the 96-stake node's vote (via the SAME Gossip entry) tips stake
	// over ⅔ and finalizes.
	vb0, err := encodeSignedVote(vs.nodeID(0), vs.sign(0, pos))
	if err != nil {
		t.Fatalf("encode node0 vote: %v", err)
	}
	if !rt.HandleIncomingVote(blk.id, vb0) {
		t.Fatalf("the 96-stake node's genuine vote must verify via the Gossip entry")
	}
	if !waitFor(2*time.Second, func() bool { return rt.IsAccepted(blk.id) }) {
		t.Fatalf("liveness: block did not finalize once >⅔ stake voted via the Gossip entry (Accept=%d)",
			blk.AcceptCalled())
	}
}

// RED-INT-3 — DOUBLE-COUNT UNDER STAKE isolates the replay/double-count attack
// under stake by making SELF a low-stake node: self=node4 (1 stake). Now self's
// own auto-vote (1/100) is NOT a supermajority, and replaying another 1-stake
// validator (node1) any number of times yields distinct stake {node4,node1} =
// 2/100 ≪ ⅔. The block MUST NOT finalize no matter how many replays. A genuine
// high-stake voter (node0, 96) then finalizes it (anti-vacuity).
func TestRED_INT_DoubleCount_SelfLowStake(t *testing.T) {
	vs := newTestValidatorSet(5)
	stake := newStakeMap(vs, 96, 1, 1, 1, 1) // node0=96, nodes1..4=1 each
	cu := &deliveringCatchup{store: map[ids.ID]*verifyOnceBlock{}}
	// self = node4 (1 stake): its auto-vote alone is not a supermajority.
	rt, chainID := newStakeWeightedRuntime(t, vs, 4, stake, cu)
	cu.mu.Lock()
	cu.rt = rt
	cu.mu.Unlock()

	blk := newTestBlock(1, ids.Empty, "doublecount-selflow")
	cu.mu.Lock()
	cu.store[blk.id] = blk
	cu.mu.Unlock()
	pos := posFor(chainID, blk)

	// Replay node1 (1 stake) heavily, before and after track.
	v1 := vs.signedVote(1, pos)
	for i := 0; i < 32; i++ {
		rt.ReceiveVote(v1)
	}
	if !waitFor(2*time.Second, func() bool { return cu.wasRequested(blk.id) }) {
		t.Fatalf("vote for untracked block must trigger fetch")
	}
	for i := 0; i < 32; i++ {
		rt.ReceiveVote(v1)
	}

	// The COUNT predicate trips (self's auto-vote + node1's replays push acceptVotes
	// well past α=3), so the finality road is exercised.
	if !waitFor(2*time.Second, func() bool { return rt.HasEnoughResponsesForRetry(blk.id) }) {
		t.Fatalf("precondition: COUNT predicate never tripped (vacuous) — Accept=%d", blk.AcceptCalled())
	}

	// SAFETY: distinct stake = {node4:1, node1:1} = 2/100 ≪ floor(2/3·100)=66. The
	// stake-weighted finalize must refuse despite the inflated count.
	if waitFor(900*time.Millisecond, func() bool { return rt.IsAccepted(blk.id) }) {
		t.Fatalf("SAFETY VIOLATION: block finalized with distinct stake 2/100 by replaying one "+
			"1-stake validator (VM.Accept=%d) — NodeID-keyed stake summation failed; the count "+
			"road manufactured stake-finality", blk.AcceptCalled())
	}

	// ANTI-VACUITY: node0's genuine 96-stake vote tips {node4,node1,node0}=98/100>⅔.
	rt.ReceiveVote(vs.signedVote(0, pos))
	if !waitFor(2*time.Second, func() bool { return rt.IsAccepted(blk.id) }) {
		t.Fatalf("liveness: block did not finalize once the 96-stake node voted (Accept=%d)",
			blk.AcceptCalled())
	}
}
