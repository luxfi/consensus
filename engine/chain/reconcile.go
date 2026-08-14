// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// reconcile.go — BOUNDED PHANTOM-FLOOR RECONCILE: the recovery that un-sticks a chain
// whose durable decided-floor ran AHEAD of every VM's applied head.
//
// THE INCIDENT. Before the fail-closed finalize→VM-accept fix, applyBranchFinalization
// SWALLOWED VM.Accept's error and advanced the durable decided-floor (vote-guard
// finalizedThrough) ANYWAY. So a height could be α-of-K FINALIZED in consensus while the
// EVM refused to apply it — and the floor moved past it regardless. On the frozen testnet
// C-Chain this left finalizedThrough = 429 uniform across all 5 validators while the EVM
// heads sat at 421 (luxd-0/4) and 415 (luxd-1/2/3). On every reboot WithVoteGuard re-seeds
// decidedFloor = 429, the sign gate treats 416..429 as decided (permanently unsignable),
// the view-change can NEVER re-finalize them, and the chain is wedged forever.
//
// THE GAP SPLITS at the fleet-max VM-applied height (421 = the highest height ANY node
// actually applied to its EVM):
//
//   - 416..421 COMMITTED-DIVERGENT — luxd-0/4 VM-applied these (real, non-empty blocks in
//     their coreth store, served over RPC). They are recoverable and were externalized, so
//     they MUST NOT be abandoned; a behind node ADOPTS the exact same blocks via the
//     cert-carrying catch-up path (AcceptCatchupBlock), never re-finalizes a different block.
//
//   - 422..429 PHANTOM — NO VM in the fleet ever applied them (all EVM heads <= 421;
//     eth_getBlockByNumber(422) returns "cannot query unfinalized data" on every node). The
//     block bytes are gone (the in-memory pendingBlocks / finalizedByCert / served-cert
//     window were cleared on restart; the durable vote-guard stores NO block bytes and NO
//     signatures — only the floor, the pruned bindings, and the lock rounds). They can be
//     neither applied nor served. The ONLY recovery is to abandon them and let the
//     view-change re-finalize 422+ fresh on the caught-up, consistent state.
//
// THE SAFETY PROOF (no observable double-certification). Reconciling the floor down to
// safeFloor and abandoning the in-consensus finalizations at heights (safeFloor, decidedFloor]
// cannot produce an observable fork — a state where two parties present two valid α-of-K
// certs for DIFFERENT blocks at one height — PROVIDED safeFloor >= the fleet-max VM-applied
// height. Under that bound, for every abandoned height h:
//
//   P1. No VM applied h (h > fleet-max). [caller's contract; verified out of band per node]
//   P2. The EVM serves a block over RPC / to a bridge / to a light client / in a warp
//       attestation ONLY after it ACCEPTS it; a never-accepted height returns "cannot query
//       unfinalized data". So by P1, no block at h was ever externalized — the finalization
//       at h was purely INTERNAL to the consensus layer.
//   P3. No node holds a presentable cert for h. The in-memory homes (finalizedByCert, the
//       served-cert window, the roundView precommit sets) clear on the restart this reconcile
//       runs within, and the durable vote-guard persists bindings and a floor, never
//       signatures. The durable cert store is bounded by the SAME condition as P1:
//       applyBranchFinalization returns at its first VM.Accept error and reaches
//       storeServedCertLocked / persistServedCert only on a clean accept, so a cert is
//       recorded — in memory or on disk — only for a tip the VM applied. Contrapositive: a
//       stored cert at height h means some node applied h, hence h <= fleet-max <= safeFloor,
//       hence h is not in the abandoned range at all.
//   P4. From P2 (never externalized) + P3 (cert unrecoverable): no party holds two certs for
//       h. Re-finalizing a fresh block at h yields exactly ONE presentable cert there. No
//       double-cert is observable. ∎
//
// The bound safeFloor >= fleet-max is enforced two ways: (a) the caller supplies target =
// the operator-VERIFIED fleet-max VM height (a read-only probe of every node's accepted
// head), and (b) an INTRINSIC GUARD raises safeFloor to at least THIS node's own applied
// head, so the reconcile can never abandon a block this node itself committed even under a
// wrong-low target. Heights at/below safeFloor stay decided in the floor (unsignable) and
// are recovered by cert-carrying catch-up, never re-finalized — so a peer's externalized
// block is adopted exactly, never replaced.

