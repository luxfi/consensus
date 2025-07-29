// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package focus

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func MonadicFocusStateTest(t *testing.T, mf *monadicFocus, expectedPreferenceStrength int, expectedConfidence []int, expectedFinalized bool) {
	require := require.New(t)

	require.Equal(expectedPreferenceStrength, mf.preferenceStrength)
	require.Equal(expectedConfidence, mf.confidence)
	require.Equal(expectedFinalized, mf.Finalized())
}

func TestMonadicFocusLegacy(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	mf := newMonadicFocus(alphaPreference, terminationConditions)

	mf.RecordPoll(alphaPreference)
	MonadicFocusStateTest(t, mf, 1, []int{1}, false)

	mf.RecordPoll(alphaConfidence)
	MonadicFocusStateTest(t, mf, 2, []int{2}, false)

	mf.RecordUnsuccessfulPoll()
	MonadicFocusStateTest(t, mf, 2, []int{0}, false)

	mf.RecordPoll(alphaConfidence)
	MonadicFocusStateTest(t, mf, 3, []int{1}, false)

	mfCloneIntf := mf.Clone()
	require.IsType(&monadicFocus{}, mfCloneIntf)
	mfClone := mfCloneIntf.(*monadicFocus)

	MonadicFocusStateTest(t, mfClone, 3, []int{1}, false)

	dyadicFocus := mfClone.Extend(0)

	expected := "DyadicFocus(Preference = 0, PreferenceStrength[0] = 3, PreferenceStrength[1] = 0"
	require.Contains(dyadicFocus.String(), expected)

	dyadicFocus.RecordUnsuccessfulPoll()
	for i := 0; i < 5; i++ {
		require.Zero(dyadicFocus.Preference())
		require.False(dyadicFocus.Finalized())
		dyadicFocus.RecordUnsuccessfulPoll()
	}

	require.Equal(0, dyadicFocus.Preference())
	require.False(dyadicFocus.Finalized())

	dyadicFocus.RecordPoll(alphaConfidence, 1)
	require.Equal(0, dyadicFocus.Preference()) // Still 0 because preferenceStrength is equal
	require.False(dyadicFocus.Finalized())

	dyadicFocus.RecordPoll(alphaConfidence, 1)
	require.Equal(1, dyadicFocus.Preference())
	require.True(dyadicFocus.Finalized())

	expected = "MonadicFocus(PreferenceStrength = 3, Confidence = [1], Finalized = false)"
	require.Equal(expected, mf.String())
}
