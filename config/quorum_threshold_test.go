// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_threshold_test.go — proves the dynamic live-set threshold is DERIVED
// from the strict-⅔ stake rule the cert verifier enforces (TwoThirdsStakeFloor),
// not a hardcoded schedule: the equal-stake closed form equals the heaviest-first
// general computation, the committee is sized to the live set (K=N), the
// 21-validator threshold is 15 (not 14), and a five-validator net never gets an
// oversized K.
package config

import (
	"errors"
	"testing"

	"github.com/luxfi/constants"
)

// TestTwoThirdsStakeFloor pins the single ⅔-floor definition the cert verifier
// and the threshold derivation both read. floor(2·total/3), overflow-safe.
func TestTwoThirdsStakeFloor(t *testing.T) {
	cases := []struct {
		total uint64
		want  uint64
	}{
		{0, 0}, {1, 0}, {2, 1}, {3, 2}, {4, 2}, {5, 3}, {6, 4},
		{9, 6}, {10, 6}, {11, 7}, {21, 14}, {99, 66}, {100, 66},
		// overflow-safety: a total near 2^64 must not wrap.
		{1 << 62, (1 << 62) / 3 * 2}, // 2*(total/3) since total%3==... check below
	}
	for _, tc := range cases {
		if got := TwoThirdsStakeFloor(tc.total); got != tc.want {
			// Recompute expected with the exact formula for the large case.
			q, r := tc.total/3, tc.total%3
			exp := 2 * q
			if r == 2 {
				exp++
			}
			if got != exp {
				t.Errorf("TwoThirdsStakeFloor(%d)=%d, want %d", tc.total, got, exp)
			}
		}
	}
	// A supermajority must STRICTLY EXCEED the floor: for total=21, 14 is NOT a
	// supermajority (14/21=66.67% ≤ ⅔ floor of 14), 15 IS.
	if !(15 > TwoThirdsStakeFloor(21)) {
		t.Fatal("15 must strictly exceed the ⅔ floor of 21")
	}
	if 14 > TwoThirdsStakeFloor(21) {
		t.Fatal("14 must NOT strictly exceed the ⅔ floor of 21 (14/21 = 66.67%)")
	}
}

// TestEqualStakeMatchesWeighted proves the equal-stake closed form
// (floor(2N/3)+1) equals the heaviest-first general computation over N unit
// weights — the two definitions of α cannot diverge, so the convenience form is
// not a parallel hardcode.
func TestEqualStakeMatchesWeighted(t *testing.T) {
	for n := 1; n <= 64; n++ {
		ones := make([]uint64, n)
		for i := range ones {
			ones[i] = 1
		}
		closed := EqualStakeSupermajorityThreshold(n)
		general := WeightedSupermajorityThreshold(ones)
		if closed != general {
			t.Errorf("n=%d: closed form %d != general %d", n, closed, general)
		}
		// And both must satisfy the strict-⅔ predicate exactly: the closed-form
		// count's stake exceeds the floor, the count-1 does not.
		total := uint64(n)
		floor := TwoThirdsStakeFloor(total)
		if uint64(closed) <= floor {
			t.Errorf("n=%d: alpha=%d stake %d does not exceed floor %d", n, closed, closed, floor)
		}
		if closed > 1 && uint64(closed-1) > floor {
			t.Errorf("n=%d: alpha-1=%d stake should NOT exceed floor %d", n, closed-1, floor)
		}
	}
}

// TestKEqualsLiveValidatorSet proves FeasibleParams sizes K to the live
// validator count for every network — there is no per-tier K schedule.
func TestKEqualsLiveValidatorSet(t *testing.T) {
	nets := []uint32{constants.MainnetID, constants.TestnetID, constants.DevnetID, constants.LocalID, 8675309 /*sovereign L1*/}
	for _, net := range nets {
		for _, n := range []int{4, 5, 7, 11, 16, 21, 50} {
			p := FeasibleParams(net, n)
			if p.K != n {
				t.Errorf("net=%d n=%d: K=%d, want K=n=%d", net, n, p.K, n)
			}
		}
	}
	// Below the minimal BFT committee, K floors at 4 (fail to a SAFE small set).
	for _, n := range []int{0, 1, 2, 3} {
		p := FeasibleParams(constants.DevnetID, n)
		if p.K != 4 {
			t.Errorf("n=%d: K=%d, want clamp to 4", n, p.K)
		}
	}
}

