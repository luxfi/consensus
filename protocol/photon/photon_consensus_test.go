// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon_test

import (
	"sync"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/photon"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// Helper function to create test IDs
func makeTestID(i int) ids.ID {
	return ids.ID{byte(i)}
}

func TestDyadicWave(t *testing.T) {
	require := require.New(t)

	// Test dyadic consensus using Photon protocol
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	// Create two Photon instances for dyadic choice
	photonA := photon.NewPhoton(params)
	photonB := photon.NewPhoton(params)

	choiceA := makeTestID(1)
	choiceB := makeTestID(2)

	require.NoError(photonA.Add(choiceA))
	require.NoError(photonB.Add(choiceB))

	// Vote for A strongly
	votesA := bag.Of(choiceA, choiceA, choiceA, choiceA)
	require.NoError(photonA.RecordVotes(votesA))

	// Vote for B weakly
	votesB := bag.Of(choiceB, choiceB)
	require.NoError(photonB.RecordVotes(votesB))

	// A should progress towards finalization
	require.False(photonA.Finalized())

	// Another strong vote for A should finalize
	require.NoError(photonA.RecordVotes(votesA))
	require.True(photonA.Finalized())

	// B should not be finalized
	require.False(photonB.Finalized())
}

func TestDyadicWavePreferenceChange(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            3,
	}

	photon := photon.NewPhoton(params)
	choice := makeTestID(1)
	require.NoError(photon.Add(choice))

	// Vote with preference but not confidence (resets confidence to 0)
	votes := bag.Of(choice, choice, choice)
	require.NoError(photon.RecordVotes(votes))
	require.False(photon.Finalized())
	require.Equal(0, photon.Confidence())

	// Vote with confidence (confidence = 1)
	votesStrong := bag.Of(choice, choice, choice, choice)
	require.NoError(photon.RecordVotes(votesStrong))
	require.False(photon.Finalized())
	require.Equal(1, photon.Confidence())

	// Second vote with confidence (confidence = 2)
	require.NoError(photon.RecordVotes(votesStrong))
	require.False(photon.Finalized())
	require.Equal(2, photon.Confidence())

	// Third vote with confidence to reach beta (confidence = 3)
	require.NoError(photon.RecordVotes(votesStrong))
	require.True(photon.Finalized())
	require.Equal(3, photon.Confidence())
}

func TestDyadicWaveMultipleTerminationConditions(t *testing.T) {
	require := require.New(t)

	// Test with different termination parameters
	params := config.Parameters{
		K:               7,
		AlphaPreference: 4,
		AlphaConfidence: 6,
		Beta:            2,
	}

	photon := photon.NewPhoton(params)
	choice := makeTestID(1)
	require.NoError(photon.Add(choice))

	// Vote with high confidence
	votes := bag.Of(choice, choice, choice, choice, choice, choice)
	require.NoError(photon.RecordVotes(votes))
	require.Equal(1, photon.Confidence())

	// One more round should finalize (beta=2)
	require.NoError(photon.RecordVotes(votes))
	require.True(photon.Finalized())
	require.Equal(2, photon.Confidence())

	// Additional votes shouldn't change finalized state
	require.NoError(photon.RecordVotes(votes))
	require.True(photon.Finalized())
}

func TestPolyadicWave(t *testing.T) {
	require := require.New(t)

	// Test with multiple Photon instances (simulating polyadic choice)
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	// Create multiple photon instances for each choice
	numChoices := 5
	photons := make([]*photon.Photon, numChoices)
	choices := make([]ids.ID, numChoices)

	for i := 0; i < numChoices; i++ {
		photons[i] = photon.NewPhoton(params)
		choices[i] = makeTestID(i + 1)
		require.NoError(photons[i].Add(choices[i]))
	}

	// Vote strongly for choice 3
	votes := bag.Of(choices[2], choices[2], choices[2], choices[2])
	require.NoError(photons[2].RecordVotes(votes))

	// Vote weakly for other choices
	for i := 0; i < numChoices; i++ {
		if i != 2 {
			weakVotes := bag.Of(choices[i], choices[i])
			require.NoError(photons[i].RecordVotes(weakVotes))
		}
	}

	// Continue voting for choice 3 to finalize
	require.NoError(photons[2].RecordVotes(votes))
	require.True(photons[2].Finalized())

	// Others should not be finalized
	for i := 0; i < numChoices; i++ {
		if i != 2 {
			require.False(photons[i].Finalized())
		}
	}
}

