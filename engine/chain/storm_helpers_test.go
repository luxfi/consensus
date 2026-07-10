// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// storm_helpers_test.go — the shared STORM-INERT harness. Competing siblings must
// converge to ONE finalized head per height with NO lock / view-change machinery,
// purely by Nova sampling + the deterministic lowest-canonical settle. Shared by the
// storm-inert, committee-scaling, competing-fork, and mainnet-committee gates.

// stormParams5 is the 5-validator BFT param set for the storm gate: K=5, α=4, with a
// round budget large enough that the convergence settle window (RoundTO/2 = 500ms)
// comfortably exceeds sibling-gossip latency — even under the pathological slowdown of
// the `-race` detector — so every honest node sees the full sibling set before it binds
// its one signature.
func stormParams5() config.Parameters {
	p := prodParams5()
	p.RoundTO = 1 * time.Second // settle = RoundTO/2 = 500ms — comfortably exceeds real gossip latency
	return p
}

// stormTO is a generous per-height convergence ceiling for the storm tests. It is only a
// CEILING — stormAwaitSingleHead returns the instant the net converges, so a healthy run
// is unaffected; the headroom exists so the gate does not flake under the heavy CPU
// contention of the full `go test` package run.
const stormTO = 30 * time.Second

// stormAwaitSingleHead waits until every UP node has finalized the SAME single block at
// height h (emergent convergence), and returns that head. Fails on a fork (two distinct
// finalized heads at one height) immediately.
func stormAwaitSingleHead(t *testing.T, net *simNet, h uint64) ids.ID {
	t.Helper()
	deadline := time.Now().Add(stormTO)
	for time.Now().Before(deadline) {
		heads := net.headsAtHeight(h)
		if len(heads) > 1 {
			t.Fatalf("DOUBLE-FINALIZATION at height %d: distinct finalized heads %v — two finality "+
				"attestations at one height (the safety violation Nova single-accept must make impossible)", h, heads)
		}
		if len(heads) == 1 {
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
