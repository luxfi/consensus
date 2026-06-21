// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/crypto/mldsa"
	magnetar "github.com/luxfi/magnetar/ref/go/pkg/magnetar"
)

// ----------------------------------------------------------------------------
// Test scaffold: a real validator set with real ML-DSA / SLH-DSA keys,
// real independent signatures over a real domain-separated message, and a
// real assembled cert. Every cryptographic operation is the production path
// — no mocks, no stubs.
// ----------------------------------------------------------------------------

// testSigner is one validator's full key material + accountability fields.
type testSigner struct {
	id       [32]byte
	scheme   QuorumSchemeID
	weight   uint64
	keyVer   uint32
	pubBytes []byte

	// exactly one of these is set per scheme
	mldsaPriv *mldsa.PrivateKey
	slhPriv   *magnetar.PrivateKey
}

// sign produces this signer's independent signature over message under ctx.
func (s *testSigner) sign(t *testing.T, message, ctx []byte) []byte {
	t.Helper()
	switch {
	case s.mldsaPriv != nil:
		sig, err := s.mldsaPriv.SignCtx(rand.Reader, message, ctx)
		if err != nil {
			t.Fatalf("ml-dsa sign: %v", err)
		}
		return sig
	case s.slhPriv != nil:
		// magnetar.ValidatorSign binds the empty FIPS context; tests that
		// use SLH-DSA records therefore pass ctx=nil at verify time.
		sig, err := magnetar.ValidatorSign(s.slhPriv, rand.Reader, message)
		if err != nil {
			t.Fatalf("slh-dsa sign: %v", err)
		}
		return sig
	default:
		t.Fatal("testSigner has no key")
		return nil
	}
}

// newMLDSASigner builds a real ML-DSA-65 signer at the given id/weight.
func newMLDSASigner(t *testing.T, idByte byte, weight uint64) *testSigner {
	t.Helper()
	priv, err := mldsa.GenerateKey(rand.Reader, mldsa.MLDSA65)
	if err != nil {
		t.Fatalf("ml-dsa keygen: %v", err)
	}
	pub := priv.Public().(*mldsa.PublicKey)
	var id [32]byte
	for i := range id {
		id[i] = idByte
	}
	return &testSigner{
		id:        id,
		scheme:    QuorumSchemeMLDSA65,
		weight:    weight,
		keyVer:    1,
		pubBytes:  pub.Bytes(),
		mldsaPriv: priv,
	}
}

// newSLHDSASigner builds a real SLH-DSA-192s signer at the given id/weight.
func newSLHDSASigner(t *testing.T, idByte byte, weight uint64) *testSigner {
	t.Helper()
	priv, err := magnetar.GenerateKey(magnetar.ParamsM192s, rand.Reader)
	if err != nil {
		t.Fatalf("slh-dsa keygen: %v", err)
	}
	var id [32]byte
	for i := range id {
		id[i] = idByte
	}
	return &testSigner{
		id:       id,
		scheme:   QuorumSchemeSLHDSA192s,
		weight:   weight,
		keyVer:   1,
		pubBytes: priv.Public().Bytes,
		slhPriv:  priv,
	}
}

// testEnv builds the canonical message envelope for the scenario.
func testEnv() QuorumMessageEnvelope {
	return QuorumMessageEnvelope{
		ProfileID:       0xC57E0001,
		HashSuite:       config.HashSuiteSHA3NIST,
		IdentityScheme:  config.IdentitySchemeMLDSA65,
		FinalityScheme:  config.SigSchemePulsar65,
		ProofPolicy:     config.ProofPolicySTARKFRISHA3PQ,
		ProofBackend:    config.ProofBackendDirectWeightedQuorum,
		ProofFormat:     config.ProofFormatDirectWeightedQuorumV1,
		VerifierID:      config.VerifierDirectWeightedQuorumPQ,
		EffectivePolicy: 1,
		NetworkID:       0xC0DE0001,
		ChainID:         0xDEADBEEF,
		Epoch:           42,
		Height:          1000,
		Round:           3,
		QCType:          2,
	}
}

