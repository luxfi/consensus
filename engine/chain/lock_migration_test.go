// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// lock_migration_test.go — the teeth of the STALE OUTER-WRAPPER LOCK MIGRATION (lock_migration.go).
//
// It reproduces the EXACT uncovered state the mainnet C-Chain froze in: durable view-change locks
// written by OUTER wrapper id (pre-fix canonical=outer) that survive the canonical=inner roll and
// are never re-canonicalized, so nodes stay locked on stale aliases and refuse to prevote the
// inner winner → no POL → frozen. The existing Part A/B tests only cover FRESH convergence; these
// cover the persisted-state MIGRATION across the canonical flip.
//
// The 6 required tests (owner charter): (1) stale outer-lock canonicalize + prune fallback; (2)
// n=5 α=4 SPLIT stale-lock → prevote-silence pre-fix, converge post-migration (multinode, in
// multinode_splitlock_test.go); (3) never touch finalized state ≤ floor; (4) never erase a real
// α=4 cert (STOP); (5) DIFFERENT inner canonicals → STOP (no merge); (6) idempotence.

package chain

import (
	"testing"

	"github.com/luxfi/ids"
)

// mapResolver is a deterministic outer→inner canonical resolver for the migration tests. A
// fixpoint entry (m[I]=I) models an already-canonical id, which is what makes the migration
// idempotent on re-run. It ignores the height binding (the production vmCanonicalResolver
// enforces it — see TestLockMigration_ResolverHeightGuard).
type mapResolver map[ids.ID]ids.ID

func (m mapResolver) CanonicalOf(id ids.ID, _ uint64) (ids.ID, bool) {
	c, ok := m[id]
	return c, ok
}

// runMigrate drives the engine-core migration under slotMu with injected fakes and returns the
// applied report. certAt/candidatesAt default to "no cert / no extra candidates" when nil.
func runMigrate(t *testing.T, e *Transitive, resolver LockCanonicalResolver, certAt func(uint64) bool, candidatesAt func(uint64) []ids.ID) LockMigrationReport {
	t.Helper()
	e.slotMu.Lock()
	defer e.slotMu.Unlock()
	rep, err := e.migrateStaleLocksAboveFloorLocked(resolver, certAt, candidatesAt)
	if err != nil {
		t.Fatalf("migrateStaleLocksAboveFloorLocked: %v", err)
	}
	return rep
}

func committedAt(e *Transitive, h uint64) (ids.ID, bool) {
	e.slotMu.Lock()
	defer e.slotMu.Unlock()
	v, ok := e.committedSlot[SlotKey{Height: h}]
	return v, ok
}

// -----------------------------------------------------------------------------
// TEST 1 — stale outer-lock canonicalize, and the prune fallback.
// -----------------------------------------------------------------------------

func TestLockMigration_StaleOuterLock_CanonicalizesToInner(t *testing.T) {
	floor := uint64(1082879)
	outerA := ids.GenerateTestID() // the stale pre-fix wrapper id
	innerI := ids.GenerateTestID() // the inner canonical
	// A v3 lock (lockRound present) at height floor+1 on the OUTER wrapper — the mainnet shape.
	e, _, path := seedPhantomEngine(t, floor,
		map[SlotKey]ids.ID{{Height: floor + 1}: outerA},
		map[uint64]uint32{floor + 1: 8450},
	)

	resolver := mapResolver{outerA: innerI, innerI: innerI}
	rep := runMigrate(t, e, resolver, nil, nil)

	if rep.Stop {
		t.Fatalf("unexpected STOP: %s", rep.StopReason)
	}
	if !rep.Changed || len(rep.Entries) != 1 || rep.Entries[0].Kind != lockCanonicalize {
		t.Fatalf("expected one canonicalize entry, got changed=%v entries=%+v", rep.Changed, rep.Entries)
	}
	// In-memory: the lock is now the INNER canonical, round preserved.
	if got, ok := committedAt(e, floor+1); !ok || got != innerI {
		t.Fatalf("in-memory lock@%d = %s (ok=%v), want inner %s", floor+1, got, ok, innerI)
	}
	// Durable: re-open the vote-guard file and confirm the rewrite persisted, floor UNCHANGED.
	store, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := store.Snapshot()[SlotKey{Height: floor + 1}]; got != innerI {
		t.Fatalf("durable lock@%d = %s, want inner %s", floor+1, got, innerI)
	}
	if got := store.LockRoundsSnapshot()[floor+1]; got != 8450 {
		t.Fatalf("durable lock round@%d = %d, want 8450 (preserved)", floor+1, got)
	}
	if got := store.FinalizedThrough(); got != floor {
		t.Fatalf("durable floor = %d, want %d UNCHANGED (condition 1)", got, floor)
	}
}

