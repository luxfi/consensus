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

// TestWaveMultiChoice tests wave multi-choice consensus with multiple choices
func TestWaveMultiChoice(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := NewWave(params)

	// Add many choices
	numColors := 10
	colors := make([]ids.ID, numColors)
	for i := 0; i < numColors; i++ {
		colors[i] = ids.GenerateTestID()
		require.NoError(w.Add(colors[i]))
	}

	// Initially should prefer first
	require.Equal(colors[0], w.Preference())

	// Vote for middle color
	targetColor := colors[5]
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votes.Add(targetColor)
	}

	// Should converge on target
	for i := 0; i < int(params.Beta); i++ {
		require.NoError(w.RecordVotes(votes))
	}

	require.True(w.Finalized())
	require.Equal(targetColor, w.Preference())
}

// TestWaveMultiChoiceRecordUnsuccessfulPoll tests wave multi-choice with unsuccessful polls
func TestWaveMultiChoiceRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := NewWave(params)

	// Add 5 choices
	colors := make([]ids.ID, 5)
	for i := 0; i < 5; i++ {
		colors[i] = ids.GenerateTestID()
		require.NoError(w.Add(colors[i]))
	}

	// Vote for color 2 successfully
	goodVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		goodVotes.Add(colors[2])
	}

	require.NoError(w.RecordVotes(goodVotes))
	require.Equal(colors[2], w.Preference())
	require.Equal(1, w.confidence)

	// Weak poll should reset confidence
	weakVotes := bag.Bag[ids.ID]{}
	weakVotes.Add(colors[2])

	require.NoError(w.RecordVotes(weakVotes))
	require.Equal(0, w.confidence)
	require.False(w.Finalized())
}

// TestWaveMultiChoiceBinary tests wave multi-choice with only 2 choices
func TestWaveMultiChoiceBinary(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))

	// Should behave like binary
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		blueVotes.Add(Blue)
	}

	for i := 0; i < int(params.Beta); i++ {
		require.NoError(w.RecordVotes(blueVotes))
	}

	require.True(w.Finalized())
	require.Equal(Blue, w.Preference())
}

// TestWaveMultiChoiceUnary tests wave multi-choice with only 1 choice
func TestWaveMultiChoiceUnary(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := NewWave(params)
	require.NoError(w.Add(Red))

	// Should behave like unary
	redVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		redVotes.Add(Red)
	}

	for i := 0; i < int(params.Beta); i++ {
		require.NoError(w.RecordVotes(redVotes))
	}

	require.True(w.Finalized())
	require.Equal(Red, w.Preference())
}

// TestWaveMultiChoiceRecordPollPreference tests preference changes in wave multi-choice
func TestWaveMultiChoiceRecordPollPreference(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := NewWave(params)

	// Add 7 colors
	colors := make([]ids.ID, 7)
	for i := 0; i < 7; i++ {
		colors[i] = ids.GenerateTestID()
		require.NoError(w.Add(colors[i]))
	}

	// Vote for different colors in sequence
	for i, color := range colors {
		votes := bag.Bag[ids.ID]{}
		for j := 0; j < params.AlphaPreference; j++ {
			votes.Add(color)
		}

		require.NoError(w.RecordVotes(votes))
		require.Equal(color, w.Preference())

		// Only last few should build confidence
		if i >= 4 {
			// Continue with same color to build confidence
			for j := 1; j < int(params.Beta); j++ {
				require.NoError(w.RecordVotes(votes))
			}
			require.True(w.Finalized())
			require.Equal(color, w.Preference())
			break
		}
	}
}

// TestWaveMultiChoiceDynamicChoices tests adding choices dynamically
func TestWaveMultiChoiceDynamicChoices(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := NewWave(params)

	// Start with 3 choices
	colors := make([]ids.ID, 10)
	for i := 0; i < 3; i++ {
		colors[i] = ids.GenerateTestID()
		require.NoError(w.Add(colors[i]))
	}

	// Vote for first color
	votes1 := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		votes1.Add(colors[0])
	}
	require.NoError(w.RecordVotes(votes1))

	// Add more choices
	for i := 3; i < 7; i++ {
		colors[i] = ids.GenerateTestID()
		require.NoError(w.Add(colors[i]))
	}

	// Vote for new color
	votes2 := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votes2.Add(colors[5])
	}

	for i := 0; i < int(params.Beta); i++ {
		require.NoError(w.RecordVotes(votes2))
	}

	require.True(w.Finalized())
	require.Equal(colors[5], w.Preference())
}

// TestWaveMultiChoiceConvergence tests convergence with many participants
func TestWaveMultiChoiceConvergence(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters

	// Create multiple nodes
	numNodes := 20
	nodes := make([]*Wave, numNodes)

	// Common choices
	numColors := 5
	colors := make([]ids.ID, numColors)
	for i := 0; i < numColors; i++ {
		colors[i] = ids.GenerateTestID()
	}

	// Initialize all nodes
	for i := 0; i < numNodes; i++ {
		nodes[i] = NewWave(params)
		for _, color := range colors {
			require.NoError(nodes[i].Add(color))
		}
	}

	// All vote for same color
	targetColor := colors[2]
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votes.Add(targetColor)
	}

	// Run until all converge
	allFinalized := false
	rounds := 0
	for !allFinalized && rounds < 100 {
		allFinalized = true
		for _, node := range nodes {
			if !node.Finalized() {
				require.NoError(node.RecordVotes(votes))
				allFinalized = false
			}
		}
		rounds++
	}

	// All should agree
	require.True(allFinalized)
	for _, node := range nodes {
		require.True(node.Finalized())
		require.Equal(targetColor, node.Preference())
	}
}

// TestWaveMultiChoiceSplitVotes tests wave multi-choice with split votes
func TestWaveMultiChoiceSplitVotes(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	w := NewWave(params)

	// Add 4 choices
	colors := make([]ids.ID, 4)
	for i := 0; i < 4; i++ {
		colors[i] = ids.GenerateTestID()
		require.NoError(w.Add(colors[i]))
	}

	// Split votes evenly - should not progress
	splitVotes := bag.Bag[ids.ID]{}
	votesPerColor := params.K / 4
	for i := 0; i < 4; i++ {
		for j := 0; j < votesPerColor; j++ {
			splitVotes.Add(colors[i])
		}
	}

	initialPref := w.Preference()
	require.NoError(w.RecordVotes(splitVotes))
	require.Equal(initialPref, w.Preference())
	require.Equal(0, w.confidence)
	require.False(w.Finalized())

	// Now give one color majority
	majorityVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		majorityVotes.Add(colors[1])
	}

	for i := 0; i < int(params.Beta); i++ {
		require.NoError(w.RecordVotes(majorityVotes))
	}

	require.True(w.Finalized())
	require.Equal(colors[1], w.Preference())
}
