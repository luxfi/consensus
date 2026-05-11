// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"errors"
	"testing"
)

// TestBridgeProfileID_String pins the wire-name table so a rename is
// immediately visible in CI.
func TestBridgeProfileID_String(t *testing.T) {
	cases := []struct {
		id   BridgeProfileID
		want string
	}{
		{BridgeProfileIDNone, "none"},
		{BridgeProfileIDLuxStrictPQ, "lux-strict-pq-bridge"},
		{BridgeProfileIDClassicalCompat, "bridge-classical-compat-unsafe"},
	}
	for _, c := range cases {
		if got := c.id.String(); got != c.want {
			t.Errorf("BridgeProfileID(0x%x).String() = %q, want %q", uint32(c.id), got, c.want)
		}
	}
	unknown := BridgeProfileID(0x1234)
	if got := unknown.String(); got != "bridge-profile(0x00001234)" {
		t.Errorf("unknown BridgeProfileID.String() = %q, want bridge-profile(0x00001234)", got)
	}
}

// TestLuxStrictPQBridgeProfile_Fields pins every field of the canonical
// strict-PQ bridge profile. Any rename / renumbering trips this test.
func TestLuxStrictPQBridgeProfile_Fields(t *testing.T) {
	p := LuxStrictPQBridgeProfile
	if p.ProfileID != 0x01 {
		t.Errorf("ProfileID = 0x%x, want 0x01", p.ProfileID)
	}
	if p.Name != "LUX_STRICT_PQ_BRIDGE" {
		t.Errorf("Name = %q, want LUX_STRICT_PQ_BRIDGE", p.Name)
	}
	if p.SourceFinalityScheme != SigSchemePulsarM65 {
		t.Errorf("SourceFinalityScheme = %s, want pulsar-m-65", p.SourceFinalityScheme)
	}
	if p.DestFinalityScheme != SigSchemePulsarM65 {
		t.Errorf("DestFinalityScheme = %s, want pulsar-m-65", p.DestFinalityScheme)
	}
	if p.ProofPolicyID != ProofPolicySTARKFRISHA3PQ {
		t.Errorf("ProofPolicyID = %s, want stark-fri-sha3-pq", p.ProofPolicyID)
	}
	if p.BridgeAdminScheme != ContractAuthMLDSA87 {
		t.Errorf("BridgeAdminScheme = %s, want ml-dsa-87", p.BridgeAdminScheme)
	}
	if p.BridgePauseScheme != ContractAuthMultisigMLDSA {
		t.Errorf("BridgePauseScheme = %s, want multisig-ml-dsa", p.BridgePauseScheme)
	}
	if p.HashSuiteID != HashSuiteSHA3NIST {
		t.Errorf("HashSuiteID = %s, want sha3-nist", p.HashSuiteID)
	}
	if !p.PostQuantumEndToEnd {
		t.Errorf("PostQuantumEndToEnd = false, want true")
	}
}

// TestBridgeProfile_LuxStrictPQ_AllForbidBitsTrue — every classical-allow
// gate on the canonical strict-PQ profile MUST be false. The test name
// matches the spec: pinning all `Allows*` to false is what makes the
// profile E2E-PQ.
func TestBridgeProfile_LuxStrictPQ_AllForbidBitsTrue(t *testing.T) {
	p := LuxStrictPQBridgeProfile
	if p.AllowsClassicalAdmin {
		t.Errorf("AllowsClassicalAdmin = true, want false on strict-PQ")
	}
	if p.AllowsBLSAggregate {
		t.Errorf("AllowsBLSAggregate = true, want false on strict-PQ")
	}
	if p.AllowsKZGCommitment {
		t.Errorf("AllowsKZGCommitment = true, want false on strict-PQ")
	}
	if p.AllowsGroth16Wrap {
		t.Errorf("AllowsGroth16Wrap = true, want false on strict-PQ")
	}
	if p.AllowsPairingPrecompile {
		t.Errorf("AllowsPairingPrecompile = true, want false on strict-PQ")
	}
}

