// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zchain

import (
	"errors"
	"fmt"

	"github.com/luxfi/consensus/config"
)

// verify.go — the strict, fail-closed verifier for ZProofEnvelope.
//
// One entry point: VerifyZProofUnderProfile. Every safety decision flows
// through it. The 15 checks run in a fixed order (cheap structural →
// manifest lookup → public-inputs binding → backend dispatch); the
// rationale is that we never want to pay a backend verifier's cost
// for a misconfigured envelope, and we never want a backend to assert
// its own safety.
//
// No panic() in this file. Every refusal returns a typed error from
// the set below so the caller (or audit pipeline) can route the
// failure deterministically.

// Typed verifier errors. The names map 1:1 to the 15 checks in
// VerifyZProofUnderProfile. New checks add a new error variable;
// existing errors are never renamed (their wire-equivalent identity
// is "the typed error a strict-PQ verifier returned for rule N").
var (
	ErrZProofProfileIDMismatch               = errors.New("zchain: proof.ProfileID does not match profile.ProfileID")
	ErrZProofHashSuiteMismatch               = errors.New("zchain: proof.HashSuiteID does not match profile.HashSuiteID")
	ErrZProofPolicyMismatch                  = errors.New("zchain: proof.ProofPolicyID does not match profile.ProofPolicyID")
	ErrZProofBackendForbidden                = errors.New("zchain: proof.ProofBackendID is not in profile.AllowedBackends")
	ErrZProofFormatForbidden                 = errors.New("zchain: proof.ProofFormatID is not in profile.AllowedFormats")
	ErrZProofSoundnessTooLow                 = errors.New("zchain: proof.SoundnessBitsClaimed below profile.MinSoundnessBits")
	ErrZProofHashOutputTooShort              = errors.New("zchain: proof.HashOutputBits below profile.MinHashOutputBits")
	ErrZProofTransparentRequired             = errors.New("zchain: profile.RequireTransparent demands proof.TransparentSetup=true")
	ErrZProofPairingsForbidden               = errors.New("zchain: profile.ForbidPairings demands proof.UsesPairings=false")
	ErrZProofKZGForbidden                    = errors.New("zchain: profile.ForbidKZG demands proof.UsesKZG=false")
	ErrZProofTrustedSetupForbidden           = errors.New("zchain: profile.ForbidTrustedSetup demands proof.UsesTrustedSetup=false")
	ErrZProofClassicalSNARKForbidden         = errors.New("zchain: profile.ForbidClassicalSNARK demands proof.UsesClassicalSNARKWrapper=false")
	ErrZProofVerifierUnknown                 = errors.New("zchain: VerifierManifestRegistry has no manifest for proof.VerifierID")
	ErrZProofVerifierManifestBackendMismatch = errors.New("zchain: VerifierManifest.BackendID does not match proof.ProofBackendID")
	ErrZProofProgramHashMismatch             = errors.New("zchain: VerifierManifest.ProgramOrAirHash does not match proof.ProgramOrAirHash")
	ErrZProofVerifierKeyMismatch             = errors.New("zchain: VerifierManifest.VerifierKeyHash does not match proof.VerifierKeyHash")
	ErrZProofPublicInputsMismatch            = errors.New("zchain: HashZPublicInputs(input) does not match proof.PublicInputsHash")
	ErrZProofVerifierManifestFormatMismatch  = errors.New("zchain: VerifierManifest.ProofFormatID does not match proof.ProofFormatID")
	ErrZProofVerifierManifestPolicyMismatch  = errors.New("zchain: VerifierManifest.SupportsPolicyIDs does not include proof.ProofPolicyID")
	ErrZProofNilProfile                      = errors.New("zchain: nil ChainSecurityProfile")
	ErrZProofNilRegistry                     = errors.New("zchain: nil VerifierManifestRegistry")
	ErrZProofNilInput                        = errors.New("zchain: nil ZPublicInputs")
	ErrZProofNilProof                        = errors.New("zchain: nil ZProofEnvelope")
	ErrZProofBackendVerifyFailed             = errors.New("zchain: backend verifier returned false")
)

