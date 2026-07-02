// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// singleSelfSampler is a ValidatorSampler for a genuinely single-validator chain: the only
// validator is this node. Sample returns [self] so RequestVotes filters it out (len(filtered)==0)
// and takes the single-node selfVoter branch; Count==1 marks the live set as one validator.
type singleSelfSampler struct{ self ids.NodeID }

func (s singleSelfSampler) Sample(ids.ID, int) ([]ids.NodeID, error) {
	return []ids.NodeID{s.self}, nil
}
func (s singleSelfSampler) Count(ids.ID) int { return 1 }

// TestBlue_SingleValidator_DecidesThroughFullRuntimePath drives the REAL runtime path a live
// single-validator luxd uses — NewRuntime wires the gossiperProposer + selfVoter, so a build
// runs Propose → RequestVotes → selfVoter → engine.ReceiveVote AND the inline build-loop finalize.
// (The earlier tests used raw New with no proposer, so they never exercised the selfVoter path —
// that is why they passed while the live chain froze.) With VM.Accept succeeding, the block MUST
// DECIDE and the finalized height advance to 1.
func TestBlue_SingleValidator_DecidesThroughFullRuntimePath(t *testing.T) {
	self := ids.GenerateTestNodeID()
	p := config.SingleValidatorParams() // K=1
	blk := &trackingMockBlock{id: ids.GenerateTestID(), parentID: ids.Empty, height: 1, timestamp: time.Now(), bytes: []byte("b1")}
	vm := &trackingMockVM{blocks: []*trackingMockBlock{blk}}

	rt := NewRuntime(NetworkConfig{
		ChainID:      ids.GenerateTestID(),
		NetworkID:    ids.GenerateTestID(),
		NodeID:       self,
		Validators:   singleSelfSampler{self: self},
		Logger:       log.Noop(),
		Gossiper:     &recordingGossiper{}, // all sends return 0 (no peers) — the single-node condition
		VM:           vm,
		Params:       &p,
		VoteVerifier: rejectingVerifier{},        // wired like a value chain; rejects → my n=1 synthesize path
		VoteSigner:   testAuth.signerFor(self),
	})
	ctx := context.Background()
	if err := rt.Start(ctx, true); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = rt.Stop(context.Background()) })

	// Drive the build through the FULL runtime path.
	if err := rt.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	time.Sleep(400 * time.Millisecond)

	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("FULL-RUNTIME single-validator STALL: the block did not DECIDE through the real "+
			"NewRuntime path (Propose→RequestVotes→selfVoter→ReceiveVote + inline finalize); AcceptCalled=%d", got)
	}
	if h, ok := rt.consensus.GetFinalizedHeight(); !ok || h != 1 {
		t.Fatalf("consensus finalized height must be 1 after the single-validator block decides; got (%d,%v)", h, ok)
	}
}

// rejectingVerifier is a VoteVerifier that rejects EVERY signature — the exact runtime
// condition on a fresh single-validator sovereign L1 (zood / Zoo 200200) whose validator set
// is not yet resolvable at the block's P-chain epoch height, so the sole validator's own
// signed self-vote cannot be verified against it.
type rejectingVerifier struct{}

func (rejectingVerifier) VerifyVote(ids.NodeID, []byte, []byte, uint64) bool { return false }

// TestBlue_SingleValidator_DecidesWhenSelfVoteUnverifiable is the n=1 DECIDE-stall regression
// (neo's live zood repro). A K==1 engine whose self-vote CANNOT be verified against its
// (unresolvable) single-validator set MUST still DECIDE — the sole validator's own accept IS
// the 1-of-1 quorum, and FinalizeBranch's per-height gate is the real single-node safety.
//
// PRE-FIX: buildSingleValidatorCertLocked hits its middle branch (a verifier is wired but the
// signed self-vote did not assemble into a verified cert) and returns a ZERO cert, so
// acceptWithCertCore refuses (ErrNoVerifiedQC, discarded by the inline build-loop finalize) —
// the block never decides and the VM re-builds the same height every poll (EVM head frozen).
//
// This reproduces on the SHARED single-node path, so it fails identically on the view-change
// line (v1.32.x) and the finality-admission prod line (v1.34.x). n>1 chains are unaffected:
// they never reach the K()==1 branch.
func TestBlue_SingleValidator_DecidesWhenSelfVoteUnverifiable(t *testing.T) {
	self := ids.GenerateTestNodeID()
	p := config.SingleValidatorParams() // K=1, alpha=1

	// Verifier wired (as on a value chain whose preset K>1 was clamped to the live
	// single-validator count) but REJECTING — the unresolvable-set condition.
	e := New(WithParams(p), WithQuorumCert(ids.Empty, self, rejectingVerifier{}, nil, testAuth.signerFor(self)))
	ctx := context.Background()
	if err := e.Start(ctx, true); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	blk := &trackingMockBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now(),
		bytes:     []byte("b1"),
	}
	vm := &trackingMockVM{blocks: []*trackingMockBlock{blk}}
	e.SetVM(vm)

	// Notify → buildBlocksLocked: the real single-node build+finalize path (finalizes K==1
	// INLINE via acceptWithCertCore).
	if err := e.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	// Allow the poll/finalize a tick (inline finalize is synchronous, but be generous).
	time.Sleep(150 * time.Millisecond)

	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("n=1 DECIDE STALL: the single-validator block must finalize (VM.Accept called once) even "+
			"when its self-vote is unverifiable; got AcceptCalled=%d", got)
	}
	if h, ok := e.consensus.GetFinalizedHeight(); !ok || h != 1 {
		t.Fatalf("consensus must report finalized height 1 after the single-validator block decides, got (%d,%v)", h, ok)
	}
}

