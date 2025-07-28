// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func UnaryThresholdStateTest(t *testing.T, ut *unaryThreshold, expectedConfidence []int, expectedFinalized bool) {
	require := require.New(t)

	require.Equal(expectedConfidence, ut.confidence)
	require.Equal(expectedFinalized, ut.Finalized())
}

func TestUnaryThreshold(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	ut := newUnaryThreshold(alphaPreference, terminationConditions)

	ut.RecordPoll(alphaConfidence)
	UnaryThresholdStateTest(t, &ut, []int{1}, false)

	ut.RecordPoll(alphaPreference)
	UnaryThresholdStateTest(t, &ut, []int{0}, false)

	ut.RecordPoll(alphaConfidence)
	UnaryThresholdStateTest(t, &ut, []int{1}, false)

	ut.RecordUnsuccessfulPoll()
	UnaryThresholdStateTest(t, &ut, []int{0}, false)

	ut.RecordPoll(alphaConfidence)
	UnaryThresholdStateTest(t, &ut, []int{1}, false)

	utClone := ut.Clone()
	UnaryThresholdStateTest(t, &utClone, []int{1}, false)

	binaryThreshold := utClone.Extend(0)

	expected := "UT(Confidence = [1], Finalized = false)"
	require.Equal(expected, utClone.String())

	binaryThreshold.RecordUnsuccessfulPoll()
	for i := 0; i < 5; i++ {
		require.Zero(binaryThreshold.Preference())
		require.False(binaryThreshold.Finalized())
		binaryThreshold.RecordPoll(alphaConfidence, 1)
		binaryThreshold.RecordUnsuccessfulPoll()
	}

	require.Equal(1, binaryThreshold.Preference())
	require.False(binaryThreshold.Finalized())

	binaryThreshold.RecordPoll(alphaConfidence, 1)
	require.Equal(1, binaryThreshold.Preference())
	require.False(binaryThreshold.Finalized())

	binaryThreshold.RecordPoll(alphaConfidence, 1)
	require.Equal(1, binaryThreshold.Preference())
	require.True(binaryThreshold.Finalized())

	expected = "UT(Confidence = [1], Finalized = false)"
	require.Equal(expected, ut.String())
}