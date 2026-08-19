package integration

import (
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/stretchr/testify/require"
)

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
