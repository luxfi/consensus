// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// attestation.go — Quasar, the post-acceptance ATTESTATION sidecar.
//
// THE DECOMPLECTION (owner decision, 2026-07-09). We had built the finality certificate as a
// SECOND consensus — Tendermint rounds, prevotes, POL, locks, view-change — with its own
// liveness, layered on top of the metastable sampler (Nova). Two deciders braided is where
// every incident lived (the static committee, the cert-receive alias, the stale-alias lock).
// But the artifact we actually need — a portable ⅔-stake quorum cert for bridges and the
// 0x9999 DEX settlement receipt — is an ATTESTATION OF AN OUTCOME, not a decider. The sampler
// already decides. So the cert demotes to this sidecar:
//
//   - Nova (metastable sampling) is THE ONE decider. A block is ACCEPTED by β-confidence; that
//     is what advances lastAccepted and drives VM.Accept. Chain liveness == sampling liveness.
//   - Quasar (this file) runs strictly AFTER Accept. Having accepted inner block B at height H,
//     a node signs the finality message for B and gossips it. A collector aggregates the
//     signatures of ≥⅔ of the LIVE stake into a certificate for external consumers. The cert
//     TRAILS acceptance and NEVER gates it — an absent or late cert cannot stall the chain.
//
// WHY IT IS SAFE (the reduction). Honest nodes sign ONLY what they accepted. Sampling accepts
// at most one block per height (w.h.p. under the metastable adversary bound). A conflicting
// cert for B'≠B at height H needs ≥α signers on B'; with α=⌊2n/3⌋+1 and Byzantine stake f<⅓,
// every α-set holds >⅓ honest stake, and those honest nodes accepted B, not B' — so they never
// signed B'. The cert's safety therefore REDUCES to sampling's single-accept-per-height plus
// the same ≥⅔-honest-stake assumption the old α-of-K cert already required: no worse, and with
// NO added liveness dependency (nothing waits on the cert).
//
// INNER-CANONICAL BY CONSTRUCTION. A node attests only after it accepted the INNER block, so
// the outer proposervm envelope does not exist at this layer — the whole stale-alias class
// (an outer id where an inner id belongs) cannot arise here. The lock rebase, committedSlot,
// vote-guard, and view-change machinery become unnecessary, not merely fixed.
//
// DRY. This reuses the finality crypto UNCHANGED: the signed message is CanonicalVoteMessage
// (binds the inner canonical identity + epoch set-root, excludes the outer envelope), the
// cert is a QuorumCert, aggregation is AssembleQuorumCert, and external verification is
// QuorumCert.VerifyWeighted (⅔-by-stake at the epoch). An attestation is therefore
// byte-identical to a legacy finality accept vote — only WHEN it is cast changes (post-Accept,
// deterministically the accepted block; no rounds, no locks, no POL). External verifiers are
// unaffected by the engine swap.

package chain

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/luxfi/ids"
)

// quasarQuorum is the BFT count floor α = ⌊2n/3⌋+1 over the LIVE committee size n. There is no
// static K/α: n comes from the validator set at the block's epoch each height, so the quorum
// tracks the live set (a 1-of-1 for n=1, 2-of-2 for n=2, ⌊2n/3⌋+1 thereafter). The authoritative
// gate is still ⅔-by-STAKE (VerifyWeighted); this count floor makes the assembled cert satisfy
// the count clause too, and selects when to attempt assembly.
func quasarQuorum(n int) int {
	if n < 1 {
		return 1
	}
	return 2*n/3 + 1
}

// attestKey identifies the finality subject: (height, inner canonical execution id). The outer
// envelope id is ABSENT by construction — attestation is post-Accept, over the inner block.
type attestKey struct {
	height    uint64
	canonical ids.ID
}

// attestBucket accumulates the verified attestations for ONE subject. All votes in a bucket
// bind the byte-identical CanonicalVoteMessage(pos) (enforced on ingest), so AssembleQuorumCert
// over them yields a cert that verifies.
type attestBucket struct {
	pos   VotePosition
	msg   []byte // CanonicalVoteMessage(pos), cached; the bucket's fixed signed message
	votes map[ids.NodeID]SignedVote
}