// scenario is a fully-built, valid cert + everything needed to verify it.
type scenario struct {
	env     QuorumMessageEnvelope
	cfg     QuorumVerifierConfig
	message []byte
	cert    *WeightedQuorumCert
	set     *WeightedValidatorSet
	signers []*testSigner
}

// buildScenario assembles a real, valid cert from real signers. threshold is
// the quorum-weight floor; ctx is the FIPS context (nil for the SLH-DSA
// path). The value hash is fixed and bound into the message + cert.
func buildScenario(t *testing.T, signers []*testSigner, threshold uint64, ctx []byte) scenario {
	t.Helper()

	env := testEnv()
	var valueHash [32]byte
	for i := range valueHash {
		valueHash[i] = 0xAB
	}
	env.ValueHash = valueHash
	// The quorum threshold is bound into the SIGNED message, so it must be set
	// on the envelope before the signers sign (and it equals the cert's
	// QuorumThreshold below).
	env.QuorumThreshold = threshold

	// Build the weighted validator set commitment from the signers' leaves.
	leaves := make([]WeightedValidatorLeaf, len(signers))
	for i, s := range signers {
		leaves[i] = WeightedValidatorLeaf{
			ValidatorID:    s.id,
			PublicKey:      s.pubBytes,
			VotingWeight:   s.weight,
			ParameterSetID: uint8(s.scheme),
			KeyVersion:     s.keyVer,
		}
	}
	set, err := BuildWeightedValidatorSet(env.Epoch, leaves)
	if err != nil {
		t.Fatalf("build set: %v", err)
	}
	env.ValidatorSetRoot = set.Root()

	// The domain-separated consensus message.
	message, err := QuorumConsensusMessage(env)
	if err != nil {
		t.Fatalf("build message: %v", err)
	}

	// Each signer signs INDEPENDENTLY; build records with Merkle paths.
	sorted := set.Leaves()
	idxOf := func(id [32]byte) int {
		for i := range sorted {
			if sorted[i].ValidatorID == id {
				return i
			}
		}
		t.Fatalf("signer id %x not in set", id[:])
		return -1
	}
	records := make([]QuorumSignerRecord, 0, len(signers))
	for _, s := range signers {
		proof, err := set.InclusionProof(idxOf(s.id))
		if err != nil {
			t.Fatalf("inclusion proof: %v", err)
		}
		records = append(records, QuorumSignerRecord{
			ValidatorID:  s.id,
			PublicKey:    s.pubBytes,
			VotingWeight: s.weight,
			Scheme:       s.scheme,
			ParamSetID:   uint8(s.scheme),
			KeyVersion:   s.keyVer,
			MerklePath:   proof,
			Signature:    s.sign(t, message, ctx),
		})
	}

	cert, err := BuildWeightedQuorumCert(QuorumCertParams{
		ChainID:          env.ChainID,
		Epoch:            env.Epoch,
		Height:           env.Height,
		Round:            env.Round,
		ValueHash:        env.ValueHash,
		QCType:           env.QCType,
		ValidatorSetRoot: set.Root(),
		QuorumThreshold:  threshold,
	}, records)
	if err != nil {
		t.Fatalf("build cert: %v", err)
	}

	return scenario{
		env: env,
		// MinThreshold is the mandatory BFT-quorum floor. The scenario pins it
		// to the cert's threshold (the verifier MUST pin a non-zero floor or
		// Verify fails closed). Tests that drive a below-floor cert override it.
		cfg:     QuorumVerifierConfig{Context: ctx, MinThreshold: threshold},
		message: message,
		cert:    cert,
		set:     set,
		signers: signers,
	}
}

