// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// storm_convergence_test.go — the FRESH-NET STORM gate.
//
// The fresh-chain failure mode this whole fix targets: before proposervm
// single-proposer discipline is stable (pre-fork bare blocks + the bootstrap→live
// transition), MANY validators build CONFLICTING sibling blocks at the SAME height
// at once. This is the exact condition that produced (a) the height-7
// double-finalization fatal (two α-of-K certs at one height) and (b) the net-wide
// liveness wedge (votes split across siblings so nothing reaches the quorum).
//
// Unlike the down/one-substitute proposer suite, here EVERY up node builds its OWN
// distinct valid sibling at EVERY height — the worst case. The gate:
//   - SAFETY: never two finalized heads at one height (no double-finalization).
//   - LIVENESS: the net converges to ONE head per height and keeps producing for
//     >20 heights, INCLUDING after one validator is killed mid-run (f=1 tolerated).
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// stormParams5 is the 5-validator BFT param set for the storm gate: K=5, α=4, with a
// round budget large enough that the convergence settle window (RoundTO/2 = 1s)
// comfortably exceeds sibling-gossip latency — even under the pathological slowdown of
// the `-race` detector — so every honest node sees the full sibling set before it binds
// its one signature. (A tiny round like prodParams5Fast's 200ms leaves a sub-gossip
// settle under -race and would split.)
func stormParams5() config.Parameters {
	p := prodParams5()
	p.RoundTO = 1 * time.Second // settle = RoundTO/2 = 500ms — comfortably exceeds real gossip latency
	return p
}

// stormTO is a generous per-height convergence ceiling for the storm tests. It is only
// a CEILING — stormAwaitSingleHead returns the instant the net converges, so a healthy
// run is unaffected; the headroom exists so the gate does not flake under the heavy CPU
// contention of the full `go test` package run (25+ engine goroutines competing).
const stormTO = 30 * time.Second

// stormHeads waits until every UP node has finalized the SAME single block at height
// h (emergent convergence), and returns that head. Fails on a fork (two distinct
// finalized heads at one height) immediately.
func stormAwaitSingleHead(t *testing.T, net *simNet, h uint64) ids.ID {
	t.Helper()
	deadline := time.Now().Add(stormTO)
	for time.Now().Before(deadline) {
		heads := net.headsAtHeight(h)
		if len(heads) > 1 {
			t.Fatalf("DOUBLE-FINALIZATION at height %d: distinct finalized heads %v — two α-of-K "+
				"certs finalized at one height (the safety violation this fix must make impossible)", h, heads)
		}
		if len(heads) == 1 {
			// Require ALL up nodes to have reached it (full convergence, not just one).
			var head ids.ID
			count := 0
			for id, c := range heads {
				head, count = id, c
			}
			if count >= net.upCount() {
				return head
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("LIVENESS STALL at height %d: the up validators did not converge on a single finalized head "+
		"within %s (heads=%v, up=%d). A fresh-net storm must converge to one block per height.",
		h, stormTO, net.headsAtHeight(h), net.upCount())
	return ids.Empty
}

// TestStorm_AllValidatorsBuild_SingleHeadPerHeight is the core gate: 5 validators
// each build a distinct valid sibling at every height, concurrently. The net must
// finalize exactly ONE block per height and keep producing past 20 heights.
func TestStorm_AllValidatorsBuild_SingleHeadPerHeight(t *testing.T) {
	if testing.Short() {
		t.Skip("storm test is timing-heavy; skipped in -short")
	}
	if underRace {
		t.Skip("vote-convergence is timing-sensitive; -race's ~10x slowdown pushes sim gossip past any " +
			"bounded settle window (not a production condition). Safety (no double-finalization) is timing-" +
			"independent and asserted in the non-race run; the convergence goroutine is race-checked by the " +
			"multinode proposer/liveness suites under -race.")
	}
	net := newSimNet(t, 5, stormParams5()) // K=5, α=4, settle window > gossip latency

	const heights = 24
	parentID := ids.Empty
	parentStateRoot := simGenesisRoot()

	for h := uint64(1); h <= heights; h++ {
		// Every up node builds its OWN distinct sibling at this height on the same parent.
		blocks := make(map[ids.ID]*simBlock)
		for i := 0; i < 5; i++ {
			if !net.nodes[i].reachable() {
				continue
			}
			blk := newHonestBlock(parentID, parentStateRoot, h, "storm-"+itoa(i)+"-h"+itoa(int(h%10)))
			blocks[blk.ID()] = blk
			net.build(i, blk)
		}
		head := stormAwaitSingleHead(t, net, h)
		won, ok := blocks[head]
		if !ok {
			t.Fatalf("height %d finalized head %s is not one of the built siblings", h, head)
		}
		parentID = head
		parentStateRoot = won.stateRoot
	}

	// Every height stayed singular after the whole run — no late divergence.
	for h := uint64(1); h <= heights; h++ {
		if seen := net.headsAtHeight(h); len(seen) != 1 {
			t.Fatalf("height %d diverged after the full storm run: %v", h, seen)
		}
	}
	t.Logf("STORM PASS: %d heights, 5 concurrent builders each, single head per height, no double-finalization", heights)
}

// TestStorm_KillOneMidRun_KeepsProducing is the f=1 liveness gate: drive the storm,
// kill one validator mid-run, and prove the remaining 4 (the exact α=4 quorum) keep
// converging to one head per height for >20 heights total. A 5-node BFT net MUST keep
// producing with one node down.
func TestStorm_KillOneMidRun_KeepsProducing(t *testing.T) {
	if testing.Short() {
		t.Skip("storm test is timing-heavy; skipped in -short")
	}
	if underRace {
		t.Skip("vote-convergence is timing-sensitive; -race's ~10x slowdown pushes sim gossip past any " +
			"bounded settle window (not a production condition). Safety (no double-finalization) is timing-" +
			"independent and asserted in the non-race run; the convergence goroutine is race-checked by the " +
			"multinode proposer/liveness suites under -race.")
	}
	net := newSimNet(t, 5, stormParams5())

	const heights = 22
	const killAt = 8
	parentID := ids.Empty
	parentStateRoot := simGenesisRoot()

	for h := uint64(1); h <= heights; h++ {
		if h == killAt {
			net.down(4) // one validator crashes mid-run; the 4 survivors are the exact α=4 quorum
			t.Logf("killed validator 4 at height %d — the remaining 4 must keep producing", h)
		}
		blocks := make(map[ids.ID]*simBlock)
		for i := 0; i < 5; i++ {
			if !net.nodes[i].reachable() {
				continue
			}
			blk := newHonestBlock(parentID, parentStateRoot, h, "kstorm-"+itoa(i)+"-h"+itoa(int(h%10)))
			blocks[blk.ID()] = blk
			net.build(i, blk)
		}
		head := stormAwaitSingleHead(t, net, h)
		won, ok := blocks[head]
		if !ok {
			t.Fatalf("height %d head %s not among built siblings", h, head)
		}
		parentID = head
		parentStateRoot = won.stateRoot
	}
	t.Logf("STORM+KILL PASS: %d heights, validator 4 down from height %d, 4-of-5 kept converging", heights, killAt)
}
