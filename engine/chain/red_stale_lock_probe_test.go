// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// red_stale_lock_probe_test.go — RED adversarial probes against the stale-lock migration
// (lock_migration.go: planLockMigration / migrateStaleLocksAboveFloorLocked, wired by the node
// as MigrateStaleLocks behind LUX_CONSENSUS_MIGRATE_STALE_LOCKS=apply:<floor>). Originally these
// probes PROVED red's findings R1/R2/R4; after the fixes landed they were inverted to PIN the
// fixed behavior. Reuses helpers from lock_migration_test.go (runMigrate, mapResolver,
// committedAt) and reconcile_test.go (seedPhantomEngine).
//
//   R1 — prune discards the round-scoped lock (safety-relevant in NORMAL crash-restart, fine for
//        the incident where the wrapper bytes are gone) → apply is operator-targeted
//        (apply:<floor>) and SELF-DISARMS when the live floor moves (TestRedStaleLock_SelfDisarm).
//   R4 — the migration now DELETES the cached t.views entry for every migrated height, so a
//        runtime invocation re-seeds from the rewritten binding (TestRedStaleLock_MigrationLeavesStaleView).
//   R2 — certAt now consults DURABLE finality (the consensus ledger), engaging exactly at the
//        boot invocation point where views is empty (TestRedStaleLock_BootCertGuardVacuous).
//   R3 — vmCanonicalResolver height-binds resolution (TestRedStaleLock_ResolverHeightGuard).

package chain