// QuasarAttestor collects post-acceptance attestations into a ⅔-stake finality certificate for
// external consumers. It is an ARTIFACT builder, never a decider: no acceptance path, no
// lock/round/committedSlot state, no durable consensus state. Safe for concurrent use.
type QuasarAttestor struct {
	mu sync.Mutex

	verifier VoteVerifier // verifies a peer attestation's signature at the block's epoch
	stake    StakeSource  // ⅔-by-stake threshold + live committee size at the epoch

	buckets  map[attestKey]*attestBucket    // in-flight, pruned when a cert emits at the height
	certs    map[uint64]*QuorumCert         // emitted artifacts, by height (the external cert chain)
	subjects map[uint64]map[ids.ID]struct{} // distinct canonicals attested per height (equivocation evidence)
}

// NewQuasarAttestor builds the sidecar. verifier + stake are the SAME finality-crypto
// dependencies the engine already wires. The block's P-chain EPOCH is a VALUE the caller binds
// to each Ingest (bound to the position it verifies), NOT a shared height→epoch place — so two
// concurrent ingests of same-height forked blocks under different epochs can never cross epochs.
// A nil stake source is rejected at emit time (fail-closed — a cert must be ⅔-stake checkable).
func NewQuasarAttestor(verifier VoteVerifier, stake StakeSource) *QuasarAttestor {
	return &QuasarAttestor{
		verifier: verifier,
		stake:    stake,
		buckets:  map[attestKey]*attestBucket{},
		certs:    map[uint64]*QuorumCert{},
		subjects: map[uint64]map[ids.ID]struct{}{},
	}
}

// Attest is the POST-ACCEPT hook: this node, having ALREADY accepted the block at pos via the
// sampler, signs the finality message and returns its own attestation to gossip. It performs NO
// acceptance decision and has NO side effect on the chain — a signing failure is returned to the
// caller and simply means this node contributes no attestation (best-effort; the chain does not
// care). The returned SignedVote is byte-compatible with a legacy finality accept vote.
func (q *QuasarAttestor) Attest(pos VotePosition, signer VoteSigner, self ids.NodeID) (SignedVote, error) {
	if signer == nil {
		return SignedVote{}, fmt.Errorf("quasar attest: nil signer")
	}
	sig, err := signer.SignVote(CanonicalVoteMessage(pos))
	if err != nil {
		return SignedVote{}, fmt.Errorf("quasar attest: sign: %w", err)
	}
	return SignedVote{NodeID: self, Accept: true, Signature: sig}, nil
}

