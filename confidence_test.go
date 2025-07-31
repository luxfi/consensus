// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	. "github.com/luxfi/consensus"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/photon"
	"github.com/luxfi/consensus/protocol/pulse"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

// TestConfidenceReset tests confidence reset behavior across protocols
func TestConfidenceReset(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               2,
		AlphaPreference: 2,
		AlphaConfidence: 2,
		Beta:            5, // Higher beta to test confidence reset
	}
	
	// Test photon confidence reset
	t.Run("Photon", func(t *testing.T) {
		p := photon.NewPhoton(params)
		require.NoError(p.Add(Red))
		
		// Build confidence
		goodVotes := bag.Bag[ids.ID]{}
		for i := 0; i < params.AlphaConfidence; i++ {
			goodVotes.Add(Red)
		}
		
		require.NoError(p.RecordVotes(goodVotes))
		require.NoError(p.RecordVotes(goodVotes))
		// Should have 2 rounds of confidence but not finalized
		require.False(p.Finalized())
		
		// Weak vote resets confidence - should need Beta rounds again
		weakVotes := bag.Bag[ids.ID]{}
		weakVotes.Add(Red)
		
		require.NoError(p.RecordVotes(weakVotes))
		// Should need full Beta rounds to finalize
		for i := 0; i < params.Beta; i++ {
			require.NoError(p.RecordVotes(goodVotes))
		}
		require.True(p.Finalized())
	})
	
	// Test pulse confidence reset
	t.Run("Pulse", func(t *testing.T) {
		p := pulse.NewPulse(params)
		require.NoError(p.Add(Red))
		require.NoError(p.Add(Blue))
		
		// Build confidence with Blue
		blueVotes := bag.Bag[ids.ID]{}
		for i := 0; i < params.AlphaConfidence; i++ {
			blueVotes.Add(Blue)
		}
		
		require.NoError(p.RecordVotes(blueVotes))
		require.NoError(p.RecordVotes(blueVotes))
		require.False(p.Finalized()) // Not yet finalized
		
		// Vote for Red resets confidence
		redVotes := bag.Bag[ids.ID]{}
		for i := 0; i < params.AlphaPreference; i++ {
			redVotes.Add(Red)
		}
		
		require.NoError(p.RecordVotes(redVotes))
		require.Equal(Red, p.Preference())
		
		// Should need full Beta rounds to finalize on Red
		for i := 0; i < params.Beta; i++ {
			require.NoError(p.RecordVotes(redVotes))
		}
		require.True(p.Finalized())
	})
	
	// Test wave confidence reset
	t.Run("Wave", func(t *testing.T) {
		w := wave.NewWave(params)
		require.NoError(w.Add(Red))
		require.NoError(w.Add(Blue))
		require.NoError(w.Add(Green))
		
		// Build confidence with Green
		greenVotes := bag.Bag[ids.ID]{}
		for i := 0; i < params.AlphaConfidence; i++ {
			greenVotes.Add(Green)
		}
		
		require.NoError(w.RecordVotes(greenVotes))
		require.NoError(w.RecordVotes(greenVotes))
		require.NoError(w.RecordVotes(greenVotes))
		require.False(w.Finalized()) // Not yet finalized
		
		// Empty votes reset confidence
		emptyVotes := bag.Bag[ids.ID]{}
		require.NoError(w.RecordVotes(emptyVotes))
		
		// Should need full Beta rounds to finalize
		for i := 0; i < params.Beta; i++ {
			require.NoError(w.RecordVotes(greenVotes))
		}
		require.True(w.Finalized())
	})
}

// TestConfidenceThreshold tests alpha confidence threshold behavior
func TestConfidenceThreshold(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               10,
		AlphaPreference: 5,
		AlphaConfidence: 7,
		Beta:            3,
	}
	
	p := pulse.NewPulse(params)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Blue))
	
	// Just below confidence threshold
	belowVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence-1; i++ {
		belowVotes.Add(Blue)
	}
	
	require.NoError(p.RecordVotes(belowVotes))
	// No confidence gain - shouldn't finalize even with Beta rounds
	for i := 0; i < params.Beta; i++ {
		require.NoError(p.RecordVotes(belowVotes))
	}
	require.False(p.Finalized())
	
	// Exactly at confidence threshold
	atVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		atVotes.Add(Blue)
	}
	
	require.NoError(p.RecordVotes(atVotes))
	// Confidence gained - one round counted
	
	// Above confidence threshold
	aboveVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence+2; i++ {
		aboveVotes.Add(Blue)
	}
	
	require.NoError(p.RecordVotes(aboveVotes))
	// Should finalize after Beta-2 more rounds
	for i := 0; i < params.Beta-2; i++ {
		require.NoError(p.RecordVotes(aboveVotes))
	}
	require.True(p.Finalized())
}

