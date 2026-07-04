// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// red_stale_lock_probe_test.go — RED adversarial probes against the SURVIVING stale-lock
// migration (lock_migration.go: planLockMigration / migrateStaleLocksAboveFloorLocked, wired
// by the node as MigrateStaleLocks behind LUX_CONSENSUS_MIGRATE_STALE_LOCKS). Blue's earlier
// duplicate stale_lock.go was removed during the session; these probes target what actually
// runs. They do NOT modify any non-test file. Reuses helpers from lock_migration_test.go
// (runMigrate, mapResolver, committedAt) and reconcile_test.go (seedPhantomEngine).
//
//   R1 — PRUNE resets the height to UNLOCKED@round0, discarding the v3 round-scoped lock the
//        ViewChange safety proof (Tendermint 2α−n>f) depends on; CANONICALIZE (VM-resolver
//        path) keeps it. Drives two rollout requirements: InspectLocks must show canonicalize
//        (VM store resolves) before apply, and apply MUST be a ONE-SHOT (env unset after).
//   R4 — the migration rewrites committedSlot/lockRounds but does NOT invalidate a cached
//        t.views entry. Correct ONLY under the boot-before-Start ordering (views empty). A
//        runtime invocation of the public MigrateStaleLocks leaves a stale view locked on the
//        old id. Blue's removed stale_lock.go deleted the view on migrate; this path lost that.
//   R2 — the wired certAt guard consults only in-session t.views, which is EMPTY at boot, so it
//        is a no-op exactly when the migration runs (redundant with the floor invariant S1, but
//        the comment overstates it).

package chain

import (
	"testing"

	"github.com/luxfi/ids"
)

func TestRedStaleLock_PruneUnlocksVsCanonicalizeKeepsLock(t *testing.T) {
	const floor = uint64(1082879)
	const H = floor + 1
	const lockRound = uint32(8450)

	t.Run("unresolvable_prune_boots_UNLOCKED_round0", func(t *testing.T) {
		outerGone := ids.GenerateTestID()
		e, _, _ := seedPhantomEngine(t, floor,
			map[SlotKey]ids.ID{{Height: H}: outerGone}, map[uint64]uint32{H: lockRound})

		// PRECONDITION: the recovered v3 lock makes viewForLocked seed a LOCKED view on the stale
		// outer id at round 8450 — the crash-restart protection. Drop the cached view so the
		// migration re-seeds from the (about-to-be-pruned) binding, as the boot path does.
		e.slotMu.Lock()
		pre := e.viewForLocked(H, 4, 5)
		lp, rp, bp := pre.haveLocked, pre.lockRound, pre.lockBlock
		delete(e.views, H)
		e.slotMu.Unlock()
		if !lp || rp != lockRound || bp != outerGone {
			t.Fatalf("precondition: view must be LOCKED on outer@%d, got locked=%v round=%d block=%s", lockRound, lp, rp, bp)
		}

		rep := runMigrate(t, e, mapResolver{ /* resolves nothing */ }, nil, nil)
		if rep.Stop || len(rep.Entries) != 1 || rep.Entries[0].Kind != lockPrune {
			t.Fatalf("expected one prune entry, got %+v (stop=%v)", rep.Entries, rep.Stop)
		}

		// FINDING R1: the height now boots UNLOCKED at round 0 — the round-8450 lock is gone.
		e.slotMu.Lock()
		post := e.viewForLocked(H, 4, 5)
		locked, round := post.haveLocked, post.round
		e.slotMu.Unlock()
		if locked {
			t.Fatalf("post-prune view unexpectedly LOCKED (round=%d)", round)
		}
		if round != 0 {
			t.Fatalf("post-prune view must restart at round 0, got %d", round)
		}
		t.Logf("CONFIRMED R1: unresolvable PRUNE discarded the round-%d lock → height %d boots UNLOCKED@round0. "+
			"Mainnet apply MUST resolve via the VM store (canonicalize, not prune) AND be a one-shot; wired "+
			"every-boot, prune re-opens the HIGH-1 crash-restart double-precommit the vote-guard prevents.", lockRound, H)
	})

	t.Run("resolvable_canonicalize_KEEPS_lock_on_inner", func(t *testing.T) {
		outerA, innerI := ids.GenerateTestID(), ids.GenerateTestID()
		e, _, _ := seedPhantomEngine(t, floor,
			map[SlotKey]ids.ID{{Height: H}: outerA}, map[uint64]uint32{H: lockRound})

		rep := runMigrate(t, e, mapResolver{outerA: innerI, innerI: innerI}, nil, nil)
		if rep.Stop || len(rep.Entries) != 1 || rep.Entries[0].Kind != lockCanonicalize {
			t.Fatalf("expected one canonicalize entry, got %+v", rep.Entries)
		}
		e.slotMu.Lock()
		v := e.viewForLocked(H, 4, 5)
		locked, round, block := v.haveLocked, v.lockRound, v.lockBlock
		e.slotMu.Unlock()
		if !locked || round != lockRound || block != innerI {
			t.Fatalf("canonicalize must KEEP the lock on inner I@%d, got locked=%v round=%d block=%s", lockRound, locked, round, block)
		}
		t.Logf("CONTRAST: canonicalize kept height %d LOCKED on inner I@%d — safety preserved. This is the "+
			"VM-resolver path the node wires (MigrateStaleLocks), and why prune is only a fallback.", H, lockRound)
	})
}

