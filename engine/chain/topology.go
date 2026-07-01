// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// topology.go — the vote/cert distribution handlers that make α-of-K
// finality both SAFE and LIVE.
//
// The topology in one paragraph: a follower verifies a gossiped block and
// BROADCASTS its signed accept vote to ALL validators (followVerifiedBlock,
// integration.go). Every validator feeds incoming signed votes into the engine
// (HandleIncomingVote), which collects them toward a QuorumCert. Whichever node
// first collects α distinct verified votes assembles the cert, GOSSIPS it
// (tryFinalizeBlock), and every node finalizes the block on receipt of that
// verifiable proof (HandleIncomingCert). No node finalizes without a cert
// (safety); the cert reaches everyone via vote-broadcast + cert-gossip + the
// poll-timeout re-request, so finality never hinges on one node's inbound Chits
// (liveness — the proposer-freeze cannot recur).
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
// rebuilds is derived from the RECEIVER's tracked position for that block — a
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
// envelope); the engine rebuilds the canonical message from ITS OWN tracked
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

	if !exists || verifier == nil {
		return false
	}

	// Verify the signature against OUR position for this block, resolving the
	// voter's pubkey at the block's P-CHAIN epoch height (RESIDUAL-B). A vote that
	// signed a different position (different height/round/parent/set-root) fails.
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
// finalizes that block. This is the SAFE replacement for fast-follow: a
// follower commits a gossiped block ONLY against a verifiable α-of-K proof.
//
// Safety gates:
//   - the cert must decode and Verify under our VoteVerifier (α distinct
//     correctly-signed accepts over the cert's position),
//   - the cert's position chain must match ours,
//   - we must have VERIFIED the block locally (it is in pendingBlocks). We do
//     NOT accept a block whose contents we have not validated, even with a
//     valid cert — the cert proves agreement, local Verify proves validity;
//     both are required.
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
	// The cert's threshold MUST meet our own α floor — a cert that asserts a
	// LOWER threshold than this chain requires is rejected even if its internal
	// signatures verify (sub-quorum finality forgery defence, mirrors quasar's
	// MinThreshold floor).
	if alpha := t.consensus.Alpha(); alpha > 0 && cert.Threshold < uint32(alpha) {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("incoming cert: threshold below chain alpha floor",
				log.Stringer("blockID", cert.Position.BlockID),
				log.Uint32("certThreshold", cert.Threshold),
				log.Int("alphaFloor", alpha))
		}
		return false
	}
	if cert.Position.ChainID != chainID {
		return false
	}

	// MED-5 HEIGHT GATE — reject the cert on height BEFORE any finalize work:
	//   - a cert at or below the last-finalized height is stale or a fork attempt
	//     (the height is already decided); and
	//   - a cert whose height does not match the height of the block we are
	//     tracking under that ID is internally inconsistent.
	// This is the cheap front-line check; FinalizeBranch (inside AcceptWithCert)
	// is the authoritative backstop that also produces equivocation evidence.
	if fh, set := t.consensus.GetFinalizedHeight(); set && cert.Position.Height <= fh {
		// Equivocation is decided on the CANONICAL commitment, NEVER the outer envelope
		// (the incident-1082814 fix). A DIFFERENT canonical block already finalized at
		// this height is a POTENTIAL fork; the SAME canonical id under a different
		// envelope is a harmless DUPLICATE alias (and an identical envelope is a stale
		// replay). Either way the cert finalizes nothing new.
		certCanonical := cert.Position.CanonicalID
		if certCanonical == ids.Empty {
			certCanonical = cert.Position.BlockID
		}
		finCanonical, have := t.consensus.FinalizedBlockAtHeight(cert.Position.Height)
		if have && finCanonical != certCanonical {
			// HARD GATE on equivocation STATE (H1): a conflicting cert is fork EVIDENCE
			// only if it is a VERIFIED QC. Run the FULL predicate (α distinct in-set
			// signatures over the canonical position, stake-weighted when wired) BEFORE
			// recording ANY slashing evidence. The pre-fix code recorded a DoubleVote per
			// NAMED voter here before checking a single signature — so a forged cert (junk
			// signatures naming honest validators, a random different canonical, height ≤
			// finalized) could jail honest validators below quorum and re-halt the chain.
			// Resolve the epoch from the tracked block if we have it; on a fixed-set chain
			// the verifier ignores epoch (equal-stake admission invariant), and on a
			// stake-weighted chain an unresolvable epoch fails verification → no evidence
			// (fail-closed: we never false-slash, we may miss-slash).
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
			// A VERIFIED α-of-K cert over a DIFFERENT canonical at a decided height — a
			// genuine, attributable equivocation. Now (and only now) it is evidence.
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

	// Resolve the cert's epoch height (MEDIUM-1) from OUR locally-tracked,
	// locally-verified block — the proposervm P-chain height we recorded for this
	// block ID. Every honest node derives the same epoch height from the same
	// signed block, so the cert verifies against the IDENTICAL set/root/stake on
	// every node. A cert for a block we have not yet tracked is deferred below
	// (we cannot know its epoch — and we must not Accept an unverified block
	// anyway), so for a stake-weighted finalize `exists` is required here.
	var epochHeight uint64
	if exists && pending.ConsensusBlock != nil {
		epochHeight = pending.ConsensusBlock.pChainHeight
	}

	// Defence in depth (epoch binding): the cert's set-root MUST equal the set-root
	// WE recompute at our epoch height for this block. The set-root is folded into
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

	// THE finality predicate — the SAME gate the equivocation-evidence path runs
	// (verifyCert): α distinct in-set signatures over the canonical position,
	// stake-weighted to a strict ⅔ when a stake source is wired (HIGH-3), count-only
	// otherwise (equal-stake admission invariant). A forged cert dies here.
	if verr := t.verifyCert(cert, epochHeight); verr != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("incoming cert: verification failed",
				log.Stringer("blockID", cert.Position.BlockID), log.Err(verr))
		}
		return false
	}

	// Cert is a valid α-of-K finality proof. Finalize the block IF we have
	// verified it locally. If we do NOT track the block (an eclipsed follower that
	// adopted a losing sibling envelope, or a node behind the frontier), we cannot
	// safely Accept it — but we must NOT silently drop a VERIFIED finality proof
	// either (M1). Trigger a throttled, best-effort catch-up fetch for the certified
	// block so finalization resumes once a reachable peer serves it; the fetch is
	// gated on the cert having VERIFIED above, so a forged cert can never make us
	// fetch arbitrary ids. Peer selection is the node layer's job (EmptyNodeID ⇒
	// sample a peer); claimCatchupLocked rate-limits the request.
	if !exists {
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

	// Record the cert candidate, then finalize through the SOLE finalizer.
	t.mu.Lock()
	if pending.Decided {
		t.mu.Unlock()
		return false
	}
	pending.cert = cert
	t.mu.Unlock()

	// The cert cleared VerifyWeighted/Verify above (the SAME predicate
	// BuildVerifiedQuorumCert runs), so promote it to the finality authority token.
	vcert, ok := wrapVerifiedCert(cert)
	if !ok {
		return false
	}

	// AcceptWithCert is the SOLE finalizer: it commits the certified branch through
	// FinalizeBranch (the per-height equivocation gate + the sibling REORG: prune the
	// losers, accept the path) and applies the VM effects. A safety violation is
	// returned here and NOTHING is VM-accepted:
	//   - ErrHeightAlreadyFinalized → a SECOND cert at an already-finalized height:
	//     surface the conflict as equivocation evidence (the two-certs-one-height fork);
	//   - ErrConflictsWithFinalizedBranch → a cert for a losing/pruned branch: drop;
	//   - ErrAncestorNotTracked → we are behind: drop (the gap fetch re-applies it).
	if err := t.AcceptWithCert(ctx, cert.Position.BlockID, vcert); err != nil {
		if errors.Is(err, ErrHeightAlreadyFinalized) {
			if fin, ok := t.consensus.FinalizedBlockAtHeight(cert.Position.Height); ok {
				rt.reportCertEquivocation(cert, fin)
			}
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
	// the finalizer AcceptWithCert → acceptWithCertCore (the one funnel every finality
	// path shares — MEDIUM-1), so no separate prune is needed here.

	if !rt.config.Logger.IsZero() {
		rt.config.Logger.Info("finalized block via α-of-K quorum cert",
			log.Stringer("blockID", cert.Position.BlockID),
			log.Uint64("height", cert.Position.Height),
			log.Int("voters", cert.VoterCount()))
	}
	return true
}

// verifyCert runs the FULL finality predicate — α distinct in-set signatures over
// the cert's CANONICAL position, stake-weighted to a strict ⅔ when a stake source is
// wired (HIGH-3), count-only otherwise. It is the SINGLE gate a cert must pass before
// it is treated as finality (the finalize path) OR as equivocation evidence (the
// height-gate fork path, H1): a forged cert with junk signatures fails here and can
// neither finalize a block nor slash a validator. epochHeight is the P-chain height
// the per-voter pubkeys / stake are resolved at (MEDIUM-1). A nil verifier fails
// closed.
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

// reportCertEquivocation records that a SECOND, conflicting finality cert was
// presented for a height already finalized to a DIFFERENT CANONICAL commitment — a
// provable safety-equivocation (a genuine fork: two valid certs select different
// execution blocks at one height). `finalizedCanonical` is the canonical id already
// final at this height; the conflicting cert's canonical id differs. This is keyed
// on canonical identity, so a duplicate ENVELOPE wrapping the same canonical block
// NEVER reaches here (that is the bug this whole change removes). Each voter is
// recorded as a DoubleVote. Logged at CRIT — a Byzantine-fault signal. Best effort:
// never blocks the safety reject.
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
