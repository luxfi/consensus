// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// accept_cert_gate_test.go — the acceptance-authority collapse tests.
//
// One rule under test: No VerifiedQuorumCert, no finality. Every finality
// trigger funnels through Transitive.TryAccept; the SOLE finalizer is
// AcceptWithCert, which cannot run without a VerifiedQuorumCert; and a raw α-of-K
// COUNT ("enough voters responded", consensus.IsAccepted) is a LIVENESS signal
// only — it may trigger TryAccept but can never decide finality. These tests
// drive REAL signed votes through the live engine (no forged anything) and prove
// the count road is no longer an acceptance authority.
package chain

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// driveSignedAccepts tracks blk as a verified, non-own pending block on a fresh
// stake-weighted engine, then feeds REAL signed accept votes from validators
// [first,last]. It returns the engine + chainID so a test can assert finality.
// No self-vote is injected (non-own block), so the ONLY accepts are the ones fed.
func driveSignedAccepts(
	t *testing.T,
	vs *testValidatorSet,
	stake StakeSource,
	rec *recordingGossiper,
	blk *verifyOnceBlock,
	voters []int,
) (*Transitive, ids.ID) {
	t.Helper()
	e, chainID := newQuorumEngineOpts(t, dyn5(), vs, 0, rec, WithStakeWeighting(stake))

	cb := &Block{id: blk.id, parentID: blk.parentID, height: blk.height, timestamp: blk.timestamp.Unix(), data: blk.bytes}
	_ = e.consensus.AddBlock(context.Background(), cb)
	e.mu.Lock()
	e.pendingBlocks[blk.id] = &PendingBlock{ConsensusBlock: cb, VMBlock: blk, ProposedAt: time.Now(), Round: 0}
	e.mu.Unlock()

	pos := VotePosition{ChainID: chainID, Height: blk.height, Round: 0, BlockID: blk.id, ParentID: blk.parentID}
	for _, i := range voters {
		e.ReceiveVote(vs.signedVote(i, pos))
	}
	return e, chainID
}

// mustNotFinalize fails if blk is VM-accepted (AcceptCalled>=1) or reported
// accepted within d. It is the SAFETY assertion: a sub-quorum coalition must
// never reach finality.
func mustNotFinalize(t *testing.T, e *Transitive, blk *verifyOnceBlock, d time.Duration, why string) {
	t.Helper()
	if waitFor(d, func() bool { return blk.AcceptCalled() >= 1 }) {
		t.Fatalf("%s: SAFETY VIOLATION — block finalized (VM.Accept ran %d×), IsAccepted=%v",
			why, blk.AcceptCalled(), e.IsAccepted(blk.id))
	}
	if e.IsAccepted(blk.id) {
		t.Fatalf("%s: IsAccepted reported finality with no verified cert", why)
	}
}

// mustFinalize fails unless blk is VM-accepted exactly once and reported accepted
// within d. It is the LIVENESS assertion: a real quorum DOES finalize.
func mustFinalize(t *testing.T, e *Transitive, blk *verifyOnceBlock, d time.Duration, why string) {
	t.Helper()
	if !waitFor(d, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatalf("%s: LIVENESS FAILURE — a real quorum did not finalize (AcceptCalled=%d)", why, blk.AcceptCalled())
	}
	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("%s: block must VM.Accept exactly once, got %d", why, got)
	}
}

