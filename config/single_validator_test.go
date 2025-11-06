package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSingleValidatorParams verifies that single validator params are correctly configured
func TestSingleValidatorParams(t *testing.T) {
	require := require.New(t)

	// Get single validator parameters
	params := SingleValidatorParams()

	// Verify K=1 configuration
	require.Equal(1, params.K, "K should be 1 for single validator")
	require.Equal(1.0, params.Alpha, "Alpha should be 1.0 for single validator")
	require.Equal(uint32(1), params.Beta, "Beta should be 1 for single validator")
	require.Equal(1, params.AlphaPreference, "AlphaPreference should be 1 for single validator")
	require.Equal(1, params.AlphaConfidence, "AlphaConfidence should be 1 for single validator")
	require.Equal(1, params.BetaVirtuous, "BetaVirtuous should be 1 for single validator")
	require.Equal(1, params.BetaRogue, "BetaRogue should be 1 for single validator")

	// Verify optimized parameters for single node
	require.Equal(1, params.ConcurrentPolls, "ConcurrentPolls should be 1 for single validator")
	require.Equal(1, params.ConcurrentRepolls, "ConcurrentRepolls should be 1 for single validator")
	require.Equal(1, params.OptimalProcessing, "OptimalProcessing should be 1 for single validator")
	require.Equal(1, params.Parents, "Parents should be 1 for linear chain")

	// Verify timing parameters
	require.Equal(100*time.Millisecond, params.BlockTime, "BlockTime should be 100ms for fast blocks")
	require.Equal(200*time.Millisecond, params.RoundTO, "RoundTO should be 200ms for quick timeouts")

	// Verify params are valid
	err := params.Valid()
	require.NoError(err, "Single validator params should be valid")
}

// TestSingleValidatorValidation ensures single validator params pass validation
func TestSingleValidatorValidation(t *testing.T) {
	require := require.New(t)

	params := SingleValidatorParams()

	// Test validation with K=1
	err := params.Valid()
	require.NoError(err, "K=1 should be valid")

	// Test that we properly handle Alpha=1.0 (100% threshold)
	params.Alpha = 1.0
	err = params.Valid()
	require.NoError(err, "Alpha=1.0 should be valid for single validator")

	// Test that we reject invalid Alpha values even for single validator
	params.Alpha = 1.1
	err = params.Valid()
	require.Error(err, "Alpha > 1.0 should be invalid")

	params.Alpha = 0.5
	err = params.Valid()
	require.Error(err, "Alpha < 0.68 should be invalid (below 69% threshold)")
}

// TestK1IntegrationWithConsensus verifies K=1 works with consensus.Config
func TestK1IntegrationWithConsensus(t *testing.T) {
	require := require.New(t)

	// Single validator params should match what consensus expects for K=1
	singleParams := SingleValidatorParams()

	// All threshold values should be 1 for single validator
	require.Equal(1, singleParams.K)
	require.Equal(1, singleParams.AlphaPreference)
	require.Equal(1, singleParams.AlphaConfidence)
	require.Equal(1, singleParams.BetaVirtuous)

	// Alpha should be 1.0 (100% - only one validator)
	require.Equal(1.0, singleParams.Alpha)

	// Should pass validation
	require.NoError(singleParams.Valid())
}