// TestBridgeClassicalCompat_Fields pins every field of the
// classical-compat profile. The label MUST advertise PQ=false; a
// strict-PQ verifier MUST refuse classical-compat deposits via this
// gate (callers).
func TestBridgeClassicalCompat_Fields(t *testing.T) {
	p := BridgeClassicalCompat
	if p.ProfileID != 0x90 {
		t.Errorf("ProfileID = 0x%x, want 0x90", p.ProfileID)
	}
	if p.Name != "BRIDGE_CLASSICAL_COMPAT_UNSAFE" {
		t.Errorf("Name = %q, want BRIDGE_CLASSICAL_COMPAT_UNSAFE", p.Name)
	}
	if p.PostQuantumEndToEnd {
		t.Errorf("PostQuantumEndToEnd = true, want false on classical-compat")
	}
	if !p.AllowsClassicalAdmin {
		t.Errorf("AllowsClassicalAdmin = false, want true on classical-compat")
	}
	if !p.AllowsBLSAggregate {
		t.Errorf("AllowsBLSAggregate = false, want true on classical-compat")
	}
	if !p.AllowsKZGCommitment {
		t.Errorf("AllowsKZGCommitment = false, want true on classical-compat")
	}
	if !p.AllowsGroth16Wrap {
		t.Errorf("AllowsGroth16Wrap = false, want true on classical-compat")
	}
	if !p.AllowsPairingPrecompile {
		t.Errorf("AllowsPairingPrecompile = false, want true on classical-compat")
	}
}

// TestBridgeProfile_PostQuantumEndToEnd_Inference — E2E PQ is true ONLY
// when both source+dest are PQ finality AND every Allows* gate is false
// AND every contract-auth/proof-policy is PQ-positive. The matrix below
// exhaustively walks the one-bit-off cases.
func TestBridgeProfile_PostQuantumEndToEnd_Inference(t *testing.T) {
	// Baseline: the canonical strict-PQ profile — every field flipped to
	// its strict-PQ value. computeImpliedPQ MUST return true.
	base := func() BridgeProfile {
		p := LuxStrictPQBridgeProfile
		return p
	}

	b := base()
	if !b.computeImpliedPQ() {
		t.Fatalf("baseline strict-PQ profile computeImpliedPQ() = false, want true")
	}

	// Each subtest flips exactly one field to a classical / non-PQ
	// value and asserts the inference flips to false.
	t.Run("flip_source_finality_to_BLS", func(t *testing.T) {
		p := base()
		p.SourceFinalityScheme = SigSchemeBLS12381
		if p.computeImpliedPQ() {
			t.Errorf("source=BLS classical, computeImpliedPQ() = true, want false")
		}
	})
	t.Run("flip_dest_finality_to_BLS", func(t *testing.T) {
		p := base()
		p.DestFinalityScheme = SigSchemeBLS12381
		if p.computeImpliedPQ() {
			t.Errorf("dest=BLS classical, computeImpliedPQ() = true, want false")
		}
	})
	t.Run("flip_proof_policy_to_Groth16", func(t *testing.T) {
		p := base()
		p.ProofPolicyID = ProofPolicyGroth16BN254Forbid
		if p.computeImpliedPQ() {
			t.Errorf("policy=Groth16, computeImpliedPQ() = true, want false")
		}
	})
	t.Run("flip_proof_policy_to_KZG", func(t *testing.T) {
		p := base()
		p.ProofPolicyID = ProofPolicyPLONKKZGForbid
		if p.computeImpliedPQ() {
			t.Errorf("policy=KZG, computeImpliedPQ() = true, want false")
		}
	})
	t.Run("flip_admin_scheme_to_ECDSA", func(t *testing.T) {
		p := base()
		p.BridgeAdminScheme = ContractAuthECDSAUnsafe
		if p.computeImpliedPQ() {
			t.Errorf("admin=ECDSA, computeImpliedPQ() = true, want false")
		}
	})
	t.Run("flip_pause_scheme_to_BLS", func(t *testing.T) {
		p := base()
		p.BridgePauseScheme = ContractAuthBLSUnsafe
		if p.computeImpliedPQ() {
			t.Errorf("pause=BLS, computeImpliedPQ() = true, want false")
		}
	})
	t.Run("flip_allows_classical_admin", func(t *testing.T) {
		p := base()
		p.AllowsClassicalAdmin = true
		if p.computeImpliedPQ() {
			t.Errorf("AllowsClassicalAdmin=true, computeImpliedPQ() = true, want false")
		}
	})
	t.Run("flip_allows_bls_aggregate", func(t *testing.T) {
		p := base()
		p.AllowsBLSAggregate = true
		if p.computeImpliedPQ() {
			t.Errorf("AllowsBLSAggregate=true, computeImpliedPQ() = true, want false")
		}
	})
	t.Run("flip_allows_kzg", func(t *testing.T) {
		p := base()
		p.AllowsKZGCommitment = true
		if p.computeImpliedPQ() {
			t.Errorf("AllowsKZGCommitment=true, computeImpliedPQ() = true, want false")
		}
	})
	t.Run("flip_allows_groth16", func(t *testing.T) {
		p := base()
		p.AllowsGroth16Wrap = true
		if p.computeImpliedPQ() {
			t.Errorf("AllowsGroth16Wrap=true, computeImpliedPQ() = true, want false")
		}
	})
	t.Run("flip_allows_pairing", func(t *testing.T) {
		p := base()
		p.AllowsPairingPrecompile = true
		if p.computeImpliedPQ() {
			t.Errorf("AllowsPairingPrecompile=true, computeImpliedPQ() = true, want false")
		}
	})
}

