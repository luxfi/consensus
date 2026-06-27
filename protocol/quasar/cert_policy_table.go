// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// cert_policy_table.go — the named compact-evidence policy table.
//
// Four operator-facing finality postures, each a concrete ConsensusCertPolicy
// the envelope verifies under. Every posture pins ITS required leg KINDS (bound
// into the cert's RequiredLegsRoot) and ITS permitted (kind, mode, param)
// triples — the "OR" between Pulsar and P3Q is expressed at the MODE level on a
// single required leg KIND, never as an alternative leg:
//
//	BLS_FAST              require Beam only
//	    (mempool / fast local block acceptance)
//	HYBRID_PQ_CHECKPOINT  require Beam ∧ (Pulsar OR P3Q-rollup)
//	    (checkpoints: a compact Pulsar threshold sig, or the P3Q fallback)
//	STRICT_QUASAR         require Beam ∧ Pulsar ∧ Corona
//	    (full dual-lattice AND-mode strict finality)
//	RECOVERY_MODE         require Beam ∧ P3Q-rollup
//	    (recovery / migration / bridge: independent ML-DSA certs, no Pulsar key)
//
// The required PQ leg KIND is LegPulsarMLDSA in HYBRID / STRICT / RECOVERY; the
// MODE the policy permits for it differs: STRICT permits ONLY the compact
// threshold sig (EvidenceThresholdSig); RECOVERY permits ONLY the P3Q rollup
// (EvidenceP3QRollup); HYBRID permits EITHER. Because the required KINDS of
// HYBRID and RECOVERY coincide, their RequiredLegsRoot coincides — they are
// distinguished by EvidencePolicyID (bound into M) and by their Allows()
// mode-gate. This is the right decomplection: the required leg root commits the
// REQUIREMENT (kinds); the policy id + allow-table commit the MECHANISM (modes).
//
// This is the envelope-level realisation of the operator's cert posture; the
// operator selects a posture (config.CertPolicy on the chain VM) which resolves
// to one of these. Decomplected: this file owns ONLY the leg/mode requirement
// table. It decides nothing about message construction (quasar_finality.go),
// leg verification (the leg verifiers), or key eras (the KeyEra registry).
package quasar

import "fmt"

// QuasarEvidenceMode names one of the four finality postures.
type QuasarEvidenceMode uint8

const (
	// PolicyBLSFast — Beam only.
	PolicyBLSFast QuasarEvidenceMode = iota + 1

	// PolicyHybridPQCheckpoint — Beam ∧ (Pulsar OR P3Q-rollup).
	PolicyHybridPQCheckpoint

	// PolicyStrictQuasar — Beam ∧ Pulsar ∧ Corona.
	PolicyStrictQuasar

	// PolicyRecoveryMode — Beam ∧ P3Q-rollup.
	PolicyRecoveryMode
)

// String returns the canonical posture name.
func (m QuasarEvidenceMode) String() string {
	switch m {
	case PolicyBLSFast:
		return "BLS_FAST"
	case PolicyHybridPQCheckpoint:
		return "HYBRID_PQ_CHECKPOINT"
	case PolicyStrictQuasar:
		return "STRICT_QUASAR"
	case PolicyRecoveryMode:
		return "RECOVERY_MODE"
	default:
		return fmt.Sprintf("quasar-policy(%d)", uint8(m))
	}
}

// blsParam is the Beam leg's parameter-set byte (the classical BLS-12-381
// scheme byte). Pins the Beam suite's ParamSet so the policy gate matches.
const blsParam = uint8(ClassicalSchemeBLS12381)

// QuasarEvidencePolicy is the concrete ConsensusCertPolicy for one posture. It
// also implements ProofAssumptionPolicy (the optional classical-proof opt-in).
type QuasarEvidencePolicy struct {
	mode       QuasarEvidenceMode
	mldsaParam uint8  // ML-DSA param byte for the PQ legs (default 0x42 = ML-DSA-65)
	threshold  uint64 // BFT quorum weight floor
	policyID   uint32 // EvidencePolicyID, bound into M

	// acceptClassicalProof opts the policy into classical-assumption succinct
	// P3Q proofs (the Groth16 suite). Default false — fail closed, PQ-safe.
	acceptClassicalProof bool
}

// stablePolicyID assigns each posture a stable evidence-policy id so HYBRID and
// RECOVERY (which share required leg KINDS) are distinguishable in M.
func stablePolicyID(mode QuasarEvidenceMode) uint32 {
	return 0x0C0DE000 | uint32(mode)
}

