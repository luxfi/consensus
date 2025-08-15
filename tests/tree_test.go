// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package tests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/photon"
	"github.com/luxfi/consensus/protocol/pulse"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

// TestPhotonSingleton tests a single-choice consensus
func TestPhotonSingleton(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	// Test with photon (unary)
	p := photon.NewPhoton(params)
	require.NoError(p.Add(Red))

	// Should already have preference
	require.Equal(Red, p.Preference())
	require.False(p.Finalized())

	// Vote for Red with sufficient weight
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votes.Add(Red)
	}

	// Need Beta successful polls
	for i := 0; i < int(params.Beta); i++ {
		require.NoError(p.RecordVotes(votes))
	}

	require.True(p.Finalized())
	require.Equal(Red, p.Preference())
}

// TestPulseRecordUnsuccessfulPoll tests unsuccessful poll handling
func TestPulseRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	// Test with pulse (binary)
	p := pulse.NewPulse(params)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Blue))

	// First, make progress with Red
	redVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		redVotes.Add(Red)
	}

	// Record one successful poll
	require.NoError(p.RecordVotes(redVotes))
	require.False(p.Finalized())

	// Now record unsuccessful poll (not enough votes)
	weakVotes := bag.Bag[ids.ID]{}
	weakVotes.Add(Red) // Only 1 vote, less than alpha

	require.NoError(p.RecordVotes(weakVotes))
	require.False(p.Finalized())

	// Should need to start over with Beta polls
	for i := 0; i < int(params.Beta); i++ {
		require.NoError(p.RecordVotes(redVotes))
	}

	require.True(p.Finalized())
	require.Equal(Red, p.Preference())
}

// TestPulseBinary tests binary choice consensus
func TestPulseBinary(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	p := pulse.NewPulse(params)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Blue))

	// Initially should prefer first added
	require.Equal(Red, p.Preference())

	// Vote for Blue
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		blueVotes.Add(Blue)
	}

	require.NoError(p.RecordVotes(blueVotes))
	require.Equal(Blue, p.Preference())

	// Continue voting for Blue to finalize
	for i := 1; i < int(params.Beta); i++ {
		require.NoError(p.RecordVotes(blueVotes))
	}

	require.True(p.Finalized())
	require.Equal(Blue, p.Preference())
}

// TestWaveTrinary tests three-choice consensus
func TestWaveTrinary(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := wave.NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))
	require.NoError(w.Add(Green))

	// Vote for Green
	greenVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		greenVotes.Add(Green)
	}

	// Should switch to Green and finalize
	for i := 0; i < int(params.Beta); i++ {
		require.NoError(w.RecordVotes(greenVotes))
	}

	require.True(w.Finalized())
	require.Equal(Green, w.Preference())
}

// TestWaveTransitiveReset tests preference transitivity
func TestWaveTransitiveReset(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := wave.NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))
	require.NoError(w.Add(Green))

	// First vote for Red
	redVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		redVotes.Add(Red)
	}
	require.NoError(w.RecordVotes(redVotes))
	require.Equal(Red, w.Preference())

	// Switch to Blue
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		blueVotes.Add(Blue)
	}
	require.NoError(w.RecordVotes(blueVotes))
	require.Equal(Blue, w.Preference())

	// Switch to Green
	greenVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		greenVotes.Add(Green)
	}
	require.NoError(w.RecordVotes(greenVotes))
	require.Equal(Green, w.Preference())

	// Finalize on Green
	for i := 1; i < int(params.Beta); i++ {
		require.NoError(w.RecordVotes(greenVotes))
	}

	require.True(w.Finalized())
	require.Equal(Green, w.Preference())
}

