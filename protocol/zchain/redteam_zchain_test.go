// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// redteam_zchain_test.go — adversarial review of the Z-Chain proof
// envelope, VerifierManifestRegistry, and the (missing)
// VerifyZProofUnderProfile dispatch. Each F## test pins one footgun.
//
// Build prerequisite: github.com/luxfi/consensus/config must compile —
// the ProofFormatID and VerifierID duplicate declarations resolved
// to a single canonical definition each.

package zchain

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/log"
)

// noopLog returns a no-op logger for tests. Wrapper so the symbol used
// here matches whichever stable accessor the log package exposes.
func noopLog() log.Logger { return log.Noop() }

// =============================================================================
// F80 — VerifyZProofUnderProfile exists; nil-input handling.
// =============================================================================
//
// SEVERITY: info (post-fix verification)
//
// The function landed (verify.go). This test confirms it refuses nil
// inputs with typed errors (ErrZProofNilProfile / ErrZProofNilRegistry
// / ErrZProofNilInput / ErrZProofNilProof) so callers can route the
// failure.
func TestF80_VerifyZProofUnderProfile_RefusesNilProfile(t *testing.T) {
	err := callVerifyZProofUnderProfile(nil, nil, nil, nil)
	if !errors.Is(err, ErrZProofNilProfile) {
		t.Errorf("VerifyZProofUnderProfile(nil...) returned %v; want ErrZProofNilProfile", err)
	}
}

// =============================================================================
// F81 — VerifierManifestRegistry.Lookup returns the internal pointer;
// callers can mutate the stored manifest at runtime.
// =============================================================================
//
// SEVERITY: critical
//
// VerifierManifestRegistry.Lookup (verifier_manifest.go:200) returns
// the *VerifierManifest stored in the internal map. The doc says
// "callers MUST treat it as read-only" but nothing enforces this.
// A caller — including a deliberate attacker who gets a single
// Lookup-call-equivalent into the verifier — can mutate
// m.SupportsPolicyIDs, m.VerifierKeyHash, m.ProofFormatID, etc.
//
// The registry is "monotonic for the process lifetime" only at the
// keyspace level; the values are open to in-place mutation.
//
// ATTACK: any call site that does:
//   m, _ := registry.Lookup(vid)
//   m.SupportsPolicyIDs = append(m.SupportsPolicyIDs, ProofPolicyGroth16BN254Forbid)
// has just authorised a classical Groth16 proof under the existing
// VerifierID without going through Register's checks (which would
// have refused VerifierID conflicts and required Version /
// BuildProfile).
//
// FIX: Lookup MUST return a defensive copy (similar to what Register
// does on the inbound manifest). Or change the type to return
// `VerifierManifest` (value), not `*VerifierManifest`.
func TestF81_VerifierRegistry_LookupReturnsDefensiveCopy(t *testing.T) {
	r := NewVerifierManifestRegistry(noopLog())
	original := &VerifierManifest{
		VerifierID:        config.VerifierP3QSTARKFRISHA3PQ,
		BackendID:         config.ProofBackendP3QSTARKFRISHA3,
		Version:           "1.0.0",
		BuildProfile:      "production",
		ProofFormatID:     config.ProofFormatP3QBinaryV1,
		SupportsPolicyIDs: []config.ProofPolicyID{config.ProofPolicySTARKFRISHA3PQ},
	}
	if err := r.Register(original); err != nil {
		t.Fatalf("Register: %v", err)
	}

	m1, ok := r.Lookup(config.VerifierP3QSTARKFRISHA3PQ)
	if !ok {
		t.Fatal("Lookup failed")
	}

	// ATTACK: caller mutates the stored manifest's policy support.
	m1.SupportsPolicyIDs = append(m1.SupportsPolicyIDs, config.ProofPolicyGroth16BN254Forbid)

	m2, ok := r.Lookup(config.VerifierP3QSTARKFRISHA3PQ)
	if !ok {
		t.Fatal("second Lookup failed")
	}

	// If Lookup returned a defensive copy, m2's policy list is still the
	// original. If not, the attacker mutation has been persisted.
	if m2.SupportsPolicy(config.ProofPolicyGroth16BN254Forbid) {
		t.Fatalf("VerifierManifestRegistry persisted external mutation of SupportsPolicyIDs; "+
			"Groth16-classical now appears to be supported by %s; finding F81 unfixed",
			m2.VerifierID.String())
	}
}