// TestAlphaSatisfiesVerifyWeighted proves the α FeasibleParams emits is exactly
// the minimum equal-stake count that strictly exceeds the ⅔ floor — i.e. α votes
// of unit stake clear the supermajority and α−1 do not. This is the property the
// cert verifier (VerifyWeighted) enforces, so α is derived from the SAME rule.
func TestAlphaSatisfiesVerifyWeighted(t *testing.T) {
	for _, n := range []int{4, 5, 7, 11, 16, 21, 33, 50} {
		p := FeasibleParams(constants.MainnetID, n)
		alpha := p.AlphaPreference
		total := uint64(n)                  // equal unit stake
		floor := TwoThirdsStakeFloor(total) // the verifier's accept threshold
		// α voters (unit stake) must STRICTLY EXCEED the floor (VerifyWeighted passes).
		if uint64(alpha) <= floor {
			t.Errorf("n=%d: alpha=%d stake %d must exceed ⅔ floor %d (VerifyWeighted would reject)", n, alpha, alpha, floor)
		}
		// α−1 voters must NOT exceed it (so α is the MINIMUM that finalizes) —
		// EXCEPT where the BFT overlap floor raised α above the strict-⅔ minimum
		// (only relevant if they differ; for equal stake strict-⅔ ≥ overlap, so
		// they coincide and α−1 must fail).
		if alpha == EqualStakeSupermajorityThreshold(n) {
			if alpha > 1 && uint64(alpha-1) > floor {
				t.Errorf("n=%d: alpha-1=%d should NOT exceed ⅔ floor %d (alpha not minimal)", n, alpha-1, floor)
			}
		}
		// α must also satisfy the BFT overlap bound enforced by Valid().
		if err := p.Valid(); err != nil {
			t.Errorf("n=%d: FeasibleParams not Valid(): %v", n, err)
		}
	}
}

// TestTwentyOneThresholdMatchesVerifier is the headline proof that α is DERIVED,
// not hardcoded: for 21 equal-stake validators the threshold MUST be 15 (not 14).
// 14/21 = 66.666…% does NOT strictly exceed ⅔; 15/21 = 71.4% does. The value is
// computed by the general heaviest-first routine over 21 unit weights — the same
// routine the closed form mirrors — so it cannot be a magic number.
func TestTwentyOneThresholdMatchesVerifier(t *testing.T) {
	ones := make([]uint64, 21)
	for i := range ones {
		ones[i] = 1
	}
	got := WeightedSupermajorityThreshold(ones)
	if got != 15 {
		t.Fatalf("21-validator equal-stake threshold = %d, want 15 (14 fails strict >⅔)", got)
	}
	// Prove it against the verifier's floor directly.
	floor := TwoThirdsStakeFloor(21) // = 14
	if !(15 > floor) {
		t.Fatal("15 must strictly exceed floor(2*21/3)=14")
	}
	if 14 > floor {
		t.Fatal("14 must NOT strictly exceed floor(2*21/3)=14 — this is why α=15, not 14")
	}
	// FeasibleParams must yield K=21, α=15 (strict-⅔ dominates the overlap floor 14).
	p := FeasibleParams(constants.MainnetID, 21)
	if p.K != 21 || p.AlphaPreference != 15 || p.AlphaConfidence != 15 {
		t.Fatalf("FeasibleParams(mainnet,21) = K%d/α%d, want K21/α15", p.K, p.AlphaPreference)
	}
	// And the BFT overlap floor alone would have under-set it to 14 — proving the
	// strict-⅔ rule is what binds here.
	if floorOverlap := (Parameters{K: 21}).bftQuorumFloor(); floorOverlap != 14 {
		t.Fatalf("BFT overlap floor for K=21 = %d, want 14 (so 15 comes from the ⅔ rule, not overlap)", floorOverlap)
	}
}

