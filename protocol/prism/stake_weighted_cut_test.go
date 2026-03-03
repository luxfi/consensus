// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"testing"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestStakeWeightedCutNoValidators(t *testing.T) {
	_, err := NewStakeWeightedCut(nil)
	require.ErrorIs(t, err, ErrNoValidators)

	_, err = NewStakeWeightedCut([]Validator{})
	require.ErrorIs(t, err, ErrNoValidators)
}

func TestStakeWeightedCutZeroWeightFiltered(t *testing.T) {
	_, err := NewStakeWeightedCut([]Validator{
		{ID: ids.GenerateTestNodeID(), Weight: 0},
		{ID: ids.GenerateTestNodeID(), Weight: 0},
	})
	require.ErrorIs(t, err, ErrNoValidators)
}

func TestStakeWeightedCutSampleAll(t *testing.T) {
	require := require.New(t)

	v1 := Validator{ID: ids.GenerateTestNodeID(), Weight: 100}
	v2 := Validator{ID: ids.GenerateTestNodeID(), Weight: 200}
	cut, err := NewStakeWeightedCut([]Validator{v1, v2})
	require.NoError(err)

	// k >= n returns all validators.
	nodes := cut.Sample(5)
	require.Len(nodes, 2)

	nodes = cut.Sample(2)
	require.Len(nodes, 2)
}

func TestStakeWeightedCutSampleKZero(t *testing.T) {
	v1 := Validator{ID: ids.GenerateTestNodeID(), Weight: 100}
	cut, err := NewStakeWeightedCut([]Validator{v1})
	require.NoError(t, err)

	nodes := cut.Sample(0)
	require.Empty(t, nodes)

	nodes = cut.Sample(-1)
	require.Empty(t, nodes)
}

func TestStakeWeightedCutSampleNoDuplicates(t *testing.T) {
	require := require.New(t)

	validators := make([]Validator, 10)
	for i := range validators {
		validators[i] = Validator{
			ID:     ids.GenerateTestNodeID(),
			Weight: uint64(i + 1),
		}
	}
	cut, err := NewStakeWeightedCut(validators)
	require.NoError(err)

	// Sample 5 from 10 -- must have no duplicates.
	for trial := 0; trial < 100; trial++ {
		nodes := cut.Sample(5)
		require.Len(nodes, 5)

		seen := make(map[ids.NodeID]bool)
		for _, n := range nodes {
			require.False(seen[n], "duplicate node in sample")
			seen[n] = true
		}
	}
}

func TestStakeWeightedCutStakeDistribution(t *testing.T) {
	require := require.New(t)

	// One node has 99% weight, others 1% each.
	heavy := Validator{ID: ids.GenerateTestNodeID(), Weight: 9900}
	validators := []Validator{heavy}
	for i := 0; i < 99; i++ {
		validators = append(validators, Validator{
			ID:     ids.GenerateTestNodeID(),
			Weight: 1,
		})
	}
	cut, err := NewStakeWeightedCut(validators)
	require.NoError(err)

	// Over many samples of k=1, the heavy node should appear most of the time.
	heavyCount := 0
	trials := 10000
	for i := 0; i < trials; i++ {
		nodes := cut.Sample(1)
		require.Len(nodes, 1)
		if nodes[0] == heavy.ID {
			heavyCount++
		}
	}

	// Heavy node has ~99% weight. Allow wide tolerance: should appear >90% of the time.
	ratio := float64(heavyCount) / float64(trials)
	require.Greater(ratio, 0.90, "heavy node should be selected proportional to weight, got ratio=%f", ratio)
}

func TestStakeWeightedCutLuminance(t *testing.T) {
	require := require.New(t)

	// Small network.
	validators := make([]Validator, 5)
	for i := range validators {
		validators[i] = Validator{ID: ids.GenerateTestNodeID(), Weight: 10}
	}
	cut, err := NewStakeWeightedCut(validators)
	require.NoError(err)

	lum := cut.Luminance()
	require.Equal(5, lum.ActivePeers)
	require.Equal(5, lum.TotalPeers)
	require.Equal(float64(5), lum.Lx)

	// Medium network.
	validators = make([]Validator, 25)
	for i := range validators {
		validators[i] = Validator{ID: ids.GenerateTestNodeID(), Weight: 10}
	}
	cut, err = NewStakeWeightedCut(validators)
	require.NoError(err)
	lum = cut.Luminance()
	require.Equal(300.0, lum.Lx)

	// Large network.
	validators = make([]Validator, 100)
	for i := range validators {
		validators[i] = Validator{ID: ids.GenerateTestNodeID(), Weight: 10}
	}
	cut, err = NewStakeWeightedCut(validators)
	require.NoError(err)
	lum = cut.Luminance()
	require.Equal(500.0, lum.Lx)
}