// fourMLDSA builds a standard 4-signer ML-DSA scenario, weights 25 each,
// threshold 100 (== Σ weight). ctx = nil.
func fourMLDSA(t *testing.T) scenario {
	signers := []*testSigner{
		newMLDSASigner(t, 0x01, 25),
		newMLDSASigner(t, 0x02, 25),
		newMLDSASigner(t, 0x03, 25),
		newMLDSASigner(t, 0x04, 25),
	}
	return buildScenario(t, signers, 100, nil)
}

// ----------------------------------------------------------------------------
// Happy path + independent-verifier interop
// ----------------------------------------------------------------------------

func TestQuorumCert_HappyPath(t *testing.T) {
	sc := fourMLDSA(t)
	if err := sc.cert.Verify(sc.env, sc.cfg); err != nil {
		t.Fatalf("valid cert rejected: %v", err)
	}
}

// TestQuorumCert_IndependentVerifierInterop proves each record's signature
// verifies under the STOCK FIPS verifier directly — no Lux cert machinery,
// just mldsa.PublicKey.VerifySignatureCtx. This is the wire-identity claim:
// any peer can verify a signer's contribution with a stock FIPS library.
func TestQuorumCert_IndependentVerifierInterop(t *testing.T) {
	sc := fourMLDSA(t)
	for i := range sc.cert.Signers {
		rec := &sc.cert.Signers[i]
		pk, err := mldsa.PublicKeyFromBytes(rec.PublicKey, mldsa.MLDSA65)
		if err != nil {
			t.Fatalf("record %d: parse pub: %v", i, err)
		}
		if !pk.VerifySignatureCtx(sc.message, rec.Signature, sc.cfg.Context) {
			t.Fatalf("record %d: stock FIPS 204 verifier rejected the signature", i)
		}
	}
}

// TestQuorumCert_SLHDSAInterop proves the SLH-DSA records verify under the
// stock FIPS 205 verifier (magnetar's thin VerifyCtx over circl/slhdsa).
func TestQuorumCert_SLHDSAInterop(t *testing.T) {
	signers := []*testSigner{
		newSLHDSASigner(t, 0x01, 60),
		newSLHDSASigner(t, 0x02, 60),
	}
	sc := buildScenario(t, signers, 100, nil)
	if err := sc.cert.Verify(sc.env, sc.cfg); err != nil {
		t.Fatalf("valid SLH-DSA cert rejected: %v", err)
	}
	for i := range sc.cert.Signers {
		rec := &sc.cert.Signers[i]
		params := magnetar.ParamsM192s
		pk := &magnetar.PublicKey{Mode: magnetar.ModeM192s, Bytes: rec.PublicKey}
		sig := &magnetar.Signature{Mode: magnetar.ModeM192s, Bytes: rec.Signature}
		if err := magnetar.VerifyCtx(params, pk, sc.message, sc.cfg.Context, sig); err != nil {
			t.Fatalf("record %d: stock FIPS 205 verifier rejected: %v", i, err)
		}
	}
}

// TestQuorumCert_MixedScheme proves a single cert can carry ML-DSA and
// SLH-DSA signer records side by side (the honest realization of a combined
// per-validator surface).
func TestQuorumCert_MixedScheme(t *testing.T) {
	signers := []*testSigner{
		newMLDSASigner(t, 0x01, 40),
		newSLHDSASigner(t, 0x02, 40),
		newMLDSASigner(t, 0x03, 40),
	}
	sc := buildScenario(t, signers, 100, nil)
	if err := sc.cert.Verify(sc.env, sc.cfg); err != nil {
		t.Fatalf("valid mixed-scheme cert rejected: %v", err)
	}
}