// NewQuasarEvidencePolicy builds a posture policy. mldsaParam must be one of
// QuorumSchemeMLDSA44/65/87; 0 defaults to ML-DSA-65. threshold is the BFT
// quorum weight floor.
func NewQuasarEvidencePolicy(mode QuasarEvidenceMode, mldsaParam uint8, threshold uint64) *QuasarEvidencePolicy {
	if mldsaParam == 0 {
		mldsaParam = uint8(QuorumSchemeMLDSA65)
	}
	return &QuasarEvidencePolicy{
		mode:       mode,
		mldsaParam: mldsaParam,
		threshold:  threshold,
		policyID:   stablePolicyID(mode),
	}
}

// WithClassicalProofAssumption opts the policy into classical-assumption
// succinct P3Q proofs. Use only where a classical proof object is an acceptable
// risk and the raw cert set remains challengeable.
func (p *QuasarEvidencePolicy) WithClassicalProofAssumption() *QuasarEvidencePolicy {
	p.acceptClassicalProof = true
	return p
}

// Mode returns the posture.
func (p *QuasarEvidencePolicy) Mode() QuasarEvidenceMode { return p.mode }

// EvidencePolicyID returns the policy id bound into M.
func (p *QuasarEvidencePolicy) EvidencePolicyID() uint32 { return p.policyID }

// RequiredLegs returns the leg KINDS the posture requires (canonical order:
// Beam first, then Pulsar/PQ, then Corona). Bound into RequiredLegsRoot.
func (p *QuasarEvidencePolicy) RequiredLegs() []LegSpec {
	beam := LegSpec{Kind: LegClassical, ParamSetID: blsParam}
	pq := LegSpec{Kind: LegPulsarMLDSA, ParamSetID: p.mldsaParam}
	corona := LegSpec{Kind: LegCoronaLattice, ParamSetID: p.mldsaParam}
	switch p.mode {
	case PolicyBLSFast:
		return []LegSpec{beam}
	case PolicyHybridPQCheckpoint:
		return []LegSpec{beam, pq}
	case PolicyStrictQuasar:
		return []LegSpec{beam, pq, corona}
	case PolicyRecoveryMode:
		return []LegSpec{beam, pq}
	default:
		return nil
	}
}

// Allows gates the (kind, mode, param) triple. This is where the Pulsar-or-P3Q
// "OR" lives: the PQ leg KIND is fixed (LegPulsarMLDSA) but the permitted MODE
// depends on the posture.
func (p *QuasarEvidencePolicy) Allows(leg LegSpec, mode EvidenceMode, paramSet uint8) bool {
	switch leg.Kind {
	case LegClassical:
		// Beam — required by every posture.
		return mode == EvidenceClassicalAggregate && paramSet == blsParam

	case LegPulsarMLDSA:
		if paramSet != p.mldsaParam {
			return false
		}
		switch p.mode {
		case PolicyHybridPQCheckpoint:
			// Pulsar OR P3Q.
			return mode == EvidenceThresholdSig || mode == EvidenceP3QRollup
		case PolicyStrictQuasar:
			// Compact Pulsar threshold sig ONLY.
			return mode == EvidenceThresholdSig
		case PolicyRecoveryMode:
			// P3Q rollup ONLY.
			return mode == EvidenceP3QRollup
		default:
			return false
		}

	case LegCoronaLattice:
		// Corona — only in STRICT, only as a compact threshold sig.
		return p.mode == PolicyStrictQuasar &&
			mode == EvidenceThresholdSig && paramSet == p.mldsaParam

	default:
		return false
	}
}

// ThresholdWeight returns the BFT quorum weight floor.
func (p *QuasarEvidencePolicy) ThresholdWeight() uint64 { return p.threshold }

// AllowsClassicalScheme gates the Beam leg's classical scheme.
func (p *QuasarEvidencePolicy) AllowsClassicalScheme(scheme ClassicalScheme) bool {
	return scheme == ClassicalSchemeBLS12381
}

// AcceptsClassicalProofAssumption implements ProofAssumptionPolicy: whether the
// posture admits a classical-assumption succinct P3Q proof.
func (p *QuasarEvidencePolicy) AcceptsClassicalProofAssumption() bool {
	return p.acceptClassicalProof
}
