// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave_test

import (
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestBinaryWave(t *testing.T) {
	require := require.New(t)

	// Test binary (dyadic) wave consensus
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	w := wave.NewWave(params)

	// Add binary choices
	choice0 := ids.ID{0x01}
	choice1 := ids.ID{0x02}

	require.NoError(w.Add(choice0))
	require.NoError(w.Add(choice1))

	// Initial preference is first added
	require.Equal(choice0, w.Preference())
	require.False(w.Finalized())

	// Vote for choice1 with confidence
	votes := bag.Of(choice1, choice1, choice1, choice1)
	require.NoError(w.RecordVotes(votes))

	// Should switch preference to choice1 (it had the max votes)
	require.Equal(choice1, w.Preference())
	require.False(w.Finalized())

	// Vote again to reach beta
	require.NoError(w.RecordVotes(votes))

	// Should be finalized on choice1
	require.Equal(choice1, w.Preference())
	require.True(w.Finalized())
}

func TestBinaryWaveConfidence(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               7,
		AlphaPreference: 4,
		AlphaConfidence: 6,
		Beta:            3,
	}

	w := wave.NewWave(params)

	choice0 := ids.ID{0x01}
	choice1 := ids.ID{0x02}

	require.NoError(w.Add(choice0))
	require.NoError(w.Add(choice1))

	// Vote below preference threshold for choice1
	weakVotes := bag.Of(choice1, choice1, choice1)
	require.NoError(w.RecordVotes(weakVotes))

	// Should not switch preference
	require.Equal(choice0, w.Preference())
	require.False(w.Finalized())

	// Vote at preference but below confidence for choice1
	prefVotes := bag.Of(choice1, choice1, choice1, choice1)
	require.NoError(w.RecordVotes(prefVotes))

	// Should switch preference but not gain confidence
	require.Equal(choice1, w.Preference())
	require.False(w.Finalized())

	// Vote at confidence threshold
	confVotes := bag.Of(choice1, choice1, choice1, choice1, choice1, choice1)
	require.NoError(w.RecordVotes(confVotes))

	// Should have confidence but not finalized yet
	require.Equal(choice1, w.Preference())
	require.False(w.Finalized())

	// Continue voting with confidence
	require.NoError(w.RecordVotes(confVotes))
	require.NoError(w.RecordVotes(confVotes))

	// Should be finalized after beta rounds
	require.True(w.Finalized())
	require.Equal(choice1, w.Preference())
}

func TestBinaryWaveFinalization(t *testing.T) {
	require := require.New(t)

	// Test with low beta for quick finalization
	params := config.Parameters{
		K:               3,
		AlphaPreference: 2,
		AlphaConfidence: 3,
		Beta:            1,
	}

	w := wave.NewWave(params)

	choice0 := ids.ID{0x01}
	choice1 := ids.ID{0x02}

	require.NoError(w.Add(choice0))
	require.NoError(w.Add(choice1))

	// Single vote with confidence should finalize (beta=1)
	votes := bag.Of(choice0, choice0, choice0)
	require.NoError(w.RecordVotes(votes))

	require.True(w.Finalized())
	require.Equal(choice0, w.Preference())

	// Additional votes after finalization should not change state
	votesOther := bag.Of(choice1, choice1, choice1)
	require.NoError(w.RecordVotes(votesOther))

	// Should still be finalized on choice0
	require.True(w.Finalized())
	require.Equal(choice0, w.Preference())
}

