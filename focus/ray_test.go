// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package focus

import (
	"testing"

	"github.com/luxfi/consensus/photon"
	"github.com/stretchr/testify/require"
)

type TestID string

func TestRayBasic(t *testing.T) {
	// Initial state
	prev := Step[TestID]{Prefer: false, Conf: 0}

	// All votes for true
	samples := []photon.Photon[TestID]{
		{Item: "test", Prefer: true},
		{Item: "test", Prefer: true},
		{Item: "test", Prefer: true},
		{Item: "test", Prefer: true},
		{Item: "test", Prefer: true},
	}

	next := Apply(samples, prev, Params{Alpha: 0.8})

	require.True(t, next.Prefer)
	require.Equal(t, uint32(1), next.Conf)
}

func TestRayConfidenceBuildup(t *testing.T) {
	prev := Step[TestID]{Prefer: true, Conf: 3}

	// Continue voting true
	samples := []photon.Photon[TestID]{
		{Item: "test", Prefer: true},
		{Item: "test", Prefer: true},
		{Item: "test", Prefer: true},
		{Item: "test", Prefer: true},
	}

	next := Apply(samples, prev, Params{Alpha: 0.8})

	require.True(t, next.Prefer)
	require.Equal(t, uint32(4), next.Conf) // Confidence increases
}

func TestRayFlip(t *testing.T) {
	prev := Step[TestID]{Prefer: true, Conf: 2}

	// All votes for false
	samples := []photon.Photon[TestID]{
		{Item: "test", Prefer: false},
		{Item: "test", Prefer: false},
		{Item: "test", Prefer: false},
		{Item: "test", Prefer: false},
		{Item: "test", Prefer: false},
	}

	next := Apply(samples, prev, Params{Alpha: 0.8})

	require.False(t, next.Prefer) // Flipped to false
	require.Equal(t, uint32(1), next.Conf) // Reset confidence
}

func TestRayInconclusive(t *testing.T) {
	prev := Step[TestID]{Prefer: true, Conf: 5}

	// Split vote - inconclusive
	samples := []photon.Photon[TestID]{
		{Item: "test", Prefer: true},
		{Item: "test", Prefer: true},
		{Item: "test", Prefer: false},
		{Item: "test", Prefer: false},
	}

	next := Apply(samples, prev, Params{Alpha: 0.8})

	// State unchanged on inconclusive
	require.Equal(t, prev.Prefer, next.Prefer)
	require.Equal(t, prev.Conf, next.Conf)
}

func TestRayThreshold(t *testing.T) {
	testCases := []struct {
		name      string
		samples   []bool
		alpha     float64
		prevPref  bool
		expectPref bool
		expectInc bool // expect confidence increase
	}{
		{
			name:      "exact_threshold",
			samples:   []bool{true, true, true, true, false}, // 4/5 = 0.8
			alpha:     0.8,
			prevPref:  true,
			expectPref: true,
			expectInc: true,
		},
		{
			name:      "below_threshold",
			samples:   []bool{true, true, true, false, false}, // 3/5 = 0.6
			alpha:     0.8,
			prevPref:  true,
			expectPref: true,
			expectInc: false, // inconclusive
		},
		{
			name:      "unanimous",
			samples:   []bool{true, true, true, true, true},
			alpha:     0.8,
			prevPref:  false,
			expectPref: true,
			expectInc: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prev := Step[TestID]{Prefer: tc.prevPref, Conf: 2}
			
			samples := make([]photon.Photon[TestID], len(tc.samples))
			for i, pref := range tc.samples {
				samples[i] = photon.Photon[TestID]{Item: "test", Prefer: pref}
			}

			next := Apply(samples, prev, Params{Alpha: tc.alpha})

			require.Equal(t, tc.expectPref, next.Prefer)
			if tc.expectInc {
				if next.Prefer == prev.Prefer {
					require.Greater(t, next.Conf, prev.Conf)
				} else {
					require.Equal(t, uint32(1), next.Conf)
				}
			} else {
				require.Equal(t, prev.Conf, next.Conf)
			}
		})
	}
}

func TestRayEmptySamples(t *testing.T) {
	prev := Step[TestID]{Prefer: true, Conf: 10}

	// No samples
	samples := []photon.Photon[TestID]{}

	next := Apply(samples, prev, Params{Alpha: 0.8})

	// Should return previous state unchanged
	require.Equal(t, prev, next)
}

func BenchmarkRayApply(b *testing.B) {
	prev := Step[TestID]{Prefer: false, Conf: 0}
	samples := make([]photon.Photon[TestID], 100)
	for i := range samples {
		samples[i] = photon.Photon[TestID]{
			Item:   TestID("test"),
			Prefer: i%2 == 0,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Apply(samples, prev, Params{Alpha: 0.8})
	}
}