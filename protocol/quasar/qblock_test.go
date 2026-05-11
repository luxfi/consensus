// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"bytes"
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
)

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

// canonicalProfileID is the test ProfileID used by validBlock + validCtx.
// Until the ChainSecurityProfile agent lands its canonical registry, this
// is an arbitrary non-zero uint32 wired through both producer (the QBlock)
// and verifier (the AcceptanceContext) so the profile-match gate exercises.
const canonicalProfileID uint32 = 0xC57E0001

// fakeVerifier is a deterministic Pulsar-M verifier stub. It rejects a
// signature iff its first byte is FailByte. Tests flip that byte to simulate
// post-sign mutation; the rest of the bytes are opaque. No real ML-DSA work.
type fakeVerifier struct {
	FailByte byte
}

func (v fakeVerifier) Verify(transcriptHash [32]byte, sig []byte, groupPubKeyHash [32]byte) bool {
	if len(sig) == 0 {
		return false
	}
	return sig[0] != v.FailByte
}

// trackingVerifier records the transcripts it was asked to verify so tests
// can assert TranscriptHash() was actually consulted.
type trackingVerifier struct {
	seenTranscript [32]byte
	seenSig        []byte
	seenGroup      [32]byte
	result         bool
}

func (v *trackingVerifier) Verify(transcriptHash [32]byte, sig []byte, groupPubKeyHash [32]byte) bool {
	v.seenTranscript = transcriptHash
	v.seenSig = append([]byte(nil), sig...)
	v.seenGroup = groupPubKeyHash
	return v.result
}

// fakeThreshold is a deterministic ThresholdVerifier. Returns true iff the
// first byte of the bitmap commitment is below the threshold byte set on
// the verifier — used to simulate "below threshold" mutations.
type fakeThreshold struct {
	FailFirstByte byte
}

func (t fakeThreshold) BitmapMeetsThreshold(bitmap [48]byte, committeeRoot [48]byte) bool {
	if bitmap[0] == t.FailFirstByte {
		return false
	}
	return true
}

// validBlock returns a Q-Block that, paired with validCtx(), accepts on
// the strict-PQ policy. All bytes are deterministic so tests can flip one
// field at a time and observe TranscriptHash changes.
func validBlock() *QBlock {
	return &QBlock{
		Version:                   0x0001,
		ProfileID:                 canonicalProfileID,
		NetworkID:                 96369,
		ChainID:                   1,
		Height:                    100,
		RoundOrView:               7,
		ParentQBlockHash:          b32(0x01),
		StateRoot:              b48(0x02),
		ZChainStateRoot:           b48(0x03),
		ValidatorSetRoot:          b48(0x04),
		CommitteeRoot:             b48(0x05),
		DKGTranscriptRoot:         b48(0x06),
		GroupPublicKeyHash:        b48(0x07),
		PayloadRoot:               b48(0x08),
		DARoot:                    b48(0x09),
		ProofPolicyID:             config.ProofPolicySTARKFRISHA3PQ,
		ProofBackendID:            config.ProofBackendP3QSTARKFRISHA3,
		ProofFormatID:             config.ProofFormatP3QBinaryV1,
		VerifierID:                config.VerifierP3QSTARKFRISHA3PQ,
		HashSuiteID:               config.HashSuiteSHA3NIST,
		IdentitySchemeID:          config.IdentitySchemeMLDSA65,
		FinalitySchemeID:          config.SigSchemePulsarM65,
		SignerBitmapCommitment:    b48(0xAA),
		PulsarMThresholdSignature: append([]byte{0x55}, bytes.Repeat([]byte{0xaa}, 64)...),
	}
}

