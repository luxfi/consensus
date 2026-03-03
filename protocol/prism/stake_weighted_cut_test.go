// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"math"
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

// TestStakeWeightedCut1000ValidatorsChiSquared verifies that the sampling
// distribution across 1000 validators statistically matches their stake
// weights using a chi-squared goodness-of-fit test.
func TestStakeWeightedCut1000ValidatorsChiSquared(t *testing.T) {
	require := require.New(t)

	const numValidators = 1000
	validators := make([]Validator, numValidators)
	var totalWeight uint64
	for i := range validators {
		w := uint64(i + 1) // weights 1..1000
		validators[i] = Validator{
			ID:     ids.GenerateTestNodeID(),
			Weight: w,
		}
		totalWeight += w
	}

	cut, err := NewStakeWeightedCut(validators)
	require.NoError(err)

	// Run many single-sample trials.
	const trials = 50000
	counts := make(map[ids.NodeID]int, numValidators)
	for i := 0; i < trials; i++ {
		nodes := cut.Sample(1)
		require.Len(nodes, 1)
		counts[nodes[0]]++
	}

	// Chi-squared: sum of (observed - expected)^2 / expected
	var chiSq float64
	for _, v := range validators {
		expected := float64(trials) * float64(v.Weight) / float64(totalWeight)
		observed := float64(counts[v.ID])
		chiSq += (observed - expected) * (observed - expected) / expected
	}

	// Degrees of freedom = numValidators - 1 = 999.
	// At alpha=0.001, critical value for df=999 is ~1119.
	// This is a very lenient test -- we just want to catch gross errors.
	criticalValue := 1200.0
	require.Less(chiSq, criticalValue,
		"chi-squared %.2f exceeds critical value %.2f -- sampling is not proportional to weight",
		chiSq, criticalValue)
}

// TestStakeWeightedCutDominantValidator verifies that a validator with 99%
// of the total stake is sampled ~99% of the time in single-sample draws.
func TestStakeWeightedCutDominantValidator(t *testing.T) {
	require := require.New(t)

	dominant := Validator{ID: ids.GenerateTestNodeID(), Weight: 99_000}
	validators := []Validator{dominant}
	for i := 0; i < 100; i++ {
		validators = append(validators, Validator{
			ID:     ids.GenerateTestNodeID(),
			Weight: 10, // 1000 total from others = 1%
		})
	}

	cut, err := NewStakeWeightedCut(validators)
	require.NoError(err)

	hits := 0
	const trials = 20000
	for i := 0; i < trials; i++ {
		nodes := cut.Sample(1)
		require.Len(nodes, 1)
		if nodes[0] == dominant.ID {
			hits++
		}
	}

	ratio := float64(hits) / float64(trials)
	require.Greater(ratio, 0.95, "dominant validator (99%% stake) sampled only %.2f%% of the time", ratio*100)
}

// TestStakeWeightedCutKExceedsN verifies that requesting more samples than
// validators returns all validators exactly once.
func TestStakeWeightedCutKExceedsN(t *testing.T) {
	require := require.New(t)

	validators := make([]Validator, 5)
	idSet := make(map[ids.NodeID]bool)
	for i := range validators {
		validators[i] = Validator{ID: ids.GenerateTestNodeID(), Weight: uint64(10 * (i + 1))}
		idSet[validators[i].ID] = true
	}

	cut, err := NewStakeWeightedCut(validators)
	require.NoError(err)

	// k=10 > n=5 -- should get all 5
	nodes := cut.Sample(10)
	require.Len(nodes, 5)

	// k=100 > n=5 -- still all 5
	nodes = cut.Sample(100)
	require.Len(nodes, 5)

	// Verify all returned nodes are valid
	for _, n := range nodes {
		require.True(idSet[n], "returned unknown node %s", n)
	}
}

// TestStakeWeightedCutEqualWeight verifies that equal-weight validators
// are sampled approximately uniformly.
func TestStakeWeightedCutEqualWeight(t *testing.T) {
	require := require.New(t)

	const n = 20
	validators := make([]Validator, n)
	for i := range validators {
		validators[i] = Validator{ID: ids.GenerateTestNodeID(), Weight: 100}
	}

	cut, err := NewStakeWeightedCut(validators)
	require.NoError(err)

	const trials = 40000
	counts := make(map[ids.NodeID]int, n)
	for i := 0; i < trials; i++ {
		nodes := cut.Sample(1)
		require.Len(nodes, 1)
		counts[nodes[0]]++
	}

	expected := float64(trials) / float64(n) // 2000 per validator
	for _, v := range validators {
		observed := float64(counts[v.ID])
		// Each should be within 40% of expected (generous for randomness).
		require.InDelta(expected, observed, expected*0.4,
			"validator %s: expected ~%.0f samples, got %.0f", v.ID, expected, observed)
	}
}

// TestStakeWeightedCutMaxUint64Weight verifies no overflow when a validator
// has weight near uint64 max.
func TestStakeWeightedCutMaxUint64Weight(t *testing.T) {
	require := require.New(t)

	// Two validators: one near max, one with weight 1.
	// Total weight would overflow naive uint64 addition, but our implementation
	// adds sequentially -- let's verify it doesn't panic or loop forever.
	largeWeight := uint64(math.MaxUint64 / 2)
	v1 := Validator{ID: ids.GenerateTestNodeID(), Weight: largeWeight}
	v2 := Validator{ID: ids.GenerateTestNodeID(), Weight: 1}

	cut, err := NewStakeWeightedCut([]Validator{v1, v2})
	require.NoError(err)

	// Sample should not panic or infinite loop
	for i := 0; i < 100; i++ {
		nodes := cut.Sample(1)
		require.Len(nodes, 1)
	}

	// v1 should dominate
	hits := 0
	for i := 0; i < 1000; i++ {
		nodes := cut.Sample(1)
		if nodes[0] == v1.ID {
			hits++
		}
	}
	require.Greater(hits, 900, "large-weight validator should be selected most of the time")
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
