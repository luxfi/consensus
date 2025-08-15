// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon_test

import (
	"testing"
	
	"github.com/stretchr/testify/require"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/photon"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

func TestDyadicPhoton(t *testing.T) {
	require := require.New(t)
	
	// Test basic dyadic photon consensus
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}
	
	// Create photon for dyadic consensus
	p := photon.NewPhoton(params)
	
	// Add choice
	choice := ids.ID{0x01}
	require.NoError(p.Add(choice))
	require.Equal(choice, p.Preference())
	
	// Initial state
	require.False(p.Finalized())
	require.Equal(0, p.Confidence())
	require.Equal(0, p.PreferenceStrength())
	
	// Vote with preference threshold
	votes := bag.Of(choice, choice, choice)
	require.NoError(p.RecordVotes(votes))
	
	// Should have preference strength but no confidence
	require.Equal(1, p.PreferenceStrength())
	require.Equal(0, p.Confidence())
	require.False(p.Finalized())
	
	// Vote with confidence threshold
	votesStrong := bag.Of(choice, choice, choice, choice)
	require.NoError(p.RecordVotes(votesStrong))
	
	// Should have confidence now
	require.Equal(2, p.PreferenceStrength())
	require.Equal(1, p.Confidence())
	require.False(p.Finalized())
	
	// One more vote to reach beta
	require.NoError(p.RecordVotes(votesStrong))
	
	// Should be finalized
	require.Equal(3, p.PreferenceStrength())
	require.Equal(2, p.Confidence())
	require.True(p.Finalized())
}

func TestDyadicPhotonConfidence(t *testing.T) {
	require := require.New(t)
	
	params := config.Parameters{
		K:               7,
		AlphaPreference: 4,
		AlphaConfidence: 6,
		Beta:            3,
	}
	
	p := photon.NewPhoton(params)
	choice := ids.ID{0x02}
	require.NoError(p.Add(choice))
	
	// Vote below preference threshold
	weakVotes := bag.Of(choice, choice, choice)
	require.NoError(p.RecordVotes(weakVotes))
	require.Equal(0, p.Confidence())
	require.Equal(0, p.PreferenceStrength())
	
	// Vote at preference but below confidence
	prefVotes := bag.Of(choice, choice, choice, choice)
	require.NoError(p.RecordVotes(prefVotes))
	require.Equal(1, p.PreferenceStrength())
	require.Equal(0, p.Confidence())
	
	// Vote at confidence threshold
	confVotes := bag.Of(choice, choice, choice, choice, choice, choice)
	require.NoError(p.RecordVotes(confVotes))
	require.Equal(2, p.PreferenceStrength())
	require.Equal(1, p.Confidence())
	
	// Weak vote should reset confidence
	require.NoError(p.RecordVotes(weakVotes))
	require.Equal(2, p.PreferenceStrength()) // Preference strength doesn't reset
	require.Equal(0, p.Confidence()) // Confidence resets
	
	// Build confidence again
	require.NoError(p.RecordVotes(confVotes))
	require.Equal(3, p.PreferenceStrength())
	require.Equal(1, p.Confidence())
	
	require.NoError(p.RecordVotes(confVotes))
	require.Equal(4, p.PreferenceStrength())
	require.Equal(2, p.Confidence())
	
	require.NoError(p.RecordVotes(confVotes))
	require.Equal(5, p.PreferenceStrength())
	require.Equal(3, p.Confidence())
	require.True(p.Finalized())
}

func TestDyadicPhotonFinalization(t *testing.T) {
	require := require.New(t)
	
	// Test with low beta for quick finalization
	params := config.Parameters{
		K:               3,
		AlphaPreference: 2,
		AlphaConfidence: 3,
		Beta:            1,
	}
	
	p := photon.NewPhoton(params)
	choice := ids.ID{0x03}
	require.NoError(p.Add(choice))
	
	// Single vote with confidence should finalize (beta=1)
	votes := bag.Of(choice, choice, choice)
	require.NoError(p.RecordVotes(votes))
	
	require.Equal(1, p.PreferenceStrength())
	require.Equal(1, p.Confidence())
	require.True(p.Finalized())
	
	// Additional votes after finalization should not change state
	require.NoError(p.RecordVotes(votes))
	require.True(p.Finalized())
	require.Equal(1, p.Confidence()) // Confidence doesn't increase after finalization
}

