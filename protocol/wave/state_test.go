// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewState(t *testing.T) {
	require := require.New(t)

	state := NewState[string]("tx1")

	require.Equal("tx1", state.ID)
	require.Equal(0, state.Confidence)
	require.False(state.Finalized)
	require.Empty(state.Parents)
	require.Equal(uint64(0), state.Height)
}

func TestStateIsPreferred(t *testing.T) {
	require := require.New(t)

	state := NewState[string]("tx1")

	// Initially not preferred (confidence = 0)
	require.False(state.IsPreferred())

	// After incrementing confidence, should be preferred
	state.IncrementConfidence()
	require.True(state.IsPreferred())

	// Still preferred with higher confidence
	state.IncrementConfidence()
	require.True(state.IsPreferred())
}

func TestStateIncrementConfidence(t *testing.T) {
	require := require.New(t)

	state := NewState[string]("tx1")
	require.Equal(0, state.Confidence)

	state.IncrementConfidence()
	require.Equal(1, state.Confidence)

	state.IncrementConfidence()
	require.Equal(2, state.Confidence)

	state.IncrementConfidence()
	require.Equal(3, state.Confidence)
}

func TestStateFinalize(t *testing.T) {
	require := require.New(t)

	state := NewState[string]("tx1")
	require.False(state.Finalized)

	state.Finalize()
	require.True(state.Finalized)

	// Finalize is idempotent
	state.Finalize()
	require.True(state.Finalized)
}

func TestStateWithParentsAndHeight(t *testing.T) {
	require := require.New(t)

	state := NewState[string]("tx1")
	state.Parents = []string{"parent1", "parent2"}
	state.Height = 42

	require.Equal([]string{"parent1", "parent2"}, state.Parents)
	require.Equal(uint64(42), state.Height)
}

func TestStateGenericTypes(t *testing.T) {
	require := require.New(t)

	// Test with int type
	intState := NewState[int](123)
	require.Equal(123, intState.ID)
	intState.IncrementConfidence()
	require.True(intState.IsPreferred())
	intState.Finalize()
	require.True(intState.Finalized)

	// Test with custom struct type
	type txID struct {
		hash [32]byte
	}
	txState := NewState[txID](txID{hash: [32]byte{1, 2, 3}})
	require.Equal(txID{hash: [32]byte{1, 2, 3}}, txState.ID)
}