// validCtx returns an AcceptanceContext aligned with validBlock(). The
// Verifier returns true unless the sig's first byte is 0xDE.
func validCtx() AcceptanceContext {
	b := validBlock()
	return AcceptanceContext{
		Profile: SecurityProfile{
			ProfileID:        canonicalProfileID,
			HashSuiteID:      config.HashSuiteSHA3NIST,
			FinalitySchemeID: config.SigSchemePulsarM65,
			ProofPolicyID:    config.ProofPolicySTARKFRISHA3PQ,
			ProofBackendID:   config.ProofBackendP3QSTARKFRISHA3,
		},
		EpochCommitment: EpochCommitment{
			ZChainStateRoot:    b.ZChainStateRoot,
			ValidatorSetRoot:   b.ValidatorSetRoot,
			CommitteeRoot:      b.CommitteeRoot,
			DKGTranscriptRoot:  b.DKGTranscriptRoot,
			GroupPublicKeyHash: b.GroupPublicKeyHash,
		},
		ChainTipHash:      b.ParentQBlockHash,
		NetworkPolicy:     NetworkPolicyStrictPQ,
		ThresholdVerifier: fakeThreshold{FailFirstByte: 0xDE},
		Verifier:          fakeVerifier{FailByte: 0xDE},
	}
}

// b32 produces a 32-byte array filled with a single byte. Test fixture only.
func b32(v byte) [32]byte {
	var out [32]byte
	for i := range out {
		out[i] = v
	}
	return out
}

