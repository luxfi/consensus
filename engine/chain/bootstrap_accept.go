// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// bootstrap_accept.go — the BOOTSTRAP accept path: how an EMPTY or BEHIND node
// performs INITIAL SYNC by fetching the chain from a peer's accepted frontier and
// re-executing it to the tip, WITHOUT a vote and WITHOUT a stored α-of-K cert.
//
// WHY A SEPARATE PATH. There are now three roads a block can take to finality and
// they are deliberately decomplected:
//
//   - LIVE (cert) — followVerifiedBlock → vote → α-of-K QuorumCert → AcceptWithCert.
//     The block is being decided NOW; we participate in the quorum. This is the only
//     road once the chain is live.
//   - CATCH-UP (cert-carry) — AcceptCatchupBlock: a behind node is handed each gap
//     block TOGETHER WITH the finality cert the network already assembled, and
//     finalizes through the SAME verified-cert predicate. This recovers a node lagging
//     by at most the served-cert window (maxServedCerts).
//   - BOOTSTRAP (frontier-trust) — THIS FILE: an EMPTY node (genesis → tip) or a node
//     lagging by MORE than the cert window cannot use either road above. The producers
//     do not re-gossip already-finalized blocks and the network will NOT re-vote a
//     decided height (so the live vote road is dead for it), and a peer does not retain
//     certs for ancient heights (so the cert-carry road cannot serve genesis → tip).
//     The only way in is the standard avalanche weak-subjectivity-on-the-beacon-set
//     model: FETCH each block from the network's accepted frontier and RE-EXECUTE it.
//
// THE TRUST MODEL — why accepting a fetched block without a vote/cert is safe HERE,
// and ONLY here. During bootstrap the node trusts the BEACON/VALIDATOR SET it samples
// for the accepted frontier (the same weak-subjectivity anchor avalanche bootstraps
// against). It does NOT trust the bytes: every fetched block is RE-EXECUTED
// (block.Verify) against the already-accepted parent state, so a malicious peer cannot
// advance the sync with an invalid block — Verify fails and the block is REJECTED. And
// it does NOT trust ORDER: the single finalize (consensus.FinalizeBranch) finalizes
// only the contiguous ancestry from the finalized tip, so a bootstrap fed oldest-first
// single-steps (height == finalizedHeight+1, parent == finalizedTip) and a gapped or
// forked block is refused. The result is exactly avalanche's: a bootstrapping node
// converges to the beacon set's frontier by re-execution, with no quorum to join.
//
// WHERE BOOTSTRAP ENDS AND LIVE CONSENSUS BEGINS. This path is permitted ONLY while
// Transitive.InBootstrapPhase() is true. The node ends the phase (FinishBootstrap)
// exactly when it has executed up to the discovered frontier and signals the chain
// bootstrapped — and from that instant AcceptBootstrapBlock is fail-closed: a fetched
// block can no longer finalize without an α-of-K cert. So the frontier-trust authority
// can never be used to bypass the live cert-gate. The live path (vote/cert) is
// UNCHANGED by this file.
package chain

