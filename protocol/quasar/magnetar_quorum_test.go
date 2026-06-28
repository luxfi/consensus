// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// magnetar_quorum_test.go — proofs for Magnetar-Quorum (Track A): the trustless
// hash-based PQ lane. Each test pins one property of the claim:
//
//   - Independent per-validator FIPS-205 SLH-DSA sigs -> MagnetarQuorumCert ->
//     VerifyMagnetarQuorum passes, and each inner sig verifies under the STOCK
//     FIPS-205 verifier (no Lux machinery in the per-signer check).
//   - Tamper / sub-threshold / root-mismatch / non-hash-based -> hard reject.
//   - The succinct STARK seam fails closed (audit-gated).
//   - Suite dispatch can never cross lanes or downgrade the param set.
//   - The POLARIS_HASH_PQ / POLARIS_MAX postures admit Magnetar ONLY as the
//     trustless rollup, NEVER as a threshold sig.
//   - Structural trustlessness: independent keys, no dealer / shared secret.
//
// Reuses the foundation's real-key SLH-DSA harness (newSLHDSASigner /
// buildScenario in quorum_cert_test.go) and the envelope harness (envPolicy /
// envValidators / newCertForBH in consensus_cert_test.go).
package quasar

import (
	"bytes"
	"errors"
	"testing"

	magnetar "github.com/luxfi/magnetar/ref/go/pkg/magnetar"
)

// slhParam is the SLH-DSA-192s parameter byte the Magnetar leg is built under.
const slhParam = uint8(QuorumSchemeSLHDSA192s)

// magnetarDirectSuite is the production Direct SLH-DSA-192s Magnetar suite.
const magnetarDirectSuite = "Lux-Magnetar-SLHDSA192s-Direct-v1"

// twoSLHDSA builds a real 2-signer SLH-DSA-192s scenario (weights 60+60=120,
// threshold 100). Each signer holds an INDEPENDENT FIPS-205 keypair.
func twoSLHDSA(t *testing.T) scenario {
	t.Helper()
	signers := []*testSigner{
		newSLHDSASigner(t, 0x01, 60),
		newSLHDSASigner(t, 0x02, 60),
	}
	return buildScenario(t, signers, 100, nil)
}

// magnetarFixture wraps an SLH-DSA scenario as a complete Magnetar-leg
// ConsensusCert + policy + validator set + the decoded MagnetarQuorumCert.
type magnetarFixture struct {
	sc         scenario
	mc         *MagnetarQuorumCert
	store      *envStore
	validators *envValidators
	policy     *envPolicy
	cert       *ConsensusCert
	msg        []byte
}

// buildMagnetarFixture assembles a Direct Magnetar-Quorum cert over an SLH-DSA
// scenario and the envelope that verifies it (single Magnetar leg required).
func buildMagnetarFixture(t *testing.T, sc scenario) magnetarFixture {
	t.Helper()

	mc, err := BuildMagnetarQuorumCert(magnetarDirectSuite, sc.cert)
	if err != nil {
		t.Fatalf("BuildMagnetarQuorumCert: %v", err)
	}

	magLeg := LegSpec{Kind: LegMagnetarSLHDSA, ParamSetID: slhParam}
	policy := &envPolicy{
		required: []LegSpec{magLeg},
		allow: map[legModeParam]bool{
			{LegMagnetarSLHDSA, EvidenceMagnetarRollup, slhParam}: true,
		},
		thresholdWeight: sc.cert.QuorumThreshold,
		classical:       map[ClassicalScheme]bool{},
	}
	store := &envStore{policy: policy}
	validators := &envValidators{
		root:      sc.cert.ValidatorSetRoot,
		epoch:     sc.env.Epoch,
		cfg:       sc.cfg, // empty AllowedSchemes => SLH-DSA admitted; MinThreshold re-pinned by leg
		env:       sc.env,
		classKeys: map[ClassicalScheme][]byte{},
	}

	cert := newCertForBH(policy, sc.cert.ChainID, sc.cert.Epoch, sc.cert.Height, sc.cert.Round,
		sc.cert.ValueHash, sc.cert.ValidatorSetRoot,
		[]LegEvidence{{
			Leg:     magLeg,
			Mode:    EvidenceMagnetarRollup,
			Payload: mc.Encode(),
		}})
	cert.AggregateWeight = sc.cert.AggregateWeight

	return magnetarFixture{
		sc:         sc,
		mc:         mc,
		store:      store,
		validators: validators,
		policy:     policy,
		cert:       cert,
		msg:        consensusCertMessage(cert, HashRequiredLegs(policy.RequiredLegs())),
	}
}

