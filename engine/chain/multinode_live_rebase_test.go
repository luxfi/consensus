// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// multinode_live_rebase_test.go — the LIVE (runtime, no operator migration) fix for the mainnet
// 1085761 stale-alias lock freeze, end-to-end on the real concurrent multi-engine harness.
//
// This is the state luxd-4 froze in AFTER the v1.35.36 cert-receive fix let it past the storm: a
// pre-fix durable v3 view-change lock stored by an OUTER proposervm wrapper id, re-seeded by
// viewForLocked, so each node prevotes its stale outer alias — not the inner-canonical winner. Split
// across wrappers of ONE inner block I, the prevotes never reach α=4, no POL(I) forms, and the fleet
// is SAFE but FROZEN. Unlike multinode_splitlock_test.go (which repairs via the boot-time operator
// migration), here the running engine self-heals PER STEP: maybeRebaseStaleLock resolves each stale
// outer lock to its inner and, iff it equals the round winner, rebases the lock onto the winner.
//
// CONTROL (stale split-lock, resolver that resolves NOTHING → the sim's vmCanonicalResolver on
// untracked ids): the fleet must NOT finalize — the freeze, reproduced, with the live rebase inert.
// FIX (same split-lock + a resolver that maps both stale wrappers to inner I): all five rebase to I
// AT RUNTIME, prevote I, form POL(I), assemble one α-of-K cert, and finalize — SINGLE head, no
// double-finalize, no boot migration, no operator action.

package chain

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// wireLiveResolver installs a runtime outer→inner resolver on node i so maybeRebaseStaleLock can
// re-canonicalize a stale lock without the VM store the sim lacks. Production wires the default
// vmCanonicalResolver (nil here); this is the test seam, the live analogue of the migration's
// injected LockCanonicalResolver.
func (net *simNet) wireLiveResolver(i int, resolver LockCanonicalResolver) {
	net.nodes[i].rt.lockResolver = resolver
}

func TestMultiNode_LiveStaleLockRebase_ConvergesNoFork(t *testing.T) {
	p := prodParams5()
	p.ViewChange = true

	// -------- CONTROL: stale 3/2 split-lock, live rebase INERT (nothing resolves) → FROZEN. --------
	t.Run("control_frozen_when_unresolvable", func(t *testing.T) {
		net := newSimNet(t, 5, p)
		outerA, outerB := ids.GenerateTestID(), ids.GenerateTestID()
		blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "liverebase")
		for _, i := range []int{0, 3, 4} {
			net.seedSplitLock(i, 1, outerA, 8450)
		}
		for _, i := range []int{1, 2} {
			net.seedSplitLock(i, 1, outerB, 8450)
		}
		// No live resolver wired: the default vmCanonicalResolver cannot resolve the untracked stale
		// outer ids in the sim (no VM block), so the live rebase is a no-op — the freeze stands.
		net.build(0, blk)
		if waitFor(3*time.Second, func() bool { all, _ := net.finalizedEverywhere(blk); return all }) {
			t.Fatal("CONTROL: an unresolvable 3/2 stale split-lock must FREEZE (no POL(I), no cert)")
		}
		if seen := net.headsAtHeight(1); len(seen) > 1 {
			t.Fatalf("CONTROL: the frozen split-lock must remain SAFE (no fork); heads=%v", seen)
		}
	})

	// -------- FIX: same split-lock + a live resolver mapping both wrappers → inner I → converge. --------
	t.Run("converges_via_live_rebase_no_migration", func(t *testing.T) {
		net := newSimNet(t, 5, p)
		outerA, outerB := ids.GenerateTestID(), ids.GenerateTestID()
		blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "liverebase")
		innerI := blk.ID()
		for _, i := range []int{0, 3, 4} {
			net.seedSplitLock(i, 1, outerA, 8450)
		}
		for _, i := range []int{1, 2} {
			net.seedSplitLock(i, 1, outerB, 8450)
		}
		// Wire the LIVE resolver on every node: both stale wrappers resolve to the SAME inner winner
		// I (the fixpoint m[I]=I keeps a rebased/fresh lock idempotent). NO boot migration is run —
		// the running engine rebases each stale lock at step time.
		resolver := mapResolver{outerA: innerI, outerB: innerI, innerI: innerI}
		for i := 0; i < 5; i++ {
			net.wireLiveResolver(i, resolver)
		}
		net.build(0, blk)
		if !waitFor(emergeTO, func() bool {
			all, fork := net.finalizedEverywhere(blk)
			return all && !fork
		}) {
			t.Fatalf("FIX: the running fleet must LIVE-rebase the stale split-lock and finalize %s; heads=%v",
				innerI, net.headsAtHeight(1))
		}
		if seen := net.headsAtHeight(1); len(seen) != 1 {
			t.Fatalf("FIX: single-head (no double-finalize) required; got %v", seen)
		}
	})
}
