// Copyright (C) 2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import "testing"

// BenchmarkCertPolicy_Validate isolates the rule-check path (no
// parsing).
func BenchmarkCertPolicy_Validate(b *testing.B) {
	cp := CertPolicy{Mode: CertModeStrict, Variant: CertVariantHybrid, TimeoutMs: 5_000, Fallback: CertModeFast}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cp.Validate(); err != nil {
			b.Fatalf("Validate: %v", err)
		}
	}
}

// BenchmarkCertPolicy_RequiredLegs measures the leg-set computation.
// Called per cert verification to know which legs to require.
func BenchmarkCertPolicy_RequiredLegs(b *testing.B) {
	cp := CertPolicy{Mode: CertModeHeavy, Variant: CertVariantHybrid, TimeoutMs: 8_000, Fallback: CertModeFast}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cp.RequiredLegs()
	}
}

// BenchmarkCertPolicy_WireName measures the wire-name format path
// (called on every cert emission for the wire identifier).
func BenchmarkCertPolicy_WireName(b *testing.B) {
	cp := CertPolicy{Mode: CertModeHeavy, Variant: CertVariantStrict, TimeoutMs: 8_000, Fallback: CertModeFast}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cp.WireName()
	}
}