package chain

import (
	"context"
	"fmt"

	"github.com/luxfi/ids"
)

// ReconcilePhantomFloor lowers this node's durable decided-through floor to a SAFE height so
// the sign gate stops treating a range of never-applied heights as decided — the fix for a
// phantom floor left by the pre-fix swallowed-VM.Accept bug (see the file header for the full
// incident + safety proof). It is a DELIBERATE, one-shot recovery action driven by the node
// at boot when an operator-verified reconcile target is configured.
//
//	safeFloor = max(target, localVMHeight)
//
// target is the fleet-max VM-applied height (verified out of band). localVMHeight is THIS
// node's own applied head, an intrinsic guard that only ever RAISES safeFloor — so the
// reconcile can never abandon a block this node committed, even under a wrong-low target.
//
// It abandons EXACTLY the range (safeFloor, decidedFloor], and only when decidedFloor >
// safeFloor. It is idempotent: after a successful reconcile decidedFloor == safeFloor, so a
// reboot with the same target is a no-op. A zero target is "not configured" → no-op (a
// reconcile MUST carry an explicit verified target; an unset one never abandons anything).
//
// Returns the floor it reconciled to (the resulting decidedFloor); nil error on a clean
// reconcile OR a no-op. A durable-write failure returns the UNCHANGED floor and an error
// (fail-closed: nothing in memory changed, the node stays safely un-recovered, the operator
// retries).
func (rt *Runtime) ReconcilePhantomFloor(ctx context.Context, target uint64) (uint64, error) {
	if rt.Transitive == nil {
		return 0, fmt.Errorf("reconcile: nil engine")
	}
	// localVMHeight — the intrinsic-guard input. FAIL CLOSED when the VM reports a
	// last-accepted block whose height we cannot read. A transiently-unreadable VM — exactly
	// the recovery regime (a corrupt / half-written store) — must NEVER be treated as height
	// 0, or the guard would degrade to max(target,0)=target and could abandon a committed
	// range the VM actually holds (a self-inflicted fork). localLastAccepted returns
	// (id,0,nil) on a GetBlock failure — a build-anchor convenience for bootstrap that is
	// fail-OPEN for this safety gate — so read the VM DIRECTLY here and distinguish:
	//   - VM.LastAccepted errors                -> refuse (cannot determine our own head).
	//   - LastAccepted is a real id but its
	//     block is unreadable                    -> refuse (VM is present-but-corrupt;
	//                                               abandoning its range would be a fork).
	//   - LastAccepted is ids.Empty              -> genuinely empty VM (a fresh/wiped node):
	//                                               height 0 is CORRECT; the reconcile no-ops
	//                                               (decidedFloor 0 <= safeFloor) or the
	//                                               target governs a truly-empty node.
	var localVMHeight uint64
	if rt.config.VM != nil {
		id, err := rt.config.VM.LastAccepted(ctx)
		if err != nil {
			return 0, fmt.Errorf(
				"reconcile: cannot read VM last-accepted — refusing to reconcile (fail-closed, floor unchanged): %w", err)
		}
		if id != ids.Empty {
			blk, gerr := rt.config.VM.GetBlock(ctx, id)
			if gerr != nil || blk == nil {
				return 0, fmt.Errorf(
					"reconcile: VM last-accepted %s is unreadable — refusing to reconcile (fail-closed; never treat "+
						"a present-but-unreadable VM head as height 0, which would degrade the intrinsic guard and "+
						"could abandon a committed range): %w", id, gerr)
			}
			localVMHeight = blk.Height()
		}
	}
	return rt.Transitive.reconcilePhantomFloor(target, localVMHeight)
}

