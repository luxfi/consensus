// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zchain

import (
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
)

// happyPathBundle is the canonical fixture used by every test below.
// It holds:
//
//   - profile:   LuxStrictPQ
//   - registry:  one manifest under VerifierP3QSTARKFRISHA3PQ
//   - input:     canonical ZPublicInputs (every root populated)
//   - envelope:  ZProofEnvelope wired to the manifest + input
//
// Tests mutate one axis at a time and assert the verifier surfaces the
// expected typed error.
type happyPathBundle struct {
	profile  *config.ChainSecurityProfile
	registry *VerifierManifestRegistry
	input    *ZPublicInputs
	proof    *ZProofEnvelope
}

func newHappyPathBundle(t *testing.T) *happyPathBundle {
	t.Helper()
	// Always start from a clean backend-binding map so tests do not
	// leak BackendVerifier bindings into each other.
	resetBackendVerifiersForTest()

	profile := config.LuxStrictPQ()
	if err := profile.Validate(); err != nil {
		t.Fatalf("LuxStrictPQ() failed validate: %v", err)
	}

	registry := NewVerifierManifestRegistry(nil)
	manifest := &VerifierManifest{
		VerifierID:            config.VerifierP3QSTARKFRISHA3PQ,
		BackendID:             config.ProofBackendP3QSTARKFRISHA3,
		Version:               "v0.1.0",
		SourceCommit:          padBytes20(0xDE),
		BuildProfile:          "production",
		ProofFormatID:         config.ProofFormatP3QBinaryV1,
		ProgramOrAirHash:      padBytes48(0x44),
		VerifierKeyHash:       padBytes48(0x22),
		SupportsPolicyIDs:     []config.ProofPolicyID{config.ProofPolicySTARKFRISHA3PQ},
		SoundnessBitsReviewed: 128,
		HashOutputBits:        384,
	}
	if err := registry.Register(manifest); err != nil {
		t.Fatalf("Register(manifest) failed: %v", err)
	}

	input := fixturePublicInputs()
	publicInputsHash := HashZPublicInputs(input)

	proof := &ZProofEnvelope{
		Version:              1,
		ProfileID:            uint32(profile.ProfileID),
		ChainID:              42,
		NetworkID:            1,
		Epoch:                7,
		ProofPolicyID:        config.ProofPolicySTARKFRISHA3PQ,
		ProofBackendID:       config.ProofBackendP3QSTARKFRISHA3,
		ProofFormatID:        config.ProofFormatP3QBinaryV1,
		VerifierID:           config.VerifierP3QSTARKFRISHA3PQ,
		HashSuiteID:          config.HashSuiteSHA3NIST,
		IdentitySchemeID:     config.SigSchemeMLDSA65,
		FinalitySchemeID:     config.SigSchemePulsarM65,
		PublicInputsHash:     publicInputsHash,
		VerifierKeyHash:      padBytes48(0x22),
		ProgramOrAirID:       padBytes16(0x33),
		ProgramOrAirHash:     padBytes48(0x44),
		SoundnessBitsClaimed: 128,
		HashOutputBits:       384,
		TransparentSetup:     true,
		ProofBytes:           []byte("opaque-stark-proof-bytes"),
	}

	return &happyPathBundle{
		profile:  profile,
		registry: registry,
		input:    input,
		proof:    proof,
	}
}

func padBytes16(b byte) [16]byte {
	var out [16]byte
	for i := range out {
		out[i] = b
	}
	return out
}

func padBytes20(b byte) [20]byte {
	var out [20]byte
	for i := range out {
		out[i] = b
	}
	return out
}

func padBytes48(b byte) [48]byte {
	var out [48]byte
	for i := range out {
		out[i] = b
	}
	return out
}

// TestVerifyZProofUnderProfile_HappyPath proves a fully-correct
// LuxStrictPQ envelope (ML-DSA-65 / Pulsar-M-65 / STARK_FRI_SHA3_PQ)
// passes every check and returns nil. Dev-build path: no backend
// binding required.
func TestVerifyZProofUnderProfile_HappyPath(t *testing.T) {
	b := newHappyPathBundle(t)
	if err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof); err != nil {
		t.Fatalf("happy path returned %v; want nil", err)
	}
}

// TestVerifyZProofUnderProfile_HappyPath_WithBoundBackend exercises check 15
// when a backend is bound. The fake backend returns true; the verifier
// should pass.
func TestVerifyZProofUnderProfile_HappyPath_WithBoundBackend(t *testing.T) {
	b := newHappyPathBundle(t)
	if err := RegisterBackendVerifier(
		config.VerifierP3QSTARKFRISHA3PQ,
		BackendVerifierFunc(func(_ *VerifierManifest, _ *ZPublicInputs, _ *ZProofEnvelope) (bool, error) {
			return true, nil
		}),
	); err != nil {
		t.Fatalf("RegisterBackendVerifier: %v", err)
	}
	if err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof); err != nil {
		t.Fatalf("happy path with bound backend returned %v; want nil", err)
	}
}

