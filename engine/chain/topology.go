// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// The vote/cert distribution handlers that make α-of-K finality both safe and live.
//
// The topology in one paragraph: a follower verifies a gossiped block and
// broadcasts its signed accept vote to every validator (followVerifiedBlock,
// integration.go). Each validator feeds incoming signed votes into the engine
// (HandleIncomingVote), which collects them toward a QuorumCert. Whichever node
// first collects α distinct verified votes assembles the cert and gossips it
// (tryFinalizeBlock); every node finalizes the block on receipt of that
// verifiable proof (HandleIncomingCert). No node finalizes without a cert —
// that is safety. The cert reaches everyone by three independent routes
// (vote-broadcast, cert-gossip, the poll-timeout re-request), so finality never
// depends on any single node's inbound chits — that is liveness.
package chain

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/luxfi/consensus/core/slashing"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// ErrVoteWireCorrupt is returned by decodeSignedVote on any structural defect.
var ErrVoteWireCorrupt = errors.New("chain: signed vote wire corrupt")

// signed-vote wire layout (big-endian):
//
//	node_id:20
//	sig_len:4  sig:sig_len
//
// The signed vote travels with the blockID in the gossip envelope (the network
// message already names the chain + block), so the canonical message a verifier
// rebuilds is derived from the receiver's tracked position for that block — a
// vote cannot smuggle a different position because the signature is checked
// against the receiver's own (chain,height,round,block,parent).

// encodeSignedVote encodes (nodeID, signature) for broadcast.
func encodeSignedVote(nodeID ids.NodeID, sig []byte) ([]byte, error) {
	buf := make([]byte, 0, ids.NodeIDLen+4+len(sig))
	buf = append(buf, nodeID[:]...)
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], uint32(len(sig)))
	buf = append(buf, u32[:]...)
	buf = append(buf, sig...)
	return buf, nil
}

// decodeSignedVote is the inverse of encodeSignedVote. Strict trailing-bytes
// policy; fail-closed on short reads.
func decodeSignedVote(data []byte) (ids.NodeID, []byte, error) {
	var nodeID ids.NodeID
	if len(data) < ids.NodeIDLen+4 {
		return nodeID, nil, ErrVoteWireCorrupt
	}
	copy(nodeID[:], data[:ids.NodeIDLen])
	rest := data[ids.NodeIDLen:]
	sigLen := binary.BigEndian.Uint32(rest[:4])
	rest = rest[4:]
	if uint64(sigLen) != uint64(len(rest)) {
		return nodeID, nil, fmt.Errorf("%w: sig_len %d != remaining %d", ErrVoteWireCorrupt, sigLen, len(rest))
	}
	sig := make([]byte, sigLen)
	copy(sig, rest)
	return nodeID, sig, nil
}

// HandleIncomingVote ingests a signed accept vote received from another
// validator's broadcast. The vote is bound to a blockID (carried by the gossip
// envelope); the engine rebuilds the canonical message from its own tracked
// position for that block and verifies the signature before counting the vote.
// A vote for a block we are not tracking is dropped (we cannot know its
// position to verify against — the proposer's block gossip carries that, and
// arrives via HandleIncomingBlock).
//
// Returns true iff the vote verified and was counted toward the block's cert.
func (rt *Runtime) HandleIncomingVote(blockID ids.ID, voteBytes []byte) bool {
	nodeID, sig, err := decodeSignedVote(voteBytes)
	if err != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("incoming vote: decode failed", log.Stringer("blockID", blockID), log.Err(err))
		}
		return false
	}

	t := rt.Transitive
	t.mu.RLock()
	pending, exists := t.pendingBlocks[blockID]
	verifier := t.voteVerifier
	var pos VotePosition
	var epochHeight uint64
	if exists {
		pos = t.blockPositionLocked(pending, blockID)
		epochHeight = t.epochHeightLocked(pending)
	}
	t.mu.RUnlock()

	if verifier == nil {
		return false
	}
	if !exists {
		// The block is not pending. It may be a finalized (Nova-accepted) block whose trailing
		// ⅔-stake vote is still arriving — the ⅔-th stake vote necessarily follows the
		// bare-majority accept — so route a verified accept for it to the late-attestation path
		// (handleVote → attestFinalizedVote → the Quasar attestor) to complete the export cert.
		// Verify against the remembered accepted position: the exact bytes the accept votes
		// signed. An unknown or aged-out block drops.
		ap, remembered := t.lookupAcceptedPos(blockID)
		if !remembered || !verifier.VerifyVote(nodeID, CanonicalVoteMessage(ap.pos), sig, ap.epoch) {
			return false
		}
		t.ReceiveVote(Vote{
			BlockID:   blockID,
			NodeID:    nodeID,
			Accept:    true,
			Signature: sig,
			ParentID:  ap.pos.ParentID,
			Round:     ap.pos.Round,
		})
		return true
	}

	// Verify the signature against our position for this block, resolving the
	// voter's pubkey at the block's P-chain epoch height. A vote that signed a
	// different position (different height/round/parent/set-root) fails.
	if !verifier.VerifyVote(nodeID, CanonicalVoteMessage(pos), sig, epochHeight) {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("incoming vote: signature invalid",
				log.Stringer("blockID", blockID), log.Stringer("from", nodeID))
		}
		return false
	}

	// Count it. ReceiveVote routes through handleVote, which records the signed
	// vote toward the cert and triggers tryFinalizeBlock once alpha is reached.
	t.ReceiveVote(Vote{
		BlockID:   blockID,
		NodeID:    nodeID,
		Accept:    true,
		Signature: sig,
		ParentID:  pos.ParentID,
		Round:     pos.Round,
	})
	return true
}

