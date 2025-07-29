// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/photon"
	"github.com/luxfi/consensus/utils"
	"github.com/luxfi/consensus/utils/bag"
)

// Helper function to convert bag.Bag[ids.ID] to *utils.Bag
func convertBag(b bag.Bag[ids.ID]) *utils.Bag {
	utilsBag := utils.NewBag()
	for _, id := range b.List() {
		utilsBag.AddCount(id, b.Count(id))
	}
	return utilsBag
}

func TestFlat(t *testing.T) {
	require := require.New(t)

	params := photon.Parameters{
		K:               3,
		AlphaPreference: 2,
		AlphaConfidence: 3,
		Beta:            2,
	}
	
	red := ids.ID{0}
	green := ids.ID{1}
	blue := ids.ID{2}
	
	f := NewFlat(photon.PhotonFactory, params, red)
	f.Add(green)
	f.Add(blue)

	require.Equal(red, f.Preference())
	require.False(f.Finalized())

	// Three votes for blue
	threeBlue := bag.Of(blue, blue, blue)
	require.True(f.RecordPoll(convertBag(threeBlue)))
	require.Equal(blue, f.Preference())
	require.False(f.Finalized())

	// Two votes for green (below alpha confidence)
	twoGreen := bag.Of(green, green)
	require.True(f.RecordPoll(convertBag(twoGreen)))
	require.Equal(blue, f.Preference())
	require.False(f.Finalized())

	// Three votes for green
	threeGreen := bag.Of(green, green, green)
	require.True(f.RecordPoll(convertBag(threeGreen)))
	require.Equal(green, f.Preference())
	require.False(f.Finalized())

	// Reset the confidence from previous round
	oneEach := bag.Of(red, green, blue)
	require.False(f.RecordPoll(convertBag(oneEach)))
	require.Equal(green, f.Preference())
	require.False(f.Finalized())

	// First successful poll for green
	require.True(f.RecordPoll(convertBag(threeGreen)))
	require.Equal(green, f.Preference())
	require.False(f.Finalized()) // Not finalized before Beta rounds

	// Second successful poll - should finalize
	require.True(f.RecordPoll(convertBag(threeGreen)))
	require.Equal(green, f.Preference())
	require.True(f.Finalized())
}

// TestFlatQuickFinalization tests quick finalization with high confidence
func TestFlatQuickFinalization(t *testing.T) {
	require := require.New(t)

	params := photon.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            1,
	}
	
	red := ids.ID{0}
	green := ids.ID{1}
	
	f := NewFlat(photon.PhotonFactory, params, red)
	f.Add(green)

	// Four votes for green (meets alpha confidence)
	fourGreen := bag.Of(green, green, green, green)
	require.True(f.RecordPoll(convertBag(fourGreen)))
	require.Equal(green, f.Preference())
	require.True(f.Finalized()) // Beta = 1, so finalized immediately
}

// TestFlatNoFinalization tests that low confidence doesn't finalize
func TestFlatNoFinalization(t *testing.T) {
	require := require.New(t)

	params := photon.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            3,
	}
	
	red := ids.ID{0}
	green := ids.ID{1}
	blue := ids.ID{2}
	
	f := NewFlat(photon.PhotonFactory, params, red)
	f.Add(green)
	f.Add(blue)

	// Only meet alpha preference, not confidence
	for i := 0; i < 10; i++ {
		threeGreen := bag.Of(green, green, green)
		require.True(f.RecordPoll(convertBag(threeGreen)))
		require.Equal(green, f.Preference())
		require.False(f.Finalized()) // Never reaches confidence threshold
	}
}