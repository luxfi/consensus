// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_weighted_test.go — HIGH-3 (stake-weighted finality) and HIGH-4 (Mode
// requires a live quorum gossiper) tests.
//
// HIGH-3: on a PoS chain with UNEQUAL stake, a raw voter COUNT (α-of-K) is not
// the same as a ⅔-by-STAKE supermajority — a coalition of many low-stake
// validators can reach the count while holding a stake minority. With a
// StakeSource wired, a cert finalizes only on a strict ⅔-of-stake supermajority.
package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// stakeMap is a test StakeSource: fixed per-node weights, height-independent.
type stakeMap struct {
	w     map[ids.NodeID]uint64
	total uint64
}

func newStakeMap(vs *testValidatorSet, weights ...uint64) *stakeMap {
	s := &stakeMap{w: make(map[ids.NodeID]uint64)}
	for i, wt := range weights {
		s.w[vs.nodeID(i)] = wt
		s.total += wt
	}
	return s
}

func (s *stakeMap) Weight(nodeID ids.NodeID, _ uint64) uint64 { return s.w[nodeID] }
func (s *stakeMap) TotalStake(_ uint64) uint64                { return s.total }
func (s *stakeMap) ValidatorCount(_ uint64) int               { return len(s.w) }

// TestWeighted_LowStakeCoalitionRejected is the HIGH-3 core: 4 validators with
// stake {97, 1, 1, 1} (total 100). A cert signed by the THREE low-stake nodes
// (1,2,3) reaches the α=3 COUNT but holds only 3/100 of stake — it MUST be
// rejected. A cert that includes the 97-stake node passes.
func TestWeighted_LowStakeCoalitionRejected(t *testing.T) {
	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()
	stake := newStakeMap(vs, 97, 1, 1, 1)

	follower := NewWithConfig(Config{Params: params4()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
		WithStakeWeighting(stake))
	_ = follower.Start(context.Background(), true)
	t.Cleanup(func() { _ = follower.Stop(context.Background()) })
	rt := &Runtime{Transitive: follower, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	blk := newTestBlock(1, ids.Empty, "weighted")
	trackVerifiedBlock(rt, blk, 0)

	// Cert by the low-stake coalition {1,2,3}: count=3 (≥α) but stake=3/100.
	pos := VotePosition{ChainID: chainID, Height: 1, Round: 0, BlockID: blk.id, ParentID: ids.Empty}
	lowCert, err := AssembleQuorumCert(pos, Quasar, 3, []SignedVote{
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
		{NodeID: vs.nodeID(3), Accept: true, Signature: vs.sign(3, pos)},
	})
	if err != nil {
		t.Fatalf("assemble low cert: %v", err)
	}
	lowBytes, _ := lowCert.MarshalBinary()

	if rt.HandleIncomingCert(lowBytes) {
		t.Fatal("HIGH-3: a count-α cert holding only 3%% of stake must NOT finalize")
	}
	if follower.IsAccepted(blk.id) || blk.AcceptCalled() != 0 {
		t.Fatal("HIGH-3: low-stake coalition must not VM.Accept")
	}

	// Cert including the 97-stake node {0,1,2}: count=3 AND stake=99/100 → finalizes.
	hiCert, err := AssembleQuorumCert(pos, Quasar, 3, []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
	})
	if err != nil {
		t.Fatalf("assemble hi cert: %v", err)
	}
	hiBytes, _ := hiCert.MarshalBinary()
	if !rt.HandleIncomingCert(hiBytes) {
		t.Fatal("HIGH-3: a ⅔+ stake cert must finalize")
	}
	if blk.AcceptCalled() != 1 {
		t.Fatalf("VM.Accept exactly once on the stake-supermajority cert, got %d", blk.AcceptCalled())
	}
}

// TestWeighted_VerifyWeightedThreshold pins BOTH halves of the export rule on the
// cert predicate directly (no engine): the strict >⅔ stake boundary including the
// exactly-⅔ rejection, and the ⌊2n/3⌋+1 distinct-signer floor.
//
// Four seats, because the export count floor is ⌊2n/3⌋+1 and on three seats that
// is three — every stake row would then be answered by the count before the stake
// boundary was reached. On four it is three, so a three-signer certificate isolates
// the stake half and a two-signer one isolates the count half.
func TestWeighted_VerifyWeightedThreshold(t *testing.T) {
	vs := newTestValidatorSet(4)
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()}
	mkCert := func(idx ...int) *QuorumCert {
		votes := make([]SignedVote, 0, len(idx))
		for _, i := range idx {
			votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
		}
		c, err := AssembleQuorumCert(pos, Quasar, uint32(len(idx)), votes)
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		return c
	}

	// The stake maps are height-independent, so the epoch height passed to
	// VerifyWeighted is the cert's position height (pos.Height); the predicate
	// boundary under test is unchanged by the height-pinning refactor.
	const epoch = uint64(1) // == pos.Height
	// total=9, voters {0,2,3} hold 3+2+1 = 6 = exactly ⅔ → MUST be rejected (strict).
	// Three signers, so the count floor ⌊2·4/3⌋+1 = 3 is met and STAKE is what refuses.
	exactlyTwoThirds := &stakeMap{w: map[ids.NodeID]uint64{
		vs.nodeID(0): 3, vs.nodeID(1): 3, vs.nodeID(2): 2, vs.nodeID(3): 1}, total: 9}
	err := mkCert(0, 2, 3).VerifyWeighted(vs, exactlyTwoThirds, epoch)
	if err == nil {
		t.Fatal("exactly ⅔ of stake must be rejected (need STRICT supermajority)")
	}
	if !errors.Is(err, ErrQCStakeBelowSupermajority) {
		t.Fatalf("exactly ⅔ must be refused on the STAKE clause, got %v", err)
	}
	// voters {0,1,3} hold 3+3+1 = 7/9 > ⅔ with three signers → accepted.
	if err := mkCert(0, 1, 3).VerifyWeighted(vs, exactlyTwoThirds, epoch); err != nil {
		t.Fatalf("7/9 > ⅔ on three signers must be accepted: %v", err)
	}
	// THE COUNT HALF. The SAME 7/9 stake, held by two signers instead of three, is
	// refused — and refused on the count, not on stake, because the stake is there.
	// Export finality says a Byzantine supermajority of PARTIES agreed; two of four
	// is not that however the weights fall, and a rung that read only stake would
	// export here. See config.TwoThirdsCount.
	sameStakeFewerSigners := &stakeMap{w: map[ids.NodeID]uint64{
		vs.nodeID(0): 4, vs.nodeID(1): 3, vs.nodeID(2): 1, vs.nodeID(3): 1}, total: 9}
	err = mkCert(0, 1).VerifyWeighted(vs, sameStakeFewerSigners, epoch)
	if !errors.Is(err, ErrQCBelowThreshold) {
		t.Fatalf("two of four holding 7/9 must be refused on the export signer floor, got %v", err)
	}
	// One more signer over the same set clears it: the floor was the binding clause.
	if err := mkCert(0, 1, 2).VerifyWeighted(vs, sameStakeFewerSigners, epoch); err != nil {
		t.Fatalf("three of four holding 8/9 must be accepted: %v", err)
	}
	// nil stake source → fail closed (not silently count-only).
	if err := mkCert(0, 1, 2).VerifyWeighted(vs, nil, epoch); err == nil {
		t.Fatal("nil stake source must fail closed")
	}
}

