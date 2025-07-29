// Copyright (C) 2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/ids"
)

var (
	Red   = ids.ID{0x3d, 0xa, 0xd1, 0x2b, 0x8e, 0xe8, 0x92, 0x8e, 0xdf, 0x24, 0x8c, 0xa9, 0x1c, 0xa5, 0x56, 0x0, 0xfb, 0x38, 0x3f, 0x7, 0xc3, 0x2b, 0xff, 0x1d, 0x6d, 0xec, 0x47, 0x2b, 0x25, 0xcf, 0x59, 0xa7}
	Blue  = ids.ID{0xe9, 0x2, 0xa9, 0xa8, 0x66, 0x40, 0xbf, 0xdb, 0x1c, 0xd0, 0xe3, 0x6c, 0xc, 0xc9, 0x82, 0xb8, 0x3e, 0x57, 0x65, 0xfa, 0xd5, 0xf6, 0xbb, 0xe6, 0xab, 0xdc, 0xce, 0x7b, 0x5a, 0xe7, 0xd7, 0xc7}
	Green = ids.GenerateTestID()
)

func newPolyFocus(alphaPreference int, terminationConditions []terminationCondition, choice ids.ID) *polyThreshold {
	pt := newPolyThreshold(alphaPreference, terminationConditions, choice)
	return &pt
}

func TestPolyFocus(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	sb := newPolyFocus(alphaPreference, terminationConditions, Red)
	sb.Add(Blue)
	sb.Add(Green)

	require.Equal(Red, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, Blue)
	require.Equal(Blue, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, Red)
	require.Equal(Blue, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaPreference, Red)
	require.Equal(Red, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, Red)
	require.Equal(Red, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaPreference, Blue)
	require.Equal(Red, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, Blue)
	require.Equal(Red, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, Blue)
	require.Equal(Blue, sb.Preference())
	require.True(sb.Finalized())
}

func TestVirtuousPolyFocus(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 1
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	sb := newPolyFocus(alphaPreference, terminationConditions, Red)

	require.Equal(Red, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, Red)
	require.Equal(Red, sb.Preference())
	require.True(sb.Finalized())
}

func TestPolyFocusRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	sb := newPolyFocus(alphaPreference, terminationConditions, Red)
	sb.Add(Blue)

	require.Equal(Red, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, Blue)
	require.Equal(Blue, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordUnsuccessfulPoll()

	sb.RecordPoll(alphaConfidence, Blue)

	require.Equal(Blue, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, Blue)

	require.Equal(Blue, sb.Preference())
	require.True(sb.Finalized())

	for i := 0; i < 4; i++ {
		sb.RecordPoll(alphaConfidence, Red)

		require.Equal(Blue, sb.Preference())
		require.True(sb.Finalized())
	}
}

func TestPolyFocusDifferentWaveColor(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	sb := newPolyFocus(alphaPreference, terminationConditions, Red)
	sb.Add(Blue)

	require.Equal(Red, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, Blue)

	require.Equal(Blue, sb.Preference())

	sb.RecordPoll(alphaConfidence, Red)

	require.Equal(Red, sb.Preference())
}