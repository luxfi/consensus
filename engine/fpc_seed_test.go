// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/luxfi/consensus/protocol/wave/fpc"
	"github.com/luxfi/ids"
)

// The FPC threshold is the count of votes a round must see before it moves. It
// is drawn per phase from the epoch seed, so validators agree about what a
// majority is exactly as far as they agree about the seed. These tests measure
// that agreement rather than asserting it.

const (
	k     = 20
	alpha = 15
	beta  = 10
)

// phases covers the first rounds of an epoch, which is where a driver spends
// its life; the seed is redrawn before it gets much further.
var phases = []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

func thresholds(d *Driver) []int {
	out := make([]int, 0, len(phases))
	for _, p := range phases {
		out = append(out, d.Threshold(p))
	}
	return out
}

// TestEveryNodeDemandsTheSameMajority is the property the ruling exists for:
// twenty separately constructed drivers, told the same epoch, must demand the
// same number of votes in every phase.
func TestEveryNodeDemandsTheSameMajority(t *testing.T) {
	chain := []byte("lux-mainnet")
	parent := []byte("0x3f4fa2a0b0ce089f52bf0ae9199c75ffdd76ecafc987794050cb0d286f1ec61e")

	want := thresholds(NewLuxConsensus(k, alpha, beta, WithEpoch(7, chain, parent)))
	for node := 1; node < 20; node++ {
		got := thresholds(NewLuxConsensus(k, alpha, beta, WithEpoch(7, chain, parent)))
		for i, p := range phases {
			if got[i] != want[i] {
				t.Fatalf("node %d demands %d votes at phase %d, node 0 demands %d: "+
					"two validators disagree about what a majority is",
					node, got[i], p, want[i])
			}
		}
	}
	t.Logf("k=%d, epoch 7: thresholds %v, identical across 20 drivers", k, want)
}

// TestADrawnSeedSplitsTheMajority measures what the removed code did. It builds
// waves the old way — a seed drawn per instance — and records the spread of the
// threshold at one phase. The old path is not called; it is reconstructed here
// so the defect stays measured rather than remembered.
func TestADrawnSeedSplitsTheMajority(t *testing.T) {
	seen := map[int]int{}
	for node := 0; node < 200; node++ {
		var seed [32]byte
		if _, err := rand.Read(seed[:]); err != nil {
			t.Fatal(err)
		}
		w, err := wave.New[ids.ID](wave.Config{
			K: k, Alpha: 0.75, Beta: beta, RoundTO: time.Second,
			EnableFPC: true, ThetaMin: 0.5, ThetaMax: 0.8, FPCSeed: seed[:],
		}, &SimpleCut{k: k}, &SimpleTransport{})
		if err != nil {
			t.Fatal(err)
		}
		seen[w.Threshold(1)]++
	}
	if len(seen) < 2 {
		t.Fatalf("a drawn seed produced one threshold %v across 200 draws; "+
			"the measurement is broken, not the defect", seen)
	}
	t.Logf("drawn seed, k=%d, phase 1: threshold spread %v across 200 draws", k, seen)

	// The derived path, asked the same question, answers once.
	derived := map[int]int{}
	for node := 0; node < 200; node++ {
		derived[NewLuxConsensus(k, alpha, beta, WithEpoch(7, []byte("lux-mainnet"), nil)).Threshold(1)]++
	}
	if len(derived) != 1 {
		t.Fatalf("derived seed produced %d distinct thresholds: %v", len(derived), derived)
	}
	t.Logf("derived seed, k=%d, phase 1: threshold %v across 200 drivers", k, derived)
}

// TestTheEpochMovesTheThreshold: a seed that did not move with history would be
// a fixed schedule an adversary could read off once and plan against.
func TestTheEpochMovesTheThreshold(t *testing.T) {
	chain := []byte("lux-mainnet")
	base := thresholds(NewLuxConsensus(k, alpha, beta, WithEpoch(7, chain, []byte("parent-a"))))

	for _, c := range []struct {
		note   string
		driver *Driver
	}{
		{"the next epoch", NewLuxConsensus(k, alpha, beta, WithEpoch(8, chain, []byte("parent-a")))},
		{"another chain", NewLuxConsensus(k, alpha, beta, WithEpoch(7, []byte("lux-testnet"), []byte("parent-a")))},
		{"another parent", NewLuxConsensus(k, alpha, beta, WithEpoch(7, chain, []byte("parent-b")))},
	} {
		got := thresholds(c.driver)
		same := true
		for i := range base {
			if got[i] != base[i] {
				same = false
				break
			}
		}
		if same {
			t.Errorf("%s left the whole threshold sequence unchanged: %v", c.note, got)
		}
	}
}

// TestTheDefaultEpochIsGenesis pins what a driver constructed without an epoch
// derives, so "no epoch given" stays a named value rather than drifting.
func TestTheDefaultEpochIsGenesis(t *testing.T) {
	bare := thresholds(NewLuxConsensus(k, alpha, beta))
	genesis := thresholds(NewLuxConsensus(k, alpha, beta, WithEpoch(0, nil, nil)))
	for i, p := range phases {
		if bare[i] != genesis[i] {
			t.Fatalf("phase %d: bare driver demands %d, genesis epoch demands %d", p, bare[i], genesis[i])
		}
	}

	// And it is the seed the corpus records for epoch zero, not an accident of
	// construction.
	sel, err := fpc.NewSelector(0.5, 0.8, fpc.DeriveEpochSeed(0, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range phases {
		if want := sel.SelectThreshold(p, k); bare[i] != want {
			t.Fatalf("phase %d: driver demands %d, DeriveEpochSeed(0,nil,nil) demands %d", p, bare[i], want)
		}
	}
}

// TestTheCutDoesNotMoveTheThreshold keeps the two kinds of randomness apart:
// which peers a node samples is deliberately its own business, what majority it
// then demands is not.
func TestTheCutDoesNotMoveTheThreshold(t *testing.T) {
	chain := []byte("lux-mainnet")
	var uniform prism.Cut[ids.ID] = &SimpleCut{k: k}
	a := thresholds(NewLuxConsensus(k, alpha, beta, WithEpoch(7, chain, nil)))
	b := thresholds(NewLuxConsensus(k, alpha, beta, WithEpoch(7, chain, nil), WithCut(uniform)))
	for i, p := range phases {
		if a[i] != b[i] {
			t.Fatalf("phase %d: the sampler changed the threshold, %d vs %d", p, a[i], b[i])
		}
	}
}