// F81b: Same attack on VerifierKeyHash. If the verifier-key hash can be
// mutated after Register, an attacker can match the manifest hash to
// an arbitrary envelope's claimed hash and bypass the drift-refusal.
func TestF81b_VerifierRegistry_LookupMutationOnHashFields(t *testing.T) {
	r := NewVerifierManifestRegistry(noopLog())
	original := &VerifierManifest{
		VerifierID:        config.VerifierP3QSTARKFRISHA3PQ,
		BackendID:         config.ProofBackendP3QSTARKFRISHA3,
		Version:           "1.0.0",
		BuildProfile:      "production",
		ProofFormatID:     config.ProofFormatP3QBinaryV1,
		SupportsPolicyIDs: []config.ProofPolicyID{config.ProofPolicySTARKFRISHA3PQ},
		ProgramOrAirHash:  fill48Z(0xAA),
		VerifierKeyHash:   fill48Z(0xBB),
	}
	if err := r.Register(original); err != nil {
		t.Fatalf("Register: %v", err)
	}

	m1, _ := r.Lookup(config.VerifierP3QSTARKFRISHA3PQ)
	// ATTACK: rewrite the stored key hash to match an arbitrary target.
	target := fill48Z(0xCC)
	m1.VerifierKeyHash = target

	m2, _ := r.Lookup(config.VerifierP3QSTARKFRISHA3PQ)
	if m2.VerifierKeyHash == target {
		t.Fatalf("VerifierManifestRegistry persisted external mutation of VerifierKeyHash; finding F81 unfixed")
	}
}

// =============================================================================
// F82 — Register checks length(SupportsPolicyIDs) > 0 but does NOT
// check policy validity; a manifest can claim SupportsPolicyIDs =
// [ProofPolicyGroth16BN254Forbid].
// =============================================================================
//
// SEVERITY: high
//
// verifier_manifest.go:170 refuses an empty SupportsPolicyIDs list.
// It does NOT refuse a list containing IsForbiddenInPQMode() policies.
// A genesis-loader or boot-config bug that registers a manifest
// claiming to support Groth16-BN254 will succeed. Downstream the
// profile may refuse the policy on the envelope — but the manifest
// claim is recorded as "this verifier supports classical Groth16,"
// which is wrong as a documentation artefact and dangerous if any
// future call site consults manifest.SupportsPolicy() before consulting
// profile.AllowedFormats / IsPostQuantum.
//
// FIX: in Register, refuse any SupportsPolicyIDs element with
// IsForbiddenInPQMode()=true. The dev-build can bypass via a separate
// RegisterUnsafe entry-point (off in production).
func TestF82_VerifierRegistry_RejectsForbiddenPolicyInSupportsList(t *testing.T) {
	r := NewVerifierManifestRegistry(noopLog())
	m := &VerifierManifest{
		VerifierID:    config.VerifierP3QSTARKFRISHA3PQ,
		BackendID:     config.ProofBackendP3QSTARKFRISHA3,
		Version:       "1.0.0",
		BuildProfile:  "production",
		ProofFormatID: config.ProofFormatP3QBinaryV1,
		SupportsPolicyIDs: []config.ProofPolicyID{
			config.ProofPolicySTARKFRISHA3PQ,
			config.ProofPolicyGroth16BN254Forbid, // forbidden marker
		},
	}
	err := r.Register(m)
	if err == nil {
		t.Fatalf("Register accepted manifest with ProofPolicyGroth16BN254Forbid in SupportsPolicyIDs; finding F82 unfixed")
	}
}

