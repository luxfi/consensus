// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_redteam_test.go — adversarial closures for the red-team findings on
// the weighted quorum certificate's ENVELOPE binding. Each test reproduces a
// specific reported PoC and asserts it is now REJECTED (fail-closed). The
// crypto core (TupleHash256, FIPS verify, Merkle, leaf binding, distinct
// signer, weight guards) is sound and untouched; these tests pin the envelope
// fixes:
//
//	F1 (CRITICAL) sub-quorum finality forgery via unbound QuorumThreshold
//	F2 (HIGH)     decode DoS via attacker-controlled SignerCount
//	F3 (HIGH)     Verify trusting an opaque caller message (role/position replay)
//	F4 (MED)      weighted-Merkle (LeafIndex,LeafCount) proof malleability
//	F5 (MED)      mixed-scheme FIPS-context asymmetry
package quasar

import (
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
)

// ----------------------------------------------------------------------------
// F1 (CRITICAL) — QuorumThreshold is bound; sub-quorum finality forgery closed.
// ----------------------------------------------------------------------------

// TestRedteam_F1_ForgedBelowFloorThresholdRejected reproduces the headline
// PoC: a real but SUB-QUORUM signer set (Σweight=40) is assembled into a cert
// that asserts its OWN low QuorumThreshold=40, while the chain BFT quorum is
// 67. The mandatory verifier floor (MinThreshold) rejects it — a cert may not
// assert its own security parameter.
func TestRedteam_F1_ForgedBelowFloorThresholdRejected(t *testing.T) {
	// 4 validators, weight 10 each → Σ = 40. The forger sets QuorumThreshold=40.
	signers := []*testSigner{
		newMLDSASigner(t, 0x01, 10),
		newMLDSASigner(t, 0x02, 10),
		newMLDSASigner(t, 0x03, 10),
		newMLDSASigner(t, 0x04, 10),
	}
	sc := buildScenario(t, signers, 40, nil) // cert.QuorumThreshold = 40

	// The chain's real BFT quorum floor is 67.
	cfg := sc.cfg
	cfg.MinThreshold = 67

	err := sc.cert.Verify(sc.env, cfg)
	if !errors.Is(err, ErrQCThresholdBelowFloor) {
		t.Fatalf("forged below-floor threshold err = %v, want ErrQCThresholdBelowFloor", err)
	}
}

// TestRedteam_F1_MinThresholdUnsetFailsClosed proves the floor is MANDATORY: a
// verifier that forgets to pin MinThreshold (zero) does NOT silently accept —
// it fails closed. This is the defence against a caller accidentally trusting
// the cert's self-asserted threshold.
func TestRedteam_F1_MinThresholdUnsetFailsClosed(t *testing.T) {
	sc := fourMLDSA(t) // a perfectly valid cert
	cfg := sc.cfg
	cfg.MinThreshold = 0 // operator forgot to pin the floor

	err := sc.cert.Verify(sc.env, cfg)
	if !errors.Is(err, ErrQCMinThresholdUnset) {
		t.Fatalf("unset MinThreshold err = %v, want ErrQCMinThresholdUnset (fail closed)", err)
	}
}

// TestRedteam_F1_ThresholdBoundIntoSignedMessage proves the SIGNED-message
// half of the fix: a signature produced under threshold T1 does NOT verify
// once the cert claims a different threshold T2. The honest signers signed the
// real quorum (T1); re-presenting their signatures under a lowered T2 makes
// the verifier rebuild the message at T2, and the T1 signatures fail the FIPS
// check. This holds even with NO floor reliance (MinThreshold below both).
func TestRedteam_F1_ThresholdBoundIntoSignedMessage(t *testing.T) {
	const t1 = 100 // honest quorum the signers actually signed under
	const t2 = 40  // forged lowered threshold

	sc := fourMLDSA(t) // signers signed under threshold == 100 (env.QuorumThreshold)
	if sc.cert.QuorumThreshold != t1 {
		t.Fatalf("precondition: cert threshold = %d, want %d", sc.cert.QuorumThreshold, t1)
	}

	// Forge: lower the cert's asserted threshold to t2 and recompute the
	// commitment so the commitment clause is NOT what trips (we want to prove
	// the SIGNATURE binding specifically).
	forged := *sc.cert
	forged.QuorumThreshold = t2
	forged.SignerCommitment = forged.computeSignerCommitment()

	// Use a floor at or below t2 so the floor does NOT reject — isolating the
	// message-binding defence.
	cfg := sc.cfg
	cfg.MinThreshold = t2

	err := forged.Verify(sc.env, cfg)
	if !errors.Is(err, ErrQCSigInvalid) {
		t.Fatalf("threshold-swapped cert err = %v, want ErrQCSigInvalid (message binding)", err)
	}

	// Cross-check the binding at the message layer directly: the message for t1
	// and the message for t2 must differ (so a t1 signature can never be a t2
	// signature).
	envT2 := sc.env
	envT2.QuorumThreshold = t2
	msgT2, err := QuorumConsensusMessage(envT2)
	if err != nil {
		t.Fatalf("build t2 message: %v", err)
	}
	if string(msgT2) == string(sc.message) {
		t.Fatal("quorum_threshold is not bound into the signed message (t1 and t2 messages collide)")
	}
}