// TestBridgeProfile_Validate_LuxStrictPQRejectsBLS — a profile that
// declares PQ=true but pins BLS as a finality scheme MUST be rejected by
// Validate.
func TestBridgeProfile_Validate_LuxStrictPQRejectsBLS(t *testing.T) {
	p := LuxStrictPQBridgeProfile
	p.SourceFinalityScheme = SigSchemeBLS12381
	err := p.Validate()
	if err == nil {
		t.Fatalf("Validate() = nil, want classical-under-strict refusal")
	}
	if !errors.Is(err, ErrBridgeProfileClassicalUnderStrict) {
		t.Errorf("err = %v, want ErrBridgeProfileClassicalUnderStrict", err)
	}
}

// TestBridgeProfile_Validate_LuxStrictPQRejectsKZG — same, KZG flag.
func TestBridgeProfile_Validate_LuxStrictPQRejectsKZG(t *testing.T) {
	p := LuxStrictPQBridgeProfile
	p.AllowsKZGCommitment = true
	err := p.Validate()
	if err == nil {
		t.Fatalf("Validate() = nil, want classical-under-strict refusal")
	}
	if !errors.Is(err, ErrBridgeProfileClassicalUnderStrict) {
		t.Errorf("err = %v, want ErrBridgeProfileClassicalUnderStrict", err)
	}
}

// TestBridgeProfile_Validate_LuxStrictPQRejectsGroth16 — same, Groth16 flag.
func TestBridgeProfile_Validate_LuxStrictPQRejectsGroth16(t *testing.T) {
	p := LuxStrictPQBridgeProfile
	p.AllowsGroth16Wrap = true
	err := p.Validate()
	if err == nil {
		t.Fatalf("Validate() = nil, want classical-under-strict refusal")
	}
	if !errors.Is(err, ErrBridgeProfileClassicalUnderStrict) {
		t.Errorf("err = %v, want ErrBridgeProfileClassicalUnderStrict", err)
	}
}