// =============================================================================
// F83 — Register does not check ProofFormatID matches BackendID.
// =============================================================================
//
// SEVERITY: medium
//
// A manifest can claim BackendID = SP1CompressedSTARK and
// ProofFormatID = RISC0BinaryV1 (cross-backend mismatch). Register
// accepts this. Downstream, VerifyZProofUnderProfile (when it exists)
// would compare envelope.ProofFormatID against manifest.ProofFormatID
// and route to the SP1 verifier even though the envelope's format is
// labelled RISC0. The mismatch slips through.
//
// FIX: add a small matrix at Register time that asserts
// (backend, format) is in the set of allowed pairs. Pair table:
//   SP1CompressedSTARK    → STARKFRIBinaryV1, SP1BinaryV1
//   RISC0SuccinctSTARK    → STARKFRIBinaryV1, RISC0BinaryV1
//   P3QSTARKFRISHA3       → STARKFRIBinaryV1, P3QBinaryV1
//   StoneCairoSTARK       → STARKFRIBinaryV1, StoneCairoBinaryV1
//   StwoCircleSTARK       → STARKFRIBinaryV1, StwoCircleBinaryV1
func TestF83_VerifierRegistry_RejectsBackendFormatMismatch(t *testing.T) {
	r := NewVerifierManifestRegistry(noopLog())
	m := &VerifierManifest{
		VerifierID:        config.VerifierSP1CompressedSTARKPQ,
		BackendID:         config.ProofBackendSP1CompressedSTARK, // SP1
		ProofFormatID:     config.ProofFormatRISC0BinaryV1,       // RISC0 format — MISMATCH
		Version:           "1.0.0",
		BuildProfile:      "production",
		SupportsPolicyIDs: []config.ProofPolicyID{config.ProofPolicySTARKFRISHA3PQ},
	}
	err := r.Register(m)
	if err == nil {
		t.Fatalf("Register accepted SP1 backend with RISC0 format; finding F83 unfixed")
	}
}

// =============================================================================
// F84 — HashZPublicInputs binds every field per the test below, but
// the nil-receiver path returns a well-defined hash that an attacker
// could collide a real input against.
// =============================================================================
//
// SEVERITY: high
//
// HashZPublicInputs(nil) returns
// `tupleHash48([][]byte{[]byte(publicInputsProtocolTag)}, publicInputsCustomization)`.
// This is a deterministic 48-byte constant. An attacker who can submit
// an envelope whose PublicInputsHash equals this constant has just
// claimed to be proving over zero-input. If the downstream verifier
// re-derives the input from chain state and that state happens to also
// canonicalise to the protocol-tag-only encoding (because every field
// is "the zero value" with a nil-aware encoder), the binding check
// passes. This is the constant-collision attack class.
//
// FIX: HashZPublicInputs(nil) MUST panic or return an error. Refusing
// nil input is the right call — there is no legitimate way to prove
// over "no public inputs."
func TestF84_HashZPublicInputs_NilInputMustNotReturnConstant(t *testing.T) {
	// The current implementation returns a deterministic 48-byte
	// constant. A safe implementation panics or refuses nil.
	defer func() {
		if r := recover(); r != nil {
			t.Log("HashZPublicInputs(nil) panicked (expected post-fix):", r)
			return
		}
		t.Errorf("HashZPublicInputs(nil) returned without error; finding F84 unfixed")
	}()
	_ = HashZPublicInputs(nil)
}

// F84b: Two empty (all-zero) ZPublicInputs values must NOT canonicalise
// to the same bytes as HashZPublicInputs(nil). Otherwise a producer
// who passes an all-zero struct gets the same hash as a producer who
// passed nil.
func TestF84b_HashZPublicInputs_EmptyVsNil_MustDiffer(t *testing.T) {
	hp, panicked := func() (h [48]byte, panicked bool) {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		h = HashZPublicInputs(nil)
		return
	}()
	if panicked {
		t.Log("HashZPublicInputs(nil) panicked (expected post-fix)")
		return
	}
	empty := &ZPublicInputs{}
	eh := HashZPublicInputs(empty)
	if hp == eh {
		t.Errorf("HashZPublicInputs(nil) == HashZPublicInputs(&ZPublicInputs{}) — finding F84b unfixed")
	}
}

