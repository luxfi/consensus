// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// multinode_liveness_multiround_test.go — the fork-fix × liveness-fix INTERACTION gate.
//
// The existing down/wedged/forked-proposer suite (multinode_proposer_test.go) parks
// RoundTO at 30s so finalization is driven by the FIRST emergent gossip pass, not the
// re-poll ticker — it proves the liveness fix in isolation. Red asked for the
// complementary test: a SHORT RoundTO so the re-poll / round-change recovery ACTIVELY
// fires, over MULTIPLE heights, with a permanently-down proposer — so the per-height
// vote-once guard (the fork fix) and the undecided-own-proposal re-solicit (the liveness
// fix) are exercised TOGETHER under sustained round changes. Every re-solicit re-presents
// the SAME canonical, which the guard admits idempotently; a conflicting sibling is
// never signed. The gate: the chain makes progress at every height AND never forks.
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// prodParams5Fast is prodParams5 with a SHORT round timeout so the background re-poll
// ticker actively re-solicits — the condition under which round-change recovery and the
// fork guard must compose. K=5, α=4 (zero-margin quorum once one validator is down).
func prodParams5Fast() config.Parameters {
	p := prodParams5()
	p.RoundTO = 200 * time.Millisecond
	return p
}

func TestMultiNode_DownProposer_MultiRoundLiveness_NoFork(t *testing.T) {
	net := newSimNet(t, 5, prodParams5Fast())
	net.down(0) // the designated proposer is permanently DOWN — the 4 up nodes are the exact α=4 quorum

	const heights = 5
	parentID := ids.Empty
	parentStateRoot := simGenesisRoot()

	for h := uint64(1); h <= heights; h++ {
		// Rotate the substitute across the UP nodes {1,2,3,4} so different builders drive
		// successive heights (real substitution, not one lucky node).
		builder := 1 + int((h-1)%4)
		blk := newHonestBlock(parentID, parentStateRoot, h, "multiround-h")
		net.build(builder, blk)

		if !waitFor(emergeTO, func() bool {
			all, fork := net.finalizedEverywhere(blk)
			return all && !fork
		}) {
			t.Fatalf("MULTI-ROUND HALT at height %d: the 4 reachable validators did not converge on %s under "+
				"active re-poll (heads=%v). Round-change recovery and the vote-once guard must compose.",
				h, blk.ID(), net.headsAtHeight(h))
		}
		// SINGLE-HEAD invariant at this height across every up node — no fork accrued.
		if seen := net.headsAtHeight(h); len(seen) != 1 {
			t.Fatalf("FORK at height %d under sustained re-poll: divergent heads %v", h, seen)
		}
		parentID = blk.ID()
		parentStateRoot = blk.stateRoot
	}

	// Every height remained singular after the whole sequence — no late divergence.
	for h := uint64(1); h <= heights; h++ {
		if seen := net.headsAtHeight(h); len(seen) != 1 {
			t.Fatalf("height %d head diverged after the full multi-round run: %v", h, seen)
		}
	}
	// The down node received nothing and finalized nothing.
	if _, ok := net.nodes[0].rt.FinalizedBlockAtHeight(1); ok {
		t.Fatal("the DOWN proposer must not have finalized anything")
	}
}
