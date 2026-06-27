// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// consensus_cert.go — the ConsensusCert ENVELOPE.
//
// ConsensusCert is a policy-gated aggregate quorum certificate: policy defines
// the required cryptographic legs; certificate bytes provide per-leg evidence;
// each evidence mode must prove the same weighted-quorum predicate over the
// same domain-separated consensus message.
//
// It sits ONE level above the evidence verifiers. The WeightedSigSet evidence
// mode IS the already-green WeightedQuorumCert (quorum_cert.go); the
// ThresholdSig evidence mode wires origin's threshold-leg verifiers
// (cert_policy_verify.go); the StarkCompressedSigSet evidence mode is the
// audit-gated succinct compression of the SAME weighted-quorum predicate; the
// ClassicalAggregate evidence mode is the classical-only backstop. The envelope
// decides WHICH legs are required (from policy, never from cert bytes), binds
// the full consensus tuple into one domain-separated message, and dispatches
// each required leg to its evidence verifier.
//
// Decomplected (church of Hickey):
//
//   - Leg KIND (LegKind) names WHAT cryptographic requirement a leg satisfies
//     (Pulsar / Corona / Magnetar / Classical). It is a POLICY property.
//   - Evidence MODE (EvidenceMode) names HOW that requirement is proven
//     (ThresholdSig / WeightedSigSet / StarkCompressedSigSet /
//     ClassicalAggregate). It is a PROOF property.
//
// Kind and mode are orthogonal axes and are NEVER braided. No lifecycle word
// (Migration / Legacy / Fallback / Compat / Experimental) appears anywhere: a
// leg is named by its ROLE, never by where it sits in a rollout.
//
// ====================================================================
// THE 13 INVARIANTS (each is enforced in code below; the parenthetical
// names the clause / typed error that enforces it).
// ====================================================================
//
//	I1.  Required legs come from POLICY ONLY, never from certificate bytes.
//	     (VerifyConsensusCert step 2: required := policy.RequiredLegs(); the
//	     cert's RequiredLegsRoot is CHECKED against HashRequiredLegs(required),
//	     never trusted as the source — ErrRequiredLegsRootMismatch.)
//
//	I2.  The certificate may not assert its own required-leg set: the
//	     RequiredLegsRoot is recomputed from policy and any mismatch is a hard
//	     reject. (ErrRequiredLegsRootMismatch.)
//
//	I3.  The validator set is pinned by the verifier, not the cert: the cert's
//	     ValidatorSetRoot is checked against validators.Root().
//	     (ErrValidatorSetRootMismatch.)
//
//	I4.  The signed domain message binds the FULL consensus tuple
//	     (chain_id, epoch, height, round, block_hash, validator_set_root,
//	     policy_id, required_legs_root, signer_root, cert_profile) — NEVER the
//	     block hash alone. (consensusCertMessage; cross-domain replay closed.)
//
//	I5.  Every required leg MUST have evidence in the cert; a missing leg is a
//	     hard reject. (ErrMissingRequiredLeg.)
//
//	I6.  Every leg's (kind, mode, param-set) triple MUST be permitted by policy
//	     before any signature math. (ErrDisallowedEvidenceMode.)
//
//	I7.  SLH-DSA is NEVER threshold-signed in production: a Magnetar leg whose
//	     evidence mode is ThresholdSig is a hard reject.
//	     (ErrSLHDSAThresholdSigForbidden.)
//
//	I8.  A threshold-signature leg that verifies but lacks signer
//	     ACCOUNTABILITY (signer root / aggregate weight / policy / session
//	     binding) MUST be rejected. (ErrThresholdSigWithoutAccountability,
//	     ErrSignerRootMismatch, ErrInsufficientWeight.)
//
//	I9.  The StarkCompressedSigSet evidence mode proves the SAME predicate as
//	     VerifyWeightedSigSet — not a different / "close-enough" verifier. Until
//	     the Keccak-AIR backend is audit-gated it returns a clear error and
//	     NEVER silently accepts. (ErrStarkBackendNotAuditGated;
//	     TestStarkCompressedAndWeightedSigSetSamePredicate.)
//
//	I10. Classical evidence can satisfy ONLY a LegClassical requirement; it can
//	     NEVER stand in for a required PQ leg. (ErrClassicalCannotSatisfyPQLeg.)
//
//	I11. A classical-only cert is rejected under any policy that requires a PQ
//	     leg; classical is accepted only once EVERY required PQ leg has already
//	     been satisfied by PQ evidence. (ErrClassicalOnlyForbidden /
//	     ErrMissingRequiredPQLeg — enforced by the ENVELOPE, not the helper.)
//
//	I12. Every required leg MUST be seen exactly once; the count of satisfied
//	     legs equals the count of required legs. (ErrMissingRequiredLeg via the
//	     final seenLegs == required cardinality check.)
//
//	I13. Malformed envelope / evidence bytes yield a typed error, NEVER a panic
//	     and NEVER unbounded work. (every decode is bounds-checked and
//	     fail-closed; covered by the table + fuzz target.)
package quasar