// TestBridgeProfile_Validate_StrictPQRejectsClassicalProofPolicy — a
// profile that declares PQ=true but pins a Groth16 ProofPolicy MUST be
// rejected. This covers the proof-policy axis distinctly from the
// Allows* flags above.
func TestBridgeProfile_Validate_StrictPQRejectsClassicalProofPolicy(t *testing.T) {
	p := LuxStrictPQBridgeProfile
	p.ProofPolicyID = ProofPolicyGroth16BN254Forbid
	err := p.Validate()
	if err == nil {
		t.Fatalf("Validate() = nil, want classical-under-strict refusal")
	}
	if !errors.Is(err, ErrBridgeProfileClassicalUnderStrict) {
		t.Errorf("err = %v, want ErrBridgeProfileClassicalUnderStrict", err)
	}
}

// TestBridgeProfile_Validate_RejectsZeroProfileID — the zero value is
// not a valid bridge profile.
func TestBridgeProfile_Validate_RejectsZeroProfileID(t *testing.T) {
	p := BridgeProfile{}
	err := p.Validate()
	if err == nil {
		t.Fatalf("Validate() = nil, want field-unset error")
	}
	if !errors.Is(err, ErrBridgeProfileFieldUnset) {
		t.Errorf("err = %v, want ErrBridgeProfileFieldUnset", err)
	}
}

// TestBridgeProfile_Validate_RejectsMislabelledClassicalAsStrict — a
// classical-compat profile cannot pretend to be strict-PQ. A profile
// with classical fields but PostQuantumEndToEnd=true MUST be rejected.
func TestBridgeProfile_Validate_RejectsMislabelledClassicalAsStrict(t *testing.T) {
	p := BridgeClassicalCompat
	p.PostQuantumEndToEnd = true // operator lies about posture
	err := p.Validate()
	if err == nil {
		t.Fatalf("Validate() = nil, want refusal — operator cannot mislabel classical bridge as strict-PQ")
	}
	if !errors.Is(err, ErrBridgeProfileClassicalUnderStrict) {
		t.Errorf("err = %v, want ErrBridgeProfileClassicalUnderStrict", err)
	}
}

// TestBridgeProfile_Validate_RejectsMislabelledStrictAsClassical — the
// inverse: a strict-PQ profile cannot be labelled non-PQ; operators
// must use the explicit classical-compat profile.
func TestBridgeProfile_Validate_RejectsMislabelledStrictAsClassical(t *testing.T) {
	p := LuxStrictPQBridgeProfile
	p.PostQuantumEndToEnd = false
	err := p.Validate()
	if err == nil {
		t.Fatalf("Validate() = nil, want refusal")
	}
	if !errors.Is(err, ErrBridgeProfileFieldInvalid) {
		t.Errorf("err = %v, want ErrBridgeProfileFieldInvalid", err)
	}
}

// TestBridgeProfile_RefuseClassicalAdmin_StrictPQRefuses — calling
// RefuseClassicalAdmin under the strict-PQ profile MUST return an
// error; under classical-compat it MUST return nil.
func TestBridgeProfile_RefuseClassicalAdmin_StrictPQRefuses(t *testing.T) {
	p := LuxStrictPQBridgeProfile
	if err := p.RefuseClassicalAdmin(); err == nil {
		t.Errorf("strict-PQ RefuseClassicalAdmin() = nil, want refusal")
	} else if !errors.Is(err, ErrBridgeProfileForbidden) {
		t.Errorf("err = %v, want ErrBridgeProfileForbidden", err)
	}

	compat := BridgeClassicalCompat
	if err := compat.RefuseClassicalAdmin(); err != nil {
		t.Errorf("classical-compat RefuseClassicalAdmin() = %v, want nil", err)
	}
}

