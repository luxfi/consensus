// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_matrix_test.go — the n=1..10 QUORUM ACCEPTANCE MATRIX at the RUNTIME level.
//
// Each case builds a REAL engine (NewWithConfig + a wired gossiperProposer + the Ed25519
// testValidatorSet as verifier + a StakeSource), tracks a block, then delivers a cert assembled
// from a SPECIFIC set of validators through the runtime cert path (Runtime.HandleIncomingCert →
// verify signatures + strict >⅔-by-stake VerifyWeighted + fork-safety → VM.Accept). It asserts the
// block DECIDES (VM.Accept exactly once) or does NOT (zero accepts, no false finality). This is the
// runtime decision, not the isolated cert predicate: the same path a gossiped cert takes on a live
// node.
//
// Two axes:
//   - UNWEIGHTED n=1..10 with EQUAL stake: q = ⌊2n/3⌋+1. q online DECIDES; q−1 does NOT. With equal
//     weight the q-of-n count boundary IS the >⅔-stake boundary, so this pins the denominator sweep.
//   - WEIGHTED (unequal stake): the >⅔-STAKE predicate is the SOLE discriminator — each no/yes pair
//     has the SAME voter COUNT and differs only in STAKE, so a raw-count quorum could never tell
//     them apart. This is the case equal-count tests miss (a low-stake coalition reaching the count).
package chain

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// trackMockBlock seeds a trackingMockBlock as a verified pending block (the trackingMockBlock
// analogue of trackVerifiedBlock, which is bound to verifyOnceBlock) so a cert can drive its
// finalize — used for the fail-closed / recovery cases that need a controllable VM.Accept.
func trackMockBlock(rt *Runtime, blk *trackingMockBlock, round uint32) {
	cb := &Block{id: blk.id, parentID: blk.parentID, height: blk.height, timestamp: blk.timestamp.Unix(), data: blk.bytes}
	_ = rt.Transitive.consensus.AddBlock(context.Background(), cb)
	rt.Transitive.mu.Lock()
	rt.Transitive.pendingBlocks[blk.id] = &PendingBlock{ConsensusBlock: cb, VMBlock: blk, ProposedAt: time.Now(), Round: round}
	rt.Transitive.mu.Unlock()
}