// ----------------------------------------------------------------------------
// F2 (HIGH) — decode DoS via attacker-controlled SignerCount closed.
// ----------------------------------------------------------------------------

// TestRedteam_F2_HugeSignerCountRejectedFast crafts a FULL-kind frame whose
// header declares SignerCount = 0xFFFFFFFF but carries no record bytes. The
// decoder must reject it in O(1) without attempting a ~446 GB reservation.
func TestRedteam_F2_HugeSignerCountRejectedFast(t *testing.T) {
	// Build a minimal valid header, then overwrite kind=full and
	// signer_count=0xFFFFFFFF, and truncate to exactly the header so there are
	// zero record bytes available.
	sc := fourMLDSA(t)
	wire, err := sc.cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Take just the fixed compact header prefix and flip it to a full frame
	// with a maxed-out signer count. The header is wqcHeaderSize bytes; its
	// last 4 bytes are signer_count.
	hdr := append([]byte(nil), wire[:wqcHeaderSize]...)
	hdr[0] = wqcKindFull // kind = full
	binary.BigEndian.PutUint32(hdr[wqcHeaderSize-4:], 0xFFFFFFFF)

	done := make(chan struct{})
	var derr error
	go func() {
		_, derr = UnmarshalWeightedQuorumCert(hdr)
		close(done)
	}()

	select {
	case <-done:
		// Must be a clean wire-corrupt rejection (the count cannot fit the
		// remaining zero bytes), not a panic / OOM.
		if !errors.Is(derr, ErrQCWireCorrupt) {
			t.Fatalf("huge signer_count err = %v, want ErrQCWireCorrupt", derr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("decode of 0xFFFFFFFF signer_count did not return promptly (DoS not capped)")
	}
}

// TestRedteam_F2_SignerCountExceedingBufferRejected pins the exact cap: a
// signer_count whose minimum on-wire footprint exceeds the remaining buffer is
// rejected before allocation, even for modest-but-impossible counts.
func TestRedteam_F2_SignerCountExceedingBufferRejected(t *testing.T) {
	sc := fourMLDSA(t)
	wire, _ := sc.cert.MarshalBinary()

	hdr := append([]byte(nil), wire[:wqcHeaderSize]...)
	hdr[0] = wqcKindFull
	// One record needs ≥ wqcMinRecordBytes; with zero trailing bytes even a
	// count of 1 is impossible.
	binary.BigEndian.PutUint32(hdr[wqcHeaderSize-4:], 1)

	if _, err := UnmarshalWeightedQuorumCert(hdr); !errors.Is(err, ErrQCWireCorrupt) {
		t.Fatalf("count=1 over empty record region err = %v, want ErrQCWireCorrupt", err)
	}
}

// ----------------------------------------------------------------------------
// F3 (HIGH) — Verify rebuilds the position binding itself; opaque-message and
// proof-backend-axis replay closed.
// ----------------------------------------------------------------------------

// TestRedteam_F3_PositionTamperRejected mutates EACH consensus-position axis
// the message binds (chain_id, height, round, qc_type, value_hash), recomputes
// the signer commitment so the commitment clause is satisfied, and confirms
// the envelope-based Verify rejects every one — because it rebuilds the signing
// message from the cert's own (tampered) fields and the original signatures no
// longer verify. A QCType prepare→commit role-replay is included.
func TestRedteam_F3_PositionTamperRejected(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*WeightedQuorumCert)
	}{
		{"chain_id", func(c *WeightedQuorumCert) { c.ChainID++ }},
		{"height", func(c *WeightedQuorumCert) { c.Height++ }},
		{"round", func(c *WeightedQuorumCert) { c.Round++ }},
		{"qc_type_role_replay", func(c *WeightedQuorumCert) { c.QCType++ }},
		{"value_hash", func(c *WeightedQuorumCert) { c.ValueHash[0] ^= 0xFF }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc := fourMLDSA(t)
			tc.mutate(sc.cert)
			// Satisfy the commitment clause so the rejection is provably the
			// rebuilt-message signature check, not the commitment.
			sc.cert.SignerCommitment = sc.cert.computeSignerCommitment()

			err := sc.cert.Verify(sc.env, sc.cfg)
			if !errors.Is(err, ErrQCSigInvalid) {
				t.Fatalf("%s tamper err = %v, want ErrQCSigInvalid (verifier rebuilds position binding)", tc.name, err)
			}
		})
	}
}

