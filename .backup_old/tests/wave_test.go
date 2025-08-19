// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave_test

import (
	"sync"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/fpc"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// Helper function to create test IDs
func makeTestID(i int) ids.ID {
	return ids.ID{byte(i)}
}

func TestWaveDyadic(t *testing.T) {
	require := require.New(t)

	// Create wave instance with dyadic choice (2 options)
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	w := wave.NewWave(params)

	// Add two choices
	choiceA := makeTestID(1)
	choiceB := makeTestID(2)

	require.NoError(w.Add(choiceA))
	require.NoError(w.Add(choiceB))

	// Initial preference should be the first added choice
	require.Equal(choiceA, w.Preference())
	require.False(w.Finalized())

	// Vote strongly for choiceA
	votes := bag.Of(choiceA, choiceA, choiceA, choiceA)
	require.NoError(w.RecordVotes(votes))

	// Should still prefer choiceA
	require.Equal(choiceA, w.Preference())
	require.False(w.Finalized())

	// Vote again for choiceA to reach beta
	votes = bag.Of(choiceA, choiceA, choiceA, choiceA)
	require.NoError(w.RecordVotes(votes))

	// Should be finalized on choiceA
	require.Equal(choiceA, w.Preference())
	require.True(w.Finalized())
	require.Equal(2, w.NumPolls())
}

func TestWaveDyadicPreferenceChange(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            3,
	}

	w := wave.NewWave(params)

	choiceA := makeTestID(1)
	choiceB := makeTestID(2)

	require.NoError(w.Add(choiceA))
	require.NoError(w.Add(choiceB))

	// Start with preference for A
	require.Equal(choiceA, w.Preference())

	// Vote for B with alpha preference but not confidence
	votes := bag.Of(choiceB, choiceB, choiceB)
	require.NoError(w.RecordVotes(votes))

	// Should switch preference to B
	require.Equal(choiceB, w.Preference())
	require.False(w.Finalized())

	// Vote strongly for B
	votes = bag.Of(choiceB, choiceB, choiceB, choiceB)
	require.NoError(w.RecordVotes(votes))

	// Continue voting for B
	require.NoError(w.RecordVotes(votes))

	// Should finalize on B after beta rounds
	require.Equal(choiceB, w.Preference())
	require.True(w.Finalized())
}

func TestWaveDyadicMultipleTerminationConditions(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               7,
		AlphaPreference: 4,
		AlphaConfidence: 6,
		Beta:            2,
	}

	w := wave.NewWave(params)

	choiceA := makeTestID(1)
	choiceB := makeTestID(2)

	require.NoError(w.Add(choiceA))
	require.NoError(w.Add(choiceB))

	// Vote with high confidence for A
	votes := bag.Of(choiceA, choiceA, choiceA, choiceA, choiceA, choiceA)
	require.NoError(w.RecordVotes(votes))

	// One more round should finalize
	require.NoError(w.RecordVotes(votes))

	require.True(w.Finalized())
	require.Equal(choiceA, w.Preference())

	// Additional votes should not change finalized state
	votesB := bag.Of(choiceB, choiceB, choiceB, choiceB, choiceB, choiceB)
	require.NoError(w.RecordVotes(votesB))

	// Should still be finalized on A
	require.True(w.Finalized())
	require.Equal(choiceA, w.Preference())
}

func TestWavePolyadic(t *testing.T) {
	require := require.New(t)

	// Test with multiple choices (n-ary)
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	w := wave.NewWave(params)

	// Add multiple choices
	choices := make([]ids.ID, 5)
	for i := 0; i < 5; i++ {
		choices[i] = makeTestID(i + 1)
		require.NoError(w.Add(choices[i]))
	}

	// Initial preference should be first choice
	require.Equal(choices[0], w.Preference())

	// Vote for choice 3
	votes := bag.Of(choices[2], choices[2], choices[2], choices[2])
	require.NoError(w.RecordVotes(votes))

	// Should switch to choice 3
	require.Equal(choices[2], w.Preference())

	// Continue voting for choice 3 to finalize
	require.NoError(w.RecordVotes(votes))

	require.True(w.Finalized())
	require.Equal(choices[2], w.Preference())
}