// TestNoAcceptWithoutVerifiedQC: the SOLE finalizer refuses a zero
// VerifiedQuorumCert. Even an internal caller cannot finalize by passing the zero
// value — the rule "no VerifiedQuorumCert, no finality" is structural.
func TestNoAcceptWithoutVerifiedQC(t *testing.T) {
	vs := newTestValidatorSet(5)
	stake := newStakeMap(vs, 20, 20, 20, 20, 20)
	rec := &recordingGossiper{}
	e, _ := newQuorumEngineOpts(t, dyn5(), vs, 0, rec, WithStakeWeighting(stake))

	blk := newTestBlock(1, ids.Empty, "no-qc")
	cb := &Block{id: blk.id, parentID: blk.parentID, height: blk.height, timestamp: blk.timestamp.Unix()}
	_ = e.consensus.AddBlock(context.Background(), cb)
	e.mu.Lock()
	e.pendingBlocks[blk.id] = &PendingBlock{ConsensusBlock: cb, VMBlock: blk, ProposedAt: time.Now()}
	e.mu.Unlock()

	// Calling the sole finalizer with the ZERO cert must refuse and finalize nothing.
	if err := e.AcceptWithCert(context.Background(), blk.id, VerifiedQuorumCert{}); err != ErrNoVerifiedQC {
		t.Fatalf("AcceptWithCert(zero cert) must return ErrNoVerifiedQC, got %v", err)
	}
	if blk.AcceptCalled() != 0 {
		t.Fatalf("zero cert finalized a block (VM.Accept ran %d×)", blk.AcceptCalled())
	}
	if e.IsAccepted(blk.id) {
		t.Fatal("zero cert produced finality")
	}
}

// TestProcessPendingBlocksCannotFinalizeByCount: the count road, exercised
// through the live pollLoop's finalizer, cannot finalize. Equal VOTE weight gives
// α=4 by count; the four low-stake voters reach that count but hold a stake
// minority, so processPendingBlocks (which now routes accepts through TryAccept)
// must NOT finalize. This is the structural fix for the pre-existing RED repro,
// asserted on the public finalizer name.
func TestProcessPendingBlocksCannotFinalizeByCount(t *testing.T) {
	vs := newTestValidatorSet(5)
	// EQUAL vote weight → α=4 by count; SKEWED stake → four voters hold 4/100.
	skew := newStakeMap(vs, 96, 1, 1, 1, 1)
	rec := &recordingGossiper{}

	blk := newTestBlock(1, ids.Empty, "count-only")
	e, _ := driveSignedAccepts(t, vs, skew, rec, blk, []int{1, 2, 3, 4})

	// The COUNT gate flips consensus.IsAccepted (acceptVotes=4>=α=4), and the
	// pollLoop runs processPendingBlocks on it — but TryAccept refuses without the
	// ⅔-stake cert, so the block must NOT VM.Accept.
	mustNotFinalize(t, e, blk, 1500*time.Millisecond,
		"processPendingBlocks/count-α with 4/100 stake")

	// Drive processPendingBlocks DIRECTLY too (not only via the loop) to prove the
	// finalizer itself refuses, independent of timing.
	e.processPendingBlocks()
	if blk.AcceptCalled() != 0 {
		t.Fatalf("processPendingBlocks finalized on count alone (VM.Accept ran %d×)", blk.AcceptCalled())
	}

	rec.mu.Lock()
	gotCerts := len(rec.certs)
	rec.mu.Unlock()
	if gotCerts != 0 {
		t.Fatalf("no ⅔-stake cert should exist for a 4/100 coalition, got %d gossiped", gotCerts)
	}
}

// TestSkewedStakeHeadcountMajorityRejected is the KEY regression named in the
// brief: A=60%, B=C=D=E=10%. Votes from B+C+D+E → count=4/5 but stake=40% → NO
// accept, NO finalize, NO state transition.
func TestSkewedStakeHeadcountMajorityRejected(t *testing.T) {
	vs := newTestValidatorSet(5)
	// A=60, B..E=10 each (total 100). The four small holders sum to 40% < ⅔.
	skew := newStakeMap(vs, 60, 10, 10, 10, 10)
	rec := &recordingGossiper{}

	blk := newTestBlock(1, ids.Empty, "skew-40pct")
	// B,C,D,E vote (indices 1..4): headcount 4/5 (≥α) but only 40% of stake. A abstains.
	e, _ := driveSignedAccepts(t, vs, skew, rec, blk, []int{1, 2, 3, 4})

	mustNotFinalize(t, e, blk, 1500*time.Millisecond,
		"4/5-headcount / 40%-stake coalition")

	// And no valid stake-weighted cert was gossiped — the cert path agreed it must
	// not finalize; only the count path (now defanged) ever said otherwise.
	rec.mu.Lock()
	gotCerts := len(rec.certs)
	rec.mu.Unlock()
	if gotCerts != 0 {
		t.Fatalf("a 40%%-stake coalition must produce NO cert, got %d gossiped", gotCerts)
	}
}

