// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
	"github.com/luxfi/ids"
	magnetar "github.com/luxfi/magnetar/ref/go/pkg/magnetar"
	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
	"github.com/luxfi/threshold/protocols/pulsar"
)

// This file proves the QuasarCert AND-of-legs is VERIFY-ENFORCED via the
// chain's config.CertPolicy (Red H3). The pre-fix verifier made BLS
// mandatory and PQ legs optional/cert-byte-driven; an adversary who
// broke BLS-12-381 could ship a BLS-only cert and a legitimate pure-PQ
// cert was wrongly rejected. The fix routes the mandatory-leg decision
// through config.CertPolicy.RequiredLegs() (previously dead code).
//
// The negative tests REMOVE each required leg (not mutate bytes in place
// — leg removal is the actual attack: omit the leg you forged around)
// and assert rejection. The positive test verifies a pure-PQ (no-BLS)
// cert under a Strict-variant policy / Polaris.

// polarisFixture carries a fully-composed real Polaris cert and the keys
// to verify each leg. Every leg's bytes come from the leg's real signing
// primitive (no stubs / zero-fill), mirroring TestQuasarCert_ComposesAllThreeSchemes.
type polarisFixture struct {
	digest    []byte
	cert      *QuasarCert
	blsPK     *bls.PublicKey
	coronaKey *coronaThreshold.GroupKey
	pulsarKey []byte
	mldsaPubs []*mldsa.PublicKey
	magKnown  map[magnetar.NodeID][]byte
}

// keysFor returns the CertKeys bundle matching the fixture.
func (f polarisFixture) keys() CertKeys {
	return CertKeys{
		BLS:      f.blsPK,
		Pulsar:   f.pulsarKey,
		Corona:   f.coronaKey,
		MLDSA:    f.mldsaPubs,
		Magnetar: f.magKnown,
	}
}

