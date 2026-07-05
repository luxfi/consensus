// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// multinode_reconverge_test.go — the DEFINITIVE partition/heal re-convergence proof.
//
// The owner's reliability gate: a validator that falls behind must REJOIN and catch the
// tip in seconds, with NO operator snapshot-restore. This test partitions one node off a
// K=5 majority, advances the majority ~35 finalized blocks (each with a real α-of-K cert),
// heals the node, and asserts it re-converges to the majority tip END-TO-END — driven by
// the node's OWN engine (requestCatchup → RequestAncestors), not a hand-fed quorum.
//
// It exercises the whole rejoin fix set together: #1 (proposervm build-pref never poisons —
// modeled by the engine building on lastAccepted through the gap), #2 (steer only to held
// tips), #3 (peer-select reaches a serving peer — busCatchup always finds one), #5 (a
// behind node discovers it must fetch the CERTS, not just blocks), #4 (contiguous
// oldest-first cert-accept makes monotonic progress). The cert-carry accept path
// (AcceptCatchupBlock) is the SAME audited predicate live finality uses — no weaker
// authority is introduced by catch-up.
package chain

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// busCatchup is the harness's node-layer catch-up transport (NetworkConfig.Catchup). It
// models the FIXED node layer: when the engine calls requestCatchup(missingID) it serves
// the requester its missing FINALIZED gap — each block paired with the real cert the
// serving peer stored on finalize (CertForBlock) — oldest-first, via AcceptCatchupBlock.
// Delivery is asynchronous (a fresh goroutine) so it never re-enters the caller's engine
// lock, exactly like the real network round-trip. Selection (#3) always reaches a serving
// peer; the served pairs let the behind node finalize the gap on VERIFIED certs (#5) with
// no re-vote.
type busCatchup struct {
	net     *simNet
	selfIdx int
}

func (c *busCatchup) RequestAncestors(_ ids.ID, _ ids.ID, _ ids.ID, _ ids.NodeID) error {
	self := c.net.nodes[c.selfIdx]

	// #3: pick the most-finalized reachable peer as the server (never EmptyNodeID / self).
	var server *simNode
	var serverH uint64
	for i, n := range c.net.nodes {
		if i == c.selfIdx || !n.reachable() {
			continue
		}
		if _, h, ok := n.rt.FinalizedLedger(); ok && (server == nil || h > serverH) {
			server, serverH = n, h
		}
	}
	if server == nil {
		return nil
	}

	// This node's finalized floor — serve strictly above it, oldest-first (#4 contiguity).
	var floor uint64
	if _, h, ok := self.rt.FinalizedLedger(); ok {
		floor = h
	}
	if serverH <= floor {
		return nil
	}

	go func() {
		ctx := context.Background()
		for h := floor + 1; h <= serverH; h++ {
			id, ok := server.rt.FinalizedBlockAtHeight(h)
			if !ok {
				continue
			}
			certBytes, ok := server.rt.Transitive.CertForBlock(id)
			if !ok {
				continue // cert aged out of the served window — a real node bootstraps instead
			}
			server.vm.mu.Lock()
			blk := server.vm.blockByID[id]
			server.vm.mu.Unlock()
			if blk == nil {
				continue
			}
			if !self.reachable() {
				return
			}
			// SAME audited cert predicate as live finality; idempotent on an already-decided
			// height. A reject (bad/forged cert) does NOT halt the batch — try the next.
			_ = self.rt.AcceptCatchupBlock(ctx, blk.Bytes(), certBytes)
		}
	}()
	return nil
}

// reconvergeTO bounds the "catch up in SECONDS" claim: after heal + a single trigger block,
// the rejoiner must reach the majority's pre-heal finalized tip well inside this window.
const reconvergeTO = 15 * time.Second

// TestMultiNode_PartitionHeal_ReconvergesInSeconds is the end-to-end rejoin proof.
func TestMultiNode_PartitionHeal_ReconvergesInSeconds(t *testing.T) {
	const gap = 35 // majority advances ~35 finalized blocks while node 4 is partitioned

	net := newSimNet(t, 5, prodParams5())
	rejoiner := 4

	// PARTITION node 4 off the majority BEFORE any block: it will miss the entire gap.
	net.down(rejoiner)

	// Advance the majority (nodes 0..3 — the exact α=4 quorum) by `gap` finalized blocks.
	parentID := ids.Empty
	parentRoot := simGenesisRoot()
	var tipID ids.ID
	var tipRoot ids.ID
	for h := uint64(1); h <= gap; h++ {
		blk := newHonestBlock(parentID, parentRoot, h, "reconverge-gap")
		net.build(int(h%4), blk) // rotate the proposer across the up majority
		if !waitFor(emergeTO, func() bool { all, fork := net.finalizedEverywhere(blk); return all && !fork }) {
			t.Fatalf("majority must finalize gap height %d (heads=%v)", h, net.headsAtHeight(h))
		}
		parentID, parentRoot = blk.ID(), blk.stateRoot
		tipID, tipRoot = blk.ID(), blk.stateRoot
	}

	// The rejoiner has finalized NOTHING (it was partitioned the whole time).
	if _, h, ok := net.nodes[rejoiner].rt.FinalizedLedger(); ok && h != 0 {
		t.Fatalf("precondition: rejoiner should be at finalized height 0 before heal, got %d", h)
	}
	majTip, majH := tipID, uint64(gap)
	t.Logf("BEFORE heal: majority finalized tip = height %d (%s); rejoiner = height 0 (partitioned, missed %d blocks)", majH, majTip, gap)

	// HEAL node 4 and TRIGGER re-convergence with ONE new block: the rejoiner receives its
	// gossip, sees the parent chain missing, and its OWN engine drives requestCatchup →
	// busCatchup serves the finalized gap with certs → it finalizes 1..gap, then this block.
	net.nodes[rejoiner].setUp(true)
	trigger := newHonestBlock(tipID, tipRoot, majH+1, "reconverge-trigger")
	net.build(0, trigger)

	start := time.Now()
	ok := waitFor(reconvergeTO, func() bool {
		_, h, set := net.nodes[rejoiner].rt.FinalizedLedger()
		return set && h >= majH // reached (at least) the majority's pre-heal tip
	})
	elapsed := time.Since(start)
	if !ok {
		_, h, _ := net.nodes[rejoiner].rt.FinalizedLedger()
		t.Fatalf("rejoiner did NOT re-converge within %s: still at finalized height %d, majority was %d", reconvergeTO, h, majH)
	}

	// NO FORK: every height the rejoiner finalized must match the majority's canonical head.
	for h := uint64(1); h <= majH; h++ {
		mine, okMine := net.nodes[rejoiner].rt.FinalizedBlockAtHeight(h)
		theirs, okTheirs := net.nodes[0].rt.FinalizedBlockAtHeight(h)
		if okMine && okTheirs && mine != theirs {
			t.Fatalf("FORK: rejoiner finalized %s at height %d, majority finalized %s", mine, h, theirs)
		}
	}

	_, rejH, _ := net.nodes[rejoiner].rt.FinalizedLedger()
	t.Logf("AFTER heal: rejoiner re-converged to finalized height %d in %s (bound %s) — no fork, no restore", rejH, elapsed.Round(time.Millisecond), reconvergeTO)
}
