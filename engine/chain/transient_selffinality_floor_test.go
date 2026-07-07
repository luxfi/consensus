// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// transient_selffinality_floor_test.go — FIX #1 (v1.35.29): the transient-count
// self-finality fork (mainnet 1085013).
//
// THE FORK: validators.Manager.Count(net) reads len(m.validators[net]), which
// UNDER-reports during a restart window before the P-chain has replayed the staker
// set — it transiently returns 1. The old bftCommittee turned that transient count=1
// into K=1/α=1, and a K==1 engine synthesizes a 1-of-1 finality token that BYPASSES
// the ⅔-by-stake gate (buildSingleValidatorCertLocked) → the lone live node
// self-finalized divergent blocks and forked luxd-0/luxd-1 at 1085013..1085016.
//
// THE FIX (the ROOT): bftCommittee floors a presetK>1 committee at the minimal BFT
// size (K=4/α=3, f=1) even when the live count is transiently 1..3, so the α-of-K
// COUNT gate ALWAYS demands a real BFT quorum a single node can never reach — the
// chain HALTS fail-closed until enough validators resolve, and reclampCommitteeLocked
// grows K up toward presetK as they do. A genuine single-validator chain (presetK≤1)
// is untouched (K stays 1, it finalizes on its own accept, no peer to fork against).
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// TestFix1_BFTCommittee_TransientCountNeverSelfFinalizes is the sizer-level
// fail-without/pass-with. OLD: bftCommittee(presetK>1, count=1) = (1,1) — a
// self-finalizing 1-of-1 committee. NEW: it floors at the minimal BFT committee, so a
// multi-validator preset can NEVER be clamped to α≤1 by a transient count.
func TestFix1_BFTCommittee_TransientCountNeverSelfFinalizes(t *testing.T) {
	// The exact fork inputs: a multi-validator preset whose live set transiently reads 1.
	// The floor is the minimal BFT committee K=4/α=3, never the self-finalizing K=1/α=1.
	for _, tc := range []struct {
		presetK, count   int
		wantK, wantAlpha int
	}{
		{5, 1, 4, 3},  // the pin's "presetK=5 chain, 1 live validator at boot"
		{21, 1, 4, 3}, // mainnet preset, transient count=1 (the 1085013 fork input)
		{11, 1, 4, 3}, // testnet preset, transient count=1
		{21, 2, 4, 3}, // 2 live — still below the BFT floor, still HALTs (not 2-of-2 self-quorum)
		{21, 3, 4, 3}, // 3 live — floored
		{21, 4, 4, 3}, // 4 live — exactly the floor
		{21, 5, 5, 4}, // 5 live — the genuine set clamps normally (4-of-5)
	} {
		k, alpha, clamped := bftCommittee(tc.presetK, tc.count)
		if !clamped {
			t.Errorf("bftCommittee(%d,%d): expected clamped=true", tc.presetK, tc.count)
		}
		if k != tc.wantK || alpha != tc.wantAlpha {
			t.Errorf("bftCommittee(%d,%d) = (K=%d,α=%d); want (K=%d,α=%d)",
				tc.presetK, tc.count, k, alpha, tc.wantK, tc.wantAlpha)
		}
		// THE SAFETY INVARIANT: a multi-validator preset is NEVER clamped to a
		// self-finalizing committee (α≤1 ⇒ one vote finalizes).
		if alpha <= 1 {
			t.Errorf("SELF-FINALITY: bftCommittee(%d,%d) yielded α=%d ≤ 1 — a lone node could self-finalize",
				tc.presetK, tc.count, alpha)
		}
	}

	// Property: across every multi-validator preset and every sub-preset live count,
	// the clamp yields a satisfiable, non-self-finalizing BFT committee (2 ≤ α ≤ K ≤ presetK).
	for presetK := 2; presetK <= 30; presetK++ {
		for count := 1; count < presetK; count++ {
			k, alpha, _ := bftCommittee(presetK, count)
			if alpha < 2 {
				t.Fatalf("bftCommittee(%d,%d): α=%d permits self-finality (want ≥2)", presetK, count, alpha)
			}
			if alpha > k || k > presetK || k < 2 {
				t.Fatalf("bftCommittee(%d,%d) = (K=%d,α=%d): not a valid sub-preset BFT committee", presetK, count, k, alpha)
			}
		}
	}
}