// b48 produces a 48-byte array filled with a single byte. Test fixture only.
func b48(v byte) [48]byte {
	var out [48]byte
	for i := range out {
		out[i] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// Marshal / Unmarshal
// ---------------------------------------------------------------------------

func TestQBlock_MarshalRoundTrip(t *testing.T) {
	orig := validBlock()
	data, err := orig.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := UnmarshalQBlock(data)
	if err != nil {
		t.Fatalf("UnmarshalQBlock: %v", err)
	}

	if got.Version != orig.Version {
		t.Errorf("Version: got %d want %d", got.Version, orig.Version)
	}
	if got.ProfileID != orig.ProfileID {
		t.Errorf("ProfileID: got %d want %d", got.ProfileID, orig.ProfileID)
	}
	if got.NetworkID != orig.NetworkID {
		t.Errorf("NetworkID: got %d want %d", got.NetworkID, orig.NetworkID)
	}
	if got.ChainID != orig.ChainID {
		t.Errorf("ChainID: got %d want %d", got.ChainID, orig.ChainID)
	}
	if got.Height != orig.Height {
		t.Errorf("Height: got %d want %d", got.Height, orig.Height)
	}
	if got.RoundOrView != orig.RoundOrView {
		t.Errorf("RoundOrView: got %d want %d", got.RoundOrView, orig.RoundOrView)
	}
	if got.ParentQBlockHash != orig.ParentQBlockHash {
		t.Errorf("ParentQBlockHash mismatch")
	}
	if got.StateRoot != orig.StateRoot {
		t.Errorf("StateRoot mismatch")
	}
	if got.ZChainStateRoot != orig.ZChainStateRoot {
		t.Errorf("ZChainStateRoot mismatch")
	}
	if got.ValidatorSetRoot != orig.ValidatorSetRoot {
		t.Errorf("ValidatorSetRoot mismatch")
	}
	if got.CommitteeRoot != orig.CommitteeRoot {
		t.Errorf("CommitteeRoot mismatch")
	}
	if got.DKGTranscriptRoot != orig.DKGTranscriptRoot {
		t.Errorf("DKGTranscriptRoot mismatch")
	}
	if got.GroupPublicKeyHash != orig.GroupPublicKeyHash {
		t.Errorf("GroupPublicKeyHash mismatch")
	}
	if got.PayloadRoot != orig.PayloadRoot {
		t.Errorf("PayloadRoot mismatch")
	}
	if got.DARoot != orig.DARoot {
		t.Errorf("DARoot mismatch")
	}
	if got.SignerBitmapCommitment != orig.SignerBitmapCommitment {
		t.Errorf("SignerBitmapCommitment mismatch")
	}
	if got.ProofPolicyID != orig.ProofPolicyID {
		t.Errorf("ProofPolicyID: got %s want %s", got.ProofPolicyID, orig.ProofPolicyID)
	}
	if got.ProofBackendID != orig.ProofBackendID {
		t.Errorf("ProofBackendID: got %s want %s", got.ProofBackendID, orig.ProofBackendID)
	}
	if got.ProofFormatID != orig.ProofFormatID {
		t.Errorf("ProofFormatID: got %s want %s", got.ProofFormatID, orig.ProofFormatID)
	}
	if got.VerifierID != orig.VerifierID {
		t.Errorf("VerifierID: got %s want %s", got.VerifierID, orig.VerifierID)
	}
	if got.HashSuiteID != orig.HashSuiteID {
		t.Errorf("HashSuiteID: got %s want %s", got.HashSuiteID, orig.HashSuiteID)
	}
	if got.IdentitySchemeID != orig.IdentitySchemeID {
		t.Errorf("IdentitySchemeID: got %s want %s", got.IdentitySchemeID, orig.IdentitySchemeID)
	}
	if got.FinalitySchemeID != orig.FinalitySchemeID {
		t.Errorf("FinalitySchemeID: got %s want %s", got.FinalitySchemeID, orig.FinalitySchemeID)
	}
	if !bytes.Equal(got.PulsarMThresholdSignature, orig.PulsarMThresholdSignature) {
		t.Errorf("PulsarMThresholdSignature mismatch")
	}

	// Re-marshaling yields the same bytes (deterministic).
	again, err := got.Marshal()
	if err != nil {
		t.Fatalf("Re-Marshal: %v", err)
	}
	if !bytes.Equal(data, again) {
		t.Fatalf("non-deterministic encoding: first %d bytes, second %d", len(data), len(again))
	}
}

func TestQBlock_Unmarshal_RejectsTruncation(t *testing.T) {
	orig := validBlock()
	data, err := orig.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for cut := 0; cut < len(data); cut += 11 {
		if _, err := UnmarshalQBlock(data[:cut]); err == nil {
			t.Fatalf("UnmarshalQBlock accepted %d-byte truncation", cut)
		}
	}
}

// ---------------------------------------------------------------------------
// TranscriptHash — field-mutation table
// ---------------------------------------------------------------------------

func TestQBlock_TranscriptHash_Deterministic(t *testing.T) {
	b := validBlock()
	a := b.TranscriptHash()
	c := b.TranscriptHash()
	if a != c {
		t.Fatalf("non-deterministic: %x vs %x", a, c)
	}
}

// TestQBlock_TranscriptHash_BindsEveryField is the headline F34 closure
// test: flipping any single bound field MUST change the transcript digest.
// The mutation table is the formal contract — if you add a field to the
// QBlock spec, you MUST add a mutation here.
func TestQBlock_TranscriptHash_BindsEveryField(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*QBlock)
	}{
		{"Version", func(b *QBlock) { b.Version++ }},
		{"ProfileID", func(b *QBlock) { b.ProfileID++ }},
		{"NetworkID", func(b *QBlock) { b.NetworkID++ }},
		{"ChainID", func(b *QBlock) { b.ChainID++ }},
		{"Height", func(b *QBlock) { b.Height++ }},
		{"RoundOrView", func(b *QBlock) { b.RoundOrView++ }},
		{"ParentQBlockHash[0]", func(b *QBlock) { b.ParentQBlockHash[0]++ }},
		{"ParentQBlockHash[31]", func(b *QBlock) { b.ParentQBlockHash[31]++ }},
		{"StateRoot[0]", func(b *QBlock) { b.StateRoot[0]++ }},
		{"StateRoot[47]", func(b *QBlock) { b.StateRoot[47]++ }},
		{"ZChainStateRoot[0]", func(b *QBlock) { b.ZChainStateRoot[0]++ }},
		{"ZChainStateRoot[47]", func(b *QBlock) { b.ZChainStateRoot[47]++ }},
		{"ValidatorSetRoot[0]", func(b *QBlock) { b.ValidatorSetRoot[0]++ }},
		{"CommitteeRoot[0]", func(b *QBlock) { b.CommitteeRoot[0]++ }},
		{"DKGTranscriptRoot[0]", func(b *QBlock) { b.DKGTranscriptRoot[0]++ }},
		{"GroupPublicKeyHash[0]", func(b *QBlock) { b.GroupPublicKeyHash[0]++ }},
		{"PayloadRoot[0]", func(b *QBlock) { b.PayloadRoot[0]++ }},
		{"DARoot[0]", func(b *QBlock) { b.DARoot[0]++ }},
		{"SignerBitmapCommitment[0]", func(b *QBlock) { b.SignerBitmapCommitment[0]++ }},
		{"SignerBitmapCommitment[47]", func(b *QBlock) { b.SignerBitmapCommitment[47]++ }},
		{"HashSuiteID", func(b *QBlock) { b.HashSuiteID = config.HashSuiteBLAKE3Legacy }},
		{"IdentitySchemeID", func(b *QBlock) { b.IdentitySchemeID = config.IdentitySchemeMLDSA87 }},
		{"ProofPolicyID", func(b *QBlock) { b.ProofPolicyID = config.ProofPolicySTARKFRIKeccak }},
		{"ProofBackendID", func(b *QBlock) { b.ProofBackendID = config.ProofBackendSP1CompressedSTARK }},
		{"ProofFormatID", func(b *QBlock) { b.ProofFormatID = config.ProofFormatSP1BinaryV1 }},
		{"VerifierID", func(b *QBlock) { b.VerifierID = config.VerifierSP1CompressedSTARKPQ }},
		{"FinalitySchemeID", func(b *QBlock) { b.FinalitySchemeID = config.SigSchemePulsarM87 }},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			b1 := validBlock()
			h1 := b1.TranscriptHash()
			tc.mutate(b1)
			h2 := b1.TranscriptHash()
			if h1 == h2 {
				t.Fatalf("mutation of %s did not change digest: %x", tc.name, h1)
			}
		})
	}
}