func TestBinaryWavePreferenceSwitch(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	w := wave.NewWave(params)

	choice0 := ids.ID{0x01}
	choice1 := ids.ID{0x02}

	require.NoError(w.Add(choice0))
	require.NoError(w.Add(choice1))

	// Start with preference for choice0
	require.Equal(choice0, w.Preference())

	// Vote for choice0 with preference
	votes0 := bag.Of(choice0, choice0, choice0)
	require.NoError(w.RecordVotes(votes0))

	// Should still prefer choice0
	require.Equal(choice0, w.Preference())

	// Vote for choice1 with confidence
	votes1 := bag.Of(choice1, choice1, choice1, choice1)
	require.NoError(w.RecordVotes(votes1))

	// Should switch to choice1
	require.Equal(choice1, w.Preference())
	require.False(w.Finalized())

	// Vote for choice0 with preference again
	require.NoError(w.RecordVotes(votes0))

	// Should switch back to choice0 (but lose confidence)
	require.Equal(choice0, w.Preference())
	require.False(w.Finalized())

	// Vote for choice0 with confidence to finalize
	votes0Strong := bag.Of(choice0, choice0, choice0, choice0)
	require.NoError(w.RecordVotes(votes0Strong))
	require.NoError(w.RecordVotes(votes0Strong))

	require.True(w.Finalized())
	require.Equal(choice0, w.Preference())
}

func TestBinaryWaveRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	w := wave.NewWave(params)

	choice0 := ids.ID{0x01}
	choice1 := ids.ID{0x02}

	require.NoError(w.Add(choice0))
	require.NoError(w.Add(choice1))

	// Vote with confidence for choice0
	votes := bag.Of(choice0, choice0, choice0, choice0)
	require.NoError(w.RecordVotes(votes))

	// Should have progress but not finalized
	require.Equal(choice0, w.Preference())
	require.False(w.Finalized())

	// Record unsuccessful poll
	w.RecordUnsuccessfulPoll()

	// Preference should remain but need to rebuild confidence
	require.Equal(choice0, w.Preference())
	require.False(w.Finalized())

	// Vote again to rebuild and finalize
	require.NoError(w.RecordVotes(votes))
	require.NoError(w.RecordVotes(votes))

	require.True(w.Finalized())
	require.Equal(choice0, w.Preference())
}

func TestBinaryWaveEqualVotes(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               6,
		AlphaPreference: 4,
		AlphaConfidence: 5,
		Beta:            2,
	}

	w := wave.NewWave(params)

	choice0 := ids.ID{0x01}
	choice1 := ids.ID{0x02}

	require.NoError(w.Add(choice0))
	require.NoError(w.Add(choice1))

	// Vote equally for both (3 votes each)
	votes := bag.Of(choice0, choice0, choice0, choice1, choice1, choice1)
	require.NoError(w.RecordVotes(votes))

	// Neither should meet alpha preference, preference unchanged
	require.Equal(choice0, w.Preference())
	require.False(w.Finalized())

	// Vote strongly for choice1
	votes1 := bag.Of(choice1, choice1, choice1, choice1, choice1)
	require.NoError(w.RecordVotes(votes1))

	// Should switch to choice1
	require.Equal(choice1, w.Preference())
	require.False(w.Finalized())

	// Continue to finalize
	require.NoError(w.RecordVotes(votes1))

	require.True(w.Finalized())
	require.Equal(choice1, w.Preference())
}

func BenchmarkBinaryWave(b *testing.B) {
	params := config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	choice0 := ids.ID{0x01}
	choice1 := ids.ID{0x02}
	votes := bag.Of(choice1, choice1, choice1, choice1)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := wave.NewWave(params)
		_ = w.Add(choice0)
		_ = w.Add(choice1)

		for !w.Finalized() && w.NumPolls() < 10 {
			_ = w.RecordVotes(votes)
		}
	}
}

func BenchmarkBinaryWaveLargeBeta(b *testing.B) {
	params := config.Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
	}

	choice0 := ids.ID{0x01}
	choice1 := ids.ID{0x02}

	votes := make([]ids.ID, 18)
	for i := range votes {
		votes[i] = choice1
	}
	voteBag := bag.Of(votes...)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := wave.NewWave(params)
		_ = w.Add(choice0)
		_ = w.Add(choice1)

		for !w.Finalized() && w.NumPolls() < 20 {
			_ = w.RecordVotes(voteBag)
		}
	}
}