// TestConfidenceProgression tests beta rounds of confidence
func TestConfidenceProgression(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               10,
		AlphaPreference: 6,
		AlphaConfidence: 8,
		Beta:            5,
	}
	
	w := wave.NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))
	
	// Track confidence through Beta rounds
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		blueVotes.Add(Blue)
	}
	
	// Record Beta rounds
	for i := 0; i < params.Beta; i++ {
		require.NoError(w.RecordVotes(blueVotes))
	}
	
	require.True(w.Finalized())
}

// TestConfidenceWithPreferenceChange tests confidence during preference changes
func TestConfidenceWithPreferenceChange(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               3,
		AlphaPreference: 2,
		AlphaConfidence: 2,
		Beta:            1, // Low beta for immediate finalization
	}
	
	p := pulse.NewPulse(params)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Blue))
	require.NoError(p.Add(Green))
	
	// Build confidence with Red
	redVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		redVotes.Add(Red)
	}
	
	require.NoError(p.RecordVotes(redVotes))
	require.Equal(Red, p.Preference())
	// With Beta=1, it should finalize after 1 round
	require.True(p.Finalized())
	
	// Once finalized, preference shouldn't change
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		blueVotes.Add(Blue)
	}
	
	require.NoError(p.RecordVotes(blueVotes))
	require.Equal(Red, p.Preference()) // Should still be Red
	require.True(p.Finalized())
}

// TestConfidenceConsistency tests confidence consistency across instances
func TestConfidenceConsistency(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	// Create multiple instances
	instances := make([]*wave.Wave, 5)
	for i := range instances {
		instances[i] = wave.NewWave(params)
		require.NoError(instances[i].Add(Red))
		require.NoError(instances[i].Add(Blue))
	}
	
	// All vote identically
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		blueVotes.Add(Blue)
	}
	
	// Record 3 rounds
	for round := 0; round < 3; round++ {
		for _, instance := range instances {
			require.NoError(instance.RecordVotes(blueVotes))
		}
		
		// All should be progressing together
	}
	
	// All should have same state
	for _, instance := range instances {
		require.Equal(Blue, instance.Preference())
		require.True(instance.Finalized()) // With TestParameters Beta=1, should be finalized
	}
}

// TestConfidenceEdgeCases tests edge cases in confidence handling
func TestConfidenceEdgeCases(t *testing.T) {
	require := require.New(t)

	// Test with Beta = 1
	t.Run("Beta=1", func(t *testing.T) {
		params := config.Parameters{
			K:               5,
			AlphaPreference: 3,
			AlphaConfidence: 4,
			Beta:            1,
		}
		
		p := photon.NewPhoton(params)
		require.NoError(p.Add(Red))
		
		votes := bag.Bag[ids.ID]{}
		for i := 0; i < params.AlphaConfidence; i++ {
			votes.Add(Red)
		}
		
		require.NoError(p.RecordVotes(votes))
		require.True(p.Finalized())
	})
	
	// Test with all alphas equal
	t.Run("AllAlphasEqual", func(t *testing.T) {
		params := config.Parameters{
			K:               10,
			AlphaPreference: 7,
			AlphaConfidence: 7,
			Beta:            3,
		}
		
		w := wave.NewWave(params)
		require.NoError(w.Add(Red))
		require.NoError(w.Add(Blue))
		
		votes := bag.Bag[ids.ID]{}
		for i := 0; i < params.AlphaConfidence; i++ {
			votes.Add(Blue)
		}
		
		require.NoError(w.RecordVotes(votes))
		require.Equal(Blue, w.Preference())
	})
}

// TestConfidenceAfterFinalized tests confidence behavior after finalization
func TestConfidenceAfterFinalized(t *testing.T) {
	require := require.New(t)
	
	// Use parameters that guarantee finalization
	customParams := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}
	
	p := pulse.NewPulse(customParams)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Blue))
	
	// Finalize on Blue
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < customParams.AlphaConfidence; i++ {
		blueVotes.Add(Blue)
	}
	
	for i := 0; i < customParams.Beta; i++ {
		require.NoError(p.RecordVotes(blueVotes))
	}
	
	require.True(p.Finalized())
	
	// Additional votes shouldn't change state
	require.NoError(p.RecordVotes(blueVotes))
	require.True(p.Finalized())
	
	// Even conflicting votes shouldn't change
	redVotes := bag.Bag[ids.ID]{}
	for i := 0; i < customParams.AlphaConfidence; i++ {
		redVotes.Add(Red)
	}
	
	require.NoError(p.RecordVotes(redVotes))
	require.True(p.Finalized())
	require.Equal(Blue, p.Preference())
}