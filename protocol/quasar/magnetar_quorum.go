// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// magnetar_quorum.go — Magnetar-Quorum (Track A): the trustless-TODAY,
// hash-based PQ finality lane.
//
// THE PRINCIPLE. Magnetar is threshold SLH-DSA (FIPS-205 / SPHINCS+). True
// threshold SLH-DSA with a trustless DKG is HARD (no aggregatable structure in
// a hash-based signature; every internal SHAKE/SHA-2 evaluation is non-linear
// in the secret seed). The near-term TRUSTLESS lane does NOT threshold it.
// Instead, each validator i holds its OWN ordinary FIPS-205 SLH-DSA keypair
// (sk_i, pk_i) — no DKG, no dealer, no shared seed — and signs the SAME
// consensus subject independently. A P3Q rollup then proves that a
// >= policy-threshold WEIGHTED quorum of those independent signatures verified
// under the STOCK FIPS-205 verifier. There is no shared secret anywhere; the
// lane is trustless the moment the rollup relation is sound.
//
// WHAT THIS COMPOSES. "A weighted quorum of independent per-validator FIPS-205
// certs bound to the committed validator set" is EXACTLY the package's audited,
// scheme-generic WeightedQuorumCert (quorum_cert.go): N stock FIPS-205
// signatures, each bound to the committed validator set by a weighted-Merkle
// inclusion proof, meeting the BFT weight floor. The quorum-cert verifier
// already dispatches SLH-DSA records to magnetar.VerifyCtx (stock FIPS-205) —
// see quorum_scheme.go. Magnetar-Quorum does NOT reinvent per-validator verify
// or validator-set binding; it COMPOSES the WeightedQuorumCert and adds the one
// thing that makes it a rollup: a compact commitment (RollupRoot) over the raw
// set, plus the seam to replace the raw set with a succinct PROOF of that
// commitment.
//
// THE CROSS-FAMILY DIVERSITY INVARIANT. The whole point of the Magnetar lane is
// HASH-BASED diversity alongside the lattice legs (Pulsar Module-LWE, Corona
// Ring-LWE). So this lane verifies ONLY SLH-DSA (FIPS-205) records: a record
// under any other scheme family is a hard reject (ErrMagnetarNotHashBased). A
// lattice signature can never masquerade as the hash-based leg, so a single
// Module-LWE break cannot silently satisfy POLARIS_MAX's hash-based requirement.
//
// ITS OWN PRIMITIVE (orthogonal, not bolted on). Magnetar-Quorum is its OWN
// evidence kind ("magnetar-p3q-slhdsa-rollup"), its OWN leg kind
// (LegMagnetarSLHDSA), its OWN evidence mode (EvidenceMagnetarRollup), and its
// OWN verifier (VerifyMagnetarQuorum) — never bolted onto the Pulsar / P3Q
// ML-DSA verifier. It satisfies the hash-based leg of the POLARIS postures.
//
// NOT THE THRESHOLD PATH. Magnetar the PACKAGE also ships THBS-SE, a t-of-n
// reveal-and-aggregate construction that reconstructs the FIPS-205 seed in a
// transient combiner (a signing-time TCB). That is Magnetar-Threshold —
// research only, NOT a strict trustless lane, and NEVER admitted to a POLARIS
// posture. It has no evidence mode in this envelope: SLH-DSA can never be
// proven by a threshold-sig mode here (I7, ErrSLHDSAThresholdSigForbidden).
//
// TWO PROOF SYSTEMS (selected by the suite's ProofAssumption):
//
//	Direct (AssumptionDirect)    — the raw SLH-DSA WeightedQuorumCert is carried;
//	    verification = the rollup-root commitment binding PLUS the audited,
//	    validator-set-bound, SLH-DSA-only weighted-sig-set verify. PQ-safe
//	    (hash-based, FIPS-205), O(N), ALWAYS verifiable. This is the trustless
//	    transparent-aggregation lane that is live TODAY.
//
//	STARK (AssumptionPQSuccinct) — a succinct hash-based (STARK/FRI, p3q at slot
//	    0x012205) proof of the SAME public statement. PQ-safe by construction;
//	    AUDIT-GATED — fails closed until the proving backend is reviewed, never
//	    silently accepted. This is the OPTIMIZATION SEAM: it makes the lane
//	    compact, it does NOT make it more trustless (the Direct path is already
//	    dealer-free / shared-secret-free). No classical-assumption seam exists
//	    for Magnetar — a hash-based diversity lane must never rest on a pairing.
//
// Decomplected: this file owns ONLY Magnetar-Quorum rollup verification.
// Validator-set binding + per-validator stock FIPS-205 verify live in the
// WeightedQuorumCert it composes; which legs are required lives in the policy;
// the canonical message M lives in quasar_finality.go / consensus_cert.go.
package quasar

