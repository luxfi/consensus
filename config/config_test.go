package config

import (
	"fmt"
	"math"
	"testing"
)

// TestConsensusThreshold69 verifies the 69% threshold is properly configured
func TestConsensusThreshold69(t *testing.T) {
	tests := []struct {
		name   string
		params Parameters
		k      int
		alpha  int
	}{
		{
			name:   "DefaultParams",
			params: DefaultParams(),
			k:      20,
			alpha:  14, // 70% of 20
		},
		{
			name:   "MainnetParams",
			params: MainnetParams(),
			k:      21,
			alpha:  15, // ~71% of 21
		},
		{
			name:   "TestnetParams",
			params: TestnetParams(),
			k:      11,
			alpha:  8, // ~73% of 11
		},
		{
			name:   "LocalParams",
			params: LocalParams(),
			k:      5,
			alpha:  4, // 80% of 5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check K value
			if tt.params.K != tt.k {
				t.Errorf("K = %d, want %d", tt.params.K, tt.k)
			}

			// Check Alpha is approximately 69%
			if tt.params.Alpha < 0.68 || tt.params.Alpha > 0.70 {
				t.Errorf("Alpha = %f, want ~0.69", tt.params.Alpha)
			}

			// Check AlphaPreference meets 69% threshold
			if tt.params.AlphaPreference != tt.alpha {
				t.Errorf("AlphaPreference = %d, want %d", tt.params.AlphaPreference, tt.alpha)
			}

			// Verify AlphaPreference is at least 69% of K
			minAlpha := math.Ceil(float64(tt.params.K) * 0.69)
			if float64(tt.params.AlphaPreference) < minAlpha {
				t.Errorf("AlphaPreference %d is below 69%% threshold %f", 
					tt.params.AlphaPreference, minAlpha)
			}

			// Check that parameters are valid
			if err := tt.params.Valid(); err != nil {
				t.Errorf("Parameters validation failed: %v", err)
			}
		})
	}
}

// TestByzantineTolerance31 verifies maximum Byzantine tolerance is 31%
func TestByzantineTolerance31(t *testing.T) {
	tests := []struct {
		byzantineWeight uint64
		totalWeight     uint64
		canTolerate     bool
	}{
		{31, 100, true},   // Exactly 31%
		{30, 100, true},   // Below threshold
		{32, 100, false},  // Above threshold
		{310, 1000, true}, // 31% at scale
		{311, 1000, false}, // Slightly above 31%
	}

	for _, tt := range tests {
		result := CanTolerateFailure(tt.byzantineWeight, tt.totalWeight)
		if result != tt.canTolerate {
			t.Errorf("CanTolerateFailure(%d, %d) = %v, want %v",
				tt.byzantineWeight, tt.totalWeight, result, tt.canTolerate)
		}
	}
}

// TestQuorumCalculation verifies 69% quorum calculations
func TestQuorumCalculation(t *testing.T) {
	tests := []struct {
		totalWeight    uint64
		expectedQuorum uint64
	}{
		{100, 69},
		{1000, 690},
		{10000, 6900},
		{33, 23}, // Ceiling of 22.77
		{67, 47}, // Ceiling of 46.23
	}

	for _, tt := range tests {
		quorum := CalculateQuorum(tt.totalWeight)
		if quorum != tt.expectedQuorum {
			t.Errorf("CalculateQuorum(%d) = %d, want %d",
				tt.totalWeight, quorum, tt.expectedQuorum)
		}
	}
}

// TestHasSuperMajority verifies super majority checks
func TestHasSuperMajority(t *testing.T) {
	tests := []struct {
		weight      uint64
		totalWeight uint64
		hasMajority bool
	}{
		{69, 100, true},   // Exactly 69%
		{70, 100, true},   // Above threshold
		{68, 100, false},  // Below threshold
		{690, 1000, true}, // 69% at scale
		{689, 1000, false}, // Just below
	}

	for _, tt := range tests {
		result := HasSuperMajority(tt.weight, tt.totalWeight)
		if result != tt.hasMajority {
			t.Errorf("HasSuperMajority(%d, %d) = %v, want %v",
				tt.weight, tt.totalWeight, result, tt.hasMajority)
		}
	}
}

// TestAlphaForK verifies Alpha calculation for different K values
func TestAlphaForK(t *testing.T) {
	tests := []struct {
		k            int
		expectedAlpha int
	}{
		{20, 14}, // Ceiling of 13.8
		{21, 15}, // Ceiling of 14.49
		{11, 8},  // Ceiling of 7.59
		{5, 4},   // Ceiling of 3.45
		{100, 69}, // Exactly 69
	}

	for _, tt := range tests {
		alpha := AlphaForK(tt.k)
		if alpha != tt.expectedAlpha {
			t.Errorf("AlphaForK(%d) = %d, want %d",
				tt.k, alpha, tt.expectedAlpha)
		}
	}
}

// TestParameterValidation verifies parameter validation with 69% threshold
func TestParameterValidation(t *testing.T) {
	tests := []struct {
		name        string
		params      Parameters
		expectedErr error
	}{
		{
			name: "Valid 69% threshold",
			params: Parameters{
				K:     20,
				Alpha: 0.69,
				Beta:  14,
			},
			expectedErr: nil,
		},
		{
			name: "Below 69% threshold",
			params: Parameters{
				K:     20,
				Alpha: 0.67, // Old 67% threshold
				Beta:  14,
			},
			expectedErr: ErrInvalidAlpha,
		},
		{
			name: "Invalid K",
			params: Parameters{
				K:     0,
				Alpha: 0.69,
				Beta:  14,
			},
			expectedErr: ErrInvalidK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.Valid()
			if err != tt.expectedErr {
				t.Errorf("Valid() error = %v, want %v", err, tt.expectedErr)
			}
		})
	}
}

// BenchmarkQuorumCalculation benchmarks quorum calculations
func BenchmarkQuorumCalculation(b *testing.B) {
	weights := []uint64{100, 1000, 10000, 100000, 1000000}
	
	for _, w := range weights {
		b.Run(fmt.Sprintf("weight_%d", w), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = CalculateQuorum(w)
			}
		})
	}
}

// BenchmarkHasSuperMajority benchmarks super majority checks
func BenchmarkHasSuperMajority(b *testing.B) {
	totalWeight := uint64(1000000)
	weight := uint64(690000)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HasSuperMajority(weight, totalWeight)
	}
}