import (
	"encoding/binary"
	"errors"
	"fmt"

	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
)

// consensusCertVersion pins the envelope struct/wire version. A bump is
// non-malleable: it is bound into the domain message.
const consensusCertVersion uint16 = 1

// consensusCertDomainTag is the domain-separation prefix for the ConsensusCert
// signing message. Wire-stable cryptographic constant: never rename.
const consensusCertDomainTag = "Lux/ConsensusCert/v1"

// consensusCertMessageCustomization is the cSHAKE256 customization tag for the
// envelope's domain message. Distinct from the WeightedSigSet message tag so an
// envelope message can never be replayed as an inner-cert message.
const consensusCertMessageCustomization = "LUX-CONSENSUSCERT-MESSAGE-V1"

// requiredLegsCustomization is the cSHAKE256 customization tag for the
// commitment over the policy-derived required-leg set.
const requiredLegsCustomization = "LUX-CONSENSUSCERT-REQUIRED-LEGS-V1"

// requiredLegsProtocolTag is the in-band redundant protocol tag for the
// required-legs commitment.
const requiredLegsProtocolTag = "Lux/ConsensusCert/RequiredLegs"

// ----------------------------------------------------------------------------
// The two orthogonal axes: leg KIND (what) and evidence MODE (how).
// ----------------------------------------------------------------------------

// EvidenceMode names HOW a leg's requirement is proven. Orthogonal to LegKind
// (what is required). No lifecycle word appears: a mode is named by the proof
// structure it denotes, never by a rollout phase.
type EvidenceMode uint8

const (
	// EvidenceThresholdSig — one O(1) aggregate threshold signature (Pulsar /
	// Corona) over the message, PLUS a signer-accountability binding. A bare
	// threshold signature has no signer-set binding, so accountability is
	// MANDATORY (see VerifyThresholdSigLeg).
	EvidenceThresholdSig EvidenceMode = iota + 1

	// EvidenceWeightedSigSet — N independent stock FIPS 204/205 signatures with
	// a weighted-validator-set Merkle quorum (the WeightedQuorumCert in
	// quorum_cert.go). This is the already-green foundation.
	EvidenceWeightedSigSet

	// EvidenceStarkCompressedSigSet — a succinct STARK proof that the SAME
	// weighted-quorum predicate VerifyWeightedSigSet checks holds. It proves an
	// identical public statement; it is NOT a different verifier. Audit-gated:
	// rejected until the Keccak-AIR backend lands (never silently accepted).
	EvidenceStarkCompressedSigSet

	// EvidenceClassicalAggregate — a classical aggregate (e.g. BLS-12-381). It
	// can satisfy ONLY a LegClassical requirement and never a PQ leg.
	EvidenceClassicalAggregate

	// EvidenceP3QRollup — the P3Q FALLBACK mode: a compact root/proof over a
	// set of INDEPENDENT per-validator ML-DSA certificates (the MLDSACertSet),
	// rather than one aggregate threshold signature. It is its OWN primitive
	// (own verifier in p3q_rollup.go, conceptual precompile slot 0x012205), not
	// bolted onto the Pulsar threshold-sig verifier; it satisfies the SAME
	// LegPulsarMLDSA requirement by a different mechanism. Used for
	// migration / recovery / bridge / audit, NEVER as the primary finality
	// object when a compact Pulsar threshold leg is available.
	EvidenceP3QRollup
)

// String returns the canonical wire name of the evidence mode.
func (m EvidenceMode) String() string {
	switch m {
	case EvidenceThresholdSig:
		return "threshold-sig"
	case EvidenceWeightedSigSet:
		return "weighted-sig-set"
	case EvidenceStarkCompressedSigSet:
		return "stark-compressed-sig-set"
	case EvidenceClassicalAggregate:
		return "classical-aggregate"
	case EvidenceP3QRollup:
		return "p3q-rollup"
	default:
		return fmt.Sprintf("evidence-mode(0x%02x)", uint8(m))
	}
}

// LegKind names WHAT cryptographic requirement a leg satisfies. Orthogonal to
// EvidenceMode (how it is proven). The byte values mirror config.LegName for
// the PQ legs so a reader sees consistent numbering; LegClassical is the
// classical backstop.
type LegKind uint8