// ----------------------------------------------------------------------------
// Happy path — the trustless statement holds.
// ----------------------------------------------------------------------------

// TestMagnetarQuorum_HappyPath proves independent SLH-DSA sigs roll up into a
// MagnetarQuorumCert that VerifyMagnetarQuorum (standalone) and the leg verifier
// both accept.
func TestMagnetarQuorum_HappyPath(t *testing.T) {
	f := buildMagnetarFixture(t, twoSLHDSA(t))

	if err := VerifyMagnetarQuorum(f.policy, f.validators, f.cert, f.msg, f.mc); err != nil {
		t.Fatalf("VerifyMagnetarQuorum rejected a valid cert: %v", err)
	}

	// The LegEvidence dispatch form must agree.
	ev := f.cert.Evidence[0]
	if err := VerifyMagnetarQuorumLeg(f.policy, f.validators, f.cert, f.msg, ev); err != nil {
		t.Fatalf("VerifyMagnetarQuorumLeg rejected a valid leg: %v", err)
	}
}

// TestMagnetarQuorum_EndToEndConsensusCert proves the leg verifies through the
// full policy-gated VerifyConsensusCert envelope (POLARIS-style Magnetar leg).
func TestMagnetarQuorum_EndToEndConsensusCert(t *testing.T) {
	f := buildMagnetarFixture(t, twoSLHDSA(t))
	if err := VerifyConsensusCert(f.store, f.validators, f.cert); err != nil {
		t.Fatalf("VerifyConsensusCert rejected a valid Magnetar-leg cert: %v", err)
	}
}

// TestMagnetarQuorum_StockFIPS205Interop proves the headline trustless claim:
// every signer record verifies under the STOCK FIPS-205 verifier (magnetar's
// thin VerifyCtx over circl/slhdsa) with NO Lux cert machinery. This is what
// makes the lane dealer-free and independently auditable.
func TestMagnetarQuorum_StockFIPS205Interop(t *testing.T) {
	f := buildMagnetarFixture(t, twoSLHDSA(t))

	inner, err := UnmarshalWeightedQuorumCert(f.mc.CertSet)
	if err != nil {
		t.Fatalf("decode inner SLH-DSA cert set: %v", err)
	}
	if len(inner.Signers) == 0 {
		t.Fatal("inner cert carries no signer records")
	}
	for i := range inner.Signers {
		rec := &inner.Signers[i]
		if rec.Scheme.FIPS() != "205" {
			t.Fatalf("record %d is not FIPS-205: scheme %s", i, rec.Scheme)
		}
		pk := &magnetar.PublicKey{Mode: magnetar.ModeM192s, Bytes: rec.PublicKey}
		sig := &magnetar.Signature{Mode: magnetar.ModeM192s, Bytes: rec.Signature}
		// The SLH-DSA leg signs under the EMPTY FIPS-205 context (the round-digest
		// message carries the domain binding) — match contextForScheme.
		if err := magnetar.VerifyCtx(magnetar.ParamsM192s, pk, f.sc.message, nil, sig); err != nil {
			t.Fatalf("record %d: STOCK FIPS-205 verifier rejected: %v", i, err)
		}
	}
}

// ----------------------------------------------------------------------------
// Tamper / threshold rejection.
// ----------------------------------------------------------------------------

// TestMagnetarQuorum_RootMismatchRejected proves a rollup root that does not
// commit to the carried cert set is rejected (the commitment binding).
func TestMagnetarQuorum_RootMismatchRejected(t *testing.T) {
	f := buildMagnetarFixture(t, twoSLHDSA(t))
	bad := *f.mc
	bad.RollupRoot[0] ^= 0xFF
	err := VerifyMagnetarQuorum(f.policy, f.validators, f.cert, f.msg, &bad)
	if !errors.Is(err, ErrMagnetarRootMismatch) {
		t.Fatalf("root-mismatch err = %v, want ErrMagnetarRootMismatch", err)
	}
}

