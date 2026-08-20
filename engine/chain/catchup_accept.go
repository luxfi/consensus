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
	"fmt"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// ErrCatchupCertRejected is returned by AcceptCatchupBlock when the (block, cert)
// pair did not finalize the block through the verified-cert path: the block did
// not parse/verify locally, or the cert was forged / sub-quorum / below the chain
// α-floor / for a different position / applied out of parent order. It is a CLEAN
// rejection — nothing was finalized. The caller tries another peer or re-polls the
// frontier; it must NEVER treat this as a finalize.
var ErrCatchupCertRejected = errors.New("chain: catch-up cert rejected — block not finalized (unverifiable block, or forged/sub-quorum/out-of-order cert)")

// ErrCatchupMissingAncestor: the block is fine and the cert is fine; this node
// simply does not hold the OUTER envelope the block names as its parent.
//
// It used to share ErrCatchupCertRejected, and sharing one error value with a
// bad cert is what made the condition unrecoverable. The two facts have opposite
// remedies: a rejected cert means ask a different peer, a missing ancestor means
// ask THIS peer for the ancestor. Collapsed together, the only move the requester
// makes is the one move that cannot help, and every operator reading the log
// starts from "forged/sub-quorum cert", which is not what happened.
//
// A node reaches this while holding byte-identical execution to its peers: it has
// the right inner block at the parent height under its own wrapper, and the
// network names a different wrapper of that same block. The wrapper is what is
// missing, and a wrapper is fetchable.
var ErrCatchupMissingAncestor = errors.New("chain: catch-up block's parent envelope is not held — fetch the parent")

// ErrCatchupDeferred marks the one non-failure in this file: a block ABOVE the
// contiguous next height was verified and TRACKED, and its finality is deferred until
// the fold reaches it in order. Nothing was rejected and nothing needs retrying — the
// caller's only correct move is to keep feeding the batch.
//
// It exists because this case used to return ErrCatchupCertRejected, so a node doing
// exactly the right thing logged hundreds of "rejected" lines per batch, and every
// diagnosis made from those logs started from a lie. An operator must be able to read
// a counter as written: deferred is progress, rejected is failure, and one error value
// cannot carry both.
var ErrCatchupDeferred = errors.New("chain: catch-up block tracked — finality deferred until the fold reaches its height (not a failure)")

// maxServedCerts bounds the store of finality certs this node retains to SERVE a
// catching-up peer (CertForBlock): a sliding window of the most recently finalized
// heights, hard-bounded so it can never grow without limit. Eviction is by ascending
// finalized height (oldest-first), which equals insertion order because finality is
// monotonic (FinalizeBranch advances finalized history forward by contiguous heights).
//
// THE WINDOW IS THE RECOVERY CEILING. A behind node can only be caught up through
// heights some peer still holds a cert for, so this number is the deepest gap the
// network can close without a re-sync — and every node evicts on the same rule, so a
// straggler below the window finds the cert nowhere rather than on a slower peer.
// At 4096 against a live chain that is a few hours of history: a node down for a
// working day came back permanently unrecoverable, having been served every block it
// needed and no cert for any of them.
//
// A cert is a few hundred bytes, so depth here is cheap in a way block retention is
// not — 64Ki heights is tens of megabytes per chain and covers gaps far beyond what
// a restart, a rollout, or a node replacement produces. The bound still exists; it is
// simply set by what recovery costs rather than by what felt tidy.
const maxServedCerts = 64 * 1024

// recoveryDepth bounds the per-height recovery index (recoveredAt) — how many heights
// below the finalized tip catch-up replay can still name after the ledger's own byHeight
// window has pruned them. Sized like maxServedCerts and for the same reason: recovery
// cost, not tidiness. A height entry is one id, so 64Ki heights is a few megabytes per
// chain and covers any gap a restart, roll, or transient OOM produces. It is deliberately
// far deeper than the ledger's equivocation window, because the two answer opposite
// questions — equivocation looks near the tip, recovery looks far below it.
//
// The FIFO evicts by INSERTION order, which equals height order while finality advances
// monotonically (the common case) — so the retained set is the 64Ki heights nearest the
// tip. It does not track depth-below-tip directly: were heights ever recorded out of
// order, the oldest INSERTION is evicted, not the lowest height. That is acceptable
// because eviction only forfeits the in-place replay shortcut; a height evicted from the
// index still recovers by descent/resync (the else arm below), never incorrectly.
const recoveryDepth = 64 * 1024

