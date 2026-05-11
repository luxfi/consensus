// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"errors"
	"testing"
)

// TestProfileID_String pins the canonical wire name for every well-known
// profile. A rename here is a wire-breaking change; the test exists so a
// typo in a constant name surfaces in `go test` before it leaks into a
// genesis.
func TestProfileID_String(t *testing.T) {
	cases := []struct {
		id   ProfileID
		want string
	}{
		{ProfileNone, "none"},
		{ProfileStrictPQ, "strict-pq"},
		{ProfilePermissive, "permissive"},
		{ProfileFIPS, "fips"},
	}
	for _, c := range cases {
		if got := c.id.String(); got != c.want {
			t.Errorf("ProfileID(%d).String() = %q, want %q", c.id, got, c.want)
		}
	}
}

// TestProofFormatID_String pins the canonical wire name for every format.
func TestProofFormatID_String(t *testing.T) {
	cases := []struct {
		id   ProofFormatID
		want string
	}{
		{ProofFormatNone, "none"},
		{ProofFormatSTARKFRIBinaryV1, "stark-fri-binary-v1"},
		{ProofFormatSP1BinaryV1, "sp1-binary-v1"},
		{ProofFormatRISC0BinaryV1, "risc0-binary-v1"},
		{ProofFormatP3QBinaryV1, "p3q-binary-v1"},
		{ProofFormatStoneCairoBinaryV1, "stone-cairo-binary-v1"},
		{ProofFormatStwoCircleBinaryV1, "stwo-circle-binary-v1"},
		{ProofFormatGroth16WrappedForbid, "groth16-wrapped-classical-forbidden-in-pq"},
		{ProofFormatKZGWrappedForbid, "kzg-wrapped-classical-forbidden-in-pq"},
	}
	for _, c := range cases {
		if got := c.id.String(); got != c.want {
			t.Errorf("ProofFormatID(0x%02x).String() = %q, want %q", c.id, got, c.want)
		}
	}
}

// TestProofFormatID_IsForbiddenInPQMode pins the two refusal markers.
func TestProofFormatID_IsForbiddenInPQMode(t *testing.T) {
	for _, f := range []ProofFormatID{
		ProofFormatNone,
		ProofFormatSTARKFRIBinaryV1,
		ProofFormatSP1BinaryV1,
		ProofFormatRISC0BinaryV1,
		ProofFormatP3QBinaryV1,
		ProofFormatStoneCairoBinaryV1,
		ProofFormatStwoCircleBinaryV1,
	} {
		if f.IsForbiddenInPQMode() {
			t.Errorf("ProofFormatID(%s).IsForbiddenInPQMode() = true, want false", f.String())
		}
	}
	for _, f := range []ProofFormatID{
		ProofFormatGroth16WrappedForbid,
		ProofFormatKZGWrappedForbid,
	} {
		if !f.IsForbiddenInPQMode() {
			t.Errorf("ProofFormatID(%s).IsForbiddenInPQMode() = false, want true", f.String())
		}
	}
}

// TestVerifierID_String pins the canonical wire name for every verifier.
func TestVerifierID_String(t *testing.T) {
	cases := []struct {
		id   VerifierID
		want string
	}{
		{VerifierNone, "none"},
		{VerifierLuxSTARKFRISHA3PQ, "lux-stark-fri-sha3-pq"},
		{VerifierSP1CompressedSTARKPQ, "sp1-compressed-stark-pq"},
		{VerifierRISC0SuccinctSTARKPQ, "risc0-succinct-stark-pq"},
		{VerifierP3QSTARKFRISHA3PQ, "p3q-stark-fri-sha3-pq"},
		{VerifierStoneCairoSTARKPQ, "stone-cairo-stark-pq"},
		{VerifierStwoCircleSTARKPQ, "stwo-circle-stark-pq"},
	}
	for _, c := range cases {
		if got := c.id.String(); got != c.want {
			t.Errorf("VerifierID(0x%04x).String() = %q, want %q", c.id, got, c.want)
		}
	}
}

// TestStrictPQ_Validate ensures the canonical strict-PQ profile passes
// every internal invariant. Failure here means we shipped an invalid
// production profile — a hard refusal at genesis-load.
func TestStrictPQ_Validate(t *testing.T) {
	p := StrictPQ()
	if err := p.Validate(); err != nil {
		t.Fatalf("StrictPQ().Validate() returned %v; want nil", err)
	}
	if p.ProfileID != uint32(ProfileStrictPQ) {
		t.Errorf("StrictPQ().ProfileID = %d, want %d", p.ProfileID, ProfileStrictPQ)
	}
	if p.ProfileName != ProfileNameStrictPQ {
		t.Errorf("StrictPQ().ProfileName = %q, want %q", p.ProfileName, ProfileNameStrictPQ)
	}
	if p.HashSuiteID != HashSuiteSHA3NIST {
		t.Errorf("StrictPQ().HashSuiteID = %s, want sha3-nist", p.HashSuiteID.String())
	}
	if p.ProofPolicyID != ProofPolicySTARKFRISHA3PQ {
		t.Errorf("StrictPQ().ProofPolicyID = %s, want stark-fri-sha3-pq", p.ProofPolicyID.String())
	}
	if !p.RequireTransparent {
		t.Errorf("StrictPQ().RequireTransparent = false, want true")
	}
	if !p.ForbidPairings || !p.ForbidKZG || !p.ForbidTrustedSetup ||
		!p.ForbidClassicalSNARKs || !p.ForbidDevProofs || !p.ForbidFallbacks {
		t.Errorf("StrictPQ() must forbid every classical primitive AND dev proofs AND fallbacks; got pairings=%v kzg=%v trusted=%v snark=%v dev=%v fallback=%v",
			p.ForbidPairings, p.ForbidKZG, p.ForbidTrustedSetup,
			p.ForbidClassicalSNARKs, p.ForbidDevProofs, p.ForbidFallbacks)
	}
}