// TestMagnetarQuorum_TamperedSigRejected proves flipping a byte of an inner
// SLH-DSA signature (and re-binding the root over the tampered set, so the
// commitment passes) is caught by the composed stock-FIPS-205 verify.
func TestMagnetarQuorum_TamperedSigRejected(t *testing.T) {
	f := buildMagnetarFixture(t, twoSLHDSA(t))

	inner, err := UnmarshalWeightedQuorumCert(f.mc.CertSet)
	if err != nil {
		t.Fatalf("decode inner: %v", err)
	}
	inner.Signers[0].Signature[10] ^= 0xFF // corrupt a real FIPS-205 signature
	tamperedSet, err := inner.MarshalBinary()
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	bad := &MagnetarQuorumCert{
		SuiteID:    magnetarDirectSuite,
		RollupRoot: MagnetarRollupRoot(magnetarDirectSuite, tamperedSet), // commitment still binds
		CertSet:    tamperedSet,
	}
	err = VerifyMagnetarQuorum(f.policy, f.validators, f.cert, f.msg, bad)
	if !errors.Is(err, ErrQCSigInvalid) {
		t.Fatalf("tampered-sig err = %v, want ErrQCSigInvalid (surfaced from the inner verify)", err)
	}
}

// TestMagnetarQuorum_SubThresholdRejected proves a quorum below the policy
// weight floor is rejected — the BFT threshold is the verifier's, not the
// cert's. Build a 2-signer scenario whose Σweight (120) is below a 200 floor.
func TestMagnetarQuorum_SubThresholdRejected(t *testing.T) {
	// Σweight = 120; quorum threshold 200 -> the inner cert is below threshold.
	signers := []*testSigner{
		newSLHDSASigner(t, 0x01, 60),
		newSLHDSASigner(t, 0x02, 60),
	}
	sc := buildScenario(t, signers, 200, nil) // threshold > Σweight
	f := buildMagnetarFixture(t, sc)
	err := VerifyMagnetarQuorum(f.policy, f.validators, f.cert, f.msg, f.mc)
	if !errors.Is(err, ErrQCBelowThreshold) {
		t.Fatalf("sub-threshold err = %v, want ErrQCBelowThreshold", err)
	}
}

// ----------------------------------------------------------------------------
// Cross-family diversity — the hash-based lane verifies ONLY SLH-DSA.
// ----------------------------------------------------------------------------

// TestMagnetarQuorum_BuildRejectsMLDSA proves the constructor refuses to build a
// Magnetar cert from a lattice (ML-DSA) weighted-quorum cert.
func TestMagnetarQuorum_BuildRejectsMLDSA(t *testing.T) {
	sc := fourMLDSA(t) // a real ML-DSA-65 weighted-quorum cert
	_, err := BuildMagnetarQuorumCert(magnetarDirectSuite, sc.cert)
	if !errors.Is(err, ErrMagnetarNotHashBased) {
		t.Fatalf("build-from-ML-DSA err = %v, want ErrMagnetarNotHashBased", err)
	}
}

