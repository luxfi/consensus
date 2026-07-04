// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// lock_migration.go — STALE OUTER-WRAPPER LOCK MIGRATION: the one-shot, safety-gated repair of
// pre-fix durable view-change locks after the canonical=inner fix (Part A/B, v1.34.14).
//
// THE INCIDENT (mainnet C-Chain, frozen at 1082879, 2026-07-04). Before the canonical fix a
// node's durable view-change lock (committedSlot[height] = the value it precommitted) was stored
// by the block's canonical AT STORE TIME — and pre-fix that canonical fell back to the OUTER
// proposervm wrapper id (canonicalCommitter was unimplemented, so canonicalIDOf returned the
// envelope). Many outer wrappers of ONE inner block therefore produced MANY distinct lock values.
// At height 1082880 the fleet ended split across two of them: luxd-0/3/4 locked on outer-A
// (8ad2d23c…), luxd-1/2 on outer-B (80a457cc…) — TWO wrappers of the SAME inner block I.
//
// After the roll to v1.34.14 (canonical=inner) the round-scoped view machine runs in canonical
// space: it computes winnerCanon = I and steps on it. BUT viewForLocked (round_view.go) seeds the
// recovered lock's value VERBATIM from committedSlot — the stale OUTER id — and NEVER
// re-canonicalizes it. Under the Tendermint lock rule (prevoteTarget) a locked node prevotes its
// lock value UNLESS a POL forms for a different value at a round strictly above the lock round.
// So every node prevotes its stale outer alias (8ad2 or 80a457), the prevotes split 3/2 across
// two values, neither reaches α=4, no POL(I) ever forms, no precommit, no cert — FROZEN. The vote
// transport is HEALTHY (a 3/2 durable split-lock could not have formed without working
// emit/gossip/receive/count at round 8450); the freeze is pure stale-lock silence.
//
// THE REPAIR (owner rule, CANONICALIZE-FIRST). A persisted lock ABOVE the decided floor whose
// value is an OUTER alias is REWRITTEN to the block's inner canonical id. Because outer-A and
// outer-B both canonicalize to the SAME inner I, all five nodes become locked on I → they prevote
// I → POL(I) forms → precommit → α-of-K cert → 1082880 finalizes. A lock that CANNOT be resolved
// to an inner (its block bytes are gone) is instead PRUNED (the node boots UNLOCKED at that height
// and re-converges on the winner) — the safe fallback. Nothing AT OR BELOW the decided floor is
// ever touched (finalized history is immutable), and a height that already holds an α-of-K cert is
// never disturbed. If two wrappers at one height canonicalize to DIFFERENT inner blocks, that is a
// REAL fork: the migration STOPS and requires manual recovery rather than merge two distinct
// values.
//
// This is a STATE-COMPATIBILITY MIGRATION of pre-fix durable metadata — NOT a consensus-algorithm
// change and NOT transport wiring. The planner (planLockMigration) is PURE so the 8-condition
// safety gate is exhaustively unit-testable; the durable rewrite reuses the ONE atomic
// write+swap the phantom-floor reconcile also uses (reconcile.go writeReconciledStateLocked).

package chain

import (
	"context"
	"fmt"
	"sort"

	"github.com/luxfi/ids"
)

// LockCanonicalResolver resolves a persisted lock target — for a pre-fix binding, the OUTER
// proposervm wrapper id — to the block's INNER canonical id (canonicalRep). ok=false means the
// id is unknown/unresolvable at migration time (its block bytes are gone), which routes the lock
// to the safety-gated PRUNE fallback. A resolver that is a FIXPOINT on canonical ids
// (CanonicalOf(inner, h) = inner) makes the migration idempotent. `height` is the locked height
// the id is bound to: a resolver MUST refuse (ok=false) an id whose block lives at a DIFFERENT
// height, so a cross-height id collision can never rewrite a slot binding to another height's
// canonical (defence-in-depth — committedSlot[H] only ever holds a height-H id by construction).
type LockCanonicalResolver interface {
	CanonicalOf(id ids.ID, height uint64) (canonical ids.ID, ok bool)
}