import (
	"errors"
	"fmt"
)

// MagnetarQuorumCert is the Magnetar-Quorum (Track A) evidence object: a P3Q
// rollup that compresses a weighted quorum of INDEPENDENT validator FIPS-205
// SLH-DSA signatures over the consensus subject into a compact commitment.
//
// The public statement it attests is: "a subset of the registered validator
// SLH-DSA public keys whose total voting weight is >= the policy threshold each
// produced a valid stock-FIPS-205 signature over the same consensus message,
// and each is a committed member of the validator set." NO dealer, NO DKG, NO
// shared secret — the signers hold independent keys.
//
// For a Direct suite it carries the raw SLH-DSA WeightedQuorumCert (CertSet) and
// the rollup root committing to it; Proof is empty. For a succinct suite it
// carries the Proof and the rollup root the proof attests; CertSet is empty.
type MagnetarQuorumCert struct {
	// SuiteID names the concrete Magnetar-Quorum scheme (SLH-DSA param set +
	// proof system). Resolved through the suite registry (compact_evidence.go)
	// so it can never route to a non-Magnetar verifier or a different param set.
	SuiteID string

	// RollupRoot is the canonical commitment over the raw SLH-DSA CertSet bytes
	// (Direct) or the root the succinct proof attests (succinct). It is the
	// bridge between the raw and compressed forms: the SAME root, two ways to
	// satisfy it.
	RollupRoot [32]byte

	// CertSet is the raw SLH-DSA WeightedQuorumCert — a marshalled
	// WeightedQuorumCert whose every signer record is a FIPS-205 (SLH-DSA)
	// scheme (Direct path only). Empty for succinct suites.
	CertSet []byte

	// Proof is the succinct (STARK/FRI) proof bytes (succinct suites only).
	// Empty for the Direct path.
	Proof []byte
}

// magnetarQuorum domain tags (wire-stable). Distinct from the P3Q ML-DSA tags
// so a Magnetar rollup root can never be transplanted onto the ML-DSA lane.
const (
	magnetarRollupRootCustomization = "LUX-MAGNETAR-SLHDSA-ROLLUP-ROOT-V1"
	magnetarStatementCustomization  = "LUX-MAGNETAR-SLHDSA-STATEMENT-V1"
	magnetarRollupRootProtocolTag   = "Lux/Magnetar/SLHDSARollup/Root"
	magnetarStatementProtocolTag    = "Lux/Magnetar/SLHDSARollup/Statement"
)

// Typed errors for the Magnetar-Quorum lane.
var (
	// ErrMagnetarRootMismatch — the rollup root does not equal the canonical
	// commitment over the carried SLH-DSA CertSet bytes.
	ErrMagnetarRootMismatch = errors.New("quasar: magnetar rollup root does not match the canonical commitment over the cert set")

	// ErrMagnetarRollupEmpty — the rollup carries no cert set on the Direct path.
	ErrMagnetarRollupEmpty = errors.New("quasar: magnetar Direct rollup carries an empty SLH-DSA cert set")

	// ErrMagnetarNotHashBased — a signer record under a non-SLH-DSA scheme was
	// found in the Magnetar cert set. The hash-based diversity lane verifies ONLY
	// FIPS-205 records; a lattice signature can never satisfy it.
	ErrMagnetarNotHashBased = errors.New("quasar: magnetar cert set carries a non-SLH-DSA (non-FIPS-205) signer record — the hash-based lane verifies only SLH-DSA")

	// ErrMagnetarBackendNotAuditGated — the succinct Magnetar proving backend
	// (STARK/FRI) is not yet audit-gated. Fail closed; the raw SLH-DSA cert set
	// remains challengeable via the Direct suite.
	ErrMagnetarBackendNotAuditGated = errors.New("quasar: magnetar succinct proof backend is not audit-gated — fail closed (raw SLH-DSA cert set is challengeable via the Direct suite)")
)