// BackendVerifier is the interface that a backend implementation
// (SP1, RISC0, P3Q, Stone, Stwo) MUST implement to be dispatched by
// VerifyZProofUnderProfile. Implementations live OUTSIDE this package
// (e.g. github.com/luxfi/sp1/zverifier) and bind themselves at boot
// via RegisterBackendVerifier.
//
// The verifier receives:
//   - the manifest pinned at boot (immutable, registry-owned pointer)
//   - the canonical public inputs (already hash-bound by the caller)
//   - the envelope (every axis already checked against profile +
//     manifest by the caller)
//
// All the implementation needs to do is:
//  1. Parse proof.ProofBytes under manifest.ProofFormatID.
//  2. Run the cryptographic verifier.
//  3. Return true iff the proof verifies under (publicInputs,
//     manifest.VerifierKeyHash, manifest.ProgramOrAirHash).
//
// No I/O. No network. Constant-time on accepted vs rejected inputs.
type BackendVerifier interface {
	Verify(
		manifest *VerifierManifest,
		publicInputs *ZPublicInputs,
		proof *ZProofEnvelope,
	) (bool, error)
}

// BackendVerifierFunc adapts a function to the BackendVerifier interface
// so backend bindings can register a closure without defining a type.
type BackendVerifierFunc func(*VerifierManifest, *ZPublicInputs, *ZProofEnvelope) (bool, error)

// Verify dispatches to the function.
func (f BackendVerifierFunc) Verify(
	m *VerifierManifest,
	in *ZPublicInputs,
	p *ZProofEnvelope,
) (bool, error) {
	return f(m, in, p)
}