const (
	// LegPulsarMLDSA — a Module-LWE (Pulsar / FIPS 204 ML-DSA) requirement.
	LegPulsarMLDSA LegKind = iota + 1

	// LegCoronaLattice — a Ring-LWE (Corona) lattice requirement.
	LegCoronaLattice

	// LegMagnetarSLHDSA — a hash-based (Magnetar / FIPS 205 SLH-DSA) cross-family
	// requirement. NEVER satisfiable by EvidenceThresholdSig (no aggregatable
	// threshold structure; production-forbidden).
	LegMagnetarSLHDSA

	// LegClassical — a classical-aggregate (e.g. BLS-12-381) requirement. The
	// only kind EvidenceClassicalAggregate may satisfy.
	LegClassical
)

// String returns the canonical wire name of the leg kind.
func (k LegKind) String() string {
	switch k {
	case LegPulsarMLDSA:
		return "pulsar-mldsa"
	case LegCoronaLattice:
		return "corona-lattice"
	case LegMagnetarSLHDSA:
		return "magnetar-slhdsa"
	case LegClassical:
		return "classical"
	default:
		return fmt.Sprintf("leg-kind(0x%02x)", uint8(k))
	}
}

// IsPostQuantum reports whether this leg kind is a post-quantum requirement.
// LegClassical is the only non-PQ kind.
func (k LegKind) IsPostQuantum() bool {
	switch k {
	case LegPulsarMLDSA, LegCoronaLattice, LegMagnetarSLHDSA:
		return true
	default:
		return false
	}
}

// ----------------------------------------------------------------------------
// Classical scheme axis (the only schemes a LegClassical evidence may name).
// ----------------------------------------------------------------------------

// ClassicalScheme names a classical aggregate signature scheme carried by an
// EvidenceClassicalAggregate payload. Kept distinct from the PQ scheme axis so
// a classical scheme can never be confused with a FIPS 204/205 parameter set.
type ClassicalScheme uint8

const (
	// ClassicalSchemeNone is the zero value (no classical scheme).
	ClassicalSchemeNone ClassicalScheme = 0x00

	// ClassicalSchemeBLS12381 is a BLS-12-381 aggregate over the message.
	ClassicalSchemeBLS12381 ClassicalScheme = 0x01
)

// String returns the canonical wire name of the classical scheme.
func (s ClassicalScheme) String() string {
	switch s {
	case ClassicalSchemeNone:
		return "none"
	case ClassicalSchemeBLS12381:
		return "bls12-381"
	default:
		return fmt.Sprintf("classical-scheme(0x%02x)", uint8(s))
	}
}

// ----------------------------------------------------------------------------
// Certificate structure.
// ----------------------------------------------------------------------------

// LegSpec names one leg's requirement axes: its kind plus the parameter-set
// byte the evidence is produced under (e.g. 0x42 = ML-DSA-65). The (kind,
// mode, param-set) triple is what policy.Allows gates.
type LegSpec struct {
	// Kind is WHAT requirement this leg satisfies.
	Kind LegKind

	// ParamSetID is the parameter-set byte (config.SigSchemeID / IdentitySchemeID
	// byte for PQ legs; ClassicalScheme byte for LegClassical) the evidence is
	// produced under. Gated by policy.Allows so a leg cannot silently downgrade
	// its parameter set.
	ParamSetID uint8
}

// LegEvidence is one leg's contribution to a ConsensusCert: its requirement
// (LegSpec), the evidence mode (how it is proven), and the opaque per-mode
// payload the matching evidence verifier decodes.
type LegEvidence struct {
	// Leg names the requirement this evidence claims to satisfy.
	Leg LegSpec

	// Mode names how the requirement is proven; selects the evidence verifier.
	Mode EvidenceMode

	// Payload is the mode-specific evidence bytes (a marshalled
	// WeightedQuorumCert, a ThresholdSigPayload, a StarkCompressedPayload, or a
	// ClassicalAggregatePayload). The matching verifier owns its decoding;
	// every decode is bounds-checked and fail-closed.
	Payload []byte
}

// ParamSet returns the parameter-set byte of this evidence's leg. Convenience
// for the policy gate.
func (e LegEvidence) ParamSet() uint8 { return e.Leg.ParamSetID }