func TestWaveFactory(t *testing.T) {
	require := require.New(t)

	// Test creating multiple photon instances with different parameters
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

	p1 := photon.NewPhoton(params1)
	p2 := photon.NewPhoton(params2)

	choice1 := makeTestID(1)
	choice2 := makeTestID(2)

	require.NoError(p1.Add(choice1))
	require.NoError(p2.Add(choice2))

	// Finalize p1 quickly (beta=1)
	votes1 := bag.Of(choice1, choice1, choice1)
	require.NoError(p1.RecordVotes(votes1))
	require.True(p1.Finalized())

	// p2 should still not be finalized
	votes2 := bag.Of(choice2, choice2, choice2, choice2)
	require.NoError(p2.RecordVotes(votes2))
	require.False(p2.Finalized())

	// One more vote for p2
	require.NoError(p2.RecordVotes(votes2))
	require.True(p2.Finalized())
}

func TestWaveConsensusConcurrent(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            3,
	}

	photon := photon.NewPhoton(params)
	choice := makeTestID(1)
	require.NoError(photon.Add(choice))

	// Simulate concurrent voting with proper synchronization
	var wg sync.WaitGroup
	var mu sync.Mutex
	numVoters := 10
	votesPerVoter := 5

	for i := 0; i < numVoters; i++ {
		wg.Add(1)
		go func(voterID int) {
			defer wg.Done()

			for j := 0; j < votesPerVoter; j++ {
				// Most voters vote strongly
				var votes bag.Bag[ids.ID]
				if voterID < 7 {
					votes = bag.Of(choice, choice, choice, choice)
				} else {
					votes = bag.Of(choice, choice, choice)
				}

				mu.Lock()
				_ = photon.RecordVotes(votes)
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Should be finalized after concurrent voting
	require.True(photon.Finalized())
	require.Equal(choice, photon.Preference())
}

func TestPhotonRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	photon := photon.NewPhoton(params)
	choice := makeTestID(1)
	require.NoError(photon.Add(choice))

	// Vote with confidence
	votes := bag.Of(choice, choice, choice, choice)
	require.NoError(photon.RecordVotes(votes))
	require.Equal(1, photon.Confidence())

	// Record unsuccessful poll
	photon.RecordUnsuccessfulPoll()
	require.Equal(0, photon.Confidence())

	// Need to rebuild confidence
	require.NoError(photon.RecordVotes(votes))
	require.Equal(1, photon.Confidence())

	require.NoError(photon.RecordVotes(votes))
	require.True(photon.Finalized())
}

func TestPhotonAddMultipleChoices(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               3,
		AlphaPreference: 2,
		AlphaConfidence: 3,
		Beta:            1,
	}

	photon := photon.NewPhoton(params)

	choice1 := makeTestID(1)
	choice2 := makeTestID(2)

	// Add first choice
	require.NoError(photon.Add(choice1))
	require.Equal(choice1, photon.Preference())

	// Try to add different choice (should fail)
	err := photon.Add(choice2)
	require.Error(err)
	require.Contains(err.Error(), "already has a choice")

	// Adding same choice should be fine
	require.NoError(photon.Add(choice1))
}

func TestPhotonString(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               3,
		AlphaPreference: 2,
		AlphaConfidence: 3,
		Beta:            1,
	}

	photon := photon.NewPhoton(params)
	choice := makeTestID(1)
	require.NoError(photon.Add(choice))

	str := photon.String()
	require.Contains(str, "Photon{")
	require.Contains(str, "finalized=false")
}

func BenchmarkDyadicWave(b *testing.B) {
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	choice := makeTestID(1)
	votes := bag.Of(choice, choice, choice, choice)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		photon := photon.NewPhoton(params)
		_ = photon.Add(choice)

		for !photon.Finalized() {
			_ = photon.RecordVotes(votes)
		}
	}
}

func BenchmarkPolyadicWave(b *testing.B) {
	params := config.Parameters{
		K:               7,
		AlphaPreference: 4,
		AlphaConfidence: 5,
		Beta:            3,
	}

	numChoices := 5
	choices := make([]ids.ID, numChoices)
	for i := 0; i < numChoices; i++ {
		choices[i] = makeTestID(i + 1)
	}

	votes := bag.Of(choices[2], choices[2], choices[2], choices[2], choices[2])

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		photons := make([]*photon.Photon, numChoices)
		for j := 0; j < numChoices; j++ {
			photons[j] = photon.NewPhoton(params)
			_ = photons[j].Add(choices[j])
		}

		// Vote until choice 2 finalizes
		for !photons[2].Finalized() {
			_ = photons[2].RecordVotes(votes)
		}
	}
}