// lockPlanKind is the disposition the planner assigns each persisted lock ABOVE the decided floor.
type lockPlanKind uint8

const (
	lockKeep         lockPlanKind = iota // resolves to itself (already canonical) — leave untouched
	lockCanonicalize                     // outer alias — rewrite the lock value to its inner canonical
	lockPrune                            // unresolvable & above floor — drop the lock (boot unlocked)
	lockStop                             // divergent inners / a cert exists — DO NOT auto-repair
)

func (k lockPlanKind) String() string {
	switch k {
	case lockKeep:
		return "keep"
	case lockCanonicalize:
		return "canonicalize"
	case lockPrune:
		return "prune"
	case lockStop:
		return "STOP"
	default:
		return "unknown"
	}
}

// LockPlanEntry is the planned disposition of ONE persisted lock above the floor. It doubles as a
// trace-table row for the inspection tool.
type LockPlanEntry struct {
	Height    uint64       // the locked height (always > DecidedFloor)
	LockOuter ids.ID       // the persisted lock value (the stale OUTER alias, pre-fix)
	LockCanon ids.ID       // the resolved INNER canonical (ids.Empty if unresolvable)
	LockRound uint32       // the recovered v3 lock round (meaningful only when HasRound)
	HasRound  bool         // true = v3 soft lock (lockRounds); false = hard/legacy lock (recoveredLocks)
	CertAt    bool         // an α-of-K cert exists at this height (repair FORBIDDEN)
	Kind      lockPlanKind // keep / canonicalize / prune / STOP
	Reason    string       // human-readable rationale (audit trail)
}

// LockMigrationReport is the full plan across every persisted lock above the floor. It is what the
// inspection tool PRINTS (read-only) and what the migration APPLIES. Deterministic: Entries are
// sorted by height.
type LockMigrationReport struct {
	DecidedFloor     uint64          // the durable decided floor — nothing at/below is ever touched
	FinalizedThrough uint64          // the durable vote-guard floor (== DecidedFloor on a healthy node)
	Entries          []LockPlanEntry // one row per persisted lock ABOVE the floor
	Changed          bool            // any canonicalize/prune — a durable rewrite is warranted
	Stop             bool            // a STOP entry exists — NO write is performed; manual recovery
	StopReason       string          // why the migration stopped (audit trail)
	Skipped          bool            // apply self-disarmed: operator floor target ≠ live decided floor
	SkipReason       string          // why the apply was skipped (audit trail)
}

// distinctInnerOf resolves every candidate outer id AT `height` to its inner canonical and
// reports the SINGLE inner they agree on. resolved is the count of DISTINCT non-empty inner
// canonicals: 0 = none resolvable, 1 = a single agreed inner (returned in inner), ≥2 =
// divergence (a real fork). This is the audited primitive behind the
// canonicalize-vs-prune-vs-STOP decision.
func distinctInnerOf(height uint64, candidates []ids.ID, resolver LockCanonicalResolver) (inner ids.ID, resolved int) {
	if resolver == nil {
		return ids.Empty, 0
	}
	seen := map[ids.ID]struct{}{}
	for _, c := range candidates {
		if c == ids.Empty {
			continue
		}
		if canon, ok := resolver.CanonicalOf(c, height); ok && canon != ids.Empty {
			if _, dup := seen[canon]; !dup {
				seen[canon] = struct{}{}
				inner = canon
			}
		}
	}
	return inner, len(seen)
}