// TestBridgeProfile_RefuseBLSAggregate_StrictPQRefuses — same shape for
// BLS aggregates.
func TestBridgeProfile_RefuseBLSAggregate_StrictPQRefuses(t *testing.T) {
	p := LuxStrictPQBridgeProfile
	if err := p.RefuseBLSAggregate(); err == nil {
		t.Errorf("strict-PQ RefuseBLSAggregate() = nil, want refusal")
	} else if !errors.Is(err, ErrBridgeProfileForbidden) {
		t.Errorf("err = %v, want ErrBridgeProfileForbidden", err)
	}

	compat := BridgeClassicalCompat
	if err := compat.RefuseBLSAggregate(); err != nil {
		t.Errorf("classical-compat RefuseBLSAggregate() = %v, want nil", err)
	}
}

// TestBridgeProfile_RefuseKZGCommitment_StrictPQRefuses — KZG.
func TestBridgeProfile_RefuseKZGCommitment_StrictPQRefuses(t *testing.T) {
	p := LuxStrictPQBridgeProfile
	if err := p.RefuseKZGCommitment(); err == nil {
		t.Errorf("strict-PQ RefuseKZGCommitment() = nil, want refusal")
	} else if !errors.Is(err, ErrBridgeProfileForbidden) {
		t.Errorf("err = %v, want ErrBridgeProfileForbidden", err)
	}

	compat := BridgeClassicalCompat
	if err := compat.RefuseKZGCommitment(); err != nil {
		t.Errorf("classical-compat RefuseKZGCommitment() = %v, want nil", err)
	}
}

// TestBridgeProfile_RefuseGroth16Wrap_StrictPQRefuses — Groth16.
func TestBridgeProfile_RefuseGroth16Wrap_StrictPQRefuses(t *testing.T) {
	p := LuxStrictPQBridgeProfile
	if err := p.RefuseGroth16Wrap(); err == nil {
		t.Errorf("strict-PQ RefuseGroth16Wrap() = nil, want refusal")
	} else if !errors.Is(err, ErrBridgeProfileForbidden) {
		t.Errorf("err = %v, want ErrBridgeProfileForbidden", err)
	}

	compat := BridgeClassicalCompat
	if err := compat.RefuseGroth16Wrap(); err != nil {
		t.Errorf("classical-compat RefuseGroth16Wrap() = %v, want nil", err)
	}
}

// TestBridgeProfile_RefusePairingPrecompile_StrictPQRefuses — pairing
// precompile (EVM 0x08).
func TestBridgeProfile_RefusePairingPrecompile_StrictPQRefuses(t *testing.T) {
	p := LuxStrictPQBridgeProfile
	if err := p.RefusePairingPrecompile(); err == nil {
		t.Errorf("strict-PQ RefusePairingPrecompile() = nil, want refusal")
	} else if !errors.Is(err, ErrBridgeProfileForbidden) {
		t.Errorf("err = %v, want ErrBridgeProfileForbidden", err)
	}

	compat := BridgeClassicalCompat
	if err := compat.RefusePairingPrecompile(); err != nil {
		t.Errorf("classical-compat RefusePairingPrecompile() = %v, want nil", err)
	}
}

// TestBridgeWithdraw_RefusesECDSAUnderStrictPQ — the spec test name:
// a withdraw call site that gates ecrecover behind RefuseClassicalAdmin
// MUST refuse the operation under the strict-PQ profile. This test
// stands in for the integration site (the actual ecrecover lives in
// the bridge package; the consensus test pins the gate behaviour).
func TestBridgeWithdraw_RefusesECDSAUnderStrictPQ(t *testing.T) {
	// Simulate the bridge code path: a withdraw handler receives an
	// inbound ECDSA signature; it MUST consult RefuseClassicalAdmin
	// before calling ecrecover.
	p := LuxStrictPQBridgeProfile
	if err := p.RefuseClassicalAdmin(); err == nil {
		t.Fatalf("withdraw handler under strict-PQ did not refuse ECDSA — call site would invoke ecrecover")
	}

	// Same path under classical-compat: the gate returns nil, the
	// call site proceeds (and is expected to emit the
	// bridge_classical_compat_total metric, which is a concern of the
	// bridge package, not consensus).
	compat := BridgeClassicalCompat
	if err := compat.RefuseClassicalAdmin(); err != nil {
		t.Fatalf("withdraw handler under classical-compat refused ECDSA, want nil: %v", err)
	}
}