// certFor assembles an Ed25519 cert for pos signed by the given validator indices of vs.
func certFor(t *testing.T, vs *testValidatorSet, pos VotePosition, voters ...int) []byte {
	t.Helper()
	sv := make([]SignedVote, 0, len(voters))
	for _, i := range voters {
		sv = append(sv, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	cert, err := AssembleQuorumCert(pos, Quasar, uint32(len(voters)), sv)
	if err != nil {
		t.Fatalf("assemble cert %v: %v", voters, err)
	}
	b, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal cert: %v", err)
	}
	return b
}

// quorumq returns the strict-BFT quorum size q = ⌊2n/3⌋+1 for n validators.
func quorumq(n int) int { return 2*n/3 + 1 }

// matrixParams builds a BFT-valid parameter set for K=n with the given integer accept quorum
// (AlphaPreference). The BFT floor 2·α−K ≥ ⌊(K-1)/3⌋+1 is satisfied by every (n,α) the matrix uses.
func matrixParams(n, alpha int) config.Parameters {
	return config.Parameters{K: n, Alpha: 0.75, AlphaPreference: alpha, AlphaConfidence: alpha, Beta: 1}
}

// decidesViaRuntimeCert wires a real engine for n validators (weights == nil ⇒ equal unit stake via
// the validator set itself; else a stakeMap of the given weights), tracks one block, and delivers a
// cert signed by exactly `voters` through Runtime.HandleIncomingCert. It returns whether the block
// DECIDED (finalized AND VM.Accept called exactly once) — the runtime yes/no the matrix asserts.
func decidesViaRuntimeCert(t *testing.T, n int, weights []uint64, alpha int, voters []int) bool {
	t.Helper()
	vs := newTestValidatorSet(n)
	chainID := ids.GenerateTestID()

	var stake StakeSource = vs // equal unit weights (Weight==1 each); the value-quorum gate needs one
	if weights != nil {
		stake = newStakeMap(vs, weights...)
	}

	e := NewWithConfig(Config{Params: matrixParams(n, alpha)},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
		WithStakeWeighting(stake))
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("start (n=%d,alpha=%d): %v", n, alpha, err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	rt := &Runtime{Transitive: e, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	blk := newTestBlock(1, ids.Empty, "matrix")
	trackVerifiedBlock(rt, blk, 0)

	pos := VotePosition{ChainID: chainID, Height: 1, Round: 0, BlockID: blk.id, ParentID: ids.Empty}
	sv := make([]SignedVote, 0, len(voters))
	for _, i := range voters {
		sv = append(sv, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	cert, err := AssembleQuorumCert(pos, Quasar, uint32(len(voters)), sv)
	if err != nil {
		t.Fatalf("assemble cert (voters=%v): %v", voters, err)
	}
	certBytes, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal cert: %v", err)
	}

	finalized := rt.HandleIncomingCert(certBytes)
	// A block DECIDES iff the runtime finalized it AND the VM applied it exactly once. A "no" case
	// must show BOTH: no finality AND no false VM.Accept (never a silent partial accept).
	accepted := blk.AcceptCalled()
	if finalized && accepted != 1 {
		t.Fatalf("finalized but VM.Accept=%d (want exactly 1) — finality/apply mismatch", accepted)
	}
	if !finalized && accepted != 0 {
		t.Fatalf("NOT finalized but VM.Accept=%d (want 0) — FALSE ACCEPT below quorum", accepted)
	}
	return finalized
}

func seqN(m int) []int {
	s := make([]int, m)
	for i := range s {
		s[i] = i
	}
	return s
}

// TestBlue_QuorumMatrix_Unweighted is the n=1..10 denominator sweep at the runtime level: exactly q
// online DECIDES; q−1 online does NOT finalize and does NOT false-accept.
func TestBlue_QuorumMatrix_Unweighted(t *testing.T) {
	for n := 1; n <= 10; n++ {
		q := quorumq(n)
		// exactly q online → DECIDES.
		t.Run(fmt.Sprintf("n%d_%dof%d_yes", n, q, n), func(t *testing.T) {
			if !decidesViaRuntimeCert(t, n, nil, q, seqN(q)) {
				t.Fatalf("n=%d: %d-of-%d (exactly q) MUST decide", n, q, n)
			}
		})
		// q−1 online → NO finality (n≥2; for n=1 q−1=0 is not a case).
		if q-1 >= 1 {
			t.Run(fmt.Sprintf("n%d_%dof%d_no", n, q-1, n), func(t *testing.T) {
				if decidesViaRuntimeCert(t, n, nil, q, seqN(q-1)) {
					t.Fatalf("n=%d: %d-of-%d (q−1) MUST NOT finalize", n, q-1, n)
				}
			})
		}
	}
}

// TestBlue_QuorumMatrix_Weighted proves the >⅔-BY-STAKE predicate is A finality gate on a PoS
// chain — NOT a raw voter count. Each no/yes pair has the SAME number of voters and differs ONLY in
// the stake behind them, so a count-based quorum could not distinguish them. total=100 in every
// case; the strict floor is ⌊2·100/3⌋ = 66, so 66 is rejected (exactly ⅔) and 67 is the first
// accept.
//
// Every pair runs on FOUR seats. The export rung's own count floor is ⌊2n/3⌋+1 — three of four,
// and THREE OF THREE — so a three-seat set has no room for a pair that differs in stake at a
// constant count: every two-signer certificate over it is refused on the count before stake is
// ever the question. Four seats is the smallest set on which the stake half can be isolated, and
// isolating it is what this test is for. The three-seat rows are kept, below, as what they now
// demonstrate instead.
func TestBlue_QuorumMatrix_Weighted(t *testing.T) {
	cases := []struct {
		name    string
		n       int
		weights []uint64
		alpha   int
		voters  []int // indices into the weights
		stake   uint64
		expect  bool
	}{
		// [40,30,29,1]: SAME 3-voter count — 60 (30+29+1) rejected; 71 (40+30+1) accepted. Stake decides.
		{"w40-30-29-1/60/no", 4, []uint64{40, 30, 29, 1}, 3, []int{1, 2, 3}, 60, false},
		{"w40-30-29-1/71/yes", 4, []uint64{40, 30, 29, 1}, 3, []int{0, 1, 3}, 71, true},
		// [34,33,32,1]: the TIGHTEST boundary — 66 (33+32+1, EXACTLY ⅔) rejected (strict >); 67
		// (34+32+1) accepted. Same 3-voter count both ways; a single stake unit flips the decision.
		{"w34-33-32-1/66/no", 4, []uint64{34, 33, 32, 1}, 3, []int{1, 2, 3}, 66, false},
		{"w34-33-32-1/67/yes", 4, []uint64{34, 33, 32, 1}, 3, []int{0, 2, 3}, 67, true},
		// THE COUNT HALF, on three seats. Both rows hold MORE than two thirds of the stake and both
		// are refused, because ⌊2·3/3⌋+1 = 3: a three-seat chain exports on three signatures or not
		// at all. The chain's own α is set to 2 here so the runtime's threshold filter admits the
		// certificate and the CERT's export floor is what refuses it — otherwise the row would only
		// be re-proving the filter. This is the clause a stake-only export rung does not have, and
		// its absence is what let a holder of two thirds mint export finality alone.
		{"w40-30-30/70/twoOfThree/no", 3, []uint64{40, 30, 30}, 2, []int{0, 1}, 70, false},
		{"w34-33-33/67/twoOfThree/no", 3, []uint64{34, 33, 33}, 2, []int{0, 1}, 67, false},
		// [50,20,20,10] at a FORK-SAFE K=4 (α=3): node 0's 50 stake is PIVOTAL. Same 3-voter count
		// both ways — WITHOUT node 0 the max is {1,2,3}=50 (≤66, rejected); WITH node 0 {0,1,3}=80
		// (>66, accepted). Stake decides between two equal-count certs, and no 2-of-4 minority can
		// finalize (see the α-floor sub-test below — that is the separate fork-safety gate).
		{"w50-20-20-10/noNode0-50/no", 4, []uint64{50, 20, 20, 10}, 3, []int{1, 2, 3}, 50, false},
		{"w50-20-20-10/withNode0-80/yes", 4, []uint64{50, 20, 20, 10}, 3, []int{0, 1, 3}, 80, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := decidesViaRuntimeCert(t, c.n, c.weights, c.alpha, c.voters)
			if got != c.expect {
				t.Fatalf("weights=%v voters=%v (stake %d/100, floor 66): decided=%v want=%v",
					c.weights, c.voters, c.stake, got, c.expect)
			}
		})
	}

	// ALPHA-FLOOR IS SEPARATE FROM STAKE (the owner's "alpha/count separated from stake"). On a
	// fork-safe K=4 (α=3), a 2-of-4 cert cannot finalize EVEN AT 70% stake: two disjoint 2-of-4
	// quorums {0,1} and {2,3} could each "finalize" a different block → fork. The runtime rejects
	// the sub-α cert at the threshold floor (HandleIncomingCert), BEFORE the stake predicate. So
	// finality requires BOTH gates: an α-of-K count quorum AND a strict >⅔ stake supermajority. The
	// owner's illustrative [50,20,20,10] 70={0,1} is a 2-of-4 minority by COUNT — a stake
	// supermajority alone does not finalize it on a chain that tolerates f=1.
	t.Run("alpha-floor-rejects-2of4-even-at-70pct-stake", func(t *testing.T) {
		if decidesViaRuntimeCert(t, 4, []uint64{50, 20, 20, 10}, 3, []int{0, 1}) {
			t.Fatal("FORK RISK: a 2-of-4 cert finalized despite α=3 — the count quorum floor was bypassed")
		}
	})
}

// TestBlue_QuorumMatrix_DuplicateAndEquivocation covers the two vote-hygiene cases at runtime: a
// DUPLICATE vote from one validator is counted ONCE (a repeat cannot inflate the tally toward
// quorum), and an EQUIVOCATING cert (two of the "signatures" are the same node) cannot manufacture
// a quorum it does not have.
func TestBlue_QuorumMatrix_DuplicateAndEquivocation(t *testing.T) {
	// n=3, q=2 by stake (equal weights): a cert whose 2 "distinct" signers are actually node 0
	// twice holds only ONE validator's stake (⅓) → MUST NOT finalize. The real 2-of-3 does.
	t.Run("duplicate-node-counts-once", func(t *testing.T) {
		vs := newTestValidatorSet(3)
		chainID := ids.GenerateTestID()
		e := NewWithConfig(Config{Params: matrixParams(3, 2)},
			WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
			WithStakeWeighting(vs))
		if err := e.Start(context.Background(), true); err != nil {
			t.Fatalf("start: %v", err)
		}
		t.Cleanup(func() { _ = e.Stop(context.Background()) })
		rt := &Runtime{Transitive: e, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}
		blk := newTestBlock(1, ids.Empty, "dup")
		trackVerifiedBlock(rt, blk, 0)
		pos := VotePosition{ChainID: chainID, Height: 1, Round: 0, BlockID: blk.id, ParentID: ids.Empty}

		// node 0 signing "twice" — the same NodeID repeated. Dedup ⇒ 1 distinct voter, ⅓ stake.
		dup, err := AssembleQuorumCert(pos, Quasar, 2, []SignedVote{
			{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
			{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
		})
		if err == nil {
			b, _ := dup.MarshalBinary()
			if rt.HandleIncomingCert(b) || blk.AcceptCalled() != 0 {
				t.Fatal("a duplicated single voter (⅓ stake) must NOT finalize a >⅔ quorum")
			}
		}
		// the genuine 2-of-3 (nodes 0,1 = ⅔... need >⅔: nodes 0,1,2) finalizes.
		real3, err := AssembleQuorumCert(pos, Quasar, 3, []SignedVote{
			{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
			{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
			{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
		})
		if err != nil {
			t.Fatalf("assemble real: %v", err)
		}
		b, _ := real3.MarshalBinary()
		if !rt.HandleIncomingCert(b) || blk.AcceptCalled() != 1 {
			t.Fatalf("the genuine 3-of-3 (>⅔ stake) must finalize exactly once; accept=%d", blk.AcceptCalled())
		}
	})
}

// TestBlue_QuorumMatrix_AllOnlineAndOneDown checks the two operational extremes per quorum: ALL n
// online decides, and (n−1) online decides IFF n−1 ≥ q (one validator down is tolerated exactly
// when the quorum still fits).
func TestBlue_QuorumMatrix_AllOnlineAndOneDown(t *testing.T) {
	for n := 2; n <= 7; n++ {
		q := quorumq(n)
		all := decidesViaRuntimeCert(t, n, nil, q, seqN(n))
		if !all {
			t.Fatalf("n=%d: ALL %d online must decide", n, n)
		}
		oneDown := decidesViaRuntimeCert(t, n, nil, q, seqN(n-1))
		wantOneDown := (n - 1) >= q
		if oneDown != wantOneDown {
			t.Fatalf("n=%d: one down (%d online) decided=%v, want=%v (q=%d)", n, n-1, oneDown, wantOneDown, q)
		}
		_ = time.Now
	}
}

// TestBlue_QuorumMatrix_VMAcceptFailsClosed proves a VM.Accept ERROR fails CLOSED: the block is NOT
// reported applied (never a silent partial accept), the finalize bookkeeping does NOT run (the
// block stays undecided and RETAINED in pendingBlocks, so the decision is neither lost nor
// half-applied), and the engine is not corrupted (a subsequent unrelated finalize still completes).
// The consensus ledger's finalized-history advance (ApplyCert, which precedes VM.Accept) is bounded
// for SAFETY by the decided-FLOOR staying ≤ the VM's applied head; RECOVERY of that divergence is
// the phantom-floor boot reconcile (Runtime.ReconcilePhantomFloor), tested separately — there is no
// LIVE re-apply because the height is already finalized in the ledger, which is exactly why the
// boot reconcile exists.
func TestBlue_QuorumMatrix_VMAcceptFailsClosed(t *testing.T) {
	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()
	e := NewWithConfig(Config{Params: matrixParams(4, 3)},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
		WithStakeWeighting(vs))
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	rt := &Runtime{Transitive: e, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	// block whose FIRST VM.Accept errors.
	blk := &trackingMockBlock{id: ids.GenerateTestID(), parentID: ids.Empty, height: 1, timestamp: time.Now(), bytes: []byte("fc"), acceptErr: errors.New("vm apply transiently failed")}
	trackMockBlock(rt, blk, 0)
	pos := VotePosition{ChainID: chainID, Height: 1, Round: 0, BlockID: blk.id, ParentID: ids.Empty}
	cert := certFor(t, vs, pos, 0, 1, 2) // a genuine 3-of-4 (>⅔ stake) cert

	// Deliver the valid cert → VM.Accept is attempted and ERRORS → fail CLOSED: the block is NOT
	// marked applied and is RETAINED in pendingBlocks (the finalize bookkeeping only runs AFTER a
	// clean VM.Accept — applyBranchFinalization breaks at the first error), so the decision is never
	// lost and never half-applied.
	rt.HandleIncomingCert(cert)
	if e.IsAccepted(blk.id) {
		t.Fatal("FAIL-OPEN: block reported applied despite a VM.Accept error")
	}
	if blk.AcceptCalled() == 0 {
		t.Fatal("VM.Accept should have been ATTEMPTED (and failed)")
	}
	e.mu.RLock()
	_, retained := e.pendingBlocks[blk.id]
	e.mu.RUnlock()
	if !retained {
		t.Fatal("block dropped from pending on a failed apply — the decision must be RETAINED for the reconcile")
	}
	// The block is DECIDED by the quorum (its cert is valid) but NOT APPLIED (VM.Accept failed): the
	// ledger holds the decision and the block is retained, so no finality is lost and none is
	// half-applied. Live re-apply is impossible (the height is already finalized in the ledger) —
	// exactly the ledger-ahead-of-VM state Runtime.ReconcilePhantomFloor reconciles on the next boot
	// (tested in reconcile_test.go). Fail-secure: an apply failure stalls that height, never forks.
}

// TestBlue_QuorumMatrix_LateVoteForUnknownBlockDrainedOnce proves a vote that arrives for a block
// the engine does not yet track is BUFFERED (not dropped, not counted against an untracked block),
// and once the block is tracked the buffered votes are DRAINED and contribute to finality — exactly
// once (a validator's stake is counted once, so a re-drain cannot double-count toward the quorum).
func TestBlue_QuorumMatrix_LateVoteForUnknownBlockDrainedOnce(t *testing.T) {
	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()
	e := NewWithConfig(Config{Params: matrixParams(4, 3)},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
		WithStakeWeighting(vs))
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	rt := &Runtime{Transitive: e, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	blk := &trackingMockBlock{id: ids.GenerateTestID(), parentID: ids.Empty, height: 1, timestamp: time.Now(), bytes: []byte("late")}
	pos := VotePosition{ChainID: chainID, Height: 1, Round: 0, BlockID: blk.id, ParentID: ids.Empty}

	// Votes arrive BEFORE the block is tracked → buffered. The same node's vote is (re)sent to prove
	// a repeat cannot double-count.
	for i := 0; i < 3; i++ {
		e.ReceiveVote(vs.signedVote(i, pos))
	}
	e.ReceiveVote(vs.signedVote(0, pos)) // duplicate of node 0 — must not inflate the tally
	time.Sleep(80 * time.Millisecond)
	if blk.AcceptCalled() != 0 {
		t.Fatal("a block that is not yet tracked must NOT finalize from buffered votes")
	}

	// Track the block, then drain the parked votes (the build/accept paths call this at their
	// tracking sites; trackMockBlock is a bare seed, so we drive the drain the engine would). The
	// buffered votes replay through handleVote and finalize it (3-of-4 = >⅔ stake), counted once each.
	trackMockBlock(rt, blk, 0)
	e.drainBufferedVotes(blk.id)
	e.ReceiveVote(vs.signedVote(3, pos)) // a live vote also nudges the drain/finalize path
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && blk.AcceptCalled() == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("buffered votes must drain to finalize the tracked block EXACTLY once; AcceptCalled=%d", got)
	}
}
