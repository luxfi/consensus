// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// The transient-count self-finality floor.
//
// validators.Manager.Count(net) reads the live staker set, which under-reports
// while the P-chain is still replaying it after a restart — it can transiently
// read 1. Sizing the committee straight off that count gives K=1/α=1, and a K==1
// engine synthesizes a 1-of-1 finality token that bypasses the ⅔-by-stake gate
// (buildSingleValidatorCertLocked): the one live node finalizes alone, and whatever
// block it picks in that window is a fork the rest of the set never voted for.
//
// So bftCommittee floors a presetK>1 committee at the minimal BFT size (K=4/α=3,
// f=1) even when the live count reads 1..3. The α-of-K count gate then always
// demands a quorum one node cannot reach: the chain halts fail-closed until enough
// validators resolve, and reclampCommitteeLocked grows K back toward presetK as
// they do. A genuine single-validator chain (presetK≤1) is untouched — K stays 1
// and it finalizes on its own accept, having no peer to fork against.
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// TestBFTCommittee_TransientCountNeverSelfFinalizes pins the sizer: a
// multi-validator preset whose live count reads below the BFT floor sizes to the
// floor, never to a self-finalizing 1-of-1 committee.
func TestBFTCommittee_TransientCountNeverSelfFinalizes(t *testing.T) {
	// A multi-validator preset whose live set transiently reads short. The floor is
	// the minimal BFT committee K=4/α=3, never the self-finalizing K=1/α=1.
	for _, tc := range []struct {
		presetK, count   int
		wantK, wantAlpha int
	}{
		{5, 1, 4, 3},  // a 5-validator chain, 1 live validator at boot
		{21, 1, 4, 3}, // mainnet preset, live count transiently 1
		{11, 1, 4, 3}, // testnet preset, live count transiently 1
		{21, 2, 4, 3}, // 2 live — still below the BFT floor, still halts (not a 2-of-2 self-quorum)
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
		// The safety invariant: a multi-validator preset is never clamped to a
		// self-finalizing committee (α≤1 ⇒ one vote finalizes).
		if alpha <= 1 {
			t.Errorf("self-finality: bftCommittee(%d,%d) yielded α=%d ≤ 1 — a lone node could self-finalize",
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

// TestBFTCommittee_GenuineSingleAndNormalClamp covers the two paths the floor must
// leave alone: the genuine single-validator chain, and the ordinary down-clamp of an
// oversized preset onto a smaller live set.
func TestBFTCommittee_GenuineSingleAndNormalClamp(t *testing.T) {
	// (a) Genuine single validator: presetK≤1 ⇒ K stays 1 (self-finalizes correctly —
	// there is no peer to fork against). presetK≤count ⇒ no clamp.
	for _, count := range []int{1, 5, 21} {
		if k, _, clamped := bftCommittee(1, count); clamped || k != 1 {
			t.Errorf("bftCommittee(1,%d) = (K=%d,clamped=%v); genuine single must stay K=1, unclamped", count, k, clamped)
		}
	}
	// (b) An oversized preset still shrinks to the live set (K=5/α=4).
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

// TestNewRuntime_TransientCountBootsFlooredCommittee drives the real construction
// path: a presetK=5 chain whose validator sampler transiently reports 1 live
// validator boots the floored committee (K≥4/α≥3), not a self-finalizing K=1/α=1.
func TestNewRuntime_TransientCountBootsFlooredCommittee(t *testing.T) {
	self, sampler := makeValidators(1) // Count(net) == 1 — the transient short read
	params := config.LocalBFTParams()
	params.K = 5 // a 5-validator chain whose live set momentarily reads 1
	rt := NewRuntime(NetworkConfig{
		ChainID:    ids.GenerateTestID(),
		NetworkID:  ids.Empty,
		NodeID:     self,
		Validators: sampler,
		Params:     &params,
	})
	if got := rt.Transitive.consensus.K(); got != 4 {
		t.Fatalf("transient count=1 with presetK=5 must floor the committee to K=4 (minimal BFT), got K=%d "+
			"(K=1 is the self-finalizing committee)", got)
	}
	if got := rt.Transitive.consensus.Alpha(); got < 3 {
		t.Fatalf("floored committee α must be ≥3 (BFT quorum), got α=%d — α=1 self-finalizes", got)
	}
	if pk := rt.Transitive.presetK; pk != 5 {
		t.Fatalf("presetK must be preserved as the re-clamp target, got %d", pk)
	}
}

// TestFlooredCommittee_NovaAcceptsAtMajorityQuasarAtTwoThirds carries the floor
// through both tiers: a floored K=4/α=3 committee backed by a 5-validator stake source
//   - halts on a lone self-vote (below the Nova majority NovaQuorum=3 — what the floor
//     exists for: a single node can never accept),
//   - Nova-accepts a 3-of-5 bare majority (local execution — surviving 3/5 keeps the
//     chain live), but
//   - reaches the export (Quasar) tier only at a 4-of-5 ⅔-stake supermajority.
func TestFlooredCommittee_NovaAcceptsAtMajorityQuasarAtTwoThirds(t *testing.T) {
	vs := newTestValidatorSet(5) // equal unit stake ⇒ ⅔-of-5 needs 4 voters
	rec := &recordingGossiper{}
	params := config.LocalBFTParams() // K=4/α=3 — the committee the transient floor produces
	e, chainID := newQuorumEngineOpts(t, params, vs, 0, rec, WithStakeWeighting(vs))

	blk := newTestBlock(1, ids.Empty, "floored-halt")
	pos := trackProposal(e, chainID, blk, 0) // inserts own proposal + records THIS node's (node 0) signed accept

	// 1 self-vote — below the Nova majority (NovaQuorum(4)=3), so it halts: lone
	// self-finality is what the floor prevents, and it stays prevented at the Nova tier.
	mustNotFinalize(t, e, blk, 1200*time.Millisecond, "lone self-vote (below Nova majority)")

	// 3 votes {0,1,2} — the bare majority NovaQuorum=3. Nova accepts (local execution). But 3/5 =
	// 60% stake ≤ ⅔, so no Quasar export yet (the degraded mode).
	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos))
	mustFinalize(t, e, blk, 2*time.Second, "3-of-5 bare majority (Nova local accept)")
	mustNotQuasar(t, e, blk, 500*time.Millisecond, "3-of-5 = 60% stake (export gate)")

	// 4th vote {0,1,2,3} — 4/5 = 80% > ⅔. The export (Quasar) cert now forms.
	e.ReceiveVote(vs.signedVote(3, pos))
	mustQuasar(t, e, blk, 3*time.Second, "4-of-5 ⅔-stake supermajority (Quasar export)")
}
