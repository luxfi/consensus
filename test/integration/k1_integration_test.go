package integration

import (
	"testing"

	"github.com/luxfi/consensus"
	"github.com/luxfi/consensus/config"
	"github.com/stretchr/testify/require"
)

// TestK1ConsensusIntegration verifies K=1 single validator consensus configuration
func TestK1ConsensusIntegration(t *testing.T) {
	require := require.New(t)

	// Test that Config(1) returns single validator params
	params := consensus.Config(1)

	// Verify it matches SingleValidatorParams
	expectedParams := config.SingleValidatorParams()
	require.Equal(expectedParams.K, params.K, "K should match single validator params")
	require.Equal(expectedParams.Alpha, params.Alpha, "Alpha should match single validator params")
	require.Equal(expectedParams.Beta, params.Beta, "Beta should match single validator params")
	require.Equal(expectedParams.AlphaPreference, params.AlphaPreference, "AlphaPreference should match")
	require.Equal(expectedParams.AlphaConfidence, params.AlphaConfidence, "AlphaConfidence should match")

	// Verify K=1 specific values
	require.Equal(1, params.K, "K should be 1 for single validator")
	require.Equal(1.0, params.Alpha, "Alpha should be 1.0 (100% threshold)")
	require.Equal(uint32(1), params.Beta, "Beta should be 1 for immediate finalization")

	// Verify params are valid
	err := params.Valid()
	require.NoError(err, "K=1 params should be valid")
}

// TestK1EngineCompatibility verifies consensus engines work with K=1
func TestK1EngineCompatibility(t *testing.T) {
	require := require.New(t)

	// Get K=1 parameters
	params := config.SingleValidatorParams()

	// Verify critical consensus invariants for K=1
	// With only one validator, all decisions are immediate
	require.Equal(1, params.K, "Sample size is 1")
	require.Equal(1, params.AlphaPreference, "Preference threshold is 1")
	require.Equal(1, params.AlphaConfidence, "Confidence threshold is 1")
	require.Equal(1, params.BetaVirtuous, "Virtuous beta is 1")
	require.Equal(1, params.BetaRogue, "Rogue beta is 1")

	// These values ensure immediate consensus with single validator
	require.Equal(1, params.ConcurrentPolls, "Only 1 concurrent poll needed")
	require.Equal(1, params.OptimalProcessing, "Process 1 item at a time")
	require.Equal(1, params.Parents, "Linear chain with single parent")

	// Timing should be fast for single validator
	require.LessOrEqual(params.BlockTime.Milliseconds(), int64(100), "Block time should be <= 100ms")
	require.LessOrEqual(params.RoundTO.Milliseconds(), int64(200), "Round timeout should be <= 200ms")
}

// TestConfigNodeCounts verifies Config returns correct params for different node counts
func TestConfigNodeCounts(t *testing.T) {
	tests := []struct {
		nodes        int
		expectedType string
	}{
		{1, "SingleValidator"},
		{2, "Local"},
		{5, "Local"},
		{6, "Testnet"},
		{11, "Testnet"},
		{12, "Mainnet"},
		{21, "Mainnet"},
		{100, "CustomMainnet"},
	}

	for _, tt := range tests {
		t.Run(tt.expectedType, func(t *testing.T) {
			require := require.New(t)

			params := consensus.Config(tt.nodes)

			// Verify params are valid
			err := params.Valid()
			require.NoError(err, "Params should be valid for %d nodes", tt.nodes)

			// Verify K value makes sense
			if tt.nodes == 1 {
				require.Equal(1, params.K, "K should be 1 for single validator")
				require.Equal(1.0, params.Alpha, "Alpha should be 1.0 for single validator")
			} else if tt.nodes > 21 {
				// For large networks, K should equal node count
				require.Equal(tt.nodes, params.K, "K should equal node count for large networks")
			} else {
				// For standard configs, K should be reasonable
				require.Greater(params.K, 0, "K should be positive")
				require.LessOrEqual(params.K, 21, "K should not exceed mainnet default for standard configs")
			}
		})
	}
}
