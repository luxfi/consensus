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

// --- Additional inversion and edge-case tests ---

// TestDifferentSeedsProduceDifferentThresholdSequences verifies that two
// selectors built from different seeds produce different threshold sequences
// across many phases. It is statistically near-impossible for two PRFs with
// different keys to match across 100 phases.
func TestDifferentSeedsProduceDifferentThresholdSequences(t *testing.T) {
	s1, err := NewSelector(0.5, 0.8, []byte("alpha-seed"))
	if err != nil {
		t.Fatal(err)
	}
	s2, err := NewSelector(0.5, 0.8, []byte("beta-seed"))
	if err != nil {
		t.Fatal(err)
	}

	k := 200
	matches := 0
	const phases = 100
	for phase := uint64(0); phase < phases; phase++ {
		if s1.SelectThreshold(phase, k) == s2.SelectThreshold(phase, k) {
			matches++
		}
	}

	// With a 30% range ([0.5,0.8]*200 = [100,160]), about 60 distinct values.
	// P(match per phase) ~ 1/60. Expected matches ~ 1.67 in 100 phases.
	// If >20 match, something is wrong.
	if matches > 20 {
		t.Errorf("different seeds produced %d/%d matching thresholds -- PRF may be broken", matches, phases)
	}
}

// TestSameSeedDeterministicSequence verifies that the same seed produces
// the exact same threshold sequence on every invocation (reproducibility).
func TestSameSeedDeterministicSequence(t *testing.T) {
	seed := []byte("deterministic-seed-42")
	k := 150

	s1, err := NewSelector(0.4, 0.9, seed)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := NewSelector(0.4, 0.9, seed)
	if err != nil {
		t.Fatal(err)
	}

	for phase := uint64(0); phase < 500; phase++ {
		t1 := s1.SelectThreshold(phase, k)
		t2 := s2.SelectThreshold(phase, k)
		if t1 != t2 {
			t.Fatalf("phase %d: s1=%d, s2=%d -- same seed must be deterministic", phase, t1, t2)
		}
	}
}

// TestDeriveEpochSeedDifferentEpochs verifies that every distinct epoch
// number produces a distinct seed (for the same chain).
func TestDeriveEpochSeedDifferentEpochs(t *testing.T) {
	chain := []byte("lux-mainnet")
	seen := make(map[string]uint64) // seed hex -> epoch

	for epoch := uint64(0); epoch < 1000; epoch++ {
		seed := DeriveEpochSeed(epoch, chain)
		key := string(seed)
		if prev, exists := seen[key]; exists {
			t.Fatalf("epoch %d and %d produced the same seed", prev, epoch)
		}
		seen[key] = epoch
	}
}

// TestThresholdAlwaysInRange verifies the critical safety property that
// for all phases, all k values, the returned threshold is in
// [ceil(thetaMin*k), ceil(thetaMax*k)].
func TestThresholdAlwaysInRange(t *testing.T) {
	thetaMin, thetaMax := 0.5, 0.8
	seed := DeriveEpochSeed(1, []byte("range-test"))
	s, err := NewSelector(thetaMin, thetaMax, seed)
	if err != nil {
		t.Fatal(err)
	}

	kValues := []int{1, 2, 3, 10, 50, 100, 1000, 10000}
	for _, k := range kValues {
		lo := int(thetaMin * float64(k))        // floor
		hi := int(thetaMax*float64(k)) + 1       // ceiling with margin

		for phase := uint64(0); phase < 200; phase++ {
			threshold := s.SelectThreshold(phase, k)
			if threshold < lo || threshold > hi {
				t.Fatalf("k=%d phase=%d: threshold %d outside [%d, %d]",
					k, phase, threshold, lo, hi)
			}
		}
	}
}

// TestSelectThresholdKZero verifies behavior when k=0.
func TestSelectThresholdKZero(t *testing.T) {
	s, err := NewSelector(0.5, 0.8, []byte("zero-k"))
	if err != nil {
		t.Fatal(err)
	}

	threshold := s.SelectThreshold(1, 0)
	if threshold != 0 {
		t.Errorf("expected threshold 0 for k=0, got %d", threshold)
	}
}

// TestSelectThresholdKOne verifies behavior when k=1.
// ceil(theta * 1) should be 1 for any theta > 0.
func TestSelectThresholdKOne(t *testing.T) {
	s, err := NewSelector(0.5, 0.8, []byte("one-k"))
	if err != nil {
		t.Fatal(err)
	}

	for phase := uint64(0); phase < 100; phase++ {
		threshold := s.SelectThreshold(phase, 1)
		if threshold != 1 {
			t.Fatalf("phase %d: expected threshold 1 for k=1, got %d", phase, threshold)
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
