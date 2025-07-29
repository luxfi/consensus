// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/bag"
)

func TestPhotonTreeSingleton(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               1,
		AlphaPreference: 1,
		AlphaConfidence: 1,
		Beta:            2,
	}
	
	red := ids.ID{0}
	blue := ids.ID{1}
	
	tree := NewTree(PhotonFactory, params, red)

	require.False(tree.Finalized())

	oneRed := bag.Of(red)
	require.True(tree.RecordPoll(oneRed))
	require.False(tree.Finalized())

	empty := bag.Bag[ids.ID]{}
	require.False(tree.RecordPoll(empty))
	require.False(tree.Finalized())

	require.True(tree.RecordPoll(oneRed))
	require.False(tree.Finalized())

	require.True(tree.RecordPoll(oneRed))
	require.Equal(red, tree.Preference())
	require.True(tree.Finalized())

	tree.Add(blue)

	require.True(tree.Finalized())

	// Because the tree is already finalized, RecordPoll can return either true
	// or false.
	oneBlue := bag.Of(blue)
	tree.RecordPoll(oneBlue)
	require.Equal(red, tree.Preference())
	require.True(tree.Finalized())
}

func TestPhotonTreeRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               1,
		AlphaPreference: 1,
		AlphaConfidence: 1,
		Beta:            3,
	}
	
	red := ids.ID{0}
	
	tree := NewTree(PhotonFactory, params, red)

	require.False(tree.Finalized())

	oneRed := bag.Of(red)
	require.True(tree.RecordPoll(oneRed))

	tree.RecordUnsuccessfulPoll()

	require.True(tree.RecordPoll(oneRed))
	require.False(tree.Finalized())

	require.True(tree.RecordPoll(oneRed))
	require.False(tree.Finalized())

	require.True(tree.RecordPoll(oneRed))
	require.Equal(red, tree.Preference())
	require.True(tree.Finalized())
}

func TestPhotonTreeBinary(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               1,
		AlphaPreference: 1,
		AlphaConfidence: 1,
		Beta:            2,
	}
	
	red := ids.ID{0}
	blue := ids.ID{1}
	
	tree := NewTree(PhotonFactory, params, red)
	tree.Add(blue)

	require.Equal(red, tree.Preference())
	require.False(tree.Finalized())

	oneBlue := bag.Of(blue)
	require.True(tree.RecordPoll(oneBlue))
	require.Equal(blue, tree.Preference())
	require.False(tree.Finalized())

	oneRed := bag.Of(red)
	require.True(tree.RecordPoll(oneRed))
	require.Equal(blue, tree.Preference())
	require.False(tree.Finalized())

	require.True(tree.RecordPoll(oneBlue))
	require.Equal(blue, tree.Preference())
	require.False(tree.Finalized())

	require.True(tree.RecordPoll(oneBlue))
	require.Equal(blue, tree.Preference())
	require.True(tree.Finalized())
}

func TestPhotonTreeTernary(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               1,
		AlphaPreference: 1,
		AlphaConfidence: 1,
		Beta:            2,
	}
	
	red := ids.ID{0}
	blue := ids.ID{1}
	green := ids.ID{2}
	
	tree := NewTree(PhotonFactory, params, red)
	tree.Add(blue)
	tree.Add(green)

	require.Equal(red, tree.Preference())
	require.False(tree.Finalized())

	// Vote for green
	oneGreen := bag.Of(green)
	require.True(tree.RecordPoll(oneGreen))
	require.Equal(green, tree.Preference())
	require.False(tree.Finalized())

	// Vote for blue
	oneBlue := bag.Of(blue)
	require.True(tree.RecordPoll(oneBlue))
	// Preference should still be green due to focus
	require.Equal(green, tree.Preference())
	require.False(tree.Finalized())

	// Vote for green again
	require.True(tree.RecordPoll(oneGreen))
	require.Equal(green, tree.Preference())
	require.False(tree.Finalized())

	// Final vote for green to finalize
	require.True(tree.RecordPoll(oneGreen))
	require.Equal(green, tree.Preference())
	require.True(tree.Finalized())
}

func TestPhotonTreeMultiPreference(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               3,
		AlphaPreference: 2,
		AlphaConfidence: 2,
		Beta:            1,
	}
	
	red := ids.ID{0}
	blue := ids.ID{1}
	green := ids.ID{2}
	yellow := ids.ID{3}
	
	tree := NewTree(PhotonFactory, params, red)
	tree.Add(blue)
	tree.Add(green)
	tree.Add(yellow)

	require.Equal(red, tree.Preference())
	require.False(tree.Finalized())

	// Vote with multiple colors
	multiVote := bag.Of(blue, blue, green)
	require.True(tree.RecordPoll(multiVote))
	require.Equal(blue, tree.Preference())
	require.True(tree.Finalized()) // Beta = 1

	// Additional votes shouldn't change finalized preference
	yellowVote := bag.Of(yellow, yellow, yellow)
	tree.RecordPoll(yellowVote)
	require.Equal(blue, tree.Preference())
	require.True(tree.Finalized())
}