// recordRecoveredLocked notes that this node finalized outerID at height h, for the deep
// recovery index. Caller holds t.mu. Idempotent per height; FIFO-bounded to recoveryDepth.
func (t *Transitive) recordRecoveredLocked(h uint64, outerID ids.ID) {
	if t.recoveredAt == nil {
		t.recoveredAt = make(map[uint64]ids.ID, recoveryDepth)
	}
	if _, ok := t.recoveredAt[h]; ok {
		return
	}
	t.recoveredAt[h] = outerID
	t.recoveredOrder = append(t.recoveredOrder, h)
	for len(t.recoveredOrder) > recoveryDepth {
		evict := t.recoveredOrder[0]
		t.recoveredOrder = t.recoveredOrder[1:]
		delete(t.recoveredAt, evict)
	}
}

// recoveredOuterAt returns the outer block id this node finalized at height h, if the
// recovery index still holds it. This is a lookup of this node's OWN finalization
// history — a height enters the index only after acceptWithCertCore folded a verified
// cert over it — so a match is local provenance, never a peer's say-so.
func (t *Transitive) recoveredOuterAt(h uint64) (ids.ID, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	id, ok := t.recoveredAt[h]
	return id, ok
}

// storeServedCertLocked records the marshaled finality cert for a just-finalized
// block so this node can serve it to a peer catching up. Called from the SOLE
// finalizer (acceptWithCertCore) with the engine lock held, so EVERY finalize path
// — local assembly, an incoming gossiped cert, and the K==1 single-validator cert —
// captures its cert in this ONE place. Idempotent per block id; bounded to
// maxServedCerts by oldest-height (== FIFO) eviction.
//
// The caller holds t.mu. The DURABLE copy is written separately, by
// persistServedCert, off the lock — see there for why.
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

