// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// byzantine_scale_red_test.go — RED adversarial Byzantine-fault suite. It attacks the
// two invariants that must hold at ANY size:
//
//	SAFETY   — no two conflicting blocks finalize at one height, EVEN under a full
//	           equivocating α-quorum (choke #3 / the 1085013 fork family).
//	LIVENESS — progress resumes whenever > ⅔ of the committee is honest+online, and the
//	           chain HALTS fail-closed (never forks) at exactly ⅔ or below (choke #1/#3).
//
// The ⅔ boundary is exercised at n=7 (α = ⌊2·7/3⌋+1 = 5), a scale not covered by the
// existing n=5 suite: 5 online = the exact quorum (finalizes), 4 online = below quorum
// (must halt). This pins the live-set liveness/safety knife-edge the dynamic committee
// sizer creates.
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// prodParams7 is the n=7 production-shaped BFT committee: K=7, α=5 (⌊2·7/3⌋+1). With two
// validators down the remaining 5 are the EXACT quorum; with three down (4 up) quorum is
// unreachable and the chain must HALT (no fork). RoundTO parked long so finalization is
// driven by the emergent gossip pass, not the re-poll ticker.
func prodParams7() config.Parameters {
	p := prodParams5()
	p.K = 7
	p.AlphaPreference = 5
	p.AlphaConfidence = 5
	return p
}

// TestRedByzantine_SilentValidators_TwoThirdsBoundary_N7 is the crash-fault liveness/
// safety knife-edge at n=7. Silent (crash-failed) validators are modeled by net.down.
func TestRedByzantine_SilentValidators_TwoThirdsBoundary_N7(t *testing.T) {
	// > ⅔ ONLINE (5 of 7 = exactly α): the chain MUST finalize a single head.
	t.Run("five_online_finalizes", func(t *testing.T) {
		net := newSimNet(t, 7, prodParams7())
		net.down(5)
		net.down(6) // 5 up: nodes {0,1,2,3,4} — the exact α=5 quorum
		blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "n7-five-online")
		net.build(0, blk)

		if !waitFor(emergeTO, func() bool {
			all, fork := net.finalizedEverywhere(blk)
			return all && !fork
		}) {
			t.Fatalf("LIVENESS: 5-of-7 online (exact α=5) must finalize %s, got heads=%v up=%d",
				blk.ID(), net.headsAtHeight(1), net.upCount())
		}
		if seen := net.headsAtHeight(1); len(seen) != 1 {
			t.Fatalf("SAFETY: divergent heads with 5 online: %v", seen)
		}
	})

	// < MAJORITY ONLINE (3 of 7 < NovaQuorum(7)=4): the Nova accept quorum is unreachable. The
	// chain MUST HALT (fail-closed), never accept, never fork. This is the self-finality floor at
	// the fleet level — and under v1.36 the floor is the MAJORITY (NovaQuorum), not the ⅔ quorum:
	// a 4-of-7 majority DOES accept (Nova, "survive with a majority"), but a 3-of-7 below-majority
	// set produces NO cert (the 1085013 fault family stays closed at the Nova tier).
	t.Run("below_majority_halts_safe", func(t *testing.T) {
		net := newSimNet(t, 7, prodParams7())
		net.down(3)
		net.down(4)
		net.down(5)
		net.down(6) // 3 up: {0,1,2} < NovaQuorum(7)=4
		blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "n7-three-online")
		net.build(0, blk)

		// Give it real time to (incorrectly) accept if the floor were broken.
		accepted := waitFor(3*time.Second, func() bool { return len(net.headsAtHeight(1)) > 0 })
		if accepted {
			t.Fatalf("SAFETY/FLOOR: 3-of-7 online (below NovaQuorum=4) MUST HALT, but a head accepted: %v — "+
				"a below-majority online set self-accepted (the 1085013 fault family)", net.headsAtHeight(1))
		}
	})
}

// TestRedByzantine_EquivocatingQuorum_CannotReFinalizeDecidedHeight is the sharpest
// safety probe: after height 1 is legitimately finalized on block A by a real 4-of-5
// ⅔-stake quorum, a FULL equivocating α-quorum (validators 0..3 re-signing a conflicting
// sibling B at the SAME height) must NOT produce a second finalize. The per-height
// single-finalize invariant (decided-floor gate + non-equivocation) is the last line
// against the 1085013 divergent-finalize fork. A second finalized head here is a CRITICAL
// safety break.
func TestRedByzantine_EquivocatingQuorum_CannotReFinalizeDecidedHeight(t *testing.T) {
	vs := newTestValidatorSet(5) // equal unit stake ⇒ ⅔-of-5 needs 4 signers
	rec := &recordingGossiper{}
	params := config.LocalBFTParams()
	params.K = 5
	params.AlphaPreference = 4
	params.AlphaConfidence = 4
	e, chainID := newQuorumEngineOpts(t, params, vs, 0, rec, WithStakeWeighting(vs))

	// --- Legitimately finalize A at height 1 (real 4-of-5 quorum) ---
	blkA := newTestBlock(1, ids.Empty, "equivocation-A")
	posA := trackProposal(e, chainID, blkA, 0) // own vote (node 0) recorded
	e.ReceiveVote(vs.signedVote(1, posA))
	e.ReceiveVote(vs.signedVote(2, posA))
	e.ReceiveVote(vs.signedVote(3, posA)) // 4-of-5 → A finalizes
	mustFinalize(t, e, blkA, 3*time.Second, "legitimate 4-of-5 ⅔-stake quorum for A")

	decidedHead, ok := e.consensus.FinalizedBlockAtHeight(1)
	if !ok || decidedHead != blkA.id {
		t.Fatalf("precondition: height 1 must be finalized on A (%s), got (%s, ok=%v)", blkA.id, decidedHead, ok)
	}

	// --- Byzantine: a full α-quorum equivocates onto sibling B at the SAME height ---
	blkB := newTestBlock(1, ids.Empty, "equivocation-B") // distinct id, same height/parent
	posB := trackProposal(e, chainID, blkB, 0)           // track B (models the Byzantine own-proposal)
	// Raw equivocating votes bypass the test set's honest vote-once discipline: validators
	// 1..3 (who already signed A) re-sign B. Signatures are valid (vs.sign signs any position),
	// so ONLY the engine's per-height guard + decided-floor can stop the second finalize.
	for _, i := range []int{1, 2, 3} {
		e.ReceiveVote(Vote{
			BlockID:   blkB.id,
			NodeID:    vs.nodeID(i),
			Accept:    true,
			SignedAt:  time.Now(),
			Signature: vs.sign(i, posB),
			ParentID:  blkB.parentID,
			Round:     posB.Round,
		})
	}

	// B MUST NOT finalize — height 1 is already decided on A.
	mustNotFinalize(t, e, blkB, 2*time.Second, "equivocating 4-of-5 quorum onto sibling B at a DECIDED height")

	// And the finalized head at height 1 is STILL A (no silent flip).
	if head, ok := e.consensus.FinalizedBlockAtHeight(1); !ok || head != blkA.id {
		t.Fatalf("SAFETY BREAK: height-1 head changed after equivocation — was A=%s, now (%s, ok=%v) — FORK",
			blkA.id, head, ok)
	}
}