// TestMagnetarQuorum_VerifyRejectsMLDSA proves the verifier refuses a Magnetar
// cert whose (well-committed) cert set carries lattice records — a single
// Module-LWE break can never satisfy the hash-based diversity requirement.
func TestMagnetarQuorum_VerifyRejectsMLDSA(t *testing.T) {
	mldsaSc := fourMLDSA(t)
	certSet, err := mldsaSc.cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal ML-DSA cert: %v", err)
	}
	// A hand-crafted Magnetar cert whose root correctly commits to an ML-DSA set.
	mc := &MagnetarQuorumCert{
		SuiteID:    magnetarDirectSuite,
		RollupRoot: MagnetarRollupRoot(magnetarDirectSuite, certSet),
		CertSet:    certSet,
	}
	// Verify against an envelope/validators bound to the ML-DSA scenario so the
	// ONLY thing wrong is the scheme family (not position / merkle / threshold).
	magLeg := LegSpec{Kind: LegMagnetarSLHDSA, ParamSetID: slhParam}
	policy := &envPolicy{
		required:        []LegSpec{magLeg},
		allow:           map[legModeParam]bool{{LegMagnetarSLHDSA, EvidenceMagnetarRollup, slhParam}: true},
		thresholdWeight: mldsaSc.cert.QuorumThreshold,
		classical:       map[ClassicalScheme]bool{},
	}
	validators := &envValidators{
		root: mldsaSc.cert.ValidatorSetRoot, epoch: mldsaSc.env.Epoch,
		cfg: mldsaSc.cfg, env: mldsaSc.env, classKeys: map[ClassicalScheme][]byte{},
	}
	cert := newCertForBH(policy, mldsaSc.cert.ChainID, mldsaSc.cert.Epoch, mldsaSc.cert.Height,
		mldsaSc.cert.Round, mldsaSc.cert.ValueHash, mldsaSc.cert.ValidatorSetRoot, nil)
	err = VerifyMagnetarQuorum(policy, validators, cert, []byte("M"), mc)
	if !errors.Is(err, ErrMagnetarNotHashBased) {
		t.Fatalf("verify-ML-DSA-set err = %v, want ErrMagnetarNotHashBased", err)
	}
}

// ----------------------------------------------------------------------------
// Succinct seam — fails closed until audited.
// ----------------------------------------------------------------------------

// TestMagnetarQuorum_STARKSeamFailsClosed proves the succinct (STARK/FRI) suite
// pins its statement but fails closed; the raw set stays challengeable via
// Direct. Succinctness is the optimization seam, not a trust requirement.
func TestMagnetarQuorum_STARKSeamFailsClosed(t *testing.T) {
	f := buildMagnetarFixture(t, twoSLHDSA(t))
	starkCert := &MagnetarQuorumCert{
		SuiteID:    "Lux-Magnetar-SLHDSA192s-STARK-v1",
		RollupRoot: f.mc.RollupRoot,
		Proof:      []byte("a-succinct-proof-that-must-not-be-trusted-yet"),
	}
	err := VerifyMagnetarQuorum(f.policy, f.validators, f.cert, f.msg, starkCert)
	if !errors.Is(err, ErrMagnetarBackendNotAuditGated) {
		t.Fatalf("STARK-seam err = %v, want ErrMagnetarBackendNotAuditGated", err)
	}
}

// ----------------------------------------------------------------------------
// Suite dispatch safety — never cross lanes / downgrade params.
// ----------------------------------------------------------------------------

// TestMagnetarQuorum_RejectsForeignSuite proves a P3Q ML-DSA suite id cannot be
// dispatched to the Magnetar verifier.
func TestMagnetarQuorum_RejectsForeignSuite(t *testing.T) {
	f := buildMagnetarFixture(t, twoSLHDSA(t))
	foreign := *f.mc
	foreign.SuiteID = "Lux-P3Q-MLDSA65-Direct-v1" // a real suite, wrong lane
	err := VerifyMagnetarQuorum(f.policy, f.validators, f.cert, f.msg, &foreign)
	if !errors.Is(err, ErrSuiteLaneMismatch) {
		t.Fatalf("foreign-suite err = %v, want ErrSuiteLaneMismatch", err)
	}
}

// TestMagnetarQuorum_RejectsUnknownSuite proves an unregistered suite is a hard
// reject (fail closed).
func TestMagnetarQuorum_RejectsUnknownSuite(t *testing.T) {
	f := buildMagnetarFixture(t, twoSLHDSA(t))
	unknown := *f.mc
	unknown.SuiteID = "Lux-Magnetar-SLHDSA999s-Direct-v1"
	err := VerifyMagnetarQuorum(f.policy, f.validators, f.cert, f.msg, &unknown)
	if !errors.Is(err, ErrUnknownSuite) {
		t.Fatalf("unknown-suite err = %v, want ErrUnknownSuite", err)
	}
}