// HandleIncomingCert ingests a finality cert gossiped by another validator and,
// if it verifies as a valid α-of-K witness for a block we have verified,
// finalizes that block. A follower commits a gossiped block only against a
// verifiable α-of-K proof.
//
// What a cert must clear:
//   - it must decode and Verify under our VoteVerifier (α distinct
//     correctly-signed accepts over the cert's position),
//   - its position chain must match ours,
//   - we must have verified the block locally (it is in pendingBlocks). A block
//     whose contents we have not validated is not accepted even with a valid
//     cert — the cert proves agreement, local Verify proves validity; finality
//     needs both.
//
// Returns true iff the block was finalized as a result of this cert.
func (rt *Runtime) HandleIncomingCert(certBytes []byte) bool {
	cert, err := UnmarshalQuorumCert(certBytes)
	if err != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("incoming cert: decode failed", log.Err(err))
		}
		return false
	}

	t := rt.Transitive
	t.mu.RLock()
	verifier := t.voteVerifier
	chainID := t.chainID
	pending, exists := t.pendingBlocks[cert.Position.BlockID]
	t.mu.RUnlock()

	if verifier == nil {
		return false
	}
	// The cert's threshold must meet our own floor for its tier: a cert asserting a lower
	// threshold than this chain requires for that tier is rejected even if its internal
	// signatures verify. This is a cheap sub-quorum forgery filter; the authority is
	// verifyCert → VerifyWeighted below, which re-derives the tier threshold from the
	// validator set at the cert's epoch. A gossiped Nova cert legitimately carries a
	// bare-majority threshold below the ⅔ Quasar floor, so the floor is tier-selected:
	// NovaSignerFloor(K) for Nova, the ⅔ count Alpha() for Quasar. K/Alpha track the live
	// committee (construction clamp + reclampCommitteeLocked), so both floors follow the
	// live set. An unknown tier is left to verifyCert, which rejects fail-closed.
	//
	// The Nova floor is the signer floor, not the count majority: on a stake-weighted chain
	// the Nova majority is measured in stake, so a legitimate cert's self-declared threshold
	// is the signer floor and a count-majority pre-filter here would drop it before the
	// authoritative stake check ever ran. On an equal-stake chain the signer floor is ≤ the
	// count majority every legitimate cert already carries, so the filter is unweakened.
	floor := t.consensus.Alpha() // Quasar ⅔ count floor
	if cert.Tier == Nova {
		floor = NovaSignerFloor(t.consensus.K()) // Nova signer floor
	}
	if floor > 0 && cert.Threshold < uint32(floor) {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("incoming cert: threshold below chain floor for its tier",
				log.Stringer("blockID", cert.Position.BlockID),
				log.Uint32("certThreshold", cert.Threshold),
				log.Int("tierFloor", floor))
		}
		return false
	}
	if cert.Position.ChainID != chainID {
		return false
	}

	// Height gate — reject on height before any finalize work:
	//   - a cert at or below the last-finalized height is stale or a fork attempt
	//     (the height is already decided); and
	//   - a cert whose height does not match the height of the block we are
	//     tracking under that ID is internally inconsistent.
	// This is the cheap front-line check; FinalizeBranch (inside AcceptWithCert)
	// is the authoritative backstop that also produces equivocation evidence.
	if fh, set := t.consensus.GetFinalizedHeight(); set && cert.Position.Height <= fh {
		// Equivocation is decided on the canonical commitment, not the outer envelope.
		// Under anyone-can-propose every validator wraps the same inner execution block
		// in its own envelope, so envelope identity says nothing about agreement: a
		// different canonical block already finalized at this height is a potential
		// fork, while the same canonical id under a different envelope is a harmless
		// alias (and an identical envelope is a stale replay). Either way the cert
		// finalizes nothing new.
		certCanonical := cert.Position.CanonicalID
		if certCanonical == ids.Empty {
			certCanonical = cert.Position.BlockID
		}
		finCanonical, have := t.consensus.FinalizedBlockAtHeight(cert.Position.Height)
		if have && finCanonical != certCanonical {
			// A conflicting cert is fork evidence only if it is a verified QC, so run the
			// full predicate (α distinct in-set signatures over the canonical position,
			// stake-weighted when a stake source is wired) before recording any slashing
			// evidence. Naming a validator costs nothing to forge: junk signatures over a
			// random canonical at a decided height would otherwise jail honest validators
			// below quorum and halt the chain — the attack is cheaper than the fork it
			// claims to prove. Resolve the epoch from the tracked block if we have it; on a
			// fixed-set chain the verifier ignores epoch (equal-stake admission), and on a
			// stake-weighted chain an unresolvable epoch fails verification and yields no
			// evidence. Fail-closed here means we may miss a slash, never invent one.
			var epochHeight uint64
			if exists && pending.ConsensusBlock != nil {
				epochHeight = pending.ConsensusBlock.pChainHeight
			}
			if verr := t.verifyCert(cert, epochHeight); verr != nil {
				if !rt.config.Logger.IsZero() {
					rt.config.Logger.Warn("incoming cert: UNVERIFIED conflicting cert at a finalized height — dropping, NO evidence (forged/junk signatures cannot slash)",
						log.Uint64("certHeight", cert.Position.Height), log.Err(verr))
				}
				return false
			}
			// A verified α-of-K cert over a different canonical at a decided height is a
			// genuine, attributable equivocation. Now, and only now, it is evidence.
			rt.reportCertEquivocation(cert, finCanonical)
		} else if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("incoming cert: height at/below finalized; dropping (duplicate or stale, not a fork)",
				log.Uint64("certHeight", cert.Position.Height),
				log.Uint64("finalizedHeight", fh))
		}
		return false
	}
	if exists && pending.ConsensusBlock != nil && pending.ConsensusBlock.height != cert.Position.Height {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("incoming cert: height does not match tracked block; dropping",
				log.Stringer("blockID", cert.Position.BlockID),
				log.Uint64("certHeight", cert.Position.Height),
				log.Uint64("trackedHeight", pending.ConsensusBlock.height))
		}
		return false
	}
	// We found this block by the cert's OUTER id, and the outer ids are the one
	// part of a position no signature covers — the signed identity is the inner
	// canonical. So the id that led us here proves nothing on its own: re-point it
	// at a sibling at the same height and a genuine cert for one block arrives
	// asking us to accept another. The block we found has to BE the block the
	// signatures name. The no-wrapper arm below already compares the full canonical
	// tuple before it finalizes an alias; this is the same question asked on the
	// arm that happens to hold a wrapper.
	if exists && pending.ConsensusBlock != nil && pending.ConsensusBlock.canonicalRep() != certCanonical(cert) {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("incoming cert: canonical does not match the block tracked under this id; dropping",
				log.Stringer("blockID", cert.Position.BlockID),
				log.Stringer("certCanonical", certCanonical(cert)),
				log.Stringer("trackedCanonical", pending.ConsensusBlock.canonicalRep()))
		}
		return false
	}

	// Resolve the cert's epoch height from our locally-tracked, locally-verified
	// block — the proposervm P-chain height we recorded for this block ID. Every
	// honest node derives the same epoch height from the same signed block, so the
	// cert verifies against an identical set/root/stake everywhere.
	//
	// A cert can name an envelope we do not track while we hold a DIFFERENT wrapper
	// of the same inner execution block (the storm alias handled below). The epoch
	// is a property of the inner block, not of the outer envelope, so resolve it by
	// canonical id in that case. Reading it off the envelope alone left the epoch at
	// 0 — the genesis validator set — and on any chain whose set is not the height-0
	// set every voter's pubkey and the whole stake tally resolved against the wrong
	// committee, so the cert failed verification here and the alias cure below was
	// unreachable exactly on the chains old enough to need it. No wrapper of the
	// inner block at all leaves the epoch at 0, which is what a node that has never
	// seen the block can honestly say.
	var epochHeight uint64
	if exists && pending.ConsensusBlock != nil {
		epochHeight = pending.ConsensusBlock.pChainHeight
	} else {
		canon := cert.Position.CanonicalID
		if canon == ids.Empty {
			canon = cert.Position.BlockID
		}
		t.mu.RLock()
		if local, _ := t.pendingByCanonicalLocked(canon); local != nil {
			epochHeight = t.epochHeightLocked(local)
		}
		t.mu.RUnlock()
	}

	// Defence in depth (epoch binding): the cert's set-root must equal the set-root
	// we recompute at our epoch height for this block. The set-root is folded into
	// every signed vote, so a verifying cert already implies the signers agreed on
	// it; this cross-check additionally rejects a cert whose epoch (set-root) does
	// not match the epoch of the block we tracked under this ID — a cert laundered
	// from a different validator-set epoch. nil source ⟹ Empty on both sides (no
	// epoch bound), so this is a no-op for a fixed-set chain.
	t.mu.RLock()
	setRootSrc := t.setRootSource
	t.mu.RUnlock()
	if setRootSrc != nil && exists {
		localRoot := setRootSrc.ValidatorSetRoot(epochHeight)
		if localRoot != cert.Position.ValidatorSetRoot {
			if !rt.config.Logger.IsZero() {
				rt.config.Logger.Warn("incoming cert: set-root does not match our epoch for this block; dropping",
					log.Stringer("blockID", cert.Position.BlockID),
					log.Uint64("epochHeight", epochHeight))
			}
			return false
		}
	}

	// The finality predicate — the same gate the equivocation-evidence path runs
	// (verifyCert): α distinct in-set signatures over the canonical position,
	// stake-weighted to a strict ⅔ when a stake source is wired, count-only
	// otherwise (equal-stake admission). A forged cert dies here.
	if verr := t.verifyCert(cert, epochHeight); verr != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("incoming cert: verification failed",
				log.Stringer("blockID", cert.Position.BlockID), log.Err(verr))
		}
		return false
	}

	// The cert is a valid α-of-K finality proof. Finalize the block if we have
	// verified it locally. If we do not track the block (an eclipsed follower that
	// adopted a losing sibling envelope, or a node behind the frontier), we cannot
	// safely Accept it — and silently dropping a verified finality proof leaves the
	// node stuck with no way back. Trigger a throttled, best-effort catch-up fetch
	// for the certified block so finalization resumes once a reachable peer serves
	// it. The fetch is reachable only after the cert verified above, so a forged
	// cert can never make us fetch arbitrary ids. Peer selection is the node layer's
	// job (EmptyNodeID ⇒ sample a peer); claimCatchupLocked rate-limits the request.
	if !exists {
		// The cert verified as a valid α-of-K witness, but its outer envelope is not one
		// we track. Under pChainHeight=0 anyone-can-propose, every validator wraps the
		// same inner execution block in its own proposervm envelope, so the cert's
		// envelope is an alias of a wrapper we do hold and verified. The signed vote
		// message binds the canonical execution identity, not the outer ids
		// (CanonicalVoteMessage), so the α votes verify against our wrapper's position
		// too — and Accepting any wrapper of the certified inner block applies the same
		// execution. Finalize the local wrapper, so the VM advances on a network-final
		// height instead of stalling while it fetches an alias it already has. Only when
		// we hold no wrapper of this inner block at all do we fall back to catch-up.
		if localID, ok := t.finalizeLocalAliasFromVerifiedCert(cert); ok {
			rt.fastFollowMu.Lock()
			if cert.Position.Height > rt.fastFollowHeight {
				rt.fastFollowHeight = cert.Position.Height
			}
			rt.fastFollowMu.Unlock()
			if !rt.config.Logger.IsZero() {
				rt.config.Logger.Info("finalized local wrapper via α-of-K cert for a sibling envelope (storm-alias)",
					log.Stringer("localEnvelope", localID),
					log.Stringer("certEnvelope", cert.Position.BlockID),
					log.Stringer("canonical", cert.Position.CanonicalID),
					log.Uint64("height", cert.Position.Height),
					log.Int("voters", cert.VoterCount()))
			}
			return true
		}
		rt.requestCatchup(cert.Position.BlockID, ids.EmptyNodeID)
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("incoming cert: valid but block not locally tracked; fetching the certified block",
				log.Stringer("blockID", cert.Position.BlockID))
		}
		return false
	}

	ctx := context.Background()
	if c := t.ctx; c != nil {
		ctx = c
	}

	// Record the cert candidate, then finalize through the one finalizer.
	t.mu.Lock()
	if pending.Decided {
		t.mu.Unlock()
		return false
	}
	pending.cert = cert
	t.mu.Unlock()

	// The cert cleared VerifyWeighted/Verify above (the same predicate
	// BuildVerifiedQuorumCert runs), so promote it to the finality authority token.
	vcert, ok := wrapVerifiedCert(cert)
	if !ok {
		return false
	}

	// AcceptWithCert is the one finalizer: it commits the certified branch through
	// FinalizeBranch (the per-height equivocation gate plus the sibling reorg — prune
	// the losers, accept the path) and applies the VM effects. A safety violation is
	// returned here and nothing is VM-accepted:
	//   - ErrHeightAlreadyFinalized → a second cert at an already-finalized height:
	//     surface the conflict as equivocation evidence (two certs, one height);
	//   - ErrConflictsWithFinalizedBranch → a cert for a losing/pruned branch: drop;
	//   - ErrAncestorNotTracked → we are behind: drop (the gap fetch re-applies it).
	if err := t.AcceptWithCert(ctx, cert.Position.BlockID, vcert); err != nil {
		if errors.Is(err, ErrHeightAlreadyFinalized) {
			if fin, ok := t.consensus.FinalizedBlockAtHeight(cert.Position.Height); ok {
				rt.reportCertEquivocation(cert, fin)
			}
		}
		// ErrAncestorNotTracked means we do track the certified block but are missing an
		// intermediate ancestor between it and our finalized tip — we are behind. A node in
		// that state cannot heal itself by waiting: every later cert hits the same missing
		// ancestor, so it never finalizes again and the set runs one validator short until
		// it restarts. Trigger the throttled ancestor fetch for the specific missing block
		// so the node layer serves the finalized gap oldest-first and this verified cert
		// re-applies. The fetch is reachable only after the cert verified above, so a forged
		// cert can never make us fetch arbitrary ids; claimCatchupLocked rate-limits it.
		// Peer selection is the node layer's job (EmptyNodeID ⇒ sample a serving peer).
		var missingAncestor *AncestorNotTracked
		if errors.As(err, &missingAncestor) {
			rt.requestCatchup(missingAncestor.Missing, ids.EmptyNodeID)
		}
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("incoming cert: REFUSED by finality guard (no VM.Accept)",
				log.Stringer("blockID", cert.Position.BlockID),
				log.Uint64("height", cert.Position.Height), log.Err(err))
		}
		// Roll back the speculative cert cache so a later legitimate finalize of
		// this ID is not confused.
		t.mu.Lock()
		if pd, ok := t.pendingBlocks[cert.Position.BlockID]; ok && !pd.Decided {
			pd.cert = nil
		}
		t.mu.Unlock()
		return false
	}

	rt.fastFollowMu.Lock()
	if cert.Position.Height > rt.fastFollowHeight {
		rt.fastFollowHeight = cert.Position.Height
	}
	rt.fastFollowMu.Unlock()

	// The equivocation guard for this (now-decided) height was already dropped inside
	// the finalizer AcceptWithCert → acceptWithCertCore, the one funnel every finality
	// path shares, so no separate prune is needed here.

	if !rt.config.Logger.IsZero() {
		rt.config.Logger.Info("finalized block via α-of-K quorum cert",
			log.Stringer("blockID", cert.Position.BlockID),
			log.Uint64("height", cert.Position.Height),
			log.Int("voters", cert.VoterCount()))
	}
	return true
}

