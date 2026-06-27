// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// p3q_rollup.go — the P3Q rollup leg: a compact root/proof over a set of
// INDEPENDENT per-validator ML-DSA certificates (the MLDSACertSet).
//
// ITS ROLE. P3Q is the FALLBACK lane. When a compact Pulsar/TALUS threshold
// signature is unavailable (migration before the group key is installed,
// recovery after a key loss, a bridge that only has independent validator
// signatures, or an audit that wants per-validator attribution), the chain can
// still finalise by COMPRESSING the independent validator ML-DSA certs into one
// rollup object. The raw MLDSACertSet is the accountability / challenge data —
// it is NEVER the normal finality object when Pulsar is live.
//
// ITS OWN PRIMITIVE. P3Q is its own evidence mode (EvidenceP3QRollup), its own
// verifier (this file), and conceptually its own on-chain precompile slot
// (0x012205). It is NOT bolted onto the Pulsar threshold-sig verifier; it
// merely satisfies the SAME LegPulsarMLDSA requirement (Module-LWE ML-DSA
// PQ-finality) by a different mechanism. That is the policy-table "OR".
//
// THREE PROOF SYSTEMS (selected by the suite's ProofAssumption):
//
//	Direct (AssumptionDirect)              — the raw MLDSACertSet is carried and
//	    re-verified: every independent ML-DSA-{44,65,87} signature is checked
//	    over M, their weights summed against the threshold, and the rollup root
//	    re-derived from the leaves. PQ-safe (FIPS-204), O(N), ALWAYS verifiable.
//	    This is the audit / challenge path and the one the tests exercise.
//
//	STARK (AssumptionPQSuccinct)           — a succinct hash-based proof of the
//	    same statement. PQ-safe by construction; AUDIT-GATED — fails closed
//	    until the proving backend is reviewed (exactly like the StarkCompressed
//	    sig-set mode), never silently accepted.
//
//	Groth16 (AssumptionClassicalSuccinct)  — a succinct proof whose SOUNDNESS
//	    rests on a CLASSICAL (pairing/DLOG) assumption. THE PQ-SAFETY CAVEAT:
//	    the proof OBJECT is breakable by a CRQC, so it is admissible ONLY on a
//	    policy that explicitly opts in (ProofAssumptionPolicy); even then it is
//	    audit-gated. The RAW MLDSACertSet it attests stays PQ-safe and is
//	    challengeable via the Direct path — a verifier who distrusts the
//	    classical proof can always demand and re-check the underlying certs.
//
// Decomplected: this file owns ONLY P3Q rollup verification. Which legs are
// required and whether classical proof assumptions are policy-acceptable live
// in the policy; the canonical message M lives in quasar_finality.go.
package quasar

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/luxfi/crypto/mldsa"
)

// MLDSAValidatorCert is ONE independent validator's ML-DSA certificate: the
// per-validator signature P3Q compresses. A set of these (the MLDSACertSet) is
// what the rollup commits to.
type MLDSAValidatorCert struct {
	// ValidatorID is the signer's id (matches the committed validator set).
	ValidatorID [32]byte

	// Weight is the signer's stake weight, summed against the threshold.
	Weight uint64

	// PubKey is the validator's ML-DSA public key (parsed under the suite's
	// parameter set).
	PubKey []byte

	// Sig is the validator's independent ML-DSA signature over M.
	Sig []byte
}

// P3QRollupPayload is the EvidenceP3QRollup payload. For a Direct suite it
// carries the raw CertSet (the challengeable MLDSACertSet) and the rollup root
// derived from it; for a succinct suite it carries the Proof and the rollup
// root the proof attests (CertSet empty).
type P3QRollupPayload struct {
	// SuiteID names the concrete P3Q scheme (param set + proof system). Resolved
	// through the suite registry so it can never route to a non-P3Q verifier.
	SuiteID string

	// RollupRoot is the canonical commitment over the MLDSACertSet (Direct) or
	// the root the succinct proof attests. Bound to the leaves in the Direct
	// path; pinned into the public statement in the succinct path.
	RollupRoot [32]byte

	// CertSet is the raw MLDSACertSet (Direct path only). Empty for succinct
	// suites.
	CertSet []MLDSAValidatorCert

	// Proof is the succinct proof bytes (succinct suites only). Empty for Direct.
	Proof []byte
}