// ConsensusCert is the policy-gated aggregate quorum certificate. The header
// pins the full consensus position + policy/validator-set/signer commitments;
// Evidence carries one entry per leg.
//
// CRITICAL: the header's RequiredLegsRoot is NOT the source of which legs are
// required — it is a value the verifier CHECKS against the policy-derived set
// (I1/I2). A cert cannot weaken itself by claiming a smaller required set.
type ConsensusCert struct {
	// Version pins the envelope format.
	Version uint16

	// Profile is the chain security profile byte bound into the domain message
	// (so a cert produced under one posture cannot be re-presented under
	// another).
	Profile uint8

	// Consensus position.
	ChainID uint32
	Epoch   uint64
	Height  uint64
	Round   uint32

	// BlockHash is the block / value being finalised. Bound into the message —
	// but NEVER alone (I4).
	BlockHash [32]byte

	// ValidatorSetRoot is the weighted-validator-set commitment the verifier
	// pins against validators.Root() (I3).
	ValidatorSetRoot [48]byte

	// PolicyID identifies the policy record the verifier loads from the policy
	// store. Bound into the message.
	PolicyID uint32

	// RequiredLegsRoot is the cert's CLAIMED commitment to the required-leg set.
	// CHECKED against HashRequiredLegs(policy.RequiredLegs()) — never trusted as
	// the source of the required set (I1/I2).
	RequiredLegsRoot [32]byte

	// SignerRoot is the commitment to the signer set across the evidence. Bound
	// into the message and cross-checked by the threshold-sig accountability
	// clause (I8).
	SignerRoot [32]byte

	// StateRoot is the post-state commitment this cert finalises (owner-spec
	// finality tuple). Bound into the canonical message M. Zero when the chain
	// commits state only transitively through BlockHash.
	StateRoot [32]byte

	// KeyEraID identifies the KeyEra (compact group-key era) this cert's
	// threshold legs verify under. Bound into M so a signature under one era's
	// group key can never be replayed under another; the boring VerifyThresholdLeg
	// ALSO checks it structurally against the resolved era (defence in depth).
	KeyEraID uint64

	// AggregateWeight is the cert's claimed total signer weight (header-level;
	// each evidence mode independently re-establishes weight against the
	// threshold).
	AggregateWeight uint64

	// Evidence is the per-leg evidence, one entry per leg the cert provides.
	Evidence []LegEvidence
}

// ----------------------------------------------------------------------------
// Policy + validator-set interfaces (the decomplected inputs).
// ----------------------------------------------------------------------------

// ConsensusCertPolicy is the chain's cert posture, supplied to the verifier.
// It is the SINGLE source of which legs are required and which (kind, mode,
// param-set) triples are permitted. The cert never supplies any of this.
type ConsensusCertPolicy interface {
	// RequiredLegs returns the legs the cert MUST satisfy. POLICY-derived,
	// deterministic, and the sole source of the required set (I1).
	RequiredLegs() []LegSpec

	// Allows reports whether a leg may be proven by the given evidence mode
	// under the given parameter-set byte. Gates the (kind, mode, param-set)
	// triple before any signature math (I6).
	Allows(leg LegSpec, mode EvidenceMode, paramSet uint8) bool

	// ThresholdWeight is the minimum aggregate signer weight any leg's evidence
	// must establish — the chain BFT quorum floor. A threshold-sig leg whose
	// accountability asserts less is rejected (I8); a weighted-sig-set leg's own
	// MinThreshold floor is pinned to this value.
	ThresholdWeight() uint64

	// AllowsClassicalScheme reports whether a classical aggregate under the
	// named scheme is admissible (gates the classical leg's scheme byte).
	AllowsClassicalScheme(scheme ClassicalScheme) bool
}

// ConsensusCertPolicyStore resolves the policy for a (chain, epoch, policy-id).
// The verifier loads the policy from here — never from the cert.
type ConsensusCertPolicyStore interface {
	Policy(chainID uint32, epoch uint64, policyID uint32) (ConsensusCertPolicy, error)
}

// ConsensusValidatorSet is the committed epoch validator set the verifier pins
// the cert against. Root() is checked against the cert's ValidatorSetRoot (I3);
// the per-leg key accessors supply the trusted verification keys for the
// threshold-sig / weighted-sig-set / classical legs.
type ConsensusValidatorSet interface {
	// Root returns the 48-byte weighted-validator-set commitment.
	Root() [48]byte

	// Epoch returns the epoch this set was committed under (folded into the
	// weighted-Merkle leaf digests; passed to the WeightedSigSet verifier).
	Epoch() uint64

	// WeightedConfig returns the QuorumVerifierConfig the WeightedSigSet
	// evidence mode verifies under (allowed schemes, FIPS context, and the
	// MinThreshold floor). The floor MUST be the chain BFT quorum.
	WeightedConfig() QuorumVerifierConfig

	// WeightedEnvelope returns the round-digest posture axes (profile, hash
	// suite, schemes, proof backend/format/verifier, network) the inner
	// WeightedQuorumCert verifies under. The position fields are filled from the
	// inner cert by QuorumMessageForCert, so a stale position here is harmless;
	// the posture axes pin the security model into the inner cert's message.
	WeightedEnvelope() QuorumMessageEnvelope

	// ThresholdGroupKey returns the threshold-signature group public key for a
	// leg kind (Pulsar group key bytes / Corona group key), or (nil, false) if
	// the set has no group key for that kind. Used by the threshold-sig leg.
	ThresholdGroupKey(kind LegKind) (ThresholdGroupKey, bool)

	// ClassicalAggregateKey returns the classical aggregate verification key for
	// a scheme (e.g. the BLS aggregate public key), or (nil, false). Used by the
	// classical leg.
	ClassicalAggregateKey(scheme ClassicalScheme) ([]byte, bool)
}