// finalizeLocalAliasFromVerifiedCert takes an already-verified α-of-K cert whose outer
// envelope this node does not track and finalizes a local wrapper of the same certified
// inner execution block, if we hold one.
//
// It is safe because one certified inner block has one execution, and any wrapper of it
// Accepts that same execution:
//   - It resolves the local wrapper by the cert's canonical id (pendingByCanonicalLocked),
//     then requires the wrapper's full canonical tuple (id + parent-canonical + exec-state
//     root + payload root) to equal the cert's, so it can only ever Accept the exact
//     execution the cert certified, never a colliding-id impostor. A verified local wrapper
//     always satisfies this (its inner execution was re-executed at Verify); the check makes
//     the invariant explicit and fails closed on any drift.
//   - It rebases the cert onto the local wrapper by copying the cert's Position and swapping
//     only the transport ids (BlockID/ParentID). Every signed field — canonical id/parent,
//     exec-state root, payload root, height, round, validator-set root — is unchanged, so the
//     α votes carry over unmodified (outer ids are never in the signed message).
//   - It re-verifies the rebased cert against the local wrapper's epoch before finalizing —
//     the same predicate verifyCert runs (α distinct in-set signatures, ⅔-by-stake when
//     wired) plus the set-root epoch cross-check the exists-path applies — and finalizes
//     only through the one finalizer (AcceptWithCert → FinalizeBranch). A forged or
//     epoch-mismatched cert fails here and we fall back to catch-up. This cannot fork: the
//     per-height gate keys on the canonical id, so a duplicate envelope of an already-final
//     canonical is idempotent.
//
// Returns the finalized local envelope id and true on success. (Empty,false) means either
// no local wrapper of this inner block, or it did not re-verify; the caller then defers to
// catch-up.
func (t *Transitive) finalizeLocalAliasFromVerifiedCert(cert *QuorumCert) (ids.ID, bool) {
	canon := cert.Position.CanonicalID
	if canon == ids.Empty {
		canon = cert.Position.BlockID
	}

	t.mu.RLock()
	pL, localID := t.pendingByCanonicalLocked(canon)
	if pL == nil || pL.ConsensusBlock == nil {
		t.mu.RUnlock()
		return ids.Empty, false
	}
	cb := pL.ConsensusBlock
	// Full canonical-tuple equality — we finalize only a wrapper committing to the exact
	// certified inner execution, never a canonical-id collision with a different execution.
	// Height is bound too: a canonical (inner execution) id is unique per height, so a height
	// mismatch means the resolver crossed heights — fail closed.
	if cb.height != cert.Position.Height ||
		cb.canonicalID != cert.Position.CanonicalID ||
		cb.parentCanonicalID != cert.Position.ParentCanonicalID ||
		cb.execStateRoot != cert.Position.ExecutionStateRoot ||
		cb.payloadRoot != cert.Position.PayloadRoot {
		t.mu.RUnlock()
		return ids.Empty, false
	}
	localParentID := cb.parentID
	// The epoch our wrapper was built at — every honest node that wraps this inner block
	// derives the identical set/root/stake here (the votes were cast under it).
	epochHeight := t.epochHeightLocked(pL)
	setRootSrc := t.setRootSource
	t.mu.RUnlock()

	// Set-root epoch cross-check (parity with the exists-path defense in depth): the cert's
	// bound validator-set root must equal the root WE compute for this epoch, so a cert
	// laundered from a different validator-set epoch is refused even if its signatures verify.
	if setRootSrc != nil {
		if setRootSrc.ValidatorSetRoot(epochHeight) != cert.Position.ValidatorSetRoot {
			return ids.Empty, false
		}
	}

	// Rebase: copy the verified cert's Position, swap only the transport ids to the local
	// wrapper. Every signed field is unchanged, so the votes verify unmodified.
	rebasedPos := cert.Position
	rebasedPos.BlockID = localID
	rebasedPos.ParentID = localParentID

	rebased, err := AssembleQuorumCert(rebasedPos, cert.Tier, cert.Threshold, cert.Votes)
	if err != nil {
		return ids.Empty, false
	}
	// The rebased cert must clear the same α-of-K predicate. Because the signed message is
	// byte-identical to the incoming cert's (outer ids excluded), this passes exactly when
	// the incoming cert did — re-run for defence in depth, since a nil verifier or a stake
	// shortfall fails closed.
	if verr := t.verifyCert(rebased, epochHeight); verr != nil {
		return ids.Empty, false
	}
	vcert, ok := wrapVerifiedCert(rebased)
	if !ok {
		return ids.Empty, false
	}

	ctx := context.Background()
	if c := t.ctx; c != nil {
		ctx = c
	}
	if err := t.AcceptWithCert(ctx, localID, vcert); err != nil {
		return ids.Empty, false
	}
	return localID, true
}