// TestEqualStakeFourOfFiveAcceptedWithQC: equal stake, four of five validators
// sign accept → count=4=α AND stake=80% > ⅔ → a verified QC exists → the block
// finalizes through AcceptWithCert. Proves the gate does not over-block real
// quorums (liveness).
func TestEqualStakeFourOfFiveAcceptedWithQC(t *testing.T) {
	vs := newTestValidatorSet(5)
	equal := newStakeMap(vs, 20, 20, 20, 20, 20) // 80% from any four
	rec := &recordingGossiper{}

	blk := newTestBlock(1, ids.Empty, "equal-4of5")
	e, _ := driveSignedAccepts(t, vs, equal, rec, blk, []int{0, 1, 2, 3})

	mustFinalize(t, e, blk, 2*time.Second, "equal-stake 4/5 with verified QC")

	// A verified cert was gossiped (the finality proof followers finalize on).
	if !waitFor(time.Second, func() bool {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return len(rec.certs) >= 1
	}) {
		t.Fatal("a verified α-of-K cert must be gossiped on finalization")
	}
}

// TestThreeOfFiveRejectedEvenIfCountThresholdMet: only three of five sign. On the
// dyn5 sizer α=4, so 3 < α never even reaches the count trigger — and crucially
// holds only 60% of equal stake (< ⅔). The block must NOT finalize. (Three-of-N
// is below the BFT overlap floor — exactly what the quorum must refuse.)
func TestThreeOfFiveRejectedEvenIfCountThresholdMet(t *testing.T) {
	vs := newTestValidatorSet(5)
	equal := newStakeMap(vs, 20, 20, 20, 20, 20) // three → 60% < ⅔
	rec := &recordingGossiper{}

	blk := newTestBlock(1, ids.Empty, "three-of-five")
	e, _ := driveSignedAccepts(t, vs, equal, rec, blk, []int{0, 1, 2})

	mustNotFinalize(t, e, blk, 1500*time.Millisecond, "3/5 (below quorum + 60% stake)")

	e.processPendingBlocks() // direct drive, prove the finalizer refuses
	if blk.AcceptCalled() != 0 {
		t.Fatalf("3/5 finalized (VM.Accept ran %d×)", blk.AcceptCalled())
	}
}

// TestVoteArrivalTriggersTryAcceptButRequiresQC: a vote arriving is a LIVENESS
// trigger. It funnels into TryAccept, but with only a sub-⅔ coalition voting,
// TryAccept returns ErrNoVerifiedQC and finalizes nothing. Then the missing
// heavy-stake vote arrives, the verified cert becomes assemblable, and the SAME
// trigger path finalizes. Proves: trigger ≠ authority; the QC is the authority.
func TestVoteArrivalTriggersTryAcceptButRequiresQC(t *testing.T) {
	vs := newTestValidatorSet(5)
	skew := newStakeMap(vs, 60, 10, 10, 10, 10) // A=60%
	rec := &recordingGossiper{}

	blk := newTestBlock(1, ids.Empty, "vote-trigger")
	// First only B,C,D,E vote: 4/5 count, 40% stake. Triggers TryAccept, no QC.
	e, chainID := driveSignedAccepts(t, vs, skew, rec, blk, []int{1, 2, 3, 4})
	mustNotFinalize(t, e, blk, 1*time.Second, "4/5-count/40%-stake before heavy vote")

	// Direct: TryAccept invoked by the trigger path must report ErrNoVerifiedQC.
	if err := e.TryAccept(context.Background(), blk.id); err != ErrNoVerifiedQC {
		t.Fatalf("TryAccept with 40%% stake must return ErrNoVerifiedQC, got %v", err)
	}

	// Now the heavy-stake validator A (index 0) votes → 100% stake, verified QC
	// assemblable. The vote-arrival trigger now finalizes through AcceptWithCert.
	pos := VotePosition{ChainID: chainID, Height: blk.height, Round: 0, BlockID: blk.id, ParentID: blk.parentID}
	e.ReceiveVote(vs.signedVote(0, pos))
	mustFinalize(t, e, blk, 2*time.Second, "after heavy-stake vote completes the ⅔ quorum")
}