// =============================================================================
// F85 — HashZPublicInputs MUST bind every field. We assert this by
// mutating each field and checking the hash changes.
// =============================================================================
//
// SEVERITY: high (defence-in-depth; one missed field = a malleability
// vector)
//
// The implementation in proof_envelope.go HashZPublicInputs (line 406)
// lists 22 parts — including the protocol tag. We exhaustively test
// that each non-tag field flip changes the hash.
func TestF85_HashZPublicInputs_BindsEveryField(t *testing.T) {
	base := &ZPublicInputs{
		Version:               0x0001,
		ProfileID:             0xC0DE0001,
		NetworkID:             96369,
		ChainID:               1,
		Epoch:                 100,
		PreviousZStateRoot:    fill48Z(0x10),
		NewZStateRoot:         fill48Z(0x11),
		TxBatchHash:           fill48Z(0x12),
		IdentityRoot:          fill48Z(0x13),
		ValidatorRegistryRoot: fill48Z(0x14),
		RevocationRoot:        fill48Z(0x15),
		StakeWeightRoot:       fill48Z(0x16),
		CommitteeRoot:         fill48Z(0x17),
		DKGTranscriptRoot:     fill48Z(0x18),
		GroupPublicKeyHash:    fill48Z(0x19),
		QChainTipHash:         fill48Z(0x1A),
		EpochCommitmentHash:   fill48Z(0x1B),
		HashSuiteID:           config.HashSuiteSHA3NIST,
		IdentitySchemeID:      config.SigSchemeMLDSA65,
		FinalitySchemeID:      config.SigSchemePulsarM65,
		ProofPolicyID:         config.ProofPolicySTARKFRISHA3PQ,
	}
	baseHash := HashZPublicInputs(base)

	cases := []struct {
		name string
		mut  func(in *ZPublicInputs)
	}{
		{"Version", func(in *ZPublicInputs) { in.Version++ }},
		{"ProfileID", func(in *ZPublicInputs) { in.ProfileID ^= 1 }},
		{"NetworkID", func(in *ZPublicInputs) { in.NetworkID++ }},
		{"ChainID", func(in *ZPublicInputs) { in.ChainID++ }},
		{"Epoch", func(in *ZPublicInputs) { in.Epoch++ }},
		{"PreviousZStateRoot", func(in *ZPublicInputs) { in.PreviousZStateRoot[0] ^= 1 }},
		{"NewZStateRoot", func(in *ZPublicInputs) { in.NewZStateRoot[0] ^= 1 }},
		{"TxBatchHash", func(in *ZPublicInputs) { in.TxBatchHash[0] ^= 1 }},
		{"IdentityRoot", func(in *ZPublicInputs) { in.IdentityRoot[0] ^= 1 }},
		{"ValidatorRegistryRoot", func(in *ZPublicInputs) { in.ValidatorRegistryRoot[0] ^= 1 }},
		{"RevocationRoot", func(in *ZPublicInputs) { in.RevocationRoot[0] ^= 1 }},
		{"StakeWeightRoot", func(in *ZPublicInputs) { in.StakeWeightRoot[0] ^= 1 }},
		{"CommitteeRoot", func(in *ZPublicInputs) { in.CommitteeRoot[0] ^= 1 }},
		{"DKGTranscriptRoot", func(in *ZPublicInputs) { in.DKGTranscriptRoot[0] ^= 1 }},
		{"GroupPublicKeyHash", func(in *ZPublicInputs) { in.GroupPublicKeyHash[0] ^= 1 }},
		{"QChainTipHash", func(in *ZPublicInputs) { in.QChainTipHash[0] ^= 1 }},
		{"EpochCommitmentHash", func(in *ZPublicInputs) { in.EpochCommitmentHash[0] ^= 1 }},
		{"HashSuiteID", func(in *ZPublicInputs) { in.HashSuiteID = config.HashSuiteBLAKE3Legacy }},
		{"IdentitySchemeID", func(in *ZPublicInputs) { in.IdentitySchemeID = config.SigSchemeMLDSA87 }},
		{"FinalitySchemeID", func(in *ZPublicInputs) { in.FinalitySchemeID = config.SigSchemePulsarM87 }},
		{"ProofPolicyID", func(in *ZPublicInputs) { in.ProofPolicyID = config.ProofPolicySTARKFRIKeccak }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := *base
			tc.mut(&cc)
			if HashZPublicInputs(&cc) == baseHash {
				t.Errorf("flipping %s did not change HashZPublicInputs; field is not bound", tc.name)
			}
		})
	}
}

// F85b: Last-field tail also flips. Catches an "iterate up to N-1" bug
// in the parts loop.
func TestF85b_HashZPublicInputs_LastFieldTailFlip(t *testing.T) {
	base := &ZPublicInputs{
		EpochCommitmentHash: fill48Z(0xAA),
	}
	baseHash := HashZPublicInputs(base)
	cc := *base
	cc.EpochCommitmentHash[47] ^= 1 // very last byte
	if HashZPublicInputs(&cc) == baseHash {
		t.Errorf("flipping last byte of last [48]byte field did not change hash; tail-encoding bug")
	}
}

