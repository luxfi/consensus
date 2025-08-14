// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package tests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocols/protocol/photon"
	"github.com/luxfi/consensus/protocols/protocol/pulse"
	"github.com/luxfi/consensus/protocols/protocol/wave"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

// TestFlat tests flat consensus without hierarchical dependencies
func TestFlat(t *testing.T) {
	params := config.TestParameters
	
	// Test with each protocol type
	testFlatConsensus(t, photon.NewPhoton(params), params)
	testFlatConsensus(t, pulse.NewPulse(params), params)
	testFlatConsensus(t, wave.NewWave(params), params)
}

func testFlatConsensus(t *testing.T, consensus interface {
	Add(ids.ID) error
	RecordVotes(bag.Bag[ids.ID]) error
	Finalized() bool
	Preference() ids.ID
}, params config.Parameters) {
	require := require.New(t)
	
	// Add choices
	choices := []ids.ID{
		ids.GenerateTestID(),
		ids.GenerateTestID(),
		ids.GenerateTestID(),
	}
	
	// Add all choices (photon will only accept first)
	for _, choice := range choices {
		consensus.Add(choice)
	}
	
	// Vote for first choice
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votes.Add(choices[0])
	}
	
	// Record votes until finalized
	for i := 0; i < params.Beta && !consensus.Finalized(); i++ {
		require.NoError(consensus.RecordVotes(votes))
	}
	
	require.True(consensus.Finalized())
	require.Equal(choices[0], consensus.Preference())
}

// TestFlatWithEqualVotes tests consensus with equal votes
func TestFlatWithEqualVotes(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	// Only test with protocols that support multiple choices
	p := pulse.NewPulse(params)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Blue))
	
	// Create equal votes
	equalVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference/2; i++ {
		equalVotes.Add(Red)
		equalVotes.Add(Blue)
	}
	
	// Should not change preference with equal votes
	initialPref := p.Preference()
	require.NoError(p.RecordVotes(equalVotes))
	require.Equal(initialPref, p.Preference())
	require.False(p.Finalized())
	
	// Now give Blue majority
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		blueVotes.Add(Blue)
	}
	
	for i := 0; i < params.Beta; i++ {
		require.NoError(p.RecordVotes(blueVotes))
	}
	
	require.True(p.Finalized())
	require.Equal(Blue, p.Preference())
}

// TestFlatNoVotes tests consensus with no votes
func TestFlatNoVotes(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	p := pulse.NewPulse(params)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Blue))
	
	// Record empty votes
	emptyVotes := bag.Bag[ids.ID]{}
	
	initialPref := p.Preference()
	require.NoError(p.RecordVotes(emptyVotes))
	require.Equal(initialPref, p.Preference())
	require.False(p.Finalized())
	
	// Should still be able to finalize with proper votes
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

// TestFlatSingleVoter tests consensus with single voter
func TestFlatSingleVoter(t *testing.T) {
	require := require.New(t)

	// Use parameters that allow single voter
	params := config.Parameters{
		K:               1,
		AlphaPreference: 1,
		AlphaConfidence: 1,
		Beta:            1,
	}
	
	p := pulse.NewPulse(params)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Blue))
	
	// Single vote should be enough
	singleVote := bag.Bag[ids.ID]{}
	singleVote.Add(Blue)
	
	require.NoError(p.RecordVotes(singleVote))
	if !p.Finalized() {
		t.Logf("Not finalized after single vote. Preference: %v", p.Preference())
		t.Logf("Pulse state: %v", p.String())
	}
	require.True(p.Finalized())
	require.Equal(Blue, p.Preference())
}

// TestFlatManyChoices tests consensus with many choices
func TestFlatManyChoices(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	w := wave.NewWave(params)
	
	// Add many choices
	choices := make([]ids.ID, 100)
	for i := range choices {
		choices[i] = ids.GenerateTestID()
		require.NoError(w.Add(choices[i]))
	}
	
	// Vote for middle choice
	target := choices[50]
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votes.Add(target)
	}
	
	for i := 0; i < params.Beta; i++ {
		require.NoError(w.RecordVotes(votes))
	}
	
	require.True(w.Finalized())
	require.Equal(target, w.Preference())
}

// TestFlatPreferencePersistence tests preference persistence
func TestFlatPreferencePersistence(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	p := pulse.NewPulse(params)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Blue))
	require.NoError(p.Add(Green))
	
	// Set preference to Blue
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		blueVotes.Add(Blue)
	}
	require.NoError(p.RecordVotes(blueVotes))
	require.Equal(Blue, p.Preference())
	
	// Weak votes for Green shouldn't change preference
	weakGreenVotes := bag.Bag[ids.ID]{}
	weakGreenVotes.Add(Green)
	require.NoError(p.RecordVotes(weakGreenVotes))
	require.Equal(Blue, p.Preference())
	
	// Strong votes for Green should change preference
	strongGreenVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		strongGreenVotes.Add(Green)
	}
	require.NoError(p.RecordVotes(strongGreenVotes))
	require.Equal(Green, p.Preference())
}