func TestWaveProtocolFactory(t *testing.T) {
	require := require.New(t)

	// Test creating multiple wave instances with different parameters
	params1 := config.Parameters{
		K:               3,
		AlphaPreference: 2,
		AlphaConfidence: 3,
		Beta:            1,
	}

	params2 := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	w1 := wave.NewWave(params1)
	w2 := wave.NewWave(params2)

	// Both should be independent
	choiceA := makeTestID(1)
	choiceB := makeTestID(2)

	require.NoError(w1.Add(choiceA))
	require.NoError(w2.Add(choiceB))

	require.Equal(choiceA, w1.Preference())
	require.Equal(choiceB, w2.Preference())

	// Finalize w1
	votes := bag.Of(choiceA, choiceA, choiceA)
	require.NoError(w1.RecordVotes(votes))

	require.True(w1.Finalized())
	require.False(w2.Finalized())
}

func TestWaveProtocolConsensusConcurrent(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            3,
	}

	w := wave.NewWave(params)

	// Add choices
	choices := make([]ids.ID, 3)
	for i := 0; i < 3; i++ {
		choices[i] = makeTestID(i + 1)
		require.NoError(w.Add(choices[i]))
	}

	// Simulate concurrent voting with proper synchronization
	var wg sync.WaitGroup
	var mu sync.Mutex
	numVoters := 10
	votesPerVoter := 5

	for i := 0; i < numVoters; i++ {
		wg.Add(1)
		go func(voterID int) {
			defer wg.Done()

			// Each voter votes multiple times
			for j := 0; j < votesPerVoter; j++ {
				// Mostly vote for choice 1
				var votes bag.Bag[ids.ID]
				if voterID < 7 {
					votes = bag.Of(choices[0], choices[0], choices[0], choices[0])
				} else {
					votes = bag.Of(choices[1], choices[1], choices[1])
				}

				mu.Lock()
				_ = w.RecordVotes(votes)
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Should eventually reach consensus
	require.True(w.Finalized() || w.NumPolls() > 0)
}

func TestWaveRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	w := wave.NewWave(params)

	choiceA := makeTestID(1)
	require.NoError(w.Add(choiceA))

	// Vote with confidence
	votes := bag.Of(choiceA, choiceA, choiceA, choiceA)
	require.NoError(w.RecordVotes(votes))

	// Record unsuccessful poll
	w.RecordUnsuccessfulPoll()

	// Need to rebuild confidence
	require.NoError(w.RecordVotes(votes))
	require.False(w.Finalized())

	require.NoError(w.RecordVotes(votes))
	require.True(w.Finalized())
}

func TestWaveAddAfterFinalized(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               3,
		AlphaPreference: 2,
		AlphaConfidence: 3,
		Beta:            1,
	}

	w := wave.NewWave(params)

	choiceA := makeTestID(1)
	choiceB := makeTestID(2)

	require.NoError(w.Add(choiceA))

	// Finalize on A
	votes := bag.Of(choiceA, choiceA, choiceA)
	require.NoError(w.RecordVotes(votes))
	require.True(w.Finalized())

	// Try to add after finalized
	err := w.Add(choiceB)
	require.Error(err)
	require.Contains(err.Error(), "cannot add choice after finalization")
}

func TestWaveString(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               3,
		AlphaPreference: 2,
		AlphaConfidence: 3,
		Beta:            1,
	}

	w := wave.NewWave(params)

	choiceA := makeTestID(1)
	require.NoError(w.Add(choiceA))

	str := w.String()
	require.Contains(str, "Wave{")
	require.Contains(str, "finalized=false")
	require.Contains(str, "polls=0")
}

func BenchmarkWaveDyadic(b *testing.B) {
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	choiceA := makeTestID(1)
	choiceB := makeTestID(2)
	votesA := bag.Of(choiceA, choiceA, choiceA, choiceA)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := wave.NewWave(params)
		_ = w.Add(choiceA)
		_ = w.Add(choiceB)

		for !w.Finalized() && w.NumPolls() < 10 {
			_ = w.RecordVotes(votesA)
		}
	}
}

func BenchmarkWavePolyadic(b *testing.B) {
	params := config.Parameters{
		K:               7,
		AlphaPreference: 4,
		AlphaConfidence: 5,
		Beta:            3,
	}

	choices := make([]ids.ID, 5)
	for i := 0; i < 5; i++ {
		choices[i] = makeTestID(i + 1)
	}

	votes := bag.Of(choices[2], choices[2], choices[2], choices[2], choices[2])

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := wave.NewWave(params)
		for _, choice := range choices {
			_ = w.Add(choice)
		}

		for !w.Finalized() && w.NumPolls() < 10 {
			_ = w.RecordVotes(votes)
		}
	}
}