// reconcilePhantomFloor is the engine core of the bounded floor reconcile, taking an explicit
// localVMHeight so it is directly unit-testable without standing up a VM. It runs entirely
// under slotMu (the same lock the sign gate and the finalize prune hold), so a concurrent
// signer sees either the pre- or the post-reconcile floor+bindings atomically. FAIL-CLOSED:
// it durably commits the lowered floor + pruned bindings FIRST and only then mutates memory,
// so a store-write failure leaves the node exactly as it was.
func (t *Transitive) reconcilePhantomFloor(target, localVMHeight uint64) (uint64, error) {
	if target == 0 {
		// Not configured: a reconcile with no explicit target is a no-op — never derive an
		// abandonment bound implicitly (that could abandon another node's committed range).
		t.slotMu.Lock()
		floor := t.decidedFloor
		t.slotMu.Unlock()
		return floor, nil
	}

	// INTRINSIC GUARD: safeFloor is at least this node's own applied head, so no block this
	// node committed can ever be abandoned (a wrong-low target only ever fails to recover,
	// never violates safety on the local node).
	safeFloor := target
	if localVMHeight > safeFloor {
		safeFloor = localVMHeight
	}

	t.slotMu.Lock()
	defer t.slotMu.Unlock()

	if t.decidedFloor <= safeFloor {
		// No phantom gap above the safe height (or already reconciled): idempotent no-op.
		return t.decidedFloor, nil
	}
	abandonedTo := t.decidedFloor

	// Build the pruned binding set: keep the committed window (height <= safeFloor), drop the
	// phantom range (height > safeFloor). A copy, so the live map is swapped in ONLY after the
	// durable write commits (in writeReconciledStateLocked).
	prunedBindings := make(map[SlotKey]ids.ID, len(t.committedSlot))
	for k, v := range t.committedSlot {
		if k.Height <= safeFloor {
			prunedBindings[k] = v
		}
	}

	// FAIL-CLOSED: durably rewrite the LOWERED floor + pruned bindings FIRST, then swap memory.
	// On failure, mutate NOTHING in memory — the node stays in the (safe, un-recovered) phantom
	// state and the operator retries.
	if err := t.writeReconciledStateLocked(prunedBindings, safeFloor); err != nil {
		return t.decidedFloor, err
	}

	t.log.Warn("PHANTOM-RECONCILE: lowered durable decided-floor — abandoned internal-only "+
		"finalizations that no VM applied and no observer saw (heights re-finalizable fresh "+
		"once every VM has caught up to the safe floor)",
		"from", abandonedTo,
		"to", safeFloor,
		"target", target,
		"localVM", localVMHeight,
		"abandonedRange", fmt.Sprintf("(%d,%d]", safeFloor, abandonedTo))
	return safeFloor, nil
}

// writeReconciledStateLocked is the fail-closed durable-rewrite + in-memory-swap of the
// phantom-floor reconcile (above): it durably rewrites the vote-guard to EXACTLY the given
// bindings + floor via the one atomic writer (VoteGuardStore.Reconcile), and only after that
// write commits does it swap the in-memory committedSlot and set decidedFloor. A store-write
// failure returns an error and mutates NOTHING in memory, so the node stays exactly as it was
// (fail-closed). Caller holds slotMu; the map becomes the live map (the caller passes a fresh
// copy it no longer mutates). newFloor is written verbatim (no monotonic clamp — it is the
// caller's responsibility to never RAISE the floor through this path; the caller passes a
// floor <= the current decidedFloor).
func (t *Transitive) writeReconciledStateLocked(committed map[SlotKey]ids.ID, newFloor uint64) error {
	if t.voteGuard != nil {
		if err := t.recordGuardWriteLocked(t.voteGuard.Reconcile(committed, newFloor)); err != nil {
			return fmt.Errorf("reconcile: durable vote-guard write failed (fail-closed, nothing changed): %w", err)
		}
	}
	t.committedSlot = committed
	t.decidedFloor = newFloor
	return nil
}
