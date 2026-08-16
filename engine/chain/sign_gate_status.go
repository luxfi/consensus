// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"fmt"

	"github.com/luxfi/ids"
)

// sign_gate_status.go — READ-ONLY introspection of the durable sign gate.
//
// A validator that cannot sign is indistinguishable from a healthy one at every counter an
// operator can reach: the block verifies on every peer, chits come back accept from all of
// them, the node reports healthy — and the chain never advances, because no node ever emits
// the SIGNED vote a quorum cert is assembled from. Two independent live investigations
// (devnet and testnet, 2026-07-31) each reached reserveSlotForSign and STOPPED, because the
// state that decides the answer — is a guard wired, where does it write, what did it recover,
// which floor is enforced, is this height already bound — was reachable only by reading code
// and guessing at a filesystem path. One of them guessed the path wrong, found no file, and
// concluded the guard was unwired; the boot warning it cited actually PROVES a guard IS wired
// (t.voteGuard != nil is one of its preconditions), so the inference was backwards.
//
// Everything here is a pure observer. Nothing in this file changes what is signed, what is
// tallied, or when: SignGateStatus and ExplainHeight only read, and recordGuardWriteLocked
// returns its argument's error unchanged. The decisions stay in reserveSlotForSign, which
// remains the sole authority on whether a signature is permitted.

// SignGateStatus is a point-in-time snapshot of everything that decides whether this node is
// ABLE to place its one signature at a height: the durable equivocation guard (is one wired,
// which implementation, where it writes, whether that artifact is on disk), what the guard
// recovered at boot, the two floors reserveSlotForSign enforces, and the durable-write
// counters. It is the answer to "why is this node silent?" that does not require a code read.
type SignGateStatus struct {
	// GuardConfigured reports whether a durable VoteGuardStore is wired (t.voteGuard != nil).
	// False is memory-only: correct for verify-only nodes and tests, and on a signer it means a
	// crash forgets every unfinalized binding. On luxd a signing chain always has one — the
	// chain refuses to build if the guard cannot be opened — so false on a production validator
	// is itself the finding.
	GuardConfigured bool
	// GuardImplementation is the store's concrete Go type ("*chain.fileVoteGuard" for the
	// production file store, a stub's type under test). It is the only way to tell the real
	// durable store from a memory stub without reading the wiring — and it exposes a typed-nil
	// store (a non-nil interface holding a nil pointer) that every `== nil` guard would pass.
	GuardImplementation string
	// GuardPath is where the store persists, from the optional VoteGuardLocator. Empty when the
	// store does not implement it (memory-only stubs). THIS is the field whose absence cost a
	// live investigation its conclusion: the guard is not at the data root, it is
	// <chain-data-dir>/network-<N>/<blockchainID>/vote-guard and there is one PER CHAIN.
	GuardPath string
	// GuardFileExists is a LIVE stat of GuardPath at call time. It separates "this node never
	// persisted a binding" (no file was ever written — OpenVoteGuard creates none, the first
	// committed Persist does) from "the file was written and then destroyed". Those are
	// opposite findings: the first says the sign path was never reached, the second says the
	// equivocation memory is gone and the staking identity must be rotated.
	GuardFileExists bool
	// LoadedBindingCount and LoadedFinalizedThrough are the guard's OPEN-TIME recovery — what
	// this node BOOTED with. LoadedFinalizedThrough is NOT re-derivable later: every Persist
	// advances the store's floor, so after the first write FinalizedThrough() answers a
	// different question than the one a post-mortem asks.
	LoadedBindingCount     int
	LoadedFinalizedThrough uint64
	// GuardFloor is t.decidedFloor: the durable, monotonic certified-through height seeded from
	// the guard file at boot and advanced ONLY by compactVoteGuardThroughQuasar (⅔-by-stake
	// export certs — never reorgable Nova).
	GuardFloor uint64
	// QuasarFloor is consensus.GetQuasarSigningFloor(): the in-memory export frontier, re-seeded
	// on boot from the VM's durable LastQuasarHeight.
	QuasarFloor uint64
	// CertifiedFloor is the floor ACTUALLY ENFORCED — max(GuardFloor, QuasarFloor), exactly the
	// value reserveSlotForSign computes. Reported alongside its two inputs so a refusal can be
	// attributed to the right source without re-deriving it, matching the refusal log's own
	// floor/guardFloor/quasarFloor triple.
	CertifiedFloor uint64
	// PersistAttempts, PersistSuccesses and PersistFailures count every durable guard write.
	// A failing store is fail-closed: it costs this node every vote, forever. A nonzero
	// PersistFailures is that fault's only aggregate signal; PersistAttempts == 0 on a running
	// signer says the sign path was never reached, which is a different fault entirely.
	PersistAttempts  uint64
	PersistSuccesses uint64
	PersistFailures  uint64
	// BindingCount is the LIVE committedSlot size: how many heights this node currently holds a
	// signature commitment at. It is pruned to the window above the certified floor, so a count
	// that grows without bound is a chain whose export frontier has stopped advancing.
	BindingCount int
}