// TestPermissive_Validate ensures the testnet profile passes Validate.
// Permissive must NOT set ForbidDevProofs (testnet uses dev backends).
func TestPermissive_Validate(t *testing.T) {
	p := Permissive()
	if err := p.Validate(); err != nil {
		t.Fatalf("Permissive().Validate() returned %v; want nil", err)
	}
	if p.ForbidDevProofs {
		t.Errorf("Permissive() must NOT forbid dev proofs (testnet uses them)")
	}
}

// TestFIPS_Validate ensures the FIPS profile passes and admits only P3Q.
func TestFIPS_Validate(t *testing.T) {
	p := FIPS()
	if err := p.Validate(); err != nil {
		t.Fatalf("FIPS().Validate() returned %v; want nil", err)
	}
	if len(p.AllowedProofBackends) != 1 || p.AllowedProofBackends[0] != ProofBackendP3QSTARKFRISHA3 {
		t.Errorf("FIPS() must allow only P3Q; got %v", p.AllowedProofBackends)
	}
}

// TestForkClassicalCompatUnsafe_Validate ensures the fork profile passes
// AND that its ProfileName names it as fork / unsafe-for-mainnet-marketing.
func TestForkClassicalCompatUnsafe_Validate(t *testing.T) {
	if err := ForkClassicalCompatUnsafeProfile.Validate(); err != nil {
		t.Fatalf("ForkClassicalCompatUnsafeProfile.Validate() returned %v; want nil", err)
	}
	if ForkClassicalCompatUnsafeProfile.ProfileName != "FORK_CLASSICAL_COMPAT_UNSAFE" {
		t.Errorf("ForkClassicalCompatUnsafeProfile.ProfileName = %q, want %q",
			ForkClassicalCompatUnsafeProfile.ProfileName, "FORK_CLASSICAL_COMPAT_UNSAFE")
	}
	if ForkClassicalCompatUnsafeProfile.ProfileID != ForkClassicalCompatUnsafeProfileID {
		t.Errorf("ForkClassicalCompatUnsafeProfile.ProfileID = %d, want %d",
			ForkClassicalCompatUnsafeProfile.ProfileID, ForkClassicalCompatUnsafeProfileID)
	}
	// Critical: the fork uses the keccak-merkle STARK policy so it MUST
	// NOT collide with StrictPQ's policy.
	if ForkClassicalCompatUnsafeProfile.ProofPolicyID == StrictPQProfile.ProofPolicyID {
		t.Errorf("fork profile ProofPolicyID must NOT match StrictPQ — that's the whole point")
	}
}

// =============================================================================
// ChainSecurityProfile.Validate() — invariant tests
// =============================================================================

// TestChainSecurityProfile_Validate_NilReceiver guards the nil-pointer
// invariant; we want a typed error rather than a panic.
func TestChainSecurityProfile_Validate_NilReceiver(t *testing.T) {
	var p *ChainSecurityProfile
	if err := p.Validate(); !errors.Is(err, ErrProfileNil) {
		t.Errorf("Validate(nil) = %v; want ErrProfileNil", err)
	}
}

// TestChainSecurityProfile_Validate_ZeroValue proves the zero value is
// rejected. The doc claims "Zero value = INVALID (never default-secure)"
// and Validate is the gate.
func TestChainSecurityProfile_Validate_ZeroValue(t *testing.T) {
	var p ChainSecurityProfile
	if err := p.Validate(); err == nil {
		t.Errorf("ChainSecurityProfile{}.Validate() returned nil; zero value MUST be rejected")
	}
}

