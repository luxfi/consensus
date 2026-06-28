// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// compact_evidence.go — the chain-facing COMPACT-EVIDENCE vocabulary and the
// suite-ID → verifier dispatch registry.
//
// OWNER ARCHITECTURE. For a 1000+ validator committee the chain stores ONE
// compact typed evidence per lane, NEVER 1000 raw ML-DSA signatures. There are
// four typed evidence kinds:
//
//	EvidenceBeamBLS              — fast classical BLS aggregate (one aggregate)
//	EvidencePulsarThresholdMLDSA — one FIPS-204 ML-DSA threshold sig (TALUS)
//	EvidenceCoronaRingtail       — one dealerless Ringtail threshold sig
//	EvidenceP3QMLDSARollup       — FALLBACK: a compact root/proof over a set of
//	                               INDEPENDENT validator ML-DSA certs
//
// Beam, Pulsar and Corona are O(1) in committee size — one signature each. P3Q
// is the fallback that COMPRESSES a set of independent per-validator certs
// (migration / recovery / bridge / audit); the raw MLDSACertSet it compresses
// is the accountability / challenge object, NEVER the normal finality object.
//
// DECOMPLECTED ONTO THE ORTHOGONAL AXES. These four names are the value-level
// vocabulary; underneath, the envelope's two orthogonal axes do the work:
//
//	EvidenceKind                  LegKind (what)     EvidenceMode (how)
//	----------------------------  -----------------  --------------------------
//	EvidenceBeamBLS               LegClassical       EvidenceClassicalAggregate
//	EvidencePulsarThresholdMLDSA  LegPulsarMLDSA     EvidenceThresholdSig
//	EvidenceCoronaRingtail        LegCoronaLattice   EvidenceThresholdSig
//	EvidenceP3QMLDSARollup        LegPulsarMLDSA     EvidenceP3QRollup
//
// Pulsar and P3Q satisfy the SAME leg KIND (the Module-LWE ML-DSA PQ-finality
// requirement) by DIFFERENT mechanisms — this is exactly the policy-table "OR"
// (Beam ∧ (Pulsar OR P3Q)): one required leg kind, two permitted modes.
//
// SUITE-ID DISPATCH SAFETY. A suite ID is a wire string that names a concrete
// parameterised scheme (e.g. "Lux-Pulsar-TALUS-MLDSA65"). The suite registry
// below is the SINGLE authority mapping a suite ID to its (EvidenceKind,
// LegKind, EvidenceMode, param-set). Verifiers resolve a suite ONLY through
// this registry, so NO suite string can dispatch to the wrong verifier: a
// "corona-*" suite resolves to LegCoronaLattice + the Corona verifier and can
// never reach the Pulsar verifier; an unknown suite is a hard reject. The
// param set (ML-DSA-44 / 65 / 87) is carried IN the suite, so a verifier can
// pin the exact parameterisation the evidence claims.
package quasar

import (
	"errors"
	"fmt"
)

// EvidenceKind is the value-level name of one compact evidence lane. It is a
// string so it is self-describing on the wire and in logs; the registry maps
// it (and each concrete suite ID) to the orthogonal (LegKind, EvidenceMode)
// axes the envelope dispatches on.
type EvidenceKind string

const (
	// EvidenceBeamBLS — the fast classical BLS aggregate lane (Beam).
	EvidenceBeamBLS EvidenceKind = "beam-bls"

	// EvidencePulsarThresholdMLDSA — ONE standard FIPS-204 ML-DSA threshold
	// signature under the committee's compact group key (TALUS / Pulsar).
	EvidencePulsarThresholdMLDSA EvidenceKind = "pulsar-threshold-mldsa"

	// EvidenceCoronaRingtail — ONE dealerless Ringtail (Ring-LWE) threshold
	// signature (Corona). Not FIPS — a distinct lattice family from Pulsar.
	EvidenceCoronaRingtail EvidenceKind = "corona-ringtail"

	// EvidenceP3QMLDSARollup — the FALLBACK lane: a compact root/proof over a
	// set of independent validator ML-DSA certs (the MLDSACertSet). Used for
	// migration / recovery / bridge / audit, NOT primary when Pulsar is live.
	EvidenceP3QMLDSARollup EvidenceKind = "p3q-mldsa-rollup"

	// EvidenceMagnetarP3QSLHDSARollup — the Magnetar-Quorum (Track A) lane: a
	// compact root/proof over a set of INDEPENDENT validator SLH-DSA (FIPS-205)
	// certs. The trustless-TODAY hash-based diversity lane — NO dealer / DKG /
	// shared secret. Each validator signs independently with its own FIPS-205
	// key; the rollup proves a >= policy-threshold weighted quorum verified under
	// the stock FIPS-205 verifier. Distinct lattice-free cross-family backstop to
	// the Module-LWE (Pulsar) and Ring-LWE (Corona) legs.
	EvidenceMagnetarP3QSLHDSARollup EvidenceKind = "magnetar-p3q-slhdsa-rollup"
)

