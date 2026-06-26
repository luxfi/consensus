// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// catchup_accept.go — the CERT-CARRYING catch-up path: how a validator that fell
// behind the finalized frontier converges to the tip WITHOUT a restart and
// WITHOUT re-voting.
//
// THE PROBLEM. The producers do not re-gossip already-finalized blocks to a node
// that fell behind, and the network will NOT re-vote heights it has already
// finalized. So a behind node cannot recover by re-entering the VOTING path
// (followVerifiedBlock → cast a vote → wait for α-of-K): there is no quorum to
// rejoin for a decided height. It must instead be handed each missing block
// TOGETHER WITH the finality cert the network already assembled for it, and accept
// it through the CERT path.
//
// THE RULE IS UNCHANGED. Catch-up does NOT introduce a second, weaker acceptance
// authority. AcceptCatchupBlock finalizes ONLY by running the supplied cert through
// HandleIncomingCert — the SAME audited predicate live finality uses (decode →
// α-floor → height gate → set-root cross-check → VerifyWeighted's strict ⅔-of-stake
// → per-height guard → AcceptWithCert). A forged or sub-quorum cert delivered via
// catch-up is rejected with EXACTLY the rigor of live finality: the cert must
// independently rebuild to a VerifiedQuorumCert or the block does not finalize.
// "No VerifiedQuorumCert, no finality" holds through this path too — a node cannot
// be force-fed a chain the cert does not prove.
package chain

import (
	"context"
	"errors"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// ErrCatchupCertRejected is returned by AcceptCatchupBlock when the (block, cert)
// pair did not finalize the block through the verified-cert path: the block did
// not parse/verify locally, or the cert was forged / sub-quorum / below the chain
// α-floor / for a different position / applied out of parent order. It is a CLEAN
// rejection — nothing was finalized. The caller tries another peer or re-polls the
// frontier; it must NEVER treat this as a finalize.
var ErrCatchupCertRejected = errors.New("chain: catch-up cert rejected — block not finalized (unverifiable block, or forged/sub-quorum/out-of-order cert)")

// maxServedCerts bounds the in-memory store of finality certs this node retains to
// SERVE a catching-up peer (CertForBlock). It is a sliding window of the most
// recently finalized heights — large enough that any node lagging by a normal
// network hiccup (a few blocks to a few thousand) can be served its whole gap, yet
// hard-bounded so the store can never grow without limit. A node lagging by more
// than this window is not a "catch-up" case — it bootstraps/state-syncs instead.
// Eviction is by ascending finalized height (oldest-first), which equals insertion
// order because finality is monotonic (markFinalizedLocked: height advances by
// exactly one).
const maxServedCerts = 4096

// storeServedCertLocked records the marshaled finality cert for a just-finalized
// block so this node can serve it to a peer catching up. Called from the SOLE
// finalizer (acceptWithCertCore) with the engine lock held, so EVERY finalize path
// — local assembly, an incoming gossiped cert, and the K==1 single-validator cert —
// captures its cert in this ONE place. Idempotent per block id; bounded to
// maxServedCerts by oldest-height (== FIFO) eviction.
//
// The caller holds t.mu.
func (t *Transitive) storeServedCertLocked(blockID ids.ID, certBytes []byte) {
	if len(certBytes) == 0 {
		return
	}
	if t.certBytesByBlock == nil {
		t.certBytesByBlock = make(map[ids.ID][]byte, maxServedCerts)
	}
	if _, exists := t.certBytesByBlock[blockID]; exists {
		return
	}
	t.certBytesByBlock[blockID] = certBytes
	t.certServedOrder = append(t.certServedOrder, blockID)
	// Evict the oldest finalized cert(s) once past the window. A single insert can
	// only overflow by one, but loop defensively so the invariant holds even if the
	// cap is lowered at runtime.
	for len(t.certServedOrder) > maxServedCerts {
		evict := t.certServedOrder[0]
		t.certServedOrder = t.certServedOrder[1:]
		delete(t.certBytesByBlock, evict)
	}
}

// CertForBlock returns the marshaled α-of-K finality cert this node recorded when
// it finalized blockID, so the node can hand it to a peer catching up. ok is false
// when blockID is not finalized here, or its cert has aged out of the served window
// (the peer then fetches from another node, or bootstraps if it is too far behind).
//
// The returned bytes decode+verify to the SAME VerifiedQuorumCert every node
// finalized blockID on — serving it lets the peer finalize through its own
// HandleIncomingCert with no trust in this node. A defensive copy is returned so a
// caller cannot mutate the served buffer.
func (t *Transitive) CertForBlock(blockID ids.ID) ([]byte, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	b, ok := t.certBytesByBlock[blockID]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), b...), true
}