// TestChainSecurityProfile_Validate_RejectsForbiddenPolicy proves the
// profile validator refuses a profile that pins a forbidden classical
// policy.
func TestChainSecurityProfile_Validate_RejectsForbiddenPolicy(t *testing.T) {
	p := StrictPQ()
	p.ProofPolicyID = ProofPolicyGroth16BN254Forbid
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with Groth16 policy returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_RejectsForbiddenBackend proves the
// validator refuses a profile that lists a forbidden backend.
func TestChainSecurityProfile_Validate_RejectsForbiddenBackend(t *testing.T) {
	p := StrictPQ()
	p.AllowedProofBackends = append(append([]ProofBackendID(nil), p.AllowedProofBackends...),
		ProofBackendGroth16WrapForbid)
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with Groth16 backend returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_RejectsForbiddenFormat mirrors the
// backend check on the format axis.
func TestChainSecurityProfile_Validate_RejectsForbiddenFormat(t *testing.T) {
	p := StrictPQ()
	p.AllowedProofFormats = append(append([]ProofFormatID(nil), p.AllowedProofFormats...),
		ProofFormatGroth16WrappedForbid)
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with Groth16 format returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_RejectsEmptyAllowlist proves an empty
// backend allowlist is rejected.
func TestChainSecurityProfile_Validate_RejectsEmptyAllowlist(t *testing.T) {
	p := StrictPQ()
	p.AllowedProofBackends = nil
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldUnset) {
		t.Errorf("Validate() with empty AllowedProofBackends returned %v; want ErrProfileFieldUnset", err)
	}
}

// TestChainSecurityProfile_Validate_RejectsNoForbidFlags proves a profile
// with every Forbid* flag false is rejected — silently accepting weak
// primitives is the operator footgun.
func TestChainSecurityProfile_Validate_RejectsNoForbidFlags(t *testing.T) {
	p := StrictPQ()
	p.ForbidPairings = false
	p.ForbidKZG = false
	p.ForbidTrustedSetup = false
	p.ForbidClassicalSNARKs = false
	p.ForbidDevProofs = false
	p.ForbidFallbacks = false
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with all-false Forbid bits returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_RejectsM44HighValue proves the
// devnet-only Pulsar-M-44 cannot be the high-value scheme.
func TestChainSecurityProfile_Validate_RejectsM44HighValue(t *testing.T) {
	p := StrictPQ()
	p.HighValueSchemeID = SigSchemePulsarM44
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with HighValueSchemeID=Pulsar-M-44 returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_RejectsRawMLDSAFinality proves raw
// FIPS 204 ML-DSA cannot be the finality scheme — finality is threshold.
func TestChainSecurityProfile_Validate_RejectsRawMLDSAFinality(t *testing.T) {
	p := StrictPQ()
	p.FinalitySchemeID = SigSchemeMLDSA65
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with raw ML-DSA-65 finality returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_RejectsThresholdIdentity proves
// Pulsar-M threshold cannot be the identity scheme — identity is
// single-party.
func TestChainSecurityProfile_Validate_RejectsThresholdIdentity(t *testing.T) {
	p := StrictPQ()
	p.IdentitySchemeID = SigSchemePulsarM65
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with Pulsar-M-65 identity returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_RejectsLowSoundness proves a soundness
// floor below 128 bits is rejected.
func TestChainSecurityProfile_Validate_RejectsLowSoundness(t *testing.T) {
	p := StrictPQ()
	p.MinSoundnessBits = 127
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with MinSoundnessBits=127 returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_RejectsLowHashWidth proves a hash
// output width below 256 bits is rejected.
func TestChainSecurityProfile_Validate_RejectsLowHashWidth(t *testing.T) {
	p := StrictPQ()
	p.MinHashOutputBits = 255
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with MinHashOutputBits=255 returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_RejectsHashSuiteNone proves a locked
// profile cannot carry HashSuiteNone.
func TestChainSecurityProfile_Validate_RejectsHashSuiteNone(t *testing.T) {
	p := StrictPQ()
	p.HashSuiteID = HashSuiteNone
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldUnset) {
		t.Errorf("Validate() with HashSuiteNone returned %v; want ErrProfileFieldUnset", err)
	}
}

// TestChainSecurityProfile_Validate_RejectsEmptyProfileName guards the
// ProfileName invariant.
func TestChainSecurityProfile_Validate_RejectsEmptyProfileName(t *testing.T) {
	p := StrictPQ()
	p.ProfileName = ""
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldUnset) {
		t.Errorf("Validate() with empty ProfileName returned %v; want ErrProfileFieldUnset", err)
	}
}

// =============================================================================
// AllowsBackend / AllowsFormat
// =============================================================================

// TestChainSecurityProfile_AllowsBackend pins the allow-list semantics
// across the canonical profile.
func TestChainSecurityProfile_AllowsBackend(t *testing.T) {
	p := StrictPQ()
	for _, b := range p.AllowedProofBackends {
		if !p.AllowsBackend(b) {
			t.Errorf("StrictPQ.AllowsBackend(%s) = false, want true", b.String())
		}
	}
	if p.AllowsBackend(ProofBackendGroth16WrapForbid) {
		t.Errorf("StrictPQ.AllowsBackend(groth16-wrap) = true, want false")
	}
	if p.AllowsBackend(ProofBackendNone) {
		t.Errorf("StrictPQ.AllowsBackend(None) = true, want false")
	}
	if p.AllowsBackend(ProofBackendRISC0RawSTARKDev) {
		t.Errorf("StrictPQ.AllowsBackend(dev) = true; ForbidDevProofs must exclude")
	}
}

// TestChainSecurityProfile_AllowsFormat pins the format allow-list.
func TestChainSecurityProfile_AllowsFormat(t *testing.T) {
	p := StrictPQ()
	for _, f := range p.AllowedProofFormats {
		if !p.AllowsFormat(f) {
			t.Errorf("StrictPQ.AllowsFormat(%s) = false, want true", f.String())
		}
	}
	if p.AllowsFormat(ProofFormatKZGWrappedForbid) {
		t.Errorf("StrictPQ.AllowsFormat(kzg-wrapped) = true, want false")
	}
	if p.AllowsFormat(ProofFormatNone) {
		t.Errorf("StrictPQ.AllowsFormat(None) = true, want false")
	}
}

// =============================================================================
// ComputeHash
// =============================================================================

// TestChainSecurityProfile_ComputeHash_Determinism proves the hash is a
// function of the profile contents: equal profiles hash to equal bytes.
func TestChainSecurityProfile_ComputeHash_Determinism(t *testing.T) {
	a := StrictPQ()
	b := StrictPQ()
	ah, err := a.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash(a): %v", err)
	}
	bh, err := b.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash(b): %v", err)
	}
	if ah != bh {
		t.Errorf("StrictPQ() hashes inconsistently across calls: %x vs %x", ah, bh)
	}
	// And the well-known constant matches.
	if ah != StrictPQProfile.ProfileHash {
		t.Errorf("StrictPQ ComputeHash() differs from init-pinned ProfileHash: got %x want %x",
			ah, StrictPQProfile.ProfileHash)
	}
}

// TestChainSecurityProfile_ComputeHash_NonZero proves the canonical
// profile produces a non-zero hash. A zero hash indicates the
// init-pinning logic ran but the actual digest computation broke.
func TestChainSecurityProfile_ComputeHash_NonZero(t *testing.T) {
	h, err := StrictPQ().ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}
	var zero [48]byte
	if h == zero {
		t.Errorf("StrictPQ ComputeHash() returned zero — digest function broken")
	}
}

// TestChainSecurityProfile_ComputeHash_DistinguishesProfiles proves each
// canonical profile maps to a distinct hash.
func TestChainSecurityProfile_ComputeHash_DistinguishesProfiles(t *testing.T) {
	hs, err := StrictPQ().ComputeHash()
	if err != nil {
		t.Fatalf("strict: %v", err)
	}
	hp, err := Permissive().ComputeHash()
	if err != nil {
		t.Fatalf("permissive: %v", err)
	}
	hf, err := FIPS().ComputeHash()
	if err != nil {
		t.Fatalf("fips: %v", err)
	}
	hfork, err := ForkClassicalCompatUnsafeProfile.ComputeHash()
	if err != nil {
		t.Fatalf("fork: %v", err)
	}

	all := []struct {
		name string
		h    [48]byte
	}{
		{"StrictPQ", hs},
		{"Permissive", hp},
		{"FIPS", hf},
		{"ForkClassicalCompatUnsafe", hfork},
	}
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[i].h == all[j].h {
				t.Errorf("%s and %s collide: %x", all[i].name, all[j].name, all[i].h)
			}
		}
	}
}

// TestChainSecurityProfile_ComputeHash_ListOrderInvariant proves that
// rearranging AllowedProofBackends / AllowedProofFormats does not change
// the profile hash. Genesis-pinning must not depend on listing order.
func TestChainSecurityProfile_ComputeHash_ListOrderInvariant(t *testing.T) {
	a := StrictPQ()
	b := StrictPQ()
	// Reverse the allow-lists in b. Length matches; ComputeHash sorts.
	rev := func(bs []ProofBackendID) []ProofBackendID {
		out := append([]ProofBackendID(nil), bs...)
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
		return out
	}
	revF := func(fs []ProofFormatID) []ProofFormatID {
		out := append([]ProofFormatID(nil), fs...)
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
		return out
	}
	b.AllowedProofBackends = rev(b.AllowedProofBackends)
	b.AllowedProofFormats = revF(b.AllowedProofFormats)

	ah, err := a.ComputeHash()
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	bh, err := b.ComputeHash()
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if ah != bh {
		t.Errorf("ComputeHash is order-dependent; AllowedProofBackends/Formats must be sorted before hashing")
	}
}

// TestChainSecurityProfile_ComputeHash_DiffersByField proves every
// hash-bound field actually changes the hash. The test is the
// last-line-of-defence against a future linter / refactor accidentally
// dropping a field from ComputeHash.
func TestChainSecurityProfile_ComputeHash_DiffersByField(t *testing.T) {
	baseHash, err := StrictPQ().ComputeHash()
	if err != nil {
		t.Fatalf("base: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(p *ChainSecurityProfile)
	}{
		{"ProfileID", func(p *ChainSecurityProfile) { p.ProfileID = uint32(ProfilePermissive) }},
		{"ProfileName", func(p *ChainSecurityProfile) { p.ProfileName = "OTHER" }},
		{"HashSuiteID", func(p *ChainSecurityProfile) { p.HashSuiteID = HashSuiteBLAKE3Legacy }},
		{"IdentitySchemeID", func(p *ChainSecurityProfile) { p.IdentitySchemeID = SigSchemeMLDSA87 }},
		{"FinalitySchemeID", func(p *ChainSecurityProfile) { p.FinalitySchemeID = SigSchemePulsarM87 }},
		{"HighValueSchemeID", func(p *ChainSecurityProfile) { p.HighValueSchemeID = SigSchemePulsarM65 }},
		// Mutating ProofPolicyID to STARK_FRI_Keccak keeps Validate happy
		// (still PQ-positive, still not forbidden) and changes the hash.
		{"ProofPolicyID", func(p *ChainSecurityProfile) { p.ProofPolicyID = ProofPolicySTARKFRIKeccak }},
		{"MinSoundnessBits", func(p *ChainSecurityProfile) { p.MinSoundnessBits = 192 }},
		{"MinHashOutputBits", func(p *ChainSecurityProfile) { p.MinHashOutputBits = 512 }},
		// Each Forbid* flip must change the hash. None of these flips on
		// their own makes Validate fail (the "at least one forbid flag"
		// rule is satisfied by the remaining true flags).
		{"ForbidPairings", func(p *ChainSecurityProfile) { p.ForbidPairings = false }},
		{"ForbidKZG", func(p *ChainSecurityProfile) { p.ForbidKZG = false }},
		{"ForbidTrustedSetup", func(p *ChainSecurityProfile) { p.ForbidTrustedSetup = false }},
		{"ForbidClassicalSNARKs", func(p *ChainSecurityProfile) { p.ForbidClassicalSNARKs = false }},
		{"ForbidDevProofs", func(p *ChainSecurityProfile) { p.ForbidDevProofs = false }},
		{"ForbidFallbacks", func(p *ChainSecurityProfile) { p.ForbidFallbacks = false }},
		// RequireTransparent is always-true on the canonical profile;
		// flipping false MUST change the hash.
		{"RequireTransparent", func(p *ChainSecurityProfile) { p.RequireTransparent = false }},
		// Drop one element from the allow-lists; canonicalisation sorts,
		// so a length change MUST change the hash.
		{"AllowedProofBackends drop", func(p *ChainSecurityProfile) {
			p.AllowedProofBackends = p.AllowedProofBackends[:len(p.AllowedProofBackends)-1]
		}},
		{"AllowedProofFormats drop", func(p *ChainSecurityProfile) {
			p.AllowedProofFormats = p.AllowedProofFormats[:len(p.AllowedProofFormats)-1]
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mutated := StrictPQ()
			c.mutate(mutated)
			got, err := mutated.ComputeHash()
			if err != nil {
				t.Fatalf("ComputeHash(mutated %s): %v", c.name, err)
			}
			if got == baseHash {
				t.Errorf("mutating %s did not change ComputeHash output", c.name)
			}
		})
	}
}

// TestChainSecurityProfile_ComputeHash_RefusesInvalidProfile proves
// ComputeHash refuses to digest a malformed profile — operators cannot
// learn the hash of a partly initialised profile by trial-and-error.
func TestChainSecurityProfile_ComputeHash_RefusesInvalidProfile(t *testing.T) {
	var p ChainSecurityProfile
	if _, err := p.ComputeHash(); err == nil {
		t.Errorf("ComputeHash on zero-value profile returned nil error; want validation failure")
	}
}

// TestStrictPQProfile_NoOperatorFootguns walks every field of the
// canonical strict-PQ profile and asserts it carries a definite,
// audit-defensible value. The test is a tripwire against future
// refactors that introduce zero-default branches into a "secure" profile.
func TestStrictPQProfile_NoOperatorFootguns(t *testing.T) {
	p := StrictPQProfile
	if p.ProfileID == 0 {
		t.Errorf("ProfileID is zero")
	}
	if p.ProfileName == "" {
		t.Errorf("ProfileName is empty")
	}
	var zeroHash [48]byte
	if p.ProfileHash == zeroHash {
		t.Errorf("ProfileHash is zero — init() did not pin it")
	}
	if p.HashSuiteID != HashSuiteSHA3NIST {
		t.Errorf("HashSuiteID = %s; StrictPQ MUST be NIST-aligned", p.HashSuiteID.String())
	}
	if p.IdentitySchemeID != SigSchemeMLDSA65 {
		t.Errorf("IdentitySchemeID = %s; StrictPQ pins ML-DSA-65", p.IdentitySchemeID.String())
	}
	if p.FinalitySchemeID != SigSchemePulsarM65 {
		t.Errorf("FinalitySchemeID = %s; StrictPQ pins Pulsar-M-65", p.FinalitySchemeID.String())
	}
	if p.HighValueSchemeID != SigSchemePulsarM87 {
		t.Errorf("HighValueSchemeID = %s; StrictPQ pins Pulsar-M-87 for governance", p.HighValueSchemeID.String())
	}
	if p.ProofPolicyID != ProofPolicySTARKFRISHA3PQ {
		t.Errorf("ProofPolicyID = %s; StrictPQ pins STARK_FRI_SHA3_PQ", p.ProofPolicyID.String())
	}
	if p.MinSoundnessBits < 128 {
		t.Errorf("MinSoundnessBits=%d; StrictPQ MUST pin ≥ 128", p.MinSoundnessBits)
	}
	if p.MinHashOutputBits < 384 {
		t.Errorf("MinHashOutputBits=%d; StrictPQ MUST pin ≥ 384 (SHA3-384)", p.MinHashOutputBits)
	}
	if !p.RequireTransparent {
		t.Errorf("RequireTransparent=false; StrictPQ MUST demand transparent setup")
	}
	if !p.ForbidPairings {
		t.Errorf("ForbidPairings=false; StrictPQ MUST forbid EC pairings")
	}
	if !p.ForbidKZG {
		t.Errorf("ForbidKZG=false; StrictPQ MUST forbid KZG commitments")
	}
	if !p.ForbidTrustedSetup {
		t.Errorf("ForbidTrustedSetup=false; StrictPQ MUST forbid trusted setups")
	}
	if !p.ForbidClassicalSNARKs {
		t.Errorf("ForbidClassicalSNARKs=false; StrictPQ MUST forbid Groth16/PLONK wrappers")
	}
	if !p.ForbidDevProofs {
		t.Errorf("ForbidDevProofs=false; StrictPQ MUST forbid dev backends in production")
	}
	if !p.ForbidFallbacks {
		t.Errorf("ForbidFallbacks=false; StrictPQ MUST refuse silent downgrade")
	}
	// Every backend in the allowlist MUST be production-PQ.
	for _, b := range p.AllowedProofBackends {
		if !b.IsProductionPQ() {
			t.Errorf("AllowedProofBackends contains non-production %s", b.String())
		}
		if b.IsForbiddenInPQMode() {
			t.Errorf("AllowedProofBackends contains forbidden %s", b.String())
		}
	}
	if len(p.AllowedProofBackends) < 1 {
		t.Errorf("AllowedProofBackends is empty")
	}
	// Format allowlist MUST be non-empty and free of forbidden entries.
	for _, f := range p.AllowedProofFormats {
		if f == ProofFormatNone {
			t.Errorf("AllowedProofFormats contains None")
		}
		if f.IsForbiddenInPQMode() {
			t.Errorf("AllowedProofFormats contains forbidden %s", f.String())
		}
	}
	if len(p.AllowedProofFormats) < 1 {
		t.Errorf("AllowedProofFormats is empty")
	}
}

// =============================================================================
// Cross-network strict-PQ profiles (Zoo, Hanzo)
// =============================================================================
//
// LP-168..179, ZIP-0809..0820, HIP-0085..0104 cite ProfileZooStrictPQ=0x04
// and ProfileHanzoStrictPQ=0x05. These profiles MUST be byte-identical to
// StrictPQ on every security-relevant field; ProfileID and ProfileName
// are the only divergences allowed. Cross-network coherence holds iff the
// three profiles share their forbid / scheme / floor surface.

// TestProfileByID_ZooStrictPQ proves the canonical lookup returns the Zoo
// profile, that it validates, and that ProfileID/ProfileName are the
// expected values.
func TestProfileByID_ZooStrictPQ(t *testing.T) {
	p, err := ProfileByID(ProfileZooStrictPQ)
	if err != nil {
		t.Fatalf("ProfileByID(ProfileZooStrictPQ) returned %v; want nil", err)
	}
	if p == nil {
		t.Fatalf("ProfileByID(ProfileZooStrictPQ) returned nil profile")
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("ZooStrictPQ.Validate() returned %v; want nil", err)
	}
	if p.ProfileID != uint32(ProfileZooStrictPQ) {
		t.Errorf("ZooStrictPQ.ProfileID = %d, want %d", p.ProfileID, ProfileZooStrictPQ)
	}
	if p.ProfileName != ProfileNameZooStrictPQ {
		t.Errorf("ZooStrictPQ.ProfileName = %q, want %q", p.ProfileName, ProfileNameZooStrictPQ)
	}
}

// TestProfileByID_HanzoStrictPQ mirrors the Zoo test for the Hanzo profile.
func TestProfileByID_HanzoStrictPQ(t *testing.T) {
	p, err := ProfileByID(ProfileHanzoStrictPQ)
	if err != nil {
		t.Fatalf("ProfileByID(ProfileHanzoStrictPQ) returned %v; want nil", err)
	}
	if p == nil {
		t.Fatalf("ProfileByID(ProfileHanzoStrictPQ) returned nil profile")
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("HanzoStrictPQ.Validate() returned %v; want nil", err)
	}
	if p.ProfileID != uint32(ProfileHanzoStrictPQ) {
		t.Errorf("HanzoStrictPQ.ProfileID = %d, want %d", p.ProfileID, ProfileHanzoStrictPQ)
	}
	if p.ProfileName != ProfileNameHanzoStrictPQ {
		t.Errorf("HanzoStrictPQ.ProfileName = %q, want %q", p.ProfileName, ProfileNameHanzoStrictPQ)
	}
}

// TestProfileByID_StrictPQ pins the canonical PQ profile lookup. PQ
// mode is binary; ProfileStrictPQ (0x01, "STRICT_PQ") is the single
// canonical strict-PQ profile every Lux chain pins at genesis.
func TestProfileByID_StrictPQ(t *testing.T) {
	p, err := ProfileByID(ProfileStrictPQ)
	if err != nil {
		t.Fatalf("ProfileByID(ProfileStrictPQ) returned %v; want nil", err)
	}
	if p == nil {
		t.Fatalf("ProfileByID(ProfileStrictPQ) returned nil profile")
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("StrictPQ.Validate() returned %v; want nil", err)
	}
	if p.ProfileID != uint32(ProfileStrictPQ) {
		t.Errorf("StrictPQ.ProfileID = %d, want %d", p.ProfileID, ProfileStrictPQ)
	}
	if p.ProfileName != ProfileNameStrictPQ {
		t.Errorf("StrictPQ.ProfileName = %q, want %q", p.ProfileName, ProfileNameStrictPQ)
	}
}

// TestIsPQ_StrictProfilesReturnTrue pins the IsPQ contract for the
// canonical strict-PQ profile family. PQ mode is binary; every entry
// here is a chain on the strict-PQ envelope.
func TestIsPQ_StrictProfilesReturnTrue(t *testing.T) {
	cases := []struct {
		name string
		p    *ChainSecurityProfile
	}{
		{"strict-pq", StrictPQ()},
		{"fips", FIPS()},
		{"zoo-strict", ZooStrictPQ()},
		{"hanzo-strict", HanzoStrictPQ()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !c.p.IsPQ() {
				t.Fatalf("%s.IsPQ() = false; want true", c.name)
			}
		})
	}
}

// TestIsPQ_NonStrictReturnsFalse covers nil, permissive, and unknown
// profiles — the negative side of the IsPQ contract.
func TestIsPQ_NonStrictReturnsFalse(t *testing.T) {
	var nilProfile *ChainSecurityProfile
	if nilProfile.IsPQ() {
		t.Fatalf("nil profile IsPQ() must be false")
	}
	if Permissive().IsPQ() {
		t.Fatalf("Permissive IsPQ() must be false")
	}
	unknown := &ChainSecurityProfile{ProfileID: 0xFE}
	if unknown.IsPQ() {
		t.Fatalf("unknown ProfileID 0xFE IsPQ() must be false")
	}
}

// TestStrictPQProfiles_AllFieldsByteIdentical proves canonical strict-PQ,
// Zoo strict-PQ, and Hanzo strict-PQ profiles agree byte-for-byte on every
// field except ProfileID, ProfileName, and ProfileHash. This is the
// cross-network coherence invariant: a Zoo / Hanzo deployment cannot
// accidentally weaken the strict-PQ posture relative to the canonical
// strict-PQ profile.
//
// The check compares every primitive / slice / boolean field explicitly
// rather than reflect.DeepEqual on the whole struct so a failure names
// exactly which field diverged.
func TestStrictPQProfiles_AllFieldsByteIdentical(t *testing.T) {
	canonical := StrictPQ()
	zoo := ZooStrictPQ()
	hanzo := HanzoStrictPQ()
	lux := canonical // local alias kept short for the symmetric drift checks below

	cases := []struct {
		name  string
		other *ChainSecurityProfile
		// wantID/wantName are the only fields allowed to differ.
		wantID   uint32
		wantName string
	}{
		{"zoo", zoo, uint32(ProfileZooStrictPQ), ProfileNameZooStrictPQ},
		{"hanzo", hanzo, uint32(ProfileHanzoStrictPQ), ProfileNameHanzoStrictPQ},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.other.ProfileID != c.wantID {
				t.Errorf("%s.ProfileID = %d, want %d", c.name, c.other.ProfileID, c.wantID)
			}
			if c.other.ProfileName != c.wantName {
				t.Errorf("%s.ProfileName = %q, want %q", c.name, c.other.ProfileName, c.wantName)
			}
			// Every other field MUST equal Lux's value.
			if c.other.HashSuiteID != lux.HashSuiteID {
				t.Errorf("%s.HashSuiteID drift: got %s, want %s", c.name, c.other.HashSuiteID, lux.HashSuiteID)
			}
			if c.other.IdentitySchemeID != lux.IdentitySchemeID {
				t.Errorf("%s.IdentitySchemeID drift: got %s, want %s", c.name, c.other.IdentitySchemeID, lux.IdentitySchemeID)
			}
			if c.other.FinalitySchemeID != lux.FinalitySchemeID {
				t.Errorf("%s.FinalitySchemeID drift: got %s, want %s", c.name, c.other.FinalitySchemeID, lux.FinalitySchemeID)
			}
			if c.other.HighValueSchemeID != lux.HighValueSchemeID {
				t.Errorf("%s.HighValueSchemeID drift: got %s, want %s", c.name, c.other.HighValueSchemeID, lux.HighValueSchemeID)
			}
			if c.other.ProofPolicyID != lux.ProofPolicyID {
				t.Errorf("%s.ProofPolicyID drift: got %s, want %s", c.name, c.other.ProofPolicyID, lux.ProofPolicyID)
			}
			if len(c.other.AllowedProofBackends) != len(lux.AllowedProofBackends) {
				t.Errorf("%s.AllowedProofBackends length drift: got %d, want %d",
					c.name, len(c.other.AllowedProofBackends), len(lux.AllowedProofBackends))
			} else {
				for i := range lux.AllowedProofBackends {
					if c.other.AllowedProofBackends[i] != lux.AllowedProofBackends[i] {
						t.Errorf("%s.AllowedProofBackends[%d] drift: got %s, want %s",
							c.name, i, c.other.AllowedProofBackends[i], lux.AllowedProofBackends[i])
					}
				}
			}
			if len(c.other.AllowedProofFormats) != len(lux.AllowedProofFormats) {
				t.Errorf("%s.AllowedProofFormats length drift: got %d, want %d",
					c.name, len(c.other.AllowedProofFormats), len(lux.AllowedProofFormats))
			} else {
				for i := range lux.AllowedProofFormats {
					if c.other.AllowedProofFormats[i] != lux.AllowedProofFormats[i] {
						t.Errorf("%s.AllowedProofFormats[%d] drift: got %s, want %s",
							c.name, i, c.other.AllowedProofFormats[i], lux.AllowedProofFormats[i])
					}
				}
			}
			if c.other.MinSoundnessBits != lux.MinSoundnessBits {
				t.Errorf("%s.MinSoundnessBits drift: got %d, want %d", c.name, c.other.MinSoundnessBits, lux.MinSoundnessBits)
			}
			if c.other.MinHashOutputBits != lux.MinHashOutputBits {
				t.Errorf("%s.MinHashOutputBits drift: got %d, want %d", c.name, c.other.MinHashOutputBits, lux.MinHashOutputBits)
			}
			if c.other.RequireTransparent != lux.RequireTransparent {
				t.Errorf("%s.RequireTransparent drift: got %v, want %v", c.name, c.other.RequireTransparent, lux.RequireTransparent)
			}
			if c.other.ForbidPairings != lux.ForbidPairings {
				t.Errorf("%s.ForbidPairings drift: got %v, want %v", c.name, c.other.ForbidPairings, lux.ForbidPairings)
			}
			if c.other.ForbidKZG != lux.ForbidKZG {
				t.Errorf("%s.ForbidKZG drift: got %v, want %v", c.name, c.other.ForbidKZG, lux.ForbidKZG)
			}
			if c.other.ForbidTrustedSetup != lux.ForbidTrustedSetup {
				t.Errorf("%s.ForbidTrustedSetup drift: got %v, want %v", c.name, c.other.ForbidTrustedSetup, lux.ForbidTrustedSetup)
			}
			if c.other.ForbidClassicalSNARKs != lux.ForbidClassicalSNARKs {
				t.Errorf("%s.ForbidClassicalSNARKs drift: got %v, want %v", c.name, c.other.ForbidClassicalSNARKs, lux.ForbidClassicalSNARKs)
			}
			if c.other.ForbidDevProofs != lux.ForbidDevProofs {
				t.Errorf("%s.ForbidDevProofs drift: got %v, want %v", c.name, c.other.ForbidDevProofs, lux.ForbidDevProofs)
			}
			if c.other.ForbidFallbacks != lux.ForbidFallbacks {
				t.Errorf("%s.ForbidFallbacks drift: got %v, want %v", c.name, c.other.ForbidFallbacks, lux.ForbidFallbacks)
			}
			if c.other.WalletSchemeID != lux.WalletSchemeID {
				t.Errorf("%s.WalletSchemeID drift: got %s, want %s", c.name, c.other.WalletSchemeID, lux.WalletSchemeID)
			}
			if c.other.TxSchemeID != lux.TxSchemeID {
				t.Errorf("%s.TxSchemeID drift: got %s, want %s", c.name, c.other.TxSchemeID, lux.TxSchemeID)
			}
			if c.other.ContractAuthID != lux.ContractAuthID {
				t.Errorf("%s.ContractAuthID drift: got %s, want %s", c.name, c.other.ContractAuthID, lux.ContractAuthID)
			}
			if c.other.KeyExchangeID != lux.KeyExchangeID {
				t.Errorf("%s.KeyExchangeID drift: got %s, want %s", c.name, c.other.KeyExchangeID, lux.KeyExchangeID)
			}
			if c.other.HighValueKEM != lux.HighValueKEM {
				t.Errorf("%s.HighValueKEM drift: got %s, want %s", c.name, c.other.HighValueKEM, lux.HighValueKEM)
			}
			if c.other.RecoverySchemeID != lux.RecoverySchemeID {
				t.Errorf("%s.RecoverySchemeID drift: got %s, want %s", c.name, c.other.RecoverySchemeID, lux.RecoverySchemeID)
			}
			if c.other.ForbidECDSAWallets != lux.ForbidECDSAWallets {
				t.Errorf("%s.ForbidECDSAWallets drift: got %v, want %v", c.name, c.other.ForbidECDSAWallets, lux.ForbidECDSAWallets)
			}
			if c.other.ForbidECDSAContractAuth != lux.ForbidECDSAContractAuth {
				t.Errorf("%s.ForbidECDSAContractAuth drift: got %v, want %v", c.name, c.other.ForbidECDSAContractAuth, lux.ForbidECDSAContractAuth)
			}
			if c.other.ForbidBLSContractAuth != lux.ForbidBLSContractAuth {
				t.Errorf("%s.ForbidBLSContractAuth drift: got %v, want %v", c.name, c.other.ForbidBLSContractAuth, lux.ForbidBLSContractAuth)
			}
			if c.other.ForbidClassicalKEM != lux.ForbidClassicalKEM {
				t.Errorf("%s.ForbidClassicalKEM drift: got %v, want %v", c.name, c.other.ForbidClassicalKEM, lux.ForbidClassicalKEM)
			}
			if c.other.RequireTypedTxAuth != lux.RequireTypedTxAuth {
				t.Errorf("%s.RequireTypedTxAuth drift: got %v, want %v", c.name, c.other.RequireTypedTxAuth, lux.RequireTypedTxAuth)
			}
		})
	}
}

