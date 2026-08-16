// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"errors"
	"reflect"
	"testing"
)

// TestCertPolicy_WireName_EighthSlot — (PQ-off, strict) is the eighth
// slot; WireName returns "" and Validate refuses.
func TestCertPolicy_WireName_EighthSlot(t *testing.T) {
	cp := CertPolicy{Mode: CertModeOff, Variant: CertVariantStrict}
	if got := cp.WireName(); got != "" {
		t.Errorf("(PQ-off,strict).WireName(): got %q want \"\"", got)
	}
}

// TestCertPolicy_RequiredLegs walks the seven configurations and
// asserts the leg set matches LP-217 §"The 4 modes".
func TestCertPolicy_RequiredLegs(t *testing.T) {
	cases := []struct {
		name   string
		policy CertPolicy
		legs   []LegName
	}{
		{"PQ-off",
			CertPolicy{Mode: CertModeOff, Variant: CertVariantHybrid},
			[]LegName{LegBLS}},
		{"PQ-fast",
			CertPolicy{Mode: CertModeFast, Variant: CertVariantHybrid},
			[]LegName{LegBLS, LegPulsar}},
		{"PQ-strict",
			CertPolicy{Mode: CertModeStrict, Variant: CertVariantHybrid},
			[]LegName{LegBLS, LegPulsar, LegCorona}},
		{"PQ-heavy",
			CertPolicy{Mode: CertModeHeavy, Variant: CertVariantHybrid},
			[]LegName{LegBLS, LegPulsar, LegCorona, LegMagnetar}},
		{"strict-PQ-fast",
			CertPolicy{Mode: CertModeFast, Variant: CertVariantStrict},
			[]LegName{LegPulsar}},
		{"strict-PQ-strict",
			CertPolicy{Mode: CertModeStrict, Variant: CertVariantStrict},
			[]LegName{LegPulsar, LegCorona}},
		{"strict-PQ-heavy",
			CertPolicy{Mode: CertModeHeavy, Variant: CertVariantStrict},
			[]LegName{LegPulsar, LegCorona, LegMagnetar}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.policy.RequiredLegs()
			if !reflect.DeepEqual(got, tc.legs) {
				t.Errorf("RequiredLegs: got %v want %v", got, tc.legs)
			}
		})
	}
}

// TestCertPolicy_Validate_Rule1 — Fallback > Mode is refused.
func TestCertPolicy_Validate_Rule1(t *testing.T) {
	cp := CertPolicy{
		Mode:      CertModeFast,
		Variant:   CertVariantHybrid,
		TimeoutMs: 1000,
		Fallback:  CertModeStrict, // > Mode
	}
	err := cp.Validate()
	if !errors.Is(err, ErrCertPolicyFallbackTooStrong) {
		t.Errorf("Validate: got %v want ErrCertPolicyFallbackTooStrong", err)
	}
}

// TestCertPolicy_Validate_Rule2 — Variant=Strict with Mode=PQ-off
// is refused.
func TestCertPolicy_Validate_Rule2(t *testing.T) {
	cp := CertPolicy{
		Mode:      CertModeOff,
		Variant:   CertVariantStrict,
		TimeoutMs: 1000,
		Fallback:  CertModeOff,
	}
	err := cp.Validate()
	if !errors.Is(err, ErrCertPolicyStrictOff) {
		t.Errorf("Validate: got %v want ErrCertPolicyStrictOff", err)
	}
}

// TestCertPolicy_Validate_Rule3 — TimeoutMs below 2 × floor latency
// is refused.
func TestCertPolicy_Validate_Rule3(t *testing.T) {
	cp := CertPolicy{
		Mode:      CertModeHeavy,
		Variant:   CertVariantHybrid,
		TimeoutMs: 100, // < 160 (2 × 80)
		Fallback:  CertModeStrict,
	}
	err := cp.Validate()
	if !errors.Is(err, ErrCertPolicyTimeoutTooShort) {
		t.Errorf("Validate: got %v want ErrCertPolicyTimeoutTooShort", err)
	}
}

// TestCertPolicy_Validate_Rule4 — Variant=Strict + Fallback=PQ-off
// is refused (the fallback as a (Mode, Variant) pair is invalid).
func TestCertPolicy_Validate_Rule4(t *testing.T) {
	cp := CertPolicy{
		Mode:      CertModeFast,
		Variant:   CertVariantStrict,
		TimeoutMs: 100,
		Fallback:  CertModeOff,
	}
	err := cp.Validate()
	if !errors.Is(err, ErrCertPolicyFallbackInvalid) {
		t.Errorf("Validate: got %v want ErrCertPolicyFallbackInvalid", err)
	}
}

// TestCertPolicy_Validate_Valid — every valid (Mode, Variant) pair
// with a satisfying TimeoutMs and Fallback passes Validate.
func TestCertPolicy_Validate_Valid(t *testing.T) {
	cases := []CertPolicy{
		{CertModeOff, CertVariantHybrid, 10, CertModeOff},
		{CertModeFast, CertVariantHybrid, 100, CertModeOff},
		{CertModeStrict, CertVariantHybrid, 1000, CertModeFast},
		{CertModeHeavy, CertVariantHybrid, 2000, CertModeStrict},
		{CertModeFast, CertVariantStrict, 100, CertModeFast},
		{CertModeStrict, CertVariantStrict, 1000, CertModeFast},
		{CertModeHeavy, CertVariantStrict, 2000, CertModeStrict},
	}
	for _, cp := range cases {
		if err := cp.Validate(); err != nil {
			t.Errorf("Validate %s: unexpected error %v", cp.WireName(), err)
		}
	}
}

// TestCertPolicy_IsPostQuantum — PQ-off is classical-only; everything
// else is post-quantum.
func TestCertPolicy_IsPostQuantum(t *testing.T) {
	if (CertPolicy{Mode: CertModeOff}).IsPostQuantum() {
		t.Error("PQ-off IsPostQuantum: got true want false")
	}
	if !(CertPolicy{Mode: CertModeFast}).IsPostQuantum() {
		t.Error("PQ-fast IsPostQuantum: got false want true")
	}
	if !(CertPolicy{Mode: CertModeHeavy}).IsPostQuantum() {
		t.Error("PQ-heavy IsPostQuantum: got false want true")
	}
}

// TestCertPolicy_AllowsBLS — Hybrid includes BLS; Strict does not.
func TestCertPolicy_AllowsBLS(t *testing.T) {
	if !(CertPolicy{Variant: CertVariantHybrid}).AllowsBLS() {
		t.Error("Hybrid AllowsBLS: got false want true")
	}
	if (CertPolicy{Variant: CertVariantStrict}).AllowsBLS() {
		t.Error("Strict AllowsBLS: got true want false")
	}
}