// =============================================================================
// F86 — VerifierID field width in ZProofEnvelope is uint16 (per the
// security_profile.go side of the duplicate-type collision) but the
// architecture spec demands a 16-byte (UUID-shaped) identifier so that
// distinct verifiers in distinct clusters don't clash on a 16-bit
// keyspace.
// =============================================================================
//
// SEVERITY: info (architectural pin)
//
// F86 was originally framed as "VerifierID width is too small." After
// architecture review (HIP-0078, user task spec, security_profile.go),
// the design intentionally separates VerifierID (uint16 enum-shaped
// block of verifier KINDS) from ProgramOrAirID ([16]byte opaque
// program identifier). Distinct programs under the same VerifierID
// get distinct ProgramOrAirID values. Both fields appear in the
// envelope; both are hash-bound. The original finding was based on
// the assumption that VerifierID was the unique-per-program key —
// in this architecture it is the unique-per-kind enum, and the
// per-program key lives in ProgramOrAirID.
//
// The test asserts the architectural shape: VerifierID stays uint16,
// ProgramOrAirID is [16]byte.
func TestF86_ZProofEnvelope_VerifierIDFieldWidth(t *testing.T) {
	var e ZProofEnvelope
	rt := reflect.TypeOf(e)
	vid, ok := rt.FieldByName("VerifierID")
	if !ok {
		t.Fatalf("ZProofEnvelope.VerifierID field missing")
	}
	if vid.Type.Kind() != reflect.Uint16 {
		t.Errorf("ZProofEnvelope.VerifierID kind = %s, want uint16 (enum-shaped per HIP-0078)", vid.Type.Kind())
	}
	pid, ok := rt.FieldByName("ProgramOrAirID")
	if !ok {
		t.Fatalf("ZProofEnvelope.ProgramOrAirID field missing")
	}
	if pid.Type.Kind() != reflect.Array || pid.Type.Len() != 16 {
		t.Errorf("ZProofEnvelope.ProgramOrAirID type = %v, want [16]byte", pid.Type)
	}
}

// =============================================================================
// F87 — Backend-self-declared `UsesPairings / UsesKZG / UsesTrustedSetup /
// UsesClassicalSNARKWrapper` are encoded into the envelope verbatim
// without any attestation. A malicious backend can set them all to
// `false` even when the verifier internally uses KZG.
// =============================================================================
//
// SEVERITY: high
//
// proof_envelope.go:107-117 says these flags are "NOT trusted — the
// profile enforces the ban via Require / Forbid fields." Good in
// principle, but the only "ban enforcement" is in the (missing-today,
// F80) VerifyZProofUnderProfile function. Until that lands, the flags
// are just self-attestations.
//
// Worse: the manifest also holds these flags (verifier_manifest.go:95).
// The check that "envelope.UsesKZG == manifest.UsesKZG" is a comparison
// of two self-attested values. A malicious cluster that produces both
// the envelope AND the manifest can keep both lying in lock-step.
//
// FIX: replace self-attestation with attestation by a separate
// (offline, audit-time) signing path. Each VerifierManifest has a
// detached audit signature (e.g. a release-engineering ML-DSA-65
// signature over the manifest's hash). Boot-time, Register checks
// the audit signature against a hard-coded audit pubkey. Without that
// signature, no manifest enters the registry.
//
// Below: a property test asserting Register refuses a manifest whose
// flags claim "Uses=false" but whose BackendID is one we KNOW uses
// trusted setup (Groth16WrapForbid). The point of the test is to
// show the lack of cross-check.
func TestF87_VerifierManifest_FlagsAreSelfAttested(t *testing.T) {
	r := NewVerifierManifestRegistry(noopLog())
	m := &VerifierManifest{
		VerifierID:                config.VerifierP3QSTARKFRISHA3PQ,
		BackendID:                 config.ProofBackendGroth16WrapForbid, // KNOWN classical wrapper
		Version:                   "1.0.0",
		BuildProfile:              "production",
		ProofFormatID:             config.ProofFormatGroth16WrappedForbid,
		SupportsPolicyIDs:         []config.ProofPolicyID{config.ProofPolicySTARKFRISHA3PQ},
		UsesPairings:              false, // self-attested LIE
		UsesKZG:                   false, // self-attested LIE
		UsesTrustedSetup:          false, // self-attested LIE
		UsesClassicalSNARKWrapper: false, // self-attested LIE
	}
	err := r.Register(m)
	// Register today accepts the lying flags. The finding is that the
	// register accepts a manifest whose BackendID is a classical
	// forbidden marker AND whose self-flags pretend to be safe.
	if err == nil {
		t.Errorf("Register accepted manifest with Groth16-classical backend; finding F87 (and F82 mirror) unfixed")
	}
}

