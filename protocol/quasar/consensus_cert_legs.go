// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// consensus_cert_legs.go — the four evidence-mode verifiers the ConsensusCert
// envelope dispatches to. Each verifies ONE evidence mode and stays in its
// lane: it never decides whether its leg is required or allowed (the envelope
// does that), only whether the evidence it was handed proves the weighted-
// quorum predicate over the domain-separated message.
//
// Decomplected from the envelope (consensus_cert.go):
//
//	VerifyThresholdSigLeg        — O(1) aggregate threshold sig + accountability
//	VerifyWeightedSigSet         — the already-green WeightedQuorumCert
//	VerifyStarkCompressedSigSet  — audit-gated succinct proof of the SAME predicate
//	VerifyClassicalAggregateLeg  — classical aggregate, LegClassical only
package quasar

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// ----------------------------------------------------------------------------
// VerifyThresholdSigLeg — EvidenceThresholdSig.
// ----------------------------------------------------------------------------

// VerifyThresholdSigLeg verifies a threshold-signature leg: an O(1) aggregate
// threshold signature (Pulsar / Corona) over the message, PLUS the signer
// accountability a bare threshold signature lacks.
//
//	I7  — a Magnetar (SLH-DSA) leg in threshold-sig mode is a hard reject:
//	      SLH-DSA has no aggregatable threshold structure (THBS-SE reconstructs
//	      the FIPS-205 seed — research-only, production-forbidden).
//	I8  — the leg MUST bind signer root / aggregate weight / policy / session.
//	      A threshold sig that verifies but lacks accountability is rejected;
//	      Accountability.SignerRoot must equal cert.SignerRoot (which is bound
//	      into the signed message), and AggregateWeight must meet the policy
//	      threshold weight.
//
// SOUNDNESS: the envelope's domain message binds signer_root (I4). The threshold
// signers signed over THAT message, so a verifying signature attests the quorum
// committed to this signer_root. The accountability echo + the weight floor are
// the envelope's cross-checks that the cert's CLAIMED signer set and weight
// match what was signed — a bare threshold sig expresses neither.
func VerifyThresholdSigLeg(policy ConsensusCertPolicy, validators ConsensusValidatorSet, cert *ConsensusCert, msg []byte, ev LegEvidence) error {
	// I7 — SLH-DSA is never threshold-signed in production.
	if ev.Leg.Kind == LegMagnetarSLHDSA {
		return ErrSLHDSAThresholdSigForbidden
	}

	payload, err := decodeThresholdSigPayload(ev.Payload)
	if err != nil {
		return err
	}

	// I8 (presence) — accountability is mandatory. A threshold sig with no
	// signer-set/weight binding is unacceptable for an accountable cert.
	if payload.Accountability == nil {
		return ErrThresholdSigWithoutAccountability
	}

	// I8 (signer root) — the accountability's signer root MUST equal the cert's
	// signer root, which is bound into the signed message. A mismatch means the
	// cert is claiming a different signer set than the one whose commitment the
	// quorum signed.
	if payload.Accountability.SignerRoot != cert.SignerRoot {
		return ErrSignerRootMismatch
	}

	// I8 (weight floor) — the accountability's aggregate weight MUST meet the
	// policy threshold weight. A threshold signature alone cannot express the
	// weight that signed; the accountability asserts it and the envelope pins
	// the floor.
	if payload.Accountability.AggregateWeight < policy.ThresholdWeight() {
		return fmt.Errorf("%w: have %d need %d",
			ErrInsufficientWeight, payload.Accountability.AggregateWeight, policy.ThresholdWeight())
	}

	// Fetch the trusted group key for this leg kind (verifier-supplied, never
	// cert bytes).
	gk, ok := validators.ThresholdGroupKey(ev.Leg.Kind)
	if !ok {
		return fmt.Errorf("%w: kind %s", ErrThresholdGroupKeyMissing, ev.Leg.Kind)
	}
	// Defence-in-depth: the group key must belong to the same kind as the leg
	// (no cross-kind key swap).
	if gk.Kind != ev.Leg.Kind {
		return fmt.Errorf("%w: key kind %s leg kind %s",
			ErrThresholdGroupKeyKindMismatch, gk.Kind, ev.Leg.Kind)
	}

	// Verify the O(1) aggregate threshold signature against the group key over
	// the recomputed message, routing through origin's per-leg verifiers
	// (cert_policy_verify.go / polaris.go) — the SAME verifiers QuasarCert uses.
	var verified bool
	switch ev.Leg.Kind {
	case LegCoronaLattice:
		if gk.CoronaGroupKey == nil {
			return fmt.Errorf("%w: corona key nil", ErrThresholdGroupKeyMissing)
		}
		verified = verifyCoronaLeg(msg, gk.CoronaGroupKey, payload.Signature)
	case LegPulsarMLDSA:
		if len(gk.PulsarGroupKey) == 0 {
			return fmt.Errorf("%w: pulsar key empty", ErrThresholdGroupKeyMissing)
		}
		verified = verifyPulsarLeg(msg, gk.PulsarGroupKey, payload.Signature)
	default:
		// LegClassical / unknown can never be a threshold-sig leg.
		return fmt.Errorf("%w: kind %s cannot be a threshold-sig leg", ErrDisallowedEvidenceMode, ev.Leg.Kind)
	}
	if !verified {
		return fmt.Errorf("%w: kind %s", ErrThresholdSigInvalid, ev.Leg.Kind)
	}
	return nil
}

