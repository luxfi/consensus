// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// committee_sizer_equivalence_test.go — RED item #3: eliminate the LATENT DIVERGENCE
// between the two committee sizers by proving they are EQUIVALENT (a drift-guard).
//
// Two sizers exist:
//   - ACTIVE (deployed): a network preset (MainnetParams K=21) → bftCommittee clamps K
//     DOWN to the live count, floored at the minimal BFT committee (minBFTCommittee=4) —
//     the path node/chains/quorum.go selects and engine/chain effectiveCommittee applies.
//   - REFERENCE (unwired, tests only): config.FeasibleParams(networkID, n) sizes K=n
//     directly (K<4 floored to 4), α = strict-⅔ supermajority.
//
// RED flagged the risk that these two could DRIFT. They must not: both derive α from the
// SINGLE ⅔ definition (bftAlpha = config.TwoThirdsStakeFloor+1 = EqualStakeSupermajority)
// and both floor a sub-BFT live set at K=4. This test LOCKS that: for the MainnetParams
// preset (K=21) at every live count n, the ACTIVE clamp must yield the IDENTICAL (K,α) the
// REFERENCE FeasibleParams emits — so a future edit to either that breaks the equivalence
// fails here. (Decision: keep the ACTIVE path as canonical; FeasibleParams stays the
// analytical reference the tests check against — one behavior, two proven-equal expressions.)
package chain

import (
	"testing"

	"github.com/luxfi/consensus/config"
)

func TestCommitteeSizers_ActiveClampEqualsFeasibleReference(t *testing.T) {
	const presetK = 21 // MainnetParams Snowman sample size
	// MainnetID for FeasibleParams' network-timing selection (timing is irrelevant to K/α here).
	const mainnetID uint32 = 1

	// EQUIVALENCE holds on the CLAMP RANGE (live count n ≤ presetK): both sizers yield
	// (max(n,4), strict-⅔). At n == presetK both give the full committee (21,15).
	for n := 1; n <= presetK; n++ {
		ref := config.FeasibleParams(mainnetID, n) // reference: K=max(n,4), α=strict-⅔

		// ACTIVE path: preset clamped to the live count. At n == presetK it keeps the preset
		// verbatim (clamped=false, α from the preset = MainnetParams α); below, it clamps.
		gotK, gotAlpha, clamped := bftCommittee(presetK, n)
		if !clamped {
			// Only n == presetK reaches here (n < presetK always clamps). The full committee.
			gotK, gotAlpha = presetK, int(config.MainnetParams().AlphaConfidence)
		}

		if gotK != ref.K {
			t.Fatalf("n=%d: sizer DIVERGENCE on K — active=%d, FeasibleParams=%d", n, gotK, ref.K)
		}
		if gotAlpha != ref.AlphaConfidence {
			t.Fatalf("n=%d: sizer DIVERGENCE on α — active=%d, FeasibleParams=%d "+
				"(both must derive from the single ⅔ supermajority)", n, gotAlpha, ref.AlphaConfidence)
		}
		// The floor holds identically: a sub-BFT live set never sizes below K=4/α=3 (1085013 guard).
		if n < 4 && (gotK != 4 || gotAlpha != 3) {
			t.Fatalf("n=%d: sub-BFT live set must floor at K=4/α=3, got K=%d/α=%d", n, gotK, gotAlpha)
		}
	}

	// ABOVE the preset (n > presetK) the two INTENTIONALLY differ and that is not divergence:
	// the ACTIVE path treats presetK as a MAX (a bounded sample never grows past the preset),
	// while FeasibleParams sizes K=n unbounded. Lock that intentional cap so it stays deliberate.
	for _, n := range []int{presetK + 1, presetK + 5} {
		if _, _, clamped := bftCommittee(presetK, n); clamped {
			t.Fatalf("n=%d > presetK: active path must NOT clamp UP past the preset (it is a sample cap)", n)
		}
		if ref := config.FeasibleParams(mainnetID, n); ref.K != n {
			t.Fatalf("n=%d: FeasibleParams (unbounded reference) must size K=n=%d, got %d", n, n, ref.K)
		}
	}
}