// verifyCert runs the full finality predicate — α distinct in-set signatures over the
// cert's canonical position, stake-weighted to a strict ⅔ when a stake source is wired,
// count-only otherwise. It is the single gate a cert passes before it counts either as
// finality (the finalize path) or as equivocation evidence (the height-gate fork path):
// a forged cert with junk signatures fails here and can neither finalize a block nor
// slash a validator. epochHeight is the P-chain height the per-voter pubkeys and stake
// are resolved at. A nil verifier fails closed.
func (t *Transitive) verifyCert(cert *QuorumCert, epochHeight uint64) error {
	t.mu.RLock()
	verifier := t.voteVerifier
	stake := t.stakeSource
	t.mu.RUnlock()
	if verifier == nil {
		return ErrQCVerifierNil
	}
	if stake != nil {
		return cert.VerifyWeighted(verifier, stake, epochHeight)
	}
	return cert.Verify(verifier, epochHeight)
}

// reportCertEquivocation records that a second, conflicting finality cert was
// presented for a height already finalized to a different canonical commitment — a
// provable safety equivocation: two valid certs selecting different execution blocks
// at one height. `finalizedCanonical` is the canonical id already final at this
// height; the conflicting cert's canonical id differs. Evidence is keyed on canonical
// identity, so a duplicate envelope wrapping the same canonical block cannot reach
// here — an alias is agreement, not a fork, and slashing on it would punish honest
// validators. Each voter is recorded as a DoubleVote. Best effort: it never blocks
// the safety reject.
func (rt *Runtime) reportCertEquivocation(cert *QuorumCert, finalizedCanonical ids.ID) {
	conflicting := cert.Position.CanonicalID
	if conflicting == ids.Empty {
		conflicting = cert.Position.BlockID
	}
	if !rt.config.Logger.IsZero() {
		rt.config.Logger.Crit("EQUIVOCATION: conflicting finality cert at finalized height (different canonical block)",
			log.Uint64("height", cert.Position.Height),
			log.Stringer("finalizedCanonical", finalizedCanonical),
			log.Stringer("conflictingCanonical", conflicting),
			log.Stringer("conflictingEnvelope", cert.Position.BlockID),
			log.Int("conflictingVoters", cert.VoterCount()))
	}
	t := rt.Transitive
	t.mu.RLock()
	sdb := t.slashingDB
	t.mu.RUnlock()
	if sdb == nil {
		return
	}
	for i := range cert.Votes {
		proof := fmt.Appendf(nil, "height=%d finalizedCanonical=%s conflictingCanonical=%s",
			cert.Position.Height, finalizedCanonical, conflicting)
		sdb.RecordEvidence(slashing.Evidence{
			Type:        slashing.DoubleVote,
			ValidatorID: cert.Votes[i].NodeID,
			Height:      cert.Position.Height,
			Timestamp:   time.Now(),
			Proof:       proof,
		})
	}
}

// certCanonical is the execution identity a cert's signatures actually cover.
// An empty CanonicalID means the position names a bare block, whose canonical is
// its own id — the same resolution every other reader of a position uses.
func certCanonical(cert *QuorumCert) ids.ID {
	if cert.Position.CanonicalID != ids.Empty {
		return cert.Position.CanonicalID
	}
	return cert.Position.BlockID
}