// ThresholdGroupKey is the trusted group public key the threshold-sig leg
// verifies against. It carries the leg kind it belongs to and the raw key
// material the per-kind verifier (Corona group key / Pulsar group-key bytes)
// consumes.
type ThresholdGroupKey struct {
	// Kind is the leg kind this group key verifies. A key whose Kind does not
	// match the evidence's leg kind is a hard reject (defence-in-depth against a
	// cross-kind key swap).
	Kind LegKind

	// CoronaGroupKey is the decoded Corona (Ring-LWE) threshold group key, set
	// iff Kind == LegCoronaLattice.
	CoronaGroupKey *coronaThreshold.GroupKey

	// PulsarGroupKey is the Pulsar (Module-LWE) threshold group public-key
	// bytes, set iff Kind == LegPulsarMLDSA.
	PulsarGroupKey []byte
}

// ----------------------------------------------------------------------------
// Per-mode payloads.
// ----------------------------------------------------------------------------

// ThresholdSigPayload is the EvidenceThresholdSig payload: one O(1) aggregate
// threshold signature PLUS the signer accountability that a bare threshold
// signature lacks. Both are mandatory — a threshold sig with no accountability
// is rejected (I8).
//
// SOUNDNESS of the accountability binding: the envelope's domain message (I4)
// already binds signer_root. The threshold signers signed over THAT message, so
// a verifying threshold signature attests the quorum committed to this
// signer_root. Accountability.SignerRoot is the cert's echo of that value; the
// clause SignerRoot == msg.SignerRoot ties the cert's claim to what was signed,
// and AggregateWeight >= ThresholdWeight() is the weight floor the threshold sig
// alone cannot express.
type ThresholdSigPayload struct {
	// Signature is the O(1) aggregate threshold signature bytes (Corona
	// CORS-framed signature / Pulsar PULS-framed signature) over the message.
	Signature []byte

	// Accountability binds the signer set + weight + session to the leg. MUST be
	// present and well-formed; absence is ErrThresholdSigWithoutAccountability.
	Accountability *ThresholdAccountability
}

// ThresholdAccountability is the signer-set / weight / session binding a
// threshold-signature leg MUST carry so the cert is attributable. Without it a
// threshold sig proves "some quorum signed" but binds nothing about WHO or HOW
// MUCH weight — unacceptable for an accountable finality cert.
type ThresholdAccountability struct {
	// SignerRoot MUST equal the envelope's SignerRoot (which is bound into the
	// signed message). A mismatch is ErrSignerRootMismatch.
	SignerRoot [32]byte

	// AggregateWeight MUST be >= policy.ThresholdWeight(). Below floor is
	// ErrInsufficientWeight.
	AggregateWeight uint64
}

// StarkCompressedPayload is the EvidenceStarkCompressedSigSet payload. It is a
// succinct proof of the SAME weighted-quorum public statement
// VerifyWeightedSigSet checks (I9). The PublicInputs field pins, in the clear,
// the exact statement the circuit proves so the predicate identity is
// inspectable and test-pinnable; the Proof is the (audit-gated) succinct bytes.
type StarkCompressedPayload struct {
	// PublicInputs is the circuit's public statement — byte-identical to the
	// WeightedSigSet predicate's bound inputs (validator_set_root ‖
	// quorum_threshold ‖ aggregate_weight ‖ signer_commitment ‖ message). The
	// circuit proves VerifyWeightedSigSet(...) == nil over EXACTLY these inputs.
	PublicInputs []byte

	// Proof is the succinct STARK proof bytes. Verified only once the Keccak-AIR
	// backend is audit-gated; until then the mode fails closed.
	Proof []byte
}

// ClassicalAggregatePayload is the EvidenceClassicalAggregate payload: a
// classical aggregate signature under a named scheme. Satisfies ONLY a
// LegClassical requirement (I10).
type ClassicalAggregatePayload struct {
	// Scheme names the classical aggregate scheme (gated by
	// policy.AllowsClassicalScheme).
	Scheme ClassicalScheme

	// Payload is the classical aggregate signature bytes over the message.
	Payload []byte
}

// ----------------------------------------------------------------------------
// Typed envelope errors. Each maps 1:1 to one invariant clause so a caller /
// test can name the exact failure. Every one is a CLEAN rejection — never a
// panic, never unbounded work.
// ----------------------------------------------------------------------------