// TestVerifyZProofUnderProfile_BackendReturnsFalse proves check 15
// surfaces a backend-false outcome as ErrZProofBackendVerifyFailed.
func TestVerifyZProofUnderProfile_BackendReturnsFalse(t *testing.T) {
	b := newHappyPathBundle(t)
	if err := RegisterBackendVerifier(
		config.VerifierP3QSTARKFRISHA3PQ,
		BackendVerifierFunc(func(_ *VerifierManifest, _ *ZPublicInputs, _ *ZProofEnvelope) (bool, error) {
			return false, nil
		}),
	); err != nil {
		t.Fatalf("RegisterBackendVerifier: %v", err)
	}
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofBackendVerifyFailed) {
		t.Errorf("backend-false case returned %v; want ErrZProofBackendVerifyFailed", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsProfileIDMismatch — check 1.
func TestVerifyZProofUnderProfile_RejectsProfileIDMismatch(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.ProfileID = uint32(config.ProfileLuxPermissive)
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofProfileIDMismatch) {
		t.Errorf("got %v; want ErrZProofProfileIDMismatch", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsHashSuiteMismatch — check 2.
func TestVerifyZProofUnderProfile_RejectsHashSuiteMismatch(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.HashSuiteID = config.HashSuiteBLAKE3Legacy
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofHashSuiteMismatch) {
		t.Errorf("got %v; want ErrZProofHashSuiteMismatch", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsPolicyMismatch — check 3.
func TestVerifyZProofUnderProfile_RejectsPolicyMismatch(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.ProofPolicyID = config.ProofPolicySTARKFRIKeccak
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofPolicyMismatch) {
		t.Errorf("got %v; want ErrZProofPolicyMismatch", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsBackendNotInProfile — check 4.
func TestVerifyZProofUnderProfile_RejectsBackendNotInProfile(t *testing.T) {
	b := newHappyPathBundle(t)
	// Use the forbidden Groth16-wrap backend — not in LuxStrictPQ's allow list.
	b.proof.ProofBackendID = config.ProofBackendGroth16WrapForbid
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofBackendForbidden) {
		t.Errorf("got %v; want ErrZProofBackendForbidden", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsFormatNotInProfile — check 5.
func TestVerifyZProofUnderProfile_RejectsFormatNotInProfile(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.ProofFormatID = config.ProofFormatGroth16WrappedForbid
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofFormatForbidden) {
		t.Errorf("got %v; want ErrZProofFormatForbidden", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsSoundnessTooLow — check 6.
func TestVerifyZProofUnderProfile_RejectsSoundnessTooLow(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.SoundnessBitsClaimed = b.profile.MinSoundnessBits - 1
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofSoundnessTooLow) {
		t.Errorf("got %v; want ErrZProofSoundnessTooLow", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsHashOutputTooShort — check 7.
func TestVerifyZProofUnderProfile_RejectsHashOutputTooShort(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.HashOutputBits = b.profile.MinHashOutputBits - 1
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofHashOutputTooShort) {
		t.Errorf("got %v; want ErrZProofHashOutputTooShort", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsTransparentRequired — check 8.
func TestVerifyZProofUnderProfile_RejectsTransparentRequired(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.TransparentSetup = false
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofTransparentRequired) {
		t.Errorf("got %v; want ErrZProofTransparentRequired", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsPairings — check 9 (pairings axis).
func TestVerifyZProofUnderProfile_RejectsPairings(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.UsesPairings = true
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofPairingsForbidden) {
		t.Errorf("got %v; want ErrZProofPairingsForbidden", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsKZG — check 9 (KZG axis).
// Named to match the spec ("RejectsKZG").
func TestVerifyZProofUnderProfile_RejectsKZG(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.UsesKZG = true
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofKZGForbidden) {
		t.Errorf("got %v; want ErrZProofKZGForbidden", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsTrustedSetup — check 9 (trusted-setup axis).
func TestVerifyZProofUnderProfile_RejectsTrustedSetup(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.UsesTrustedSetup = true
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofTrustedSetupForbidden) {
		t.Errorf("got %v; want ErrZProofTrustedSetupForbidden", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsClassicalWrap — check 9
// (classical-SNARK-wrapper axis). Named to match the spec.
func TestVerifyZProofUnderProfile_RejectsClassicalWrap(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.UsesClassicalSNARKWrapper = true
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofClassicalSNARKForbidden) {
		t.Errorf("got %v; want ErrZProofClassicalSNARKForbidden", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsUnknownVerifier — check 10.
func TestVerifyZProofUnderProfile_RejectsUnknownVerifier(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.VerifierID = config.VerifierSP1CompressedSTARKPQ // registry only has P3Q
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofVerifierUnknown) {
		t.Errorf("got %v; want ErrZProofVerifierUnknown", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsManifestBackendMismatch — check 11.
// Mutate the ENVELOPE's BackendID after the bundle is built. The
// manifest registered under VerifierP3Q claims BackendID=P3Q; the
// envelope now claims SP1 backend. Verifier surfaces
// ErrZProofVerifierManifestBackendMismatch (provided SP1 is also in
// the profile's AllowedBackends, which LuxStrictPQ has).
func TestVerifyZProofUnderProfile_RejectsManifestBackendMismatch(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.ProofBackendID = config.ProofBackendSP1CompressedSTARK
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofVerifierManifestBackendMismatch) {
		t.Errorf("got %v; want ErrZProofVerifierManifestBackendMismatch", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsManifestFormatMismatch — check 11b.
// Mutate the ENVELOPE's ProofFormatID after the bundle is built. The
// manifest registered under VerifierP3Q claims ProofFormatID=P3QBinaryV1;
// the envelope now claims STARKFRIBinaryV1. Verifier surfaces
// ErrZProofVerifierManifestFormatMismatch.
func TestVerifyZProofUnderProfile_RejectsManifestFormatMismatch(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.ProofFormatID = config.ProofFormatSTARKFRIBinaryV1
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofVerifierManifestFormatMismatch) {
		t.Errorf("got %v; want ErrZProofVerifierManifestFormatMismatch", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsProgramHashMismatch — check 12.
func TestVerifyZProofUnderProfile_RejectsProgramHashMismatch(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.ProgramOrAirHash[0] ^= 0xFF
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofProgramHashMismatch) {
		t.Errorf("got %v; want ErrZProofProgramHashMismatch", err)
	}
}

// TestVerifyZProofUnderProfile_RejectsVerifierKeyMismatch — check 13.
func TestVerifyZProofUnderProfile_RejectsVerifierKeyMismatch(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.VerifierKeyHash[0] ^= 0xFF
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofVerifierKeyMismatch) {
		t.Errorf("got %v; want ErrZProofVerifierKeyMismatch", err)
	}
}

// TestVerifyZProofUnderProfile_PublicInputsBindingMismatch — check 14.
// Mutate the input AFTER the producer hashed; the verifier-side recompute
// MUST diverge from proof.PublicInputsHash.
func TestVerifyZProofUnderProfile_PublicInputsBindingMismatch(t *testing.T) {
	b := newHappyPathBundle(t)
	// Mutate one input field — the proof still carries the pre-mutation hash.
	b.input.NewZStateRoot[0] ^= 0xFF
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofPublicInputsMismatch) {
		t.Errorf("got %v; want ErrZProofPublicInputsMismatch", err)
	}
}

// TestVerifyZProofUnderProfile_NilGuards proves every nil-pointer
// argument surfaces a typed error rather than panicking.
func TestVerifyZProofUnderProfile_NilGuards(t *testing.T) {
	b := newHappyPathBundle(t)
	cases := []struct {
		name string
		fn   func() error
		want error
	}{
		{"nil profile", func() error {
			return VerifyZProofUnderProfile(nil, b.registry, b.input, b.proof)
		}, ErrZProofNilProfile},
		{"nil registry", func() error {
			return VerifyZProofUnderProfile(b.profile, nil, b.input, b.proof)
		}, ErrZProofNilRegistry},
		{"nil input", func() error {
			return VerifyZProofUnderProfile(b.profile, b.registry, nil, b.proof)
		}, ErrZProofNilInput},
		{"nil proof", func() error {
			return VerifyZProofUnderProfile(b.profile, b.registry, b.input, nil)
		}, ErrZProofNilProof},
	}
	for _, c := range cases {
		if err := c.fn(); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v; want %v", c.name, err, c.want)
		}
	}
}

// TestVerifyZProofUnderProfile_RejectsBackendNotInProfile_DevBackend
// proves a dev backend (SP1CoreSTARKDev) is not admitted on LuxStrictPQ
// even though it is post-quantum: the profile lists production backends only.
func TestVerifyZProofUnderProfile_RejectsBackendNotInProfile_DevBackend(t *testing.T) {
	b := newHappyPathBundle(t)
	b.proof.ProofBackendID = config.ProofBackendSP1CoreSTARKDev
	err := VerifyZProofUnderProfile(b.profile, b.registry, b.input, b.proof)
	if !errors.Is(err, ErrZProofBackendForbidden) {
		t.Errorf("got %v; want ErrZProofBackendForbidden", err)
	}
}