// TestWave5Choices tests consensus with 5 choices
func TestWave5Choices(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	colors := []ids.ID{
		ids.GenerateTestID(),
		ids.GenerateTestID(),
		ids.GenerateTestID(),
		ids.GenerateTestID(),
		ids.GenerateTestID(),
	}

	w := wave.NewWave(params)
	for _, color := range colors {
		require.NoError(w.Add(color))
	}

	// Vote for the third color
	targetColor := colors[2]
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votes.Add(targetColor)
	}

	for i := 0; i < int(params.Beta); i++ {
		require.NoError(w.RecordVotes(votes))
	}

	require.True(w.Finalized())
	require.Equal(targetColor, w.Preference())
}

// TestPulseConsistent tests consistency across multiple instances
func TestPulseConsistent(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	// Create multiple instances
	instances := make([]*pulse.Pulse, 5)
	for i := range instances {
		instances[i] = pulse.NewPulse(params)
		require.NoError(instances[i].Add(Red))
		require.NoError(instances[i].Add(Blue))
	}

	// All vote for Blue
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		blueVotes.Add(Blue)
	}

	// All should finalize on Blue
	for _, instance := range instances {
		for i := 0; i < int(params.Beta); i++ {
			require.NoError(instance.RecordVotes(blueVotes))
		}
		require.True(instance.Finalized())
		require.Equal(Blue, instance.Preference())
	}
}

// TestPulseFilterBinaryChildren tests binary choice filtering
func TestPulseFilterBinaryChildren(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	// Create parent and child choices
	parent := ids.GenerateTestID()
	child1 := ids.GenerateTestID()
	child2 := ids.GenerateTestID()

	p := pulse.NewPulse(params)
	require.NoError(p.Add(parent))
	require.NoError(p.Add(child1))
	require.NoError(p.Add(child2))

	// Vote for parent first
	parentVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		parentVotes.Add(parent)
	}
	require.NoError(p.RecordVotes(parentVotes))

	// Then vote for child1
	childVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		childVotes.Add(child1)
	}

	for i := 0; i < int(params.Beta); i++ {
		require.NoError(p.RecordVotes(childVotes))
	}

	require.True(p.Finalized())
	require.Equal(child1, p.Preference())
}

// TestPulseDoubleAdd tests adding same choice twice
func TestPulseDoubleAdd(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	p := pulse.NewPulse(params)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Red)) // Should be idempotent
	require.NoError(p.Add(Blue))

	// Vote for Blue
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		blueVotes.Add(Blue)
	}

	for i := 0; i < int(params.Beta); i++ {
		require.NoError(p.RecordVotes(blueVotes))
	}

	require.True(p.Finalized())
	require.Equal(Blue, p.Preference())
}

// TestPulseRecordPreferencePollBinary tests preference recording in binary
func TestPulseRecordPreferencePollBinary(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	p := pulse.NewPulse(params)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Blue))

	// Vote just enough for preference but not confidence
	prefVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		prefVotes.Add(Blue)
	}

	require.NoError(p.RecordVotes(prefVotes))
	require.Equal(Blue, p.Preference())
	require.False(p.Finalized())

	// Now vote with confidence threshold
	confVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		confVotes.Add(Blue)
	}

	for i := 0; i < int(params.Beta); i++ {
		require.NoError(p.RecordVotes(confVotes))
	}

	require.True(p.Finalized())
	require.Equal(Blue, p.Preference())
}

// TestPhotonRecordPreferencePollUnary tests preference recording in unary
func TestPhotonRecordPreferencePollUnary(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	p := photon.NewPhoton(params)
	require.NoError(p.Add(Red))

	// Vote with preference threshold
	prefVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		prefVotes.Add(Red)
	}

	require.NoError(p.RecordVotes(prefVotes))
	require.Equal(Red, p.Preference())
	require.False(p.Finalized())

	// Continue with confidence votes
	confVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		confVotes.Add(Red)
	}

	for i := 0; i < int(params.Beta); i++ {
		require.NoError(p.RecordVotes(confVotes))
	}

	require.True(p.Finalized())
	require.Equal(Red, p.Preference())
}
