// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// pulsar_sampled_policy.go — the three-tier finality posture for the Pulsar
// sampled-certificate layer, composed from orthogonal lane verifiers.
//
// # The three tiers (owner spec)
//
//	FAST       = VerifyBeam(M)                          — local fast confirmation
//	HYBRID_PQ  = VerifyBeam(M) ∧ VerifyPulsarSampled(M) — DEFAULT production finality
//	PQ_ROOT    = P3Q over ≥2/3 weighted independent     — checkpoints / bridges /
//	             ML-DSA (+ Beam)                           emergency / high-value
//
// HYBRID_PQ is the default: Beam (fast classical BLS) AND the r-of-m sampled
// dealerless-Mithril certificate, both over the same finalized block. The PQ
// confidence is the repetition count r against the per-committee capture
// probability (pulsar_sampled_security.go), the post-quantum analogue of
// Avalanche's β confidence. PQ_ROOT is the heavyweight maximal-trustless PQ root
// — the P3Q large-quorum rollup over ≥2/3 weighted INDEPENDENT validator ML-DSA
// signatures (EvidenceP3QMLDSARollup) — reached when the sampled-committee keys
// are not live, a committee is rotating, or a high-value checkpoint warrants the
// large-quorum root.
//
// # Composition, not entanglement
//
// This file owns ONLY the tier → required-lane structure. It does NOT construct
// the subject M, know any signature math, or re-implement Beam / P3Q. Each tier
// composes lane verifier CLOSURES — the caller (the consensus engine) wires each
// closure with the right evidence and the right M already bound:
//
//   - the Beam lane wraps the existing classical BLS aggregate verify;
//   - the Pulsar-sampled lane wraps VerifyPulsarSampled (PulsarSampledLane);
//   - the P3Q-root lane wraps the existing P3Q rollup verify (which itself
//     enforces the ≥2/3 weighted-quorum bar).
//
// The orchestrator enforces only the AND across the lanes a tier requires. This
// is the same decomplection the envelope policy table uses (cert_policy_table.go):
// the posture commits the REQUIREMENT; the lane verifiers own the MECHANISM.
//
// # Mapping onto the existing Quasar evidence-mode table
//
// Each tier maps 1:1 onto a QuasarEvidenceMode (cert_policy_table.go), so a
// sampled-cert posture selects the same envelope policy id bound into M:
//
//	TierFast      → PolicyBLSFast              (Beam only)
//	TierHybridPQ  → PolicyHybridPQCheckpoint   (Beam ∧ PQ)
//	TierPQRoot    → PolicyRecoveryMode         (Beam ∧ P3Q rollup)
//
// The envelope table expresses the single-committee Pulsar-OR-P3Q at the MODE
// level on one leg KIND; this sampled-cert tier set is the realisation of that
// PQ requirement by the SAMPLED r-of-m mechanism for HYBRID, and by the P3Q
// rollup for PQ_ROOT.
package quasar

import (
	"errors"
	"fmt"
)

// SampledFinalityTier is one of the three sampled-certificate finality postures.
type SampledFinalityTier uint8

const (
	// TierFast — Beam (classical BLS) only. Fast local block acceptance.
	TierFast SampledFinalityTier = iota + 1

	// TierHybridPQ — Beam ∧ VerifyPulsarSampled. The DEFAULT production finality
	// posture: fast classical confirmation plus the r-of-m sampled dealerless
	// Mithril post-quantum certificate over the same finalized block.
	TierHybridPQ

	// TierPQRoot — the P3Q large-quorum rollup over ≥2/3 weighted INDEPENDENT
	// validator ML-DSA signatures (with Beam if supplied). The heavyweight
	// maximal-trustless PQ root for checkpoints / bridges / emergency / recovery.
	TierPQRoot
)

// DefaultSampledFinalityTier is the production default: HYBRID_PQ.
var DefaultSampledFinalityTier = TierHybridPQ

// String returns the canonical posture name.
func (tier SampledFinalityTier) String() string {
	switch tier {
	case TierFast:
		return "FAST"
	case TierHybridPQ:
		return "HYBRID_PQ"
	case TierPQRoot:
		return "PQ_ROOT"
	default:
		return fmt.Sprintf("sampled-tier(%d)", uint8(tier))
	}
}

