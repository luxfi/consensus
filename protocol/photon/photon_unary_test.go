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

// TestPhotonUnary tests basic photon unary consensus behavior
func TestPhotonUnary(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	p := NewPhoton(params)
	
	// Initially no choice
	require.Equal(ids.Empty, p.Preference())
	require.False(p.Finalized())
	
	// Add single choice
	require.NoError(p.Add(Red))
	require.Equal(Red, p.Preference())
	require.False(p.Finalized())
	
	// Vote for Red
	redVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		redVotes.Add(Red)
	}
	
	// Need Beta rounds to finalize
	for i := 0; i < params.Beta; i++ {
		require.NoError(p.RecordVotes(redVotes))
	}
	
	require.True(p.Finalized())
	require.Equal(Red, p.Preference())
}

// TestPhotonUnaryRecordUnsuccessfulPoll tests unsuccessful poll in photon
func TestPhotonUnaryRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	p := NewPhoton(params)
	require.NoError(p.Add(Red))
	
	// First build confidence
	goodVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		goodVotes.Add(Red)
	}
	
	require.NoError(p.RecordVotes(goodVotes))
	require.Equal(1, p.confidence)
	
	// Unsuccessful poll (not enough votes)
	weakVotes := bag.Bag[ids.ID]{}
	weakVotes.Add(Red)
	
	require.NoError(p.RecordVotes(weakVotes))
	require.Equal(0, p.confidence)
	require.False(p.Finalized())
}

// TestPhotonUnarySingleton tests photon with single voter scenario
func TestPhotonUnarySingleton(t *testing.T) {
	require := require.New(t)

	// Single voter parameters
	params := config.Parameters{
		K:               1,
		AlphaPreference: 1,
		AlphaConfidence: 1,
		Beta:            1,
	}
	
	p := NewPhoton(params)
	require.NoError(p.Add(Red))
	
	// Single vote should finalize immediately
	singleVote := bag.Bag[ids.ID]{}
	singleVote.Add(Red)
	
	require.NoError(p.RecordVotes(singleVote))
	require.True(p.Finalized())
	require.Equal(Red, p.Preference())
}

// TestPhotonUnaryRecordPollPreference tests preference strength
func TestPhotonUnaryRecordPollPreference(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	p := NewPhoton(params)
	require.NoError(p.Add(Red))
	
	// With TestParameters, AlphaPreference == AlphaConfidence
	// So we need to vote with less than AlphaPreference to avoid building confidence
	weakVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference-1; i++ {
		weakVotes.Add(Red)
	}
	
	// This won't increment preference strength or confidence
	require.NoError(p.RecordVotes(weakVotes))
	require.Equal(Red, p.Preference()) // Still Red as initial choice
	require.False(p.Finalized())
	
	// Vote with exactly AlphaPreference to increment preference but also confidence
	prefVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		prefVotes.Add(Red)
	}
	
	require.NoError(p.RecordVotes(prefVotes))
	require.Equal(Red, p.Preference())
	// With TestParameters, this gives confidence = 1, not finalized yet
	require.False(p.Finalized())
}

// TestPhotonUnaryDifferentChoice tests voting for non-added choice
func TestPhotonUnaryDifferentChoice(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	p := NewPhoton(params)
	require.NoError(p.Add(Red))
	
	// Vote for Blue (not added)
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		blueVotes.Add(Blue)
	}
	
	require.NoError(p.RecordVotes(blueVotes))
	require.Equal(Red, p.Preference()) // Still Red
	require.False(p.Finalized())
}

// TestPhotonUnaryLock tests that choice is locked after adding
func TestPhotonUnaryLock(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	p := NewPhoton(params)
	
	// Add Red
	require.NoError(p.Add(Red))
	require.Equal(Red, p.Preference())
	
	// Cannot add Blue
	require.Error(p.Add(Blue))
	require.Equal(Red, p.Preference())
	
	// Can add Red again (idempotent)
	require.NoError(p.Add(Red))
	require.Equal(Red, p.Preference())
}

// TestPhotonUnaryNoChoice tests behavior with no choice
func TestPhotonUnaryNoChoice(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	p := NewPhoton(params)
	
	// No choice added
	require.Equal(ids.Empty, p.Preference())
	require.False(p.Finalized())
	
	// Empty votes should not crash
	emptyVotes := bag.Bag[ids.ID]{}
	require.NoError(p.RecordVotes(emptyVotes))
	require.Equal(ids.Empty, p.Preference())
	require.False(p.Finalized())
	
	// Votes for any color should not affect
	redVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		redVotes.Add(Red)
	}
	
	require.NoError(p.RecordVotes(redVotes))
	require.Equal(ids.Empty, p.Preference())
	require.False(p.Finalized())
}

// TestPhotonUnaryConfidenceBuildup tests confidence counter
func TestPhotonUnaryConfidenceBuildup(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               10,
		AlphaPreference: 6,
		AlphaConfidence: 8,
		Beta:            5,
	}
	
	p := NewPhoton(params)
	require.NoError(p.Add(Red))
	
	// Track confidence progression
	confVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		confVotes.Add(Red)
	}
	
	// Record Beta rounds
	for i := 0; i < params.Beta; i++ {
		require.NoError(p.RecordVotes(confVotes))
	}
	
	// Should finalize after Beta rounds
	require.True(p.Finalized())
}

// TestPhotonUnaryReset tests preference and confidence reset
func TestPhotonUnaryReset(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	p := NewPhoton(params)
	require.NoError(p.Add(Red))
	
	// Build up state
	goodVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		goodVotes.Add(Red)
	}
	
	// Build confidence but not enough to finalize
	// With Beta=2, we need to do only 1 round
	require.NoError(p.RecordVotes(goodVotes))
	// Should have 1 round of confidence, not finalized yet
	require.False(p.Finalized())
	
	// Explicit unsuccessful poll
	p.RecordUnsuccessfulPoll()
	require.False(p.Finalized())
	
	// Can still finalize
	for i := 0; i < params.Beta; i++ {
		require.NoError(p.RecordVotes(goodVotes))
	}
	
	require.True(p.Finalized())
	require.Equal(Red, p.Preference())
}