// TestRepollTriggersTryAcceptButRequiresQC: a re-poll firing is a LIVENESS
// trigger (the pollLoop re-examines pending blocks). Driving the re-poll path
// (processPendingBlocks, which the pollLoop calls each tick) repeatedly over a
// sub-⅔ coalition must never finalize — re-polling is a retry, not an authority.
func TestRepollTriggersTryAcceptButRequiresQC(t *testing.T) {
	vs := newTestValidatorSet(5)
	skew := newStakeMap(vs, 60, 10, 10, 10, 10)
	rec := &recordingGossiper{}

	blk := newTestBlock(1, ids.Empty, "repoll-trigger")
	e, _ := driveSignedAccepts(t, vs, skew, rec, blk, []int{1, 2, 3, 4})

	// Fire the re-poll finalizer repeatedly — each is a trigger, none an authority.
	for i := 0; i < 25; i++ {
		e.processPendingBlocks()
		if blk.AcceptCalled() != 0 {
			t.Fatalf("re-poll #%d finalized a 40%%-stake block (VM.Accept ran %d×)", i, blk.AcceptCalled())
		}
	}
	if e.IsAccepted(blk.id) {
		t.Fatal("repeated re-polls produced finality without a verified cert")
	}
}

// TestVerifiedQuorumCertUnforgeableOutsideBuilder: a VerifiedQuorumCert can be
// produced ONLY by the verifying builder. The zero value (the only literal a
// foreign package can write, since qc is unexported) is NOT a finality authority,
// and BuildVerifiedQuorumCert refuses to mint one for a sub-quorum / bad-signature
// vote set. So no code path outside the builder can fabricate the authority token.
func TestVerifiedQuorumCertUnforgeableOutsideBuilder(t *testing.T) {
	vs := newTestValidatorSet(5)
	equal := newStakeMap(vs, 20, 20, 20, 20, 20)
	chainID := ids.GenerateTestID()
	pos := VotePosition{ChainID: chainID, Height: 1, Round: 0, BlockID: ids.GenerateTestID(), ParentID: ids.Empty}

	// 1) The zero value carries no witness and is rejected by the finalizer.
	var zero VerifiedQuorumCert
	if !zero.IsZero() || zero.Cert() != nil {
		t.Fatal("zero VerifiedQuorumCert must be empty (no embedded cert)")
	}

	// 2) Builder REFUSES a sub-quorum (3 votes, α=4): no token minted.
	sub := []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
	}
	if vc, err := BuildVerifiedQuorumCert(vs, equal, 4, 1, pos, sub); err == nil || !vc.IsZero() {
		t.Fatalf("builder must refuse a sub-α vote set (got err=%v zero=%v)", err, vc.IsZero())
	}

	// 3) Builder REFUSES a forged signature (wrong signer's bytes): no token.
	forged := []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(1, pos)}, // node 0 claims, node 1 signed
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
		{NodeID: vs.nodeID(3), Accept: true, Signature: vs.sign(3, pos)},
	}
	if vc, err := BuildVerifiedQuorumCert(vs, equal, 4, 1, pos, forged); err == nil || !vc.IsZero() {
		t.Fatalf("builder must refuse a forged-signature vote set (got err=%v zero=%v)", err, vc.IsZero())
	}

	// 4) Builder MINTS a token for a genuine α-of-K, ⅔-stake vote set — and ONLY
	// then is it non-zero. This is the sole production route.
	good := []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
		{NodeID: vs.nodeID(3), Accept: true, Signature: vs.sign(3, pos)},
	}
	vc, err := BuildVerifiedQuorumCert(vs, equal, 4, 1, pos, good)
	if err != nil || vc.IsZero() {
		t.Fatalf("builder must mint a token for a real ⅔ quorum (err=%v zero=%v)", err, vc.IsZero())
	}
	if vc.Cert() == nil || vc.Cert().VoterCount() != 4 {
		t.Fatalf("minted token must carry the 4-voter cert, got %v", vc.Cert())
	}
}
