// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"errors"
	"testing"
)

// TestQuorumZeroAlphaRejected is the regression for the accept-on-zero-votes hole.
//
// The chain engine's accept predicate is literally `acceptVotes() >= alpha`
// (engine/chain/topological.go), so AlphaPreference=0 satisfies it with an EMPTY
// vote set: every block finalizes on every node with no quorum at all, and two
// partitioned nodes independently finalize conflicting blocks. Valid() used to gate
// its α checks behind `AlphaPreference != 0`, so the single value that destroys
// safety was the single value that skipped validation.
func TestQuorumZeroAlphaRejected(t *testing.T) {
	// K=21 is the mainnet committee — the exact shape the audit found reachable.
	for _, p := range []Parameters{
		{K: 21, Alpha: 0.69, Beta: 15, AlphaPreference: 0, AlphaConfidence: 0},
		{K: 21, Alpha: 0.69, Beta: 15, AlphaPreference: 0, AlphaConfidence: 15},
		{K: 1, Alpha: 0.69, Beta: 1, AlphaPreference: 0, AlphaConfidence: 0},
	} {
		if err := p.ValidQuorum(); err == nil {
			t.Fatalf("ValidQuorum() accepted AlphaPreference=0 at K=%d — zero votes would satisfy the accept predicate", p.K)
		}
		if err := p.Valid(); err == nil {
			t.Fatalf("Valid() accepted AlphaPreference=0 at K=%d", p.K)
		}
	}

	// A zero CONFIDENCE quorum is equally unacceptable.
	p := Parameters{K: 21, Alpha: 0.69, Beta: 15, AlphaPreference: 15, AlphaConfidence: 0}
	if err := p.ValidQuorum(); err == nil {
		t.Fatal("ValidQuorum() accepted AlphaConfidence=0")
	}
}

// TestQuorumAlphaBounds pins α into [1, K] from both ends.
func TestQuorumAlphaBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    Parameters
	}{
		{"preference above K", Parameters{K: 5, Beta: 4, AlphaPreference: 6, AlphaConfidence: 6}},
		{"confidence above K", Parameters{K: 5, Beta: 4, AlphaPreference: 4, AlphaConfidence: 6}},
		{"negative preference", Parameters{K: 5, Beta: 4, AlphaPreference: -1, AlphaConfidence: 4}},
		{"negative confidence", Parameters{K: 5, Beta: 4, AlphaPreference: 4, AlphaConfidence: -1}},
	} {
		if err := tc.p.ValidQuorum(); err == nil {
			t.Errorf("%s: ValidQuorum() accepted α outside [1,K]", tc.name)
		}
	}
}

// TestQuorumConfidenceNotBelowPreference: confidence is the STRONGER commitment, so
// it can never be reached on fewer votes than preference required. Nothing checked
// this before, so K=21/preference=15/confidence=1 passed — the safety floor was
// applied to preference while the weaker confidence governed the confidence round.
func TestQuorumConfidenceNotBelowPreference(t *testing.T) {
	p := Parameters{K: 21, Alpha: 0.69, Beta: 15, AlphaPreference: 15, AlphaConfidence: 1}
	err := p.ValidQuorum()
	if !errors.Is(err, ErrAlphaConfidenceBelowPreference) {
		t.Fatalf("ValidQuorum() = %v; want ErrAlphaConfidenceBelowPreference", err)
	}
	if err := p.Valid(); !errors.Is(err, ErrAlphaConfidenceBelowPreference) {
		t.Fatalf("Valid() = %v; want ErrAlphaConfidenceBelowPreference", err)
	}
	// Equal is fine — every preset runs preference == confidence.
	p.AlphaConfidence = 15
	if err := p.ValidQuorum(); err != nil {
		t.Fatalf("ValidQuorum() rejected confidence == preference: %v", err)
	}
}

// TestQuorumBFTFloorAlwaysRuns proves the Byzantine overlap floor is no longer
// skippable. Two α-quorums must overlap in more than f nodes; below that, two
// disjoint quorums can each certify a conflicting block.
func TestQuorumBFTFloorAlwaysRuns(t *testing.T) {
	for _, tc := range []struct {
		k, alpha int
	}{
		{4, 2},  // 2*2-4 = 0 < f+1 = 2 — the audit's A9 example
		{5, 3},  // 2*3-5 = 1 < 2
		{21, 8}, // 2*8-21 < 0
	} {
		p := Parameters{K: tc.k, Alpha: 0.69, Beta: 2, AlphaPreference: tc.alpha, AlphaConfidence: tc.alpha}
		if err := p.ValidQuorum(); !errors.Is(err, ErrAlphaBelowBFTQuorum) {
			t.Errorf("K=%d α=%d: ValidQuorum() = %v; want ErrAlphaBelowBFTQuorum", tc.k, tc.alpha, err)
		}
	}
}

// TestQuorumSingleValidatorStaysValid guards the legitimate K=1/α=1 chain (and the
// other live committees) against over-tightening: hardening α must not make a
// genuine single-validator network — or any shipped preset — invalid.
func TestQuorumSingleValidatorStaysValid(t *testing.T) {
	solo := Parameters{K: 1, Alpha: 1.0, Beta: 1, AlphaPreference: 1, AlphaConfidence: 1}
	if err := solo.ValidQuorum(); err != nil {
		t.Fatalf("K=1/α=1 single-validator rejected: %v", err)
	}
	if err := solo.Valid(); err != nil {
		t.Fatalf("K=1/α=1 single-validator rejected by Valid(): %v", err)
	}
	for name, p := range map[string]Parameters{
		"Default": DefaultParams(),
		"Mainnet": MainnetParams(),
		"Testnet": TestnetParams(),
		"Local":   LocalParams(),
	} {
		if err := p.ValidQuorum(); err != nil {
			t.Errorf("%sParams() quorum rejected: %v", name, err)
		}
	}
}

// TestBFTQuorumFloor pins the floor: never zero (α=0 is the hole), never above the
// ⅔ supermajority the committee sizer installs (or every re-clamp would be refused),
// and never above K itself (or no committee could satisfy it).
func TestBFTQuorumFloor(t *testing.T) {
	if got := BFTQuorumFloor(0); got != 1 {
		t.Errorf("BFTQuorumFloor(0) = %d; want 1 — a floor of 0 accepts on zero votes", got)
	}
	if got := BFTQuorumFloor(1); got != 1 {
		t.Errorf("BFTQuorumFloor(1) = %d; want 1 (single validator)", got)
	}
	for k := 1; k <= 2000; k++ {
		floor := BFTQuorumFloor(k)
		if floor < 1 {
			t.Fatalf("BFTQuorumFloor(%d) = %d; must be >= 1", k, floor)
		}
		if floor > k {
			t.Fatalf("BFTQuorumFloor(%d) = %d; unsatisfiable (> K)", k, floor)
		}
		// The ⅔ supermajority the committee sizer derives must clear the floor,
		// otherwise a legitimate re-clamp would be refused as unsafe.
		if twoThirds := (2*k)/3 + 1; twoThirds < floor {
			t.Fatalf("K=%d: supermajority %d < floor %d", k, twoThirds, floor)
		}
		// The floor is exactly the least α satisfying the overlap bound.
		f := (k - 1) / 3
		if 2*floor-k < f+1 {
			t.Fatalf("K=%d: floor %d does not satisfy 2α-K >= f+1", k, floor)
		}
		if floor > 1 && 2*(floor-1)-k >= f+1 {
			t.Fatalf("K=%d: floor %d is not minimal", k, floor)
		}
	}
}
