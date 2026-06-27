// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// p3q_rollup.go — the P3Q rollup leg: a compact root/proof over the raw
// MLDSACertSet.
//
// WHAT THE MLDSACertSet IS. "A set of independent per-validator ML-DSA certs
// with a weighted-validator-set quorum" is EXACTLY the package's existing
// WeightedQuorumCert (EvidenceWeightedSigSet): N stock FIPS-204 signatures,
// each bound to the committed validator set by a weighted-Merkle inclusion
// proof, meeting the BFT weight floor. P3Q does NOT reinvent per-validator
// verification or validator-set binding — it COMPOSES the audited
// WeightedQuorumCert and adds the one thing that makes it a rollup: a compact
// commitment (RollupRoot) over the raw set, plus the option to replace the raw
// set with a succinct PROOF of that commitment.
//
// ITS ROLE. P3Q is the FALLBACK lane. The raw MLDSACertSet is the
// accountability / challenge object; it is NEVER the normal finality object
// when a compact Pulsar/TALUS threshold sig is live. P3Q lets the chain finalise
// from independent validator certs during migration (before the group key is
// installed), recovery (after a key loss), a bridge (only independent sigs
// available), or an audit (per-validator attribution wanted).
//
// ITS OWN PRIMITIVE. P3Q is its own evidence mode (EvidenceP3QRollup), its own
// verifier (this file), and conceptually its own precompile slot (0x012205). It
// satisfies the SAME LegPulsarMLDSA requirement by a DIFFERENT mechanism than
// the compact threshold sig — that is the policy-table "OR".
//
// THREE PROOF SYSTEMS (selected by the suite's ProofAssumption):
//
//	Direct (AssumptionDirect)              — the raw MLDSACertSet (a marshalled
//	    WeightedQuorumCert) is carried; verification = the rollup-root commitment
//	    binding PLUS the audited, validator-set-bound weighted-sig-set verify.
//	    PQ-safe (FIPS-204), O(N), ALWAYS verifiable. The audit / challenge path.
//
//	STARK (AssumptionPQSuccinct)           — a succinct hash-based proof of the
//	    SAME public statement. PQ-safe; AUDIT-GATED — fails closed until the
//	    proving backend is reviewed, never silently accepted.
//
//	Groth16 (AssumptionClassicalSuccinct)  — a succinct proof whose SOUNDNESS
//	    rests on a CLASSICAL (pairing/DLOG) assumption. THE PQ-SAFETY CAVEAT: the
//	    proof OBJECT is breakable by a CRQC, so it is admissible ONLY on a policy
//	    that explicitly opts in (ProofAssumptionPolicy); even then it is
//	    audit-gated. The RAW set stays PQ-safe and is challengeable via Direct.
//
// "All lanes sign the same M" holds for the compact Beam/Pulsar/Corona legs. The
// P3Q direct path is the independent-cert fallback: each underlying validator
// signed the inner round-digest, and the leg is bound to the SAME consensus
// POSITION (chain/epoch/height/round/value/validator-set-root) as the envelope —
// the same position binding the audited weighted-sig-set leg already enforces.
//
// Decomplected: this file owns ONLY P3Q rollup verification. Validator-set
// binding + per-validator FIPS verification live in the WeightedQuorumCert it
// composes; which legs are required lives in the policy; M lives in
// quasar_finality.go.
package quasar

import (
	"errors"
	"fmt"
)

// P3QRollupPayload is the EvidenceP3QRollup payload. For a Direct suite it
// carries the raw MLDSACertSet (a marshalled WeightedQuorumCert) and the rollup
// root committing to it; for a succinct suite it carries the Proof and the
// rollup root the proof attests (CertSet empty).
type P3QRollupPayload struct {
	// SuiteID names the concrete P3Q scheme (param set + proof system). Resolved
	// through the suite registry so it can never route to a non-P3Q verifier.
	SuiteID string

	// RollupRoot is the canonical commitment over the raw MLDSACertSet bytes
	// (Direct) or the root the succinct proof attests (succinct). It is the
	// bridge between the raw and compressed forms: the SAME root, two ways to
	// satisfy it.
	RollupRoot [32]byte

	// CertSet is the raw MLDSACertSet — a marshalled WeightedQuorumCert (Direct
	// path only). Empty for succinct suites.
	CertSet []byte

	// Proof is the succinct proof bytes (succinct suites only). Empty for Direct.
	Proof []byte
}