// TestRedteam_F3_VerifyOwnsMessageConstruction proves Verify never trusts a
// caller-supplied message: there is no public Verify form that accepts raw
// bytes. The envelope-based Verify rebuilds the message from the cert, so a
// caller cannot hand it a message minted for a DIFFERENT position. We assert
// the cert verifies under its real envelope and is rejected under an envelope
// whose ValidatorSetRoot axis the verifier would otherwise have to trust.
func TestRedteam_F3_VerifyOwnsMessageConstruction(t *testing.T) {
	sc := fourMLDSA(t)
	if err := sc.cert.Verify(sc.env, sc.cfg); err != nil {
		t.Fatalf("valid cert rejected under its real envelope: %v", err)
	}

	// A second, independent validator set the attacker would like the cert to
	// be read against. The cert pins its OWN ValidatorSetRoot, so Verify binds
	// that root into the rebuilt message regardless of the envelope's stale
	// root — but the envelope's *posture* axes still feed the round digest.
	// Flip the envelope's NetworkID (a round-digest axis): the rebuilt message
	// changes and the signatures fail. The caller cannot smuggle in a message
	// for a different network.
	wrongEnv := sc.env
	wrongEnv.NetworkID++
	err := sc.cert.Verify(wrongEnv, sc.cfg)
	if !errors.Is(err, ErrQCSigInvalid) {
		t.Fatalf("cross-network envelope err = %v, want ErrQCSigInvalid", err)
	}
}

// TestRedteam_F3_ProofBackendAxisMismatchRejected folds in the LOW finding:
// Verify asserts the envelope's proof-backend axis equals the cert's
// ProofBackendID(). An envelope naming a different backend is rejected before
// any message is built — a direct weighted-quorum cert cannot be verified
// under a STARK backend's envelope.
func TestRedteam_F3_ProofBackendAxisMismatchRejected(t *testing.T) {
	sc := fourMLDSA(t)
	wrongEnv := sc.env
	wrongEnv.ProofBackend = config.ProofBackendP3QSTARKFRISHA3 // not the cert's backend

	err := sc.cert.Verify(wrongEnv, sc.cfg)
	if !errors.Is(err, ErrQCProofBackendMismatch) {
		t.Fatalf("proof-backend mismatch err = %v, want ErrQCProofBackendMismatch", err)
	}
}

// ----------------------------------------------------------------------------
// F4 (MED) — weighted-Merkle (LeafIndex,LeafCount) proof malleability closed.
// ----------------------------------------------------------------------------