// TestNoOversizedKForFiveNodeNet proves the outage cause is gone: a five-validator
// net (mainnet, testnet OR devnet) gets K=5/α=4 — NOT the retired oversized
// mainnet K=21/α=15 or testnet K=11/α=8 that a 5-peer set could never satisfy.
func TestNoOversizedKForFiveNodeNet(t *testing.T) {
	for _, net := range []uint32{constants.MainnetID, constants.TestnetID, constants.DevnetID, constants.LocalID} {
		p := FeasibleParams(net, 5)
		if p.K != 5 {
			t.Errorf("net=%d: K=%d, want 5 (no oversized committee)", net, p.K)
		}
		if p.AlphaPreference != 4 || p.AlphaConfidence != 4 {
			t.Errorf("net=%d: α=%d, want 4 (4-of-5 finalizes, 1 laggard tolerated)", net, p.AlphaPreference)
		}
		// 4-of-5 must be Byzantine-safe (overlap bound) and a valid config.
		if err := p.Valid(); err != nil {
			t.Errorf("net=%d: K=5/α=4 not Valid(): %v", net, err)
		}
		// And it must clear the LIVE-aware value backstop the manager uses.
		if err := p.ValidateForLiveValueNetwork(net, 5); err != nil {
			t.Errorf("net=%d: K=5/α=4 rejected by live backstop with liveN=5: %v", net, err)
		}
	}
}

// TestLiveValueNetworkRelaxesStaticFloor proves the live-aware backstop admits a
// 5-validator mainnet (K=5) — which the STATIC ValidateForValueNetwork rejects
// (K<11) — while still rejecting genuinely unsafe committees (K=3, f=0) and an
// under-sized committee (K below the live set).
func TestLiveValueNetworkRelaxesStaticFloor(t *testing.T) {
	// 5-validator mainnet: static floor rejects, live-aware admits.
	five := FeasibleParams(constants.MainnetID, 5)
	if err := five.ValidateForValueNetwork(constants.MainnetID); !errors.Is(err, ErrKTooLowForMainnet) {
		t.Fatalf("static floor must reject K=5 on mainnet, got %v", err)
	}
	if err := five.ValidateForLiveValueNetwork(constants.MainnetID, 5); err != nil {
		t.Fatalf("live-aware floor must ADMIT K=5 on a 5-validator mainnet, got %v", err)
	}
	// f=0 committee is still rejected for value regardless of live count.
	k3 := Parameters{K: 3, Alpha: 0.67, Beta: 2, AlphaPreference: 2, AlphaConfidence: 2}
	if err := k3.ValidateForLiveValueNetwork(constants.MainnetID, 3); !errors.Is(err, ErrKTooLowForValue) {
		t.Fatalf("K=3 (f=0) must be rejected for value even live-aware, got %v", err)
	}
	// An operator that under-samples the live set (K=4 while 11 validators exist on
	// mainnet) is rejected: effective floor = min(11, 11) = 11 > 4.
	k4 := Parameters{K: 4, Alpha: 0.75, Beta: 2, AlphaPreference: 3, AlphaConfidence: 3}
	if err := k4.ValidateForLiveValueNetwork(constants.MainnetID, 11); !errors.Is(err, ErrKBelowLiveFloor) {
		t.Fatalf("K=4 under-sampling an 11-validator mainnet must be rejected, got %v", err)
	}
	// A full 21-validator mainnet at K=21 is admitted (≥ tier floor 11).
	full := FeasibleParams(constants.MainnetID, 21)
	if err := full.ValidateForLiveValueNetwork(constants.MainnetID, 21); err != nil {
		t.Fatalf("K=21 on a 21-validator mainnet must be admitted, got %v", err)
	}
}