import (
	"context"
	"testing"
	"time"

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

// TestRedStaleLock_MigrationLeavesStaleView pins the R4 FIX: the migration must invalidate a
// cached view for every migrated height, so a RUNTIME invocation of the public MigrateStaleLocks
// (views non-empty) re-seeds the round machine from the REWRITTEN binding instead of leaving it
// locked on the pre-migration id (the re-freeze red originally proved here).
func TestRedStaleLock_MigrationLeavesStaleView(t *testing.T) {
	const floor = uint64(1082879)
	const H = floor + 1
	outerA, innerI := ids.GenerateTestID(), ids.GenerateTestID()
	e, _, _ := seedPhantomEngine(t, floor,
		map[SlotKey]ids.ID{{Height: H}: outerA}, map[uint64]uint32{H: 8450})

	// A view already exists at H (models a RUNTIME invocation — NOT the boot-before-Start path).
	e.slotMu.Lock()
	pre := e.viewForLocked(H, 4, 5)
	preBlock := pre.lockBlock
	e.slotMu.Unlock()
	if preBlock != outerA {
		t.Fatalf("precondition: cached view must lock the stale outer %s, got %s", outerA, preBlock)
	}

	rep := runMigrate(t, e, mapResolver{outerA: innerI, innerI: innerI}, nil, nil)
	if rep.Entries[0].Kind != lockCanonicalize {
		t.Fatalf("expected canonicalize, got %+v", rep.Entries)
	}
	if got, _ := committedAt(e, H); got != innerI {
		t.Fatalf("binding must be rewritten to inner I, got %s", got)
	}
	// R4 FIX: the migration dropped the cached view; the next viewForLocked re-seeds from the
	// REWRITTEN binding — locked on inner I with the round preserved.
	e.slotMu.Lock()
	_, cached := e.views[H]
	v := e.viewForLocked(H, 4, 5)
	locked, round, block := v.haveLocked, v.lockRound, v.lockBlock
	e.slotMu.Unlock()
	if cached {
		t.Fatalf("R4: the migration must DELETE the cached view at %d (it still holds the pre-migration lock)", H)
	}
	if !locked || round != 8450 || block != innerI {
		t.Fatalf("re-seeded view must lock inner I@8450, got locked=%v round=%d block=%s", locked, round, block)
	}
	t.Logf("FIXED R4: canonicalize dropped the cached view; re-seed locks inner %s@8450 — a runtime "+
		"MigrateStaleLocks can no longer re-freeze on a stale cached view.", innerI)
}

// TestRedStaleLock_BootCertGuardVacuous pins the R2 FIX: certAt consults DURABLE finality (the
// consensus finality ledger), not just the in-session views map that is empty at the boot
// invocation point — so the S4/cert gate now engages exactly when the migration runs.
func TestRedStaleLock_BootCertGuardVacuous(t *testing.T) {
	const floor = uint64(1082879)
	const H = floor + 1
	outerA := ids.GenerateTestID()
	e, _, _ := seedPhantomEngine(t, floor,
		map[SlotKey]ids.ID{{Height: H}: outerA}, map[uint64]uint32{H: 8450})

	rt := &Runtime{Transitive: e}

	// (a) No durable finality, empty views → false (H > floor is genuinely unfinalized).
	_, certAt := rt.engineCanonicalContext()
	if certAt(H) {
		t.Fatal("with no durable finality and empty views, certAt must be false")
	}

	// (b) R2 FIX: a CERTIFIED ledger entry at H — the durable α-of-K finalization record — must
	// flip certAt(H) true even with the views map empty (the boot invocation point), forcing the
	// planner to STOP rather than disturb a finalized height.
	e.consensus = NewChainConsensus(5, 4, 1)
	e.consensus.ledger = seedLedger(outerA, outerA, H)
	_, certAt = rt.engineCanonicalContext()
	if !certAt(H) {
		t.Fatalf("R2: certAt(%d) must consult the DURABLE finality ledger (views empty at boot)", H)
	}
	rep := runMigrate(t, e, mapResolver{}, certAt, nil)
	if !rep.Stop {
		t.Fatal("R2: a durable cert at the locked height must STOP the migration (no write)")
	}
	t.Logf("FIXED R2: certAt(%d)=true from the durable finality ledger with empty views; migration STOPs. "+
		"The cert gate is a real boot-time protection, not vacuous.", H)
}

// TestRedStaleLock_SelfDisarm pins the R1 FIX: apply carries the operator's observed floor target;
// a live floor that has MOVED past it (the incident is over) refuses the apply as a no-op — a
// lingering apply env can never degrade into an every-boot prune of genuine crash locks.
func TestRedStaleLock_SelfDisarm(t *testing.T) {
	const floor = uint64(1082879)
	const H = floor + 1
	outerGone := ids.GenerateTestID()
	e, _, path := seedPhantomEngine(t, floor,
		map[SlotKey]ids.ID{{Height: H}: outerGone}, map[uint64]uint32{H: 8450})

	rt := &Runtime{Transitive: e}
	// Operator armed apply:1082878 (a STALE target — the live floor is 1082879).
	rep, err := rt.MigrateStaleLocks(context.Background(), floor-1)
	if err != nil {
		t.Fatalf("MigrateStaleLocks: %v", err)
	}
	if !rep.Skipped || rep.Changed {
		t.Fatalf("R1: floor mismatch must self-disarm (Skipped, no write), got skipped=%v changed=%v", rep.Skipped, rep.Changed)
	}
	// Nothing written: the durable binding + lock survive untouched.
	store, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := store.Snapshot()[SlotKey{Height: H}]; got != outerGone {
		t.Fatalf("self-disarm must write NOTHING: durable lock@%d = %s, want untouched %s", H, got, outerGone)
	}
	if ft := store.FinalizedThrough(); ft != floor {
		t.Fatalf("self-disarm must not move the floor: got %d, want %d", ft, floor)
	}
	t.Logf("FIXED R1: apply target %d ≠ live floor %d → Skipped, zero writes, floor intact. A stale "+
		"LUX_CONSENSUS_MIGRATE_STALE_LOCKS=apply:<h> env is inert on later boots.", floor-1, floor)
}

// TestRedStaleLock_ResolverHeightGuard pins the R3 FIX: the production vmCanonicalResolver refuses
// to resolve an id whose block lives at a DIFFERENT height than the queried lock height — a
// cross-height id can never rewrite a slot binding to another height's canonical.
func TestRedStaleLock_ResolverHeightGuard(t *testing.T) {
	const floor = uint64(1082879)
	const H = floor + 1
	outerA, innerI := ids.GenerateTestID(), ids.GenerateTestID()
	e, _, _ := seedPhantomEngine(t, floor,
		map[SlotKey]ids.ID{{Height: H}: outerA}, map[uint64]uint32{H: 8450})

	rt := &Runtime{Transitive: e}
	resolver := vmCanonicalResolver{ctx: context.Background(), rt: rt}

	// Track outerA's block at the WRONG height (H+5): the resolver must refuse it for H.
	e.mu.Lock()
	e.pendingBlocks[outerA] = &PendingBlock{
		ConsensusBlock: &Block{id: outerA, height: H + 5, canonicalID: innerI},
		ProposedAt:     time.Now(),
	}
	e.mu.Unlock()
	if canon, ok := resolver.CanonicalOf(outerA, H); ok {
		t.Fatalf("R3: resolver must refuse a block at height %d when asked for height %d (got %s)", H+5, H, canon)
	}
	// Same block queried at ITS OWN height resolves fine.
	if canon, ok := resolver.CanonicalOf(outerA, H+5); !ok || canon != innerI {
		t.Fatalf("resolver must resolve at the block's own height, got %s ok=%v", canon, ok)
	}
	t.Logf("FIXED R3: vmCanonicalResolver height-binds resolution — cross-height ids are unresolvable.")
}
