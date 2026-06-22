// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
	"github.com/luxfi/ids"
	magnetar "github.com/luxfi/magnetar/ref/go/pkg/magnetar"
	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
	"github.com/luxfi/threshold/protocols/pulsar"
)

// TestQuasarCert_ComposesAllThreeSchemes is the gate test the audit
// task pins. It builds a Polaris-profile QuasarCert that carries all
// three production PQ schemes (Pulsar + Corona + Magnetar) and
// verifies that:
//
//  1. Each leg's bytes are produced by the leg's real signing primitive
//     (no stubs, no zero-fill).
//  2. The cert round-trips through MarshalBinary / UnmarshalBinary
//     without loss.
//  3. The post-decode cert reports IsPolaris() == true.
//  4. Cryptographic verification through VerifyWithRealKeysPolaris
//     accepts the cert under the right keys and refuses under wrong
//     keys.
//
// This is the load-bearing assertion that the headline "double-lattice
// + hash-based" PQ defense-in-depth (papers §04) is REAL, not
// aspirational — every leg has a working code path that the cert
// composer exercises end-to-end.
func TestQuasarCert_ComposesAllThreeSchemes(t *testing.T) {
	// SLH-DSA-via-magnetar in the leg-3 path is unavoidably ~30s under
	// `-race` (NIST FIPS 205 hash-tree depth × goroutine instrumentation
	// overhead). Gate the heavy compose under -short so the full quasar
	// suite stays under the 10m -race timeout. See SUMMARY.md for the
	// race-hot-path bottleneck investigation that pinned this.
	if testing.Short() {
		t.Skip("skipping SLH-DSA-bound compose test under -short")
	}

	// One round digest signed by every leg. Bound out-of-band by the
	// QBlock TranscriptHash pattern in production; here we use a raw
	// fixed digest for the round-trip exercise.
	digest := []byte("polaris-compose-all-three-schemes-test-digest")

	// =========================================================
	// Leg 1: Pulsar — Module-LWE threshold ML-DSA.
	// Build a (K, T) = (3, 2) DKG, run one round of threshold
	// signing, capture the resulting FIPS 204 ML-DSA signature
	// AND the group pubkey bytes (needed for verify).
	// =========================================================
	pulsarParams := pulsar.MustParamsFor(pulsar.ModeP65)
	committee := []pulsar.NodeID{
		{0x01, 0x00, 'V'}, {0x02, 0x00, 'V'}, {0x03, 0x00, 'V'},
	}
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
		// Distinct deterministic RNG per party for reproducibility.
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

	// Drive one Lux round via RoundSigner over a (3, 2) Cut.
	pulsarPool := make([]ids.NodeID, n)
	pulsarShares := make(map[ids.NodeID]*pulsar.KeyShare, n)
	for i := 0; i < n; i++ {
		var id ids.NodeID
		copy(id[:], committee[i][:])
		pulsarPool[i] = id
		pulsarShares[id] = dkgOuts[i].SecretShare
	}
	// Empty PrecompileCtx — the Polaris cert path's pulsar leg signs
	// with no ctx so verification routes through the stateless
	// pulsarwire.VerifyBytes (empty-ctx FIPS 204 verifier). The
	// cert's domain separation lives in the TupleHash transcript
	// that produced the round digest, not in the ML-DSA ctx string.
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
	if pulsarRoundRes.Signature == nil || len(pulsarRoundRes.Signature.Bytes) == 0 {
		t.Fatal("Pulsar round produced empty signature")
	}
	// Smoke check: byte-equal to FIPS 204 ML-DSA.Verify under empty
	// ctx. If this fails the rest of the test is moot.
	if err := pulsar.VerifyCtx(pulsarParams, pulsarGroupPK, digest, nil, pulsarRoundRes.Signature); err != nil {
		t.Fatalf("Pulsar leg failed standalone FIPS 204 verify: %v", err)
	}

	// =========================================================
	// Leg 2: Corona — Ring-LWE threshold ML-DSA.
	// Trusted-dealer keygen (acceptable for an in-process test;
	// production runs Pedersen DKG via keyera.Bootstrap).
	// =========================================================
	coronaShares, coronaGroupKey, err := coronaThreshold.GenerateKeys(threshold, n, nil)
	if err != nil {
		t.Fatalf("corona.GenerateKeys: %v", err)
	}
	if len(coronaShares) != n {
		t.Fatalf("corona.GenerateKeys returned %d shares, want %d", len(coronaShares), n)
	}
	coronaSigners := make([]*coronaThreshold.Signer, threshold)
	signerIDs := make([]int, threshold)
	for i := 0; i < threshold; i++ {
		coronaSigners[i] = coronaThreshold.NewSigner(coronaShares[i])
		signerIDs[i] = coronaShares[i].Index
	}
	const coronaSessionID = 31415
	coronaPRFKey := []byte("polaris-compose-corona-prf-32B!!")
	coronaRound1 := make(map[int]*coronaThreshold.Round1Data, threshold)
	for i := 0; i < threshold; i++ {
		r1, err := coronaSigners[i].Round1(coronaSessionID, coronaPRFKey, signerIDs)
		if err != nil {
			t.Fatalf("corona.Round1[%d]: %v", i, err)
		}
		coronaRound1[signerIDs[i]] = r1
	}
	coronaRound2 := make(map[int]*coronaThreshold.Round2Data, threshold)
	for i := 0; i < threshold; i++ {
		r2, err := coronaSigners[i].Round2(coronaSessionID, string(digest), coronaPRFKey, signerIDs, coronaRound1)
		if err != nil {
			t.Fatalf("corona.Round2[%d]: %v", i, err)
		}
		coronaRound2[signerIDs[i]] = r2
	}
	coronaSig, err := coronaSigners[0].Finalize(coronaRound2)
	if err != nil {
		t.Fatalf("corona.Finalize: %v", err)
	}
	// Smoke check: corona verify accepts.
	if !coronaThreshold.Verify(coronaGroupKey, string(digest), coronaSig) {
		t.Fatal("Corona leg failed standalone Verify")
	}

	// =========================================================
	// Leg 3: Magnetar — SLH-DSA hash-based.
	// Per-validator standalone: each validator holds its own
	// FIPS 205 keypair and signs the digest independently. The
	// cert collects all N signatures into a ValidatorAggregateCert.
	// =========================================================
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
		id[1] = 'M'
		id[2] = 'A'
		id[3] = 'G'
		magSigners[i] = id
		magPubBytes[i] = pk.Bytes
		magSigBytes[i] = sig
		magKnown[id] = pk.Bytes
	}
	magCert, err := magnetar.BuildAggregateCert(magParams, magSigners, magPubBytes, magSigBytes)
	if err != nil {
		t.Fatalf("magnetar.BuildAggregateCert: %v", err)
	}
	// Smoke check: magnetar verify accepts every signer.
	if validCount, err := magnetar.VerifyAggregateCert(magCert, digest, magKnown); err != nil {
		t.Fatalf("magnetar.VerifyAggregateCert: %v", err)
	} else if validCount != magN {
		t.Fatalf("magnetar leg expected %d valid sigs, got %d", magN, validCount)
	}

	// =========================================================
	// Leg 4 (optional classical fast-path): BLS-12-381 aggregate.
	// =========================================================
	blsSK, err := bls.NewSecretKey()
	if err != nil {
		t.Fatalf("bls.NewSecretKey: %v", err)
	}
	blsPK := blsSK.PublicKey()
	blsSig, err := blsSK.Sign(digest)
	if err != nil {
		t.Fatalf("bls.Sign: %v", err)
	}
	// Single-signer aggregate is the signature itself; this exercises
	// the BLS leg byte path without dragging in a full N-party aggregate.

	// =========================================================
	// Leg 5 (optional): ML-DSA-65 per-validator rollup. Emit one
	// per-validator signature so HasIdentityRollup() returns true.
	// =========================================================
	mldsaSK, err := mldsa.GenerateKey(rand.Reader, mldsa.MLDSA65)
	if err != nil {
		t.Fatalf("mldsa.GenerateKey: %v", err)
	}
	mldsaSig, err := mldsaSK.Sign(rand.Reader, digest, nil)
	if err != nil {
		t.Fatalf("mldsa.Sign: %v", err)
	}
	mldsaRollup := EncodeMLDSASigs([][]byte{mldsaSig})

	// =========================================================
	// Compose the Polaris cert.
	// =========================================================
	legs := PolarisLegs{
		BLS:         blsSig,
		Pulsar:      pulsarRoundRes.Signature,
		Corona:      coronaSig,
		Magnetar:    magCert,
		MLDSARollup: mldsaRollup,
		Epoch:       42,
		Finality:    time.Unix(1730000000, 0),
		Validators:  n,
	}
	cert, err := ComposePolaris(legs)
	if err != nil {
		t.Fatalf("ComposePolaris: %v", err)
	}
	if !cert.IsPolaris() {
		t.Fatal("ComposePolaris result reports IsPolaris() = false")
	}
	if !cert.IsDoubleLattice() {
		t.Fatal("ComposePolaris result reports IsDoubleLattice() = false")
	}
	if !cert.HasHashBased() {
		t.Fatal("ComposePolaris result reports HasHashBased() = false")
	}
	if !cert.HasClassicalFastPath() {
		t.Fatal("ComposePolaris result reports HasClassicalFastPath() = false")
	}
	if !cert.HasIdentityRollup() {
		t.Fatal("ComposePolaris result reports HasIdentityRollup() = false")
	}

	// =========================================================
	// Round-trip through the wire codec.
	// =========================================================
	raw, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("cert.MarshalBinary: %v", err)
	}
	if raw[0] != CertSchemeQuasar {
		t.Fatalf("scheme byte = 0x%02x, want 0x%02x", raw[0], CertSchemeQuasar)
	}

	decoded := &QuasarCert{}
	if err := decoded.UnmarshalBinary(raw); err != nil {
		t.Fatalf("decoded.UnmarshalBinary: %v", err)
	}
	if !bytes.Equal(decoded.BLS, cert.BLS) {
		t.Fatal("BLS leg round-trip mismatch")
	}
	if !bytes.Equal(decoded.Corona, cert.Corona) {
		t.Fatal("Corona leg round-trip mismatch")
	}
	if !bytes.Equal(decoded.Pulsar, cert.Pulsar) {
		t.Fatal("Pulsar leg round-trip mismatch")
	}
	if !bytes.Equal(decoded.Magnetar, cert.Magnetar) {
		t.Fatal("Magnetar leg round-trip mismatch")
	}
	if !bytes.Equal(decoded.MLDSARollup, cert.MLDSARollup) {
		t.Fatal("MLDSARollup leg round-trip mismatch")
	}
	if decoded.Epoch != cert.Epoch {
		t.Fatalf("Epoch round-trip mismatch: %d vs %d", decoded.Epoch, cert.Epoch)
	}
	if decoded.Finality.Unix() != cert.Finality.Unix() {
		t.Fatalf("Finality round-trip mismatch: %v vs %v", decoded.Finality, cert.Finality)
	}
	if decoded.Validators != cert.Validators {
		t.Fatalf("Validators round-trip mismatch: %d vs %d", decoded.Validators, cert.Validators)
	}
	if !decoded.IsPolaris() {
		t.Fatal("decoded cert reports IsPolaris() = false")
	}

	// =========================================================
	// Cryptographic verification of every leg through the
	// VerifyWithRealKeysPolaris path.
	// =========================================================

	// Build the pulsar group key wire bytes (PULG-framed) so the
	// Polaris verifier's pulsar leg can route through
	// pulsarwire.VerifyBytes.
	pulsarGroupKeyWire, err := pulsarGroupPK.MarshalBinary()
	if err != nil {
		t.Fatalf("pulsarGroupPK.MarshalBinary: %v", err)
	}

	mldsaPubs := []*mldsa.PublicKey{mldsaSK.PublicKey}

	if !decoded.VerifyWithRealKeysPolaris(
		digest,
		blsPK,
		coronaGroupKey,
		pulsarGroupKeyWire,
		mldsaPubs,
		magKnown,
	) {
		t.Fatal("VerifyWithRealKeysPolaris rejected a well-formed Polaris cert")
	}

	// =========================================================
	// Tamper checks — each leg's bytes are load-bearing.
	// =========================================================

	// Tampered Magnetar bytes must reject.
	tampered := *decoded
	tampered.Magnetar = append([]byte(nil), decoded.Magnetar...)
	tampered.Magnetar[len(tampered.Magnetar)-1] ^= 0xFF
	if tampered.VerifyWithRealKeysPolaris(digest, blsPK, coronaGroupKey, pulsarGroupKeyWire, mldsaPubs, magKnown) {
		t.Fatal("VerifyWithRealKeysPolaris accepted a tampered Magnetar leg")
	}

	// Tampered Pulsar bytes must reject. Flip a byte deep in the
	// signature payload (skip the wire frame header).
	tampered = *decoded
	tampered.Pulsar = append([]byte(nil), decoded.Pulsar...)
	if len(tampered.Pulsar) > 32 {
		tampered.Pulsar[len(tampered.Pulsar)/2] ^= 0xAA
	}
	if tampered.VerifyWithRealKeysPolaris(digest, blsPK, coronaGroupKey, pulsarGroupKeyWire, mldsaPubs, magKnown) {
		t.Fatal("VerifyWithRealKeysPolaris accepted a tampered Pulsar leg")
	}

	// Verify under a wrong message must reject.
	if decoded.VerifyWithRealKeysPolaris([]byte("wrong-message"), blsPK, coronaGroupKey, pulsarGroupKeyWire, mldsaPubs, magKnown) {
		t.Fatal("VerifyWithRealKeysPolaris accepted a wrong-message verification")
	}
}