var (
	ErrConsensusCertNil          = errors.New("quasar: nil consensus cert")
	ErrConsensusCertVersion      = errors.New("quasar: consensus cert version mismatch")
	ErrConsensusCertPolicyLoad   = errors.New("quasar: policy store could not resolve policy")
	ErrConsensusCertNoPolicyLegs = errors.New("quasar: policy declares no required legs")

	// I1/I2 — required legs are policy-derived; the cert's claimed root must match.
	ErrRequiredLegsRootMismatch = errors.New("quasar: cert RequiredLegsRoot != hash of policy-derived required legs (required legs are policy-derived, never cert-derived)")

	// I3 — validator set is verifier-pinned.
	ErrValidatorSetRootMismatch = errors.New("quasar: cert ValidatorSetRoot != committed validator set root")

	// I5/I12 — every required leg must have evidence, exactly once.
	ErrMissingRequiredLeg = errors.New("quasar: a policy-required leg has no evidence in the cert")
	ErrDuplicateLegKind   = errors.New("quasar: duplicate evidence for the same leg kind")

	// I6 — (kind, mode, param-set) must be policy-permitted.
	ErrDisallowedEvidenceMode = errors.New("quasar: policy does not allow this (leg, evidence-mode, param-set) triple")
	ErrUnknownEvidenceMode    = errors.New("quasar: unknown evidence mode")

	// I7 — SLH-DSA never threshold-signed.
	ErrSLHDSAThresholdSigForbidden = errors.New("quasar: SLH-DSA (Magnetar) may not be proven by threshold-sig evidence — no aggregatable threshold structure exists; production-forbidden")

	// I8 — threshold sig accountability.
	ErrThresholdSigWithoutAccountability = errors.New("quasar: threshold-sig leg carries no signer accountability — a threshold sig that verifies but binds no signer set/weight is rejected")
	ErrSignerRootMismatch                = errors.New("quasar: threshold-sig accountability SignerRoot != cert SignerRoot")
	ErrInsufficientWeight                = errors.New("quasar: threshold-sig accountability AggregateWeight is below the policy threshold weight")
	ErrThresholdGroupKeyMissing          = errors.New("quasar: no threshold group key for this leg kind")
	ErrThresholdSigInvalid               = errors.New("quasar: threshold aggregate signature failed verification")
	ErrThresholdGroupKeyKindMismatch     = errors.New("quasar: threshold group key kind does not match the leg kind")

	// I9 — STARK proves the SAME predicate; audit-gated until Keccak-AIR lands.
	ErrStarkBackendNotAuditGated = errors.New("quasar: stark-compressed-sig-set backend is not audit-gated (Keccak-AIR unbuilt) — fail closed, never silently accept")
	ErrStarkPublicInputsMismatch = errors.New("quasar: stark-compressed public inputs do not equal the WeightedSigSet predicate statement")

	// I10/I11 — classical leg.
	ErrClassicalCannotSatisfyPQLeg = errors.New("quasar: classical-aggregate evidence cannot satisfy a post-quantum leg requirement")
	ErrClassicalOnlyForbidden      = errors.New("quasar: classical evidence present but policy requires a PQ leg that no PQ evidence satisfies (classical-only is forbidden under a PQ policy)")
	ErrClassicalAggregateDisallowed = errors.New("quasar: classical aggregate scheme is not permitted by policy")
	ErrMissingRequiredPQLeg         = errors.New("quasar: a policy-required PQ leg is unsatisfied")
	ErrClassicalAggregateInvalid    = errors.New("quasar: classical aggregate signature failed verification")

	// I13 — malformed evidence bytes.
	ErrEvidenceWireCorrupt = errors.New("quasar: consensus cert evidence payload is malformed")
)

// ----------------------------------------------------------------------------
// HashRequiredLegs — the policy-derived required-leg commitment (I1/I2).
// ----------------------------------------------------------------------------

// HashRequiredLegs computes the 32-byte commitment over a required-leg set.
// Canonical: legs are bound in their given order (RequiredLegs() returns a
// deterministic order), each as (kind, param_set) so a relabel of either is
// caught. This is the value VerifyConsensusCert recomputes from policy and
// checks against the cert's RequiredLegsRoot — the cert never supplies it.
func HashRequiredLegs(legs []LegSpec) [32]byte {
	parts := make([][]byte, 0, 1+2*len(legs))
	parts = append(parts, []byte(requiredLegsProtocolTag))
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(legs)))
	parts = append(parts, append([]byte(nil), n[:]...))
	for _, l := range legs {
		parts = append(parts, []byte{byte(l.Kind), l.ParamSetID})
	}
	out := tupleHash256RoundDigest(parts, 32, requiredLegsCustomization)
	var h [32]byte
	copy(h[:], out)
	return h
}

// ----------------------------------------------------------------------------
// consensusCertMessage — the FULL-tuple domain message (I4).
// ----------------------------------------------------------------------------

