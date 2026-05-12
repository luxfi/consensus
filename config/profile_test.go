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
		{ProfilePermissiveName, "permissive"},
		{ProfileStrict, "strict"},
		{ProfileFIPSx, "fips"},
	}
	for _, c := range cases {
		if got := c.p.String(); got != c.want {
			t.Errorf("Profile(%q).String() = %q, want %q", c.p, got, c.want)
		}
	}
}

// TestProfile_IsStrict pins the canonical "should this chain refuse
// classical primitives at the EVM gate?" predicate.
func TestProfile_IsStrict(t *testing.T) {
	cases := []struct {
		p    Profile
		want bool
	}{
		{ProfileStrict, true},
		{ProfileFIPSx, true},
		{ProfilePermissiveName, false},
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
// mapping. Each known Profile resolves to a struct that passes Validate.
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

// TestProfile_Resolve_Unknown asserts the typed-error path for unknown
// / empty Profile strings.
func TestProfile_Resolve_Unknown(t *testing.T) {
	cases := []Profile{"", "strict-pq", "STRICT", "hybrid", "bls", "classical", "nonsense"}
	for _, p := range cases {
		t.Run(string(p), func(t *testing.T) {
			_, err := p.Resolve()
			if err == nil {
				t.Fatalf("Profile(%q).Resolve() returned nil error; want unknown-profile error", p)
			}
			if !strings.Contains(err.Error(), "permissive, strict, fips") {
				t.Errorf("error message does not enumerate canonical values: %v", err)
			}
		})
	}
}

// TestProfile_WireByte pins the wire-byte mapping for each Profile.
func TestProfile_WireByte(t *testing.T) {
	cases := []struct {
		p    Profile
		want uint8
	}{
		{ProfileStrict, 0x01},
		{ProfilePermissiveName, 0x02},
		{ProfileFIPSx, 0x03},
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
				t.Fatalf("ProfileFromWireByte(WireByte(%q)): %v", p, err)
			}
			if got != p {
				t.Errorf("round-trip mismatch: got %q, want %q", got, p)
			}
		})
	}
}

// TestProfileFromWireByte_Unknown asserts unknown bytes return error.
func TestProfileFromWireByte_Unknown(t *testing.T) {
	for _, b := range []uint8{0x00, 0x04, 0x05, 0x80, 0xFF} {
		_, err := ProfileFromWireByte(b)
		if err == nil {
			t.Errorf("ProfileFromWireByte(0x%02x) returned nil error; want unknown-byte error", b)
		}
	}
}