// TestRedStaleLock_MigrationLeavesStaleView shows the surviving migration does not invalidate a
// cached view — correct only under boot-before-Start (views empty). A runtime invocation of the
// public MigrateStaleLocks would leave the round machine locked on the pre-migration id.
func TestRedStaleLock_MigrationLeavesStaleView(t *testing.T) {
	const floor = uint64(1082879)
	const H = floor + 1
	outerA, innerI := ids.GenerateTestID(), ids.GenerateTestID()
	e, _, _ := seedPhantomEngine(t, floor,
		map[SlotKey]ids.ID{{Height: H}: outerA}, map[uint64]uint32{H: 8450})

	// A view already exists at H (models a RUNTIME invocation — NOT the boot-before-Start path).
	e.slotMu.Lock()
	_ = e.viewForLocked(H, 4, 5)
	e.slotMu.Unlock()

	rep := runMigrate(t, e, mapResolver{outerA: innerI, innerI: innerI}, nil, nil)
	if rep.Entries[0].Kind != lockCanonicalize {
		t.Fatalf("expected canonicalize, got %+v", rep.Entries)
	}
	if got, _ := committedAt(e, H); got != innerI {
		t.Fatalf("binding must be rewritten to inner I, got %s", got)
	}
	// The CACHED view still targets the STALE outer id — the migration did not invalidate it.
	e.slotMu.Lock()
	v := e.viewForLocked(H, 4, 5)
	staleBlock := v.lockBlock
	e.slotMu.Unlock()
	if staleBlock != outerA {
		t.Fatalf("expected the cached view to RETAIN the stale outer %s (bug), got %s", outerA, staleBlock)
	}
	t.Logf("CONFIRMED R4: after canonicalize, committedSlot=%s (inner) but the CACHED roundView still locks "+
		"the stale outer %s. Correct only when views are empty (boot-before-Start). The public MigrateStaleLocks "+
		"does not enforce that ordering, and unlike the removed stale_lock.go it does not delete t.views on migrate.", innerI, outerA)
}

// TestRedStaleLock_BootCertGuardVacuous pins that the wired certAt closure (t.views based) is
// false at boot because views is empty — so the S4/cert gate cannot engage when the migration runs.
func TestRedStaleLock_BootCertGuardVacuous(t *testing.T) {
	const floor = uint64(1082879)
	const H = floor + 1
	outerA := ids.GenerateTestID()
	e, _, _ := seedPhantomEngine(t, floor,
		map[SlotKey]ids.ID{{Height: H}: outerA}, map[uint64]uint32{H: 8450})

	rt := &Runtime{Transitive: e}
	_, certAt := rt.engineCanonicalContext()
	if certAt(H) {
		t.Fatal("precondition: with an empty views map the wired certAt must be false")
	}
	t.Logf("CONFIRMED R2: the wired certAt(%d)=false at boot (views empty). The stale-lock cert gate consults "+
		"only in-session t.views, never durable finality — a no-op exactly when MigrateStaleLocks runs. Harmless "+
		"only because H>decidedFloor already implies unfinalized (S1); the fix is to snapshot finalized heights "+
		"under the t.mu.RLock engineCanonicalContext already holds.", H)
}
