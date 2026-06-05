// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package config — cert_policy.go is the canonical operator-facing
// configuration record for a chain's cert posture, per LP-217
// §"Operator config".
//
// One struct. Four fields. One decision per chain.
//
//	Mode      — PQ-off | PQ-fast | PQ-strict | PQ-heavy
//	Variant   — Hybrid (BLS + PQ legs) | Strict (pure PQ, no BLS)
//	TimeoutMs — max wait for full-mode cert; past TimeoutMs LP-202
//	            tier degradation kicks in
//	Fallback  — tier to settle at if Mode's legs do not arrive in
//	            TimeoutMs; MUST satisfy Fallback <= Mode
//
// Three knobs previously scattered across three LPs (LP-217's
// cert_mode, LP-202's cert_timeout_ms, LP-204's per-L1 mode picker)
// collapse to four fields of one record. LP-202 references
// CertPolicy.TimeoutMs and CertPolicy.Fallback for the temporal
// degradation contract. LP-204 attaches a CertPolicy per chain-VM
// (default: inherit from parent L1). LP-218 reuses CertPolicy.Mode as
// the rollup's inherited cert mode.
//
// CertPolicy is validated at chain genesis (Validate() below); a
// chain MUST refuse to launch with an invalid CertPolicy. There is
// no migration; the new final Lux network starts at genesis with
// CertPolicy.
//
// CertPolicy is distinct from pq_mode.go's PQMode enum. pq_mode.go
// owns the v1 INTERNAL codenames (PQModePulsar, PQModeQuasar,
// PQModeMLDSA, ...) — the cryptographic building blocks. CertPolicy
// is the v2 OPERATOR-FACING surface that maps to internal codenames
// via ToV1Predicate.

package config

import (
	"errors"
	"fmt"
	"strings"
)

// CertPolicy is the single configuration record that declares a
// chain's cert posture. See LP-217 §"Operator config".
type CertPolicy struct {
	Mode      CertMode
	Variant   CertVariant
	TimeoutMs uint32
	Fallback  CertMode
}

// CertMode is the additive cert-mode ladder. Subset-ordered:
//
//	CertModeOff < CertModeFast < CertModeStrict < CertModeHeavy
//
// (Named CertMode here, not PQMode, to keep the namespace clean
// against pq_mode.go's PQMode which owns the v1 codename enum.)
type CertMode uint8

const (
	// CertModeOff — Hybrid: BLS only. Strict: (invalid; refused by
	// Validate). Floor latency ~1 ms final on Blackwell N=64.
	CertModeOff CertMode = 0

	// CertModeFast — Hybrid: BLS + Pulsar. Strict: Pulsar only.
	// Floor latency ~5 ms GPU on Blackwell N=64.
	CertModeFast CertMode = 1

	// CertModeStrict — Hybrid: BLS + Pulsar + Corona. Strict: Pulsar +
	// Corona. Floor latency ~50 ms GPU on Blackwell N=64.
	CertModeStrict CertMode = 2

	// CertModeHeavy — Hybrid: BLS + Pulsar + Corona + Magnetar.
	// Strict: Pulsar + Corona + Magnetar. Floor latency ~80 ms final
	// on Blackwell N=64.
	CertModeHeavy CertMode = 3
)

// CertVariant picks whether BLS is part of the required leg set.
type CertVariant uint8

const (
	// CertVariantHybrid — include BLS as classical leg.
	CertVariantHybrid CertVariant = 0

	// CertVariantStrict — drop BLS; pure PQ legs only.
	CertVariantStrict CertVariant = 1
)

// LegName is the wire identifier of a single cert leg. Values match
// LP-182 QuasarCert field tags.
type LegName uint8

const (
	LegBLS      LegName = 1
	LegPulsar   LegName = 2
	LegCorona   LegName = 3
	LegMagnetar LegName = 4
)

// String returns the canonical mode name (without variant prefix).
func (m CertMode) String() string {
	switch m {
	case CertModeOff:
		return "PQ-off"
	case CertModeFast:
		return "PQ-fast"
	case CertModeStrict:
		return "PQ-strict"
	case CertModeHeavy:
		return "PQ-heavy"
	default:
		return fmt.Sprintf("cert-mode(%d)", uint8(m))
	}
}

