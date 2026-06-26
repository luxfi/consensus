// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"testing"

	"github.com/luxfi/consensus/config"
)

// TestBFTCommittee_ClampsOversizedPresetToLiveSet is the regression for the
// testnet finality wedge: TestnetParams (K=11, α=8) was assigned to a network
// running only 5 validators, so the α-of-K quorum cert demanded 8 affirmative
// votes from 5 nodes — unreachable — and the chain wedged (every block verified,
// no block ever finalized). bftCommittee must shrink K to the live set and pick a
// reachable BFT quorum.
func TestBFTCommittee_ClampsOversizedPresetToLiveSet(t *testing.T) {
	k, alpha, clamped := bftCommittee(11, 5) // testnet's exact wedge
	if !clamped {
		t.Fatalf("K=11 with 5 validators MUST clamp; got clamped=false")
	}
	if k != 5 {
		t.Errorf("K should clamp to live count 5, got %d", k)
	}
	if alpha != 4 { // ⌊2·5/3⌋+1 = 4 → 4-of-5, a reachable BFT supermajority
		t.Errorf("α for 5 validators should be 4, got %d", alpha)
	}
	if alpha > k {
		t.Errorf("α=%d > K=%d is unsatisfiable — the wedge would persist", alpha, k)
	}
	// BFT α-floor: 2α − K ≥ f+1 with f = ⌊(K-1)/3⌋. For K=5: f=1, 2·4−5=3 ≥ 2. ✓
	if f := (k - 1) / 3; 2*alpha-k < f+1 {
		t.Errorf("α=%d K=%d violates BFT floor 2α−K ≥ f+1 (f=%d)", alpha, k, f)
	}
}

// TestBFTCommittee_LeavesFittingPresetsUntouched proves the clamp never rewrites a
// preset that already fits the validator set — its hand-tuned α (which is NOT
// always ⌊2K/3⌋+1, e.g. LocalParams K=3/α=2) must survive verbatim.
func TestBFTCommittee_LeavesFittingPresetsUntouched(t *testing.T) {
	for _, count := range []int{11, 21, 50} { // count ≥ K for each preset below
		for name, k := range map[string]int{"default": 20, "mainnet": 21, "testnet": 11} {
			if k > count {
				continue
			}
			if _, _, clamped := bftCommittee(k, count); clamped {
				t.Errorf("%s preset (K=%d) with %d validators must NOT clamp", name, k, count)
			}
		}
	}
	// count unknown (-1) or empty (0): never clamp — a missing/empty set must not
	// force K to a degenerate value.
	for _, count := range []int{0, -1} {
		if _, _, clamped := bftCommittee(20, count); clamped {
			t.Errorf("count=%d must not clamp", count)
		}
	}
}

// TestBFTCommittee_FormulaReproducesEveryPreset is the invariant that lets the
// clamp reuse the BFT formula safely: ⌊2K/3⌋+1 equals the α every production
// preset already encodes, so a clamped committee is indistinguishable from a
// preset authored for that exact size.
func TestBFTCommittee_FormulaReproducesEveryPreset(t *testing.T) {
	cases := []struct {
		name        string
		params      config.Parameters
		wantedAlpha int
	}{
		{"DefaultParams", config.DefaultParams(), 14}, // K=20 → 14
		{"MainnetParams", config.MainnetParams(), 15}, // K=21 → 15
		{"TestnetParams", config.TestnetParams(), 8},  // K=11 → 8
		{"LocalBFTParams", config.LocalBFTParams(), 3}, // K=4 → 3
	}
	for _, tc := range cases {
		// Clamp a network whose live set is exactly the preset's K: bftCommittee is
		// invoked with count == K-? Use count = K so the "shrink" path with
		// count==k is a no-op; instead verify the formula directly against α.
		gotAlpha := 2*tc.params.K/3 + 1
		if gotAlpha != tc.wantedAlpha {
			t.Errorf("%s: ⌊2·%d/3⌋+1 = %d, preset α = %d — formula diverges from preset",
				tc.name, tc.params.K, gotAlpha, tc.wantedAlpha)
		}
		if tc.params.AlphaPreference != tc.wantedAlpha {
			t.Errorf("%s: preset AlphaPreference=%d, expected %d (preset drift)",
				tc.name, tc.params.AlphaPreference, tc.wantedAlpha)
		}
	}
}