// persistServedCert writes the cert to the durable store, so a peer catching up
// can still be handed this height after our process exits.
//
// Called OFF the engine lock, deliberately. The write is an atomic replace with
// an fsync — measured at ~17ms — and finality must never wait on a disk. The
// in-memory window (storeServedCertLocked) has already answered for this height
// by the time we get here, so the only thing at stake in the gap between the two
// is whether a crash loses the ability to SERVE this one height. That costs a
// straggler one fetch from another peer; it cannot cost safety, because nothing
// finalizes on a cert this node failed to write.
func (t *Transitive) persistServedCert(blockID ids.ID, height uint64, certBytes []byte) {
	t.mu.RLock()
	certs := t.certs
	t.mu.RUnlock()
	if certs == nil || len(certBytes) == 0 {
		return
	}
	if err := certs.Put(blockID, height, certBytes); err != nil && t.log != nil {
		t.log.Warn("could not persist the finality cert — once this process exits, no peer can be caught up through this height",
			log.Stringer("blockID", blockID),
			log.Uint64("height", height),
			log.Err(err))
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
	b, ok := t.certBytesByBlock[blockID]
	certs := t.certs
	t.mu.RUnlock()
	if ok {
		return append([]byte(nil), b...), true
	}
	// Not in this process's window — ask the durable store. This is the branch that
	// answers a peer whose gap predates our last restart; without it the cert exists
	// nowhere and the peer stalls at that height forever.
	if certs == nil {
		return nil, false
	}
	return certs.Get(blockID)
}

// VerifyCatchupCertificate verifies that certBytes is a portable quorum proof for
// blockBytes without tracking, finalizing, or otherwise mutating consensus state.
// Bootstrap frontier discovery uses this read-only predicate to distinguish a peer's
// unsigned claim about its tip from a tip backed by the same cryptographic evidence
// that the live finalizer accepts.
//
// The block is parsed only to bind the proof to the local chain, height, canonical
// execution commitment, and validator-set epoch. Its state transition is deliberately
// not executed here; ordered bootstrap execution still verifies each block when it is
// applied by AcceptCatchupBlock.
func (rt *Runtime) VerifyCatchupCertificate(ctx context.Context, blockBytes, certBytes []byte) error {
	if rt.config.VM == nil || len(certBytes) == 0 {
		return ErrCatchupCertRejected
	}
	blk, err := rt.config.VM.ParseBlock(ctx, blockBytes)
	if err != nil {
		return errors.Join(ErrCatchupCertRejected, err)
	}
	cert, err := UnmarshalQuorumCert(certBytes)
	if err != nil {
		return errors.Join(ErrCatchupCertRejected, err)
	}

	t := rt.Transitive
	t.mu.RLock()
	chainID := t.chainID
	floor := t.consensus.Alpha()
	if cert.Tier == Nova {
		floor = NovaSignerFloor(t.consensus.K())
	}
	setRootSource := t.setRootSource
	t.mu.RUnlock()

	if cert.Position.ChainID != chainID ||
		cert.Position.Height != blk.Height() ||
		certCanonical(cert) != canonicalIDOf(blk) ||
		(floor > 0 && cert.Threshold < uint32(floor)) {
		return ErrCatchupCertRejected
	}

	epochHeight, parentEpoch, regressed := rt.epochRegresses(blk)
	if regressed {
		if rt.config.Logger != nil && !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("catch-up: REFUSED block — P-chain epoch regresses below parent (far-past epoch attack)",
				log.Stringer("blockID", blk.ID()),
				log.Stringer("parentID", blk.ParentID()),
				log.Uint64("childEpoch", epochHeight),
				log.Uint64("parentEpoch", parentEpoch))
		}
		return ErrCatchupCertRejected
	}
	if setRootSource != nil && setRootSource.ValidatorSetRoot(epochHeight) != cert.Position.ValidatorSetRoot {
		return ErrCatchupCertRejected
	}
	if err := t.verifyCert(cert, epochHeight); err != nil {
		return errors.Join(ErrCatchupCertRejected, err)
	}
	return nil
}

// epochRegresses reports whether blk claims a P-chain epoch BELOW its parent's.
//
// The epoch selects the validator set that verifies the block's cert, and it is
// read out of a block a peer handed us. Letting it move backwards lets that peer
// choose which historical set gets to attest -- so the bound is that a chain's
// epoch only moves forward, and safety reduces to current-set BFT.
//
// The gossip door has enforced this since the far-past epoch work. The catch-up
// door read the same peer-supplied field and enforced nothing, so a node fetching
// history could be steered onto a set the live path would have refused. One
// invariant with two doors is one guarded invariant; this is the second door.
//
// A parent that is not tracked leaves nothing to regress against and is admitted:
// the attack needs an epoch below the PARENT's, which is only meaningful once the
// parent is tracked, and an orphan cannot extend finalized history anyway.
func (rt *Runtime) epochRegresses(blk block.Block) (child, parent uint64, bad bool) {
	child = pChainHeightOf(blk)
	parentEpoch, ok := rt.Transitive.consensus.EpochHeightOf(blk.ParentID())
	if !ok || child >= parentEpoch {
		return child, parentEpoch, false
	}
	return child, parentEpoch, true
}