func TestLockMigration_Unresolvable_PrunesAboveFloor(t *testing.T) {
	floor := uint64(1082879)
	outerGone := ids.GenerateTestID() // block bytes gone — unresolvable
	e, _, path := seedPhantomEngine(t, floor,
		map[SlotKey]ids.ID{{Height: floor + 1}: outerGone},
		map[uint64]uint32{floor + 1: 8450},
	)

	rep := runMigrate(t, e, mapResolver{ /* resolves nothing */ }, nil, nil)
	if rep.Stop || !rep.Changed || rep.Entries[0].Kind != lockPrune {
		t.Fatalf("expected one prune entry, got %+v", rep.Entries)
	}
	if _, ok := committedAt(e, floor+1); ok {
		t.Fatalf("lock@%d must be PRUNED (node boots unlocked)", floor+1)
	}
	store, _ := OpenVoteGuard(path)
	if _, ok := store.Snapshot()[SlotKey{Height: floor + 1}]; ok {
		t.Fatalf("durable lock@%d must be pruned", floor+1)
	}
	if got := store.FinalizedThrough(); got != floor {
		t.Fatalf("durable floor = %d, want %d UNCHANGED", got, floor)
	}
}

// A sibling wrapper recovers the inner even when the persisted lock id itself is unresolvable —
// canonicalize wins over prune when ANY candidate resolves.
func TestLockMigration_SiblingWrapperRecoversInner(t *testing.T) {
	floor := uint64(1082879)
	outerA := ids.GenerateTestID() // persisted lock — its own bytes gone
	outerB := ids.GenerateTestID() // a tracked sibling wrapper of the same inner
	innerI := ids.GenerateTestID()
	e, _, _ := seedPhantomEngine(t, floor,
		map[SlotKey]ids.ID{{Height: floor + 1}: outerA},
		map[uint64]uint32{floor + 1: 8450},
	)
	resolver := mapResolver{outerB: innerI, innerI: innerI} // outerA NOT resolvable; outerB is
	candidatesAt := func(h uint64) []ids.ID {
		if h == floor+1 {
			return []ids.ID{outerB}
		}
		return nil
	}
	rep := runMigrate(t, e, resolver, nil, candidatesAt)
	if rep.Stop || rep.Entries[0].Kind != lockCanonicalize {
		t.Fatalf("expected canonicalize via sibling, got %+v", rep.Entries)
	}
	if got, _ := committedAt(e, floor+1); got != innerI {
		t.Fatalf("lock@%d = %s, want inner %s (recovered from sibling wrapper)", floor+1, got, innerI)
	}
}

// -----------------------------------------------------------------------------
// TEST 3 — never touch finalized state AT OR BELOW the floor.
// -----------------------------------------------------------------------------

