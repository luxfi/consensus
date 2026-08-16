package config

import (
	"testing"
	"time"
)

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