// TestMagnetarQuorum_LegParamPin proves the leg verifier pins the leg's param
// set against the suite: a 256s suite under a 192s leg spec is rejected.
func TestMagnetarQuorum_LegParamPin(t *testing.T) {
	f := buildMagnetarFixture(t, twoSLHDSA(t))
	// Re-build the cert set under the 256s suite id so the root commits, then
	// present it on a 192s leg.
	mismatch := &MagnetarQuorumCert{
		SuiteID:    "Lux-Magnetar-SLHDSA256s-Direct-v1",
		RollupRoot: MagnetarRollupRoot("Lux-Magnetar-SLHDSA256s-Direct-v1", f.mc.CertSet),
		CertSet:    f.mc.CertSet,
	}
	ev := LegEvidence{
		Leg:     LegSpec{Kind: LegMagnetarSLHDSA, ParamSetID: slhParam}, // 192s leg
		Mode:    EvidenceMagnetarRollup,
		Payload: mismatch.Encode(),
	}
	err := VerifyMagnetarQuorumLeg(f.policy, f.validators, f.cert, f.msg, ev)
	if !errors.Is(err, ErrSuiteParamMismatch) {
		t.Fatalf("leg-param-pin err = %v, want ErrSuiteParamMismatch", err)
	}
}

// TestMagnetarQuorum_RejectsWrongLegKind proves the leg verifier refuses a
// non-Magnetar leg kind.
func TestMagnetarQuorum_RejectsWrongLegKind(t *testing.T) {
	f := buildMagnetarFixture(t, twoSLHDSA(t))
	ev := LegEvidence{
		Leg:     LegSpec{Kind: LegPulsarMLDSA, ParamSetID: slhParam}, // wrong kind
		Mode:    EvidenceMagnetarRollup,
		Payload: f.mc.Encode(),
	}
	err := VerifyMagnetarQuorumLeg(f.policy, f.validators, f.cert, f.msg, ev)
	if !errors.Is(err, ErrDisallowedEvidenceMode) {
		t.Fatalf("wrong-leg-kind err = %v, want ErrDisallowedEvidenceMode", err)
	}
}

// ----------------------------------------------------------------------------
// Policy table — POLARIS postures, trustless admission only.
// ----------------------------------------------------------------------------

// TestMagnetarQuorum_PolarisPolicyTiers proves the two POLARIS postures require
// the right leg kinds and admit Magnetar ONLY as the trustless rollup, NEVER as
// a threshold sig (the reveal-and-aggregate exclusion).
func TestMagnetarQuorum_PolarisPolicyTiers(t *testing.T) {
	const thr = 1000

	hash := NewQuasarEvidencePolicy(PolicyPolarisHashPQ, uint8(QuorumSchemeMLDSA65), thr)
	if got := legKinds(hash.RequiredLegs()); !sameKinds(got, []LegKind{LegClassical, LegPulsarMLDSA, LegMagnetarSLHDSA}) {
		t.Fatalf("POLARIS_HASH_PQ legs = %v, want Beam∧Pulsar∧Magnetar", got)
	}

	maxp := NewQuasarEvidencePolicy(PolicyPolarisMax, uint8(QuorumSchemeMLDSA65), thr)
	if got := legKinds(maxp.RequiredLegs()); !sameKinds(got, []LegKind{LegClassical, LegPulsarMLDSA, LegCoronaLattice, LegMagnetarSLHDSA}) {
		t.Fatalf("POLARIS_MAX legs = %v, want Beam∧Pulsar∧Corona∧Magnetar", got)
	}

	magLeg := LegSpec{Kind: LegMagnetarSLHDSA, ParamSetID: slhParam}
	for _, p := range []*QuasarEvidencePolicy{hash, maxp} {
		// Admitted: the trustless rollup under the SLH-DSA param.
		if !p.Allows(magLeg, EvidenceMagnetarRollup, slhParam) {
			t.Fatalf("%s must admit the Magnetar rollup", p.Mode())
		}
		// Forbidden: a threshold-sig over the Magnetar leg (reveal-and-aggregate).
		if p.Allows(magLeg, EvidenceThresholdSig, slhParam) {
			t.Fatalf("%s must NOT admit a Magnetar threshold sig (reveal-and-aggregate TCB)", p.Mode())
		}
		// Forbidden: wrong param set.
		if p.Allows(magLeg, EvidenceMagnetarRollup, uint8(QuorumSchemeMLDSA65)) {
			t.Fatalf("%s must reject a non-SLH-DSA Magnetar param", p.Mode())
		}
	}

	// A non-POLARIS posture must never admit the Magnetar leg at all.
	strict := NewQuasarEvidencePolicy(PolicyStrictQuasar, uint8(QuorumSchemeMLDSA65), thr)
	if strict.Allows(magLeg, EvidenceMagnetarRollup, slhParam) {
		t.Fatal("STRICT_QUASAR must not admit the Magnetar leg")
	}
}

