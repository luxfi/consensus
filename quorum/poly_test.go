// Copyright (C) 2019-2024, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/ids"
)

var (
	Red   = ids.GenerateTestID()
	Blue  = ids.GenerateTestID()
	Green = ids.GenerateTestID()
)

// Poly interface for polymorphic consensus
type Poly interface {
	Add(id ids.ID)
	RecordPoll(count int, choice ids.ID)
	Preference() ids.ID
	Finalized() bool
}

// polyImpl implements Poly interface
type polyImpl struct {
	alphaPreference int
	terminationConditions []terminationCondition
	choice ids.ID
	finalized bool
	preference ids.ID
	successful int
}

// NewPoly creates a new Poly instance
func NewPoly(alphaPreference int, terminationConditions []terminationCondition, choice ids.ID) Poly {
	return &polyImpl{
		alphaPreference: alphaPreference,
		terminationConditions: terminationConditions,
		choice: choice,
		preference: choice,
	}
}

func (p *polyImpl) Add(id ids.ID) {
	// Add implementation
}

func (p *polyImpl) RecordPoll(count int, choice ids.ID) {
	if count >= p.alphaPreference {
		p.preference = choice
		if count >= p.terminationConditions[0].alphaConfidence {
			p.successful++
			if p.successful >= p.terminationConditions[0].beta {
				p.finalized = true
			}
		}
	}
}

func (p *polyImpl) Preference() ids.ID {
	return p.preference
}

func (p *polyImpl) Finalized() bool {
	return p.finalized
}

func newPolySnowball(alphaPreference int, terminationConditions []terminationCondition, choice ids.ID) Poly {
	return NewPoly(alphaPreference, terminationConditions, choice)
}

func TestPolySnowball(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	sb := newPolySnowball(alphaPreference, terminationConditions, Red)
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

func TestVirtuousPolySnowball(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 1
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	sb := newPolySnowball(alphaPreference, terminationConditions, Red)

	require.Equal(Red, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, Red)
	require.Equal(Red, sb.Preference())
	require.True(sb.Finalized())
}

func TestPolySnowballRecordUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	sb := newPolySnowball(alphaPreference, terminationConditions, Red)
	sb.Add(Blue)

	require.Equal(Red, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, Blue)
	require.Equal(Blue, sb.Preference())
	require.False(sb.Finalized())

	// sb.RecordUnsuccessfulPoll() // TODO: implement this method

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

func TestPolySnowballDifferentSnowflakeColor(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 2
	terminationConditions := newSingleTerminationCondition(alphaConfidence, beta)

	sb := newPolySnowball(alphaPreference, terminationConditions, Red)
	sb.Add(Blue)

	require.Equal(Red, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, Blue)

	require.Equal(Blue, sb.Preference())

	sb.RecordPoll(alphaConfidence, Red)

	require.Equal(Blue, sb.Preference())
	require.Equal(Red, sb.Preference())
}