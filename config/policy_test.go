package config

import (
	"errors"
	"testing"
)

// A PQ profile must never hand back a cert policy that still requires the
// classical BLS leg — that is the downgrade this whole profile mechanism exists
// to prevent. A non-PQ profile keeps BLS alongside the PQ legs (defence in
// depth), so the leg set must differ by exactly that leg.
func TestPQProfileYieldsNoClassicalLeg(t *testing.T) {
	strict := StrictPQ()
	if !strict.IsPQ() {
		t.Fatal("StrictPQ() is not a PQ profile; the rest of this test is meaningless")
	}
	if cp := strict.CertPolicy(); cp.Variant != CertVariantStrict {
		t.Errorf("PQ profile gave variant %v, want %v", cp.Variant, CertVariantStrict)
	} else if hasLeg(cp.RequiredLegs(), LegBLS) {
		t.Errorf("PQ profile still requires the classical leg: %v", cp.RequiredLegs())
	}

	permissive := Permissive()
	if permissive.IsPQ() {
		t.Fatal("Permissive() reports PQ; this test can no longer reach the hybrid branch")
	}
	cp := permissive.CertPolicy()
	if cp.Variant != CertVariantHybrid {
		t.Errorf("non-PQ profile gave variant %v, want %v", cp.Variant, CertVariantHybrid)
	}
	if !hasLeg(cp.RequiredLegs(), LegBLS) {
		t.Errorf("hybrid policy dropped the classical fast-path leg: %v", cp.RequiredLegs())
	}
	if cp.Mode != CertModeStrict || cp.Fallback != cp.Mode {
		t.Errorf("fallback %v must equal mode %v: the profile implies no temporal degradation", cp.Fallback, cp.Mode)
	}
}

func hasLeg(legs []LegName, want LegName) bool {
	for _, l := range legs {
		if l == want {
			return true
		}
	}
	return false
}

// The decentralisation target is a mainnet-only report at exactly K>=11 (f>=3).
// Breaks if the boundary moves, or if the report starts firing on a network it
// was never meant to gate — it is consulted at chain start, so a false positive
// on a devnet is a spurious operator alarm.
func TestDecentralizationTargetIsMainnetOnlyAtElevenValidators(t *testing.T) {
	for _, k := range []int{1, 5, 10, 11, 21} {
		p := MainnetParams()
		p.K = k

		err := p.MeetsDecentralizationTarget(1)
		if wantErr := k < 11; wantErr != (err != nil) {
			t.Errorf("mainnet K=%d: err=%v, want error=%v", k, err, wantErr)
		} else if wantErr && !errors.Is(err, ErrKBelowMainnetTarget) {
			t.Errorf("mainnet K=%d: got %v, want ErrKBelowMainnetTarget", k, err)
		}

		for _, networkID := range []uint32{0, 2, 3, 1337} {
			if err := p.MeetsDecentralizationTarget(networkID); err != nil {
				t.Errorf("network %d K=%d: reported %v, want no report off mainnet", networkID, k, err)
			}
		}
	}
}

// HalfStakeFloor names the value a majority must STRICTLY exceed, so the floor
// itself is never a majority and one unit above it always is. An off-by-one here
// ignites Nova on half the stake — the safety bug this floor exists to prevent.
func TestHalfStakeFloorIsNeverItselfAMajority(t *testing.T) {
	for total := uint64(0); total <= 1000; total++ {
		floor := HalfStakeFloor(total)

		if 2*floor > total {
			t.Fatalf("total=%d: floor %d is already a majority", total, floor)
		}
		if total > 0 && 2*(floor+1) <= total {
			t.Fatalf("total=%d: floor %d leaves %d short of a majority", total, floor, floor+1)
		}
	}

	// The case the boundary is really about: with two units of stake, one unit
	// is half and must not carry the vote.
	if got := HalfStakeFloor(2); got != 1 {
		t.Fatalf("HalfStakeFloor(2) = %d, want 1 so that a single unit does not exceed it", got)
	}
}