// MagnetarRollupRoot is the canonical commitment over a raw SLH-DSA cert set
// under a suite: TupleHash256(tag || suite_id || cert_set_bytes). Binding the
// suite id stops a root being transplanted across parameter sets / proof
// systems; length-prefixing (TupleHash) stops any field bleeding into a
// neighbour.
func MagnetarRollupRoot(suiteID string, certSet []byte) [32]byte {
	parts := [][]byte{
		[]byte(magnetarRollupRootProtocolTag),
		[]byte(suiteID),
		certSet,
	}
	var out [32]byte
	copy(out[:], tupleHash256RoundDigest(parts, 32, magnetarRollupRootCustomization))
	return out
}

// magnetarPublicStatement is the public statement a succinct Magnetar proof
// MUST attest: the rollup root, the canonical message M, the threshold weight
// floor, and the cert's signer root. Pinned in the clear so the predicate is
// inspectable; when a proving backend is audit-gated it can only ever attest
// THIS relation (the same weighted-quorum-of-independent-FIPS-205 predicate the
// Direct path checks).
func magnetarPublicStatement(cert *ConsensusCert, msg []byte, rollupRoot [32]byte, threshold uint64) []byte {
	parts := [][]byte{
		[]byte(magnetarStatementProtocolTag),
		rollupRoot[:],
		msg,
		cert.SignerRoot[:],
		u64be(threshold),
	}
	return tupleHash256RoundDigest(parts, 32, magnetarStatementCustomization)
}

// BuildMagnetarQuorumCert assembles a Direct Magnetar-Quorum cert from an
// already-built SLH-DSA WeightedQuorumCert. It is permissionless and
// deterministic: NO secrets, NO randomness, NO shared state. It marshals the
// cert, computes the rollup root over the marshalled bytes, and enforces the
// cross-family diversity invariant (every signer record must be SLH-DSA /
// FIPS-205) so a Direct Magnetar cert can never carry a lattice record.
//
// suiteID must be a registered Magnetar Direct suite (e.g.
// "Lux-Magnetar-SLHDSA192s-Direct-v1"). The wqc's signer records must all be a
// FIPS-205 scheme; otherwise ErrMagnetarNotHashBased.
func BuildMagnetarQuorumCert(suiteID string, wqc *WeightedQuorumCert) (*MagnetarQuorumCert, error) {
	if wqc == nil {
		return nil, ErrQCNil
	}
	// The Direct path needs the records present (a compact cert has them in the
	// DA layer; re-attach before building the rollup).
	if len(wqc.Signers) == 0 {
		return nil, ErrQCCompactNoRecords
	}
	// Cross-family diversity: every record MUST be SLH-DSA (FIPS-205). This is
	// the load-bearing invariant that makes this the hash-based lane.
	for i := range wqc.Signers {
		if !isHashBasedScheme(wqc.Signers[i].Scheme) {
			return nil, fmt.Errorf("%w: record %d scheme %s", ErrMagnetarNotHashBased, i, wqc.Signers[i].Scheme)
		}
	}
	certSet, err := wqc.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("%w: magnetar cert set marshal: %v", ErrEvidenceWireCorrupt, err)
	}
	return &MagnetarQuorumCert{
		SuiteID:    suiteID,
		RollupRoot: MagnetarRollupRoot(suiteID, certSet),
		CertSet:    certSet,
	}, nil
}