// planLockMigration computes the disposition of every persisted lock ABOVE `floor`, per the
// owner's canonicalize-first rule and the 8-condition safety gate. It is PURE — no engine, clock,
// or store — so the safety gate is exhaustively unit-testable.
//
//   - It NEVER considers a height ≤ floor (finalized history is immutable; conditions 1/5/6).
//   - certAt(h) true ⇒ an α-of-K cert exists at h ⇒ the lock is left as STOP and no write happens
//     (condition 2/4: never disturb a finalized block).
//   - candidatesAt(h) supplies additional outer wrappers KNOWN at height h (tracked siblings), so
//     the inner can be recovered even from a sibling wrapper and cross-wrapper DIVERGENCE is
//     detected. nil ⇒ only the persisted lock value is a candidate.
//   - Two or more DISTINCT inner canonicals at one height ⇒ STOP (condition-parallel to test 5: a
//     real fork must not be merged).
//   - Exactly one inner I: canonicalize the lock to I (or keep, if the lock already equals I —
//     idempotence, condition 6/test 6). No inner resolvable ⇒ prune (the safe fallback).
func planLockMigration(
	committed map[SlotKey]ids.ID,
	lockRounds map[uint64]uint32,
	floor uint64,
	resolver LockCanonicalResolver,
	certAt func(uint64) bool,
	candidatesAt func(uint64) []ids.ID,
) LockMigrationReport {
	var rep LockMigrationReport
	rep.DecidedFloor = floor
	for k, lockVal := range committed {
		if k.Height <= floor {
			continue // finalized history — never inspected, never touched
		}
		// A v3 lock carries a round (soft, unlock-rule re-convergence); a legacy/hard lock has
		// none (HasRound=false) and is carried as a recovered hard lock on apply. Either way the
		// lock VALUE is what the canonicalize/prune decision below rewrites.
		round, hasRound := lockRounds[k.Height]
		e := LockPlanEntry{Height: k.Height, LockOuter: lockVal, LockRound: round, HasRound: hasRound}

		// GATE (conditions 2 & 4): a height that already holds an α-of-K cert is FINALIZED —
		// never canonicalize or prune it. Refuse the whole migration (STOP) rather than risk
		// disturbing a real finalization.
		if certAt != nil && certAt(k.Height) {
			e.CertAt = true
			e.Kind = lockStop
			e.Reason = "α-of-K cert exists at this height — refusing to disturb a finalized block"
			rep.Stop = true
			rep.StopReason = fmt.Sprintf("height %d holds an α-of-K cert (finalized) — manual recovery required", k.Height)
			rep.Entries = append(rep.Entries, e)
			continue
		}

		cands := []ids.ID{lockVal}
		if candidatesAt != nil {
			cands = append(cands, candidatesAt(k.Height)...)
		}
		inner, resolved := distinctInnerOf(k.Height, cands, resolver)
		e.LockCanon = inner
		switch {
		case resolved >= 2:
			// DIVERGENCE (condition-parallel to test 5): the wrappers at this height canonicalize
			// to DIFFERENT inner blocks — a genuine fork. Do NOT merge; STOP.
			e.Kind = lockStop
			e.Reason = "wrappers canonicalize to DIFFERENT inner blocks — real fork, manual recovery required"
			rep.Stop = true
			rep.StopReason = fmt.Sprintf("height %d has ≥2 distinct inner canonicals — manual recovery required", k.Height)
		case resolved == 0:
			// UNRESOLVABLE: the lock's block bytes are gone. Prune (boot unlocked) — the safe
			// fallback; the node re-converges on the deterministic winner once gossip re-populates.
			e.Kind = lockPrune
			e.Reason = "lock target unresolvable (block bytes gone) — prune above the floor (boot unlocked)"
			rep.Changed = true
		case inner == lockVal:
			// Already canonical (idempotence): the lock value equals the inner. Leave untouched.
			e.Kind = lockKeep
			e.Reason = "lock already canonical (inner) — no change"
		default:
			// CANONICALIZE (the owner-preferred repair): rewrite the stale outer alias → inner I.
			e.Kind = lockCanonicalize
			e.Reason = "stale outer wrapper alias — canonicalize to inner block id"
			rep.Changed = true
		}
		rep.Entries = append(rep.Entries, e)
	}
	// Deterministic order (audit + stable trace table).
	sort.Slice(rep.Entries, func(i, j int) bool { return rep.Entries[i].Height < rep.Entries[j].Height })
	// A STOP anywhere means the operator must intervene: perform NO write (Changed forced false).
	if rep.Stop {
		rep.Changed = false
	}
	return rep
}

