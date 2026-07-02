// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// competing_fork_deadlock_test.go — REPRODUCTION of the mainnet/testnet
// COMPETING-FORK FINALIZATION DEADLOCK (the go-live ship-blocker).
//
// The storm/liveness suites converge because the in-process bus delivers every
// sibling to every node INSTANTLY and the settle window (500ms) comfortably
// exceeds that ~0ms gossip latency — so every honest node sees the SAME sibling
// set before it binds its one signature (storm_convergence_test.go even SKIPS
// under -race because the detector's slowdown "pushes sim gossip past any bounded
// settle window"). That skip condition IS the production bug: on a real WAN, with
// a validator freshly dead (α=4, zero margin) and siblings arriving asymmetrically,
// the settle-window > gossip-latency assumption is violated, honest nodes bind
// their ONE per-height signature to DIFFERENT siblings, and the irrevocable
// one-signature-per-height guard (reserveSlotForSign / committedSlot) makes the
// split PERMANENT — no node may ever switch its vote to converge.
//
// This test injects the missing condition (a transient partition that outlasts the
// settle window) so two honest groups each sign a different valid sibling, then
// HEALS the partition and asserts the property go-live requires: the height must
// finalize to a SINGLE head (liveness). Under the current engine it STALLS — the
// exact 415→416 testnet freeze. Gated behind LUX_REPRO_DEADLOCK so it documents
// (and, post-fix, gates) the bug without failing the default suite.
package chain

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

func TestRepro_CompetingFork_AsymmetricSplit_Deadlocks(t *testing.T) {
	if os.Getenv("LUX_REPRO_DEADLOCK") == "" {
		t.Skip("reproduction of the competing-fork liveness deadlock; set LUX_REPRO_DEADLOCK=1 to run")
	}
	net := newSimNet(t, 5, stormParams5()) // K=5, α=4
	net.down(0)                            // designated proposer DEAD → 4 live {1,2,3,4} = exact α=4 quorum

	// Split the 4 live validators into two groups whose block/vote gossip cannot
	// cross while the partition is up. Node 1 (P1) and node 2 (P2) each build a
	// distinct VALID sibling at height 1 on the same (genesis) parent.
	nid := func(i int) ids.NodeID { return net.vs.nodeID(i) }
	group := map[ids.NodeID]int{nid(1): 1, nid(3): 1, nid(2): 2, nid(4): 2}
	var partitioned atomic.Bool
	partitioned.Store(true)
	net.bus.setLink(func(from, to ids.NodeID, _ busMsgKind) bool {
		if !partitioned.Load() {
			return true // healed — every link up
		}
		gf, gt := group[from], group[to]
		if gf == 0 || gt == 0 {
			return true // node 0 is down anyway
		}
		return gf == gt // same group ⇒ deliver; cross-group ⇒ drop
	})

	A := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "split-A")
	B := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "split-B")
	net.build(1, A) // P1 builds and gossips A within {1,3}
	net.build(2, B) // P2 builds and gossips B within {2,4}

	// Hold the partition longer than the settle window (500ms) so each group binds
	// its ONE signature to its own sibling: A←{1,3}=2, B←{2,4}=2. Neither reaches α=4.
	time.Sleep(2 * time.Second)
	partitioned.Store(false) // HEAL: all four now see BOTH siblings…

	// …but each node already cast (and durably locked) its one per-height signature.
	// P1 is committed to A, P2 to B; reserveSlotForSign refuses any switch. A stays at
	// 2, B stays at 2, node 0 is down — no sibling can ever reach α=4.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		heads := net.headsAtHeight(1)
		if len(heads) > 1 {
			t.Fatalf("DOUBLE-FINALIZATION at height 1 (safety break, not the bug under test): %v", heads)
		}
		if len(heads) == 1 {
			for _, c := range heads {
				if c >= net.upCount() {
					t.Logf("CONVERGED: height 1 finalized to a single head across all %d up nodes", net.upCount())
					return
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("LIVENESS DEADLOCK REPRODUCED: height 1 never finalized (heads=%v, up=%d). Two honest groups bound "+
		"their one per-height signature to different valid siblings (A←{1,3}, B←{2,4}); with the designated "+
		"proposer dead the 4 survivors are the exact α=4 quorum, so neither sibling can reach α, and the "+
		"irrevocable one-signature-per-height guard forbids any node from switching to converge. This is the "+
		"testnet 415→416 freeze.", net.headsAtHeight(1), net.upCount())
}
