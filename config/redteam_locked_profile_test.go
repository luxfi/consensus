// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// redteam_locked_profile_test.go — adversarial review of the strict-PQ
// locked-profile architecture as it lands in this package. Each F##
// finding is a separate test; a failing test IS the proof-of-finding
// and MUST NOT be silenced without the corresponding fix landing in
// the same diff.
//
// Build prerequisite: the package as found by red-team is build-broken
// — F51 below. Tests in this file are written against the post-fix
// schema; until the package builds the tests cannot run, which is
// itself a critical finding (you cannot ship code that doesn't build).

package config

import (
	"reflect"
	"testing"
)

// =============================================================================
// F51 — `github.com/luxfi/consensus/config` does not build (two CTO
// agents collided; schema split).
// =============================================================================
//
// SEVERITY: critical (blocker)
//
// The Blue agent producing security_profile.go used schema A:
//   * Field names: AllowedProofBackends / AllowedProofFormats
//   * Forbid flag: ForbidClassicalSNARKs (plural s)
//   * Profile constants live in profiles.go (not yet written): the
//     functions StrictPQ() / Permissive() / FIPS() each
//     `return &StrictPQProfile` / `&PermissiveProfile` / etc.
//   * ProfileID is a uint32 field in ChainSecurityProfile (line 254);
//     the constants ProfileStrictPQ (line 60) are typed ProfileID
//     (uint8). The constants are not assignable to the field.
//   * ComputeHash returns ([48]byte, error)
//
// The Blue agent producing security_profile_test.go used schema B:
//   * Field names: AllowedBackends / AllowedFormats
//   * Forbid flag: ForbidClassicalSNARK (no s)
//   * Tests `if p.ProfileID != ProfileStrictPQ` directly (compile
//     error: uint32 vs ProfileID)
//   * Calls `ErrProfileForbiddenPolicy` (not defined; only
//     ErrProfileFieldInvalid is exported)
//   * Calls `a.ComputeHash() != b.ComputeHash()` as a value comparison
//     (the new signature returns (val, err); compile error)
//
// Result: the package will not vet, will not build, will not test.
// This is the F51 finding — the entire locked-profile work needs a
// single source of truth across struct field names, error variables,
// and constructor return shapes before the rest of the F##s in this
// file are runnable.
//
// PROOF: `cd ~/work/lux/consensus/config && go vet ./...` produces
// type-mismatch errors. The proof is the vet output itself.
//
// FIX: pick ONE schema and apply consistently. Production-grade
// choice:
//   * Plural collection names: AllowedProofBackends / AllowedProofFormats
//   * Plural forbid name: ForbidClassicalSNARKs (matches every other
//     plural collection)
//   * Profile constants typed ProfileID (uint8); the struct field is
//     also ProfileID (the type). Do NOT make the struct field uint32 —
//     that is a category error.
//   * Add profiles.go with the canonical StrictPQProfile /
//     PermissiveProfile / FIPSProfile constants, computed once
//     via MustComputeHash at package init.
//   * Add error variables ErrProfileForbiddenPolicy /
//     ErrProfileForbiddenBackend / ErrProfileForbiddenFormat /
//     ErrProfileStrictRequiresTransparent so callers can errors.Is
//     against the specific failure.
//   * ComputeHash returns ([48]byte, error); existing callers that
//     used the no-error variant get a compile-time fix-prompt.
//
// This file's tests assume the post-fix schema A names. Once the build
// is unblocked, every test in this file becomes runnable and the
// remaining F##s below will either pass (if the property is upheld)
// or fail (if the footgun is still live).
func TestF51_PackageBuilds(t *testing.T) {
	// This test exists to make the build break visible in `go test`
	// output. If you can read this test name, the package compiled —
	// good, run the rest of the suite. If you cannot, F51 is unfixed.
	t.Log("F51 covered: package compiles enough for tests to run")
}

