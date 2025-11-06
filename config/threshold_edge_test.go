package config

import (
	"math"
	"testing"
)

// TestThresholdEdgeCases tests edge cases for 69% threshold
func TestThresholdEdgeCases(t *testing.T) {
	tests := []struct {
		name            string
		totalWeight     uint64
		byzantineWeight uint64
		voteWeight      uint64
		expectQuorum    uint64
		expectTolerance bool
		expectConsensus bool
	}{
		// Exact thresholds
		{
			name:            "exact_69_percent",
			totalWeight:     100,
			byzantineWeight: 31,
			voteWeight:      69,
			expectQuorum:    69,
			expectTolerance: true,
			expectConsensus: true,
		},
		{
			name:            "just_below_69_percent",
			totalWeight:     100,
			byzantineWeight: 31,
			voteWeight:      68,
			expectQuorum:    69,
			expectTolerance: true,
			expectConsensus: false,
		},
		{
			name:            "just_above_31_percent_byzantine",
			totalWeight:     100,
			byzantineWeight: 32,
			voteWeight:      68,
			expectQuorum:    69,
			expectTolerance: false,
			expectConsensus: false,
		},
		// Small networks
		{
			name:            "small_network_3_nodes",
			totalWeight:     3,
			byzantineWeight: 1,
			voteWeight:      2,
			expectQuorum:    3,     // Ceiling of 2.07
			expectTolerance: false, // 33.3% > 31%
			expectConsensus: false,
		},
		{
			name:            "small_network_4_nodes",
			totalWeight:     4,
			byzantineWeight: 1,
			voteWeight:      3,
			expectQuorum:    3,    // Ceiling of 2.76
			expectTolerance: true, // 25% < 31%
			expectConsensus: true,
		},
		{
			name:            "small_network_5_nodes",
			totalWeight:     5,
			byzantineWeight: 1,
			voteWeight:      4,
			expectQuorum:    4,    // Ceiling of 3.45
			expectTolerance: true, // 20% < 31%
			expectConsensus: true,
		},
		// Large networks
		{
			name:            "large_network_1000",
			totalWeight:     1000,
			byzantineWeight: 310,
			voteWeight:      690,
			expectQuorum:    690,
			expectTolerance: true,
			expectConsensus: true,
		},
		{
			name:            "large_network_10000",
			totalWeight:     10000,
			byzantineWeight: 3100,
			voteWeight:      6900,
			expectQuorum:    6900,
			expectTolerance: true,
			expectConsensus: true,
		},
		// Rounding edge cases
		{
			name:            "rounding_33_nodes",
			totalWeight:     33,
			byzantineWeight: 10,
			voteWeight:      23,
			expectQuorum:    23,   // Ceiling of 22.77
			expectTolerance: true, // 30.3% < 31%
			expectConsensus: true,
		},
		{
			name:            "rounding_67_nodes",
			totalWeight:     67,
			byzantineWeight: 20,
			voteWeight:      47,
			expectQuorum:    47,   // Ceiling of 46.23
			expectTolerance: true, // 29.85% < 31%
			expectConsensus: true,
		},
		// Zero and overflow protection
		{
			name:            "zero_weight",
			totalWeight:     0,
			byzantineWeight: 0,
			voteWeight:      0,
			expectQuorum:    0,
			expectTolerance: true, // No Byzantine nodes
			expectConsensus: true, // Vacuous truth
		},
		{
			name:            "max_uint64_weight",
			totalWeight:     math.MaxUint64,
			byzantineWeight: math.MaxUint64 / 3,                     // ~33%
			voteWeight:      uint64(float64(math.MaxUint64) * 0.68), // Just below 69%
			expectQuorum:    uint64(math.Ceil(float64(math.MaxUint64) * 0.69)),
			expectTolerance: false, // >31%
			expectConsensus: false, // 68% < 69%
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test quorum calculation
			quorum := CalculateQuorum(tt.totalWeight)
			if quorum != tt.expectQuorum {
				t.Errorf("CalculateQuorum(%d) = %d, want %d",
					tt.totalWeight, quorum, tt.expectQuorum)
			}

			// Test Byzantine tolerance
			canTolerate := CanTolerateFailure(tt.byzantineWeight, tt.totalWeight)
			if canTolerate != tt.expectTolerance {
				t.Errorf("CanTolerateFailure(%d, %d) = %v, want %v",
					tt.byzantineWeight, tt.totalWeight, canTolerate, tt.expectTolerance)
			}

			// Test consensus achievement
			hasConsensus := HasSuperMajority(tt.voteWeight, tt.totalWeight)
			if hasConsensus != tt.expectConsensus {
				t.Errorf("HasSuperMajority(%d, %d) = %v, want %v",
					tt.voteWeight, tt.totalWeight, hasConsensus, tt.expectConsensus)
			}
		})
	}
}

