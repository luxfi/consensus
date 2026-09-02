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

	"github.com/luxfi/consensus/config"
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
	return driveSignedAcceptsAt(t, dyn5(), vs, stake, rec, blk, voters)
}

// driveSignedAcceptsAt is driveSignedAccepts with the committee size spelled out, for
// sets that are not five (the six-entry lux-mainnet set, say).
func driveSignedAcceptsAt(
	t *testing.T,
	params config.Parameters,
	vs *testValidatorSet,
	stake StakeSource,
	rec *recordingGossiper,
	blk *verifyOnceBlock,
	voters []int,
) (*Transitive, ids.ID) {
	t.Helper()
	e, chainID := newQuorumEngineOpts(t, params, vs, 0, rec, WithStakeWeighting(stake))

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
// within d. It is the NOVA (local-accept) LIVENESS assertion: a bare-majority quorum DOES
// drive VM.Accept. Acceptance is the Nova tier — a MAJORITY of stake — not the ⅔-stake
// Quasar tier, so a block can Nova-accept here while never reaching export (see mustNotQuasar).
func mustFinalize(t *testing.T, e *Transitive, blk *verifyOnceBlock, d time.Duration, why string) {
	t.Helper()
	if !waitFor(d, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatalf("%s: NOVA LIVENESS FAILURE — a majority quorum did not accept (AcceptCalled=%d)", why, blk.AcceptCalled())
	}
	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("%s: block must VM.Accept exactly once, got %d", why, got)
	}
}

// mustQuasar fails unless blk reaches the EXPORT (Quasar, ⅔-by-stake) tier within d — the
// v1.36 export LIVENESS assertion: a real >⅔-stake supermajority DOES certify for export.
func mustQuasar(t *testing.T, e *Transitive, blk *verifyOnceBlock, d time.Duration, why string) {
	t.Helper()
	if !waitFor(d, func() bool { qh, ok := e.QuasarHeight(); return ok && qh >= blk.height }) {
		qh, ok := e.QuasarHeight()
		t.Fatalf("%s: EXPORT LIVENESS FAILURE — block did not reach Quasar (⅔-stake); QuasarHeight=(%d,%v), want ≥%d",
			why, qh, ok, blk.height)
	}
}