// evidenceLane is the orthogonal (LegKind, EvidenceMode) pair a named evidence
// kind decomposes to.
type evidenceLane struct {
	leg  LegKind
	mode EvidenceMode
}

// laneByKind maps each value-level evidence kind to its orthogonal axes. This
// is the single mapping between the chain-facing vocabulary and the envelope's
// dispatch axes — there is exactly one way to express each lane.
var laneByKind = map[EvidenceKind]evidenceLane{
	EvidenceBeamBLS:              {leg: LegClassical, mode: EvidenceClassicalAggregate},
	EvidencePulsarThresholdMLDSA: {leg: LegPulsarMLDSA, mode: EvidenceThresholdSig},
	EvidenceCoronaRingtail:       {leg: LegCoronaLattice, mode: EvidenceThresholdSig},
	EvidenceP3QMLDSARollup:       {leg: LegPulsarMLDSA, mode: EvidenceP3QRollup},
	EvidenceMagnetarP3QSLHDSARollup: {leg: LegMagnetarSLHDSA, mode: EvidenceMagnetarRollup},
}

// ----------------------------------------------------------------------------
// Proof assumption axis (for the P3Q rollup lane).
// ----------------------------------------------------------------------------

// ProofAssumption names the cryptographic hardness a compact proof object
// rests on. It is load-bearing ONLY for the P3Q rollup lane, whose succinct
// proof may be hash-based (PQ-safe) or pairing/DLOG-based (classical). Beam /
// Pulsar / Corona carry their own native signatures and use AssumptionNative.
type ProofAssumption uint8

const (
	// AssumptionNative — the evidence IS a native signature (BLS / ML-DSA /
	// Ringtail); no separate proof object, no extra assumption.
	AssumptionNative ProofAssumption = iota

	// AssumptionDirect — the P3Q rollup carries the raw MLDSACertSet and is
	// verified by re-checking the independent ML-DSA signatures directly. This
	// is PQ-safe (FIPS-204) and O(N); it is the always-available audit /
	// challenge path.
	AssumptionDirect

	// AssumptionPQSuccinct — the P3Q rollup is a succinct hash-based proof
	// (STARK/FRI) of the MLDSACertSet. PQ-safe by construction; audit-gated.
	AssumptionPQSuccinct

	// AssumptionClassicalSuccinct — the P3Q rollup is a succinct proof whose
	// soundness rests on a CLASSICAL assumption (a pairing/DLOG SNARK). The
	// proof object is only policy-acceptable on a policy that opts in
	// (ProofAssumptionPolicy.AcceptsClassicalProofAssumption); the underlying
	// raw MLDSACertSet remains PQ-safe and challengeable via AssumptionDirect.
	AssumptionClassicalSuccinct
)

// String returns the canonical name of the proof assumption.
func (a ProofAssumption) String() string {
	switch a {
	case AssumptionNative:
		return "native"
	case AssumptionDirect:
		return "direct"
	case AssumptionPQSuccinct:
		return "pq-succinct"
	case AssumptionClassicalSuccinct:
		return "classical-succinct"
	default:
		return fmt.Sprintf("proof-assumption(%d)", uint8(a))
	}
}

// ----------------------------------------------------------------------------
// Suite registry — the single suite-ID → verifier dispatch authority.
// ----------------------------------------------------------------------------

