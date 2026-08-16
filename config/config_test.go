package config

import (
	"math"
	"testing"
)

// TestConsensusThresholds verifies thresholds are properly configured
func TestConsensusThresholds(t *testing.T) {
	tests := []struct {
		name     string
		params   Parameters
		k        int
		alpha    int
		alphaF   float64
		minRatio float64
	}{
		{
			name:     "DefaultParams",
			params:   DefaultParams(),
			k:        20,
			alpha:    14,   // 70% of 20
			alphaF:   0.69, // 69% threshold
			minRatio: 0.69,
		},
		{
			name:     "MainnetParams",
			params:   MainnetParams(),
			k:        21,
			alpha:    15,   // ~71% of 21
			alphaF:   0.69, // 69% threshold
			minRatio: 0.69,
		},
		{
			name:     "TestnetParams",
			params:   TestnetParams(),
			k:        11,
			alpha:    8,    // ~73% of 11
			alphaF:   0.69, // 69% threshold
			minRatio: 0.69,
		},
		{
			name:     "LocalParams",
			params:   LocalParams(),
			k:        3,
			alpha:    2,    // 2/3 of 3
			alphaF:   0.67, // 2/3 threshold
			minRatio: 0.66,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check K value
			if tt.params.K != tt.k {
				t.Errorf("K = %d, want %d", tt.params.K, tt.k)
			}

			// Check Alpha matches expected threshold
			if tt.params.Alpha != tt.alphaF {
				t.Errorf("Alpha = %f, want %f", tt.params.Alpha, tt.alphaF)
			}

			// Check AlphaPreference matches
			if tt.params.AlphaPreference != tt.alpha {
				t.Errorf("AlphaPreference = %d, want %d", tt.params.AlphaPreference, tt.alpha)
			}

			// Verify AlphaPreference meets minimum ratio of K
			minAlpha := math.Ceil(float64(tt.params.K) * tt.minRatio)
			if float64(tt.params.AlphaPreference) < minAlpha {
				t.Errorf("AlphaPreference %d is below %.0f%% threshold %f",
					tt.params.AlphaPreference, tt.minRatio*100, minAlpha)
			}

			// Check that parameters are valid
			if err := tt.params.Valid(); err != nil {
				t.Errorf("Parameters validation failed: %v", err)
			}
		})
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
			name: "Below 2/3 threshold",
			params: Parameters{
				K:     20,
				Alpha: 0.65, // Below minimum 2/3 threshold
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
