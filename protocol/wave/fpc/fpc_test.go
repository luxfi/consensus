package fpc

import (
	"testing"
)

func TestNewSelectorRequiresSeed(t *testing.T) {
	_, err := NewSelector(0.5, 0.8, nil)
	if err != ErrEmptySeed {
		t.Fatalf("Expected ErrEmptySeed for nil seed, got %v", err)
	}

	_, err = NewSelector(0.5, 0.8, []byte{})
	if err != ErrEmptySeed {
		t.Fatalf("Expected ErrEmptySeed for empty seed, got %v", err)
	}
}

func TestNewSelector(t *testing.T) {
	s, err := NewSelector(0.5, 0.8, []byte("test-seed"))
	if err != nil {
		t.Fatalf("NewSelector returned error: %v", err)
	}
	if s == nil {
		t.Fatal("NewSelector returned nil")
	}

	min, max := s.Range()
	if min != 0.5 {
		t.Errorf("Expected min=0.5, got %f", min)
	}
	if max != 0.8 {
		t.Errorf("Expected max=0.8, got %f", max)
	}
}

func TestDeriveEpochSeed(t *testing.T) {
	seed1 := DeriveEpochSeed(1, []byte("chain-A"))
	seed2 := DeriveEpochSeed(2, []byte("chain-A"))
	seed3 := DeriveEpochSeed(1, []byte("chain-B"))

	if len(seed1) != 32 {
		t.Fatalf("Expected 32-byte seed, got %d", len(seed1))
	}

	// Different epoch => different seed
	if string(seed1) == string(seed2) {
		t.Error("Different epochs must produce different seeds")
	}

	// Different chain => different seed
	if string(seed1) == string(seed3) {
		t.Error("Different chains must produce different seeds")
	}

	// Deterministic
	seed1b := DeriveEpochSeed(1, []byte("chain-A"))
	if string(seed1) != string(seed1b) {
		t.Error("DeriveEpochSeed must be deterministic")
	}
}

func TestDeriveEpochSeedUsableInSelector(t *testing.T) {
	seed := DeriveEpochSeed(42, []byte("lux-mainnet"))
	s, err := NewSelector(0.5, 0.8, seed)
	if err != nil {
		t.Fatalf("NewSelector with derived seed returned error: %v", err)
	}

	threshold := s.SelectThreshold(1, 100)
	if threshold < 50 || threshold > 81 {
		t.Errorf("Threshold %d outside expected range [50, 81]", threshold)
	}
}

func TestSelectThreshold(t *testing.T) {
	s, err := NewSelector(0.5, 0.8, []byte("test-seed"))
	if err != nil {
		t.Fatal(err)
	}

	k := 100
	threshold := s.SelectThreshold(1, k)

	// Threshold should be in range [k*thetaMin, k*thetaMax]
	minThreshold := int(0.5 * float64(k))
	maxThreshold := int(0.8*float64(k)) + 1 // +1 for ceiling

	if threshold < minThreshold || threshold > maxThreshold {
		t.Errorf("Threshold %d outside expected range [%d, %d]", threshold, minThreshold, maxThreshold)
	}
}

func TestDeterministicThresholds(t *testing.T) {
	s, err := NewSelector(0.5, 0.8, []byte("test-seed"))
	if err != nil {
		t.Fatal(err)
	}

	k := 100
	phase := uint64(42)

	// Same phase should give same threshold
	t1 := s.SelectThreshold(phase, k)
	t2 := s.SelectThreshold(phase, k)

	if t1 != t2 {
		t.Errorf("Non-deterministic: phase %d gave thresholds %d and %d", phase, t1, t2)
	}
}

func TestDifferentPhasesGiveDifferentThresholds(t *testing.T) {
	s, err := NewSelector(0.5, 0.8, []byte("test-seed"))
	if err != nil {
		t.Fatal(err)
	}

	k := 100
	thresholds := make(map[int]bool)

	// Test multiple phases - should get variety
	for phase := uint64(0); phase < 100; phase++ {
		t := s.SelectThreshold(phase, k)
		thresholds[t] = true
	}

	// Should have at least 5 different threshold values across 100 phases
	if len(thresholds) < 5 {
		t.Errorf("Expected variety in thresholds, got only %d unique values", len(thresholds))
	}
}

func TestThetaRange(t *testing.T) {
	s, err := NewSelector(0.5, 0.8, []byte("test-seed"))
	if err != nil {
		t.Fatal(err)
	}

	// Test theta values for many phases
	for phase := uint64(0); phase < 1000; phase++ {
		theta := s.Theta(phase)

		if theta < 0.5 || theta > 0.8 {
			t.Errorf("Theta %f for phase %d outside range [0.5, 0.8]", theta, phase)
		}
	}
}

func TestDifferentSeedsGiveDifferentResults(t *testing.T) {
	s1, err := NewSelector(0.5, 0.8, []byte("seed-1"))
	if err != nil {
		t.Fatal(err)
	}
	s2, err := NewSelector(0.5, 0.8, []byte("seed-2"))
	if err != nil {
		t.Fatal(err)
	}

	k := 100
	phase := uint64(42)

	t1 := s1.SelectThreshold(phase, k)
	t2 := s2.SelectThreshold(phase, k)

	// Different seeds should (very likely) give different thresholds
	if t1 == t2 {
		t.Logf("Warning: Different seeds gave same threshold (unlikely but possible)")
	}
}

func TestInvalidRanges(t *testing.T) {
	// Test auto-correction of invalid ranges
	s, err := NewSelector(-0.1, 1.5, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}

	min, max := s.Range()

	// Should be corrected to valid range
	if min <= 0 || min >= 1 {
		t.Errorf("Invalid min should be corrected, got %f", min)
	}
	if max <= min || max > 1 {
		t.Errorf("Invalid max should be corrected, got %f", max)
	}
}

func TestPhaseCoverage(t *testing.T) {
	s, err := NewSelector(0.5, 0.8, []byte("test-seed"))
	if err != nil {
		t.Fatal(err)
	}
	k := 100

	// Test a wide range of phases to ensure no panics or errors
	phases := []uint64{0, 1, 100, 1000, 10000, ^uint64(0) - 1, ^uint64(0)}

	for _, phase := range phases {
		threshold := s.SelectThreshold(phase, k)

		if threshold <= 0 || threshold > k {
			t.Errorf("Invalid threshold %d for k=%d at phase %d", threshold, k, phase)
		}
	}
}

func BenchmarkSelectThreshold(b *testing.B) {
	s, _ := NewSelector(0.5, 0.8, []byte("bench-seed"))
	k := 100

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.SelectThreshold(uint64(i), k)
	}
}

func BenchmarkComputeTheta(b *testing.B) {
	s, _ := NewSelector(0.5, 0.8, []byte("bench-seed"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.computeTheta(uint64(i))
	}
}