// ----------------------------------------------------------------------------
// VerifyWeightedSigSet — EvidenceWeightedSigSet (the already-green foundation).
// ----------------------------------------------------------------------------

// VerifyWeightedSigSet verifies a weighted-sig-set leg: the evidence payload is
// a marshalled WeightedQuorumCert (quorum_cert.go), the already-green
// foundation. The envelope's job here is to BIND the inner cert to the same
// consensus position the envelope claims and to pin the threshold floor — the
// inner cert then proves the full weighted-quorum predicate itself (N
// independent FIPS verifies + weighted-Merkle quorum + threshold floor).
//
// The inner WeightedQuorumCert recomputes ITS OWN domain-separated FIPS message
// from a round-digest envelope; that is correct — a weighted-sig-set cert is a
// complete, self-binding quorum cert. The envelope cross-checks the inner cert's
// position (chain/epoch/height/round/value/validator-set-root) against its own
// header so the inner cert cannot certify a different block or set than the
// envelope claims, and pins MinThreshold == policy.ThresholdWeight().
func VerifyWeightedSigSet(policy ConsensusCertPolicy, validators ConsensusValidatorSet, cert *ConsensusCert, msg []byte, ev LegEvidence) error {
	inner, err := UnmarshalWeightedQuorumCert(ev.Payload)
	if err != nil {
		// A malformed inner cert is a clean evidence-corrupt rejection (I13).
		return fmt.Errorf("%w: weighted-sig-set inner cert: %v", ErrEvidenceWireCorrupt, err)
	}

	// Bind the inner cert to the envelope's consensus position. The inner cert
	// MUST certify the SAME block + position + validator set the envelope claims;
	// otherwise a valid quorum cert for a DIFFERENT block could be smuggled in as
	// evidence for this one.
	if inner.ChainID != cert.ChainID ||
		inner.Epoch != cert.Epoch ||
		inner.Height != cert.Height ||
		inner.Round != cert.Round ||
		inner.ValueHash != cert.BlockHash ||
		inner.ValidatorSetRoot != cert.ValidatorSetRoot {
		return fmt.Errorf("%w: inner weighted-sig-set cert position does not match the envelope", ErrEvidenceWireCorrupt)
	}

	// Pin the threshold floor to the policy's quorum weight (fail-closed; the
	// inner Verify rejects a zero floor). The allowed-scheme / context axes come
	// from the validator set's weighted config.
	cfg := validators.WeightedConfig()
	cfg.MinThreshold = policy.ThresholdWeight()

	// Build the inner cert's round-digest envelope from the validator set's
	// posture axes; the position fields are filled from the inner cert by
	// QuorumMessageForCert inside Verify.
	innerEnv := validators.WeightedEnvelope()
	if err := inner.Verify(innerEnv, cfg); err != nil {
		// Surface the inner predicate's typed error verbatim — it already names
		// the exact clause (Merkle / FIPS / threshold / commitment).
		return err
	}
	return nil
}

// ----------------------------------------------------------------------------
// VerifyStarkCompressedSigSet — EvidenceStarkCompressedSigSet (audit-gated).
// ----------------------------------------------------------------------------