// inspectLocksLocked builds the migration plan from this node's live durable state WITHOUT
// mutating anything (the read-only inspection path). Caller holds slotMu; candidatesAt/certAt are
// pre-captured closures that never take slotMu (no lock inversion).
func (t *Transitive) inspectLocksLocked(resolver LockCanonicalResolver, certAt func(uint64) bool, candidatesAt func(uint64) []ids.ID) LockMigrationReport {
	floor := t.decidedFloor
	rep := planLockMigration(t.committedSlot, t.lockRounds, floor, resolver, certAt, candidatesAt)
	rep.DecidedFloor = t.decidedFloor
	if t.voteGuard != nil {
		rep.FinalizedThrough = t.voteGuard.FinalizedThrough()
	} else {
		rep.FinalizedThrough = t.decidedFloor
	}
	return rep
}

// migrateStaleLocksAboveFloorLocked plans and, if warranted and SAFE, APPLIES the stale-lock
// repair via the shared atomic write+swap (writeReconciledStateLocked). It NEVER lowers the floor
// (writes t.decidedFloor verbatim) and NEVER touches a binding ≤ floor. On STOP it applies
// nothing and returns the report (the caller logs and leaves the node in its safe, frozen state
// for manual recovery). Caller holds slotMu.
func (t *Transitive) migrateStaleLocksAboveFloorLocked(resolver LockCanonicalResolver, certAt func(uint64) bool, candidatesAt func(uint64) []ids.ID) (LockMigrationReport, error) {
	rep := t.inspectLocksLocked(resolver, certAt, candidatesAt)
	if rep.Stop || !rep.Changed {
		return rep, nil // STOP (manual recovery) or nothing to do (idempotent no-op)
	}
	floor := t.decidedFloor

	// Build the new durable maps: copy the finalized window (≤ floor) verbatim, then apply each
	// above-floor entry's disposition. Copies, so the live maps swap in ONLY after the durable
	// write commits.
	newCommitted := make(map[SlotKey]ids.ID, len(t.committedSlot))
	newLocks := make(map[uint64]uint32, len(t.lockRounds))
	newRecovered := make(map[uint64]struct{}, len(t.recoveredLocks))
	for k, v := range t.committedSlot {
		if k.Height <= floor {
			newCommitted[k] = v
		}
	}
	for h, r := range t.lockRounds {
		if h <= floor {
			newLocks[h] = r
		}
	}
	for h := range t.recoveredLocks {
		if h <= floor {
			newRecovered[h] = struct{}{}
		}
	}
	for _, e := range rep.Entries {
		switch e.Kind {
		case lockPrune:
			// Drop entirely — the node boots UNLOCKED at this height.
			continue
		case lockKeep:
			newCommitted[SlotKey{Height: e.Height}] = e.LockOuter
		case lockCanonicalize:
			newCommitted[SlotKey{Height: e.Height}] = e.LockCanon
		default:
			// lockStop cannot reach here (rep.Stop short-circuits above); a defensive skip.
			continue
		}
		// Carry the lock metadata for a retained (keep/canonicalize) lock so its round-scoped
		// unlock rule (v3) or hard-lock (legacy) is preserved unchanged.
		if e.HasRound {
			newLocks[e.Height] = e.LockRound
		} else {
			newRecovered[e.Height] = struct{}{}
		}
	}

	if err := t.writeReconciledStateLocked(newCommitted, newLocks, newRecovered, floor); err != nil {
		return rep, err
	}
	// Invalidate the CACHED round-view of every migrated height (R4): a view seeded before the
	// migration still locks the PRE-migration id, and viewForLocked returns the cached machine
	// verbatim — a runtime invocation would otherwise keep prevoting the stale value and re-freeze.
	// Dropping the view is safe: the next viewForLocked re-seeds it from the just-rewritten
	// committedSlot/lockRounds (locked on the inner canonical, or unlocked for a prune).
	for _, e := range rep.Entries {
		if e.Kind == lockCanonicalize || e.Kind == lockPrune {
			delete(t.views, e.Height)
		}
	}
	t.log.Warn("STALE-LOCK MIGRATION: canonicalized/pruned pre-fix outer-wrapper view-change locks ABOVE "+
		"the decided floor (all finalized + vote-guard state ≤ floor preserved) — nodes boot locked on the "+
		"inner canonical (or unlocked) and re-converge",
		"decidedFloor", floor,
		"entries", len(rep.Entries))
	return rep, nil
}