func TestDyadicPhotonPreferenceSwitch(t *testing.T) {
	require := require.New(t)
	
	// For dyadic consensus, photon can only handle one choice
	// This test verifies that behavior
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}
	
	p := photon.NewPhoton(params)
	
	choiceA := ids.ID{0x04}
	choiceB := ids.ID{0x05}
	
	// Add first choice
	require.NoError(p.Add(choiceA))
	require.Equal(choiceA, p.Preference())
	
	// Try to add different choice (should fail)
	err := p.Add(choiceB)
	require.Error(err)
	require.Contains(err.Error(), "already has a choice")
	
	// Preference should still be choiceA
	require.Equal(choiceA, p.Preference())
	
	// Vote for choiceA
	votes := bag.Of(choiceA, choiceA, choiceA, choiceA)
	require.NoError(p.RecordVotes(votes))
	
	// Voting for choiceB should have no effect (not in the photon)
	votesB := bag.Of(choiceB, choiceB, choiceB, choiceB)
	require.NoError(p.RecordVotes(votesB))
	
	// Confidence should be reset since choiceB isn't tracked
	require.Equal(0, p.Confidence())
	
	// Vote for choiceA again
	require.NoError(p.RecordVotes(votes))
	require.Equal(1, p.Confidence())
	
	require.NoError(p.RecordVotes(votes))
	require.True(p.Finalized())
	require.Equal(choiceA, p.Preference())
}

func TestDyadicPhotonRecordPrism(t *testing.T) {
	require := require.New(t)
	
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}
	
	p := photon.NewPhoton(params)
	choice := ids.ID{0x06}
	require.NoError(p.Add(choice))
	
	// RecordPrism should work the same as RecordVotes
	votes := bag.Of(choice, choice, choice, choice)
	require.NoError(p.RecordPrism(votes))
	require.Equal(1, p.Confidence())
	
	require.NoError(p.RecordPrism(votes))
	require.True(p.Finalized())
}

func TestDyadicPhotonRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)
	
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            3,
	}
	
	p := photon.NewPhoton(params)
	choice := ids.ID{0x07}
	require.NoError(p.Add(choice))
	
	// Build confidence
	votes := bag.Of(choice, choice, choice, choice)
	require.NoError(p.RecordVotes(votes))
	require.Equal(1, p.Confidence())
	
	require.NoError(p.RecordVotes(votes))
	require.Equal(2, p.Confidence())
	
	// Record unsuccessful poll
	p.RecordUnsuccessfulPoll()
	require.Equal(0, p.Confidence())
	require.Equal(2, p.PreferenceStrength()) // Preference strength not affected
	
	// Need to rebuild confidence from 0
	require.NoError(p.RecordVotes(votes))
	require.Equal(1, p.Confidence())
	
	require.NoError(p.RecordVotes(votes))
	require.Equal(2, p.Confidence())
	
	require.NoError(p.RecordVotes(votes))
	require.Equal(3, p.Confidence())
	require.True(p.Finalized())
}

func TestDyadicPhotonEmptyVotes(t *testing.T) {
	require := require.New(t)
	
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}
	
	p := photon.NewPhoton(params)
	choice := ids.ID{0x08}
	require.NoError(p.Add(choice))
	
	// Empty votes should not cause error
	emptyVotes := bag.Bag[ids.ID]{}
	require.NoError(p.RecordVotes(emptyVotes))
	require.Equal(0, p.Confidence())
	require.Equal(0, p.PreferenceStrength())
	
	// Vote normally
	votes := bag.Of(choice, choice, choice, choice)
	require.NoError(p.RecordVotes(votes))
	require.Equal(1, p.Confidence())
	
	// Empty votes should reset confidence
	require.NoError(p.RecordVotes(emptyVotes))
	require.Equal(0, p.Confidence())
}

func BenchmarkDyadicPhoton(b *testing.B) {
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}
	
	choice := ids.ID{0x09}
	votes := bag.Of(choice, choice, choice, choice)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		p := photon.NewPhoton(params)
		_ = p.Add(choice)
		
		for !p.Finalized() {
			_ = p.RecordVotes(votes)
		}
	}
}

func BenchmarkDyadicPhotonLargeBeta(b *testing.B) {
	params := config.Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
	}
	
	choice := ids.ID{0x0A}
	votes := make([]ids.ID, 18)
	for i := range votes {
		votes[i] = choice
	}
	voteBag := bag.Of(votes...)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		p := photon.NewPhoton(params)
		_ = p.Add(choice)
		
		for !p.Finalized() {
			_ = p.RecordVotes(voteBag)
		}
	}
}