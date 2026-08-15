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
	"github.com/luxfi/log"
)

// ErrCatchupCertRejected is returned by AcceptCatchupBlock when the (block, cert)
// pair did not finalize the block through the verified-cert path: the block did
// not parse/verify locally, or the cert was forged / sub-quorum / below the chain
// α-floor / for a different position / applied out of parent order. It is a CLEAN
// rejection — nothing was finalized. The caller tries another peer or re-polls the
// frontier; it must NEVER treat this as a finalize.
var ErrCatchupCertRejected = errors.New("chain: catch-up cert rejected — block not finalized (unverifiable block, or forged/sub-quorum/out-of-order cert)")

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
	// A VM that names NO accepted block tells us nothing about what it has applied, and
	// an absent reading is not a reading of zero: taking it as zero would collapse the
	// floor to genesis and make the WHOLE chain replay-eligible. Two absence signals,
	// not one — localLastAccepted returns (Empty, 0) for a VM with no block, and
	// (id, 0) when the last-accepted id is known but its block is unreadable (a pruned
	// or partially-imported index, both live conditions). A zero applied height while
	// the ledger is set is never a reason to lower the floor. Lower it only for a VM
	// naming a block at a real height below the ledger.
	id, applied, err := rt.AppliedHead(ctx)
	if err != nil || id == ids.Empty || applied == 0 || applied >= fh {
		return fh, true
	}
	return applied, true
}

// replayFinalized executes a block the ledger has already finalized but this node's VM
// never applied. It moves the applied head; it does not move the ledger, cast a vote, or
// consume a cert.
//
// Two things can vouch for the block, and one of them must:
//
//   - the LEDGER's own record — IsFinalizedAt says this block is the one finalized at
//     this height; or
//   - the CERT riding with it — a quorum cert naming this exact position, verified
//     through the same predicate live finality uses (α-floor, ⅔ of stake, epoch-bound).
//
// The second exists because the first has a blind spot that this defect lives in. A
// ledger built from a boot seed holds ONE height, and grows only upward from there, so a
// node that restarted below its own finalized height cannot name the very range it needs
// to replay. Refusing there strands it exactly as before — which is what a fleet showed:
// the applied head moved a handful of blocks, to the bottom of the seed, and stopped.
//
// A cert is not a weaker authority than the ledger; it is what put entries in the ledger
// in the first place. What replay must never do is DECIDE, and it does not: no cert is
// consumed, no height is finalized, nothing is voted. It applies a decision the network
// already made. A peer with a forged or sub-quorum cert, or one naming a different block,
// earns a rejection and nothing else.
func (rt *Runtime) replayFinalized(ctx context.Context, blk block.Block, certBytes []byte, held bool) error {
	// CONTIGUITY, before anything is applied. Accept must run only on the block that
	// extends the applied head by one. proposervm commits its OUTER envelope (height
	// index, last-accepted, PutBlock) BEFORE the inner EVM refuses a non-child, so a gap
	// accepted here leaves a durable proposervm index hole that survives the EVM's own
	// fail-closed check and re-creates the very wedge this path exists to cure (the
	// applied head then reads AHEAD of the executed state). The check the EVM makes
	// internally has to be made here, in front of the outer commit.
	headID, headH, herr := rt.AppliedHead(ctx)
	if herr != nil || blk.Height() != headH+1 || blk.ParentID() != headID {
		return ErrCatchupCertRejected
	}
	// The ledger gets the first and last word wherever it has one. Where it has none —
	// below a boot seed, below the pruning window — a verified cert may speak instead.
	// The order matters: a cert that contradicts a height the ledger has already
	// finalized is equivocation, and letting it through here would overwrite decided
	// state on this node's own VM. It is refused without consulting the cert at all.
	// Identity is matched on the CANONICAL commitment, never the outer envelope: a
	// differing wrapper of the same certified inner block is an alias, not a rival.
	canonical, envelope, known := rt.Transitive.consensus.FinalizedAt(blk.Height())
	switch {
	case known && (canonical == canonicalIDOf(blk) || envelope == blk.ID()):
	case known:
		// A block offered at a height the ledger has already finalized to a DIFFERENT
		// canonical commitment. Refused, and worth a line: it is either a stale sibling
		// or a peer probing for a fork. Not silent, but not slashing evidence either —
		// evidence requires a verified conflicting cert, which lives in HandleIncomingCert.
		if rt.config.Logger != nil && !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("catch-up replay refused a block conflicting with finalized state",
				log.Uint64("height", blk.Height()),
				log.Stringer("offered", canonicalIDOf(blk)),
				log.Stringer("finalized", canonical))
		}
		return ErrCatchupCertRejected
	case rt.certVouchesFor(blk, certBytes):
	default:
		return ErrCatchupCertRejected
	}
	// A block our VM already holds was verified when it was stored; re-verifying asks the
	// VM to insert it a second time, which it refuses.
	if !held {
		if err := blk.Verify(ctx); err != nil {
			return errors.Join(ErrCatchupCertRejected, err)
		}
	}
	if err := blk.Accept(ctx); err != nil {
		return errors.Join(ErrCatchupCertRejected, err)
	}
	_ = rt.config.VM.SetPreference(ctx, blk.ID())
	return nil
}

