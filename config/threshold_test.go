package config

import (
	"errors"
	"testing"
)

// A set with no stake has no supermajority to reach, so the threshold is zero
// rather than a count some empty quorum could satisfy. Breaks if a stakeless or
// empty validator set ever yields a reachable threshold — that is a quorum of
// nobody.
func TestStakelessSetHasNoSupermajority(t *testing.T) {
	for name, weights := range map[string][]uint64{
		"no validators":  {},
		"nil":            nil,
		"all zero stake": {0, 0, 0},
	} {
		if got := WeightedSupermajorityThreshold(weights); got != 0 {
			t.Errorf("%s: threshold %d, want 0", name, got)
		}
	}
}

// The equal-stake closed form and the general heaviest-first computation are two
// spellings of one predicate; they must agree on every n, or a network sized by
// one rule would be validated against the other.
func TestEqualStakeClosedFormMatchesTheWeightedComputation(t *testing.T) {
	for n := 1; n <= 300; n++ {
		unit := make([]uint64, n)
		for i := range unit {
			unit[i] = 1
		}
		if closed, general := EqualStakeSupermajorityThreshold(n), WeightedSupermajorityThreshold(unit); closed != general {
			t.Fatalf("n=%d: closed form %d, heaviest-first %d", n, closed, general)
		}
	}

	// An unknown validator count still needs someone to accept, never zero.
	for _, n := range []int{0, -1, -100} {
		if got := EqualStakeSupermajorityThreshold(n); got != 1 {
			t.Errorf("n=%d: threshold %d, want 1", n, got)
		}
	}
}

// FeasibleParams derives α from the committee size alone. This holds the ground
// the function's missing clamps rest on: for every k it can produce, α already
// sits at or above the BFT overlap floor, at or below k, and its ratio lands
// inside the [0.66, 1.0] window Valid() demands. If a formula change breaks any
// of these, the clamps have to come back — and this fails first.
func TestFeasibleParamsNeedsNoClamping(t *testing.T) {
	for n := -1; n <= 2000; n++ {
		p := FeasibleParams(1, n)

		if p.K < 4 {
			t.Fatalf("n=%d: K=%d below the minimal BFT committee", n, p.K)
		}
		if floor := (Parameters{K: p.K}).bftQuorumFloor(); p.AlphaPreference < floor {
			t.Fatalf("K=%d: α=%d under the BFT overlap floor %d", p.K, p.AlphaPreference, floor)
		}
		if p.AlphaPreference > p.K {
			t.Fatalf("K=%d: α=%d demands more votes than the committee holds", p.K, p.AlphaPreference)
		}
		if p.Alpha < 0.66 || p.Alpha > 1.0 {
			t.Fatalf("K=%d: ratio %.4f outside [0.66, 1.0]", p.K, p.Alpha)
		}
		if err := p.Valid(); err != nil {
			t.Fatalf("n=%d: derived params do not validate: %v", n, err)
		}
	}
}

// The live-aware check still runs the ordinary parameter validation first, so a
// structurally broken set is refused on its own terms rather than reaching the
// live-count reasoning with nonsense.
func TestLiveValidationRefusesStructurallyBrokenParams(t *testing.T) {
	p := MainnetParams()
	p.K = 0

	err := p.ValidateForLiveValueNetwork(1, 21)
	if err == nil {
		t.Fatal("accepted params with an empty committee")
	}
	if errors.Is(err, ErrKBelowMainnetTarget) {
		t.Errorf("reported a decentralisation target for params that are not even valid: %v", err)
	}
}