// buildPolarisFixture composes a real five-leg Polaris cert. It is heavy
// (Pulsar DKG + SLH-DSA hash trees) and is therefore skipped under
// -short, exactly like the compose gate test.
func buildPolarisFixture(t *testing.T) polarisFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping SLH-DSA / DKG-bound Polaris fixture under -short")
	}

	digest := []byte("h3-policy-verify-enforced-and-of-legs-digest")

	// --- Leg 1: Pulsar (Module-LWE threshold ML-DSA). ---
	pulsarParams := pulsar.MustParamsFor(pulsar.ModeP65)
	committee := []pulsar.NodeID{{0x01, 0x00, 'V'}, {0x02, 0x00, 'V'}, {0x03, 0x00, 'V'}}
	const n, threshold = 3, 2

	identities := make(map[pulsar.NodeID]*pulsar.IdentityKey, n)
	dirEntries := make(map[pulsar.NodeID]*pulsar.IdentityPublicKey, n)
	for i := 0; i < n; i++ {
		ik, err := pulsar.GenerateIdentity(rand.Reader)
		if err != nil {
			t.Fatalf("pulsar.GenerateIdentity[%d]: %v", i, err)
		}
		identities[committee[i]] = ik
		dirEntries[committee[i]] = ik.PublicKey()
	}
	directory, err := pulsar.NewIdentityDirectory(dirEntries)
	if err != nil {
		t.Fatalf("pulsar.NewIdentityDirectory: %v", err)
	}
	dkg := make([]*pulsar.DKGSession, n)
	for i := 0; i < n; i++ {
		rng := bytes.NewReader(append([]byte{byte(i + 0xA0)}, bytes.Repeat([]byte{0xCA, 0xFE, 0xBA, 0xBE}, 32)...))
		s, err := pulsar.NewDKGSession(pulsarParams, committee, threshold, committee[i], identities[committee[i]], directory, rng)
		if err != nil {
			t.Fatalf("pulsar.NewDKGSession[%d]: %v", i, err)
		}
		dkg[i] = s
	}
	r1 := make([]*pulsar.DKGRound1Msg, n)
	for i, s := range dkg {
		m, err := s.Round1()
		if err != nil {
			t.Fatalf("pulsar.Round1[%d]: %v", i, err)
		}
		r1[i] = m
	}
	r2 := make([]*pulsar.DKGRound2Msg, n)
	for i, s := range dkg {
		m, err := s.Round2(r1)
		if err != nil {
			t.Fatalf("pulsar.Round2[%d]: %v", i, err)
		}
		r2[i] = m
	}
	dkgOuts := make([]*pulsar.DKGOutput, n)
	for i, s := range dkg {
		out, err := s.Round3(r1, r2)
		if err != nil {
			t.Fatalf("pulsar.Round3[%d]: %v", i, err)
		}
		dkgOuts[i] = out
	}
	pulsarGroupPK := dkgOuts[0].GroupPubkey
	pulsarPool := make([]ids.NodeID, n)
	pulsarShares := make(map[ids.NodeID]*pulsar.KeyShare, n)
	for i := 0; i < n; i++ {
		var id ids.NodeID
		copy(id[:], committee[i][:])
		pulsarPool[i] = id
		pulsarShares[id] = dkgOuts[i].SecretShare
	}
	pulsarRoundSigner := &RoundSigner{
		Params:    pulsarParams,
		Cut:       prism.NewUniformCut(pulsarPool),
		K:         3,
		Threshold: 2,
		Shares:    pulsarShares,
	}
	pulsarRoundRes, err := pulsarRoundSigner.RunRound(context.Background(), digest)
	if err != nil {
		t.Fatalf("Pulsar RoundSigner.RunRound: %v", err)
	}

	// --- Leg 2: Corona (Ring-LWE threshold ML-DSA). ---
	coronaShares, coronaGroupKey, err := coronaThreshold.GenerateKeys(threshold, n, nil)
	if err != nil {
		t.Fatalf("corona.GenerateKeys: %v", err)
	}
	coronaSigners := make([]*coronaThreshold.Signer, threshold)
	signerIDs := make([]int, threshold)
	for i := 0; i < threshold; i++ {
		coronaSigners[i] = coronaThreshold.NewSigner(coronaShares[i])
		signerIDs[i] = coronaShares[i].Index
	}
	const coronaSessionID = 31415
	coronaPRFKey := []byte("h3-policy-verify-corona-prf-32B!!")
	coronaRound1 := make(map[int]*coronaThreshold.Round1Data, threshold)
	for i := 0; i < threshold; i++ {
		coronaRound1[signerIDs[i]] = coronaSigners[i].Round1(coronaSessionID, coronaPRFKey, signerIDs)
	}
	coronaRound2 := make(map[int]*coronaThreshold.Round2Data, threshold)
	for i := 0; i < threshold; i++ {
		rr2, err := coronaSigners[i].Round2(coronaSessionID, string(digest), coronaPRFKey, signerIDs, coronaRound1)
		if err != nil {
			t.Fatalf("corona.Round2[%d]: %v", i, err)
		}
		coronaRound2[signerIDs[i]] = rr2
	}
	coronaSig, err := coronaSigners[0].Finalize(coronaRound2)
	if err != nil {
		t.Fatalf("corona.Finalize: %v", err)
	}

	// --- Leg 3: Magnetar (SLH-DSA / FIPS 205 hash-based). ---
	const magN = 3
	magParams := magnetar.ParamsM192s
	magPubBytes := make([][]byte, magN)
	magSigBytes := make([][]byte, magN)
	magSigners := make([]magnetar.NodeID, magN)
	magKnown := make(map[magnetar.NodeID][]byte, magN)
	for i := 0; i < magN; i++ {
		sk, pk, err := magnetar.PerValidatorKeypair(magParams, rand.Reader)
		if err != nil {
			t.Fatalf("magnetar.PerValidatorKeypair[%d]: %v", i, err)
		}
		sig, err := magnetar.ValidatorSign(sk, nil, digest)
		if err != nil {
			t.Fatalf("magnetar.ValidatorSign[%d]: %v", i, err)
		}
		var id magnetar.NodeID
		id[0] = byte(i + 1)
		id[1], id[2], id[3] = 'M', 'A', 'G'
		magSigners[i] = id
		magPubBytes[i] = pk.Bytes
		magSigBytes[i] = sig
		magKnown[id] = pk.Bytes
	}
	magCert, err := magnetar.BuildAggregateCert(magParams, magSigners, magPubBytes, magSigBytes)
	if err != nil {
		t.Fatalf("magnetar.BuildAggregateCert: %v", err)
	}

	// --- Leg 4: BLS-12-381 classical fast-path. ---
	blsSK, err := bls.NewSecretKey()
	if err != nil {
		t.Fatalf("bls.NewSecretKey: %v", err)
	}
	blsPK := blsSK.PublicKey()
	blsSig, err := blsSK.Sign(digest)
	if err != nil {
		t.Fatalf("bls.Sign: %v", err)
	}

	// --- Leg 5: ML-DSA-65 identity rollup. ---
	mldsaSK, err := mldsa.GenerateKey(rand.Reader, mldsa.MLDSA65)
	if err != nil {
		t.Fatalf("mldsa.GenerateKey: %v", err)
	}
	mldsaSig, err := mldsaSK.Sign(rand.Reader, digest, nil)
	if err != nil {
		t.Fatalf("mldsa.Sign: %v", err)
	}
	mldsaRollup := EncodeMLDSASigs([][]byte{mldsaSig})

	cert, err := ComposePolaris(PolarisLegs{
		BLS:         blsSig,
		Pulsar:      pulsarRoundRes.Signature,
		Corona:      coronaSig,
		Magnetar:    magCert,
		MLDSARollup: mldsaRollup,
		Epoch:       42,
		Finality:    time.Unix(1730000000, 0),
		Validators:  n,
	})
	if err != nil {
		t.Fatalf("ComposePolaris: %v", err)
	}

	pulsarGroupKeyWire, err := pulsarGroupPK.MarshalBinary()
	if err != nil {
		t.Fatalf("pulsarGroupPK.MarshalBinary: %v", err)
	}

	return polarisFixture{
		digest:    digest,
		cert:      cert,
		blsPK:     blsPK,
		coronaKey: coronaGroupKey,
		pulsarKey: pulsarGroupKeyWire,
		mldsaPubs: []*mldsa.PublicKey{mldsaSK.PublicKey},
		magKnown:  magKnown,
	}
}