// vmCanonicalResolver resolves a lock's (outer) id to its inner canonical using the engine's own
// block sources: the tracked pending block (canonicalRep) first, then the durable VM store
// (canonicalIDOf(VM.GetBlock)). A proposervm wrapper resolves to its inner block id; a bare block
// resolves to itself.
type vmCanonicalResolver struct {
	ctx context.Context
	rt  *Runtime
}

func (r vmCanonicalResolver) CanonicalOf(id ids.ID, height uint64) (ids.ID, bool) {
	t := r.rt.Transitive
	// Tracked-block fast path (also the fixpoint on an already-inner id that is tracked). The
	// height check is the interface's cross-bind guard: an id whose block lives at a different
	// height is treated as unresolvable, never returned as a canonical for THIS height.
	t.mu.RLock()
	pb, tracked := t.pendingBlocks[id]
	var canon ids.ID
	if tracked && pb.ConsensusBlock != nil && pb.ConsensusBlock.height == height {
		canon = pb.ConsensusBlock.canonicalRep()
	}
	t.mu.RUnlock()
	if canon != ids.Empty {
		return canon, true
	}
	// Durable VM store (survives restart — this is what resolves a pre-fix outer id at boot).
	if r.rt.config.VM == nil {
		return ids.Empty, false
	}
	blk, err := r.rt.config.VM.GetBlock(r.ctx, id)
	if err != nil || blk == nil || blk.Height() != height {
		return ids.Empty, false
	}
	c := canonicalIDOf(blk)
	if c == ids.Empty {
		return ids.Empty, false
	}
	return c, true
}

// engineCanonicalContext captures the per-height sibling wrappers + a cert-existence predicate
// the planner needs — as snapshots, so the subsequent slotMu-held plan never takes another lock
// (no inversion). The three locks are taken SEQUENTIALLY, never nested: a brief slotMu hold
// (candidate heights), the consensus ledger's own internal RLock (durable finality — R2), then
// t.mu.RLock (tracked siblings). Returns (candidatesAt, certAt).
func (rt *Runtime) engineCanonicalContext() (func(uint64) []ids.ID, func(uint64) bool) {
	t := rt.Transitive

	// 1) Candidate heights: the committedSlot keys above the floor (slotMu-guarded).
	t.slotMu.Lock()
	floor := t.decidedFloor
	heights := make([]uint64, 0, len(t.committedSlot))
	for k := range t.committedSlot {
		if k.Height > floor {
			heights = append(heights, k.Height)
		}
	}
	t.slotMu.Unlock()

	// 2) DURABLE finality snapshot (R2): the consensus finality ledger — not the in-session views
	// map, which is EMPTY at the boot invocation point and made the old gate vacuous exactly when
	// the migration runs. FinalizedBlockAtHeight takes only the ledger's own RLock (no t.mu, no
	// slotMu), so with no engine lock held here there is no inversion.
	durableCert := make(map[uint64]bool, len(heights))
	for _, h := range heights {
		if _, ok := rt.FinalizedBlockAtHeight(h); ok {
			durableCert[h] = true
		}
	}

	// 3) Tracked sibling wrappers (t.mu-guarded).
	t.mu.RLock()
	byHeight := map[uint64][]ids.ID{}
	for id, pb := range t.pendingBlocks {
		if pb.ConsensusBlock != nil {
			h := pb.ConsensusBlock.height
			byHeight[h] = append(byHeight[h], id)
		}
	}
	t.mu.RUnlock()

	candidatesAt := func(h uint64) []ids.ID { return byHeight[h] }
	certAt := func(h uint64) bool {
		// Durable ledger first (survives the boot invocation point), then the in-session view —
		// a height whose view reached the α-of-K precommit condition THIS run (or entered
		// committedSlot after the step-1 snapshot) is a cert binding the height: the migration
		// must never drop that lock. The closure runs under slotMu (planLockMigration via
		// inspectLocksLocked), so the views read takes no extra lock.
		if durableCert[h] {
			return true
		}
		if v, ok := t.views[h]; ok {
			return v.finalized
		}
		return false
	}
	return candidatesAt, certAt
}