// ----------------------------------------------------------------------------
// Structural trustlessness — independent keys, no dealer / shared secret.
// ----------------------------------------------------------------------------

// TestMagnetarQuorum_NoDealerNoSharedSecret proves the lane is structurally
// dealer-free: two DISJOINT sets of independently-generated validator keypairs
// each produce a valid Magnetar cert with NO shared key material, and the inner
// records expose only public keys + signatures (never a seed / share / group
// key). There is no construction step that forms a shared secret.
func TestMagnetarQuorum_NoDealerNoSharedSecret(t *testing.T) {
	// Two committees with completely independent keypairs (fresh keygen each).
	fA := buildMagnetarFixture(t, twoSLHDSA(t))
	signersB := []*testSigner{
		newSLHDSASigner(t, 0x11, 70),
		newSLHDSASigner(t, 0x12, 70),
	}
	fB := buildMagnetarFixture(t, buildScenario(t, signersB, 100, nil))

	if err := VerifyMagnetarQuorum(fA.policy, fA.validators, fA.cert, fA.msg, fA.mc); err != nil {
		t.Fatalf("committee A rejected: %v", err)
	}
	if err := VerifyMagnetarQuorum(fB.policy, fB.validators, fB.cert, fB.msg, fB.mc); err != nil {
		t.Fatalf("committee B rejected: %v", err)
	}

	// Structural: the committees share NO public key (independent keypairs).
	innerA, _ := UnmarshalWeightedQuorumCert(fA.mc.CertSet)
	innerB, _ := UnmarshalWeightedQuorumCert(fB.mc.CertSet)
	for i := range innerA.Signers {
		for j := range innerB.Signers {
			if bytes.Equal(innerA.Signers[i].PublicKey, innerB.Signers[j].PublicKey) {
				t.Fatal("committees share a public key — independent keygen violated")
			}
		}
	}
	// Structural: each committee's own signers hold distinct keys (no shared key).
	if bytes.Equal(innerA.Signers[0].PublicKey, innerA.Signers[1].PublicKey) {
		t.Fatal("two signers in one committee share a public key — not independent")
	}
}

// ----------------------------------------------------------------------------
// Codec round-trip.
// ----------------------------------------------------------------------------

// TestMagnetarQuorum_CodecRoundTrip proves the wire codec is a deterministic
// bijection and rejects trailing bytes (fail-closed, no panic).
func TestMagnetarQuorum_CodecRoundTrip(t *testing.T) {
	f := buildMagnetarFixture(t, twoSLHDSA(t))
	wire := f.mc.Encode()
	got, err := DecodeMagnetarQuorumCert(wire)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SuiteID != f.mc.SuiteID || got.RollupRoot != f.mc.RollupRoot ||
		!bytes.Equal(got.CertSet, f.mc.CertSet) || !bytes.Equal(got.Proof, f.mc.Proof) {
		t.Fatal("round-trip mismatch")
	}
	if !bytes.Equal(got.Encode(), wire) {
		t.Fatal("re-encode is not byte-identical (non-canonical codec)")
	}
	// Trailing byte -> fail closed.
	if _, err := DecodeMagnetarQuorumCert(append(wire, 0x00)); !errors.Is(err, ErrEvidenceWireCorrupt) {
		t.Fatalf("trailing-byte err = %v, want ErrEvidenceWireCorrupt", err)
	}
}

// --- small test helpers (leg-kind comparison) ---

func legKinds(legs []LegSpec) []LegKind {
	out := make([]LegKind, len(legs))
	for i, l := range legs {
		out[i] = l.Kind
	}
	return out
}

func sameKinds(a, b []LegKind) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[LegKind]int, len(a))
	for _, k := range a {
		seen[k]++
	}
	for _, k := range b {
		seen[k]--
	}
	for _, v := range seen {
		if v != 0 {
			return false
		}
	}
	return true
}