// VerifyStarkCompressedSigSet verifies a stark-compressed-sig-set leg. By
// CONSTRUCTION this mode proves the SAME public statement VerifyWeightedSigSet
// checks: the circuit's public inputs are byte-identical to the WeightedSigSet
// predicate's bound inputs, and the circuit proves VerifyWeightedSigSet(...) ==
// nil over EXACTLY those inputs. It is NOT a different / "close-enough"
// verifier (I9).
//
// Until the Keccak-AIR backend is audit-gated, this returns a clear error and
// NEVER silently accepts — a succinct backend that has not been reviewed must
// fail closed, exactly like an unregistered verifier.
//
// The predicate-identity check below is the part that IS live today: it pins
// that the public inputs the proof commits to equal the WeightedSigSet
// statement for this cert (so when the backend lands, it can only ever attest
// the same relation). The proof BYTES are not yet checked — the function fails
// closed on the gate.
func VerifyStarkCompressedSigSet(policy ConsensusCertPolicy, validators ConsensusValidatorSet, cert *ConsensusCert, msg []byte, ev LegEvidence) error {
	payload, err := decodeStarkCompressedPayload(ev.Payload)
	if err != nil {
		return err
	}

	// Predicate identity (I9): the proof's public inputs MUST equal the
	// WeightedSigSet statement for this cert. This is what makes the compressed
	// mode prove the SAME predicate rather than a different one — the public
	// statement is pinned and inspectable. Computed independently of the payload.
	want := starkPublicStatement(cert, msg)
	if !bytes.Equal(payload.PublicInputs, want) {
		return ErrStarkPublicInputsMismatch
	}

	// Audit gate (I9): the succinct backend (Keccak-AIR) is not yet audit-gated.
	// Fail CLOSED — never silently accept an unreviewed succinct proof.
	return ErrStarkBackendNotAuditGated
}

// starkPublicStatement returns the canonical public statement a
// stark-compressed-sig-set proof MUST commit to: it is, by construction, the
// WeightedSigSet predicate's bound inputs for this cert. The succinct circuit's
// public statement equals this; the WeightedSigSet verifier checks the same
// relation directly. This is the single source of truth both
// VerifyStarkCompressedSigSet (input check) and the equivalence test pin to.
//
// Layout (the inputs VerifyWeightedSigSet binds): the envelope domain message
// (which itself binds validator_set_root, signer_root, threshold via the full
// tuple), the cert's validator_set_root, signer_root, and aggregate_weight.
func starkPublicStatement(cert *ConsensusCert, msg []byte) []byte {
	var u64 [8]byte
	binary.BigEndian.PutUint64(u64[:], cert.AggregateWeight)
	parts := [][]byte{
		[]byte("Lux/ConsensusCert/StarkPublicStatement/v1"),
		msg,
		cert.ValidatorSetRoot[:],
		cert.SignerRoot[:],
		append([]byte(nil), u64[:]...),
	}
	return tupleHash256RoundDigest(parts, 32, "LUX-CONSENSUSCERT-STARK-STATEMENT-V1")
}

// ----------------------------------------------------------------------------
// VerifyClassicalAggregateLeg — EvidenceClassicalAggregate (LegClassical only).
// ----------------------------------------------------------------------------

// VerifyClassicalAggregateLeg verifies a classical-aggregate leg. It is KEPT
// DUMB on purpose: it checks ONLY that the leg kind is LegClassical (a classical
// aggregate can never satisfy a PQ leg — I10), then verifies the classical
// aggregate under the policy-permitted scheme. Whether classical is allowed or
// SUFFICIENT for the chain is the ENVELOPE's call, not this helper's: by the
// time the envelope dispatches here it has already verified every required PQ
// leg with PQ evidence (I11).
func VerifyClassicalAggregateLeg(policy ConsensusCertPolicy, validators ConsensusValidatorSet, cert *ConsensusCert, msg []byte, ev LegEvidence) error {
	// I10 — classical evidence can ONLY satisfy a LegClassical requirement.
	if ev.Leg.Kind != LegClassical {
		return fmt.Errorf("%w: leg kind %s", ErrClassicalCannotSatisfyPQLeg, ev.Leg.Kind)
	}

	payload, err := decodeClassicalAggregatePayload(ev.Payload)
	if err != nil {
		return err
	}

	// Scheme gate — the classical scheme must be policy-permitted.
	if !policy.AllowsClassicalScheme(payload.Scheme) {
		return fmt.Errorf("%w: scheme %s", ErrClassicalAggregateDisallowed, payload.Scheme)
	}

	// Fetch the trusted classical aggregate key (verifier-supplied).
	keyBytes, ok := validators.ClassicalAggregateKey(payload.Scheme)
	if !ok || len(keyBytes) == 0 {
		return fmt.Errorf("%w: no key for scheme %s", ErrClassicalAggregateDisallowed, payload.Scheme)
	}

	// Verify the classical aggregate over the recomputed message.
	switch payload.Scheme {
	case ClassicalSchemeBLS12381:
		pub, perr := blsPublicKeyFromBytes(keyBytes)
		if perr != nil {
			return fmt.Errorf("%w: bls key parse: %v", ErrClassicalAggregateDisallowed, perr)
		}
		if !verifyBLSLeg(msg, pub, payload.Payload) {
			return ErrClassicalAggregateInvalid
		}
		return nil
	default:
		return fmt.Errorf("%w: unhandled scheme %s", ErrClassicalAggregateDisallowed, payload.Scheme)
	}
}