// Suite is the immutable description of one concrete, parameterised evidence
// scheme named by a wire-stable suite ID. The registry maps a suite ID to
// EXACTLY one Suite; verifiers resolve suites ONLY through the registry, so a
// suite string can never reach a verifier outside its lane.
type Suite struct {
	// ID is the wire-stable suite string (e.g. "Lux-Pulsar-TALUS-MLDSA65").
	ID string

	// Kind is the value-level evidence lane this suite belongs to.
	Kind EvidenceKind

	// Leg + Mode are the orthogonal axes the envelope dispatches on. They are
	// derived from Kind via laneByKind and pinned here so a verifier never has
	// to re-derive them.
	Leg  LegKind
	Mode EvidenceMode

	// ParamSet is the parameter-set byte the evidence is produced under
	// (QuorumSchemeMLDSA44/65/87 for ML-DSA lanes; ClassicalScheme byte for
	// Beam). Carried IN the suite so a verifier pins the exact parameterisation.
	ParamSet uint8

	// Assumption names the proof assumption (native for Beam/Pulsar/Corona;
	// direct / pq-succinct / classical-succinct for P3Q).
	Assumption ProofAssumption
}

var (
	// ErrUnknownSuite — the suite ID is not in the registry. Fail closed: an
	// unregistered suite can never be dispatched.
	ErrUnknownSuite = errors.New("quasar: unknown evidence suite id")

	// ErrSuiteLaneMismatch — the resolved suite belongs to a different
	// evidence lane than the verifier was invoked for (cross-lane dispatch
	// attempt; defence in depth on top of the leg-kind dispatch).
	ErrSuiteLaneMismatch = errors.New("quasar: suite id resolves to a different evidence lane than expected")

	// ErrSuiteParamMismatch — the resolved suite's parameter set does not match
	// the parameter set the verifier expects (e.g. an ML-DSA-44 suite where the
	// era is ML-DSA-65).
	ErrSuiteParamMismatch = errors.New("quasar: suite parameter set does not match the expected parameter set")

	// ErrSuiteDuplicate — registry construction error: two suites share an ID.
	ErrSuiteDuplicate = errors.New("quasar: duplicate evidence suite id")
)

// productionSuites is the canonical registry of every dispatchable suite. Each
// entry's Leg/Mode is asserted (in init) to equal laneByKind[Kind] so the
// registry can never drift from the lane mapping.
var productionSuites = []Suite{
	// Beam — classical BLS aggregate.
	{ID: "Lux-Beam-BLS12381-v1", Kind: EvidenceBeamBLS, ParamSet: uint8(ClassicalSchemeBLS12381), Assumption: AssumptionNative},

	// Pulsar — TALUS FIPS-204 ML-DSA threshold signature, one per param set.
	{ID: "Lux-Pulsar-TALUS-MLDSA44", Kind: EvidencePulsarThresholdMLDSA, ParamSet: uint8(QuorumSchemeMLDSA44), Assumption: AssumptionNative},
	{ID: "Lux-Pulsar-TALUS-MLDSA65", Kind: EvidencePulsarThresholdMLDSA, ParamSet: uint8(QuorumSchemeMLDSA65), Assumption: AssumptionNative},
	{ID: "Lux-Pulsar-TALUS-MLDSA87", Kind: EvidencePulsarThresholdMLDSA, ParamSet: uint8(QuorumSchemeMLDSA87), Assumption: AssumptionNative},

	// Corona — dealerless Ringtail (Ring-LWE) threshold signature. Tagged with
	// the same ML-DSA security-level byte family the dual-PQ posture uses for
	// the parallel lattice leg; the lane (LegCoronaLattice) is what routes it.
	{ID: "Lux-Corona-Ringtail-L1-v1", Kind: EvidenceCoronaRingtail, ParamSet: uint8(QuorumSchemeMLDSA44), Assumption: AssumptionNative},
	{ID: "Lux-Corona-Ringtail-L3-v1", Kind: EvidenceCoronaRingtail, ParamSet: uint8(QuorumSchemeMLDSA65), Assumption: AssumptionNative},
	{ID: "Lux-Corona-Ringtail-L5-v1", Kind: EvidenceCoronaRingtail, ParamSet: uint8(QuorumSchemeMLDSA87), Assumption: AssumptionNative},

	// P3Q — fallback rollup over independent validator ML-DSA certs. Three
	// proof systems per ML-DSA param set: Direct (raw set, PQ-safe, O(N),
	// always-verifiable), STARK (succinct, PQ-safe, audit-gated), Groth16
	// (succinct, classical assumption, audit-gated + policy opt-in).
	{ID: "Lux-P3Q-MLDSA44-Direct-v1", Kind: EvidenceP3QMLDSARollup, ParamSet: uint8(QuorumSchemeMLDSA44), Assumption: AssumptionDirect},
	{ID: "Lux-P3Q-MLDSA65-Direct-v1", Kind: EvidenceP3QMLDSARollup, ParamSet: uint8(QuorumSchemeMLDSA65), Assumption: AssumptionDirect},
	{ID: "Lux-P3Q-MLDSA87-Direct-v1", Kind: EvidenceP3QMLDSARollup, ParamSet: uint8(QuorumSchemeMLDSA87), Assumption: AssumptionDirect},
	{ID: "Lux-P3Q-MLDSA65-STARK-v1", Kind: EvidenceP3QMLDSARollup, ParamSet: uint8(QuorumSchemeMLDSA65), Assumption: AssumptionPQSuccinct},
	{ID: "Lux-P3Q-MLDSA65-Groth16-v1", Kind: EvidenceP3QMLDSARollup, ParamSet: uint8(QuorumSchemeMLDSA65), Assumption: AssumptionClassicalSuccinct},

	// Magnetar-Quorum (Track A) — the trustless-TODAY hash-based diversity lane:
	// a rollup over INDEPENDENT validator SLH-DSA (FIPS-205) certs. Direct (raw
	// SLH-DSA cert set, PQ-safe, O(N), always-verifiable) per param set, plus a
	// STARK (succinct, PQ-safe, audit-gated) optimization seam. There is NO
	// classical-assumption suite: a hash-based diversity lane must never rest on
	// a pairing/DLOG proof object.
	{ID: "Lux-Magnetar-SLHDSA192s-Direct-v1", Kind: EvidenceMagnetarP3QSLHDSARollup, ParamSet: uint8(QuorumSchemeSLHDSA192s), Assumption: AssumptionDirect},
	{ID: "Lux-Magnetar-SLHDSA256s-Direct-v1", Kind: EvidenceMagnetarP3QSLHDSARollup, ParamSet: uint8(QuorumSchemeSLHDSA256s), Assumption: AssumptionDirect},
	{ID: "Lux-Magnetar-SLHDSA192s-STARK-v1", Kind: EvidenceMagnetarP3QSLHDSARollup, ParamSet: uint8(QuorumSchemeSLHDSA192s), Assumption: AssumptionPQSuccinct},
}