// TestQuorumCert_MLDSAContextBinding proves the FIPS context plumbing works:
// signing under ctx="A" and verifying under ctx="A" succeeds, under ctx="B"
// fails.
func TestQuorumCert_MLDSAContextBinding(t *testing.T) {
	ctxA := []byte("lux-quasar-wqc-ctx-A")
	signers := []*testSigner{
		newMLDSASigner(t, 0x01, 60),
		newMLDSASigner(t, 0x02, 60),
	}
	sc := buildScenario(t, signers, 100, ctxA)
	if err := sc.cert.Verify(sc.env, QuorumVerifierConfig{Context: ctxA, MinThreshold: 100}); err != nil {
		t.Fatalf("ctx-A cert rejected under ctx-A: %v", err)
	}
	if err := sc.cert.Verify(sc.env, QuorumVerifierConfig{Context: []byte("ctx-B"), MinThreshold: 100}); err == nil {
		t.Fatal("cert signed under ctx-A verified under ctx-B (context not bound)")
	}
}

// ----------------------------------------------------------------------------
// Distinct-signer enforcement
// ----------------------------------------------------------------------------

func TestQuorumCert_RejectsUnsorted(t *testing.T) {
	sc := fourMLDSA(t)
	// Swap two records so they are no longer strictly increasing.
	sc.cert.Signers[0], sc.cert.Signers[1] = sc.cert.Signers[1], sc.cert.Signers[0]
	// Recompute the commitment so the failure is the ordering check, not the
	// commitment check (we want to prove the ordering clause specifically).
	sc.cert.SignerCommitment = sc.cert.computeSignerCommitment()
	err := sc.cert.Verify(sc.env, sc.cfg)
	if !errors.Is(err, ErrQCNotStrictlyIncreasing) {
		t.Fatalf("unsorted cert err = %v, want ErrQCNotStrictlyIncreasing", err)
	}
}

func TestQuorumCert_RejectsDuplicateSigner(t *testing.T) {
	sc := fourMLDSA(t)
	// Duplicate record 1 over record 2's slot → two identical ids adjacent.
	sc.cert.Signers[2] = sc.cert.Signers[1]
	sc.cert.SignerCommitment = sc.cert.computeSignerCommitment()
	err := sc.cert.Verify(sc.env, sc.cfg)
	if !errors.Is(err, ErrQCNotStrictlyIncreasing) {
		t.Fatalf("duplicate-signer cert err = %v, want ErrQCNotStrictlyIncreasing", err)
	}
}

func TestQuorumCert_ProverRejectsDuplicate(t *testing.T) {
	// The prover itself must refuse to assemble a cert with duplicate ids.
	s := newMLDSASigner(t, 0x01, 50)
	rec := QuorumSignerRecord{
		ValidatorID:  s.id,
		PublicKey:    s.pubBytes,
		VotingWeight: s.weight,
		Scheme:       s.scheme,
		ParamSetID:   uint8(s.scheme),
		KeyVersion:   s.keyVer,
		MerklePath:   &WeightedInclusionProof{LeafCount: 1},
		Signature:    []byte{0x00},
	}
	_, err := BuildWeightedQuorumCert(QuorumCertParams{
		ChainID: 1, Epoch: 1, QuorumThreshold: 1, ValidatorSetRoot: [48]byte{1},
	}, []QuorumSignerRecord{rec, rec})
	if !errors.Is(err, ErrWVSetDuplicateID) {
		t.Fatalf("prover duplicate err = %v, want ErrWVSetDuplicateID", err)
	}
}

// ----------------------------------------------------------------------------
// Weight threshold
// ----------------------------------------------------------------------------

func TestQuorumCert_BelowThresholdRejected(t *testing.T) {
	// Σ weight = 100; set threshold to 101 → below threshold.
	signers := []*testSigner{
		newMLDSASigner(t, 0x01, 25),
		newMLDSASigner(t, 0x02, 25),
		newMLDSASigner(t, 0x03, 25),
		newMLDSASigner(t, 0x04, 25),
	}
	sc := buildScenario(t, signers, 101, nil)
	err := sc.cert.Verify(sc.env, sc.cfg)
	if !errors.Is(err, ErrQCBelowThreshold) {
		t.Fatalf("below-threshold err = %v, want ErrQCBelowThreshold", err)
	}
}