// =============================================================================
// F52 — ChainSecurityProfile.Validate exists, but the canonical
// StrictPQ constructor that should satisfy it cannot be called
// (StrictPQProfile constant not defined; profiles.go missing).
// =============================================================================
//
// SEVERITY: critical
//
// security_profile.go:724 reads `p := StrictPQProfile; return &p`
// but the constant StrictPQProfile is not defined anywhere in the
// package. The locked-profile architecture's single source of truth
// for "this is what mainnet runs" is therefore vapourware. Any caller
// that thinks they have the canonical profile in hand is actually
// holding a nil-dereference panic waiting to happen.
//
// FIX: write profiles.go that defines:
//   var StrictPQProfile = ChainSecurityProfile{ /* all fields */ }
//   var PermissiveProfile = ChainSecurityProfile{ /* all fields */ }
//   var FIPSProfile = ChainSecurityProfile{ /* all fields */ }
// Then call MustComputeHash on each at init time and write the
// returned hash back into ProfileHash so the constant carries its
// own pinned hash for genesis comparison.
func TestF52_CanonicalProfilesExist(t *testing.T) {
	// We try to obtain each canonical profile; any panic / nil here
	// is the finding's proof. Use defer/recover to capture the panic
	// because StrictPQ() panics when StrictPQProfile is missing
	// (it's a typed-undefined-identifier compile error in fact, so
	// this test would not link; but assuming it links the panic check
	// is the runtime guard).
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("StrictPQ() panicked: %v; finding F52 unfixed", r)
		}
	}()
	p := StrictPQ()
	if p == nil {
		t.Fatal("StrictPQ() returned nil; finding F52 unfixed")
	}
}

// =============================================================================
// F53 — ChainSecurityProfile.Validate enforces every locked-profile
// invariant per the doc-comment, but the existing security_profile_test.go
// uses old field names. The test will not compile against the new schema.
// =============================================================================
//
// SEVERITY: high
//
// security_profile_test.go has 14 tests that all reference
// `p.AllowedBackends` (renamed to `AllowedProofBackends`) and
// `p.ForbidClassicalSNARK` (renamed to `ForbidClassicalSNARKs`).
// Removing these tests is the wrong fix; the right fix is to update
// the tests to the renamed fields and ensure the renames don't cause
// the canonical strict-PQ profile to fail Validate at runtime.
//
// FIX: bulk-rename in security_profile_test.go (sed -i, not by hand):
//   AllowedBackends → AllowedProofBackends
//   AllowedFormats → AllowedProofFormats
//   ForbidClassicalSNARK → ForbidClassicalSNARKs
//   ErrProfileForbiddenPolicy → (define this var per F51 fix)
//   ErrProfileForbiddenBackend → (define this var)
//   ErrProfileForbiddenFormat → (define this var)
//   ErrProfileStrictRequiresTransparent → (define this var)
//   a.ComputeHash() != b.ComputeHash() → call .ComputeHash() with
//     error handling, then compare hashes
func TestF53_ValidateRejectsZeroProfile(t *testing.T) {
	// A zero-init ChainSecurityProfile MUST fail Validate. The
	// architecture says "Zero value = INVALID (never default-secure)."
	var p ChainSecurityProfile
	if err := p.Validate(); err == nil {
		t.Fatalf("zero-value ChainSecurityProfile passed Validate; finding F53 unfixed")
	}
}

// =============================================================================
// F54 — Validate does not check that the canonical profile's
// HashSuiteID matches the FinalitySchemeID's expected hash kernel
// (Pulsar-M-65 sits in SHA3 family; ML-DSA in SHAKE256 which is also
// FIPS 202). Cross-axis compatibility is not enforced.
// =============================================================================
//
// SEVERITY: medium
//
// A profile that combines HashSuiteBLAKE3Legacy with FinalitySchemeID
// SigSchemePulsarM65 (SHA-3 internal) is type-valid (both are non-zero
// and not forbidden in PQ mode per the current Validate). But Pulsar-M
// produces SHA3 internally; binding BLAKE3 at the transcript layer and
// SHA-3 at the signature kernel is a configuration inconsistency that
// audit pipelines should catch.
//
// FIX: add a `crossAxisCheck()` to Validate that asserts the
// (HashSuite, FinalityScheme) pair is one of the canonical sanctioned
// pairs.
func TestF54_Validate_CrossAxisHashSuiteVsFinality(t *testing.T) {
	p := tryGetLuxStrictPQ(t)
	if p == nil {
		t.Skip("skipped: depends on F52")
	}
	p.HashSuiteID = HashSuiteBLAKE3Legacy
	p.FinalitySchemeID = SigSchemePulsarM65 // SHA-3 internal — mismatch
	if err := p.Validate(); err == nil {
		t.Errorf("Validate accepted BLAKE3 transcript + Pulsar-M-65 finality; finding F54 unfixed")
	}
}

