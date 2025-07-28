// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBinaryThreshold(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 2, 3
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	bt := newBinaryThreshold(alphaPreference, terminationConditions, red)
	require.Equal(red, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaConfidence, red)
	require.Equal(blue, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, bt.Preference())
	require.True(bt.Finalized())
}

func TestBinaryThresholdRecordPollPreference(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	bt := newBinaryThreshold(alphaPreference, terminationConditions, red)
	require.Equal(red, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaPreference, blue)
	require.Equal(blue, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaPreference, red)
	require.Equal(red, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaPreference, red)
	require.Equal(red, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaPreference, blue)
	require.Equal(blue, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaPreference, blue)
	require.Equal(blue, bt.Preference())
	require.False(bt.Finalized())
}

func TestBinaryThresholdRecordPollConfidence(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	bt := newBinaryThreshold(alphaPreference, terminationConditions, red)
	require.Equal(red, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, bt.Preference())
	require.True(bt.Finalized())
}

func TestBinaryThresholdRecordPollConfidenceReset(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	bt := newBinaryThreshold(alphaPreference, terminationConditions, red)
	require.Equal(red, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaConfidence, red)
	require.Equal(red, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaConfidence, red)
	require.Equal(red, bt.Preference())
	require.True(bt.Finalized())
}

func TestBinaryThresholdRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	bt := newBinaryThreshold(alphaPreference, terminationConditions, red)
	require.Equal(red, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordUnsuccessfulPoll()

	bt.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, bt.Preference())
	require.False(bt.Finalized())

	bt.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, bt.Preference())
	require.True(bt.Finalized())
}

func TestBinaryThresholdString(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	bt := newBinaryThreshold(alphaPreference, terminationConditions, red)
	bt.RecordPoll(alphaConfidence, blue)

	actual := bt.String()
	// The string representation includes BinarySampler which we need to check differently
	require.Contains(actual, "BT(Confidence = [1], Finalized = false")
}