// classifyVerifyFailure says which of the two failures a Verify error is.
//
// A missing ancestor surfaces as database.ErrNotFound from deep inside the outer
// parent resolution, where the operator-visible string is bare "not found" and
// names nothing at all. Naming the parent here is the difference between a log
// line an operator can act on and one that sends them looking for a forged cert.
func classifyVerifyFailure(blk block.Block, err error) error {
	if errors.Is(err, database.ErrNotFound) {
		return fmt.Errorf("%w: %s", ErrCatchupMissingAncestor, blk.ParentID())
	}
	return errors.Join(ErrCatchupCertRejected, err)
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
// settledHeight is the height below which catch-up has nothing to offer: the highest
// height this node has BOTH finalized in consensus AND applied to its VM.
//
// These two are not the same number, and reading only the first is what makes a behind
// node stop. The ledger records what a quorum decided; the VM records what this node
// actually executed. Finalization can fold the ledger across a block the VM never
// applied — a pendingBlocks miss is folded as accepted-with-no-VM-block, and the recall
// bridge answers for blocks the VM merely holds — so the ledger legitimately runs ahead
// of the applied head. Catch-up steering by the ledger alone then discards every block
// in exactly the range it is trying to fetch: the responder serves the gap, each entry
// is at or below the ledger, each is skipped as "already decided", and the node reports
// a full batch accepted while applying none of it. Nothing retries, because nothing
// registered a failure.
//
// The lower of the two is the honest floor. A height is worth fetching whenever either
// half is missing, and skipping requires both.
func (rt *Runtime) settledHeight(ctx context.Context) (uint64, bool) {
	fh, set := rt.Transitive.consensus.GetFinalizedHeight()
	if !set {
		return 0, false
	}
	// Lower the floor to the VM's applied head only when we can READ that head and it
	// sits below the ledger. AppliedHead now returns an error for an unreadable head (a
	// pruned/partial index), so a failed read no longer masquerades as height 0 — which
	// would collapse the floor to genesis and skip the whole chain as already-applied.
	// A VM naming no block (Empty) stays at the ledger floor. A VM genuinely at height 0
	// with a ledger above it IS a wedge, and lowering the floor to 0 is exactly what lets
	// replay execute height 1 onward.
	id, applied, err := rt.AppliedHead(ctx)
	if err != nil || id == ids.Empty || applied >= fh {
		return fh, true
	}
	return applied, true
}

// replayFinalized executes a block the consensus ledger has already finalized but this
// node's VM never applied. It moves the applied head; it does not move the ledger, cast a
// vote, or consume a cert. The block is applied on the LEDGER's own authority — nothing
// weaker, and nothing external.
//
// The ledger is the only thing that vouches. It finalized this exact block at this height
// (matched on the canonical commitment, so a differing wrapper of the same inner block is
// an alias, not a rival), so re-executing it decides nothing new — it aligns the VM with a
// decision the ledger already committed. A block the ledger finalized DIFFERENTLY is
// refused; a height the ledger cannot speak for is deferred, NOT executed on a cert.
//
// WHY NO CERT PATH. On restart the ledger seeds at the VM's applied head and folds gap
// heights ABOVE that seed, so the shallow ledger-ahead-of-VM gap (a roll, a restart, an
// OOM) is entirely within byHeight and the ledger names every height of it. A gap deeper
// than the pruning window is a node that fell too far to catch up in place; its recovery
// is bootstrap/resync, the descent's job, not a runtime replay on a cert whose epoch the
// offered block would itself supply. Replay stays exactly one concern: re-run what the
// ledger decided.
func (rt *Runtime) replayFinalized(ctx context.Context, blk block.Block, held bool) error {
	// CONTIGUITY, before anything is applied. Accept must run only on the block that
	// extends the applied head by one. proposervm commits its OUTER envelope (height
	// index, last-accepted, PutBlock) BEFORE the inner EVM refuses a non-child, so a gap
	// accepted here leaves a durable proposervm index hole that survives the EVM's own
	// fail-closed check and re-creates the very wedge this path exists to cure.
	headID, headH, herr := rt.AppliedHead(ctx)
	if herr != nil || blk.Height() != headH+1 || blk.ParentID() != headID {
		return ErrCatchupCertRejected
	}
	canonical, envelope, known := rt.Transitive.consensus.FinalizedAt(blk.Height())
	switch {
	case known && (canonical == canonicalIDOf(blk) || envelope == blk.ID()):
		// The ledger names this block at this height. Fall through and execute it.
	case known:
		// A block offered at a height the ledger finalized to a DIFFERENT canonical
		// commitment — a stale sibling or a peer probing for a fork. Refused, and worth
		// a line; not slashing evidence (that needs a verified conflicting cert, which
		// lives in HandleIncomingCert).
		if rt.config.Logger != nil && !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("catch-up replay refused a block conflicting with finalized state",
				log.Uint64("height", blk.Height()),
				log.Stringer("offered", canonicalIDOf(blk)),
				log.Stringer("finalized", canonical))
		}
		return ErrCatchupCertRejected
	default:
		// The ledger's own byHeight has pruned this height (it retains only a near-tip
		// window for equivocation). Fall back to the deep recovery index — this node's
		// record of what IT finalized at each height this session. A match there is the
		// same authority the ledger arm carries: a height enters the index only after a
		// verified cert folded over it.
		if outer, ok := rt.recoveredOuterAt(blk.Height()); ok {
			// Match the OUTER id, not the canonical inner commitment. The ledger arm
			// above is alias-tolerant (canonical OR envelope) because a tracked block
			// carries its inner canonicalID; a gap-folded height does not — the block is
			// untracked by the time the fold reaches it, so all the ledger fold knows is
			// the outer id from plan.Accept, and that is what the index stores. Matching
			// the outer is therefore not a weaker check but a STRONGER one: the outer id
			// is the hash of the whole envelope, so an envelope re-wrap (same inner
			// content, different proposer/timestamp) shares the canonical but NOT the
			// outer, and is correctly refused here. Finalization is unique per height, so
			// the legitimate chain always re-serves the exact envelope this node folded.
			if outer != blk.ID() {
				return ErrCatchupCertRejected
			}
			// authorized by our own finalization record — fall through and execute
		} else {
			// Neither the ledger nor the recovery index can name this height — the gap
			// is deeper than either remembers. This node cannot replay in place; it
			// recovers by descent / resync. A plain rejection is the honest signal
			// (NOT ErrCatchupDeferred, which reads as progress and would suppress the
			// descent on exactly the node that needs it).
			return ErrCatchupCertRejected
		}
	}
	// Verify only a block the VM does NOT already hold. Re-verifying a held block asks the
	// VM to insert it a second time, which it refuses. Skipping Verify here is safe not
	// because "held implies verified" (an unwrapped VM can hold a state-synced block it
	// never executed) but because the VM's Accept is itself fail-closed: coreth's
	// ACCEPT-BACKSTOP re-executes the block against the applied parent and rejects a
	// state-root mismatch, and contiguity above bound that parent to our applied head. A
	// held block that is wrong-for-our-state fails at Accept, never commits bad state.
	if !held {
		if err := blk.Verify(ctx); err != nil {
			return errors.Join(ErrCatchupCertRejected, err)
		}
	}
	if err := blk.Accept(ctx); err != nil {
		return errors.Join(ErrCatchupCertRejected, err)
	}
	_ = rt.config.VM.SetPreference(ctx, blk.ID())

	// Settle the engine's own books, so a block that was TRACKED (from an earlier
	// out-of-order delivery) and is now applied does not linger in pendingBlocks
	// forever and the accepted counter does not under-report. Mirrors the fold's
	// bookkeeping (applyBranchFinalization), minus the vote/cert plumbing replay never
	// touches. Idempotent: a block already decided is left alone.
	t := rt.Transitive
	t.mu.Lock()
	id := blk.ID()
	if pending, ok := t.pendingBlocks[id]; ok && !pending.Decided {
		pending.Decided = true
		t.finalizedByCert[id] = struct{}{}
		t.blocksAccepted++
		t.dropPendingBlockLocked(id)
		delete(t.bufferedVotes, id)
		delete(t.catchupRequested, id)
	}
	t.recordRecoveredLocked(blk.Height(), id)
	t.mu.Unlock()
	return nil
}

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

	// The epoch selects the validator set that gets to attest, and it is read out
	// of a block a peer handed us — so it is bounded here exactly as it is on the
	// gossip door. Checked before anything is verified, tracked, or accepted,
	// because everything after this reads the epoch.
	if childEpoch, parentEpoch, regressed := rt.epochRegresses(blk); regressed {
		if rt.config.Logger != nil && !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("catch-up: REFUSED block — P-chain epoch regresses below parent (far-past epoch attack)",
				log.Stringer("blockID", blk.ID()),
				log.Stringer("parentID", blk.ParentID()),
				log.Uint64("childEpoch", childEpoch),
				log.Uint64("parentEpoch", parentEpoch))
		}
		return ErrCatchupCertRejected
	}

	// PREFER THE COPY WE HOLD, before any height decision. A block our VM has already
	// accepted was verified when we accepted it, and re-verifying it asks the VM to
	// insert an old canonical block a second time — which it refuses ("side chain
	// insertion is not supported", ErrPrunedAncestor once the gap exceeds the commit
	// interval). That refusal is our own history coming back to us, not a bad block.
	//
	// This decision belongs ABOVE the height switch and not inside one of its arms.
	// The arms differ in what they DO with the block; none of them differ in whether
	// the block needs verifying. Putting the preference in the >fh+1 arm alone left
	// the one height that actually moves the ledger — exactly fh+1 — on the
	// unconditional path, so a node behind on certs tracked every block in its gap
	// except the next one. One untracked height is enough: pathFromTip walks a cert's
	// block down to the tip and defers on the first gap, so every cert above that hole
	// is refused, however many blocks were tracked.
	held := false
	if h, herr := rt.config.VM.GetBlock(ctx, blk.ID()); herr == nil && h != nil {
		blk, held = h, true
	}

	// CONTIGUITY (defence-in-depth over the per-height guard, and an orphan-accrual
	// bound). The catch-up window the responder serves overlaps blocks we already
	// have AND, on a node lagging by more than that window, starts ABOVE our tip.
	//   - height ≤ settled: already decided AND already applied. Skip cleanly — not
	//     new work, not an error (the responder always includes blocks we hold).
	//   - height > settled+1: NOT our contiguous next block — either out of order,
	//     or the gap exceeds the served window (a too-far-behind node: it should
	//     BOOTSTRAP, not runtime-catch-up). Reject WITHOUT tracking, so such a node
	//     does not accrue unfinalizable orphans in pendingBlocks. The per-height guard
	//     would reject it regardless; this just avoids the wasted verify+track.
	// Within an ordered (oldest-first) batch this never wrongly rejects: by the time
	// N+2 is processed, N+1 has finalized, so N+2's height == settled+1.
	if fh, set := rt.settledHeight(ctx); set {
		if blk.Height() <= fh {
			// Skip only what we actually hold. "At or below the settled height" is
			// not the same claim as "already ours": a node whose wrapper of a
			// settled height differs from the network's is missing precisely the
			// envelope that arrives in this band, and discarding the band unexamined
			// throws away the one block that would let the next height verify. That
			// is why a batch can report hundreds of entries skipped while the node
			// advances nothing.
			if _, err := rt.config.VM.GetBlock(ctx, blk.ID()); err == nil {
				return nil
			}
			// Verify resolves and stores the alternate envelope. It decides nothing:
			// the inner block at this height is already applied, so the inner VM
			// returns immediately, and this path neither tracks, votes, accepts, nor
			// moves the ledger.
			if err := blk.Verify(ctx); err != nil {
				return classifyVerifyFailure(blk, err)
			}
			return nil
		}
		// REPLAY. Between the applied head and the finalized height sit blocks this node
		// has already proven and never executed. They are not a finality question —
		// asking for a cert here asks the network to re-decide what it decided long ago,
		// and the height guard correctly refuses to finalize below the ledger, so the
		// two rules meet and the node stops. What is missing is execution, so execute.
		//
		// The ledger is the authority for which block belongs at the height; a block
		// that does not match it is refused, and a height the ledger does not know is
		// refused too. Nothing here decides anything — replay applies a decision that
		// already exists.
		if ledgerH, ok := rt.Transitive.consensus.GetFinalizedHeight(); ok && blk.Height() <= ledgerH {
			return rt.replayFinalized(ctx, blk, held)
		}
		if blk.Height() > fh+1 {
			// ABOVE our next height — verify and TRACK it rather than discard it.
			//
			// A cert finalizes a CHAIN, not a block: every block names its parent, so
			// one verified ⅔ cert on any descendant establishes finality for its whole
			// ancestry, and the fold walks that path (applyBranchFinalization accepts
			// plan.Accept in ascending order). What that walk needs is for the blocks
			// in between to be TRACKED.
			//
			// Discarding them made a node's own recovery depend on certs that no longer
			// exist. The certs for old heights were assembled by processes that have
			// since exited, so a node behind by more than the live cert window could be
			// served exactly the blocks it needed and had to throw every one away, then
			// ask again forever. Keeping them costs one verify and a bounded number of
			// pendingBlocks entries (the responder's window caps the batch); it buys a
			// path home that does not require the past to still be in someone's memory.
			//
			// SAFETY IS UNCHANGED. Tracking is not finalizing. Nothing here decides
			// anything: the block is locally Verified first (we never track contents we
			// have not validated), and finality still happens only in HandleIncomingCert
			// behind the same α-floor, set-root and VerifyWeighted checks. "No
			// VerifiedQuorumCert, no finality" holds exactly as before.
			// A block our VM already holds was verified when we accepted it, and
			// re-verifying it asks the VM to insert an old canonical block a second
			// time — which it correctly refuses ("side chain insertion is not
			// supported"). That refusal is not a bad block; it is our own history
			// coming back to us. A node whose finality has fallen behind its own
			// applied head receives exactly this: every entry is a block it already
			// has, so re-verification failed on all of them and the walk that would
			// have advanced finality was never reached. Prefer the copy we hold.
			if !held {
				if err := blk.Verify(ctx); err != nil {
					return classifyVerifyFailure(blk, err)
				}
			}
			rt.trackVerifiedForCatchup(ctx, blk)
			// The cert riding with a block ABOVE our next height is deliberately NOT
			// offered here. Contiguity is a safety property, not an optimisation: a
			// valid cert does not license finalizing a height we have not reached, and
			// with the ancestry walk able to resolve intermediate blocks, offering it
			// would let one cert carry finality across a gap the per-height guard
			// exists to refuse. Track the block and stop; it finalizes when finality
			// reaches it in order.
			return ErrCatchupDeferred
		}
	}
	// LEDGER UNSET (fresh process / post-restart, before the first fold of this session)
	// is deliberately NOT special-cased here. The seed-anchor invariant — the ledger may
	// not seed above the VM's applied head — lives in ONE place, acceptWithCertCore (see
	// ErrSeedAboveAppliedHead), through which this path funnels via HandleIncomingCert
	// below. A tip cert on an unset ledger is verified+tracked here (harmless — the same
	// as the >settled+1 arm above) and then refused at the fold, which surfaces as a
	// rejection and drives the descent that fetches the gap in order.

	// Locally VERIFY the block, unless our VM already holds it — in which case it was
	// verified when we accepted it, and asking again is the re-insertion the VM
	// refuses. A cert proves the NETWORK agreed; validation against our own state
	// proves the block is sound, and BOTH are still required to finalize. Being
	// already-accepted IS that validation, carried forward.
	//
	// This is the height that moves the ledger, so it is the height a stale
	// re-verification is most expensive at: refusing here returns before the block is
	// tracked, and pathFromTip then defers every cert above it on the resulting hole.
	if !held {
		if err := blk.Verify(ctx); err != nil {
			return classifyVerifyFailure(blk, err)
		}
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
	setCanonicalFromVM(consensusBlock, blk) // stamp the inner execution commitment
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
