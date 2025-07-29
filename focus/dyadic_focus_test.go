// Copyright (C) 2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package focus

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDyadicFocusLegacy(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 2, 3
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	df := newDyadicFocus(alphaPreference, terminationConditions, red)
	require.Equal(red, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, red)
	require.Equal(blue, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, df.Preference())
	require.True(df.Finalized())
}

func TestBinaryFocusRecordPollPreference(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	df := newDyadicFocus(alphaPreference, terminationConditions, red)
	require.Equal(red, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, red)
	require.Equal(blue, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaPreference, red)
	require.Equal(red, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, red)
	require.Equal(red, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, red)
	require.Equal(red, df.Preference())
	require.True(df.Finalized())

	expected := "DyadicFocus(Preference = 0, PreferenceStrength[0] = 4, PreferenceStrength[1] = 1, DyadicWave(Confidence = [2], Finalized = true, DyadicPhoton(Preference = 0)))"
	require.Equal(expected, df.String())
}

func TestBinaryFocusRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	df := newDyadicFocus(alphaPreference, terminationConditions, red)
	require.Equal(red, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, df.Preference())
	require.False(df.Finalized())

	df.RecordUnsuccessfulPoll()

	df.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, df.Preference())
	require.True(df.Finalized())

	expected := "DyadicFocus(Preference = 1, PreferenceStrength[0] = 0, PreferenceStrength[1] = 3, DyadicWave(Confidence = [2], Finalized = true, DyadicPhoton(Preference = 1)))"
	require.Equal(expected, df.String())
}

func TestBinaryFocusAcceptWeirdColor(t *testing.T) {
	require := require.New(t)

	blue := 0
	red := 1

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	df := newDyadicFocus(alphaPreference, terminationConditions, red)

	require.Equal(red, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, red)
	df.RecordUnsuccessfulPoll()

	require.Equal(red, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, red)

	df.RecordUnsuccessfulPoll()

	require.Equal(red, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, blue)

	require.Equal(red, df.Preference())
	require.False(df.Finalized())

	df.RecordPoll(alphaConfidence, blue)

	require.Equal(blue, df.Preference())
	require.True(df.Finalized())

	expected := "DyadicFocus(Preference = 1, PreferenceStrength[0] = 2, PreferenceStrength[1] = 2, DyadicWave(Confidence = [2], Finalized = true, DyadicPhoton(Preference = 0)))"
	require.Equal(expected, df.String())
}

func TestBinaryFocusLockColor(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 1, 2
	beta := 1
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	df := newDyadicFocus(alphaPreference, terminationConditions, red)

	df.RecordPoll(alphaConfidence, red)

	require.Equal(red, df.Preference())
	require.True(df.Finalized())

	df.RecordPoll(alphaConfidence, blue)

	require.Equal(red, df.Preference())
	require.True(df.Finalized())

	df.RecordPoll(alphaPreference, blue)
	df.RecordPoll(alphaConfidence, blue)

	require.Equal(red, df.Preference())
	require.True(df.Finalized())

	expected := "DyadicFocus(Preference = 1, PreferenceStrength[0] = 1, PreferenceStrength[1] = 3, DyadicWave(Confidence = [1], Finalized = true, DyadicPhoton(Preference = 0)))"
	require.Equal(expected, df.String())
}