// TestFix1_BFTCommittee_PreservesGenuineSingleAndNormalClamp guards the fix from
// regressing (a) the genuine single-validator path and (b) the normal oversized-preset
// down-clamp (the testnet anti-wedge regression).
func TestFix1_BFTCommittee_PreservesGenuineSingleAndNormalClamp(t *testing.T) {
	// (a) Genuine single validator: presetK≤1 ⇒ K stays 1 (self-finalizes correctly —
	// there is no peer to fork against). presetK≤count ⇒ no clamp.
	for _, count := range []int{1, 5, 21} {
		if k, _, clamped := bftCommittee(1, count); clamped || k != 1 {
			t.Errorf("bftCommittee(1,%d) = (K=%d,clamped=%v); genuine single must stay K=1, unclamped", count, k, clamped)
		}
	}
	// (b) The testnet exact wedge must still shrink to the live set (K=5/α=4).
	for _, tc := range []struct{ presetK, count, wantK, wantAlpha int }{
		{11, 5, 5, 4}, // TestnetParams on 5 validators
		{21, 5, 5, 4}, // MainnetParams on 5 validators
		{21, 7, 7, 5}, // 7 live ⇒ 5-of-7
	} {
		k, alpha, clamped := bftCommittee(tc.presetK, tc.count)
		if !clamped || k != tc.wantK || alpha != tc.wantAlpha {
			t.Errorf("bftCommittee(%d,%d) = (K=%d,α=%d,clamped=%v); want (K=%d,α=%d,true)",
				tc.presetK, tc.count, k, alpha, clamped, tc.wantK, tc.wantAlpha)
		}
	}
	// (c) count unknown/empty: never clamp (a missing set must not degenerate K).
	for _, count := range []int{0, -1} {
		if _, _, clamped := bftCommittee(20, count); clamped {
			t.Errorf("bftCommittee(20,%d) must not clamp", count)
		}
	}
}

// TestFix1_NewRuntime_TransientCountBootsFlooredNotSelfFinalizing drives the REAL
// construction path: a presetK=5 chain whose validator sampler transiently reports 1
// live validator (the restart window) must boot a FLOORED committee (K≥4/α≥3), NOT the
// self-finalizing K=1/α=1 the old sizer produced.
func TestFix1_NewRuntime_TransientCountBootsFlooredNotSelfFinalizing(t *testing.T) {
	self, sampler := makeValidators(1) // Count(net) == 1 — the transient restart read
	params := config.LocalBFTParams()
	params.K = 5 // presetK=5 (a 5-validator chain); the live set momentarily reads 1
	rt := NewRuntime(NetworkConfig{
		ChainID:    ids.GenerateTestID(),
		NetworkID:  ids.Empty,
		NodeID:     self,
		Validators: sampler,
		Params:     &params,
	})
	if got := rt.Transitive.consensus.K(); got != 4 {
		t.Fatalf("transient count=1 with presetK=5 must FLOOR the committee to K=4 (minimal BFT), got K=%d "+
			"(K=1 would be the 1085013 self-finality committee)", got)
	}
	if got := rt.Transitive.consensus.Alpha(); got < 3 {
		t.Fatalf("floored committee α must be ≥3 (BFT quorum), got α=%d — α=1 self-finalizes", got)
	}
	if pk := rt.Transitive.presetK; pk != 5 {
		t.Fatalf("presetK must be preserved as the re-clamp target, got %d", pk)
	}
}

// TestFix1_FlooredCommittee_HaltsWithoutRealQuorum is the downstream proof: a floored
// K=4/α=3 committee backed by a 5-validator stake source NEVER advances finality
// (never VM.Accept, never advance the decided floor) until a real 4-of-5 ⅔-stake
// supermajority signs — a lone self-vote, and even a 3-of-5 count-quorum, HALT.
func TestFix1_FlooredCommittee_HaltsWithoutRealQuorum(t *testing.T) {
	vs := newTestValidatorSet(5) // equal unit stake ⇒ ⅔-of-5 needs 4 voters
	rec := &recordingGossiper{}
	params := config.LocalBFTParams() // K=4/α=3 — the committee the transient floor produces
	e, chainID := newQuorumEngineOpts(t, params, vs, 0, rec, WithStakeWeighting(vs))

	floorBefore := e.consensus.GetDecidedFloor()

	blk := newTestBlock(1, ids.Empty, "floored-halt")
	pos := trackProposal(e, chainID, blk, 0) // inserts own proposal + records THIS node's (node 0) signed accept

	// 1 self-vote — 1/5 = 20% stake. Must HALT (self-finality is exactly this).
	mustNotFinalize(t, e, blk, 1200*time.Millisecond, "lone self-vote (20% stake)")

	// 3 votes {0,1,2} — reaches the α=3 COUNT quorum but only 3/5 = 60% stake. Must HALT:
	// a count quorum is a liveness signal, not finality; the ⅔-stake cert is the authority.
	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos))
	mustNotFinalize(t, e, blk, 1200*time.Millisecond, "3-of-5 count quorum but 60% stake")

	if got := e.consensus.GetDecidedFloor(); got != floorBefore {
		t.Fatalf("decided floor advanced (%d→%d) WITHOUT a real 4-of-5 cert", floorBefore, got)
	}

	// 4th vote {0,1,2,3} — 4/5 = 80% > ⅔. Now a real cert assembles and the block finalizes.
	e.ReceiveVote(vs.signedVote(3, pos))
	mustFinalize(t, e, blk, 3*time.Second, "4-of-5 ⅔-stake supermajority")
}