// SignGateStatus snapshots the durable sign gate. Read-only and side-effect free.
//
// Lock discipline mirrors reserveSlotForSign exactly: the consensus export frontier is read
// BEFORE slotMu and the guard-file stat is taken AFTER it is released, so this method never
// holds slotMu across a call-out. It takes no engine lock at all, so it is safe to call from a
// health/RPC path that may be holding one.
func (t *Transitive) SignGateStatus() SignGateStatus {
	st := SignGateStatus{}
	if t.consensus != nil {
		st.QuasarFloor = t.consensus.GetQuasarSigningFloor()
	}

	t.slotMu.Lock()
	guard := t.voteGuard
	st.GuardFloor = t.decidedFloor
	st.LoadedBindingCount = t.loadedBindingCount
	st.LoadedFinalizedThrough = t.loadedFinalizedThrough
	st.PersistAttempts = t.persistAttempts
	st.PersistSuccesses = t.persistSuccesses
	st.PersistFailures = t.persistFailures
	st.BindingCount = len(t.committedSlot)
	t.slotMu.Unlock()

	st.CertifiedFloor = st.GuardFloor
	if st.QuasarFloor > st.CertifiedFloor {
		st.CertifiedFloor = st.QuasarFloor
	}

	// Outside slotMu: %T is cheap but Exists() is a syscall, and a stalled filesystem must
	// never be able to block the sign path through an observability call.
	st.GuardConfigured = guard != nil
	if guard != nil {
		st.GuardImplementation = fmt.Sprintf("%T", guard)
		if loc, ok := guard.(VoteGuardLocator); ok {
			st.GuardPath = loc.Path()
			st.GuardFileExists = loc.Exists()
		}
	}
	return st
}

// HeightSignExplain answers, for ONE height, the question a stalled chain forces: can this
// node still contribute its signature here, and if not, what is holding it? It reports the
// two durable facts reserveSlotForSign consults — the per-height binding and the certified
// floor — as VALUES, so "welded to a dead candidate" and "correctly closed at a certified
// height" stop being indistinguishable from the outside.
type HeightSignExplain struct {
	// Height is the height asked about (echoed so a response stands alone).
	Height uint64
	// Bound is the canonical this node durably committed at Height; ids.Empty when unbound.
	// A Bound that names a block the fleet has abandoned is the stall: this node will refuse
	// every other candidate at this height, forever, and the height can never reach α from it.
	Bound ids.ID
	// IsBound distinguishes an unbound slot from one bound to the zero id.
	IsBound bool
	// BelowCertifiedFloor mirrors the floor predicate in reserveSlotForSign EXACTLY —
	// Height <= CertifiedFloor, at-or-below, because a certified height is itself closed. True
	// means the refusal here is CORRECT (the network already finalized this height), which is
	// the benign case the Debug-level refusal log makes invisible in production.
	BelowCertifiedFloor bool
	// CertifiedFloor is the floor actually enforced: max(guard floor, Quasar export frontier).
	CertifiedFloor uint64
}

// ExplainHeight reports the sign gate's durable state at one height. Read-only and
// side-effect free — it binds nothing, clears nothing, and reserves nothing. Same lock
// discipline as SignGateStatus: the consensus floor is read before slotMu.
func (t *Transitive) ExplainHeight(height uint64) HeightSignExplain {
	consensusFloor := uint64(0)
	if t.consensus != nil {
		consensusFloor = t.consensus.GetQuasarSigningFloor()
	}

	t.slotMu.Lock()
	floor := t.decidedFloor
	bound, isBound := t.committedSlot[SlotKey{Height: height}]
	t.slotMu.Unlock()

	if consensusFloor > floor {
		floor = consensusFloor
	}
	if !isBound {
		bound = ids.Empty
	}
	return HeightSignExplain{
		Height:              height,
		Bound:               bound,
		IsBound:             isBound,
		BelowCertifiedFloor: height <= floor,
		CertifiedFloor:      floor,
	}
}

// recordGuardWriteLocked folds the outcome of ONE durable vote-guard write into the sign-gate
// counters and returns err UNCHANGED — a pure observer on the fail-closed paths it wraps.
//
// It is the single seam the counters move through, so an operator reads one aggregate
// rather than a story per write site.
//
// Caller holds slotMu: every guard writer already does (reserveSlotForSign,
// compactVoteGuardThroughQuasar), so plain uint64s need no atomics and cannot drift from
// the writes they count.
func (t *Transitive) recordGuardWriteLocked(err error) error {
	t.persistAttempts++
	if err != nil {
		t.persistFailures++
	} else {
		t.persistSuccesses++
	}
	return err
}
