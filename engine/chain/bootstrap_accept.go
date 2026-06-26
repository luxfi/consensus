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
// it does NOT trust ORDER: the per-height guard (markFinalizedLocked, via
// consensus.AcceptBootstrapBlock) requires height == finalizedHeight+1 AND
// parent == finalizedTip, so the chain can only be extended by the contiguous next
// block, never gapped or forked. The result is exactly avalanche's: a bootstrapping
// node converges to the beacon set's frontier by re-execution, with no quorum to join.
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
	//   - height ≤ finalized: already synced past — skip cleanly (responder overlap).
	//   - height > finalized+1: NOT our contiguous next block (out of order, or the
	//     fetch delivered a higher segment first) — reject WITHOUT verifying/accepting;
	//     the loop fetches the parent and comes back. The per-height guard would refuse
	//     it regardless; this just avoids the wasted Verify.
	// Within an ordered (oldest-first) feed this never wrongly rejects: by the time
	// N+1 is processed, N has finalized, so N+1.height == finalized+1.
	if fh, set := rt.Transitive.consensus.GetFinalizedHeight(); set {
		if blk.Height() <= fh {
			return nil
		}
		if blk.Height() > fh+1 {
			return ErrBootstrapBlockRejected
		}
	}

	// RE-EXECUTE. Local Verify proves the block is VALID against our (already-accepted)
	// parent state — this is the integrity check that makes frontier-trust safe: a
	// peer cannot advance our sync with an invalid block. Verified BEFORE any ledger
	// mutation, so a bad block never advances finalized height.
	if err := blk.Verify(ctx); err != nil {
		return errors.Join(ErrBootstrapBlockRejected, err)
	}

	// LEDGER COMMIT through the single per-height guard (frontier-trust authority).
	// markFinalizedLocked authoritatively enforces height == finalized+1 AND
	// parent == finalized tip; on any violation NOTHING advances and we reject.
	if err := rt.Transitive.consensus.AcceptBootstrapBlock(blk.ID(), blk.Height(), blk.ParentID()); err != nil {
		return errors.Join(ErrBootstrapBlockRejected, err)
	}

	// STATE TRANSITION — Accept then SetPreference, the SAME order as the cert
	// finalizer (acceptWithCertCore), so the next block builds/verifies on this
	// accepted parent. The block was just Verified; an Accept failure here is a local
	// VM fault (the network finalized this block — the ledger correctly reflects it),
	// and the NEXT block's Verify against the missing state will halt forward progress,
	// surfacing it. SetPreference keeps the VM's preferred head on the synced tip.
	if err := blk.Accept(ctx); err != nil {
		if rt.config.Logger != nil && !rt.config.Logger.IsZero() {
			rt.config.Logger.Error("bootstrap: VM.Accept failed after Verify (sync will halt at next block)",
				log.Stringer("blockID", blk.ID()), log.Err(err))
		}
	}
	_ = rt.config.VM.SetPreference(ctx, blk.ID())
	return nil
}
