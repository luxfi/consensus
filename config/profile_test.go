// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"strings"
	"testing"
)

// TestProfile_String pins the canonical user-facing strings.
func TestProfile_String(t *testing.T) {
	cases := []struct {
		p    Profile
		want string
	}{
		{ProfileClassical, "classical"},
		{ProfileHybrid, "hybrid"},
		{ProfileStrict, "strict"},
	}
	for _, c := range cases {
		if got := c.p.String(); got != c.want {
			t.Errorf("Profile(%q).String() = %q, want %q", c.p, got, c.want)
		}
	}
}

// TestProfile_IsStrict pins the canonical "should this chain refuse
// classical primitives at the EVM gate?" predicate. Strict returns
// true; everything else false (Hybrid is PQ at the witness layer but
// admits classical contract-auth).
func TestProfile_IsStrict(t *testing.T) {
	cases := []struct {
		p    Profile
		want bool
	}{
		{ProfileClassical, false},
		{ProfileHybrid, false},
		{ProfileStrict, true},
		{Profile("permissive"), false},
		{Profile("fips"), false},
		{Profile(""), false},
		{Profile("nonsense"), false},
	}
	for _, c := range cases {
		if got := c.p.IsStrict(); got != c.want {
			t.Errorf("Profile(%q).IsStrict() = %v, want %v", c.p, got, c.want)
		}
	}
}

// TestProfile_Resolve pins the canonical Profile → ChainSecurityProfile
// mapping. Each known Profile resolves to a struct that passes
// Validate(); unknown Profiles return a typed error.
func TestProfile_Resolve(t *testing.T) {
	for _, p := range AllProfiles {
		t.Run(string(p), func(t *testing.T) {
			sp, err := p.Resolve()
			if err != nil {
				t.Fatalf("Profile(%q).Resolve(): %v", p, err)
			}
			if sp == nil {
				t.Fatalf("Profile(%q).Resolve() returned nil profile", p)
			}
			if err := sp.Validate(); err != nil {
				t.Fatalf("Profile(%q).Resolve().Validate(): %v", p, err)
			}
		})
	}
}

// TestProfile_Resolve_Unknown asserts the typed-error path for
// unknown / empty Profile strings.
func TestProfile_Resolve_Unknown(t *testing.T) {
	cases := []Profile{
		"",
		"strict-pq",
		"STRICT",
		"hybrid-pq",
		"nonsense",
	}
	for _, p := range cases {
		t.Run(string(p), func(t *testing.T) {
			_, err := p.Resolve()
			if err == nil {
				t.Fatalf("Profile(%q).Resolve() returned nil error; want unknown-profile error", p)
			}
			if !strings.Contains(err.Error(), "classical, hybrid, strict") {
				t.Errorf("error message does not enumerate canonical values: %v", err)
			}
		})
	}
}

// TestProfile_WireByte pins the wire-byte mapping for each Profile.
// Strict → 0x01, Hybrid → 0x04, Classical → 0x05.
func TestProfile_WireByte(t *testing.T) {
	cases := []struct {
		p    Profile
		want uint8
	}{
		{ProfileStrict, 0x01},
		{ProfileHybrid, 0x04},
		{ProfileClassical, 0x05},
		{Profile(""), 0x00},
		{Profile("nonsense"), 0x00},
	}
	for _, c := range cases {
		if got := c.p.WireByte(); got != c.want {
			t.Errorf("Profile(%q).WireByte() = 0x%02x, want 0x%02x", c.p, got, c.want)
		}
	}
}

// TestProfileFromWireByte_RoundTrip asserts WireByte and
// ProfileFromWireByte are inverses for every known Profile.
func TestProfileFromWireByte_RoundTrip(t *testing.T) {
	for _, p := range AllProfiles {
		t.Run(string(p), func(t *testing.T) {
			got, err := ProfileFromWireByte(p.WireByte())
			if err != nil {
				t.Fatalf("ProfileFromWireByte(0x%02x): %v", p.WireByte(), err)
			}
			if got != p {
				t.Errorf("round-trip drift: started with %q, got %q", p, got)
			}
		})
	}
}