func TestQuorumCert_AtThresholdAccepted(t *testing.T) {
	// Σ weight = 100; threshold exactly 100 → accept (≥, not >).
	sc := fourMLDSA(t) // threshold == 100 == Σ weight
	if err := sc.cert.Verify(sc.env, sc.cfg); err != nil {
		t.Fatalf("at-threshold cert rejected: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Wrong message / domain / cross-epoch / cross-chain
// ----------------------------------------------------------------------------

func TestQuorumCert_WrongMessageRejected(t *testing.T) {
	sc := fourMLDSA(t)
	wrong := append([]byte(nil), sc.message...)
	wrong[0] ^= 0xFF
	// Drive the internal predicate with a deliberately-corrupted message. The
	// public Verify builds the message itself (it cannot be fed a wrong one);
	// verifyWithMessage is the demoted raw-message form, exercised here to
	// prove a message that does not match the signatures is rejected.
	err := sc.cert.verifyWithMessage(wrong, sc.cfg)
	if !errors.Is(err, ErrQCSigInvalid) {
		t.Fatalf("wrong-message err = %v, want ErrQCSigInvalid", err)
	}
}

// crossAxis re-derives a message with a single envelope axis changed and
// confirms the original cert's signatures do NOT verify against it.
func crossAxis(t *testing.T, name string, mutate func(*QuorumMessageEnvelope)) {
	t.Helper()
	sc := fourMLDSA(t)
	env := sc.env
	mutate(&env)
	msg2, err := QuorumConsensusMessage(env)
	if err != nil {
		t.Fatalf("%s: rebuild message: %v", name, err)
	}
	if bytes.Equal(msg2, sc.message) {
		t.Fatalf("%s: message unchanged after mutating axis (no binding!)", name)
	}
	// Drive the internal predicate with the cross-axis message: the cert's
	// signatures (over the original axis values) must not verify against it.
	if err := sc.cert.verifyWithMessage(msg2, sc.cfg); err == nil {
		t.Fatalf("%s: cert verified against cross-domain message (replay surface)", name)
	}
}

func TestQuorumCert_CrossDomainNonTransferability(t *testing.T) {
	crossAxis(t, "chain_id", func(e *QuorumMessageEnvelope) { e.ChainID++ })
	crossAxis(t, "epoch", func(e *QuorumMessageEnvelope) { e.Epoch++ })
	crossAxis(t, "height", func(e *QuorumMessageEnvelope) { e.Height++ })
	crossAxis(t, "round", func(e *QuorumMessageEnvelope) { e.Round++ })
	crossAxis(t, "qc_type", func(e *QuorumMessageEnvelope) { e.QCType++ })
	crossAxis(t, "value_hash", func(e *QuorumMessageEnvelope) { e.ValueHash[0] ^= 0xFF })
	crossAxis(t, "network_id", func(e *QuorumMessageEnvelope) { e.NetworkID++ })
	crossAxis(t, "proof_backend", func(e *QuorumMessageEnvelope) { e.ProofBackend = config.ProofBackendP3QSTARKFRISHA3 })
	crossAxis(t, "profile_id", func(e *QuorumMessageEnvelope) { e.ProfileID++ })
}

// ----------------------------------------------------------------------------
// Merkle-inclusion soundness (non-member rejected) at the cert layer
// ----------------------------------------------------------------------------

func TestQuorumCert_NonMemberRejected(t *testing.T) {
	sc := fourMLDSA(t)
	// Mutate a record's weight so its reconstructed leaf is no longer the
	// committed one — its Merkle path no longer proves inclusion. Re-sign
	// nothing (the signature is over the message, not the leaf); the failure
	// must be the Merkle clause.
	sc.cert.Signers[1].VotingWeight = 999
	// Keep aggregate consistent with the records so we reach the Merkle check
	// (otherwise we'd trip the aggregate-weight clause first — but here the
	// per-record Merkle check runs before the aggregate, so adjust both to be
	// safe and prove the Merkle clause specifically).
	sc.cert.AggregateWeight = sc.cert.AggregateWeight - 25 + 999
	sc.cert.SignerCommitment = sc.cert.computeSignerCommitment()
	err := sc.cert.Verify(sc.env, sc.cfg)
	if !errors.Is(err, ErrQCMerkleInclusion) {
		t.Fatalf("non-member err = %v, want ErrQCMerkleInclusion", err)
	}
}

func TestQuorumCert_UnknownValidatorNotFatal(t *testing.T) {
	// A record for a validator NOT in the committed set must make the cert
	// INVALID — but cleanly (a typed error), never a panic / DoS. We build a
	// fresh signer not in the set, give it a syntactically-valid but bogus
	// Merkle path, and confirm Verify returns ErrQCMerkleInclusion without
	// panicking.
	sc := fourMLDSA(t)
	stranger := newMLDSASigner(t, 0xFE, 25)
	// A path shaped for a 4-leaf tree at index 0, with a random sibling.
	var sib [48]byte
	for i := range sib {
		sib[i] = 0x77
	}
	bogus := &WeightedInclusionProof{
		LeafIndex: 0,
		LeafCount: 4,
		Steps: []WeightedProofStep{
			{Sibling: sib, SiblingIsRight: true},
			{Sibling: sib, SiblingIsRight: true},
		},
	}
	// Replace the lowest-id record so ordering stays strictly increasing.
	rec := QuorumSignerRecord{
		ValidatorID:  stranger.id, // 0xFE.. is the largest id → goes last
		PublicKey:    stranger.pubBytes,
		VotingWeight: stranger.weight,
		Scheme:       stranger.scheme,
		ParamSetID:   uint8(stranger.scheme),
		KeyVersion:   stranger.keyVer,
		MerklePath:   bogus,
		Signature:    stranger.sign(t, sc.message, nil),
	}
	sc.cert.Signers = append(sc.cert.Signers, rec)
	sc.cert.SignerCount++
	sc.cert.AggregateWeight += rec.VotingWeight
	sc.cert.SignerCommitment = sc.cert.computeSignerCommitment()

	// Must not panic; must return the Merkle clause error.
	err := sc.cert.Verify(sc.env, sc.cfg)
	if !errors.Is(err, ErrQCMerkleInclusion) {
		t.Fatalf("unknown-validator err = %v, want ErrQCMerkleInclusion (clean, not fatal)", err)
	}
}

// ----------------------------------------------------------------------------
// Aggregate-weight / signer-commitment tamper rejection
// ----------------------------------------------------------------------------

func TestQuorumCert_AggregateWeightTamper(t *testing.T) {
	sc := fourMLDSA(t)
	sc.cert.AggregateWeight++ // claim one more than Σ weight
	sc.cert.SignerCommitment = sc.cert.computeSignerCommitment()
	err := sc.cert.Verify(sc.env, sc.cfg)
	if !errors.Is(err, ErrQCAggregateWeight) {
		t.Fatalf("aggregate tamper err = %v, want ErrQCAggregateWeight", err)
	}
}

func TestQuorumCert_SignerCommitmentTamper(t *testing.T) {
	sc := fourMLDSA(t)
	sc.cert.SignerCommitment[0] ^= 0xFF
	err := sc.cert.Verify(sc.env, sc.cfg)
	if !errors.Is(err, ErrQCSignerCommitment) {
		t.Fatalf("commitment tamper err = %v, want ErrQCSignerCommitment", err)
	}
}

func TestQuorumCert_HeaderPositionTamperFailsVerify(t *testing.T) {
	// A cert that lies about its own consensus position must be rejected.
	// Under the envelope-based Verify, the verifier rebuilds the signing
	// message from the cert's OWN position fields (QuorumMessageForCert), so
	// flipping the height makes Verify rebuild the message at the tampered
	// height — and the signatures (produced over the real height) no longer
	// verify. This is a STRONGER guarantee than the old commitment-only catch:
	// position tampering fails at the per-signer FIPS check, earlier in the
	// predicate. (The commitment layer remains as defence-in-depth against
	// signer-set / leaf-position malleation that does not change the rebuilt
	// message — see SignerCommitmentTamper and the leaf-encoding test.)
	sc := fourMLDSA(t)
	sc.cert.Height++ // signatures were produced over the original height
	// Recompute the commitment so we PASS the commitment check and prove the
	// rejection is the signature clause specifically, not the commitment.
	sc.cert.SignerCommitment = sc.cert.computeSignerCommitment()
	err := sc.cert.Verify(sc.env, sc.cfg)
	if !errors.Is(err, ErrQCSigInvalid) {
		t.Fatalf("position tamper err = %v, want ErrQCSigInvalid", err)
	}
}

// ----------------------------------------------------------------------------
// Scheme allow-list
// ----------------------------------------------------------------------------

func TestQuorumCert_SchemeNotAllowed(t *testing.T) {
	sc := fourMLDSA(t)
	// Allow only SLH-DSA; the cert is all ML-DSA → rejected. MinThreshold must
	// be set or Verify fails closed before reaching the scheme check.
	cfg := QuorumVerifierConfig{
		Context:        nil,
		MinThreshold:   100,
		AllowedSchemes: map[QuorumSchemeID]bool{QuorumSchemeSLHDSA192s: true},
	}
	err := sc.cert.Verify(sc.env, cfg)
	if !errors.Is(err, ErrQCSchemeNotAllowed) {
		t.Fatalf("scheme-not-allowed err = %v, want ErrQCSchemeNotAllowed", err)
	}
}

func TestQuorumCert_ParamSetMismatch(t *testing.T) {
	sc := fourMLDSA(t)
	// Claim a param byte that disagrees with the scheme byte.
	sc.cert.Signers[0].ParamSetID = uint8(QuorumSchemeMLDSA87)
	sc.cert.SignerCommitment = sc.cert.computeSignerCommitment()
	err := sc.cert.Verify(sc.env, sc.cfg)
	if !errors.Is(err, ErrQCParamSetMismatch) {
		t.Fatalf("param mismatch err = %v, want ErrQCParamSetMismatch", err)
	}
}

// ----------------------------------------------------------------------------
// Structural / fail-closed
// ----------------------------------------------------------------------------

func TestQuorumCert_StructuralRejections(t *testing.T) {
	sc := fourMLDSA(t)

	var nilCert *WeightedQuorumCert
	if err := nilCert.Verify(sc.env, sc.cfg); !errors.Is(err, ErrQCNil) {
		t.Fatalf("nil cert err = %v, want ErrQCNil", err)
	}

	badVer := *sc.cert
	badVer.Version = 99
	if err := (&badVer).Verify(sc.env, sc.cfg); !errors.Is(err, ErrQCVersion) {
		t.Fatalf("bad version err = %v, want ErrQCVersion", err)
	}

	zeroThresh := *sc.cert
	zeroThresh.QuorumThreshold = 0
	if err := (&zeroThresh).Verify(sc.env, sc.cfg); !errors.Is(err, ErrQCThresholdZero) {
		t.Fatalf("zero threshold err = %v, want ErrQCThresholdZero", err)
	}

	countMismatch := *sc.cert
	countMismatch.SignerCount = 99
	if err := (&countMismatch).Verify(sc.env, sc.cfg); !errors.Is(err, ErrQCSignerCountMismatch) {
		t.Fatalf("count mismatch err = %v, want ErrQCSignerCountMismatch", err)
	}
}
