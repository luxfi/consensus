package config

import (
	"testing"
	"time"
)

// TestHasSimpleMajority verifies simple majority (51%) checks
func TestHasSimpleMajority(t *testing.T) {
	tests := []struct {
		weight      uint64
		totalWeight uint64
		hasMajority bool
	}{
		{51, 100, true},    // Exactly 51%
		{52, 100, true},    // Above threshold
		{50, 100, false},   // Below threshold
		{510, 1000, true},  // 51% at scale
		{509, 1000, false}, // Just below 51%
		{0, 100, false},    // Zero weight
		{100, 100, true},   // 100%
	}

	for _, tt := range tests {
		result := HasSimpleMajority(tt.weight, tt.totalWeight)
		if result != tt.hasMajority {
			t.Errorf("HasSimpleMajority(%d, %d) = %v, want %v",
				tt.weight, tt.totalWeight, result, tt.hasMajority)
		}
	}
}

// TestDefaultFPC verifies default FPC configuration
func TestDefaultFPC(t *testing.T) {
	fpc := DefaultFPC()

	if !fpc.Enable {
		t.Error("DefaultFPC().Enable = false, want true")
	}
	if fpc.Rounds != 10 {
		t.Errorf("DefaultFPC().Rounds = %d, want 10", fpc.Rounds)
	}
	if fpc.Threshold != 0.8 {
		t.Errorf("DefaultFPC().Threshold = %f, want 0.8", fpc.Threshold)
	}
	if fpc.SampleSize != 20 {
		t.Errorf("DefaultFPC().SampleSize = %d, want 20", fpc.SampleSize)
	}
}

// TestWithFPC verifies FPC configuration application to Parameters
func TestWithFPC(t *testing.T) {
	params := DefaultParams()
	fpc := FPCConfig{
		Enable:     true,
		Rounds:     5,
		Threshold:  0.9,
		SampleSize: 15,
	}

	newParams := params.WithFPC(fpc)

	// Verify parameters are returned (currently returns unchanged)
	if newParams.K != params.K {
		t.Errorf("WithFPC changed K from %d to %d", params.K, newParams.K)
	}
}

// TestValidAlphaPreferenceInvalid tests AlphaPreference validation errors
func TestValidAlphaPreferenceInvalid(t *testing.T) {
	// AlphaPreference > K
	params := Parameters{
		K:               10,
		Alpha:           0.69,
		Beta:            5,
		AlphaPreference: 15, // Greater than K
	}
	if err := params.Valid(); err != ErrParametersInvalid {
		t.Errorf("Valid() with AlphaPreference > K = %v, want ErrParametersInvalid", err)
	}

	// AlphaPreference negative
	params2 := Parameters{
		K:               10,
		Alpha:           0.69,
		Beta:            5,
		AlphaPreference: -1,
	}
	if err := params2.Valid(); err != ErrParametersInvalid {
		t.Errorf("Valid() with negative AlphaPreference = %v, want ErrParametersInvalid", err)
	}
}

// TestValidAlphaConfidenceInvalid tests AlphaConfidence validation errors
func TestValidAlphaConfidenceInvalid(t *testing.T) {
	// AlphaConfidence > K
	params := Parameters{
		K:               10,
		Alpha:           0.69,
		Beta:            5,
		AlphaConfidence: 15, // Greater than K
	}
	if err := params.Valid(); err != ErrParametersInvalid {
		t.Errorf("Valid() with AlphaConfidence > K = %v, want ErrParametersInvalid", err)
	}

	// AlphaConfidence negative
	params2 := Parameters{
		K:               10,
		Alpha:           0.69,
		Beta:            5,
		AlphaConfidence: -1,
	}
	if err := params2.Valid(); err != ErrParametersInvalid {
		t.Errorf("Valid() with negative AlphaConfidence = %v, want ErrParametersInvalid", err)
	}
}

// TestValidBetaVirtuousNegative tests BetaVirtuous negative validation
func TestValidBetaVirtuousNegative(t *testing.T) {
	params := Parameters{
		K:            10,
		Alpha:        0.69,
		Beta:         5,
		BetaVirtuous: -1,
	}
	if err := params.Valid(); err != ErrParametersInvalid {
		t.Errorf("Valid() with negative BetaVirtuous = %v, want ErrParametersInvalid", err)
	}
}

// TestValidBetaRogueLessThanVirtuous tests BetaRogue < BetaVirtuous validation
func TestValidBetaRogueLessThanVirtuous(t *testing.T) {
	params := Parameters{
		K:            10,
		Alpha:        0.69,
		Beta:         5,
		BetaVirtuous: 10,
		BetaRogue:    5, // Less than BetaVirtuous
	}
	if err := params.Valid(); err != ErrParametersInvalid {
		t.Errorf("Valid() with BetaRogue < BetaVirtuous = %v, want ErrParametersInvalid", err)
	}
}

// TestValidConcurrentPollsInvalid tests ConcurrentPolls validation
func TestValidConcurrentPollsInvalid(t *testing.T) {
	params := Parameters{
		K:               10,
		Alpha:           0.69,
		Beta:            5,
		ConcurrentPolls: -1, // Invalid: less than 1 when non-zero
	}
	if err := params.Valid(); err != ErrParametersInvalid {
		t.Errorf("Valid() with invalid ConcurrentPolls = %v, want ErrParametersInvalid", err)
	}
}

// TestValidOptimalProcessingInvalid tests OptimalProcessing validation
func TestValidOptimalProcessingInvalid(t *testing.T) {
	params := Parameters{
		K:                 10,
		Alpha:             0.69,
		Beta:              5,
		OptimalProcessing: -1, // Invalid: less than 1 when non-zero
	}
	if err := params.Valid(); err != ErrParametersInvalid {
		t.Errorf("Valid() with invalid OptimalProcessing = %v, want ErrParametersInvalid", err)
	}
}

// TestValidMaxOutstandingItemsInvalid tests MaxOutstandingItems validation
func TestValidMaxOutstandingItemsInvalid(t *testing.T) {
	params := Parameters{
		K:                   10,
		Alpha:               0.69,
		Beta:                5,
		MaxOutstandingItems: -1, // Invalid: less than 1 when non-zero
	}
	if err := params.Valid(); err != ErrParametersInvalid {
		t.Errorf("Valid() with invalid MaxOutstandingItems = %v, want ErrParametersInvalid", err)
	}
}

// TestValidMaxItemProcessingTimeInvalid tests MaxItemProcessingTime validation
func TestValidMaxItemProcessingTimeInvalid(t *testing.T) {
	params := Parameters{
		K:                     10,
		Alpha:                 0.69,
		Beta:                  5,
		MaxItemProcessingTime: -1 * time.Second, // Invalid: negative duration
	}
	if err := params.Valid(); err != ErrParametersInvalid {
		t.Errorf("Valid() with invalid MaxItemProcessingTime = %v, want ErrParametersInvalid", err)
	}
}
