// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

// TestWaveBinary tests basic wave binary consensus behavior
func TestWaveBinary(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))

	// Both should be valid choices
	require.Equal(Red, w.Preference())

	// Vote for Blue
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		blueVotes.Add(Blue)
	}

	require.NoError(w.RecordVotes(blueVotes))
	require.Equal(Blue, w.Preference())
	require.Equal(Blue, w.wavePreference)
	require.False(w.Finalized())
}

// TestWaveBinaryNoFlakiness tests that wave doesn't finalize without sufficient confidence
func TestWaveBinaryNoFlakiness(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))

	// Alternate votes between Red and Blue
	redVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		redVotes.Add(Red)
	}

	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		blueVotes.Add(Blue)
	}

	// Wave preference should change back and forth
	require.NoError(w.RecordVotes(blueVotes))
	require.Equal(Blue, w.wavePreference)

	require.NoError(w.RecordVotes(redVotes))
	require.Equal(Red, w.wavePreference)

	require.NoError(w.RecordVotes(blueVotes))
	require.Equal(Blue, w.wavePreference)

	// Should never finalize with flaky voting
	require.False(w.Finalized())
}

// TestWaveBinaryConfidenceReset tests confidence reset on unsuccessful polls
func TestWaveBinaryConfidenceReset(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))

	// Build confidence with Blue
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		blueVotes.Add(Blue)
	}

	require.NoError(w.RecordVotes(blueVotes))
	require.Equal(1, w.confidence)

	// Unsuccessful poll resets confidence
	weakVotes := bag.Bag[ids.ID]{}
	weakVotes.Add(Blue)

	require.NoError(w.RecordVotes(weakVotes))
	require.Equal(0, w.confidence)
	require.False(w.Finalized())
}

// TestWaveBinaryAcceptUnknownChoice tests handling of unexpected choices
func TestWaveBinaryAcceptUnknownChoice(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))

	// Vote for a color not added
	greenVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		greenVotes.Add(Green)
	}

	// Should not affect state since Green wasn't added
	require.NoError(w.RecordVotes(greenVotes))
	require.Equal(Red, w.Preference()) // Still original preference
	require.Equal(0, w.confidence)
	require.False(w.Finalized())
}

// TestWaveBinaryDynamicChoices tests that choices can be added dynamically
func TestWaveBinaryDynamicChoices(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := NewWave(params)

	// Add Red
	require.NoError(w.Add(Red))
	require.Equal(Red, w.Preference())

	// Add Blue
	require.NoError(w.Add(Blue))

	// Can add same color again (idempotent)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))

	// Can add new color
	require.NoError(w.Add(Green))
	require.Contains(w.choices, Green)
}

// TestWavePreferenceProgression tests wave preference changes
func TestWavePreferenceProgression(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            3,
	}

	w := NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))
	require.NoError(w.Add(Green))

	// Track preference history
	prefHistory := []ids.ID{}
	wavePrefHistory := []ids.ID{}

	// Vote for Red first
	redVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		redVotes.Add(Red)
	}
	require.NoError(w.RecordVotes(redVotes))
	prefHistory = append(prefHistory, w.Preference())
	wavePrefHistory = append(wavePrefHistory, w.wavePreference)

	// Vote for Blue
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		blueVotes.Add(Blue)
	}
	require.NoError(w.RecordVotes(blueVotes))
	prefHistory = append(prefHistory, w.Preference())
	wavePrefHistory = append(wavePrefHistory, w.wavePreference)

	// Vote for Green
	greenVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		greenVotes.Add(Green)
	}
	require.NoError(w.RecordVotes(greenVotes))
	prefHistory = append(prefHistory, w.Preference())
	wavePrefHistory = append(wavePrefHistory, w.wavePreference)

	// Verify preferences changed
	require.Equal([]ids.ID{Red, Blue, Green}, prefHistory)
	require.Equal([]ids.ID{Red, Blue, Green}, wavePrefHistory)
}

// TestWaveNumPolls tests poll counting
func TestWaveNumPolls(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            10, // High beta to avoid early finalization
	}

	w := NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))

	require.Equal(0, w.numPolls)

	// Each vote increments poll count
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		votes.Add(Blue)
	}

	for i := 0; i < 5; i++ {
		require.NoError(w.RecordVotes(votes))
		require.Equal(i+1, w.numPolls)
	}
}

// TestWavePreferenceStrength tests preference strength tracking
func TestWavePreferenceStrength(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            10, // High beta to avoid early finalization
	}

	w := NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))
	require.NoError(w.Add(Green))

	// Vote for Blue with preference threshold
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		blueVotes.Add(Blue)
	}

	require.NoError(w.RecordVotes(blueVotes))
	require.Equal(1, w.preferenceStrength[Blue])
	require.Equal(0, w.preferenceStrength[Red])
	require.Equal(0, w.preferenceStrength[Green])
	require.False(w.Finalized()) // Should not be finalized after first vote

	// Vote for Blue again
	require.NoError(w.RecordVotes(blueVotes))
	require.False(w.Finalized()) // Should not be finalized yet for this test
	require.Equal(2, w.preferenceStrength[Blue])

	// Vote for Green resets non-Green strengths
	greenVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		greenVotes.Add(Green)
	}

	require.NoError(w.RecordVotes(greenVotes))
	require.Equal(1, w.preferenceStrength[Green])
	require.Equal(0, w.preferenceStrength[Blue])
	require.Equal(0, w.preferenceStrength[Red])
}

// TestWaveRecordUnsuccessfulPoll tests explicit unsuccessful poll
func TestWaveRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))

	// Build up some state but not enough to finalize
	goodVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		goodVotes.Add(Blue)
	}

	// Record only Beta-1 rounds so we're not finalized yet
	for i := 0; i < int(params.Beta)-1; i++ {
		require.NoError(w.RecordVotes(goodVotes))
	}
	require.Equal(1, w.confidence)
	require.Equal(1, w.preferenceStrength[Blue])

	// Explicit unsuccessful poll
	w.RecordUnsuccessfulPoll()
	require.Equal(0, w.confidence)
	// Preference strength is maintained
	require.Equal(1, w.preferenceStrength[Blue])
	require.False(w.Finalized())
}
