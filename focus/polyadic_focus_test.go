// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package focus

import (
	"testing"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

var (
	Red   = ids.ID{0x01}
	Blue  = ids.ID{0x02}
	Green = ids.ID{0x03}
)

func TestPolyadicFocusLegacy(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	pf := newPolyadicFocus(alphaPreference, terminationConditions, Red)
	pf.Add(Blue)
	pf.Add(Green)

	require.Equal(Red, pf.Preference())
	require.False(pf.Finalized())

	pf.RecordPoll(alphaConfidence, Blue)
	require.Equal(Blue, pf.Preference())
	require.False(pf.Finalized())

	pf.RecordPoll(alphaConfidence, Red)
	require.Equal(Blue, pf.Preference())
	require.False(pf.Finalized())

	pf.RecordPoll(alphaPreference, Red)
	require.Equal(Red, pf.Preference())
	require.False(pf.Finalized())

	pf.RecordPoll(alphaConfidence, Red)
	require.Equal(Red, pf.Preference())
	require.False(pf.Finalized())

	pf.RecordPoll(alphaPreference, Blue)
	require.Equal(Red, pf.Preference())
	require.False(pf.Finalized())

	pf.RecordPoll(alphaConfidence, Blue)
	require.Equal(Red, pf.Preference())
	require.False(pf.Finalized())

	pf.RecordPoll(alphaConfidence, Blue)
	require.Equal(Blue, pf.Preference())
	require.True(pf.Finalized())
}

func TestVirtuousPolyadicFocus(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 1
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	pf := newPolyadicFocus(alphaPreference, terminationConditions, Red)

	require.Equal(Red, pf.Preference())
	require.False(pf.Finalized())

	pf.RecordPoll(alphaConfidence, Red)
	require.Equal(Red, pf.Preference())
	require.True(pf.Finalized())
}

func TestPolyadicFocusRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	pf := newPolyadicFocus(alphaPreference, terminationConditions, Red)
	pf.Add(Blue)

	require.Equal(Red, pf.Preference())
	require.False(pf.Finalized())

	pf.RecordPoll(alphaConfidence, Blue)
	require.Equal(Blue, pf.Preference())
	require.False(pf.Finalized())

	pf.RecordUnsuccessfulPoll()

	pf.RecordPoll(alphaConfidence, Blue)

	require.Equal(Blue, pf.Preference())
	require.False(pf.Finalized())

	pf.RecordPoll(alphaConfidence, Blue)

	require.Equal(Blue, pf.Preference())
	require.True(pf.Finalized())

	// Check that it contains the expected parts rather than exact string match
	str := pf.String()
	require.Contains(str, "PolyadicFocus")
	require.Contains(str, "PreferenceStrength = 3")
	require.Contains(str, "Confidence = [2]")
	require.Contains(str, "Finalized = true")

	for i := 0; i < 4; i++ {
		pf.RecordPoll(alphaConfidence, Red)

		require.Equal(Blue, pf.Preference())
		require.True(pf.Finalized())
	}
}

func TestPolyadicFocusDifferentWaveColor(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	pf := newPolyadicFocus(alphaPreference, terminationConditions, Red)
	pf.Add(Blue)

	require.Equal(Red, pf.Preference())
	require.False(pf.Finalized())

	pf.RecordPoll(alphaConfidence, Blue)

	// Blue should be the preference after the poll
	require.Equal(Blue, pf.Preference())

	pf.RecordPoll(alphaConfidence, Red)

	require.Equal(Blue, pf.Preference())
	// After unsuccessful poll, the preference resets
	require.Equal(Blue, pf.Preference())
}
