// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)


// TestPhotonBinary tests photon behavior in binary choice scenarios
func TestPhotonBinary(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	// Photon is unary, but we can test binary-like behavior with two instances
	redPhoton := NewPhoton(params)
	bluePhoton := NewPhoton(params)
	
	require.NoError(redPhoton.Add(Red))
	require.NoError(bluePhoton.Add(Blue))
	
	// Both should prefer their single choice
	require.Equal(Red, redPhoton.Preference())
	require.Equal(Blue, bluePhoton.Preference())
	
	// Vote for Red on red instance
	redVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		redVotes.Add(Red)
	}
	
	for i := 0; i < params.Beta; i++ {
		require.NoError(redPhoton.RecordVotes(redVotes))
	}
	
	require.True(redPhoton.Finalized())
	require.Equal(Red, redPhoton.Preference())
	
	// Blue instance with blue votes
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		blueVotes.Add(Blue)
	}
	
	for i := 0; i < params.Beta; i++ {
		require.NoError(bluePhoton.RecordVotes(blueVotes))
	}
	
	require.True(bluePhoton.Finalized())
	require.Equal(Blue, bluePhoton.Preference())
}

// TestPhotonBinaryRecordPollPreference tests preference strength tracking
func TestPhotonBinaryRecordPollPreference(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	p := NewPhoton(params)
	require.NoError(p.Add(Red))
	
	// Vote with preference threshold
	prefVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		prefVotes.Add(Red)
	}
	
	require.NoError(p.RecordVotes(prefVotes))
	require.Equal(Red, p.Preference())
	require.False(p.Finalized())
	
	// Didn't meet confidence threshold - shouldn't finalize
	require.False(p.Finalized())
}

// TestPhotonBinaryRecordUnsuccessfulPoll tests unsuccessful poll handling
func TestPhotonBinaryRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	p := NewPhoton(params)
	require.NoError(p.Add(Red))
	
	// First successful poll
	successVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		successVotes.Add(Red)
	}
	
	require.NoError(p.RecordVotes(successVotes))
	// Should have 1 round of confidence
	require.False(p.Finalized())
	
	// Unsuccessful poll
	weakVotes := bag.Bag[ids.ID]{}
	weakVotes.Add(Red) // Only 1 vote
	
	require.NoError(p.RecordVotes(weakVotes))
	// Confidence should be reset
	require.False(p.Finalized())
	
	// Can still finalize with enough successful polls
	for i := 0; i < params.Beta; i++ {
		require.NoError(p.RecordVotes(successVotes))
	}
	
	require.True(p.Finalized())
}

// TestPhotonBinaryAcceptUnknownChoice tests handling of unexpected choices
func TestPhotonBinaryAcceptUnknownChoice(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	p := NewPhoton(params)
	require.NoError(p.Add(Red))
	
	// Try to vote for a different color
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		blueVotes.Add(Blue)
	}
	
	// Should not affect photon since it only tracks Red
	require.NoError(p.RecordVotes(blueVotes))
	require.Equal(Red, p.Preference())
	require.False(p.Finalized())
	
	// Can still finalize with correct color
	redVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		redVotes.Add(Red)
	}
	
	for i := 0; i < params.Beta; i++ {
		require.NoError(p.RecordVotes(redVotes))
	}
	
	require.True(p.Finalized())
	require.Equal(Red, p.Preference())
}

// TestPhotonBinaryLockChoice tests that choice is locked once added
func TestPhotonBinaryLockChoice(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	p := NewPhoton(params)
	
	// Add Red
	require.NoError(p.Add(Red))
	require.Equal(Red, p.Preference())
	
	// Try to add Blue - should fail
	require.Error(p.Add(Blue))
	require.Equal(Red, p.Preference())
	
	// Can add Red again (idempotent)
	require.NoError(p.Add(Red))
	require.Equal(Red, p.Preference())
}

// TestPhotonConfidenceProgression tests confidence counter progression
func TestPhotonConfidenceProgression(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            3,
	}
	
	p := NewPhoton(params)
	require.NoError(p.Add(Red))
	
	// Track confidence progression
	// Vote with exactly confidence threshold
	confVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		confVotes.Add(Red)
	}
	
	// Record Beta polls
	for i := 0; i < params.Beta; i++ {
		require.NoError(p.RecordVotes(confVotes))
	}
	
	// Should finalize after Beta rounds
	require.True(p.Finalized())
}

// TestPhotonRecordUnsuccessfulPoll tests explicit unsuccessful poll
func TestPhotonRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	p := NewPhoton(params)
	require.NoError(p.Add(Red))
	
	// Build up some confidence
	goodVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		goodVotes.Add(Red)
	}
	
	require.NoError(p.RecordVotes(goodVotes))
	require.NoError(p.RecordVotes(goodVotes))
	// Should have 2 rounds of confidence
	require.False(p.Finalized())
	
	// Explicit unsuccessful poll
	p.RecordUnsuccessfulPoll()
	require.False(p.Finalized())
}