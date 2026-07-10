// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// b1_cert_no_viewchange_test.go — the B1 premise, made explicit.
//
// B1 (owner decision 2026-07-09): keep the ⅔-stake α-of-K quorum cert as the SOLE finality
// decider (it is the real, working BFT finality) and DELETE the Tendermint view-change/lock/POL
// liveness layer bolted on top. The view-change exists ONLY to converge COMPETING siblings under
// the anyone-can-propose storm; with ONE proposer per height (the node-side windower fix, the B1
// gate) there are no competing siblings, so α canonical signatures align on the single block and
// the cert finalizes with NO view-change.
//
// This pins that premise directly: n=5, ViewChange EXPLICITLY DISABLED, one block per height, the
// fleet finalizes every height via the α-of-K cert with a single head. It is the standing proof
// that deleting the view-change removes only the storm-convergence layer, never the decider — the
// cert path is complete on its own. (The mainstream multinode suite already runs at this default;
// this makes the B1 contract an explicit, named regression rather than incidental coverage.)

package chain

import (
	"testing"

	"github.com/luxfi/ids"
)

func TestB1_CertFinalizesWithoutViewChange_OneProposerPerHeight(t *testing.T) {
	p := prodParams5()
	p.ViewChange = false // EXPLICIT: the view-change/lock layer is OFF — the cert alone decides.

	net := newSimNet(t, 5, p)

	// One proposer per height (rotated) — the post-storm-fix regime: no competing siblings, so
	// every honest node signs the SAME canonical block and α=4 signatures assemble one cert.
	const heights = 6
	parentID := ids.Empty
	parentRoot := simGenesisRoot()
	for h := uint64(1); h <= heights; h++ {
		blk := newHonestBlock(parentID, parentRoot, h, "b1-one-proposer")
		net.build(int(h%5), blk) // a single deterministic proposer builds this height
		if !waitFor(emergeTO, func() bool {
			all, fork := net.finalizedEverywhere(blk)
			return all && !fork
		}) {
			t.Fatalf("B1: height %d must finalize via the α-of-K cert with ViewChange OFF; heads=%v",
				h, net.headsAtHeight(h))
		}
		if seen := net.headsAtHeight(h); len(seen) != 1 {
			t.Fatalf("B1: single head required at height %d (the cert is the decider), got %v", h, seen)
		}
		parentID, parentRoot = blk.ID(), blk.stateRoot
	}

	// Every node finalized the identical chain — the cert alone gave BFT agreement, no view-change.
	for h := uint64(1); h <= heights; h++ {
		var first ids.ID
		for i, n := range net.nodes {
			got, ok := n.rt.FinalizedBlockAtHeight(h)
			if !ok {
				t.Fatalf("B1: node %d missing finalized height %d", i, h)
			}
			if i == 0 {
				first = got
			} else if got != first {
				t.Fatalf("B1: divergence at height %d — node %d has %s, node 0 has %s", h, i, got, first)
			}
		}
	}
}