func TestLockMigration_NeverTouchesAtOrBelowFloor(t *testing.T) {
	floor := uint64(1082879)
	below1, below2 := ids.GenerateTestID(), ids.GenerateTestID()
	outerA, innerI := ids.GenerateTestID(), ids.GenerateTestID()
	// Bindings at/below the floor (finalized window, kept for cert-catch-up) + one stale above.
	e, _, path := seedPhantomEngine(t, floor,
		map[SlotKey]ids.ID{
			{Height: floor - 1}: below1,
			{Height: floor}:     below2,
			{Height: floor + 1}: outerA,
		},
		map[uint64]uint32{floor - 1: 3, floor: 4, floor + 1: 8450},
	)
	resolver := mapResolver{outerA: innerI, innerI: innerI}
	rep := runMigrate(t, e, resolver, nil, nil)
	if rep.Stop {
		t.Fatalf("unexpected STOP: %s", rep.StopReason)
	}
	// Only ONE entry (the >floor lock) is ever in the plan — ≤floor is invisible to the migration.
	if len(rep.Entries) != 1 || rep.Entries[0].Height != floor+1 {
		t.Fatalf("plan must contain ONLY the >floor lock, got %+v", rep.Entries)
	}
	store, _ := OpenVoteGuard(path)
	snap := store.Snapshot()
	if snap[SlotKey{Height: floor - 1}] != below1 || snap[SlotKey{Height: floor}] != below2 {
		t.Fatalf("≤floor bindings MUST be preserved verbatim (condition 5/6); got %v", snap)
	}
	if store.LockRoundsSnapshot()[floor-1] != 3 || store.LockRoundsSnapshot()[floor] != 4 {
		t.Fatalf("≤floor lock rounds MUST be preserved verbatim")
	}
	if snap[SlotKey{Height: floor + 1}] != innerI {
		t.Fatalf(">floor lock should be canonicalized to inner")
	}
}

// -----------------------------------------------------------------------------
// TEST 4 — never disturb a height that already holds an α-of-K cert (STOP).
// -----------------------------------------------------------------------------

func TestLockMigration_CertExists_Stops_NoWrite(t *testing.T) {
	floor := uint64(1082879)
	outerA, innerI := ids.GenerateTestID(), ids.GenerateTestID()
	e, _, path := seedPhantomEngine(t, floor,
		map[SlotKey]ids.ID{{Height: floor + 1}: outerA},
		map[uint64]uint32{floor + 1: 8450},
	)
	certAt := func(h uint64) bool { return h == floor+1 } // a real cert exists here
	rep := runMigrate(t, e, mapResolver{outerA: innerI, innerI: innerI}, certAt, nil)
	if !rep.Stop {
		t.Fatalf("a height with an α-of-K cert MUST force STOP")
	}
	if rep.Changed {
		t.Fatalf("STOP must perform NO write")
	}
	// The lock is untouched (still the outer alias) — nothing was erased.
	if got, _ := committedAt(e, floor+1); got != outerA {
		t.Fatalf("lock@%d = %s, want UNCHANGED %s (no cert erasure)", floor+1, got, outerA)
	}
	store, _ := OpenVoteGuard(path)
	if store.Snapshot()[SlotKey{Height: floor + 1}] != outerA {
		t.Fatalf("durable lock MUST be untouched on STOP")
	}
}

// -----------------------------------------------------------------------------
// TEST 5 — two wrappers at one height canonicalize to DIFFERENT inner blocks → STOP.
// -----------------------------------------------------------------------------

func TestLockMigration_DivergentInners_Stops_NoMerge(t *testing.T) {
	floor := uint64(1082879)
	outerA, outerB := ids.GenerateTestID(), ids.GenerateTestID()
	innerI, innerJ := ids.GenerateTestID(), ids.GenerateTestID() // DIFFERENT inner blocks — a real fork
	e, _, path := seedPhantomEngine(t, floor,
		map[SlotKey]ids.ID{{Height: floor + 1}: outerA},
		map[uint64]uint32{floor + 1: 8450},
	)
	resolver := mapResolver{outerA: innerI, outerB: innerJ, innerI: innerI, innerJ: innerJ}
	candidatesAt := func(h uint64) []ids.ID {
		if h == floor+1 {
			return []ids.ID{outerB} // a sibling that resolves to a DIFFERENT inner
		}
		return nil
	}
	rep := runMigrate(t, e, resolver, nil, candidatesAt)
	if !rep.Stop || rep.Changed {
		t.Fatalf("divergent inner canonicals MUST STOP with no write; got stop=%v changed=%v", rep.Stop, rep.Changed)
	}
	if got, _ := committedAt(e, floor+1); got != outerA {
		t.Fatalf("on divergence STOP the lock MUST be untouched, got %s", got)
	}
	store, _ := OpenVoteGuard(path)
	if store.Snapshot()[SlotKey{Height: floor + 1}] != outerA {
		t.Fatalf("durable state MUST be untouched on divergence STOP")
	}
}