import (
	"context"
	"errors"
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// ErrBootstrapBlockRejected is returned by AcceptBootstrapBlock when the fetched
// block did NOT finalize: the bootstrap phase had already ended (the node is live —
// fail-closed), the bytes did not parse or did not locally Verify (we never finalize
// contents we have not validated), or the block was out of parent order / gapped (the
// per-height guard refused it). It is a CLEAN rejection — nothing was finalized,
// nothing was VM-accepted. The caller (the fetch loop) tries the parent / another
// peer; it must NEVER treat this as a finalize.
var ErrBootstrapBlockRejected = errors.New("chain: bootstrap block rejected — phase ended, unverifiable block, or out-of-order/gapped (not finalized)")

// AcceptBootstrapBlock finalizes ONE block during INITIAL SYNC from a peer's
// accepted frontier: parse → contiguity → local Verify → ledger commit → VM.Accept →
// SetPreference. It accepts on FRONTIER-TRUST + RE-EXECUTION — no vote, no cert — and
// is the missing primitive that lets an empty/behind node sync genesis → tip.
//
// It NEVER finalizes outside the bootstrap phase (InBootstrapPhase), and NEVER
// finalizes a block it has not locally Verified or that violates contiguity:
//   - phase ended (node is live) ⇒ ErrBootstrapBlockRejected (fail-closed: only the
//     α-of-K cert path finalizes once live);
//   - bytes do not parse / do not Verify ⇒ ErrBootstrapBlockRejected (a malicious
//     peer cannot advance the sync with an invalid block);
//   - height ≤ finalized ⇒ no-op nil (the frontier responder always serves some
//     blocks we already hold; not new work, not an error);
//   - height > finalized+1, or parent != finalized tip ⇒ ErrBootstrapBlockRejected
//     (gapped / out of order — the loop must fetch and accept the parent FIRST).
//
// ORDERING INVARIANT (caller's responsibility, engine-ENFORCED). Blocks MUST be fed
// oldest-first — ascending height, each block's parent already finalized — so the EVM
// Verify runs against the already-accepted parent state and the per-height guard is
// satisfied. The guard ENFORCES it (a gapped/out-of-order block is refused, never
// force-accepted), so the invariant is not merely assumed.
func (rt *Runtime) AcceptBootstrapBlock(ctx context.Context, blockBytes []byte) error {
	if rt.config.VM == nil {
		return ErrBootstrapBlockRejected
	}

	// SAFETY GATE — fail-closed once live. The frontier-trust authority exists ONLY
	// for initial sync; the instant the node reaches the frontier (FinishBootstrap)
	// this path is refused and finality flows only through the α-of-K cert-gate.
	if !rt.Transitive.InBootstrapPhase() {
		return ErrBootstrapBlockRejected
	}

	// Parse through the SAME builder the engine frames/parses through, so the block
	// ID and (height, parent) match what the per-height guard records.
	blk, err := rt.config.VM.ParseBlock(ctx, blockBytes)
	if err != nil {
		return errors.Join(ErrBootstrapBlockRejected, err)
	}

	// CONTIGUITY pre-check (cheap, oldest-first). The frontier responder serves an
	// oldest-first window that overlaps blocks we already hold, and on a node lagging
	// by more than one window starts ABOVE our tip:
	//   - height ≤ the SETTLED floor (decided AND executed): already synced past —
	//     skip cleanly (responder overlap).
	//   - settled < height ≤ finalized: decided but never executed — the ledger folded
	//     across blocks this VM did not run, so what these heights need is execution,
	//     not a decision. Verify against our own applied parent state and Accept. The
	//     authority is the same one this whole path rests on — the ⅔-stake-named
	//     frontier plus local Verify — and the ledger serves as the NEGATIVE check: a
	//     block contradicting what it finalized at a height is refused outright.
	//   - height > finalized+1: NOT our contiguous next block (out of order, or the
	//     fetch delivered a higher segment first) — reject WITHOUT verifying/accepting;
	//     the loop fetches the parent and comes back. The per-height guard would refuse
	//     it regardless; this just avoids the wasted Verify.
	// Skipping on the LEDGER height alone is the wedge this replaces: every block in
	// (applied, finalized] read as already-synced and was dropped with a nil error, so
	// the loop counted a full batch accepted, advanced nothing, and re-asked forever.
	// Within an ordered (oldest-first) feed this never wrongly rejects: by the time
	// N+1 is processed, N has been executed, so N+1.height == floor+1.
	if fh, set := rt.Transitive.consensus.GetFinalizedHeight(); set {
		// ONE floor, shared with the runtime catch-up lane (settledHeight): the lower of
		// the ledger and the VM's applied head, with the absence/zero-height guards in
		// exactly one place. Re-deriving it inline is how the two lanes drifted — one got
		// the applied==0 guard, the other collapsed to genesis on an unreadable head.
		floor, _ := rt.settledHeight(ctx)
		if blk.Height() <= floor {
			return nil
		}
		if h := blk.Height(); h <= fh {
			// The replay band: decided by the ledger, not yet executed by the VM. Bind
			// BOTH the height AND the parent to the applied head — a height match alone
			// admits a sibling at the right height, and proposervm commits its outer
			// envelope before the inner EVM refuses a non-child.
			headID, headH, herr := rt.AppliedHead(ctx)
			if herr != nil || h != headH+1 || blk.ParentID() != headID {
				return ErrBootstrapBlockRejected
			}
			// Identity, matched exactly as the catch-up lane matches it — a one-sided
			// negative (reject only on a KNOWN mismatch) let an unknown height fall
			// through to Accept with nothing vouching for it. The ledger's own record
			// first; where it has pruned the height, this node's deep recovery index.
			if canonical, envelope, known := rt.Transitive.consensus.FinalizedAt(h); known {
				if canonical != canonicalIDOf(blk) && envelope != blk.ID() {
					return ErrBootstrapBlockRejected
				}
			} else if outer, ok := rt.recoveredOuterAt(h); !ok || outer != blk.ID() {
				return ErrBootstrapBlockRejected
			}
			// Prefer the copy the VM already holds (gossiped ahead into the store), and
			// Verify only when it is NOT held: re-verifying our own stored block asks the
			// VM to insert it a second time, which it refuses.
			exec, held := blk, false
			if hb, herr := rt.config.VM.GetBlock(ctx, blk.ID()); herr == nil && hb != nil {
				exec, held = hb, true
			}
			if !held {
				if err := exec.Verify(ctx); err != nil {
					return errors.Join(ErrBootstrapBlockRejected, err)
				}
			}
			if err := exec.Accept(ctx); err != nil {
				return errors.Join(ErrBootstrapBlockRejected, err)
			}
			_ = rt.config.VM.SetPreference(ctx, exec.ID())
			return nil
		}
		if blk.Height() > fh+1 {
			return ErrBootstrapBlockRejected
		}
		// height == fh+1: parent == finalizedTip is enforced by FinalizeBranch's walk.
	} else {
		// M2 — FIRST-BLOCK ANCHOR. The finalized-height tracker is UNSET (the un-seeded
		// / empty-genesis path: SyncState only sets it when the VM has a non-empty last
		// accepted). In that state the first FinalizeBranch would SEED the ledger with
		// WHATEVER (height,
		// parent) the peer's first block claims — so a peer could seed finality at an
		// arbitrary height/parent. Bind the first bootstrap block to the VM's ACTUAL
		// last-accepted instead of trusting Verify alone.
		lastID, lastH, lerr := rt.localLastAccepted(ctx)
		if lerr != nil {
			return errors.Join(ErrBootstrapBlockRejected, lerr)
		}
		if lastID == ids.Empty {
			// Truly empty (no accepted block — not even genesis). The only valid first
			// block is genesis itself: height 0 with no parent. Anything else is a peer
			// trying to seed finality mid-chain.
			if blk.Height() != 0 || blk.ParentID() != ids.Empty {
				return ErrBootstrapBlockRejected
			}
		} else {
			if blk.Height() <= lastH {
				return nil // already hold it (responder overlap)
			}
			if blk.Height() != lastH+1 || blk.ParentID() != lastID {
				return ErrBootstrapBlockRejected // not our contiguous next block
			}
		}
	}

	// RE-EXECUTE. This is the FRESH-SYNC path (height == finalized+1, or the M2 first
	// block): the block is being added to the ledger for the FIRST time, on frontier
	// trust. Local Verify is the integrity check that makes that trust safe — a peer
	// cannot advance our sync with an invalid block — so it runs UNCONDITIONALLY here,
	// never skipped. The held-preference belongs only to the REPLAY band above (h ≤ fh),
	// where the block was already finalized and the VM genuinely holds a verified copy;
	// a not-yet-finalized block at fh+1 must be validated, held or not.
	exec := blk
	if err := blk.Verify(ctx); err != nil {
		return errors.Join(ErrBootstrapBlockRejected, err)
	}

	// LEDGER COMMIT through the single finalize (frontier-trust authority). Bootstrap
	// feeds blocks oldest-first and contiguously (the checks above), so this is always
	// a SINGLE-STEP FinalizeBranch: parent == finalized tip, height == finalized+1, no
	// siblings to prune. FinalizeBranch advances finalized history and (defensively)
	// returns a prune plan; bootstrap never produces one (single-branch). On any
	// violation NOTHING advances and we reject — a bootstrapping node cannot be fed a
	// forked/gapped history any more than a live one.
	// The canonical execution commitment is what byHeight records (the authoritative
	// finality id); for a non-wrapped block canonicalIDOf falls back to the outer id.
	// Bootstrap uses the canonical-aware ApplyCert so a proposervm-wrapped bootstrap
	// block records its INNER id, not the envelope.
	if _, err := rt.Transitive.consensus.ApplyCert(Cert{
		Block:     blk.ID(),
		Parent:    blk.ParentID(),
		Height:    blk.Height(),
		Canonical: canonicalIDOf(blk),
	}); err != nil {
		return errors.Join(ErrBootstrapBlockRejected, err)
	}

	// STATE TRANSITION — Accept then SetPreference, the SAME order as the cert
	// finalizer (acceptWithCertCore), so the next block builds/verifies on this
	// accepted parent. The block was just Verified; an Accept failure here is a local
	// VM fault (the network finalized this block — the ledger correctly reflects it),
	// and the NEXT block's Verify against the missing state will halt forward progress,
	// surfacing it. SetPreference keeps the VM's preferred head on the synced tip.
	if err := exec.Accept(ctx); err != nil {
		if rt.config.Logger != nil && !rt.config.Logger.IsZero() {
			rt.config.Logger.Error("bootstrap: VM.Accept failed after Verify (sync will halt at next block)",
				log.Stringer("blockID", exec.ID()), log.Err(err))
		}
	}
	_ = rt.config.VM.SetPreference(ctx, exec.ID())
	return nil
}

// localLastAccepted reads the VM's last-accepted block id and height — the anchor the
// FIRST bootstrap block must extend when the consensus finalized-height tracker is not
// yet seeded (M2). Returns (ids.Empty, 0, nil) for a VM with no accepted block.
func (rt *Runtime) localLastAccepted(ctx context.Context) (ids.ID, uint64, error) {
	id, err := rt.config.VM.LastAccepted(ctx)
	if err != nil {
		return ids.Empty, 0, err
	}
	if id == ids.Empty {
		return ids.Empty, 0, nil
	}
	blk, err := rt.config.VM.GetBlock(ctx, id)
	if err != nil {
		// The VM names a last-accepted id but cannot produce the block — a pruned or
		// partially-imported index. This is NOT height 0: reporting it as 0 let the
		// floor collapse to genesis (a fabricated "we're at the start" that skips the
		// whole chain as already-applied) and let M2 bind a first block to an unread
		// head. Surface it as the error it is; every caller fails closed on it.
		return id, 0, fmt.Errorf("last-accepted block %s is unreadable: %w", id, err)
	}
	return id, blk.Height(), nil
}