// p3qRollup domain tags (wire-stable).
const (
	p3qRollupRootCustomization = "LUX-P3Q-MLDSA-ROLLUP-ROOT-V1"
	p3qSignerRootCustomization = "LUX-P3Q-MLDSA-SIGNER-ROOT-V1"
	p3qStatementCustomization  = "LUX-P3Q-MLDSA-STATEMENT-V1"
	p3qRollupRootProtocolTag   = "Lux/P3Q/MLDSARollup/Root"
	p3qSignerRootProtocolTag   = "Lux/P3Q/MLDSARollup/SignerRoot"
	p3qStatementProtocolTag    = "Lux/P3Q/MLDSARollup/Statement"
)

// Typed errors for the P3Q rollup leg.
var (
	// ErrP3QRootMismatch — the rollup root does not equal the canonical
	// commitment over the carried cert set.
	ErrP3QRootMismatch = errors.New("quasar: p3q rollup root does not match the canonical commitment over the cert set")

	// ErrP3QRollupEmpty — the rollup carries no cert set on the Direct path.
	ErrP3QRollupEmpty = errors.New("quasar: p3q Direct rollup carries an empty cert set")

	// ErrP3QRollupInvalid — a committed validator cert failed ML-DSA verification
	// (a rollup is a set of VALID certs; one invalid member is a malformed rollup).
	ErrP3QRollupInvalid = errors.New("quasar: p3q rollup contains a validator cert that failed ML-DSA verification")

	// ErrP3QBackendNotAuditGated — the succinct P3Q proving backend (STARK /
	// Groth16) is not yet audit-gated. Fail closed; the raw cert set remains
	// challengeable via the Direct suite.
	ErrP3QBackendNotAuditGated = errors.New("quasar: p3q succinct proof backend is not audit-gated — fail closed (raw cert set is challengeable via the Direct suite)")

	// ErrClassicalProofAssumptionRefused — a classical-assumption succinct P3Q
	// proof was presented under a policy that does not accept classical proof
	// assumptions (the PQ-safety caveat).
	ErrClassicalProofAssumptionRefused = errors.New("quasar: p3q classical-assumption proof refused — policy does not accept classical proof assumptions")

	// ErrP3QMLDSAMode — the suite parameter set does not name a valid ML-DSA mode.
	ErrP3QMLDSAMode = errors.New("quasar: p3q suite parameter set is not a valid ML-DSA mode")
)

// ProofAssumptionPolicy is an OPTIONAL capability a ConsensusCertPolicy may
// implement to opt into classical-assumption succinct proofs (the P3Q Groth16
// suite). A policy that does not implement it is treated as REFUSING classical
// proof assumptions — fail closed.
type ProofAssumptionPolicy interface {
	AcceptsClassicalProofAssumption() bool
}

// mldsaModeForParam maps a quorum scheme parameter byte to an ML-DSA mode.
func mldsaModeForParam(param uint8) (mldsa.Mode, bool) {
	switch QuorumSchemeID(param) {
	case QuorumSchemeMLDSA44:
		return mldsa.MLDSA44, true
	case QuorumSchemeMLDSA65:
		return mldsa.MLDSA65, true
	case QuorumSchemeMLDSA87:
		return mldsa.MLDSA87, true
	default:
		return 0, false
	}
}

// P3QRollupRoot is the canonical commitment over an MLDSACertSet under a suite.
// Leaves are bound in a CANONICAL order (ascending by ValidatorID) so the root
// is independent of cert ordering; each leaf binds (validator_id, weight,
// pubkey, sig) length-prefixed (TupleHash256), so flipping any field changes
// the root. The suite id is bound so a root cannot be transplanted across
// parameter sets.
func P3QRollupRoot(suiteID string, certs []MLDSAValidatorCert) [32]byte {
	ordered := canonicalCertOrder(certs)
	parts := make([][]byte, 0, 2+4*len(ordered))
	parts = append(parts, []byte(p3qRollupRootProtocolTag))
	parts = append(parts, []byte(suiteID))
	parts = append(parts, u32be(uint32(len(ordered))))
	for _, c := range ordered {
		parts = append(parts, c.ValidatorID[:])
		parts = append(parts, u64be(c.Weight))
		parts = append(parts, c.PubKey)
		parts = append(parts, c.Sig)
	}
	var out [32]byte
	copy(out[:], tupleHash256RoundDigest(parts, 32, p3qRollupRootCustomization))
	return out
}