// hybridHeavyPolicy requires every leg: BLS + Pulsar + Corona + Magnetar.
func hybridHeavyPolicy(t *testing.T) config.CertPolicy {
	t.Helper()
	cp := config.CertPolicy{Mode: config.CertModeHeavy, Variant: config.CertVariantHybrid, TimeoutMs: 10_000, Fallback: config.CertModeHeavy}
	if err := cp.Validate(); err != nil {
		t.Fatalf("hybridHeavyPolicy invalid: %v", err)
	}
	want := []config.LegName{config.LegBLS, config.LegPulsar, config.LegCorona, config.LegMagnetar}
	got := cp.RequiredLegs()
	if len(got) != len(want) {
		t.Fatalf("hybridHeavyPolicy RequiredLegs = %v, want %v", got, want)
	}
	return cp
}

// strictHeavyPolicy requires pure PQ: Pulsar + Corona + Magnetar, NO BLS.
func strictHeavyPolicy(t *testing.T) config.CertPolicy {
	t.Helper()
	cp := config.CertPolicy{Mode: config.CertModeHeavy, Variant: config.CertVariantStrict, TimeoutMs: 10_000, Fallback: config.CertModeFast}
	if err := cp.Validate(); err != nil {
		t.Fatalf("strictHeavyPolicy invalid: %v", err)
	}
	for _, leg := range cp.RequiredLegs() {
		if leg == config.LegBLS {
			t.Fatal("strictHeavyPolicy must NOT require BLS")
		}
	}
	return cp
}

// TestVerifyUnderPolicy_AcceptsWellFormed confirms the policy verifier
// accepts a complete five-leg cert under a hybrid-heavy policy.
func TestVerifyUnderPolicy_AcceptsWellFormed(t *testing.T) {
	f := buildPolarisFixture(t)
	cp := hybridHeavyPolicy(t)
	if !f.cert.VerifyUnderPolicy(f.digest, cp, f.keys()) {
		t.Fatal("VerifyUnderPolicy rejected a well-formed full cert")
	}
}

// TestVerifyUnderPolicy_RejectsRemovedRequiredLeg is the core H3 negative
// test: for each required leg, REMOVING that leg's bytes from the cert
// must cause rejection — presence is policy-driven, not cert-byte-driven.
func TestVerifyUnderPolicy_RejectsRemovedRequiredLeg(t *testing.T) {
	f := buildPolarisFixture(t)
	cp := hybridHeavyPolicy(t)

	// Sanity: full cert verifies.
	if !f.cert.VerifyUnderPolicy(f.digest, cp, f.keys()) {
		t.Fatal("precondition: full cert must verify")
	}

	type legCase struct {
		name   string
		remove func(c *QuasarCert)
	}
	cases := []legCase{
		{"BLS", func(c *QuasarCert) { c.BLS = nil }},
		{"Pulsar", func(c *QuasarCert) { c.Pulsar = nil }},
		{"Corona", func(c *QuasarCert) { c.Corona = nil }},
		{"Magnetar", func(c *QuasarCert) { c.Magnetar = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stripped := *f.cert // shallow copy; remove() nils a slice field
			tc.remove(&stripped)
			if stripped.VerifyUnderPolicy(f.digest, cp, f.keys()) {
				t.Fatalf("VerifyUnderPolicy ACCEPTED a cert with the required %s leg removed", tc.name)
			}
		})
	}
}