// consensusCertMessage builds the domain-separated message the evidence legs
// anchor to. It binds the FULL consensus tuple — NEVER the block hash alone:
//
//	domain_tag ‖ version ‖ cert_profile ‖ chain_id ‖ epoch ‖ height ‖ round ‖
//	block_hash ‖ validator_set_root ‖ policy_id ‖ required_legs_root ‖
//	signer_root ‖ state_root ‖ key_era_id
//
// under a quorum-cert-distinct customization tag. A cert whose signers signed a
// different tuple fails every leg's signature check; a cert that lies about any
// tuple field fails because the rebuilt message no longer matches.
//
// This is the envelope's accessor for the ONE canonical finality message: it
// extracts the tuple from a *ConsensusCert and delegates to finalityMessage
// (quasar_finality.go) — the SAME builder QuasarFinalityMessage uses, so all
// lanes provably sign byte-identical M.
func consensusCertMessage(cert *ConsensusCert, requiredLegsRoot [32]byte) []byte {
	return finalityMessage(finalityTuple{
		Version:          cert.Version,
		Profile:          cert.Profile,
		ChainID:          cert.ChainID,
		Epoch:            cert.Epoch,
		Height:           cert.Height,
		Round:            cert.Round,
		BlockHash:        cert.BlockHash,
		StateRoot:        cert.StateRoot,
		ValidatorSetRoot: cert.ValidatorSetRoot,
		PolicyID:         cert.PolicyID,
		RequiredLegsRoot: requiredLegsRoot,
		SignerRoot:       cert.SignerRoot,
		KeyEraID:         cert.KeyEraID,
	})
}

// ----------------------------------------------------------------------------
// VerifyConsensusCert — the envelope verifier.
// ----------------------------------------------------------------------------

// VerifyConsensusCert verifies a ConsensusCert against the chain policy and the
// committed validator set. Returns nil iff every invariant holds; otherwise a
// typed error naming the first failure. NEVER panics, NEVER does unbounded work.
//
// The 6-step contract (each enforcing the invariants above):
//
//	1. Load the policy from the store (NEVER from the cert).
//	2. required := policy.RequiredLegs(); reject if cert.RequiredLegsRoot !=
//	   HashRequiredLegs(required) (I1/I2). Required legs are POLICY-derived.
//	3. Reject if cert.ValidatorSetRoot != validators.Root() (I3).
//	4. Build the domain message binding the FULL tuple (I4).
//	5. For each required leg: locate its evidence (I5), gate the (kind, mode,
//	   param-set) triple (I6), and dispatch to the matching evidence verifier.
//	6. Reject unless every required leg was satisfied exactly once (I12), and —
//	   the envelope, not any helper — reject a classical-only cert under a PQ
//	   policy (I11).
func VerifyConsensusCert(store ConsensusCertPolicyStore, validators ConsensusValidatorSet, cert *ConsensusCert) error {
	if cert == nil {
		return ErrConsensusCertNil
	}
	if validators == nil || store == nil {
		return ErrConsensusCertNil
	}
	if cert.Version != consensusCertVersion {
		return fmt.Errorf("%w: got %d want %d", ErrConsensusCertVersion, cert.Version, consensusCertVersion)
	}

	// Step 1 — policy from the store. NEVER from the cert.
	policy, err := store.Policy(cert.ChainID, cert.Epoch, cert.PolicyID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConsensusCertPolicyLoad, err)
	}
	if policy == nil {
		return ErrConsensusCertPolicyLoad
	}

	// Step 2 — required legs are POLICY-derived; the cert's claimed root is
	// CHECKED, never trusted as the source (I1/I2).
	required := policy.RequiredLegs()
	if len(required) == 0 {
		return ErrConsensusCertNoPolicyLegs
	}
	wantLegsRoot := HashRequiredLegs(required)
	if cert.RequiredLegsRoot != wantLegsRoot {
		return ErrRequiredLegsRootMismatch
	}

	// Step 3 — validator set is verifier-pinned (I3).
	if cert.ValidatorSetRoot != validators.Root() {
		return ErrValidatorSetRootMismatch
	}

	// Step 4 — domain message binding the FULL tuple (I4). Built from the
	// policy-derived required-legs root, NOT the cert's claimed root (they are
	// already proven equal in step 2, but binding the policy-derived value makes
	// the message construction independent of cert bytes).
	msg := consensusCertMessage(cert, wantLegsRoot)

	// Evidence is one-to-one with leg kind: reject duplicate evidence entries for
	// the same kind up front so a second (e.g. forged) entry can never shadow or
	// be silently ignored behind the first. This makes evidenceFor's "first
	// match" unambiguous.
	evKinds := make(map[LegKind]bool, len(cert.Evidence))
	for i := range cert.Evidence {
		k := cert.Evidence[i].Leg.Kind
		if evKinds[k] {
			return fmt.Errorf("%w: kind %s", ErrDuplicateLegKind, k)
		}
		evKinds[k] = true
	}

	// Step 5 — per required leg: evidence present, triple allowed, dispatch.
	// Track which leg kinds were satisfied (for the cardinality + classical-only
	// checks). A required-leg set is small (<= 4), so the linear scan is O(1).
	seen := make(map[LegKind]bool, len(required))
	satisfiedPQ := false
	for _, leg := range required {
		ev, ok := cert.evidenceFor(leg.Kind)
		if !ok {
			return fmt.Errorf("%w: kind %s", ErrMissingRequiredLeg, leg.Kind)
		}
		if seen[leg.Kind] {
			return fmt.Errorf("%w: kind %s", ErrDuplicateLegKind, leg.Kind)
		}

		// The evidence's own leg spec must name the SAME requirement the policy
		// requires (kind), and the policy must permit the (kind, mode, param-set)
		// triple BEFORE any signature math (I6).
		if ev.Leg.Kind != leg.Kind {
			return fmt.Errorf("%w: evidence kind %s != required kind %s",
				ErrDisallowedEvidenceMode, ev.Leg.Kind, leg.Kind)
		}
		if !policy.Allows(ev.Leg, ev.Mode, ev.ParamSet()) {
			return fmt.Errorf("%w: kind %s mode %s param 0x%02x",
				ErrDisallowedEvidenceMode, ev.Leg.Kind, ev.Mode, ev.ParamSet())
		}

		// Dispatch to the matching evidence verifier.
		if err := dispatchEvidence(policy, validators, cert, msg, ev); err != nil {
			return err
		}

		seen[leg.Kind] = true
		if leg.Kind.IsPostQuantum() {
			satisfiedPQ = true
		}
	}

	// Step 6 — cardinality (I12): every required leg seen exactly once. The
	// duplicate guard above plus this count makes "seen == required" total.
	if len(seen) != len(required) {
		return fmt.Errorf("%w: satisfied %d of %d required", ErrMissingRequiredLeg, len(seen), len(required))
	}

	// Step 6 (I11) — classical-only is forbidden under a PQ policy. The
	// ENVELOPE enforces this, NOT the classical helper: by here every required
	// leg (PQ included) has been satisfied by its OWN evidence, so if the policy
	// required any PQ leg, that PQ leg's PQ evidence verified. A policy that
	// requires a PQ leg therefore cannot be satisfied by classical evidence
	// alone. We assert it explicitly as defence-in-depth: if any required leg is
	// PQ, at least one PQ leg must have been satisfied.
	if requiresPQ(required) && !satisfiedPQ {
		return ErrMissingRequiredPQLeg
	}

	return nil
}

