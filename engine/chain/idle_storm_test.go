// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// idle_storm_test.go — the IDLE-INCLUSIVE re-storm gate.
//
// The existing storm (storm_convergence_test.go) drives blocks BACK-TO-BACK and
// never idles. That is the exact coverage gap that let a fleet-deadlock ship: the
// LIVE testnet fleet (v1.34.5 / consensus v1.35.15, view-change ON) finalized a
// few blocks then ALL 5 nodes went silent simultaneously and never recovered — a
// distributed VC liveness stall that a continuous-load storm never reaches.
//
// This gate drives load, then IDLES for several round budgets (the VC round timers
// fire repeatedly with no block to converge on — the "proposer window expired"
// analogue), then RESUMES, and REPEATS across multiple idle→resume cycles. It
// requires the head to advance across EVERY idle window for many blocks.
//
//	PASS = sustained finalization: head advances after every idle window.
//	FAIL = "a few blocks then a stall after an idle window" — the live trap.
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// TestStorm_IdleWindows_SustainedFinalization is the idle-inclusive liveness gate.
func TestStorm_IdleWindows_SustainedFinalization(t *testing.T) {
	if testing.Short() {
		t.Skip("idle storm is timing-heavy; skipped in -short")
	}
	params := stormParams5VC() // K=5, α=4, round-scoped view-change
	net := newSimNet(t, 5, params)

	const cycles = 6         // idle→resume cycles
	const heightsPerBurst = 3 // blocks built back-to-back within a cycle
	idle := 5 * params.RoundTO // idle >> RoundTO so VC round timers fire many times with no block

	parentID := ids.Empty
	parentStateRoot := simGenesisRoot()
	var h uint64

	for c := 0; c < cycles; c++ {
		for b := 0; b < heightsPerBurst; b++ {
			h++
			// Every up node builds its OWN distinct sibling — the worst-case storm condition.
			blocks := make(map[ids.ID]*simBlock)
			for i := 0; i < 5; i++ {
				if !net.nodes[i].reachable() {
					continue
				}
				blk := newHonestBlock(parentID, parentStateRoot, h, "idle-"+itoa(i)+"-h"+itoa(int(h%10)))
				blocks[blk.ID()] = blk
				net.build(i, blk)
			}
			head := stormAwaitSingleHead(t, net, h)
			won, ok := blocks[head]
			if !ok {
				t.Fatalf("cycle %d height %d: finalized head %s is not one of the built siblings", c, h, head)
			}
			parentID = head
			parentStateRoot = won.stateRoot
		}
		// IDLE WINDOW: no builds. The VC round machinery sits past several round budgets —
		// the exact condition (idle past the proposer window) that stalled the live fleet.
		// The very next burst builds off a parent that has been idle for `idle` — the
		// in-process analogue of the stale-parent post-idle block.
		t.Logf("cycle %d: idling %s after height %d, then resuming", c, idle, h)
		time.Sleep(idle)
	}

	// Every height must still be singular (no late divergence) AND we must have reached
	// the full height count (no mid-run stall swallowed by a partial run).
	total := uint64(cycles * heightsPerBurst)
	if h != total {
		t.Fatalf("IDLE STALL: only reached height %d of %d expected — a stall after an idle window", h, total)
	}
	for hh := uint64(1); hh <= total; hh++ {
		if seen := net.headsAtHeight(hh); len(seen) != 1 {
			t.Fatalf("height %d diverged: %v", hh, seen)
		}
	}
	t.Logf("IDLE-STORM PASS: %d cycles × %d heights across %d idle windows (%s each), sustained finalization, no stall",
		cycles, heightsPerBurst, cycles, idle)
}

// TestStorm_IdleWindows_ZeroMargin combines the live mainnet condition — EXACTLY α=4
// healthy validators (one down → zero margin) — with idle windows. This is the VC's
// known-fragile regime (zero-margin quorum + competing siblings) crossed with the
// untested idle path. If VC stalls after an idle window with only α healthy, this is
// the in-process reproduction of the live fleet stall.
func TestStorm_IdleWindows_ZeroMargin(t *testing.T) {
	if testing.Short() {
		t.Skip("idle storm is timing-heavy; skipped in -short")
	}
	params := stormParams5VC()
	net := newSimNet(t, 5, params)
	net.down(4) // 4 healthy = exact α — the zero-margin mainnet condition, held for the whole run

	const cycles = 6
	const heightsPerBurst = 3
	idle := 5 * params.RoundTO

	parentID := ids.Empty
	parentStateRoot := simGenesisRoot()
	var h uint64

	for c := 0; c < cycles; c++ {
		for b := 0; b < heightsPerBurst; b++ {
			h++
			blocks := make(map[ids.ID]*simBlock)
			for i := 0; i < 5; i++ {
				if !net.nodes[i].reachable() {
					continue
				}
				blk := newHonestBlock(parentID, parentStateRoot, h, "zm-"+itoa(i)+"-h"+itoa(int(h%10)))
				blocks[blk.ID()] = blk
				net.build(i, blk)
			}
			head := stormAwaitSingleHead(t, net, h)
			won, ok := blocks[head]
			if !ok {
				t.Fatalf("cycle %d height %d: head %s not among siblings", c, h, head)
			}
			parentID = head
			parentStateRoot = won.stateRoot
		}
		t.Logf("zero-margin cycle %d: idling %s after height %d", c, idle, h)
		time.Sleep(idle)
	}
	total := uint64(cycles * heightsPerBurst)
	if h != total {
		t.Fatalf("ZERO-MARGIN IDLE STALL: only reached height %d of %d", h, total)
	}
	t.Logf("ZERO-MARGIN IDLE-STORM PASS: α=4 exact quorum, %d heights across %d idle windows, no stall", total, cycles)
}