// TestBlue_SingleValidator_DecidesWithNoCrypto is the pure --dev K=1 case (no verifier / no
// signer): the synthesized 1-of-1 cert must decide the block. This already worked; it guards
// the fix from regressing the verifier-nil path.
func TestBlue_SingleValidator_DecidesWithNoCrypto(t *testing.T) {
	e := New(WithParams(config.SingleValidatorParams())) // no WithQuorumCert → verifier+signer nil
	ctx := context.Background()
	if err := e.Start(ctx, true); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	blk := &trackingMockBlock{id: ids.GenerateTestID(), parentID: ids.Empty, height: 1, timestamp: time.Now(), bytes: []byte("b1")}
	vm := &trackingMockVM{blocks: []*trackingMockBlock{blk}}
	e.SetVM(vm)

	if err := e.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("pure single-node (no crypto) must finalize via the synthesized 1-of-1 cert; got AcceptCalled=%d", got)
	}
}

// TestBlue_Decentralization_NoUnilateralFinalizeAfterSecondValidator is the RED HIGH 1→N fork
// regression. A chain that LAUNCHES single-validator (K clamped to 1 at construction) must NOT
// keep finalizing unilaterally after it ADDS a validator: K is re-clamped UP to the live count so
// validator-1 switches to the real k-of-N quorum path, and the 1-of-1 synthesize is refused when
// the live set > 1.
func TestBlue_Decentralization_NoUnilateralFinalizeAfterSecondValidator(t *testing.T) {
	self := ids.GenerateTestNodeID()
	// Verifier wired (a value chain) that rejects — so finality can ONLY come from the synthesized
	// 1-of-1 cert (single-validator) or, once decentralized, a real quorum (which never assembles
	// here because the verifier rejects → the block must simply NOT finalize, never fork).
	e := New(WithParams(config.SingleValidatorParams()), WithQuorumCert(ids.Empty, self, rejectingVerifier{}, nil, testAuth.signerFor(self)))
	e.presetK = 20 // the chain's TARGET committee (it will grow toward this as validators join)
	var live int32 = 1
	e.liveValidatorCount = func() int { return int(atomic.LoadInt32(&live)) }

	ctx := context.Background()
	if err := e.Start(ctx, true); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	blk1 := &trackingMockBlock{id: ids.GenerateTestID(), parentID: ids.Empty, height: 1, timestamp: time.Now(), bytes: []byte("b1")}
	blk2 := &trackingMockBlock{id: ids.GenerateTestID(), parentID: blk1.id, height: 2, timestamp: time.Now(), bytes: []byte("b2")}
	vm := &trackingMockVM{blocks: []*trackingMockBlock{blk1, blk2}}
	e.SetVM(vm)

	// Phase 1 — genuine single validator: block 1 finalizes via the synthesized 1-of-1 cert.
	if err := e.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("notify(1): %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if blk1.AcceptCalled() != 1 {
		t.Fatalf("block 1 must finalize while single-validator; AcceptCalled=%d", blk1.AcceptCalled())
	}
	if k := e.consensus.K(); k != 1 {
		t.Fatalf("K must still be 1 while the set is 1; got %d", k)
	}

	// Phase 2 — validator-2 joins the staking set.
	atomic.StoreInt32(&live, 2)
	if err := e.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("notify(2): %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	// The committee must re-clamp UP so validator-1 no longer takes the single-validator path.
	if k := e.consensus.K(); k != 2 {
		t.Fatalf("committee must re-clamp UP to the live count (2) after validator-2 joined; got K=%d", k)
	}
	// And block 2 must NOT finalize unilaterally — no synthesized 1-of-1 cert, and the real 2-of-N
	// quorum never assembles (verifier rejects), so it stays pending (fail-secure: stall, not fork).
	if blk2.AcceptCalled() != 0 {
		t.Fatalf("DECENTRALIZATION FORK: block 2 finalized UNILATERALLY after validator-2 joined "+
			"(AcceptCalled=%d) — validator-1 must require a real 2-of-N quorum", blk2.AcceptCalled())
	}
}

// TestBlue_Decentralization_UnwiredSamplerStillSynthesizes guards the fix from regressing the
// genuine single-validator / --dev path: with no live-count sampler wired, the 1-of-1 synthesize
// (the n=1 fix) is preserved.
func TestBlue_Decentralization_UnwiredSamplerStillSynthesizes(t *testing.T) {
	self := ids.GenerateTestNodeID()
	e := New(WithParams(config.SingleValidatorParams()), WithQuorumCert(ids.Empty, self, rejectingVerifier{}, nil, testAuth.signerFor(self)))
	// presetK=0 and liveValidatorCount=nil (unwired) → no re-clamp, synthesize allowed.
	ctx := context.Background()
	if err := e.Start(ctx, true); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	blk := &trackingMockBlock{id: ids.GenerateTestID(), parentID: ids.Empty, height: 1, timestamp: time.Now(), bytes: []byte("b1")}
	vm := &trackingMockVM{blocks: []*trackingMockBlock{blk}}
	e.SetVM(vm)
	if err := e.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if blk.AcceptCalled() != 1 {
		t.Fatalf("unwired sampler must preserve the single-validator synthesize (n=1 fix); AcceptCalled=%d", blk.AcceptCalled())
	}
}