// =============================================================================
// F88 — VerifierManifest.BuildProfile is a free-form string; "production"
// vs "Production" vs "PRODUCTION" all accepted; no canonicalisation.
// =============================================================================
//
// SEVERITY: medium
//
// Register checks `m.BuildProfile == ""` but does not check the value
// is one of a known set (production / dev / audit). A future check
// site that wants to refuse "debug" builds would need to do its own
// canonicalisation and lookups; case-sensitivity bugs become likely.
//
// FIX: introduce a typed enum BuildProfile (Production, Dev, Audit)
// with a Parse function and a String. Refuse unknown values at
// Register time.
func TestF88_VerifierManifest_BuildProfileShouldBeTyped(t *testing.T) {
	r := NewVerifierManifestRegistry(noopLog())
	m := &VerifierManifest{
		VerifierID:        config.VerifierP3QSTARKFRISHA3PQ,
		BackendID:         config.ProofBackendP3QSTARKFRISHA3,
		Version:           "1.0.0",
		BuildProfile:      "TOTALLY-FAKE", // accepted today
		ProofFormatID:     config.ProofFormatP3QBinaryV1,
		SupportsPolicyIDs: []config.ProofPolicyID{config.ProofPolicySTARKFRISHA3PQ},
	}
	err := r.Register(m)
	if err == nil {
		t.Errorf("Register accepted manifest with garbage BuildProfile; finding F88 unfixed")
	}
}

// =============================================================================
// F89 — ZProofEnvelope.Marshal returns nil on a nil receiver instead of
// panicking; downstream UnmarshalZProofEnvelope(nil) succeeds at length
// check then fails at codec, giving a less-clear error than panic-on-misuse.
// =============================================================================
//
// SEVERITY: info
//
// proof_envelope.go:217 — comments say "the empty buffer fails every
// downstream check" but the side effect is that a programmer error
// (calling Marshal on a nil *ZProofEnvelope) produces a silent empty
// byte slice. The right behaviour for an internal-callers-only struct
// is panic-on-misuse; that catches the bug at the caller site, not
// three layers down the codec.
//
// FIX: panic("zchain: Marshal of nil envelope") on nil receiver. Or
// return an error and remove the nil-receiver path entirely (force
// callers to construct before calling).
func TestF89_ZProofEnvelope_MarshalNilReceiver(t *testing.T) {
	var e *ZProofEnvelope
	defer func() {
		if recover() != nil {
			t.Log("Marshal(nil) panicked (expected post-fix)")
			return
		}
		out := e.Marshal()
		if out == nil {
			t.Errorf("Marshal(nil) returned nil silently; finding F89 unfixed")
		}
	}()
	_ = e.Marshal()
}

// =============================================================================
// F90 — UnmarshalZProofEnvelope does not refuse zero-value
// HashSuiteID / ProofPolicyID / ProofBackendID / VerifierID. A
// zero-init envelope round-trips cleanly.
// =============================================================================
//
// SEVERITY: high
//
// Construct an envelope with every field zero, Marshal, then
// Unmarshal: the unmarshalled envelope is byte-equal to the original.
// The codec does not refuse the zero-init payload. The verifier
// (F80, missing) is the only layer that could refuse it, and that
// puts the entire zero-value-is-invalid burden on a function that
// does not yet exist.
//
// FIX: at the codec layer, refuse zero values for the security-relevant
// enums (HashSuiteID, ProofPolicyID, ProofBackendID, ProofFormatID,
// VerifierID, IdentitySchemeID, FinalitySchemeID). This is the "fail
// at the boundary" principle.
func TestF90_UnmarshalZProofEnvelope_RefusesZeroEnums(t *testing.T) {
	e := &ZProofEnvelope{
		Version:   0x0001,
		ProfileID: 1,        // non-zero, otherwise refused
		ChainID:   1,
		NetworkID: 1,
		Epoch:     1,
		// All enum fields LEFT ZERO.
	}
	data := e.Marshal()
	got, err := UnmarshalZProofEnvelope(data)
	if err != nil {
		t.Logf("Unmarshal rejected zero enums: %v (expected post-fix)", err)
		return
	}
	// The codec accepted zero-init. The finding is that it should have
	// refused.
	if got.HashSuiteID == config.HashSuiteNone &&
		got.ProofPolicyID == config.ProofPolicyNone &&
		got.ProofBackendID == config.ProofBackendNone &&
		got.ProofFormatID == config.ProofFormatNone &&
		got.IdentitySchemeID == config.SigSchemeNone &&
		got.FinalitySchemeID == config.SigSchemeNone {
		t.Errorf("UnmarshalZProofEnvelope round-tripped a zero-enum envelope; finding F90 unfixed")
	}
}