// VerifyZProofUnderProfile runs the fail-closed admissibility check
// chain and, if every check passes, dispatches to the backend
// verifier bound to proof.VerifierID. Returns nil iff the proof is
// admissible AND verifies under the manifest-pinned backend.
//
// Checks (in order, all fail-closed):
//
//  1. proof.ProfileID == profile.ProfileID
//  2. proof.HashSuiteID == profile.HashSuiteID
//  3. proof.ProofPolicyID == profile.ProofPolicyID
//  4. profile.AllowsBackend(proof.ProofBackendID)
//  5. profile.AllowsFormat(proof.ProofFormatID)
//  6. proof.SoundnessBitsClaimed >= profile.MinSoundnessBits
//  7. proof.HashOutputBits >= profile.MinHashOutputBits
//  8. profile.RequireTransparent ⇒ proof.TransparentSetup
//  9. profile.ForbidPairings/KZG/TrustedSetup/ClassicalSNARK contradictions
//  10. manifest := registry.Lookup(proof.VerifierID); not-exists ⇒ reject
//  11. manifest.BackendID == proof.ProofBackendID
//  12. manifest.ProgramOrAirHash == proof.ProgramOrAirHash
//  13. manifest.VerifierKeyHash == proof.VerifierKeyHash
//  14. HashZPublicInputs(input) == proof.PublicInputsHash
//  15. Dispatch backend verifier (mock returns nil if no backend bound
//     under the dev build tag; production build requires a real binding).
//
// The function performs no other work; consumers wrap higher-level
// policy on top of it (e.g. Q-Chain AcceptQBlock binds Z-Chain proof
// verification when the cert carries one).
func VerifyZProofUnderProfile(
	profile *config.ChainSecurityProfile,
	registry *VerifierManifestRegistry,
	input *ZPublicInputs,
	proof *ZProofEnvelope,
) error {
	if profile == nil {
		return ErrZProofNilProfile
	}
	if registry == nil {
		return ErrZProofNilRegistry
	}
	if input == nil {
		return ErrZProofNilInput
	}
	if proof == nil {
		return ErrZProofNilProof
	}

	// Check 1: ProfileID match.
	if proof.ProfileID != profile.ProfileID {
		return fmt.Errorf("%w: profile=%d proof=%d",
			ErrZProofProfileIDMismatch, profile.ProfileID, proof.ProfileID)
	}
	// Check 2: HashSuiteID match.
	if proof.HashSuiteID != profile.HashSuiteID {
		return fmt.Errorf("%w: profile=%s proof=%s",
			ErrZProofHashSuiteMismatch, profile.HashSuiteID.String(), proof.HashSuiteID.String())
	}
	// Check 3: ProofPolicyID match.
	if proof.ProofPolicyID != profile.ProofPolicyID {
		return fmt.Errorf("%w: profile=%s proof=%s",
			ErrZProofPolicyMismatch, profile.ProofPolicyID.String(), proof.ProofPolicyID.String())
	}
	// Check 4: backend allowed.
	if !profile.AllowsBackend(proof.ProofBackendID) {
		return fmt.Errorf("%w: %s", ErrZProofBackendForbidden, proof.ProofBackendID.String())
	}
	// Check 5: format allowed.
	if !profile.AllowsFormat(proof.ProofFormatID) {
		return fmt.Errorf("%w: %s", ErrZProofFormatForbidden, proof.ProofFormatID.String())
	}
	// Check 6: soundness floor.
	if proof.SoundnessBitsClaimed < profile.MinSoundnessBits {
		return fmt.Errorf("%w: claimed=%d floor=%d",
			ErrZProofSoundnessTooLow, proof.SoundnessBitsClaimed, profile.MinSoundnessBits)
	}
	// Check 7: hash-output floor.
	if proof.HashOutputBits < profile.MinHashOutputBits {
		return fmt.Errorf("%w: claimed=%d floor=%d",
			ErrZProofHashOutputTooShort, proof.HashOutputBits, profile.MinHashOutputBits)
	}
	// Check 8: transparency requirement.
	if profile.RequireTransparent && !proof.TransparentSetup {
		return ErrZProofTransparentRequired
	}
	// Check 9: classical-primitive bans.
	if profile.ForbidPairings && proof.UsesPairings {
		return ErrZProofPairingsForbidden
	}
	if profile.ForbidKZG && proof.UsesKZG {
		return ErrZProofKZGForbidden
	}
	if profile.ForbidTrustedSetup && proof.UsesTrustedSetup {
		return ErrZProofTrustedSetupForbidden
	}
	if profile.ForbidClassicalSNARKs && proof.UsesClassicalSNARKWrapper {
		return ErrZProofClassicalSNARKForbidden
	}

	// Check 10: manifest lookup.
	manifest, ok := registry.Lookup(proof.VerifierID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrZProofVerifierUnknown, proof.VerifierID.String())
	}

	// Check 11: manifest backend matches envelope backend.
	if manifest.BackendID != proof.ProofBackendID {
		return fmt.Errorf("%w: manifest=%s proof=%s",
			ErrZProofVerifierManifestBackendMismatch,
			manifest.BackendID.String(), proof.ProofBackendID.String())
	}
	// Check 11b: manifest format matches envelope format (sub-rule of 11 —
	// pinned for completeness; the registry refuses ProofFormatNone at
	// Register so this is a drift detector).
	if manifest.ProofFormatID != proof.ProofFormatID {
		return fmt.Errorf("%w: manifest=%s proof=%s",
			ErrZProofVerifierManifestFormatMismatch,
			manifest.ProofFormatID.String(), proof.ProofFormatID.String())
	}
	// Check 11c: manifest policy supports envelope policy. Same as above:
	// the registry refuses an empty SupportsPolicyIDs at Register, so this
	// catches a policy-axis drift between manifest and envelope.
	if !manifest.SupportsPolicy(proof.ProofPolicyID) {
		return fmt.Errorf("%w: manifest=%v proof=%s",
			ErrZProofVerifierManifestPolicyMismatch,
			manifest.SupportsPolicyIDs, proof.ProofPolicyID.String())
	}

	// Check 12: program / AIR hash equality.
	if manifest.ProgramOrAirHash != proof.ProgramOrAirHash {
		return ErrZProofProgramHashMismatch
	}
	// Check 13: verifier-key hash equality.
	if manifest.VerifierKeyHash != proof.VerifierKeyHash {
		return ErrZProofVerifierKeyMismatch
	}

	// Check 14: public-inputs binding. The producer-side hash already
	// rides in proof.PublicInputsHash; we re-derive verifier-side and
	// refuse on any mismatch. This is the only check that exercises a
	// hash beyond the cheap byte comparisons above.
	computed := HashZPublicInputs(input)
	if computed != proof.PublicInputsHash {
		return ErrZProofPublicInputsMismatch
	}

	// Check 15: backend dispatch. The backend verifier was registered
	// at boot via RegisterBackendVerifier; in dev builds (build tag
	// !production) the absence of a binding falls through to a no-op
	// success so test fixtures don't need a full cryptographic
	// implementation. Production builds REQUIRE a real binding;
	// see verify_production.go.
	if backend := lookupBackendVerifier(proof.VerifierID); backend != nil {
		ok, err := backend.Verify(manifest, input, proof)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrZProofBackendVerifyFailed, err)
		}
		if !ok {
			return ErrZProofBackendVerifyFailed
		}
	} else if requireBackendBinding {
		return fmt.Errorf("%w: no backend bound for %s", ErrZProofBackendVerifyFailed, proof.VerifierID.String())
	}
	return nil
}