// TestVerifyUnderPolicy_RejectsMissingKeyForRequiredLeg confirms that a
// required leg present in the cert but with NO supplied key is rejected
// (fail-closed: a leg that cannot be verified must not pass).
func TestVerifyUnderPolicy_RejectsMissingKeyForRequiredLeg(t *testing.T) {
	f := buildPolarisFixture(t)
	cp := hybridHeavyPolicy(t)

	type keyCase struct {
		name string
		drop func(k *CertKeys)
	}
	cases := []keyCase{
		{"BLS", func(k *CertKeys) { k.BLS = nil }},
		{"Pulsar", func(k *CertKeys) { k.Pulsar = nil }},
		{"Corona", func(k *CertKeys) { k.Corona = nil }},
		{"Magnetar", func(k *CertKeys) { k.Magnetar = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keys := f.keys()
			tc.drop(&keys)
			if f.cert.VerifyUnderPolicy(f.digest, cp, keys) {
				t.Fatalf("VerifyUnderPolicy ACCEPTED with the %s key missing for a required leg", tc.name)
			}
		})
	}
}

// TestVerifyUnderPolicy_PurePQVerifiesUnderStrict is the H3 positive
// counterpart: a pure-PQ cert (NO BLS) MUST verify under a Strict-variant
// policy. The pre-fix verifier hard-rejected any no-BLS cert.
func TestVerifyUnderPolicy_PurePQVerifiesUnderStrict(t *testing.T) {
	f := buildPolarisFixture(t)
	cp := strictHeavyPolicy(t)

	// Strip BLS to make it a genuine pure-PQ cert.
	purePQ := *f.cert
	purePQ.BLS = nil

	keys := f.keys()
	keys.BLS = nil // no BLS key either — pure PQ

	if !purePQ.VerifyUnderPolicy(f.digest, cp, keys) {
		t.Fatal("VerifyUnderPolicy REJECTED a valid pure-PQ cert under a Strict-variant policy")
	}
}

// TestVerifyWithRealKeysPolaris_PurePQ confirms the wired (non-policy)
// Polaris entry point also accepts a pure-PQ (no-BLS) cert when no BLS key
// is supplied — the BLS-mandatory bug is gone.
func TestVerifyWithRealKeysPolaris_PurePQ(t *testing.T) {
	f := buildPolarisFixture(t)

	purePQ := *f.cert
	purePQ.BLS = nil

	if !purePQ.VerifyWithRealKeysPolaris(
		f.digest,
		nil, // no BLS key => pure-PQ posture
		f.coronaKey,
		f.pulsarKey,
		f.mldsaPubs,
		f.magKnown,
	) {
		t.Fatal("VerifyWithRealKeysPolaris REJECTED a valid pure-PQ cert (BLS wrongly mandatory)")
	}
}

// TestVerifyWithRealKeysPolaris_RejectsRemovedLegWhenKeyed confirms the
// wired path also rejects a removed PQ leg when its key is supplied
// (an adversary cannot strip a leg the verifier was configured to check).
func TestVerifyWithRealKeysPolaris_RejectsRemovedLegWhenKeyed(t *testing.T) {
	f := buildPolarisFixture(t)

	// Sanity: full cert verifies through the wired path.
	if !f.cert.VerifyWithRealKeysPolaris(f.digest, f.blsPK, f.coronaKey, f.pulsarKey, f.mldsaPubs, f.magKnown) {
		t.Fatal("precondition: full cert must verify via wired path")
	}

	type legCase struct {
		name   string
		remove func(c *QuasarCert)
	}
	cases := []legCase{
		{"Corona", func(c *QuasarCert) { c.Corona = nil }},
		{"Pulsar", func(c *QuasarCert) { c.Pulsar = nil }},
		{"Magnetar", func(c *QuasarCert) { c.Magnetar = nil }},
		{"MLDSARollup", func(c *QuasarCert) { c.MLDSARollup = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stripped := *f.cert
			tc.remove(&stripped)
			if stripped.VerifyWithRealKeysPolaris(f.digest, f.blsPK, f.coronaKey, f.pulsarKey, f.mldsaPubs, f.magKnown) {
				t.Fatalf("wired path ACCEPTED a cert with the %s leg removed while keyed", tc.name)
			}
		})
	}
}

// TestVerifyWithRealKeys_BLSOnlyCertWithPQKeysRejected reproduces Red's
// PoC (A) and asserts it is now CLOSED via the wired path: a BLS-only cert
// presented with PQ keys configured (Corona key supplied) must be rejected
// because the Corona leg the verifier was told to check is absent.
func TestVerifyWithRealKeys_BLSOnlyCertWithPQKeysRejected(t *testing.T) {
	f := buildPolarisFixture(t)

	blsOnly := &QuasarCert{
		BLS:      append([]byte(nil), f.cert.BLS...),
		Epoch:    f.cert.Epoch,
		Finality: f.cert.Finality,
	}
	// Corona key supplied => Corona leg required => absent leg rejects.
	if blsOnly.VerifyWithRealKeys(f.digest, f.blsPK, f.coronaKey, f.mldsaPubs) {
		t.Fatal("VerifyWithRealKeys ACCEPTED a BLS-only cert while a Corona key was configured")
	}
}
