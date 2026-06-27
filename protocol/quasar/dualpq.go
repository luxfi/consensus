// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// dualpq.go — the canonical parallel-PQ (Pulsar ‖ Corona) AND-mode posture.
//
// Every Quasar finality cert in this posture carries TWO independent
// post-quantum threshold legs, in parallel, both REQUIRED to finalize:
//
//	Pulsar leg  — Module-LWE, FIPS-204 ML-DSA threshold signature.
//	              Verified by an unmodified FIPS-204 verifier
//	              (verifyPulsarLeg → pulsarwire.VerifyBytes). On-chain
//	              precompile: ML-DSA at 0x…2200 (C-Chain) / 0x…2300 (Q-Chain).
//	Corona leg  — Ring-LWE, dealerless 2-round threshold signature
//	              (Boschini et al, ePrint 2024/1113). Verified by
//	              verifyCoronaLeg → coronaThreshold.Verify. On-chain
//	              precompile: Corona at 0x5202… (C-Chain) / 0x5302… (Q-Chain).
//
// WHY AND-MODE, NOT OR. Finality security is the security of the SAFEST
// surviving leg, not the weakest. With OR-mode, an adversary who breaks
// EITHER lattice family forges a cert; with AND-mode it must break BOTH
// the Module-LWE leg AND the Ring-LWE leg in the SAME round. Two distinct
// hardness assumptions (MLWE ⟂ RLWE) and two distinct schemes (a hint-
// recovering FIPS-204 path ⟂ a Pedersen-DKG Ring-LWE path) must fall
// together. This is the cryptographic meaning of "parallel defense in
// depth": orthogonal assumptions, conjunctive acceptance.
//
// WHY THIS IS PERMISSIONLESS-SAFE TODAY. The permissionless guarantee
// rests on the Corona leg, whose group key is installed by a genuine
// DEALERLESS distributed key generation (no party ever holds the master
// secret; corona/keyera Pedersen DKG). Even where the Pulsar leg's genesis
// key is installed under a fenced trusted/TEE ceremony (see
// PULSAR_THRESHOLD_STATUS — a byte-FIPS-204-compatible dealerless ML-DSA
// DKG is research-grade and not yet shipped), an attacker who compromises
// that ceremony and forges a Pulsar leg STILL cannot finalize: AND-mode
// requires the Corona leg too, and Corona's dealerless key has no single
// point of forgery. The two legs are decomplected — neither leg's genesis
// trust assumption can weaken the other's. (Pulsar threshold-signing status:
// luxfi/pulsar BLOCKERS.md PULSAR-V12-PARALLEL-PQ — a byte-FIPS-204-compatible
// no-reconstruct ML-DSA threshold signer is gated on a poly-vector share type
// + the V13 ZK proofs and is not yet shipped.)
//
// DECOMPLECTED. This file owns ONLY the parallel-PQ leg-set definition and
// the pure-function composition of the two leg evidences. "Which legs are
// required" stays in config.CertPolicy.RequiredLegs() (the operator knob:
// CertModeStrict + CertVariantStrict ⇒ {Pulsar, Corona}); "does a leg
// verify" stays in the verify<Leg>Leg helpers; "is a leg required HERE"
// stays in the envelope. This is composition, not extension: a dual-PQ
// cert WRAPS a corona threshold output and a pulsar threshold output; it
// does not subclass either signer.

package quasar

import (
	"errors"

	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
)

var (
	// ErrDualPQMissingPulsar — the Pulsar leg signature is empty. A dual-PQ
	// cert is AND-mode: a missing Pulsar leg cannot be composed.
	ErrDualPQMissingPulsar = errors.New("quasar: dual-PQ cert requires a non-empty Pulsar (ML-DSA) leg")

	// ErrDualPQMissingCorona — the Corona leg signature is nil. A dual-PQ
	// cert is AND-mode: a missing Corona leg cannot be composed.
	ErrDualPQMissingCorona = errors.New("quasar: dual-PQ cert requires a non-nil Corona (Ring-LWE) leg")
)

// DualPQRequiredLegs returns the canonical AND-mode dual-lattice required-leg
// set for the given parameter set: a Pulsar (Module-LWE / FIPS-204 ML-DSA)
// leg AND a Corona (Ring-LWE) leg. BOTH are required — this is the leg set a
// ConsensusCertPolicy.RequiredLegs() returns for the pure-PQ dual-lattice
// posture (config CertModeStrict + CertVariantStrict).
//
// Order is canonical: Pulsar first, then Corona. HashRequiredLegs binds the
// legs in the order returned, so the order is part of the committed
// RequiredLegsRoot — do not reorder.
func DualPQRequiredLegs(paramSet uint8) []LegSpec {
	return []LegSpec{
		{Kind: LegPulsarMLDSA, ParamSetID: paramSet},
		{Kind: LegCoronaLattice, ParamSetID: paramSet},
	}
}

// ComposeDualPQEvidence builds the two threshold-sig LegEvidence entries for a
// dual-PQ ConsensusCert from a real Pulsar (FIPS-204 ML-DSA) threshold
// signature and a real Corona (Ring-LWE) threshold signature, both produced
// over the SAME envelope message (consensusCertMessage), plus the shared
// accountability binding (signer root + aggregate weight).
//
// Pure function: same inputs ⇒ same evidence bytes. No mutable state, no
// randomness. Each leg ships its primitive's own output — pulsarSig is the
// PULS-framed FIPS-204 signature bytes (pulsarwire.Signature.MarshalBinary);
// coronaSig is the typed Corona threshold signature, wire-framed here via
// EncodeCoronaSig. The aggregator that calls this holds neither party's key
// share — it composes already-produced signatures (composition over
// inheritance).
//
// The returned slice is ordered Pulsar-then-Corona to match
// DualPQRequiredLegs; the envelope matches evidence to required legs by Kind,
// so order is not load-bearing for verification, but it is kept canonical.
func ComposeDualPQEvidence(
	paramSet uint8,
	signerRoot [32]byte,
	aggregateWeight uint64,
	pulsarSig []byte,
	coronaSig *coronaThreshold.Signature,
) ([]LegEvidence, error) {
	if len(pulsarSig) == 0 {
		return nil, ErrDualPQMissingPulsar
	}
	if coronaSig == nil {
		return nil, ErrDualPQMissingCorona
	}

	// One accountability binding, shared by both legs: both threshold quorums
	// signed the SAME envelope message, which commits this signer root. The
	// envelope cross-checks Accountability.SignerRoot == cert.SignerRoot and
	// AggregateWeight >= policy.ThresholdWeight() per leg (I8).
	acct := &ThresholdAccountability{SignerRoot: signerRoot, AggregateWeight: aggregateWeight}

	return []LegEvidence{
		{
			Leg:  LegSpec{Kind: LegPulsarMLDSA, ParamSetID: paramSet},
			Mode: EvidenceThresholdSig,
			Payload: encodeThresholdSigPayload(&ThresholdSigPayload{
				Signature:      pulsarSig,
				Accountability: acct,
			}),
		},
		{
			Leg:  LegSpec{Kind: LegCoronaLattice, ParamSetID: paramSet},
			Mode: EvidenceThresholdSig,
			Payload: encodeThresholdSigPayload(&ThresholdSigPayload{
				Signature:      EncodeCoronaSig(coronaSig),
				Accountability: acct,
			}),
		},
	}, nil
}
