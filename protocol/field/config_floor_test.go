// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.
//
// NewDriver discards wave.New's error. That is only safe if its defaulting
// leaves no Config that wave.New refuses — otherwise the Driver holds a zero
// Wave whose nil state map panics on the first Tick.
//
// Defaulting used to test `== 0`, so a negative PollSize or Beta, or an Alpha
// outside (0, 1], reached wave.New's refusal and produced exactly that.

package field

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/core/types"
	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/stretchr/testify/require"
)

type floorTransport struct{}

func (floorTransport) RequestVotes(ctx context.Context, peers []types.NodeID, item string) <-chan wave.Photon[string] {
	ch := make(chan wave.Photon[string])
	close(ch)
	return ch
}

func (floorTransport) MakeLocalPhoton(item string, prefer bool) wave.Photon[string] {
	return wave.Photon[string]{Item: item, Prefer: prefer}
}

type floorCut struct{}

func (floorCut) Sample(k int) []types.NodeID { return nil }
func (floorCut) Luminance() prism.Luminance  { return prism.Luminance{} }

type floorStore struct{}

func (floorStore) Head() []string                       { return []string{"a"} }
func (floorStore) Get(string) (BlockView[string], bool) { return nil, false }
func (floorStore) Children(string) []string             { return nil }

type floorProposer struct{}

func (floorProposer) Propose(ctx context.Context, parents []string) (string, error) {
	return "b", nil
}

type floorCommitter struct{}

func (floorCommitter) Commit(ctx context.Context, ordered []string) error { return nil }

// Every Config, however malformed, must yield a Driver whose Tick does not
// panic. Each negative case here left a zero Wave before the defaulting was
// widened past `== 0`.
func TestNewDriverSurvivesMalformedConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"zero", Config{}},
		{"negative PollSize", Config{PollSize: -1, Alpha: 0.8, Beta: 15}},
		{"zero Beta", Config{PollSize: 20, Alpha: 0.8, Beta: 0}},
		{"negative Alpha", Config{PollSize: 20, Alpha: -0.5, Beta: 15}},
		{"Alpha above one", Config{PollSize: 20, Alpha: 1.5, Beta: 15}},
		{"Alpha exactly one is legal", Config{PollSize: 20, Alpha: 1.0, Beta: 15}},
		{"every field invalid", Config{PollSize: -7, Alpha: -7, Beta: 0, RoundTO: -time.Second}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := NewDriver[string](c.cfg, floorCut{}, floorTransport{},
				floorStore{}, floorProposer{}, floorCommitter{})
			require.NotNil(t, d)
			require.NotPanics(t, func() {
				_ = d.Tick(context.Background())
			}, "a Driver built from this Config must not panic on Tick")
		})
	}
}