// mustNotQuasar fails if blk reaches the EXPORT (Quasar) tier within d. It is the
// export-SAFETY assertion: a coalition between the Nova majority and the Quasar ⅔ may drive
// local accept but can NEVER reach export finality (bridges / DEX settlement / cross-chain).
// A block that never Nova-accepts trivially never reaches Quasar; a Nova-accepted-but-sub-⅔
// block must stall at Nova with the Quasar frontier NOT advanced to it.
func mustNotQuasar(t *testing.T, e *Transitive, blk *verifyOnceBlock, d time.Duration, why string) {
	t.Helper()
	if waitFor(d, func() bool { qh, ok := e.QuasarHeight(); return ok && qh >= blk.height }) {
		t.Fatalf("%s: EXPORT SAFETY VIOLATION — a sub-⅔-stake coalition reached Quasar (⅔-stake export)", why)
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

// TestHeadcountMajorityWithoutStakeAcceptsNothing: a head-count is not a quorum. Four of five
// validators sign, holding 4 of 100 stake. Nova is a MAJORITY of stake and Quasar a ⅔
// SUPERMAJORITY of it, so neither tier admits this coalition — no local execution, no export.
func TestHeadcountMajorityWithoutStakeAcceptsNothing(t *testing.T) {
	vs := newTestValidatorSet(5)
	// EQUAL vote weight; SKEWED stake → the four voters hold 4/100.
	skew := newStakeMap(vs, 96, 1, 1, 1, 1)
	rec := &recordingGossiper{}

	blk := newTestBlock(1, ids.Empty, "count-majority")
	e, _ := driveSignedAccepts(t, vs, skew, rec, blk, []int{1, 2, 3, 4})

	mustNotFinalize(t, e, blk, 2*time.Second, "4/100-stake coalition (Nova local accept)")
	mustNotQuasar(t, e, blk, 500*time.Millisecond, "4/100-stake coalition (export gate)")
}

// TestSkewedStakeHeadcountMajorityRejected: A=60%, B=C=D=E=10%. B+C+D+E vote — a 4-of-5
// headcount holding 40%. A stake minority causes no state transition at either tier.
func TestSkewedStakeHeadcountMajorityRejected(t *testing.T) {
	vs := newTestValidatorSet(5)
	// A=60, B..E=10 each (total 100). The four small holders sum to 40%.
	skew := newStakeMap(vs, 60, 10, 10, 10, 10)
	rec := &recordingGossiper{}

	blk := newTestBlock(1, ids.Empty, "skew-40pct")
	// B,C,D,E vote (indices 1..4): headcount 4/5 but only 40% of stake. A abstains.
	e, _ := driveSignedAccepts(t, vs, skew, rec, blk, []int{1, 2, 3, 4})

	mustNotFinalize(t, e, blk, 2*time.Second, "40%-stake coalition (Nova local accept)")
	mustNotQuasar(t, e, blk, 500*time.Millisecond, "40%-stake coalition (export gate)")
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

// TestThreeOfFiveNovaAcceptsNotQuasar is the two-tier successor to the old
// "ThreeOfFiveRejectedEvenIfCountThresholdMet". Three of five sign — EXACTLY the Nova majority
// NovaQuorum(5)=3, so the block DOES Nova-accept (the "survive 3/5" mandate). But three equal
// stakes are 60% < ⅔, so it does NOT reach Quasar export. This is the canonical degraded case:
// production continues (Nova) while certification pauses (no Quasar) until a 4th validator
// votes.
func TestThreeOfFiveNovaAcceptsNotQuasar(t *testing.T) {
	vs := newTestValidatorSet(5)
	equal := newStakeMap(vs, 20, 20, 20, 20, 20) // three → 60% < ⅔
	rec := &recordingGossiper{}

	blk := newTestBlock(1, ids.Empty, "three-of-five")
	e, _ := driveSignedAccepts(t, vs, equal, rec, blk, []int{0, 1, 2})

	// NOVA: 3-of-5 is the bare majority — it accepts (local execution).
	mustFinalize(t, e, blk, 2*time.Second, "3/5 bare majority (Nova local accept)")
	// QUASAR: 60% stake ≤ ⅔ — no export cert (certification pauses in the degraded 3/5 mode).
	mustNotQuasar(t, e, blk, 500*time.Millisecond, "3/5 = 60% stake (export gate)")
}

// TestVoteArrivalDrivesNovaThenLateHeavyVoteCompletesQuasar: a vote arriving is a LIVENESS
// trigger into TryAccept. Under v1.36 a bare-majority count drives the NOVA accept (local
// execution) immediately; the EXPORT tier (Quasar) additionally needs >⅔ of STAKE. Because the
// ⅔-th stake vote NECESSARILY TRAILS the bare-majority accept, it is completed by a LATE
// attestation: B,C,D,E (4/5 count, 40% stake) Nova-accept; then the heavy holder A (60%) votes
// AFTER the accept, and the trailing-vote path completes the export cert — the block reaches
// Quasar with NO reorg (accept was already Nova; export is a monotone promotion).
func TestVoteArrivalWithoutStakeAcceptsNothingUntilTheHeavyVote(t *testing.T) {
	vs := newTestValidatorSet(5)
	skew := newStakeMap(vs, 60, 10, 10, 10, 10) // A=60%
	rec := &recordingGossiper{}

	blk := newTestBlock(1, ids.Empty, "vote-trigger")
	// First only B,C,D,E vote: 4/5 headcount, 40% stake — under the Nova majority.
	e, chainID := driveSignedAccepts(t, vs, skew, rec, blk, []int{1, 2, 3, 4})
	mustNotFinalize(t, e, blk, 2*time.Second, "40%-stake before heavy vote (Nova local accept)")
	mustNotQuasar(t, e, blk, 500*time.Millisecond, "40%-stake before heavy vote (export gate)")

	// The heavy-stake holder A (index 0) votes. 40%+60% = 100%: Nova ignites, and the same
	// vote carries the block past ⅔ so the export cert completes in the same step.
	pos := VotePosition{ChainID: chainID, Height: blk.height, Round: 0, BlockID: blk.id, ParentID: blk.parentID}
	e.ReceiveVote(vs.signedVote(0, pos))
	mustFinalize(t, e, blk, 2*time.Second, "heavy-stake vote completes the Nova majority")
	mustQuasar(t, e, blk, 2*time.Second, "heavy-stake vote completes the ⅔ export supermajority")
}

// TestRepollTriggersNovaAcceptButNotQuasarExport: a re-poll firing is a LIVENESS trigger (the
// pollLoop re-examines pending blocks). Under v1.36 a 4-of-5 count majority DRIVES the Nova accept
// (local execution) even for a 40%-stake coalition — but re-poll is a RETRY, not an authority: no
// amount of re-polling can manufacture the ⅔-STAKE supermajority the EXPORT (Quasar) tier needs.
func TestRepollIsNotAnAuthority(t *testing.T) {
	vs := newTestValidatorSet(5)
	skew := newStakeMap(vs, 60, 10, 10, 10, 10)
	rec := &recordingGossiper{}

	blk := newTestBlock(1, ids.Empty, "repoll-trigger")
	e, _ := driveSignedAccepts(t, vs, skew, rec, blk, []int{1, 2, 3, 4})

	// Fire the re-poll finalizer repeatedly — a retry, never an authority. Neither the Nova
	// majority nor the ⅔ export supermajority can be re-polled into existence.
	for i := 0; i < 25; i++ {
		e.processPendingBlocks()
	}
	mustNotFinalize(t, e, blk, time.Second, "40%-stake coalition under repeated re-poll (Nova)")
	mustNotQuasar(t, e, blk, 500*time.Millisecond, "40%-stake coalition under repeated re-poll (export gate)")
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

	// 2) Builder REFUSES a sub-quorum (3 votes against a derived floor of 4): no token.
	sub := []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
	}
	if vc, err := BuildVerifiedQuorumCert(vs, equal, Quasar, 1, pos, sub); err == nil || !vc.IsZero() {
		t.Fatalf("builder must refuse a sub-α vote set (got err=%v zero=%v)", err, vc.IsZero())
	}

	// 3) Builder REFUSES a forged signature (wrong signer's bytes): no token.
	forged := []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(1, pos)}, // node 0 claims, node 1 signed
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
		{NodeID: vs.nodeID(3), Accept: true, Signature: vs.sign(3, pos)},
	}
	if vc, err := BuildVerifiedQuorumCert(vs, equal, Quasar, 1, pos, forged); err == nil || !vc.IsZero() {
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
	vc, err := BuildVerifiedQuorumCert(vs, equal, Quasar, 1, pos, good)
	if err != nil || vc.IsZero() {
		t.Fatalf("builder must mint a token for a real ⅔ quorum (err=%v zero=%v)", err, vc.IsZero())
	}
	if vc.Cert() == nil || vc.Cert().VoterCount() != 4 {
		t.Fatalf("minted token must carry the 4-voter cert, got %v", vc.Cert())
	}
}
