// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"strings"
	"testing"
)

// The three literals operators write in chain config, in increasing order of
// enforcement. Spelled out rather than read from the package so that renaming a
// constant's value breaks this test instead of silently invalidating every
// deployed YAML.
var profileLiterals = map[Profile]string{
	profilePermissive: "permissive",
	profileStrict:     "strict",
	profileFIPS:       "fips",
}

// Each operator string resolves to a security profile that validates, and keeps
// its literal spelling. Breaks if a profile is renamed out from under deployed
// configs, or resolves to a struct that would be refused at boot.
func TestEveryProfileResolvesAndValidates(t *testing.T) {
	for p, literal := range profileLiterals {
		t.Run(literal, func(t *testing.T) {
			if got := p.String(); got != literal {
				t.Errorf("String() = %q, want %q", got, literal)
			}

			sp, err := p.Resolve()
			if err != nil {
				t.Fatalf("Resolve(): %v", err)
			}
			if sp == nil {
				t.Fatal("Resolve() returned nil profile")
			}
			if err := sp.Validate(); err != nil {
				t.Fatalf("resolved profile does not validate: %v", err)
			}
		})
	}
}

// Anything that is not one of the three literals must fail loudly at config
// load, naming the alternatives — never fall through to a default posture.
// Breaks if an unknown or empty profile starts resolving to something.
func TestUnknownProfileIsRefused(t *testing.T) {
	for _, p := range []Profile{"", "strict-pq", "STRICT", "hybrid", "bls", "classical", "nonsense"} {
		t.Run(string(p), func(t *testing.T) {
			if _, err := p.Resolve(); err == nil {
				t.Fatal("resolved; want an unknown-profile error")
			} else if !strings.Contains(err.Error(), "permissive, strict, fips") {
				t.Errorf("error does not name the alternatives: %v", err)
			}
		})
	}
}

// IsStrict decides whether a chain installs the all-classical-forbidden set at
// the precompile boundary. Both PQ postures must answer true and permissive
// false; an unknown string must never read as strict-by-accident, nor as
// permissive-by-accident — it is refused at Resolve, and this pins the
// predicate's answer meanwhile.
func TestIsStrictCoversBothPQPostures(t *testing.T) {
	for p, want := range map[Profile]bool{
		profileStrict:       true,
		profileFIPS:         true,
		profilePermissive:   false,
		Profile(""):         false,
		Profile("nonsense"): false,
	} {
		if got := p.IsStrict(); got != want {
			t.Errorf("Profile(%q).IsStrict() = %v, want %v", p, got, want)
		}
	}
}