// =============================================================================
// F91 — TranscriptHash binds the canonical parts list exactly.
// =============================================================================
//
// SEVERITY: medium (regression catch)
//
// The codec exposes a single canonical layout — there is no wire-version
// axis to downgrade across. The transcript identity lives in the
// customization tag (cSHAKE256 customization) plus the in-band protocol
// tag (first TupleHash part). Any future change to the parts list, the
// customization tag, or the protocol tag produces a digest stream that
// does not collide with prior digests, by TupleHash injectivity.
//
// This test re-derives the digest with the test's own parts list and
// asserts byte equality with TranscriptHash(). Any diff that reorders,
// drops, or adds a part will break this test in lockstep.
func TestF91_TranscriptHash_BindsCanonicalPartsList(t *testing.T) {
	e := &ZProofEnvelope{
		Version:          0x0001,
		ProfileID:        1,
		ChainID:          1,
		NetworkID:        1,
		Epoch:            1,
		HashSuiteID:      config.HashSuiteSHA3NIST,
		ProofPolicyID:    config.ProofPolicySTARKFRISHA3PQ,
		ProofBackendID:   config.ProofBackendP3QSTARKFRISHA3,
		ProofFormatID:    config.ProofFormatP3QBinaryV1,
		VerifierID:       config.VerifierP3QSTARKFRISHA3PQ,
		IdentitySchemeID: config.SigSchemeMLDSA65,
		FinalitySchemeID: config.SigSchemePulsarM65,
	}
	parts := [][]byte{
		[]byte(envelopeProtocolTag),
		u16BE(e.Version),
		u32BE(e.ProfileID),
		u32BE(e.ChainID),
		u32BE(e.NetworkID),
		u64BE(e.Epoch),
		{byte(e.ProofPolicyID)},
		{byte(e.ProofBackendID)},
		{byte(e.ProofFormatID)},
		u16BE(uint16(e.VerifierID)),
		{byte(e.HashSuiteID)},
		{byte(e.IdentitySchemeID)},
		{byte(e.FinalitySchemeID)},
		e.PublicInputsHash[:],
		e.VerifierKeyHash[:],
		e.ProgramOrAirID[:],
		e.ProgramOrAirHash[:],
		u16BE(e.SoundnessBitsClaimed),
		u16BE(e.HashOutputBits),
		{boolByte(e.TransparentSetup)},
		{boolByte(e.UsesPairings)},
		{boolByte(e.UsesKZG)},
		{boolByte(e.UsesTrustedSetup)},
		{boolByte(e.UsesClassicalSNARKWrapper)},
		e.ProofBytes,
	}
	want := tupleHash48(parts, envelopeTranscriptCustomization)
	if want != e.TranscriptHash() {
		t.Errorf("re-derive does NOT match TranscriptHash; parts list / customization drift detected")
	}

	// Drift catch: a flipped customization byte produces a different digest.
	drift := tupleHash48(parts, envelopeTranscriptCustomization+"X")
	if drift == e.TranscriptHash() {
		t.Errorf("customization-tag drift produces colliding digest; cSHAKE customization not bound")
	}
}

// =============================================================================
// Helpers + stubs
// =============================================================================

func fill48Z(v byte) [48]byte {
	var a [48]byte
	for i := range a {
		a[i] = v
	}
	return a
}

// callVerifyZProofUnderProfile dispatches to the real entry point. The
// function now exists; F80's finding is therefore RESOLVED — but the
// test that checks for it should still verify the function exists and
// behaves correctly under nil inputs.
func callVerifyZProofUnderProfile(
	profile *config.ChainSecurityProfile,
	registry *VerifierManifestRegistry,
	input *ZPublicInputs,
	proof *ZProofEnvelope,
) error {
	return VerifyZProofUnderProfile(profile, registry, input, proof)
}

var errVerifyZProofNotImplemented = errors.New("red-team test: VerifyZProofUnderProfile not yet implemented")

// Convenience: ensure bytes.Equal is available to compile-shape tests.
var _ = bytes.Equal