// suiteByID is the built, validated suite registry. Populated in init from
// productionSuites with every Leg/Mode resolved from laneByKind, so the
// registry can never name a leg/mode inconsistent with its kind.
var suiteByID = map[string]Suite{}

func init() {
	for _, s := range productionSuites {
		lane, ok := laneByKind[s.Kind]
		if !ok {
			panic(fmt.Sprintf("quasar: suite %q names unknown evidence kind %q", s.ID, s.Kind))
		}
		if _, dup := suiteByID[s.ID]; dup {
			panic(fmt.Sprintf("%v: %q", ErrSuiteDuplicate, s.ID))
		}
		// Resolve Leg/Mode from the lane mapping — never trust hand-set values.
		s.Leg = lane.leg
		s.Mode = lane.mode
		suiteByID[s.ID] = s
	}
}

// LookupSuite resolves a suite ID to its Suite. It is the ONLY way a suite
// string becomes a dispatch decision; an unregistered ID is (zero, false).
func LookupSuite(id string) (Suite, bool) {
	s, ok := suiteByID[id]
	return s, ok
}

// resolveSuiteForLane resolves a suite ID AND asserts it belongs to the
// expected evidence kind and (optionally) parameter set. This is the dispatch
// guard every suite-aware verifier calls: a suite from a different lane, or
// with a different param set, is a hard typed reject — no suite string can be
// coerced into the wrong verifier.
//
// expectParam == 0 means "do not pin the param set here" (the caller pins it
// against the era / leg spec instead).
func resolveSuiteForLane(id string, want EvidenceKind, expectParam uint8) (Suite, error) {
	s, ok := LookupSuite(id)
	if !ok {
		return Suite{}, fmt.Errorf("%w: %q", ErrUnknownSuite, id)
	}
	if s.Kind != want {
		return Suite{}, fmt.Errorf("%w: suite %q is %q, expected %q", ErrSuiteLaneMismatch, id, s.Kind, want)
	}
	if expectParam != 0 && s.ParamSet != expectParam {
		return Suite{}, fmt.Errorf("%w: suite %q param 0x%02x, expected 0x%02x", ErrSuiteParamMismatch, id, s.ParamSet, expectParam)
	}
	return s, nil
}
