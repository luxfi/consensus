// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_topology.go — the vote/cert distribution handlers that make α-of-K
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
	// This is the cheap front-line check; the per-height guard in AcceptViaCert
	// is the authoritative backstop that also produces equivocation evidence.
	if fh, set := t.consensus.GetFinalizedHeight(); set && cert.Position.Height <= fh {
		// If a DIFFERENT block is finalized at this exact height, the cert is an
		// equivocation proof — surface it. (Same block = harmless stale replay.)
		if fin, ok := t.consensus.FinalizedBlockAtHeight(cert.Position.Height); ok && fin != cert.Position.BlockID {
			rt.reportCertEquivocation(cert, fin)
		} else if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("incoming cert: height at/below finalized; dropping",
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
	stake := t.stakeSource
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

	// On a stake-weighted chain the incoming cert must clear the ⅔-of-stake
	// supermajority (HIGH-3), not just the count predicate — otherwise a
	// low-stake coalition's count-α cert would finalize. Count-only chains use
	// Verify (equal-stake invariant enforced at admission).
	var verr error
	if stake != nil {
		verr = cert.VerifyWeighted(verifier, stake, epochHeight)
	} else {
		verr = cert.Verify(verifier, epochHeight)
	}
	if verr != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("incoming cert: verification failed",
				log.Stringer("blockID", cert.Position.BlockID), log.Err(verr))
		}
		return false
	}

	// Cert is a valid α-of-K finality proof. Finalize the block IF we have
	// verified it locally. If we have not yet seen/verified the block, we cannot
	// safely Accept it — record nothing; the block gossip will arrive and the
	// cert can be re-requested/re-applied. (A production node may also fetch the
	// block by ID here; that fetch path is the node layer's responsibility.)
	if !exists {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("incoming cert: valid but block not locally verified yet; deferring",
				log.Stringer("blockID", cert.Position.BlockID))
		}
		return false
	}

	ctx := context.Background()
	if c := t.ctx; c != nil {
		ctx = c
	}

	// Record the cert candidate, then commit through the per-height guard. The
	// guard is the AUTHORITATIVE single-finalize gate: it finalizes the first
	// block at a height and REFUSES a second (different) one — even if the cert
	// itself verified — returning ErrHeightAlreadyFinalized, which we surface as
	// equivocation evidence. Only on a clean accept do we run the shared finalize
	// path (VM.Accept + SetPreference). This closes the two-certs-one-height fork.
	t.mu.Lock()
	if pending.Decided {
		t.mu.Unlock()
		return false
	}
	pending.cert = cert
	t.mu.Unlock()

	if err := t.consensus.AcceptViaCert(cert.Position.BlockID, cert.Position.Height, cert.Position.ParentID); err != nil {
		// The cert verified but violates a per-height finalization invariant.
		// This is the fork the guard exists to stop — do NOT VM.Accept.
		if errors.Is(err, ErrHeightAlreadyFinalized) {
			if fin, ok := t.consensus.FinalizedBlockAtHeight(cert.Position.Height); ok {
				rt.reportCertEquivocation(cert, fin)
			}
		}
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("incoming cert: REFUSED by per-height finality guard (no VM.Accept)",
				log.Stringer("blockID", cert.Position.BlockID),
				log.Uint64("height", cert.Position.Height), log.Err(err))
		}
		// Roll back the speculative cert cache so a later legitimate finalize of
		// this (already-decided-elsewhere) ID is not confused.
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

	// The cert cleared VerifyWeighted/Verify above (the SAME predicate
	// BuildVerifiedQuorumCert runs) and the per-height guard via AcceptViaCert, so
	// promote it to the finality authority token and finalize through the SOLE
	// finalizer. wrapVerifiedCert refuses only a nil cert; this cert is non-nil.
	vcert, ok := wrapVerifiedCert(cert)
	if !ok {
		return false
	}
	_ = t.AcceptWithCert(ctx, cert.Position.BlockID, vcert)
	if !rt.config.Logger.IsZero() {
		rt.config.Logger.Info("finalized block via α-of-K quorum cert",
			log.Stringer("blockID", cert.Position.BlockID),
			log.Uint64("height", cert.Position.Height),
			log.Int("voters", cert.VoterCount()))
	}
	return true
}

// reportCertEquivocation records that a SECOND, conflicting finality cert was
// presented for a height already finalized to a DIFFERENT block — a provable
// safety-equivocation. Each voter in the conflicting cert signed-final a block
// at a height already finalized to `finalized`, so each is recorded as a
// DoubleVote (it asserts a different block at the same height than the one that
// finalized).
//
// LOGGED AT ERROR — NOT Crit. DO NOT "upgrade" this back to Crit. The conflict
// is a Byzantine-fault signal, but safety has ALREADY been preserved by the time
// we get here: the per-height guard (AcceptViaCert → ErrHeightAlreadyFinalized,
// or the GetFinalizedHeight gate in HandleIncomingCert) REJECTED the second cert,
// so there is no double-Accept and no fork. luxfi/log Crit is Fatal → os.Exit(1):
// logging a CORRECTLY-HANDLED safety event at Crit converts it into a FLEET-WIDE
// LIVENESS KILL — every honest node that merely OBSERVES the conflicting cert
// (which is gossiped to all of them) calls os.Exit and the whole network goes
// down at once. It also made the slashing-evidence recording below DEAD CODE: the
// process exited before the loop ran, so the Byzantine voters were never even
// slashed. The correct BFT response to a handled safety event is to log it and
// record evidence so the offender is slashed — a safety event must NEVER be a
// self-DoS. Best effort: never blocks the safety reject.
func (rt *Runtime) reportCertEquivocation(cert *QuorumCert, finalized ids.ID) {
	if !rt.config.Logger.IsZero() {
		rt.config.Logger.Error("EQUIVOCATION: conflicting finality cert at finalized height",
			log.Uint64("height", cert.Position.Height),
			log.Stringer("finalizedBlock", finalized),
			log.Stringer("conflictingBlock", cert.Position.BlockID),
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
		proof := fmt.Appendf(nil, "height=%d finalized=%s conflicting=%s",
			cert.Position.Height, finalized, cert.Position.BlockID)
		sdb.RecordEvidence(slashing.Evidence{
			Type:        slashing.DoubleVote,
			ValidatorID: cert.Votes[i].NodeID,
			Height:      cert.Position.Height,
			Timestamp:   time.Now(),
			Proof:       proof,
		})
	}
}