// Encode serialises the cert to the canonical Magnetar-Quorum wire payload (the
// bytes carried in a LegEvidence.Payload). Deterministic: one canonical
// encoding per value.
func (mc *MagnetarQuorumCert) Encode() []byte {
	return encodeMagnetarQuorumCert(mc)
}

// isHashBasedScheme reports whether a quorum scheme is an SLH-DSA (FIPS-205)
// scheme — the only family the Magnetar (hash-based diversity) lane admits.
func isHashBasedScheme(s QuorumSchemeID) bool {
	return s.FIPS() == "205"
}

// ----------------------------------------------------------------------------
// The Magnetar-Quorum relation (verifier).
// ----------------------------------------------------------------------------

// VerifyMagnetarQuorum verifies a Magnetar-Quorum (Track A) cert against the
// canonical message M. It establishes the trustless hash-based statement:
//
//	a subset of the registered validator SLH-DSA public keys whose total voting
//	weight is >= the policy threshold each produced a valid STOCK-FIPS-205
//	signature over the SAME message M, and each is a committed member of the
//	validator set.
//
// Direct suite: bind the rollup root to the raw SLH-DSA cert set, enforce the
// cross-family diversity invariant (every record is SLH-DSA), then verify that
// set with the audited, validator-set-bound weighted-sig-set verifier (which
// re-checks N stock FIPS-205 verifies, the weighted-Merkle membership against
// validators.Root(), the BFT threshold floor, and the envelope position).
// Succinct suite: pin the public statement (so an audited backend can only ever
// attest THIS relation) and fail closed — the backend is not yet audit-gated.
//
// There is NO dealer, NO DKG, NO shared secret anywhere in this path: the
// signers hold INDEPENDENT FIPS-205 keys and the verifier checks independent
// signatures. msg is the canonical Quasar subject M; mc is the decoded cert.
func VerifyMagnetarQuorum(policy ConsensusCertPolicy, validators ConsensusValidatorSet, cert *ConsensusCert, msg []byte, mc *MagnetarQuorumCert) error {
	// The suite carries its own param set (the standalone entry does not pin an
	// external leg param); the leg entry pins it explicitly. expectParam == 0.
	return verifyMagnetarQuorum(policy, validators, cert, msg, mc, 0)
}

// VerifyMagnetarQuorumLeg verifies an EvidenceMagnetarRollup leg. It is the
// LegEvidence dispatch form (called from dispatchEvidence) of
// VerifyMagnetarQuorum: it pins the leg's parameter set against the suite and
// decodes the payload, then delegates to the shared core.
func VerifyMagnetarQuorumLeg(policy ConsensusCertPolicy, validators ConsensusValidatorSet, cert *ConsensusCert, msg []byte, ev LegEvidence) error {
	// This mode satisfies ONLY the hash-based (SLH-DSA / FIPS-205) PQ-finality
	// requirement; never any lattice leg.
	if ev.Leg.Kind != LegMagnetarSLHDSA {
		return fmt.Errorf("%w: magnetar rollup on leg kind %s", ErrDisallowedEvidenceMode, ev.Leg.Kind)
	}
	mc, err := DecodeMagnetarQuorumCert(ev.Payload)
	if err != nil {
		return err
	}
	return verifyMagnetarQuorum(policy, validators, cert, msg, mc, ev.Leg.ParamSetID)
}