// requiresPQ reports whether the required-leg set contains any PQ leg.
func requiresPQ(required []LegSpec) bool {
	for _, l := range required {
		if l.Kind.IsPostQuantum() {
			return true
		}
	}
	return false
}

// evidenceFor returns the cert's evidence for a leg kind, or (zero, false).
// EvidenceFor is the exported spelling the directive names; evidenceFor is the
// internal one used on the hot path.
func (c *ConsensusCert) evidenceFor(kind LegKind) (LegEvidence, bool) {
	for i := range c.Evidence {
		if c.Evidence[i].Leg.Kind == kind {
			return c.Evidence[i], true
		}
	}
	return LegEvidence{}, false
}

// EvidenceFor returns the cert's evidence for a leg kind, or (zero, false). The
// directive names this accessor in the per-leg loop.
func (c *ConsensusCert) EvidenceFor(kind LegKind) (LegEvidence, bool) {
	return c.evidenceFor(kind)
}

// dispatchEvidence routes one leg's evidence to its mode verifier (I6 dispatch).
func dispatchEvidence(policy ConsensusCertPolicy, validators ConsensusValidatorSet, cert *ConsensusCert, msg []byte, ev LegEvidence) error {
	switch ev.Mode {
	case EvidenceThresholdSig:
		return VerifyThresholdSigLeg(policy, validators, cert, msg, ev)
	case EvidenceWeightedSigSet:
		return VerifyWeightedSigSet(policy, validators, cert, msg, ev)
	case EvidenceStarkCompressedSigSet:
		return VerifyStarkCompressedSigSet(policy, validators, cert, msg, ev)
	case EvidenceClassicalAggregate:
		return VerifyClassicalAggregateLeg(policy, validators, cert, msg, ev)
	case EvidenceP3QRollup:
		return VerifyP3QRollupLeg(policy, validators, cert, msg, ev)
	default:
		return fmt.Errorf("%w: 0x%02x", ErrUnknownEvidenceMode, uint8(ev.Mode))
	}
}