// TestProfile_ResolveMatchesProfileByID asserts the string surface
// resolves to the same struct the byte-form ProfileByID returns. The
// two surfaces are different presentations of the same registry; a
// divergence here would mean operator config (string) and wire
// envelope (byte) disagree about which crypto policy applies.
func TestProfile_ResolveMatchesProfileByID(t *testing.T) {
	for _, p := range AllProfiles {
		t.Run(string(p), func(t *testing.T) {
			fromString, err := p.Resolve()
			if err != nil {
				t.Fatalf("Profile(%q).Resolve(): %v", p, err)
			}
			fromByte, err := ProfileByID(ProfileID(p.WireByte()))
			if err != nil {
				t.Fatalf("ProfileByID(0x%02x): %v", p.WireByte(), err)
			}
			if fromString.ProfileID != fromByte.ProfileID {
				t.Errorf("ProfileID mismatch: string=%d byte=%d", fromString.ProfileID, fromByte.ProfileID)
			}
			if fromString.ProfileName != fromByte.ProfileName {
				t.Errorf("ProfileName mismatch: string=%q byte=%q", fromString.ProfileName, fromByte.ProfileName)
			}
			if fromString.ProfileHash != fromByte.ProfileHash {
				t.Errorf("ProfileHash mismatch: string=%x byte=%x", fromString.ProfileHash, fromByte.ProfileHash)
			}
		})
	}
}

// TestProfile_HybridAdmitsClassicalAuth asserts the Hybrid profile's
// defining property: PQ at the consensus layer, classical permitted
// at the contract-auth layer. This is the transitional posture.
func TestProfile_HybridAdmitsClassicalAuth(t *testing.T) {
	sp, err := ProfileHybrid.Resolve()
	if err != nil {
		t.Fatalf("ProfileHybrid.Resolve(): %v", err)
	}
	if sp.ForbidECDSAContractAuth {
		t.Error("Hybrid must NOT set ForbidECDSAContractAuth (classical auth is the defining feature)")
	}
	if sp.ForbidPairings {
		t.Error("Hybrid must NOT set ForbidPairings (BLS aggregate still active at consensus layer)")
	}
	if !sp.IdentitySchemeID.IsRawMLDSA() {
		t.Errorf("Hybrid must pin PQ identity (ML-DSA), got %s", sp.IdentitySchemeID.String())
	}
	if !sp.FinalitySchemeID.IsPulsarM() {
		t.Errorf("Hybrid must pin PQ finality (Pulsar-M), got %s", sp.FinalitySchemeID.String())
	}
}

// TestProfile_ClassicalIsFullyOpen asserts the Classical profile has
// no enforced refusals at any layer — the explicit "no PQ" posture.
func TestProfile_ClassicalIsFullyOpen(t *testing.T) {
	sp, err := ProfileClassical.Resolve()
	if err != nil {
		t.Fatalf("ProfileClassical.Resolve(): %v", err)
	}
	if sp.ForbidECDSAContractAuth {
		t.Error("Classical must NOT set ForbidECDSAContractAuth")
	}
	if sp.ForbidECDSAWallets {
		t.Error("Classical must NOT set ForbidECDSAWallets")
	}
	if sp.ForbidClassicalKEM {
		t.Error("Classical must NOT set ForbidClassicalKEM")
	}
	if sp.ForbidBLSContractAuth {
		t.Error("Classical must NOT set ForbidBLSContractAuth")
	}
}

// TestProfile_StrictRefusesEverything asserts the Strict profile sets
// every Forbid* bit — the canonical "PQ-only, no classical anywhere"
// posture.
func TestProfile_StrictRefusesEverything(t *testing.T) {
	sp, err := ProfileStrict.Resolve()
	if err != nil {
		t.Fatalf("ProfileStrict.Resolve(): %v", err)
	}
	if !sp.ForbidECDSAContractAuth {
		t.Error("Strict must set ForbidECDSAContractAuth")
	}
	if !sp.ForbidECDSAWallets {
		t.Error("Strict must set ForbidECDSAWallets")
	}
	if !sp.ForbidBLSContractAuth {
		t.Error("Strict must set ForbidBLSContractAuth")
	}
	if !sp.ForbidClassicalKEM {
		t.Error("Strict must set ForbidClassicalKEM")
	}
	if !sp.ForbidPairings {
		t.Error("Strict must set ForbidPairings")
	}
	if !sp.ForbidKZG {
		t.Error("Strict must set ForbidKZG")
	}
}
