// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"errors"
	"testing"
)

var errLaneSentinel = errors.New("lane failed")

func okLane() error  { return nil }
func badLane() error { return errLaneSentinel }

// TestTier_Default asserts the production default is HYBRID_PQ.
func TestTier_Default(t *testing.T) {
	if DefaultSampledFinalityTier != TierHybridPQ {
		t.Fatalf("default tier = %s, want HYBRID_PQ", DefaultSampledFinalityTier)
	}
}

// TestTier_EvidenceModeMapping pins each tier to its envelope evidence mode.
func TestTier_EvidenceModeMapping(t *testing.T) {
	cases := map[SampledFinalityTier]QuasarEvidenceMode{
		TierFast:     PolicyBLSFast,
		TierHybridPQ: PolicyHybridPQCheckpoint,
		TierPQRoot:   PolicyRecoveryMode,
	}
	for tier, want := range cases {
		if got := tier.EvidenceMode(); got != want {
			t.Fatalf("%s.EvidenceMode() = %s, want %s", tier, got, want)
		}
	}
}

// TestTier_Fast: Beam only — passes with Beam ok regardless of the other lanes,
// and fails closed if Beam is missing.
func TestTier_Fast(t *testing.T) {
	if err := VerifySampledFinality(TierFast, SampledFinalityLanes{Beam: okLane}); err != nil {
		t.Fatalf("FAST with Beam ok should pass, got %v", err)
	}
	// Other lanes ignored entirely.
	if err := VerifySampledFinality(TierFast, SampledFinalityLanes{Beam: okLane, PulsarSampled: badLane, P3QRoot: badLane}); err != nil {
		t.Fatalf("FAST must ignore non-Beam lanes, got %v", err)
	}
	if err := VerifySampledFinality(TierFast, SampledFinalityLanes{}); !errors.Is(err, ErrTierLaneMissing) {
		t.Fatalf("FAST without Beam must be ErrTierLaneMissing, got %v", err)
	}
	if err := VerifySampledFinality(TierFast, SampledFinalityLanes{Beam: badLane}); !errors.Is(err, errLaneSentinel) {
		t.Fatalf("FAST must surface a failing Beam, got %v", err)
	}
}

// TestTier_HybridPQ: Beam ∧ PulsarSampled. Both required; the first failure
// surfaces unchanged; a missing required lane fails closed.
func TestTier_HybridPQ(t *testing.T) {
	if err := VerifySampledFinality(TierHybridPQ, SampledFinalityLanes{Beam: okLane, PulsarSampled: okLane}); err != nil {
		t.Fatalf("HYBRID with both ok should pass, got %v", err)
	}
	// Beam fails.
	if err := VerifySampledFinality(TierHybridPQ, SampledFinalityLanes{Beam: badLane, PulsarSampled: okLane}); !errors.Is(err, errLaneSentinel) {
		t.Fatalf("HYBRID must fail when Beam fails, got %v", err)
	}
	// Pulsar-sampled fails.
	if err := VerifySampledFinality(TierHybridPQ, SampledFinalityLanes{Beam: okLane, PulsarSampled: badLane}); !errors.Is(err, errLaneSentinel) {
		t.Fatalf("HYBRID must fail when Pulsar-sampled fails, got %v", err)
	}
	// Missing required lanes fail closed.
	if err := VerifySampledFinality(TierHybridPQ, SampledFinalityLanes{PulsarSampled: okLane}); !errors.Is(err, ErrTierLaneMissing) {
		t.Fatalf("HYBRID without Beam must be ErrTierLaneMissing, got %v", err)
	}
	if err := VerifySampledFinality(TierHybridPQ, SampledFinalityLanes{Beam: okLane}); !errors.Is(err, ErrTierLaneMissing) {
		t.Fatalf("HYBRID without Pulsar-sampled must be ErrTierLaneMissing, got %v", err)
	}
}