// TestMode_RequiresQuorumGossiper is the HIGH-4 guard fix: ModeQuorumFinality
// requires a present quorum gossiper, not merely K>1 && verifier!=nil. A K>1
// engine with a verifier but NO cert gossiper is degraded → ModeUnknown.
func TestMode_RequiresQuorumGossiper(t *testing.T) {
	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()

	// Verifier present, gossiper present, STAKE source present → quorum-finality, value DEX
	// permitted. (HIGH-4b: a stake source is now ALSO required — value DEX finalizes on a
	// stake-weighted supermajority, not a raw count.)
	withGossip := NewWithConfig(Config{Params: params4()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
		WithStakeWeighting(vs))
	if got := withGossip.Mode(); got != ModeQuorumFinality {
		t.Fatalf("verifier+gossiper+stake must be quorum-finality, got %s", got)
	}

	// Verifier present, NO gossiper → degraded → ModeUnknown → value DEX refused.
	noGossip := NewWithConfig(Config{Params: params4()}, WithVoteVerifier(vs), WithStakeWeighting(vs))
	if got := noGossip.Mode(); got != ModeUnknown {
		t.Fatalf("verifier WITHOUT gossiper must be ModeUnknown (degraded), got %s", got)
	}

	// HIGH-4b: verifier + gossiper but NO stake source → ModeUnknown → value DEX refused.
	// A value chain that did not wire a stake source finalizes count-α only, which is weaker
	// than the stake-weighted supermajority the launch rule requires.
	noStake := NewWithConfig(Config{Params: params4()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)))
	if got := noStake.Mode(); got != ModeUnknown {
		t.Fatalf("verifier+gossiper WITHOUT a stake source must be ModeUnknown, got %s", got)
	}
}

// guard: params4 is BFT-valid (sanity that the test fixture itself passes the
// floor we added — otherwise the tests above would be exercising an invalid cfg).
func TestParams4IsBFTValid(t *testing.T) {
	if err := params4().Valid(); err != nil {
		t.Fatalf("params4 (K=4/α=3) must be BFT-valid: %v", err)
	}
	if (config.Parameters{K: 4, AlphaPreference: 3}).ByzantineFaultTolerance() != 1 {
		t.Fatal("K=4 must tolerate f=1")
	}
}
