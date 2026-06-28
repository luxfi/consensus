// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// pulsar_sampled_subject.go — the canonical finality SUBJECT M that EVERY
// sampled Mithril committee signs.
//
// # One M, signed by all r-of-m committees
//
// The Pulsar sampled certificate is the ML-DSA analogue of Avalanche's
// repeated-small-sample path: m unpredictably-sampled dealerless Mithril
// committees each emit ONE ordinary FIPS-204 ML-DSA group signature over the
// SAME subject M, and r of them must agree. For that "agreement" to be
// meaningful every committee must sign the byte-identical M — this file builds
// that one M, and is the single authority for its layout.
//
// # What M binds (and why each field)
//
//	M = H( "PULSAR_SAMPLED_SUBJECT_V1"
//	       ‖ chainID ‖ height ‖ round ‖ blockID ‖ stateRoot   // the finalized position
//	       ‖ beamQCHash                                        // the Beam QC over THIS block
//	       ‖ signerSetID ‖ pChainHeight                        // the P-Chain-pinned signer set
//	       ‖ policyID                                          // the finality posture
//	       ‖ committeePlanHash )                               // the frozen sampling plan
//
//   - chainID/height/round/blockID/stateRoot pin the exact consensus position,
//     so a committee signature for one position can never be replayed for
//     another (cross-position non-transferability).
//   - beamQCHash binds the Beam quorum certificate that finalized THIS block:
//     the sampled cert attests "I am post-quantum evidence for the block Beam
//     already certified via this exact QC." It is a commitment to the Beam QC,
//     NOT signed by Beam (that would be circular) — Beam signs the position;
//     the Pulsar sampled cert signs the position-plus-Beam-QC. A sampled cert is
//     therefore non-transferable across different Beam QCs for the same block.
//   - signerSetID/pChainHeight pin the validator set the committees were sampled
//     from, at the P-Chain height it was pinned at.
//   - policyID is the finality posture (FAST / HYBRID_PQ / PQ_ROOT); a signature
//     under one posture's M cannot satisfy another posture.
//   - committeePlanHash is the single commitment to the ENTIRE sampling plan
//     (n, t, m, r, selectionAlgorithmID, sortitionSeed, committeeKeyEraRoot — see
//     pulsar_sortition.go). Binding it into M is what makes the committees
//     UN-RESELECTABLE: a different plan hashes differently, yields a different M,
//     and the committee signatures over the old M will not verify against it. An
//     adversary cannot wait to see the block and then re-sample committees it
//     controls — the plan is frozen into the very thing the committees sign.
//
// # Distinct subject domain — non-transferability vs the envelope Pulsar leg
//
// The canonical single-committee Pulsar threshold leg (quasar_finality.go,
// QuasarFinalityMessage) signs the envelope finality message under the
// "Lux/ConsensusCert/v1" customization. The sampled subject deliberately uses a
// DISTINCT domain tag AND a DISTINCT cSHAKE customization
// ("Lux/PulsarSampledSubject/v1"), so the two messages can never collide: a
// single-committee threshold signature is NEVER a valid sampled-committee
// signature and vice versa. They are different evidence types attesting the same
// finalized block by different mechanisms, and the subject domain separates them.
// "QUASAR_FINALITY_V1" remains the canonical NAME of the finalized-position value
// both bind; the sampled subject binds that position plus the sampled-specific
// commitments under its own domain.
//
// # Decomplected
//
// This file owns ONLY the subject construction. It decides nothing about which
// committees are sampled (pulsar_sortition.go), whether a cert verifies
// (pulsar_sampled_verify.go), the security budget (pulsar_sampled_security.go),
// or the finality posture (pulsar_sampled_policy.go).
package quasar

// Domain-separation constants for the sampled-cert subject. The domain tag is
// the first absorbed TupleHash part; the customization is the cSHAKE256
// customization string. Both differ from every other quasar subject, so no
// signature over this subject can be reinterpreted under another context.
const (
	pulsarSampledSubjectDomain = "PULSAR_SAMPLED_SUBJECT_V1"
	pulsarSampledSubjectCustom = "Lux/PulsarSampledSubject/v1"
)

// PulsarSampledSubjectParams are the explicit finality values that, together
// with the frozen committeePlanHash, define the one subject M every sampled
// committee signs. It is a pure value — the single argument to PulsarSampledSubject.
type PulsarSampledSubjectParams struct {
	// ChainID/Height/Round/BlockID/StateRoot pin the finalized consensus position.
	ChainID   uint32
	Height    uint64
	Round     uint32
	BlockID   [32]byte
	StateRoot [32]byte

	// BeamQCHash commits the Beam quorum certificate that finalized BlockID. It
	// binds the sampled cert to the exact Beam QC, making it non-transferable
	// across different QCs for the same block. The producer sets it to the hash
	// of the Beam QC it is attesting; the verifier recomputes it from the Beam QC
	// it independently checked and rejects a mismatch.
	BeamQCHash [32]byte

	// SignerSetID is the P-Chain-pinned committed validator-set identity the
	// committees were sampled from; PChainHeight is the height it was pinned at.
	SignerSetID  [48]byte
	PChainHeight uint64

	// PolicyID is the finality posture (EvidencePolicyID), also bound into the
	// sortition seed, so the plan and the subject agree on the posture.
	PolicyID uint32

	// CommitteePlanHash is the committeePlanHash from DeriveCommitteePlan — the
	// single commitment to the whole sampling plan. Binding it here freezes the
	// committees into the subject the committees sign.
	CommitteePlanHash []byte
}

// PulsarSampledSubject builds the one canonical sampled-cert subject M. It is a
// pure, deterministic function of its arguments: every verifier that recomputes
// it from the same (re-derived) inputs obtains the byte-identical M. TupleHash256
// length-prefixes every part, so flipping any single field's bytes — including a
// single bit of committeePlanHash or beamQCHash — changes M.
func PulsarSampledSubject(p PulsarSampledSubjectParams) []byte {
	parts := [][]byte{
		[]byte(pulsarSampledSubjectDomain),
		u32be(p.ChainID),
		u64be(p.Height),
		u32be(p.Round),
		p.BlockID[:],
		p.StateRoot[:],
		p.BeamQCHash[:],
		p.SignerSetID[:],
		u64be(p.PChainHeight),
		u32be(p.PolicyID),
		p.CommitteePlanHash,
	}
	return tupleHash256RoundDigest(parts, 32, pulsarSampledSubjectCustom)
}