// -----------------------------------------------------------------------------
// TEST 6 — idempotence: run twice = same state, floor never lowers.
// -----------------------------------------------------------------------------

func TestLockMigration_Idempotent(t *testing.T) {
	floor := uint64(1082879)
	outerA, innerI := ids.GenerateTestID(), ids.GenerateTestID()
	e, _, path := seedPhantomEngine(t, floor,
		map[SlotKey]ids.ID{{Height: floor + 1}: outerA},
		map[uint64]uint32{floor + 1: 8450},
	)
	resolver := mapResolver{outerA: innerI, innerI: innerI} // fixpoint on the inner

	rep1 := runMigrate(t, e, resolver, nil, nil)
	if rep1.Stop || !rep1.Changed {
		t.Fatalf("first run should canonicalize, got %+v", rep1)
	}
	// Second run on the already-canonicalized state: lock == inner → keep → NO change.
	rep2 := runMigrate(t, e, resolver, nil, nil)
	if rep2.Stop {
		t.Fatalf("second run STOP: %s", rep2.StopReason)
	}
	if rep2.Changed {
		t.Fatalf("second run MUST be a no-op (idempotent); got changed=true entries=%+v", rep2.Entries)
	}
	if rep2.Entries[0].Kind != lockKeep {
		t.Fatalf("second run entry should be keep, got %s", rep2.Entries[0].Kind)
	}
	if got, _ := committedAt(e, floor+1); got != innerI {
		t.Fatalf("state after 2nd run = %s, want %s (unchanged)", got, innerI)
	}
	store, _ := OpenVoteGuard(path)
	if store.FinalizedThrough() != floor {
		t.Fatalf("floor MUST never lower across runs; got %d want %d", store.FinalizedThrough(), floor)
	}
}

// -----------------------------------------------------------------------------
// MECHANISM (Repro B) — the pure round_view proof that a stale outer lock SUPPRESSES the winner
// prevote (the freeze), and that clearing/canonicalizing it restores it.
// -----------------------------------------------------------------------------

func TestRoundView_StaleOuterLock_SuppressesWinnerPrevote(t *testing.T) {
	winnerI := ids.GenerateTestID()    // the inner canonical winner (unanimous)
	staleOuter := ids.GenerateTestID() // a DIFFERENT (stale) outer wrapper id this node is locked on

	// A node locked on the stale OUTER at round 8450, stepping with winner = inner I.
	locked := newRoundView(1082880, 4, 5)
	locked.haveLocked = true
	locked.lockRound = 8450
	locked.lockBlock = staleOuter
	// Advance the machine's round above the lock so it is genuinely "stuck high" like mainnet.
	locked.round = 8460
	if got := locked.prevoteTarget(winnerI); got != staleOuter {
		t.Fatalf("locked-on-stale-outer node prevoteTarget = %s, want the STALE lock %s "+
			"(this is the silence: it never prevotes winner %s, so no POL(I) forms)", got, staleOuter, winnerI)
	}

	// After canonicalization the lock value == winner I: the node prevotes I.
	canon := newRoundView(1082880, 4, 5)
	canon.haveLocked = true
	canon.lockRound = 8450
	canon.lockBlock = winnerI // canonicalized outer→inner
	canon.round = 8460
	if got := canon.prevoteTarget(winnerI); got != winnerI {
		t.Fatalf("canonicalized-lock node prevoteTarget = %s, want winner %s", got, winnerI)
	}

	// After a prune the node is UNLOCKED: it prevotes the fresh winner.
	unlocked := newRoundView(1082880, 4, 5)
	unlocked.round = 8460
	if got := unlocked.prevoteTarget(winnerI); got != winnerI {
		t.Fatalf("unlocked node prevoteTarget = %s, want winner %s", got, winnerI)
	}
}