// TestQBlock_TranscriptHash_DoesNotBindSignature ensures the signature
// itself is not in the transcript — the transcript IS what the signature
// is over. Including it would be circular.
func TestQBlock_TranscriptHash_DoesNotBindSignature(t *testing.T) {
	base := validBlock()
	baseHash := base.TranscriptHash()
	cc := *base
	cc.PulsarMThresholdSignature = append([]byte(nil), base.PulsarMThresholdSignature...)
	cc.PulsarMThresholdSignature[0] ^= 0xff
	if cc.TranscriptHash() != baseHash {
		t.Fatalf("signature byte leaked into transcript hash (circular binding)")
	}
}

// ---------------------------------------------------------------------------
// AcceptQBlock: happy path
// ---------------------------------------------------------------------------

func TestAcceptQBlock_HappyPath(t *testing.T) {
	b := validBlock()
	ctx := validCtx()
	if err := AcceptQBlock(b, ctx); err != nil {
		t.Fatalf("AcceptQBlock: %v", err)
	}
}

func TestAcceptQBlock_HappyPath_PulsarM87(t *testing.T) {
	b := validBlock()
	b.FinalitySchemeID = config.SigSchemePulsarM87
	ctx := validCtx()
	ctx.Profile.FinalitySchemeID = config.SigSchemePulsarM87
	if err := AcceptQBlock(b, ctx); err != nil {
		t.Fatalf("AcceptQBlock Pulsar-M-87 on mainnet: %v", err)
	}
}

func TestAcceptQBlock_VerifierIsConsulted(t *testing.T) {
	b := validBlock()
	tv := &trackingVerifier{result: true}
	ctx := validCtx()
	ctx.Verifier = tv
	if err := AcceptQBlock(b, ctx); err != nil {
		t.Fatalf("AcceptQBlock: %v", err)
	}
	if tv.seenTranscript != b.TranscriptHash() {
		t.Fatalf("Verifier received transcript %x, want %x", tv.seenTranscript, b.TranscriptHash())
	}
	if !bytes.Equal(tv.seenSig, b.PulsarMThresholdSignature) {
		t.Fatalf("Verifier received wrong signature")
	}
	var wantGroup [32]byte
	copy(wantGroup[:], b.GroupPublicKeyHash[:32])
	if tv.seenGroup != wantGroup {
		t.Fatalf("Verifier received wrong group pubkey hash")
	}
}

