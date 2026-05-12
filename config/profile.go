// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import "fmt"

// Profile is the user-facing chain security profile — a single string
// knob set in chain config (YAML) or env (CONSENSUS_PROFILE). One
// config selects one Profile; the wire byte that gets pinned into the
// cert envelope is derived from the string at config-load time.
//
// Three values across the spectrum, in increasing enforcement
// strictness:
//
//	classical — no enforced refusals. ECDSA / X25519 / BLS permitted
//	            at every layer. Legacy chains pre-PQ migration.
//	hybrid    — BLS + PQ both required at the consensus / finality
//	            layer; classical contract-auth still permitted at the
//	            EVM precompile boundary. Transitional posture for a
//	            chain migrating from classical into strict.
//	strict    — refuses every classical primitive at every layer:
//	            ecrecover, sha256, ripemd, blake2F, alt_bn128,
//	            BLS12-381 pairings, KZG, X25519. Production chains.
//
// The name spectrum is intentionally not "PQ-prefixed" because the
// distinctions cut across more than the post-quantum axis (proof
// systems, hash suites, fallback policy, KEM choice). "Strict" is
// about strictness of crypto-policy enforcement, not solely about PQ.
//
// Operators write the string in config files; protocol layers that
// need the wire byte call WireByte() (or read ProfileID off the
// resolved ChainSecurityProfile struct).
type Profile string

const (
	ProfileClassical Profile = "classical"
	ProfileHybrid    Profile = "hybrid"
	ProfileStrict    Profile = "strict"
)

// AllProfiles is the canonical list, ordered by increasing
// enforcement strictness. Used by config-validator tooling to produce
// "did you mean?" hints when an unknown profile is supplied.
var AllProfiles = []Profile{
	ProfileClassical,
	ProfileHybrid,
	ProfileStrict,
}

// String implements fmt.Stringer.
func (p Profile) String() string { return string(p) }

// IsValid reports whether p is one of the known profile values.
func (p Profile) IsValid() bool { _, err := p.Resolve(); return err == nil }

// IsStrict reports whether this profile enforces strict crypto policy
// — i.e. refuses every classical primitive at every layer. Single
// canonical entry point for "should this chain install AllForbidden()
// at the EVM precompile boundary?".
//
// Strict returns true; Hybrid and Classical return false. Hybrid is
// PQ-positive at the consensus layer but admits classical
// contract-auth, so the EVM gate stays open under Hybrid.
func (p Profile) IsStrict() bool { return p == ProfileStrict }

// Resolve returns the canonical ChainSecurityProfile for this Profile.
// The returned profile passes Validate() by construction; the caller
// receives a fresh pointer copy and may retain it without affecting
// other callers. Canonical immutable values live in profiles.go.
//
// Returns a typed error for unknown / empty profiles so chain-config
// loaders fail loudly at boot.
func (p Profile) Resolve() (*ChainSecurityProfile, error) {
	switch p {
	case ProfileStrict:
		return StrictPQ(), nil
	case ProfileHybrid:
		return Hybrid(), nil
	case ProfileClassical:
		return Classical(), nil
	case "":
		return nil, fmt.Errorf("consensus profile is empty; must be one of: classical, hybrid, strict")
	default:
		return nil, fmt.Errorf("unknown consensus profile %q; must be one of: classical, hybrid, strict", string(p))
	}
}

// MustResolve is the panic-on-error form of Resolve. Used at boot for
// canonical profiles that MUST initialise successfully; never called
// on operator-supplied data.
func (p Profile) MustResolve() *ChainSecurityProfile {
	sp, err := p.Resolve()
	if err != nil {
		panic(fmt.Sprintf("config.Profile.MustResolve(%q): %v", string(p), err))
	}
	return sp
}

// WireByte returns the wire ProfileID byte for cert-envelope encoding.
// Operators do not normally touch this; it is exposed for protocol
// layers that encode the profile into wire transcripts.
//
// Returns 0x00 (ProfileNone) for unknown / invalid profiles; callers
// MUST validate via Resolve() first.
func (p Profile) WireByte() uint8 {
	sp, err := p.Resolve()
	if err != nil {
		return uint8(ProfileNone)
	}
	return uint8(sp.ProfileID)
}

// ProfileFromWireByte is the inverse of WireByte: maps the wire byte
// pinned in a cert envelope back to the user-facing Profile string.
// Returns the empty Profile and an error for unknown bytes.
//
// Wire-byte mapping:
//
//	0x01 → "strict"
//	0x04 → "hybrid"
//	0x05 → "classical"
//
// Bytes 0x02 (legacy Permissive) and 0x03 (legacy FIPS) are not part
// of the user-facing Profile spectrum; callers that pinned those
// bytes can resolve via ProfileByID(ProfileID(b)) at the byte layer.
func ProfileFromWireByte(b uint8) (Profile, error) {
	switch ProfileID(b) {
	case ProfileStrictPQ:
		return ProfileStrict, nil
	}
	if b == 0x04 {
		return ProfileHybrid, nil
	}
	if b == 0x05 {
		return ProfileClassical, nil
	}
	return "", fmt.Errorf("unknown wire ProfileID byte 0x%02x", b)
}