// verifyMagnetarQuorum is the shared core. expectParam == 0 means "do not pin
// the param set here" (the standalone entry; the suite carries its own param);
// a non-zero expectParam (the leg entry) asserts the suite's param equals the
// leg's so a leg cannot claim one SLH-DSA param while the suite names another.
func verifyMagnetarQuorum(policy ConsensusCertPolicy, validators ConsensusValidatorSet, cert *ConsensusCert, msg []byte, mc *MagnetarQuorumCert, expectParam uint8) error {
	// Dispatch safety: the suite MUST resolve to a Magnetar SLH-DSA lane. No
	// suite string can reach this verifier for another lane, and (when the leg
	// pins it) no param-set downgrade is possible.
	suite, err := resolveSuiteForLane(mc.SuiteID, EvidenceMagnetarP3QSLHDSARollup, expectParam)
	if err != nil {
		return err
	}
	// Defence in depth on the registry: a Magnetar suite's param set is, by
	// construction, an SLH-DSA scheme. Assert it so the hash-based invariant
	// holds even if the registry were ever mis-edited.
	if !isHashBasedScheme(QuorumSchemeID(suite.ParamSet)) {
		return fmt.Errorf("%w: suite %q param 0x%02x", ErrMagnetarNotHashBased, suite.ID, suite.ParamSet)
	}

	switch suite.Assumption {
	case AssumptionDirect:
		return verifyMagnetarDirect(policy, validators, cert, msg, suite, mc)
	case AssumptionPQSuccinct:
		// Pin the statement (so an audited backend can only attest THIS relation),
		// then fail closed — the STARK/FRI backend is not yet audit-gated. The raw
		// SLH-DSA cert set remains challengeable via the Direct suite.
		_ = magnetarPublicStatement(cert, msg, mc.RollupRoot, policy.ThresholdWeight())
		return ErrMagnetarBackendNotAuditGated
	default:
		// No classical-assumption suite exists for the hash-based lane; any other
		// assumption is disallowed.
		return fmt.Errorf("%w: magnetar suite %q assumption %s", ErrDisallowedEvidenceMode, suite.ID, suite.Assumption)
	}
}

// verifyMagnetarDirect verifies the Direct (raw SLH-DSA cert set) Magnetar
// rollup path — the trustless transparent-aggregation lane that is live today.
func verifyMagnetarDirect(policy ConsensusCertPolicy, validators ConsensusValidatorSet, cert *ConsensusCert, msg []byte, suite Suite, mc *MagnetarQuorumCert) error {
	if len(mc.CertSet) == 0 {
		return ErrMagnetarRollupEmpty
	}

	// (1) Commitment binding: the rollup root MUST equal the canonical commitment
	// over the carried SLH-DSA cert set bytes. This is the bridge the succinct
	// path attests; here it ties the root to the exact raw set.
	if MagnetarRollupRoot(suite.ID, mc.CertSet) != mc.RollupRoot {
		return ErrMagnetarRootMismatch
	}

	// (2) Cross-family diversity invariant: every signer record in the inner cert
	// MUST be SLH-DSA (FIPS-205). This is the load-bearing check that makes this
	// the HASH-BASED leg — a lattice (ML-DSA / Ringtail) record can never satisfy
	// it, so a single Module-LWE break cannot silently satisfy the Magnetar
	// requirement of POLARIS_MAX. Caught here with a precise typed error before
	// the (audited) inner verify.
	inner, err := UnmarshalWeightedQuorumCert(mc.CertSet)
	if err != nil {
		return fmt.Errorf("%w: magnetar inner cert: %v", ErrEvidenceWireCorrupt, err)
	}
	for i := range inner.Signers {
		if !isHashBasedScheme(inner.Signers[i].Scheme) {
			return fmt.Errorf("%w: record %d scheme %s", ErrMagnetarNotHashBased, i, inner.Signers[i].Scheme)
		}
	}

	// (3) Verify the raw SLH-DSA cert set via the audited, VALIDATOR-SET-BOUND
	// weighted-sig-set verifier (composition, not reinvention). This is what makes
	// the leaves real validators with real weights: the inner cert proves
	// weighted-Merkle membership against validators.Root(), N stock FIPS-205
	// verifies, and the BFT threshold floor — none of which a self-asserted leaf
	// set could supply. The envelope position is cross-checked inside.
	return VerifyWeightedSigSet(policy, validators, cert, msg, LegEvidence{
		Leg:     LegSpec{Kind: LegMagnetarSLHDSA, ParamSetID: suite.ParamSet},
		Mode:    EvidenceWeightedSigSet,
		Payload: mc.CertSet,
	})
}