// ---------------------------------------------------------------------------
// AcceptQBlock: typed-error refusal cases
// ---------------------------------------------------------------------------

func TestAcceptQBlock_RefusesProfileMismatch(t *testing.T) {
	b := validBlock()
	b.ProfileID = canonicalProfileID + 1
	if err := AcceptQBlock(b, validCtx()); !errors.Is(err, ErrQBlockProfileMismatch) {
		t.Fatalf("got %v, want ErrQBlockProfileMismatch", err)
	}
}

func TestAcceptQBlock_RefusesHashSuiteMismatch(t *testing.T) {
	b := validBlock()
	ctx := validCtx()
	ctx.Profile.HashSuiteID = config.HashSuiteBLAKE3Legacy
	if err := AcceptQBlock(b, ctx); !errors.Is(err, ErrQBlockHashSuiteMismatch) {
		t.Fatalf("got %v, want ErrQBlockHashSuiteMismatch", err)
	}
}

func TestAcceptQBlock_RefusesFinalitySchemeMismatch(t *testing.T) {
	b := validBlock()
	ctx := validCtx()
	ctx.Profile.FinalitySchemeID = config.SigSchemePulsarM87
	if err := AcceptQBlock(b, ctx); !errors.Is(err, ErrQBlockFinalitySchemeMismatch) {
		t.Fatalf("got %v, want ErrQBlockFinalitySchemeMismatch", err)
	}
}

func TestAcceptQBlock_RefusesProofPolicyMismatch(t *testing.T) {
	b := validBlock()
	ctx := validCtx()
	ctx.Profile.ProofPolicyID = config.ProofPolicySTARKFRIKeccak
	if err := AcceptQBlock(b, ctx); !errors.Is(err, ErrQBlockProofPolicyMismatch) {
		t.Fatalf("got %v, want ErrQBlockProofPolicyMismatch", err)
	}
}

func TestAcceptQBlock_RefusesZChainStateRootMismatch(t *testing.T) {
	b := validBlock()
	ctx := validCtx()
	ctx.EpochCommitment.ZChainStateRoot[0]++
	if err := AcceptQBlock(b, ctx); !errors.Is(err, ErrQBlockZChainStateRootMismatch) {
		t.Fatalf("got %v, want ErrQBlockZChainStateRootMismatch", err)
	}
}

func TestAcceptQBlock_RefusesCommitteeRootMismatch(t *testing.T) {
	b := validBlock()
	ctx := validCtx()
	ctx.EpochCommitment.CommitteeRoot[0]++
	if err := AcceptQBlock(b, ctx); !errors.Is(err, ErrQBlockCommitteeRootMismatch) {
		t.Fatalf("got %v, want ErrQBlockCommitteeRootMismatch", err)
	}
}

func TestAcceptQBlock_RefusesDKGTranscriptRootMismatch(t *testing.T) {
	b := validBlock()
	ctx := validCtx()
	ctx.EpochCommitment.DKGTranscriptRoot[0]++
	if err := AcceptQBlock(b, ctx); !errors.Is(err, ErrQBlockDKGTranscriptRootMismatch) {
		t.Fatalf("got %v, want ErrQBlockDKGTranscriptRootMismatch", err)
	}
}

func TestAcceptQBlock_RefusesGroupKeyMismatch(t *testing.T) {
	b := validBlock()
	ctx := validCtx()
	ctx.EpochCommitment.GroupPublicKeyHash[0]++
	if err := AcceptQBlock(b, ctx); !errors.Is(err, ErrQBlockGroupKeyMismatch) {
		t.Fatalf("got %v, want ErrQBlockGroupKeyMismatch", err)
	}
}