// TestThresholdPrecision tests floating point precision issues
func TestThresholdPrecision(t *testing.T) {
	// Test that we handle floating point precision correctly
	testCases := []struct {
		weight      uint64
		totalWeight uint64
		expected    bool
	}{
		// Test cases around 69% with potential floating point issues
		{68999999, 100000000, false}, // 68.999999%
		{69000000, 100000000, true},  // 69.000000%
		{69000001, 100000000, true},  // 69.000001%

		// Test with prime numbers (harder to represent exactly)
		{47, 68, true},  // 69.117% > 69%
		{46, 67, false}, // 68.656% < 69%
		{47, 67, true},  // 70.149% > 69%
		{45, 67, false}, // 67.164% < 69%
	}

	for _, tc := range testCases {
		result := HasSuperMajority(tc.weight, tc.totalWeight)
		if result != tc.expected {
			percentage := float64(tc.weight) * 100 / float64(tc.totalWeight)
			t.Errorf("HasSuperMajority(%d, %d) = %v, want %v (%.6f%%)",
				tc.weight, tc.totalWeight, result, tc.expected, percentage)
		}
	}
}

// TestAlphaForKAllValues tests Alpha calculation for all practical K values
func TestAlphaForKAllValues(t *testing.T) {
	testCases := []struct {
		k             int
		expectedAlpha int
		percentage    float64
	}{
		{1, 1, 100.0},     // Special case: need all
		{2, 2, 100.0},     // Ceiling of 1.38 = 2
		{3, 3, 100.0},     // Ceiling of 2.07 = 3
		{4, 3, 75.0},      // Ceiling of 2.76 = 3
		{5, 4, 80.0},      // Ceiling of 3.45 = 4
		{10, 7, 70.0},     // Ceiling of 6.9 = 7
		{11, 8, 72.7},     // Ceiling of 7.59 = 8
		{20, 14, 70.0},    // Ceiling of 13.8 = 14
		{21, 15, 71.4},    // Ceiling of 14.49 = 15
		{100, 69, 69.0},   // Exactly 69
		{1000, 690, 69.0}, // Exactly 690
	}

	for _, tc := range testCases {
		alpha := AlphaForK(tc.k)
		if alpha != tc.expectedAlpha {
			t.Errorf("AlphaForK(%d) = %d, want %d", tc.k, alpha, tc.expectedAlpha)
		}

		// Verify the percentage is at least 69%
		actualPercentage := float64(alpha) * 100 / float64(tc.k)
		if actualPercentage < 69.0 && tc.k > 3 { // Allow small networks to exceed
			t.Errorf("AlphaForK(%d) = %d gives %.1f%%, below 69%% threshold",
				tc.k, alpha, actualPercentage)
		}
	}
}

// TestConcurrentThresholdCalculations tests thread safety
func TestConcurrentThresholdCalculations(t *testing.T) {
	// Run many concurrent calculations to ensure thread safety
	const goroutines = 100
	const iterations = 1000

	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			for j := 0; j < iterations; j++ {
				weight := uint64(id*iterations + j)
				total := weight + uint64(j)

				// These should be thread-safe pure functions
				_ = CalculateQuorum(total)
				_ = HasSuperMajority(weight, total)
				_ = CanTolerateFailure(weight/3, total)
				_ = AlphaForK(int(total % 100))
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < goroutines; i++ {
		<-done
	}
}

// BenchmarkThresholdEdgeCases benchmarks edge case performance
func BenchmarkThresholdEdgeCases(b *testing.B) {
	b.Run("small_network", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = CalculateQuorum(5)
			_ = HasSuperMajority(4, 5)
			_ = CanTolerateFailure(1, 5)
		}
	})

	b.Run("large_network", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = CalculateQuorum(1000000)
			_ = HasSuperMajority(690000, 1000000)
			_ = CanTolerateFailure(310000, 1000000)
		}
	})

	b.Run("max_uint64", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = CalculateQuorum(math.MaxUint64)
			_ = HasSuperMajority(math.MaxUint64*69/100, math.MaxUint64)
			_ = CanTolerateFailure(math.MaxUint64*31/100, math.MaxUint64)
		}
	})
}