// p3qRollup domain tags (wire-stable).
const (
	p3qRollupRootCustomization = "LUX-P3Q-MLDSA-ROLLUP-ROOT-V1"
	p3qStatementCustomization  = "LUX-P3Q-MLDSA-STATEMENT-V1"
	p3qRollupRootProtocolTag   = "Lux/P3Q/MLDSARollup/Root"
	p3qStatementProtocolTag    = "Lux/P3Q/MLDSARollup/Statement"
)

// Typed errors for the P3Q rollup leg.
var (
	// ErrP3QRootMismatch — the rollup root does not equal the canonical
	// commitment over the carried MLDSACertSet bytes.
	ErrP3QRootMismatch = errors.New("quasar: p3q rollup root does not match the canonical commitment over the cert set")

	// ErrP3QRollupEmpty — the rollup carries no cert set on the Direct path.
	ErrP3QRollupEmpty = errors.New("quasar: p3q Direct rollup carries an empty MLDSACertSet")

	// ErrP3QBackendNotAuditGated — the succinct P3Q proving backend (STARK /
	// Groth16) is not yet audit-gated. Fail closed; the raw cert set remains
	// challengeable via the Direct suite.
	ErrP3QBackendNotAuditGated = errors.New("quasar: p3q succinct proof backend is not audit-gated — fail closed (raw cert set is challengeable via the Direct suite)")

	// ErrClassicalProofAssumptionRefused — a classical-assumption succinct P3Q
	// proof was presented under a policy that does not accept classical proof
	// assumptions (the PQ-safety caveat).
	ErrClassicalProofAssumptionRefused = errors.New("quasar: p3q classical-assumption proof refused — policy does not accept classical proof assumptions")
)

// ProofAssumptionPolicy is an OPTIONAL capability a ConsensusCertPolicy may
// implement to opt into classical-assumption succinct proofs (the P3Q Groth16
// suite). A policy that does not implement it is treated as REFUSING classical
// proof assumptions — fail closed.
type ProofAssumptionPolicy interface {
	AcceptsClassicalProofAssumption() bool
}

// P3QRollupRoot is the canonical commitment over a raw MLDSACertSet under a
// suite: TupleHash256(tag ‖ suite_id ‖ cert_set_bytes). Binding the suite id
// stops a root being transplanted across parameter sets / proof systems;
// length-prefixing (TupleHash) stops any field bleeding into a neighbour.
func P3QRollupRoot(suiteID string, certSet []byte) [32]byte {
	parts := [][]byte{
		[]byte(p3qRollupRootProtocolTag),
		[]byte(suiteID),
		certSet,
	}
	var out [32]byte
	copy(out[:], tupleHash256RoundDigest(parts, 32, p3qRollupRootCustomization))
	return out
}

// p3qPublicStatement is the public statement a succinct P3Q proof MUST attest:
// the rollup root, the canonical message M, the threshold weight floor, and the
// cert's signer root. Pinned in the clear so the predicate is inspectable; when
// a proving backend is audit-gated it can only ever attest THIS relation.
func p3qPublicStatement(cert *ConsensusCert, msg []byte, rollupRoot [32]byte, threshold uint64) []byte {
	parts := [][]byte{
		[]byte(p3qStatementProtocolTag),
		rollupRoot[:],
		msg,
		cert.SignerRoot[:],
		u64be(threshold),
	}
	return tupleHash256RoundDigest(parts, 32, p3qStatementCustomization)
}