// EvidenceMode maps the tier onto the existing envelope QuasarEvidenceMode, so a
// sampled-cert posture selects the same evidence-policy id bound into M.
func (tier SampledFinalityTier) EvidenceMode() QuasarEvidenceMode {
	switch tier {
	case TierFast:
		return PolicyBLSFast
	case TierHybridPQ:
		return PolicyHybridPQCheckpoint
	case TierPQRoot:
		return PolicyRecoveryMode
	default:
		return 0
	}
}

// RequiredLanes names the lanes the tier requires, in verification order — for
// logging and audit.
func (tier SampledFinalityTier) RequiredLanes() []string {
	switch tier {
	case TierFast:
		return []string{"beam"}
	case TierHybridPQ:
		return []string{"beam", "pulsar-sampled"}
	case TierPQRoot:
		return []string{"p3q-root"}
	default:
		return nil
	}
}

// Typed policy errors.
var (
	// ErrUnknownTier — the tier value is not one of the three defined postures.
	ErrUnknownTier = errors.New("quasar: unknown sampled finality tier")

	// ErrTierLaneMissing — a lane the tier requires was not supplied (nil
	// closure). Fail closed: a missing required lane is never an implicit pass.
	ErrTierLaneMissing = errors.New("quasar: required finality lane verifier is nil")
)

// SampledLaneVerifier is a fully-bound lane check: it verifies its lane's
// evidence over the finality subject the caller has already pinned, returning
// nil on success. The policy composes these; it knows no signature math.
type SampledLaneVerifier func() error

// SampledFinalityLanes bundles the composable lane checks a tier may require. A
// tier ignores lanes it does not require (they may be nil). The Beam lane is
// optional for TierPQRoot (a pure PQ-root supplies only P3Q; RECOVERY composes
// Beam ∧ P3Q by supplying both).
type SampledFinalityLanes struct {
	Beam          SampledLaneVerifier
	PulsarSampled SampledLaneVerifier
	P3QRoot       SampledLaneVerifier
}

// Verify enforces the tier's required-lane AND structure over the supplied lane
// verifiers. It runs the required lanes in canonical order and returns the FIRST
// failing lane's error unchanged (so errors.Is sees the underlying lane error,
// e.g. ErrInsufficientCommittees). A required lane that was not supplied is
// ErrTierLaneMissing — fail closed, never an implicit pass.
func (tier SampledFinalityLanes) verifyForTier(t SampledFinalityTier) error {
	switch t {
	case TierFast:
		return runLane("beam", tier.Beam)
	case TierHybridPQ:
		if err := runLane("beam", tier.Beam); err != nil {
			return err
		}
		return runLane("pulsar-sampled", tier.PulsarSampled)
	case TierPQRoot:
		// P3Q root is mandatory; Beam is verified too if supplied (RECOVERY = Beam
		// ∧ P3Q). A pure PQ-root may omit Beam.
		if tier.Beam != nil {
			if err := runLane("beam", tier.Beam); err != nil {
				return err
			}
		}
		return runLane("p3q-root", tier.P3QRoot)
	default:
		return fmt.Errorf("%w: %d", ErrUnknownTier, uint8(t))
	}
}

// Verify checks the supplied lanes satisfy the tier's posture. It is the
// canonical entry point: VerifySampledFinality(tier, lanes).
func VerifySampledFinality(tier SampledFinalityTier, lanes SampledFinalityLanes) error {
	return lanes.verifyForTier(tier)
}

// runLane runs one required lane, treating a nil closure as a hard
// ErrTierLaneMissing (named for the operator).
func runLane(name string, v SampledLaneVerifier) error {
	if v == nil {
		return fmt.Errorf("%w: %s", ErrTierLaneMissing, name)
	}
	return v()
}

// PulsarSampledLane adapts a PulsarSampledVerifyRequest into a lane verifier for
// the HYBRID_PQ tier: it runs VerifyPulsarSampled and reports pass/fail. Callers
// that also need the derived subject M (e.g. to confirm the Beam QC commitment)
// should call VerifyPulsarSampled directly and bind M themselves.
func PulsarSampledLane(req PulsarSampledVerifyRequest) SampledLaneVerifier {
	return func() error {
		_, err := VerifyPulsarSampled(req)
		return err
	}
}