// InspectLocks returns the stale-lock migration PLAN for this node WITHOUT mutating anything — the
// read-only inspection the operator runs on every pod to verify the 8-condition safety predicate
// before any write. The report's Entries are the trace-table rows (per-height outer→inner
// canonicalization, lock round, cert presence, and the planned disposition).
func (rt *Runtime) InspectLocks(ctx context.Context) LockMigrationReport {
	if rt.Transitive == nil {
		return LockMigrationReport{}
	}
	resolver := vmCanonicalResolver{ctx: ctx, rt: rt}
	candidatesAt, certAt := rt.engineCanonicalContext()
	rt.Transitive.slotMu.Lock()
	defer rt.Transitive.slotMu.Unlock()
	return rt.Transitive.inspectLocksLocked(resolver, certAt, candidatesAt)
}

// MigrateStaleLocks plans and, if warranted and SAFE, APPLIES the one-shot stale-lock repair
// (canonicalize pre-fix outer aliases → inner, prune the unresolvable) for every persisted lock
// ABOVE the decided floor. It is a boot-time, operator-gated, idempotent recovery: finalized +
// vote-guard state ≤ floor is preserved verbatim, the floor is never lowered, and a STOP (a height
// with a cert, or divergent inner canonicals) applies nothing and defers to manual recovery.
//
// SELF-DISARMING (R1): `expectedFloor` is the decided floor the OPERATOR observed when arming the
// repair (the incident floor — 1082879 for the mainnet one-shot). If the live decided floor has
// moved past it — i.e. the chain recovered and finalized new heights — the apply is REFUSED as a
// no-op (Skipped=true, nothing written). A lingering apply env on a later boot therefore can
// never degrade into an every-boot prune of a GENUINE in-flight crash lock, which would regress
// the HIGH-1 crash-restart double-sign protection. Returns the plan applied (or the STOP/Skipped
// report).
func (rt *Runtime) MigrateStaleLocks(ctx context.Context, expectedFloor uint64) (LockMigrationReport, error) {
	if rt.Transitive == nil {
		return LockMigrationReport{}, fmt.Errorf("migrate stale locks: nil engine")
	}
	resolver := vmCanonicalResolver{ctx: ctx, rt: rt}
	candidatesAt, certAt := rt.engineCanonicalContext()
	rt.Transitive.slotMu.Lock()
	defer rt.Transitive.slotMu.Unlock()
	if f := rt.Transitive.decidedFloor; f != expectedFloor {
		rep := rt.Transitive.inspectLocksLocked(resolver, certAt, candidatesAt)
		rep.Changed = false
		rep.Skipped = true
		rep.SkipReason = fmt.Sprintf(
			"self-disarm: live decided floor %d ≠ operator target %d — the incident this apply was armed for is over; unset the env",
			f, expectedFloor)
		rt.Transitive.log.Warn("STALE-LOCK MIGRATION SKIPPED (self-disarm): live decided floor does not match "+
			"the operator's apply target — nothing written; unset LUX_CONSENSUS_MIGRATE_STALE_LOCKS",
			"decidedFloor", f,
			"operatorTarget", expectedFloor)
		return rep, nil
	}
	return rt.Transitive.migrateStaleLocksAboveFloorLocked(resolver, certAt, candidatesAt)
}
