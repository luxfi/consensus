// Copyright (C) 2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import "testing"

// BenchmarkParseCertPolicy measures ParseCertPolicy(strings) — the
// genesis-loader path that turns YAML strings into a validated
// CertPolicy. Includes parseMode×2 + variant switch + Validate.
func BenchmarkParseCertPolicy(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseCertPolicy("PQ-strict", "hybrid", 5_000, "PQ-fast")
		if err != nil {
			b.Fatalf("ParseCertPolicy: %v", err)
		}
	}
}

// BenchmarkParseCertPolicy_Strict measures the strict-PQ variant.
func BenchmarkParseCertPolicy_Strict(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseCertPolicy("PQ-heavy", "strict", 8_000, "PQ-fast")
		if err != nil {
			b.Fatalf("ParseCertPolicy: %v", err)
		}
	}
}

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

// BenchmarkParseWireName measures the inverse — parsing one of the
// seven canonical wire names back to (Mode, Variant).
func BenchmarkParseWireName(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := ParseWireName("strict-PQ-heavy")
		if err != nil {
			b.Fatalf("ParseWireName: %v", err)
		}
	}
}