// TestBridgeProfile_NilReceiver — every gate method MUST refuse a nil
// profile rather than panicking. Defence in depth: a misconfigured
// caller that forgot to set the profile pointer hits a typed error,
// not a nil-pointer panic.
func TestBridgeProfile_NilReceiver(t *testing.T) {
	var p *BridgeProfile
	for name, fn := range map[string]func() error{
		"Validate":                p.Validate,
		"RequireAdminPQ":          p.RequireAdminPQ,
		"RefuseClassicalAdmin":    p.RefuseClassicalAdmin,
		"RefuseBLSAggregate":      p.RefuseBLSAggregate,
		"RefuseKZGCommitment":     p.RefuseKZGCommitment,
		"RefuseGroth16Wrap":       p.RefuseGroth16Wrap,
		"RefusePairingPrecompile": p.RefusePairingPrecompile,
	} {
		err := fn()
		if err == nil {
			t.Errorf("nil-receiver %s() = nil, want ErrBridgeProfileNil", name)
		} else if !errors.Is(err, ErrBridgeProfileNil) {
			t.Errorf("nil-receiver %s() = %v, want ErrBridgeProfileNil", name, err)
		}
	}
}

// TestBridgeProfile_RequireAdminPQ_AcceptsPQAdmin — RequireAdminPQ
// returns nil when the profile's BridgeAdminScheme is post-quantum.
func TestBridgeProfile_RequireAdminPQ_AcceptsPQAdmin(t *testing.T) {
	p := LuxStrictPQBridgeProfile
	if err := p.RequireAdminPQ(); err != nil {
		t.Errorf("RequireAdminPQ on strict-PQ profile = %v, want nil", err)
	}
}

// TestBridgeProfile_IsPostQuantumEndToEnd_Accessor — the public accessor
// matches the field. Pins the contract: callers MUST consult this
// rather than re-derive.
func TestBridgeProfile_IsPostQuantumEndToEnd_Accessor(t *testing.T) {
	if !LuxStrictPQBridgeProfile.IsPostQuantumEndToEnd() {
		t.Errorf("strict-PQ IsPostQuantumEndToEnd() = false, want true")
	}
	if BridgeClassicalCompat.IsPostQuantumEndToEnd() {
		t.Errorf("classical-compat IsPostQuantumEndToEnd() = true, want false")
	}
}

// TestBridgeProfile_HelperPredicates pins the local helper predicates
// against representative wire bytes so any renumbering trips CI.
func TestBridgeProfile_HelperPredicates(t *testing.T) {
	// sigSchemeIsPostQuantum
	if !sigSchemeIsPostQuantum(SigSchemePulsarM65) {
		t.Errorf("sigSchemeIsPostQuantum(PulsarM65) = false, want true")
	}
	if !sigSchemeIsPostQuantum(SigSchemeMLDSA65) {
		t.Errorf("sigSchemeIsPostQuantum(MLDSA65) = false, want true")
	}
	if sigSchemeIsPostQuantum(SigSchemeBLS12381) {
		t.Errorf("sigSchemeIsPostQuantum(BLS12381) = true, want false")
	}
	// sigSchemeIsClassical
	if !sigSchemeIsClassical(SigSchemeBLS12381) {
		t.Errorf("sigSchemeIsClassical(BLS12381) = false, want true")
	}
	if sigSchemeIsClassical(SigSchemePulsarM65) {
		t.Errorf("sigSchemeIsClassical(PulsarM65) = true, want false")
	}
}