// p3qSignerRoot is the canonical commitment over the SIGNER SET of a rollup
// (validator ids only, ascending). This is the rollup's accountability anchor:
// the verifier checks it equals the cert's SignerRoot (which is bound into M),
// proving the EXACT set of signers — stronger than the opaque echo a bare
// threshold signature can offer.
func p3qSignerRoot(certs []MLDSAValidatorCert) [32]byte {
	ordered := canonicalCertOrder(certs)
	parts := make([][]byte, 0, 2+len(ordered))
	parts = append(parts, []byte(p3qSignerRootProtocolTag))
	parts = append(parts, u32be(uint32(len(ordered))))
	for _, c := range ordered {
		parts = append(parts, c.ValidatorID[:])
	}
	var out [32]byte
	copy(out[:], tupleHash256RoundDigest(parts, 32, p3qSignerRootCustomization))
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

// canonicalCertOrder returns a copy of certs sorted ascending by ValidatorID.
// Stable canonical order makes the root independent of input ordering.
func canonicalCertOrder(certs []MLDSAValidatorCert) []MLDSAValidatorCert {
	out := make([]MLDSAValidatorCert, len(certs))
	copy(out, certs)
	// Insertion sort by 32-byte id — sets are small (audit/recovery scale) and
	// this avoids pulling sort + a closure onto the verify path.
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && bytes.Compare(out[j-1].ValidatorID[:], out[j].ValidatorID[:]) > 0 {
			out[j-1], out[j] = out[j], out[j-1]
			j--
		}
	}
	return out
}

// VerifyP3QRollupLeg verifies an EvidenceP3QRollup leg. It is the SAME-predicate
// fallback for LegPulsarMLDSA: it establishes that a threshold weight of valid
// independent ML-DSA signatures over M exist, bound to a compact rollup root and
// to the cert's signer set.
//
// Direct suite: re-derive the root from the cert set, verify every independent
// ML-DSA signature over M, sum the weights against policy.ThresholdWeight(), and
// bind the signer root to the cert. Succinct suites: pin the public statement,
// enforce the classical-assumption policy opt-in, and fail closed on the
// unaudited backend.
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
		return verifyP3QDirect(policy, cert, msg, suite, payload)
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

// verifyP3QDirect verifies the Direct (raw cert set) P3Q rollup path.
func verifyP3QDirect(policy ConsensusCertPolicy, cert *ConsensusCert, msg []byte, suite Suite, payload *P3QRollupPayload) error {
	if len(payload.CertSet) == 0 {
		return ErrP3QRollupEmpty
	}

	// (1) Root binding (cheap hash, done before the O(N) signature verifies):
	// the rollup root MUST equal the canonical commitment over the cert set.
	if got := P3QRollupRoot(suite.ID, payload.CertSet); got != payload.RollupRoot {
		return ErrP3QRootMismatch
	}

	// (2) Accountability binding: the signer set committed by the rollup MUST
	// equal the cert's SignerRoot (which is bound into M). This proves the exact
	// signer set — no opaque echo.
	if p3qSignerRoot(payload.CertSet) != cert.SignerRoot {
		return ErrSignerRootMismatch
	}

	mode, ok := mldsaModeForParam(suite.ParamSet)
	if !ok {
		return fmt.Errorf("%w: 0x%02x", ErrP3QMLDSAMode, suite.ParamSet)
	}

	// (3) Every committed cert MUST verify under its own ML-DSA key over M, and
	// duplicate signer ids are rejected (a signer cannot be double-counted
	// toward the weight floor).
	var total uint64
	seen := make(map[[32]byte]bool, len(payload.CertSet))
	for i := range payload.CertSet {
		c := payload.CertSet[i]
		if seen[c.ValidatorID] {
			return fmt.Errorf("%w: duplicate signer in rollup", ErrP3QRollupInvalid)
		}
		seen[c.ValidatorID] = true

		pub, perr := mldsa.PublicKeyFromBytes(c.PubKey, mode)
		if perr != nil {
			return fmt.Errorf("%w: pubkey parse: %v", ErrP3QRollupInvalid, perr)
		}
		if !pub.Verify(msg, c.Sig, nil) {
			return ErrP3QRollupInvalid
		}
		total += c.Weight
	}

	// (4) Weight floor: the verified signers MUST meet the policy quorum.
	if total < policy.ThresholdWeight() {
		return fmt.Errorf("%w: have %d need %d", ErrInsufficientWeight, total, policy.ThresholdWeight())
	}
	return nil
}
