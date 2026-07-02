// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

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
