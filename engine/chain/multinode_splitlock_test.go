// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// multinode_splitlock_test.go — required test 2: the n=5, α=4 DURABLE SPLIT-LOCK across the
// canonical flip, end-to-end on the real concurrent multi-engine harness.
//
// This is the EXACT mainnet-1082880 uncovered state: pre-fix durable view-change locks stored by
// OUTER wrapper id (canonical=outer), split 3/2 across two wrappers (outer-A on nodes 0/3/4,
// outer-B on 1/2) of the SAME inner block I. After the canonical=inner roll the recovered locks
// are never re-canonicalized, so every node prevotes its stale outer alias — the prevotes are
// EMITTED, gossiped, received, and counted (transport is healthy), but split 3/2 they never reach
// α=4, no POL(I) forms, and no node precommits the inner winner → FROZEN.
//
// CONTROL (seeded split-lock, NO migration): the fleet must NOT finalize — the freeze, reproduced.
// FIX (same split-lock + the boot-time canonicalize migration): both wrappers → inner I, so all
// five are locked on I at round 8450, re-broadcast precommit I@8450, assemble one α-of-K cert, and
// finalize — with a SINGLE head (no double-finalize).
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// seedSplitLock installs a stale pre-fix v3 view-change lock at `height` on `outer` (round `round`)
// into node i's guard — exactly as WithVoteGuard seeds a recovered lock at boot, BEFORE the height
// is tracked, so viewForLocked seeds the stale value into the round machine.
func (net *simNet) seedSplitLock(i int, height uint64, outer ids.ID, round uint32) {
	e := net.nodes[i].rt.Transitive
	e.slotMu.Lock()
	e.committedSlot[SlotKey{Height: height}] = outer
	e.lockRounds[height] = round
	e.slotMu.Unlock()
}

// migrateSplitLock runs the boot-time stale-lock migration on node i with an injected resolver
// (models the VM/operator resolving each outer wrapper to the inner canonical). Called with the
// height views still empty (the production boot ordering), so the canonicalized committedSlot is
// what viewForLocked later seeds.
func (net *simNet) migrateSplitLock(t *testing.T, i int, resolver LockCanonicalResolver) LockMigrationReport {
	t.Helper()
	e := net.nodes[i].rt.Transitive
	e.slotMu.Lock()
	defer e.slotMu.Unlock()
	rep, err := e.migrateStaleLocksAboveFloorLocked(resolver, nil, nil)
	if err != nil {
		t.Fatalf("node %d migrate: %v", i, err)
	}
	return rep
}

func TestMultiNode_SplitLock_Migration_ConvergesNoFork(t *testing.T) {
	p := prodParams5()
	p.ViewChange = true

	// -------- CONTROL: seeded 3/2 split-lock, NO migration → FROZEN. --------
	t.Run("control_frozen_without_migration", func(t *testing.T) {
		net := newSimNet(t, 5, p)
		outerA, outerB := ids.GenerateTestID(), ids.GenerateTestID()
		blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "splitlock")
		for _, i := range []int{0, 3, 4} {
			net.seedSplitLock(i, 1, outerA, 8450)
		}
		for _, i := range []int{1, 2} {
			net.seedSplitLock(i, 1, outerB, 8450)
		}
		net.build(0, blk)
		// The split-lock is SAFE but not LIVE: no POL for the winner can form (prevotes split
		// 3/2 across the two stale aliases), so nothing finalizes.
		finalized := waitFor(3*time.Second, func() bool {
			all, _ := net.finalizedEverywhere(blk)
			return all
		})
		if finalized {
			t.Fatal("CONTROL: a 3/2 durable split-lock must FREEZE (no POL(I), no cert) — it must not finalize without the migration")
		}
		// And it must NOT have forked either (safety holds under the freeze).
		if seen := net.headsAtHeight(1); len(seen) > 1 {
			t.Fatalf("CONTROL: split-lock must be SAFE (no fork) even while frozen; heads=%v", seen)
		}
	})

	// -------- FIX: same split-lock + boot migration canonicalizes A,B → I → converge. --------
	t.Run("converges_after_canonicalize_migration", func(t *testing.T) {
		net := newSimNet(t, 5, p)
		outerA, outerB := ids.GenerateTestID(), ids.GenerateTestID()
		blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "splitlock")
		innerI := blk.ID()
		for _, i := range []int{0, 3, 4} {
			net.seedSplitLock(i, 1, outerA, 8450)
		}
		for _, i := range []int{1, 2} {
			net.seedSplitLock(i, 1, outerB, 8450)
		}
		// Boot-time migration: both stale wrappers canonicalize to the SAME inner winner I.
		resolver := mapResolver{outerA: innerI, outerB: innerI, innerI: innerI}
		for i := 0; i < 5; i++ {
			rep := net.migrateSplitLock(t, i, resolver)
			if rep.Stop || !rep.Changed || rep.Entries[0].Kind != lockCanonicalize {
				t.Fatalf("node %d: expected canonicalize, got %+v", i, rep.Entries)
			}
		}
		net.build(0, blk)
		if !waitFor(emergeTO, func() bool {
			all, fork := net.finalizedEverywhere(blk)
			return all && !fork
		}) {
			t.Fatalf("FIX: after the stale-lock migration the 5 nodes must converge+finalize %s; heads=%v",
				innerI, net.headsAtHeight(1))
		}
		// NO double-finalize: exactly one head at the height across the whole fleet.
		if seen := net.headsAtHeight(1); len(seen) != 1 {
			t.Fatalf("FIX: single-head (no double-finalize) required; got %v", seen)
		}
	})
}