func TestAcceptQBlock_RefusesValidatorSetRootMismatch(t *testing.T) {
	b := validBlock()
	ctx := validCtx()
	ctx.EpochCommitment.ValidatorSetRoot[0]++
	if err := AcceptQBlock(b, ctx); !errors.Is(err, ErrQBlockValidatorRootMismatch) {
		t.Fatalf("got %v, want ErrQBlockValidatorRootMismatch", err)
	}
}

func TestAcceptQBlock_RefusesSignerBitmapBelowThreshold(t *testing.T) {
	b := validBlock()
	b.SignerBitmapCommitment[0] = 0xDE // fakeThreshold will refuse.
	ctx := validCtx()
	if err := AcceptQBlock(b, ctx); !errors.Is(err, ErrQBlockSignerBitmapBelowThreshold) {
		t.Fatalf("got %v, want ErrQBlockSignerBitmapBelowThreshold", err)
	}
}

func TestAcceptQBlock_RefusesZeroSignerBitmap(t *testing.T) {
	b := validBlock()
	b.SignerBitmapCommitment = [48]byte{}
	ctx := validCtx()
	if err := AcceptQBlock(b, ctx); !errors.Is(err, ErrQBlockSignerBitmapBelowThreshold) {
		t.Fatalf("got %v, want ErrQBlockSignerBitmapBelowThreshold", err)
	}
}

func TestAcceptQBlock_PulsarMVerifyFail(t *testing.T) {
	b := validBlock()
	b.PulsarMThresholdSignature[0] = 0xDE // fakeVerifier will refuse.
	ctx := validCtx()
	if err := AcceptQBlock(b, ctx); !errors.Is(err, ErrQBlockPulsarMVerifyFail) {
		t.Fatalf("got %v, want ErrQBlockPulsarMVerifyFail", err)
	}
}

// ---------------------------------------------------------------------------
// AcceptQBlock: policy refusal cases
// ---------------------------------------------------------------------------

func TestAcceptQBlock_RefusesParentMismatch(t *testing.T) {
	b := validBlock()
	ctx := validCtx()
	ctx.ChainTipHash = b32(0xff)
	if err := AcceptQBlock(b, ctx); !errors.Is(err, ErrQBlockParentMismatch) {
		t.Fatalf("got %v, want ErrQBlockParentMismatch", err)
	}
}

func TestAcceptQBlock_RefusesGroth16(t *testing.T) {
	b := validBlock()
	b.ProofPolicyID = config.ProofPolicyGroth16BN254Forbid
	ctx := validCtx()
	// Match the profile so the rule under test is the strict-PQ forbidden
	// marker, not the profile-mismatch gate.
	ctx.Profile.ProofPolicyID = config.ProofPolicyGroth16BN254Forbid
	err := AcceptQBlock(b, ctx)
	if !errors.Is(err, ErrQBlockForbiddenProofSystem) {
		t.Fatalf("got %v, want ErrQBlockForbiddenProofSystem", err)
	}
}

func TestAcceptQBlock_StrictPQRefusesNonCanonicalProofSystem(t *testing.T) {
	b := validBlock()
	b.ProofPolicyID = config.ProofPolicySTARKFRIKeccak
	ctx := validCtx()
	ctx.Profile.ProofPolicyID = config.ProofPolicySTARKFRIKeccak
	err := AcceptQBlock(b, ctx)
	if !errors.Is(err, ErrQBlockProofSystemNotPQ) {
		t.Fatalf("got %v, want ErrQBlockProofSystemNotPQ", err)
	}
}

func TestAcceptQBlock_RefusesBLAKE3OnStrictPQ(t *testing.T) {
	b := validBlock()
	b.HashSuiteID = config.HashSuiteBLAKE3Legacy
	ctx := validCtx()
	ctx.Profile.HashSuiteID = config.HashSuiteBLAKE3Legacy
	err := AcceptQBlock(b, ctx)
	if !errors.Is(err, ErrQBlockHashSuiteRefused) {
		t.Fatalf("got %v, want ErrQBlockHashSuiteRefused", err)
	}
}