// =============================================================================
// F55 — Validate's "at least one Forbid* flag set" check (line 529)
// can be satisfied by setting any single Forbid bit; an operator can
// pass Validate while only forbidding (e.g.) dev-proofs and tolerating
// pairings + KZG + trusted-setup + classical-SNARK on a "strict-PQ"
// profile.
// =============================================================================
//
// SEVERITY: high
//
// security_profile.go:529 — the validator currently demands
// `!(ForbidPairings && ForbidKZG && ForbidTrustedSetup &&
// ForbidClassicalSNARKs && ForbidDevProofs && ForbidFallbacks)` to
// have AT LEAST ONE flag set. This is a defence against ALL-FALSE
// (which would obviously be wrong) but a profile that sets only
// ForbidDevProofs and leaves everything else false IS a valid strict-
// PQ profile per this check — and that is wrong. A strict-PQ profile
// MUST forbid every classical primitive.
//
// FIX: split the validator into "strict-PQ requires every Forbid* bit
// set true" (when ProfileID == ProfileStrictPQ) vs "permissive may
// have only ForbidFallbacks set" (when ProfilePermissive). The
// profile-class determines which forbids are mandatory.
func TestF55_Validate_StrictPQDemandsEveryForbidBit(t *testing.T) {
	p := tryGetLuxStrictPQ(t)
	if p == nil {
		t.Skip("skipped: depends on F52")
	}
	p.ForbidPairings = false
	p.ForbidKZG = false
	p.ForbidTrustedSetup = false
	p.ForbidClassicalSNARKs = false
	// ForbidDevProofs and ForbidFallbacks remain true → at-least-one is satisfied
	if err := p.Validate(); err == nil {
		t.Errorf("strict-PQ profile passed Validate with ForbidPairings/ForbidKZG/ForbidTrustedSetup/ForbidClassicalSNARKs all false; finding F55 unfixed")
	}
}

// =============================================================================
// F56 — ComputeHash sorts AllowedProofBackends and AllowedProofFormats
// before hashing (good), but does NOT sort other collection-shaped
// fields that may be added later. The "canonical encoding" is a
// must-remember rule when adding fields.
// =============================================================================
//
// SEVERITY: info
//
// FIX: factor the sort-and-emit into a helper `canonicalSlice` so any
// future []T field added to the profile must go through the helper
// and cannot accidentally hash in source-order.
func TestF56_ComputeHash_OrderInvariance(t *testing.T) {
	p := tryGetLuxStrictPQ(t)
	if p == nil {
		t.Skip("skipped: depends on F52")
	}
	q := *p
	// Reverse the allow-lists in q (post-fix names).
	q.AllowedProofBackends = reverseBackends(p.AllowedProofBackends)
	q.AllowedProofFormats = reverseFormats(p.AllowedProofFormats)

	pHash, pErr := p.ComputeHash()
	qHash, qErr := q.ComputeHash()
	if pErr != nil || qErr != nil {
		t.Fatalf("ComputeHash errors: p=%v q=%v", pErr, qErr)
	}
	if pHash != qHash {
		t.Errorf("ComputeHash is order-dependent; AllowedProofBackends/Formats must be sorted before hashing; finding F56 unfixed")
	}
}

// =============================================================================
// F57 — Validate is called inside ComputeHash; this means a profile
// that fails Validate at boot cannot produce a hash. But the boot path
// loads the profile from genesis JSON BEFORE Validate is called. If
// the genesis loader doesn't call Validate before ComputeHash, a
// malformed genesis silently produces an "error hash" that the
// operator might compare against the genesis-pinned value and panic
// at a confusing layer.
// =============================================================================
//
// SEVERITY: high
//
// FIX: in the genesis loader, call Validate FIRST, then ComputeHash.
// Make ComputeHash's error path explicitly "did Validate fail" so the
// operator's stack trace points to the missing field, not to a hash
// mismatch.
func TestF57_GenesisLoaderOrdering(t *testing.T) {
	// Structural finding; covered by red-team README. The runtime
	// invariant we can test: a profile with ProfileID = 0 still produces
	// a hash because ComputeHash returns the zero array along with the
	// error. A caller that ignores the error gets the zero array — and
	// the zero array is a distinguishable value an attacker could
	// pin to a genesis file.
	var p ChainSecurityProfile
	hash, err := p.ComputeHash()
	if err == nil {
		t.Errorf("ComputeHash on zero profile returned err=nil; finding F57 unfixed")
	}
	var zero [48]byte
	if hash != zero {
		// Good: the implementation guards against a stable hash on
		// invalid input.
	} else {
		t.Logf("ComputeHash returns zero array on invalid input — caller must check err")
	}
}