// certVouchesFor reports whether certBytes is a verified quorum cert proving THIS block
// at THIS height on THIS chain. It answers a question; nothing here finalizes, votes, or
// moves the ledger.
//
// The binding is over what the cert SIGNS, never over transport. Position.BlockID and
// Position.ParentID are the outer envelope ids and are NOT in the signed message (see
// cert_codec.go), so binding on them lets anyone rewrite a held cert to name any block
// without touching a signature. The signed identity is the canonical tuple —
// {CanonicalID, ParentCanonicalID, ExecutionStateRoot, PayloadRoot, Height} — so this
// compares the cert's canonical position against the block's OWN canonical commitment.
// On a bare (non-wrapped) chain the canonical id degrades to the outer id on both sides
// and the roots are Empty on both, so the comparison is exact there too.
func (rt *Runtime) certVouchesFor(blk block.Block, certBytes []byte) bool {
	if len(certBytes) == 0 {
		return false
	}
	cert, err := UnmarshalQuorumCert(certBytes)
	if err != nil {
		return false
	}
	t := rt.Transitive
	t.mu.RLock()
	chainID := t.chainID
	setRootSrc := t.setRootSource
	t.mu.RUnlock()

	if cert.Position.ChainID != chainID || cert.Position.Height != blk.Height() {
		return false
	}

	// SIGNED-IDENTITY BINDING. Match the cert's canonical position to the block's own
	// canonical commitment, the same equality finalizeLocalAliasFromVerifiedCert uses to
	// finalize a wrapper against a verified cert. The canonical id degrade (Empty →
	// signed outer id) mirrors the signing path, so it cannot be used to launder in a
	// different block: an attacker who rewrites the outer BlockID must still match the
	// block's canonicalID, which is the signed value.
	certCanon := cert.Position.CanonicalID
	if certCanon == ids.Empty {
		certCanon = cert.Position.BlockID
	}
	if certCanon != canonicalIDOf(blk) {
		return false
	}
	if c, ok := blk.(canonicalCommitter); ok {
		if cert.Position.ParentCanonicalID != c.ParentCanonicalID() ||
			cert.Position.ExecutionStateRoot != c.ExecutionStateRoot() ||
			cert.Position.PayloadRoot != c.PayloadRoot() {
			return false
		}
	}

	// QUASAR ONLY. Replay applies a decision claimed already made, so it must rest on the
	// irreversible export tier. A Nova cert is a reorgable majority (2α−n = 1, not > f),
	// and live finality only VM-Accepts on Nova behind the ledger fold's contiguity and
	// per-height equivocation guard — neither of which replay runs.
	if cert.Tier != Quasar {
		return false
	}
	if floor := t.consensus.Alpha(); floor > 0 && cert.Threshold < uint32(floor) {
		return false
	}

	// EPOCH from the block itself, not from a pendingBlocks lookup that always misses on
	// the replay path (the block is below the ledger, so it was dropped from pending at
	// finalize). Resolving at 0 would verify against the GENESIS validator set, which both
	// rejects every honest cert on a chain with a live epoch and admits a long-range cert
	// from rotated-out genesis keys. The block carries the P-chain height its votes were
	// cast under.
	epochHeight := pChainHeightOf(blk)

	// SET-ROOT LOCK, as HandleIncomingCert applies it: the validator-set root we recompute
	// at the block's epoch must equal the one folded into the signed votes, so a cert
	// laundered from a different epoch is refused even if its signatures verify. A nil
	// source (fixed-set chain) makes both sides Empty — a no-op.
	if setRootSrc != nil {
		if setRootSrc.ValidatorSetRoot(epochHeight) != cert.Position.ValidatorSetRoot {
			return false
		}
	}

	return t.verifyCert(cert, epochHeight) == nil
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
			return rt.replayFinalized(ctx, blk, certBytes, held)
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
					return errors.Join(ErrCatchupCertRejected, err)
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
			return errors.Join(ErrCatchupCertRejected, err)
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