func TestAcceptQBlock_RefusesHashSuiteNone(t *testing.T) {
	b := validBlock()
	b.HashSuiteID = config.HashSuiteNone
	ctx := validCtx()
	ctx.Profile.HashSuiteID = config.HashSuiteNone
	err := AcceptQBlock(b, ctx)
	if !errors.Is(err, ErrQBlockHashSuiteRefused) {
		t.Fatalf("got %v, want ErrQBlockHashSuiteRefused", err)
	}
}

func TestAcceptQBlock_RefusesPulsarM44OnMainnet(t *testing.T) {
	b := validBlock()
	b.FinalitySchemeID = config.SigSchemePulsarM44
	ctx := validCtx()
	ctx.Profile.FinalitySchemeID = config.SigSchemePulsarM44
	err := AcceptQBlock(b, ctx)
	if !errors.Is(err, ErrQBlockSigSchemeRefused) {
		t.Fatalf("got %v, want ErrQBlockSigSchemeRefused", err)
	}
}

func TestAcceptQBlock_AcceptsPulsarM44OnPermissive(t *testing.T) {
	b := validBlock()
	b.FinalitySchemeID = config.SigSchemePulsarM44
	ctx := validCtx()
	ctx.Profile.FinalitySchemeID = config.SigSchemePulsarM44
	ctx.NetworkPolicy = NetworkPolicyPermissive
	if err := AcceptQBlock(b, ctx); err != nil {
		t.Fatalf("AcceptQBlock permissive Pulsar-M-44: %v", err)
	}
}

func TestAcceptQBlock_RefusesRawMLDSA(t *testing.T) {
	b := validBlock()
	b.FinalitySchemeID = config.SigSchemeMLDSA65
	ctx := validCtx()
	ctx.Profile.FinalitySchemeID = config.SigSchemeMLDSA65
	err := AcceptQBlock(b, ctx)
	if !errors.Is(err, ErrQBlockSigSchemeRefused) {
		t.Fatalf("got %v, want ErrQBlockSigSchemeRefused", err)
	}
}

func TestAcceptQBlock_RefusesEmptySignature(t *testing.T) {
	b := validBlock()
	b.PulsarMThresholdSignature = nil
	ctx := validCtx()
	err := AcceptQBlock(b, ctx)
	if !errors.Is(err, ErrQBlockPulsarMVerifyFail) {
		t.Fatalf("got %v, want ErrQBlockPulsarMVerifyFail", err)
	}
}

// ---------------------------------------------------------------------------
// AcceptanceContext invariants
// ---------------------------------------------------------------------------

func TestAcceptQBlock_RefusesNilBlock(t *testing.T) {
	if err := AcceptQBlock(nil, validCtx()); !errors.Is(err, ErrQBlockNilBlock) {
		t.Fatalf("got %v, want ErrQBlockNilBlock", err)
	}
}

func TestAcceptQBlock_RefusesMissingVerifier(t *testing.T) {
	ctx := validCtx()
	ctx.Verifier = nil
	if err := AcceptQBlock(validBlock(), ctx); !errors.Is(err, ErrQBlockMissingVerifier) {
		t.Fatalf("got %v, want ErrQBlockMissingVerifier", err)
	}
}

func TestAcceptQBlock_RefusesMissingThresholdVerifier(t *testing.T) {
	ctx := validCtx()
	ctx.ThresholdVerifier = nil
	if err := AcceptQBlock(validBlock(), ctx); !errors.Is(err, ErrQBlockMissingThresholdVerifier) {
		t.Fatalf("got %v, want ErrQBlockMissingThresholdVerifier", err)
	}
}

func TestAcceptQBlock_RefusesMissingNetworkPolicy(t *testing.T) {
	ctx := validCtx()
	ctx.NetworkPolicy = 0
	if err := AcceptQBlock(validBlock(), ctx); !errors.Is(err, ErrQBlockMissingNetworkPolicy) {
		t.Fatalf("got %v, want ErrQBlockMissingNetworkPolicy", err)
	}
}
