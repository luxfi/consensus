// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.
//
// The vote threshold and the config floor.
//
// Two ways this package decided on evidence it did not have:
//
//   - The fixed-Alpha threshold truncated. K=20 Alpha=0.69 is 13.8, so 13 votes
//     of 20 cleared a threshold declared at 14 (config.DefaultParams
//     AlphaPreference) — 65.0% on a chain configured for 69%. config.AlphaForK
//     and fpc.Selector.SelectThreshold both round up; only this path did not.
//
//   - New validated nothing. Beta=0 makes `Count >= Beta` in Tick true before a
//     vote is counted, and K=0 or Alpha=0 puts the threshold at 0, which
//     yesVotes=0 already meets. Every one of those is a zero value, so a Config
//     built by a caller that forgot a field decided immediately.

package wave

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/stretchr/testify/require"
)

// shippedProfiles are the K/Alpha pairs in config, with the AlphaPreference
// each profile declares. The threshold this package computes must agree with
// the declared value, not sit a vote below it.
var shippedProfiles = []struct {
	name            string
	k               int
	alpha           float64
	alphaPreference int
}{
	{"default", 20, 0.69, 14},
	{"mainnet", 21, 0.69, 15},
	{"testnet", 11, 0.69, 8},
}

func TestThresholdMatchesDeclaredAlphaPreference(t *testing.T) {
	for _, p := range shippedProfiles {
		t.Run(p.name, func(t *testing.T) {
			got := int(math.Ceil(float64(p.k) * p.alpha))
			require.Equal(t, p.alphaPreference, got,
				"threshold must equal the profile's declared AlphaPreference")
			require.Equal(t, config.AlphaForK(p.k), got,
				"threshold must agree with config.AlphaForK")
			require.Less(t, int(float64(p.k)*p.alpha), got,
				"truncation must be strictly below the correct threshold, "+
					"or this test cannot detect the regression")
		})
	}
}

// decide runs one round with the given votes and reports whether the item was
// decided. Beta=1 so a single cleared threshold is enough to observe.
func decide(t *testing.T, k int, alpha float64, yes, no int) (*WaveState, bool) {
	t.Helper()
	tx := newMockTransport[string]()
	for i := 0; i < yes; i++ {
		tx.AddVote("item", true)
	}
	for i := 0; i < no; i++ {
		tx.AddVote("item", false)
	}
	w, err := New[string](Config{
		K: k, Alpha: alpha, Beta: 1, RoundTO: 200 * time.Millisecond,
	}, newMockCut[string](k), tx)
	require.NoError(t, err)
	w.Tick(context.Background(), "item")
	return w.State("item")
}

// At K=20 Alpha=0.69 the threshold is 14. Truncation made it 13, so 13 yes
// votes decided Accept; they must not.
func TestVotesBelowThresholdDoNotDecide(t *testing.T) {
	state, ok := decide(t, 20, 0.69, 13, 7)
	require.True(t, ok)
	require.False(t, state.Decided,
		"13 of 20 is 65.0%%, below the 69%% threshold, and must not decide")
	require.Zero(t, state.Count, "confidence must not accrue below threshold")
}

func TestVotesAtThresholdDecide(t *testing.T) {
	state, ok := decide(t, 20, 0.69, 14, 6)
	require.True(t, ok)
	require.True(t, state.Decided, "14 of 20 meets the 69%% threshold")
	require.Equal(t, uint32(1), state.Count)
}

// The same boundary on the reject side: 13 no votes must not finalize a reject.
func TestVotesBelowThresholdDoNotDecideReject(t *testing.T) {
	state, ok := decide(t, 20, 0.69, 7, 13)
	require.True(t, ok)
	require.False(t, state.Decided,
		"13 no votes of 20 is below the threshold and must not decide")
}

func TestVotesAtThresholdDecideReject(t *testing.T) {
	state, ok := decide(t, 20, 0.69, 6, 14)
	require.True(t, ok)
	require.True(t, state.Decided)
	require.Equal(t, uint32(1), state.Count)
}

// Every profile's declared AlphaPreference must be exactly the vote count that
// flips the decision, and one fewer must not.
func TestThresholdBoundaryPerProfile(t *testing.T) {
	for _, p := range shippedProfiles {
		t.Run(p.name, func(t *testing.T) {
			below, ok := decide(t, p.k, p.alpha, p.alphaPreference-1, p.k-p.alphaPreference+1)
			require.True(t, ok)
			require.False(t, below.Decided,
				"%d of %d is below the declared threshold %d",
				p.alphaPreference-1, p.k, p.alphaPreference)

			at, ok := decide(t, p.k, p.alpha, p.alphaPreference, p.k-p.alphaPreference)
			require.True(t, ok)
			require.True(t, at.Decided,
				"%d of %d meets the declared threshold %d",
				p.alphaPreference, p.k, p.alphaPreference)
		})
	}
}

// --- the config floor: each zero value decided with no evidence ---

func TestNewRefusesConfigThatDecidesWithoutEvidence(t *testing.T) {
	cut := newMockCut[string](20)
	tx := newMockTransport[string]()

	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"zero Beta decides before a vote is counted",
			Config{K: 20, Alpha: 0.69, Beta: 0}, "Beta must be at least 1"},
		{"zero K puts the threshold at zero",
			Config{K: 0, Alpha: 0.69, Beta: 1}, "K must be at least 1"},
		{"negative K",
			Config{K: -1, Alpha: 0.69, Beta: 1}, "K must be at least 1"},
		{"zero Alpha puts the threshold at zero",
			Config{K: 20, Alpha: 0, Beta: 1}, "Alpha must be in (0, 1]"},
		{"negative Alpha",
			Config{K: 20, Alpha: -0.5, Beta: 1}, "Alpha must be in (0, 1]"},
		{"Alpha above one is unreachable, so nothing ever decides",
			Config{K: 20, Alpha: 1.5, Beta: 1}, "Alpha must be in (0, 1]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := New[string](c.cfg, cut, tx)
			require.Error(t, err, "New must refuse this Config")
			require.Contains(t, err.Error(), c.want)
		})
	}
}

// The zero Config is what a caller gets by forgetting to fill one in.
func TestNewRefusesZeroConfig(t *testing.T) {
	_, err := New[string](Config{}, newMockCut[string](1), newMockTransport[string]())
	require.Error(t, err)
}

// Every shipped profile must pass the floor, or the floor is wrong.
func TestShippedProfilesPassValidation(t *testing.T) {
	for _, p := range shippedProfiles {
		t.Run(p.name, func(t *testing.T) {
			_, err := New[string](Config{
				K: p.k, Alpha: p.alpha, Beta: 1, RoundTO: time.Second,
			}, newMockCut[string](p.k), newMockTransport[string]())
			require.NoError(t, err)
		})
	}
}
