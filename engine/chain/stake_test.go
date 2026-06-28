// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// red_processpending_stake_bypass_test.go — RED adversarial repro.
//
// CLAIM UNDER ATTACK (Blue): "the cert is the binding finality gate; on a
// stake-weighted chain a coalition holding ≤⅔ stake can never finalize because
// assembleCertLocked.VerifyWeighted refuses." This is TRUE for the cert path
// (tryFinalizeBlock). It is FALSE for the count-only path: processPendingBlocks
// (the pollLoop's finalizer) Accepts a block purely on consensus.IsAccepted(),
// which ChainConsensus sets true on acceptVotes>=alpha — a raw COUNT, NO stake
// check, NO cert. So a low-stake/high-count coalition finalizes via the pollLoop
// even though the cert path correctly refused it.
//
// This mirrors the EXISTING TestWeighted_LowStakeCoalitionRejected, but instead
// of feeding a pre-assembled cert to HandleIncomingCert (the path that DOES check
// stake), it drives SIGNED VOTES through the normal handleVote→ProcessVote path
// (exactly what a real proposer node does when peers vote) and then lets the live
// engine's own pollLoop finalize. No forged anything: every vote is a real signed
// accept from a real validator.
package chain

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// TestRED_ProcessPendingFinalizesSubTwoThirdsStake is the CRITICAL repro.
//
// Setup: 5 validators, EQUAL vote weight (so α=4 by count) but SKEWED stake
// {96,1,1,1,1} (total 100). The four low-stake validators {1,2,3,4} sign accepts:
//   - count = 4 = α  → ChainConsensus marks block.accepted=true (count gate)
//   - stake = 4/100  → far below floor(2·100/3)=66; VerifyWeighted MUST refuse
//
// The proposer (vdr 0, holding 96 stake) does NOT vote, so no ⅔-stake quorum
// exists. Correct behavior: the block NEVER finalizes (VM.Accept never runs).
//
// Observed behavior: processPendingBlocks (pollLoop) calls VM.Accept on the block
// because IsAccepted()==true — a sub-⅔-stake, cert-less finalization. That is a
// consensus SAFETY violation: any chain with unequal stake can be force-finalized
// by a low-stake majority-of-COUNT coalition.
func TestRED_ProcessPendingFinalizesSubTwoThirdsStake(t *testing.T) {
	vs := newTestValidatorSet(5)
	// EQUAL vote weight → α=4; SKEWED stake → 4 low-stake voters hold only 4/100.
	skew := newStakeMap(vs, 96, 1, 1, 1, 1)

	rec := &recordingGossiper{}
	// Wire the engine EXACTLY as the node does for a value/PoS chain: stake-weighted.
	// K=5/α=4 via FeasibleParams (the shipped sizer).
	e, chainID := newQuorumEngineOpts(t, dyn5(), vs, 0, rec, WithStakeWeighting(skew))

	blk := newTestBlock(1, ids.Empty, "stake-bypass")
	// Track as a VERIFIED, NON-OWN pending block (mirrors trackVerifiedBlock): no
	// self-vote is auto-counted, so the ONLY accepts are the four low-stake peers.
	// The heavy node (vdr 0, 96 stake) abstains — there is genuinely no ⅔-stake
	// quorum. This is the strongest form of the attack.
	cb := &Block{id: blk.id, parentID: blk.parentID, height: blk.height, timestamp: blk.timestamp.Unix(), data: blk.bytes}
	_ = e.consensus.AddBlock(context.Background(), cb)
	e.mu.Lock()
	e.pendingBlocks[blk.id] = &PendingBlock{ConsensusBlock: cb, VMBlock: blk, ProposedAt: time.Now(), Round: 0}
	e.mu.Unlock()
	pos := VotePosition{ChainID: chainID, Height: blk.height, Round: 0, BlockID: blk.id, ParentID: blk.parentID}

	// Four low-stake validators {1,2,3,4} sign real accepts. count=4=α, stake=4/100.
	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos))
	e.ReceiveVote(vs.signedVote(3, pos))
	e.ReceiveVote(vs.signedVote(4, pos))

	// The COUNT gate flips IsAccepted=true (acceptVotes=4>=α=4) with NO stake check.
	// The cert path (tryFinalizeBlock) correctly refuses (VerifyWeighted: 4 ≤ 66).
	// SAFETY REQUIREMENT: the block must NOT be VM-accepted — 4/100 stake is not a
	// ⅔ supermajority. If the pollLoop's processPendingBlocks accepts it, that is a
	// finalize-without-cert / sub-⅔-stake finality bug.
	if waitFor(2*time.Second, func() bool { return blk.AcceptCalled() >= 1 }) {
		t.Fatalf("CRITICAL SAFETY VIOLATION: block finalized (VM.Accept ran %d×) with a "+
			"4/100-stake coalition — processPendingBlocks accepted on count-α IsAccepted "+
			"without the ⅔-stake cert. accepted=%v", blk.AcceptCalled(), e.IsAccepted(blk.id))
	}

	// And confirm NO valid stake-weighted cert was ever gossiped (proving the cert
	// path agreed this should not finalize — only the count path disagreed).
	rec.mu.Lock()
	gotCerts := len(rec.certs)
	rec.mu.Unlock()
	if gotCerts != 0 {
		t.Fatalf("no ⅔-stake cert should exist for a 4/100 coalition, got %d gossiped", gotCerts)
	}
}