// String returns the canonical variant name.
func (v CertVariant) String() string {
	switch v {
	case CertVariantHybrid:
		return "hybrid"
	case CertVariantStrict:
		return "strict"
	default:
		return fmt.Sprintf("cert-variant(%d)", uint8(v))
	}
}

// WireName returns the seven canonical wire names from LP-217
// §"(Mode, Variant) enumeration":
//
//	(PQ-off,    hybrid) -> PQ-off
//	(PQ-fast,   hybrid) -> PQ-fast
//	(PQ-strict, hybrid) -> PQ-strict
//	(PQ-heavy,  hybrid) -> PQ-heavy
//	(PQ-fast,   strict) -> strict-PQ-fast
//	(PQ-strict, strict) -> strict-PQ-strict
//	(PQ-heavy,  strict) -> strict-PQ-heavy
//
// The eighth slot (PQ-off, strict) is invalid and returns the empty
// string; callers MUST run Validate() before relying on WireName.
func (cp CertPolicy) WireName() string {
	switch cp.Variant {
	case CertVariantHybrid:
		return cp.Mode.String()
	case CertVariantStrict:
		if cp.Mode == CertModeOff {
			return ""
		}
		return "strict-" + cp.Mode.String()
	default:
		return ""
	}
}

// RequiredLegs returns the leg set the cert MUST include to satisfy
// the policy's Mode. Result is sorted (BLS first if present, then
// Pulsar, Corona, Magnetar).
func (cp CertPolicy) RequiredLegs() []LegName {
	legs := make([]LegName, 0, 4)
	if cp.Variant == CertVariantHybrid && cp.Mode >= CertModeOff {
		legs = append(legs, LegBLS)
	}
	if cp.Mode >= CertModeFast {
		legs = append(legs, LegPulsar)
	}
	if cp.Mode >= CertModeStrict {
		legs = append(legs, LegCorona)
	}
	if cp.Mode >= CertModeHeavy {
		legs = append(legs, LegMagnetar)
	}
	return legs
}

// parseMode parses one of the four canonical mode names.
func parseMode(s string) (CertMode, error) {
	switch s {
	case "PQ-off":
		return CertModeOff, nil
	case "PQ-fast":
		return CertModeFast, nil
	case "PQ-strict":
		return CertModeStrict, nil
	case "PQ-heavy":
		return CertModeHeavy, nil
	default:
		return 0, fmt.Errorf("unknown cert mode %q", s)
	}
}

// ParseWireName parses one of the seven canonical wire names into a
// (Mode, Variant) pair. Returns an error for any other string.
func ParseWireName(s string) (CertMode, CertVariant, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "strict-") {
		m, err := parseMode(strings.TrimPrefix(s, "strict-"))
		if err != nil {
			return 0, 0, err
		}
		return m, CertVariantStrict, nil
	}
	m, err := parseMode(s)
	if err != nil {
		return 0, 0, err
	}
	return m, CertVariantHybrid, nil
}

// ParseCertPolicy parses a YAML-style record into a CertPolicy.
// Caller passes the four fields as strings (mode, variant, timeout,
// fallback) — the typical genesis-loader path reads the YAML map and
// hands strings here so the struct stays decoupled from yaml.v3.
//
// Returns the parsed policy AND any error from Validate. A non-nil
// error means the policy is unsafe to launch the chain with.
func ParseCertPolicy(mode, variant string, timeoutMs uint32, fallback string) (CertPolicy, error) {
	m, err := parseMode(mode)
	if err != nil {
		return CertPolicy{}, fmt.Errorf("cert_policy.mode: %w", err)
	}
	var v CertVariant
	switch strings.TrimSpace(variant) {
	case "", "hybrid":
		v = CertVariantHybrid
	case "strict":
		v = CertVariantStrict
	default:
		return CertPolicy{}, fmt.Errorf("cert_policy.variant: unknown variant %q (want hybrid|strict)", variant)
	}
	f, err := parseMode(fallback)
	if err != nil {
		return CertPolicy{}, fmt.Errorf("cert_policy.fallback: %w", err)
	}
	cp := CertPolicy{
		Mode:      m,
		Variant:   v,
		TimeoutMs: timeoutMs,
		Fallback:  f,
	}
	return cp, cp.Validate()
}