// TestTier_PQRoot: P3Q root mandatory; Beam verified if supplied (RECOVERY = Beam
// ∧ P3Q); a pure PQ-root may omit Beam.
func TestTier_PQRoot(t *testing.T) {
	// Pure PQ-root: only P3Q.
	if err := VerifySampledFinality(TierPQRoot, SampledFinalityLanes{P3QRoot: okLane}); err != nil {
		t.Fatalf("PQ_ROOT with P3Q ok should pass, got %v", err)
	}
	// RECOVERY: Beam ∧ P3Q.
	if err := VerifySampledFinality(TierPQRoot, SampledFinalityLanes{Beam: okLane, P3QRoot: okLane}); err != nil {
		t.Fatalf("PQ_ROOT (Beam ∧ P3Q) should pass, got %v", err)
	}
	// P3Q fails.
	if err := VerifySampledFinality(TierPQRoot, SampledFinalityLanes{P3QRoot: badLane}); !errors.Is(err, errLaneSentinel) {
		t.Fatalf("PQ_ROOT must fail when P3Q fails, got %v", err)
	}
	// Supplied Beam fails ⇒ whole posture fails.
	if err := VerifySampledFinality(TierPQRoot, SampledFinalityLanes{Beam: badLane, P3QRoot: okLane}); !errors.Is(err, errLaneSentinel) {
		t.Fatalf("PQ_ROOT must verify a supplied Beam, got %v", err)
	}
	// Missing P3Q fails closed.
	if err := VerifySampledFinality(TierPQRoot, SampledFinalityLanes{Beam: okLane}); !errors.Is(err, ErrTierLaneMissing) {
		t.Fatalf("PQ_ROOT without P3Q must be ErrTierLaneMissing, got %v", err)
	}
}

// TestTier_Unknown: an undefined tier value is a hard reject.
func TestTier_Unknown(t *testing.T) {
	if err := VerifySampledFinality(SampledFinalityTier(99), SampledFinalityLanes{Beam: okLane}); !errors.Is(err, ErrUnknownTier) {
		t.Fatalf("unknown tier must be ErrUnknownTier, got %v", err)
	}
}

// TestPulsarSampledLane_Integration wires the REAL VerifyPulsarSampled into the
// HYBRID_PQ tier through PulsarSampledLane: a valid sampled fixture passes the
// posture, and a tampered one propagates its typed error THROUGH the tier
// (errors.Is sees ErrSampledPlanMismatch / ErrInsufficientCommittees).
func TestPulsarSampledLane_Integration(t *testing.T) {
	// Valid: r committees sign; HYBRID with a passing Beam stub accepts.
	good := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.R))
	if err := VerifySampledFinality(TierHybridPQ, SampledFinalityLanes{
		Beam:          okLane,
		PulsarSampled: PulsarSampledLane(good.req),
	}); err != nil {
		t.Fatalf("HYBRID with a valid sampled cert should pass, got %v", err)
	}

	// Tampered plan hash propagates ErrSampledPlanMismatch through the tier.
	badPlan := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.M))
	badPlan.cert.PlanHash[0] ^= 1
	if err := VerifySampledFinality(TierHybridPQ, SampledFinalityLanes{
		Beam:          okLane,
		PulsarSampled: PulsarSampledLane(badPlan.req),
	}); !errors.Is(err, ErrSampledPlanMismatch) {
		t.Fatalf("tier must propagate ErrSampledPlanMismatch, got %v", err)
	}

	// Insufficient committees propagates ErrInsufficientCommittees through the tier.
	tooFew := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.R)-1)
	if err := VerifySampledFinality(TierHybridPQ, SampledFinalityLanes{
		Beam:          okLane,
		PulsarSampled: PulsarSampledLane(tooFew.req),
	}); !errors.Is(err, ErrInsufficientCommittees) {
		t.Fatalf("tier must propagate ErrInsufficientCommittees, got %v", err)
	}
}

// TestTier_RequiredLanes documents the lane requirements per tier.
func TestTier_RequiredLanes(t *testing.T) {
	cases := map[SampledFinalityTier][]string{
		TierFast:     {"beam"},
		TierHybridPQ: {"beam", "pulsar-sampled"},
		TierPQRoot:   {"p3q-root"},
	}
	for tier, want := range cases {
		got := tier.RequiredLanes()
		if len(got) != len(want) {
			t.Fatalf("%s.RequiredLanes() = %v, want %v", tier, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s.RequiredLanes()[%d] = %q, want %q", tier, i, got[i], want[i])
			}
		}
	}
}
