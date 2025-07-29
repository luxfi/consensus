// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// binaryWave implements the original Wave algorithm for testing
type binaryWave struct {
	// preference is the choice that is currently preferred
	preference int

	// terminationConditions gives the ascending ordered list of alphaConfidence values
	// required to increment the corresponding confidence counter.
	terminationConditions []TerminationCondition

	// confidence is the number of consecutive successful polls for a given
	// alphaConfidence threshold.
	confidence []int

	// finalized prevents the state from changing after the required number of
	// consecutive polls has been reached
	finalized bool
}

func newBinaryWave(alphaPreference int, terminationConditions []TerminationCondition, choice int) *binaryWave {
	return &binaryWave{
		preference:            choice,
		terminationConditions: terminationConditions,
		confidence:            make([]int, len(terminationConditions)),
	}
}

func (sf *binaryWave) Preference() int {
	return sf.preference
}

func (sf *binaryWave) Finalized() bool {
	return sf.finalized
}

func (sf *binaryWave) RecordPoll(count, choice int) {
	if sf.finalized {
		return // This instance is already decided.
	}

	if choice != sf.preference {
		// If the choice differs from our preference, reset confidence
		sf.preference = choice
		clear(sf.confidence)
	}

	// Build confidence based on the count
	for i, terminationCondition := range sf.terminationConditions {
		if count < terminationCondition.AlphaConfidence {
			// If we didn't reach this threshold, clear remaining confidence
			clear(sf.confidence[i:])
			break
		}

		// Increment confidence for this threshold
		sf.confidence[i]++

		// Check if we've reached finalization
		if sf.confidence[i] >= terminationCondition.Beta {
			sf.finalized = true
		}
	}
}

func (sf *binaryWave) RecordUnsuccessfulPoll() {
	clear(sf.confidence)
}

// Test binary wave
func TestBinaryWave(t *testing.T) {
	require := require.New(t)

	blue := 0
	red := 1

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := []TerminationCondition{{
		AlphaConfidence: alphaConfidence,
		Beta:            beta,
	}}

	sf := newBinaryWave(alphaPreference, terminationConditions, red)

	require.Equal(red, sf.Preference())
	require.False(sf.Finalized())

	sf.RecordPoll(alphaConfidence, blue)

	require.Equal(blue, sf.Preference())
	require.False(sf.Finalized())

	sf.RecordPoll(alphaConfidence, red)

	require.Equal(red, sf.Preference())
	require.False(sf.Finalized())

	sf.RecordPoll(alphaConfidence, blue)

	require.Equal(blue, sf.Preference())
	require.False(sf.Finalized())

	sf.RecordPoll(alphaPreference, red)
	require.Equal(red, sf.Preference())
	require.False(sf.Finalized())

	sf.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, sf.Preference())
	require.False(sf.Finalized())

	sf.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, sf.Preference())
	require.True(sf.Finalized())
}

// Test helper interface
type binaryWaveTest struct {
	require *require.Assertions
	*binaryWave
}

func newBinaryWaveTest(t *testing.T, alphaPreference int, terminationConditions []TerminationCondition) *binaryWaveTest {
	require := require.New(t)

	return &binaryWaveTest{
		require:    require,
		binaryWave: newBinaryWave(alphaPreference, terminationConditions, 0),
	}
}

func (sf *binaryWaveTest) AssertEqual(expectedConfidences []int, expectedFinalized bool, expectedPreference int) {
	sf.require.Equal(expectedPreference, sf.Preference())
	sf.require.Equal(expectedConfidences, sf.confidence)
	sf.require.Equal(expectedFinalized, sf.Finalized())
}