// expectedFloorLatencyMs returns the floor latency for a Mode on
// Blackwell N=64. These are pinned values from LP-217 §"The 4 modes"
// Table 1. Validators with non-Blackwell hardware multiply through
// CPU/M4-Max floors per LP-203.
func (m CertMode) expectedFloorLatencyMs() uint32 {
	switch m {
	case CertModeOff:
		return 1
	case CertModeFast:
		return 5
	case CertModeStrict:
		return 50
	case CertModeHeavy:
		return 80
	default:
		return 0
	}
}

// MinTimeoutMs returns the minimum TimeoutMs for this Mode (2 ×
// floor latency, per LP-217 §"Validation rules" rule 3). The 2×
// factor is the minimum; production deployments typically use 4–10×
// to absorb jitter.
func (m CertMode) MinTimeoutMs() uint32 {
	return 2 * m.expectedFloorLatencyMs()
}

// Common validation errors.
var (
	// ErrCertPolicyFallbackTooStrong — Fallback is stronger than
	// Mode; a fallback by definition cannot require more legs than
	// the target.
	ErrCertPolicyFallbackTooStrong = errors.New(
		"cert_policy: fallback tier cannot be stronger than mode")

	// ErrCertPolicyStrictOff — Variant=Strict with Mode=PQ-off
	// produces a cert with no legs; refused.
	ErrCertPolicyStrictOff = errors.New(
		"cert_policy: variant=strict requires mode>=PQ-fast (PQ-off has no PQ legs)")

	// ErrCertPolicyTimeoutTooShort — TimeoutMs < 2 ×
	// expected_floor_latency(Mode); chain at this Mode would never
	// satisfy the required set within the timeout.
	ErrCertPolicyTimeoutTooShort = errors.New(
		"cert_policy: timeout_ms < 2 × expected_floor_latency(mode)")

	// ErrCertPolicyFallbackInvalid — Fallback as a (Mode, Variant)
	// pair under the chain's Variant is itself invalid (e.g.
	// Variant=Strict with Fallback=PQ-off).
	ErrCertPolicyFallbackInvalid = errors.New(
		"cert_policy: fallback is not a valid (Mode, Variant) under the chain's Variant")
)

// Validate enforces the four LP-217 §"Validation rules":
//
//  1. Fallback <= Mode
//  2. Variant=Strict requires Mode >= PQ-fast
//  3. TimeoutMs >= 2 × expected_floor_latency(Mode)
//  4. Fallback as a (Mode, Variant) under the chain's Variant must
//     itself be valid (i.e. the fallback policy passes rule 2 too)
//
// Validate is called at chain genesis; a chain MUST refuse to launch
// when Validate returns a non-nil error.
func (cp CertPolicy) Validate() error {
	// Rule 2 — Variant=Strict requires Mode >= PQ-fast.
	if cp.Variant == CertVariantStrict && cp.Mode == CertModeOff {
		return ErrCertPolicyStrictOff
	}

	// Rule 1 — Fallback <= Mode.
	if cp.Fallback > cp.Mode {
		return fmt.Errorf("%w: fallback=%s mode=%s",
			ErrCertPolicyFallbackTooStrong, cp.Fallback, cp.Mode)
	}

	// Rule 3 — TimeoutMs >= 2 × expected_floor_latency(Mode).
	if min := cp.Mode.MinTimeoutMs(); cp.TimeoutMs < min {
		return fmt.Errorf("%w: mode=%s timeout_ms=%d min=%d",
			ErrCertPolicyTimeoutTooShort, cp.Mode, cp.TimeoutMs, min)
	}

	// Rule 4 — Fallback as a (Mode, Variant) under the chain's
	// Variant must itself be valid. The only way this fails is
	// Variant=Strict with Fallback=PQ-off (the eighth-slot
	// configuration). Catching it inline gives a more readable error
	// than recursing on a synthesized fallback policy.
	if cp.Variant == CertVariantStrict && cp.Fallback == CertModeOff {
		return fmt.Errorf("%w: fallback=%s variant=%s",
			ErrCertPolicyFallbackInvalid, cp.Fallback, cp.Variant)
	}

	return nil
}

// IsPostQuantum reports whether the policy includes any PQ leg in
// its required set. False only for (Mode=PQ-off, Variant=Hybrid)
// which is BLS-only.
func (cp CertPolicy) IsPostQuantum() bool {
	return cp.Mode >= CertModeFast
}

// AllowsBLS reports whether the policy includes BLS in its required
// set. True for Variant=Hybrid; false for Variant=Strict.
func (cp CertPolicy) AllowsBLS() bool {
	return cp.Variant == CertVariantHybrid
}