// AcceptCatchupBlock finalizes ONE gap block from a (blockBytes, certBytes) pair
// fetched during frontier catch-up. It is the receive-side counterpart of
// CertForBlock: parse → local Verify → track → verified-cert finalize.
//
// It NEVER finalizes on anything but a cert that independently clears the full
// finality predicate. The cert is run through HandleIncomingCert, which reuses
// VerifyWeighted / the α-floor / the per-height guard — the identical checks live
// finality runs. So:
//   - a block whose bytes do not parse or do not locally Verify is rejected
//     (we never finalize contents we have not validated, cert or no cert);
//   - a forged, sub-quorum, wrong-position, or below-α-floor cert is rejected
//     (HandleIncomingCert returns false → ErrCatchupCertRejected);
//   - an already-decided height is a no-op (returns nil, finalizes nothing new).
//
// ORDERING INVARIANT (caller's responsibility). Blocks MUST be applied in strict
// PARENT order — ascending height, each block's parent already finalized. The
// per-height guard requires height == finalizedHeight+1 AND parent == finalizedTip,
// so a gapped or out-of-order block is REFUSED, never force-accepted. The node-side
// catch-up transport delivers ancestors oldest-first for exactly this reason.
func (rt *Runtime) AcceptCatchupBlock(ctx context.Context, blockBytes, certBytes []byte) error {
	if rt.config.VM == nil {
		return ErrCatchupCertRejected
	}

	// Parse the block through the SAME builder the engine frames/parses through, so
	// its ID matches the cert's Position.BlockID (and the served cert's key).
	blk, err := rt.config.VM.ParseBlock(ctx, blockBytes)
	if err != nil {
		return errors.Join(ErrCatchupCertRejected, err)
	}

	// CONTIGUITY (defence-in-depth over the per-height guard, and an orphan-accrual
	// bound). The catch-up window the responder serves overlaps blocks we already
	// have AND, on a node lagging by more than that window, starts ABOVE our tip.
	//   - height ≤ finalized: already decided. Skip cleanly — not new work, not an
	//     error (the responder always includes some blocks we hold).
	//   - height > finalized+1: NOT our contiguous next block — either out of order,
	//     or the gap exceeds the served window (a too-far-behind node: it should
	//     BOOTSTRAP, not runtime-catch-up). Reject WITHOUT tracking, so such a node
	//     does not accrue unfinalizable orphans in pendingBlocks. The per-height guard
	//     would reject it regardless; this just avoids the wasted verify+track.
	// Within an ordered (oldest-first) batch this never wrongly rejects: by the time
	// N+2 is processed, N+1 has finalized, so N+2's height == finalized+1.
	if fh, set := rt.Transitive.consensus.GetFinalizedHeight(); set {
		if blk.Height() <= fh {
			return nil
		}
		if blk.Height() > fh+1 {
			return ErrCatchupCertRejected
		}
	}

	// Locally VERIFY the block. A cert proves the NETWORK agreed; local Verify proves
	// the block is VALID against our state — BOTH are required to finalize (identical
	// to the gossip path; a valid cert never substitutes for our own validation).
	if err := blk.Verify(ctx); err != nil {
		return errors.Join(ErrCatchupCertRejected, err)
	}

	// Track the verified block (no vote — see trackVerifiedForCatchup) so the
	// verified-cert finalizer can find it in pendingBlocks.
	rt.trackVerifiedForCatchup(ctx, blk)

	// Finalize through the SOLE audited cert path. It independently decodes and
	// verifies the cert (α-floor + height gate + VerifyWeighted) and commits via the
	// per-height guard + AcceptWithCert, or returns false on ANY rejection.
	if !rt.HandleIncomingCert(certBytes) {
		return ErrCatchupCertRejected
	}
	return nil
}

// trackVerifiedForCatchup records an already-Verified catch-up block in consensus +
// pendingBlocks WITHOUT casting or broadcasting a vote. This is the key difference
// from followVerifiedBlock (the live gossip path, which votes toward assembling a
// cert): catch-up applies a FINISHED cert for an already-decided height, so this
// node does not vote (there is no live quorum to join — voting an old block is pure
// spam peers drop). It only needs the block TRACKED so HandleIncomingCert can
// finalize it. Idempotent: a re-delivered block is tracked once.
//
// The caller (AcceptCatchupBlock) has already parsed+verified blk and confirmed its
// height is above the finalized tip.
func (rt *Runtime) trackVerifiedForCatchup(ctx context.Context, blk block.Block) {
	blockID := blk.ID()
	consensusBlock := &Block{
		id:           blockID,
		parentID:     blk.ParentID(),
		height:       blk.Height(),
		timestamp:    blk.Timestamp().Unix(),
		data:         blk.Bytes(),
		pChainHeight: pChainHeightOf(blk), // epoch for the weighted set (MEDIUM-1)
	}
	// AddBlock is idempotent-ish (errors if already present); the error is ignored
	// because a re-track is harmless and the pendingBlocks guard below is the gate.
	_ = rt.Transitive.consensus.AddBlock(ctx, consensusBlock)

	rt.Transitive.mu.Lock()
	if _, exists := rt.Transitive.pendingBlocks[blockID]; !exists {
		rt.Transitive.pendingBlocks[blockID] = &PendingBlock{
			ConsensusBlock: consensusBlock,
			VMBlock:        blk,
			ProposedAt:     time.Now(),
		}
	}
	rt.Transitive.mu.Unlock()
}