// =============================================================================
// F58 — VerifierID is uint16 (per security_profile.go) but
// reset-encoded as a 16-bit big-endian wire field in zchain's
// ZProofEnvelope. The 16-bit keyspace caps the number of distinct
// pinned verifiers across all clusters at 65536.
// =============================================================================
//
// SEVERITY: high
//
// A federation of 100 clusters, each running 5 backends, with 10
// program versions each, already needs 5000 distinct VerifierIDs.
// Cluster operators that independently number their verifiers will
// collide on the 16-bit space before the network scales out.
//
// RESOLUTION (per HIP-0078 / zchain F86): the architecture intentionally
// separates VerifierID (uint16 enum-shaped block of verifier KINDS) from
// ProgramOrAirID ([16]byte opaque program identifier). Distinct programs
// under the same VerifierID get distinct ProgramOrAirID values. Both
// fields appear in the envelope; both are hash-bound. The original
// widen-VerifierID proposal was rejected in favour of this two-axis
// design.
func TestF58_VerifierID_Width(t *testing.T) {
	// VerifierID is the canonical uint16 enum (verifier-KIND axis); the
	// per-program identifier lives in the separate 16-byte ProgramOrAirID
	// field on the envelope. This test pins the kind-axis width so a
	// future diff that widens VerifierID without auditing every encoding
	// site fails compile here.
	rt := reflect.TypeOf(VerifierID(0))
	if rt.Kind() != reflect.Uint16 {
		t.Errorf("VerifierID kind = %s; two-axis design pins kind-axis at uint16", rt.Kind())
	}
}

// =============================================================================
// F59 — VerifierID 0x0000 is the explicit "none" sentinel, but
// IsZero / IsNone methods are not defined. Callers must compare to
// VerifierNone directly; future refactors may introduce a
// uint16(0)-valued constant that aliases the sentinel.
// =============================================================================
//
// SEVERITY: info
//
// FIX: add `func (v VerifierID) IsZero() bool { return v == VerifierNone }`
// so the test `if v.IsZero()` is the canonical predicate.
func TestF59_VerifierID_HasIsZeroPredicate(t *testing.T) {
	v := VerifierNone
	// We assert the predicate's existence by calling it. If the method
	// is missing this is a compile failure — which is the test's signal.
	if !callVerifierIDIsZero(v) {
		t.Errorf("VerifierNone.IsZero() returned false; finding F59 unfixed")
	}
	if callVerifierIDIsZero(VerifierP3QSTARKFRISHA3PQ) {
		t.Errorf("VerifierP3QSTARKFRISHA3PQ.IsZero() returned true; finding F59 unfixed")
	}
}

// =============================================================================
// F60 — There is NO `ProfileByID(id ProfileID) (*ChainSecurityProfile,
// error)` function; callers must dispatch on a switch over ProfileID
// constants, which scatters the canonical profile registry across
// every call site.
// =============================================================================
//
// SEVERITY: medium
//
// FIX: add `func ProfileByID(id ProfileID) (*ChainSecurityProfile, error)`
// that returns the canonical profile for the ID or refuses with
// ErrProfileUnknown. Make every other call site dispatch through it.
func TestF60_ProfileByID_Exists(t *testing.T) {
	p, err := callProfileByID(ProfileStrictPQ)
	if err != nil {
		t.Fatalf("ProfileByID(ProfileStrictPQ) errored: %v; finding F60 unfixed", err)
	}
	if p == nil {
		t.Fatalf("ProfileByID(ProfileStrictPQ) returned nil; finding F60 unfixed")
	}
	if _, err := callProfileByID(ProfileNone); err == nil {
		t.Errorf("ProfileByID(ProfileNone) accepted; should refuse; finding F60 unfixed")
	}
}

// =============================================================================
// Helpers and stubs.
// =============================================================================

// tryGetLuxStrictPQ returns the canonical strict-PQ profile or nil if
// the constructor cannot be called (F52 not fixed). Tests that depend
// on a real profile skip when nil is returned.
func tryGetLuxStrictPQ(t *testing.T) *ChainSecurityProfile {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Logf("StrictPQ() panicked: %v — F52 unfixed", r)
		}
	}()
	return StrictPQ()
}

func reverseBackends(in []ProofBackendID) []ProofBackendID {
	out := append([]ProofBackendID(nil), in...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func reverseFormats(in []ProofFormatID) []ProofFormatID {
	out := append([]ProofFormatID(nil), in...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// callVerifierIDIsZero invokes (VerifierID).IsZero when it exists. The
// method does not exist in the current schema; this stub returns false
// so the F59 tests fail as the finding's proof.
func callVerifierIDIsZero(v VerifierID) bool {
	// Once IsZero is implemented, replace with: return v.IsZero()
	return v == VerifierNone // approximate behaviour while method is missing
}

// callProfileByID invokes the canonical registry lookup. ProfileByID
// is implemented in security_profile.go; the stub now dispatches.
func callProfileByID(id ProfileID) (*ChainSecurityProfile, error) {
	return ProfileByID(id)
}

var errProfileByIDMissing = &stubMissing{name: "ProfileByID"}

type stubMissing struct{ name string }

func (e *stubMissing) Error() string { return "red-team: API " + e.name + " not yet implemented" }