// TestStrictPQProfiles_ProfileHashDiverges proves the canonical hash of
// the three strict-PQ profiles is distinct — even though every other
// field is byte-identical, the ProfileID+ProfileName binding produces
// three distinct 48-byte commitments. A genesis pinned to StrictPQ
// rejects a cert envelope tagged with the Zoo profile hash and vice
// versa; this is what closes cross-network replay.
func TestStrictPQProfiles_ProfileHashDiverges(t *testing.T) {
	strict := StrictPQ()
	zoo := ZooStrictPQ()
	hanzo := HanzoStrictPQ()
	if strict.ProfileHash == zoo.ProfileHash {
		t.Errorf("StrictPQ.ProfileHash == ZooStrictPQ.ProfileHash; cross-network binding broken")
	}
	if strict.ProfileHash == hanzo.ProfileHash {
		t.Errorf("StrictPQ.ProfileHash == HanzoStrictPQ.ProfileHash; cross-network binding broken")
	}
	if zoo.ProfileHash == hanzo.ProfileHash {
		t.Errorf("ZooStrictPQ.ProfileHash == HanzoStrictPQ.ProfileHash; cross-network binding broken")
	}
	// Every hash must be non-zero (init() ran).
	var zero [48]byte
	for _, c := range []struct {
		name string
		h    [48]byte
	}{
		{"strict", strict.ProfileHash},
		{"zoo", zoo.ProfileHash},
		{"hanzo", hanzo.ProfileHash},
	} {
		if c.h == zero {
			t.Errorf("%s.ProfileHash is zero — init() did not run", c.name)
		}
	}
}

// TestProfileID_String_CrossNetwork pins the wire-name strings for the
// two new cross-network profiles.
func TestProfileID_String_CrossNetwork(t *testing.T) {
	cases := []struct {
		id   ProfileID
		want string
	}{
		{ProfileZooStrictPQ, "zoo-strict-pq"},
		{ProfileHanzoStrictPQ, "hanzo-strict-pq"},
	}
	for _, c := range cases {
		if got := c.id.String(); got != c.want {
			t.Errorf("ProfileID(%d).String() = %q, want %q", c.id, got, c.want)
		}
	}
}