// Ingest verifies and records one attestation (own or peer) for the accepted block at pos, at
// the block's P-chain `epoch` (a VALUE bound to this position — the SAME epoch the accept votes
// were verified under; NOT a shared height-keyed place). Returns the emitted certificate the
// FIRST time this subject reaches the ⅔-stake threshold (nil, false otherwise). It NEVER accepts
// a block and NEVER blocks — its only outputs are a verified-attestation record and, once enough
// stake has attested, a cert.
//
// Fail-closed: an attestation whose signature does not verify at the block's epoch, or a
// non-accept vote, or one whose message disagrees with an established bucket, is dropped.
func (q *QuasarAttestor) Ingest(pos VotePosition, epoch uint64, att SignedVote) (*QuorumCert, bool, error) {
	if !att.Accept {
		return nil, false, fmt.Errorf("quasar ingest: non-accept attestation from %s", att.NodeID)
	}
	canonical := pos.CanonicalID
	if canonical == ids.Empty {
		canonical = pos.BlockID // bare-block degrade (inner == outer)
	}
	msg := CanonicalVoteMessage(pos)

	if q.verifier == nil || !q.verifier.VerifyVote(att.NodeID, msg, att.Signature, epoch) {
		return nil, false, fmt.Errorf("quasar ingest: signature failed verification (voter %s height %d)", att.NodeID, pos.Height)
	}
	// A nil stake source is a configuration that cannot certify anything, and
	// this is fail-closed by DESIGN. It has to be a rejection here, not a nil
	// dereference at q.stake.ValidatorCount below: every entry point takes peer
	// data, so the deref is a remote crash on any node wired without stake.
	if q.stake == nil {
		return nil, false, fmt.Errorf("quasar ingest: no stake source configured")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Record the distinct-canonical evidence for the height BEFORE the
	// already-certified check. A double-signer racing the honest cert gains
	// nothing, so the ordering it picks is to sign the second execution AFTER the
	// height certifies — and returning on `done` before recording the subject
	// dropped exactly that evidence. The signature verified; only the record of
	// it was skipped. Evidence is a property of the signed votes, not of whether
	// a cert already exists, so it is recorded either way.
	if q.subjects[pos.Height] == nil {
		q.subjects[pos.Height] = map[ids.ID]struct{}{}
	}
	q.subjects[pos.Height][canonical] = struct{}{}

	// Already certified at this height → no new cert to emit, but the evidence
	// above is kept.
	if _, done := q.certs[pos.Height]; done {
		return nil, false, nil
	}

	key := attestKey{height: pos.Height, canonical: canonical}
	b := q.buckets[key]
	if b == nil {
		b = &attestBucket{pos: pos, msg: msg, votes: map[ids.NodeID]SignedVote{}}
		q.buckets[key] = b
	} else if !bytes.Equal(b.msg, msg) {
		// Same (height, canonical) but a DIFFERENT signed message (e.g. a disagreeing set-root
		// or state-root) — cannot aggregate into one cert; drop, fail-closed.
		return nil, false, fmt.Errorf("quasar ingest: attestation message mismatch for canonical %s at height %d", canonical, pos.Height)
	}
	cp := att
	cp.Signature = append([]byte(nil), att.Signature...)
	b.votes[att.NodeID] = cp

	// Try to emit once the count floor is reached; the AUTHORITATIVE gate is ⅔-by-stake
	// (VerifyWeighted). n is the LIVE committee at the epoch (no static K/α).
	n := q.stake.ValidatorCount(epoch)
	threshold := quasarQuorum(n)
	if len(b.votes) < threshold {
		return nil, false, nil
	}
	votes := make([]SignedVote, 0, len(b.votes))
	for _, v := range b.votes {
		votes = append(votes, v)
	}
	cert, err := AssembleQuorumCert(b.pos, Quasar, uint32(threshold), votes)
	if err != nil {
		return nil, false, nil // not yet assemblable (below threshold after dedup); keep collecting
	}
	// The ⅔-stake predicate is the real finality gate for external consumers.
	if err := cert.VerifyWeighted(q.verifier, q.stake, epoch); err != nil {
		return nil, false, nil // count floor reached but stake supermajority not yet — keep collecting
	}
	q.certs[pos.Height] = cert
	delete(q.buckets, key) // artifact emitted; stop accumulating for this subject
	return cert, true, nil
}

// CertAt returns the emitted finality certificate at a height, if the ⅔-stake attestation has
// formed. This is the external artifact bridges and the 0x9999 DEX receipt consume.
func (q *QuasarAttestor) CertAt(height uint64) (*QuorumCert, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	c, ok := q.certs[height]
	return c, ok
}

// EquivocationEvidenceAt returns the distinct inner canonical ids that have been ATTESTED at a
// height. More than one is evidence that stake signed two different blocks at one height —
// which, under honest-sign-only + the ⅔ threshold, can never produce two certs, but IS
// slashable evidence. One (or zero) is the healthy case.
func (q *QuasarAttestor) EquivocationEvidenceAt(height uint64) ([]ids.ID, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	s, ok := q.subjects[height]
	if !ok || len(s) == 0 {
		return nil, false
	}
	out := make([]ids.ID, 0, len(s))
	for id := range s {
		out = append(out, id)
	}
	return out, len(out) > 1
}

// PruneBelow drops all in-flight and emitted state strictly below height (bounded memory). The
// external cert chain is expected to be persisted by its consumer; the attestor keeps only a
// live window. No durable consensus state is involved — recovery is boot-from-lastAccepted and
// re-attest the frontier.
func (q *QuasarAttestor) PruneBelow(height uint64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for k := range q.buckets {
		if k.height < height {
			delete(q.buckets, k)
		}
	}
	for h := range q.certs {
		if h < height {
			delete(q.certs, h)
		}
	}
	for h := range q.subjects {
		if h < height {
			delete(q.subjects, h)
		}
	}
}
