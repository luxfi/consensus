package config

import (
	"fmt"
	"testing"
)

// Every code these enums can hold must render to a distinct string. Two codes
// sharing a name makes a policy log or a config dump ambiguous about which
// scheme a chain is actually running — and in particular an unrecognized code
// must never render as one of the canonical names, which is what would let an
// unsupported scheme read as a supported one.
func TestSchemeNamesAreUniquePerCode(t *testing.T) {
	for name, render := range map[string]func(int) string{
		"PQMode":           func(i int) string { return PQMode(i).String() },
		"HashSuiteID":      func(i int) string { return HashSuiteID(i).String() },
		"SigSchemeID":      func(i int) string { return SigSchemeID(i).String() },
		"ProofPolicyID":    func(i int) string { return ProofPolicyID(i).String() },
		"ProofBackendID":   func(i int) string { return ProofBackendID(i).String() },
		"IdentitySchemeID": func(i int) string { return IdentitySchemeID(i).String() },
		"WalletSchemeID":   func(i int) string { return WalletSchemeID(i).String() },
		"TxSchemeID":       func(i int) string { return TxSchemeID(i).String() },
		"ContractAuthID":   func(i int) string { return ContractAuthID(i).String() },
		"KeyExchangeID":    func(i int) string { return KeyExchangeID(i).String() },
		"RecoverySchemeID": func(i int) string { return RecoverySchemeID(i).String() },
		"CertMode":         func(i int) string { return CertMode(i).String() },
		"CertVariant":      func(i int) string { return CertVariant(i).String() },
		"ProfileID":        func(i int) string { return ProfileID(i).String() },
		"ProofFormatID":    func(i int) string { return ProofFormatID(i).String() },
		"VerifierID":       func(i int) string { return VerifierID(i).String() },
	} {
		t.Run(name, func(t *testing.T) {
			seen := make(map[string]int, 256)
			for code := 0; code < 256; code++ {
				s := render(code)
				if prev, dup := seen[s]; dup {
					t.Errorf("codes %d and %d both render %q", prev, code, s)
				}
				seen[s] = code
			}
		})
	}
}

// The derived properties of a PQ mode must stay defined for a code that is not
// one: an unrecognized mode falls back to the most conservative answer rather
// than inheriting whatever the previous case arm returned.
func TestUnknownPQModeFallsBackConservatively(t *testing.T) {
	const unknown = PQMode(0xFE)

	if got := unknown.String(); got != fmt.Sprintf("pq-mode(%d)", uint8(unknown)) {
		t.Errorf("String() = %q, want it to name the unrecognized code", got)
	}
	// PolicyQuorum (1) claims no PQ witness set. An unrecognized mode must land
	// here and not on 4/5/6, which would advertise a witness it cannot produce.
	if got := unknown.PolicyID(); got != 1 {
		t.Errorf("PolicyID() = %d, want 1: an unrecognized mode must claim no PQ witness", got)
	}
	if got := unknown.HashSuiteID(); got.String() == "" {
		t.Error("HashSuiteID() rendered empty for an unrecognized mode")
	}
	if got := unknown.SigSchemeID(); got.String() == "" {
		t.Error("SigSchemeID() rendered empty for an unrecognized mode")
	}
	if got := unknown.DKGRequired(); got != "unknown" {
		t.Errorf("DKGRequired() = %q, want %q", got, "unknown")
	}
}

// A policy whose variant is neither hybrid nor strict has no wire name, and the
// (PQ-off, strict) pair is the one defined combination that is also nameless —
// both must return empty so a caller that skipped Validate cannot transmit a
// name for a policy that has none.
func TestPolicyWithoutAWireNameReturnsEmpty(t *testing.T) {
	for name, cp := range map[string]CertPolicy{
		"unknown variant": {Mode: CertModeStrict, Variant: CertVariant(9)},
		"off and strict":  {Mode: CertModeOff, Variant: CertVariantStrict},
	} {
		if got := cp.WireName(); got != "" {
			t.Errorf("%s: WireName() = %q, want empty", name, got)
		}
	}
}

// An unrecognized mode has no floor latency to speak of; reporting a defined
// mode's floor would let it pass a timeout check it was never measured for.
func TestUnknownCertModeHasNoLatencyFloor(t *testing.T) {
	if got := CertMode(9).expectedFloorLatencyMs(); got != 0 {
		t.Errorf("expectedFloorLatencyMs() = %d, want 0", got)
	}
}

// Fault tolerance is a count of validators, so a sample below one tolerates
// zero faults. Breaks if the formula is allowed to go negative, which would
// make every downstream f>=1 check pass by underflow.
func TestFaultToleranceNeverGoesNegative(t *testing.T) {
	for k := -5; k <= 21; k++ {
		p := MainnetParams()
		p.K = k
		if f := p.ByzantineFaultTolerance(); f < 0 {
			t.Errorf("K=%d gives f=%d", k, f)
		}
	}
}

// The seven nameable (mode, variant) pairs each get their canonical wire name,
// hybrid unprefixed and strict prefixed. Breaks if a strict policy ever
// transmits under a hybrid name, which would let a peer read a PQ-only
// requirement as one that still admits the classical leg.
func TestWireNamesDistinguishStrictFromHybrid(t *testing.T) {
	for mode, hybrid := range map[CertMode]string{
		CertModeOff:    "PQ-off",
		CertModeFast:   "PQ-fast",
		CertModeStrict: "PQ-strict",
		CertModeHeavy:  "PQ-heavy",
	} {
		if got := (CertPolicy{Mode: mode, Variant: CertVariantHybrid}).WireName(); got != hybrid {
			t.Errorf("hybrid %v: got %q, want %q", mode, got, hybrid)
		}
		if mode == CertModeOff {
			continue // (PQ-off, strict) is the nameless eighth slot
		}
		if got, want := (CertPolicy{Mode: mode, Variant: CertVariantStrict}).WireName(), "strict-"+hybrid; got != want {
			t.Errorf("strict %v: got %q, want %q", mode, got, want)
		}
	}
}