// VerifyP3QRollupLeg verifies an EvidenceP3QRollup leg. It is the SAME-predicate
// fallback for LegPulsarMLDSA: it establishes that a validator-set-bound
// weighted quorum of independent ML-DSA signatures over the consensus position
// exists, committed to a compact rollup root.
//
// Direct suite: bind the rollup root to the raw MLDSACertSet, then verify that
// set with the audited, validator-set-bound weighted-sig-set verifier (which
// pins the threshold floor and binds the envelope position). Succinct suites:
// pin the public statement, enforce the classical-assumption policy opt-in, and
// fail closed on the unaudited backend.
func VerifyP3QRollupLeg(policy ConsensusCertPolicy, validators ConsensusValidatorSet, cert *ConsensusCert, msg []byte, ev LegEvidence) error {
	// This mode satisfies ONLY the Module-LWE ML-DSA PQ-finality requirement.
	if ev.Leg.Kind != LegPulsarMLDSA {
		return fmt.Errorf("%w: p3q rollup on leg kind %s", ErrDisallowedEvidenceMode, ev.Leg.Kind)
	}

	payload, err := decodeP3QRollupPayload(ev.Payload)
	if err != nil {
		return err
	}

	// Dispatch safety: the suite MUST resolve to a P3Q ML-DSA lane whose param
	// set equals this leg's param set. No suite string can reach this verifier
	// for another lane, and no param-set downgrade is possible.
	suite, err := resolveSuiteForLane(payload.SuiteID, EvidenceP3QMLDSARollup, ev.Leg.ParamSetID)
	if err != nil {
		return err
	}

	switch suite.Assumption {
	case AssumptionDirect:
		return verifyP3QDirect(policy, validators, cert, msg, suite, payload, ev)
	case AssumptionPQSuccinct:
		// Pin the statement (so an audited backend can only attest THIS
		// relation), then fail closed — the backend is not yet audit-gated.
		_ = p3qPublicStatement(cert, msg, payload.RollupRoot, policy.ThresholdWeight())
		return ErrP3QBackendNotAuditGated
	case AssumptionClassicalSuccinct:
		// THE PQ-SAFETY CAVEAT: a classical-assumption proof is admissible only
		// if the policy explicitly accepts classical proof assumptions.
		if pa, ok := policy.(ProofAssumptionPolicy); !ok || !pa.AcceptsClassicalProofAssumption() {
			return ErrClassicalProofAssumptionRefused
		}
		_ = p3qPublicStatement(cert, msg, payload.RollupRoot, policy.ThresholdWeight())
		// Even with policy opt-in, the proving backend is not yet audit-gated.
		return ErrP3QBackendNotAuditGated
	default:
		return fmt.Errorf("%w: p3q suite %q assumption %s", ErrDisallowedEvidenceMode, suite.ID, suite.Assumption)
	}
}

// verifyP3QDirect verifies the Direct (raw MLDSACertSet) P3Q rollup path.
func verifyP3QDirect(policy ConsensusCertPolicy, validators ConsensusValidatorSet, cert *ConsensusCert, msg []byte, suite Suite, payload *P3QRollupPayload, ev LegEvidence) error {
	if len(payload.CertSet) == 0 {
		return ErrP3QRollupEmpty
	}

	// (1) Commitment binding: the rollup root MUST equal the canonical
	// commitment over the carried MLDSACertSet bytes. This is the bridge the
	// succinct path attests; here it ties the root to the exact raw set.
	if P3QRollupRoot(suite.ID, payload.CertSet) != payload.RollupRoot {
		return ErrP3QRootMismatch
	}

	// (2) Verify the raw MLDSACertSet via the audited, VALIDATOR-SET-BOUND
	// weighted-sig-set verifier (composition, not reinvention). This is what
	// makes the leaves real validators with real weights: the inner cert proves
	// weighted-Merkle membership against validators.Root(), N stock FIPS-204
	// verifies, and the BFT threshold floor — none of which a self-asserted leaf
	// set could supply. The envelope position is cross-checked inside.
	return VerifyWeightedSigSet(policy, validators, cert, msg, LegEvidence{
		Leg:     ev.Leg,
		Mode:    EvidenceWeightedSigSet,
		Payload: payload.CertSet,
	})
}