// TestRedteam_F4_NonCanonicalLeafEncodingRejected reproduces the proof-shape
// ambiguity: weightedProofShape gives the IDENTICAL shape ("RR") for a leaf at
// index 0 whether LeafCount is 3 or 4, so a record's (LeafIndex,LeafCount) has
// multiple encodings that verify against the same root with the same sibling
// bytes. Left unbound, the same logical cert would have byte-distinct
// encodings (breaking dedup / equivocation detection). The fix binds
// (LeafIndex,LeafCount) into the signer commitment, so swapping LeafCount
// 3→4 changes the commitment and the cert is rejected.
func TestRedteam_F4_NonCanonicalLeafEncodingRejected(t *testing.T) {
	// 3 ML-DSA signers → the committed set has 3 leaves; the index-0 record's
	// canonical proof carries LeafCount=3 with shape "RR".
	signers := []*testSigner{
		newMLDSASigner(t, 0x01, 40),
		newMLDSASigner(t, 0x02, 40),
		newMLDSASigner(t, 0x03, 40),
	}
	sc := buildScenario(t, signers, 100, nil)
	if err := sc.cert.Verify(sc.env, sc.cfg); err != nil {
		t.Fatalf("baseline 3-signer cert rejected: %v", err)
	}

	// Find the index-0 record (lowest validator id after canonical sort).
	rec0 := -1
	for i := range sc.cert.Signers {
		if sc.cert.Signers[i].MerklePath.LeafIndex == 0 {
			rec0 = i
			break
		}
	}
	if rec0 < 0 {
		t.Fatal("no index-0 record found")
	}
	if got := sc.cert.Signers[rec0].MerklePath.LeafCount; got != 3 {
		t.Fatalf("precondition: index-0 LeafCount = %d, want 3", got)
	}

	// Confirm the SHAPE ambiguity is real: (0,3) and (0,4) produce the same
	// promotion/orientation sequence, so the Merkle verifier alone cannot
	// distinguish them. This is exactly why the binding must live in the cert
	// commitment.
	shape3 := weightedProofShape(0, 3)
	shape4 := weightedProofShape(0, 4)
	if len(shape3) != len(shape4) {
		t.Fatalf("shape lengths differ (%d vs %d); ambiguity premise false", len(shape3), len(shape4))
	}
	for i := range shape3 {
		if shape3[i] != shape4[i] {
			t.Fatalf("shapes differ at level %d; ambiguity premise false", i)
		}
	}

	// Malleate: claim LeafCount=4 for the index-0 record WITHOUT touching the
	// sibling bytes. The Merkle recomputation still succeeds (same shape, same
	// siblings → same root), so this would slip past the Merkle clause — but
	// the commitment now binds LeafCount, so recomputing it over the malleated
	// record yields a value != the stored commitment.
	sc.cert.Signers[rec0].MerklePath.LeafCount = 4

	// The Merkle clause must still PASS for this record (proving the malleation
	// is NOT caught by the Merkle math — only by the commitment binding).
	if !VerifyWeightedInclusion(
		sc.cert.ValidatorSetRoot, sc.cert.Epoch,
		sc.cert.Signers[rec0].leaf(), sc.cert.Signers[rec0].MerklePath,
	) {
		t.Fatal("precondition: malleated (0,4) proof must still satisfy the Merkle clause")
	}

	// Verify with the ORIGINAL (un-recomputed) commitment → the bound
	// LeafCount no longer matches → ErrQCSignerCommitment.
	err := sc.cert.Verify(sc.env, sc.cfg)
	if !errors.Is(err, ErrQCSignerCommitment) {
		t.Fatalf("malleated LeafCount err = %v, want ErrQCSignerCommitment", err)
	}
}

// ----------------------------------------------------------------------------
// F5 (MED) — mixed-scheme FIPS-context asymmetry closed.
// ----------------------------------------------------------------------------

// TestRedteam_F5_MixedSchemeNonEmptyContext proves a mixed ML-DSA ∧ SLH-DSA
// cert now verifies even when cfg.Context is NON-empty. The ML-DSA records
// were signed under ctx; the SLH-DSA records were signed under the empty
// FIPS-205 context (magnetar.ValidatorSign). The per-scheme context selection
// (contextForScheme) applies ctx to ML-DSA and the empty context to SLH-DSA,
// so both legs verify — previously a non-nil cfg.Context made the SLH-DSA leg
// reject and mixed certs only worked with ctx=nil.
func TestRedteam_F5_MixedSchemeNonEmptyContext(t *testing.T) {
	ctx := []byte("lux-quasar-wqc-mixed-ctx")

	// Build a mixed cert. The ML-DSA signers must sign under ctx; the SLH-DSA
	// signer signs under the empty context (its testSigner.sign path uses
	// ValidatorSign regardless of ctx).
	mldsa1 := newMLDSASigner(t, 0x01, 40)
	slh := newSLHDSASigner(t, 0x02, 40)
	mldsa3 := newMLDSASigner(t, 0x03, 40)

	// buildScenario signs each signer over the message under the provided ctx;
	// the ML-DSA path binds ctx, the SLH-DSA path ignores it (empty FIPS-205
	// context). This mirrors production: a chain may run a non-empty ML-DSA
	// context alongside SLH-DSA validators.
	sc := buildScenario(t, []*testSigner{mldsa1, slh, mldsa3}, 100, ctx)

	cfg := sc.cfg // Context = ctx, MinThreshold = 100
	if err := sc.cert.Verify(sc.env, cfg); err != nil {
		t.Fatalf("mixed-scheme cert under non-empty ctx rejected: %v", err)
	}

	// Sanity: contextForScheme routes the empty context to the SLH-DSA leg and
	// ctx to the ML-DSA leg.
	if got := contextForScheme(cfg, QuorumSchemeSLHDSA192s); got != nil {
		t.Fatalf("SLH-DSA context = %v, want nil (empty FIPS-205 ctx)", got)
	}
	if got := contextForScheme(cfg, QuorumSchemeMLDSA65); string(got) != string(ctx) {
		t.Fatalf("ML-DSA context = %q, want %q", got, ctx)
	}
}
