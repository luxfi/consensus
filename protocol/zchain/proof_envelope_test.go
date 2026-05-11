// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zchain

import (
	"bytes"
	"testing"

	"github.com/luxfi/consensus/config"
)

// fixtureEnvelope returns a fully-populated ZProofEnvelope for use as a
// happy-path baseline. Every field is non-zero so a missed field in
// Marshal / Unmarshal surfaces immediately.
func fixtureEnvelope() *ZProofEnvelope {
	return &ZProofEnvelope{
		Version:              1,
		ProfileID:            uint32(config.ProfileLuxStrictPQ),
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
		PublicInputsHash:     pad48(0x11),
		VerifierKeyHash:      pad48(0x22),
		ProgramOrAirID:       pad16(0x33),
		ProgramOrAirHash:     pad48(0x44),
		SoundnessBitsClaimed: 128,
		HashOutputBits:       384,
		TransparentSetup:     true,
		UsesPairings:         false,
		UsesKZG:              false,
		UsesTrustedSetup:     false,
		UsesClassicalSNARKWrapper: false,
		ProofBytes:           []byte("opaque-stark-proof-bytes"),
	}
}

func pad16(b byte) [16]byte {
	var out [16]byte
	for i := range out {
		out[i] = b
	}
	return out
}

func pad48(b byte) [48]byte {
	var out [48]byte
	for i := range out {
		out[i] = b
	}
	return out
}

// TestZProofEnvelope_Marshal_RoundTrip proves that Marshal followed by
// Unmarshal yields a byte-identical envelope.
func TestZProofEnvelope_Marshal_RoundTrip(t *testing.T) {
	orig := fixtureEnvelope()
	wire := orig.Marshal()
	if len(wire) == 0 {
		t.Fatalf("Marshal returned empty buffer")
	}

	decoded, err := UnmarshalZProofEnvelope(wire)
	if err != nil {
		t.Fatalf("UnmarshalZProofEnvelope returned %v", err)
	}

	// Re-marshal: the second encoding MUST match the first byte-for-byte.
	wire2 := decoded.Marshal()
	if !bytes.Equal(wire, wire2) {
		t.Fatalf("round-trip is non-deterministic: first=%d bytes second=%d bytes", len(wire), len(wire2))
	}

	// Field-by-field equality.
	if orig.Version != decoded.Version {
		t.Errorf("Version: %d != %d", orig.Version, decoded.Version)
	}
	if orig.ProfileID != decoded.ProfileID {
		t.Errorf("ProfileID: %d != %d", orig.ProfileID, decoded.ProfileID)
	}
	if orig.ChainID != decoded.ChainID {
		t.Errorf("ChainID: %d != %d", orig.ChainID, decoded.ChainID)
	}
	if orig.NetworkID != decoded.NetworkID {
		t.Errorf("NetworkID: %d != %d", orig.NetworkID, decoded.NetworkID)
	}
	if orig.Epoch != decoded.Epoch {
		t.Errorf("Epoch: %d != %d", orig.Epoch, decoded.Epoch)
	}
	if orig.ProofPolicyID != decoded.ProofPolicyID {
		t.Errorf("ProofPolicyID: %v != %v", orig.ProofPolicyID, decoded.ProofPolicyID)
	}
	if orig.ProofBackendID != decoded.ProofBackendID {
		t.Errorf("ProofBackendID: %v != %v", orig.ProofBackendID, decoded.ProofBackendID)
	}
	if orig.ProofFormatID != decoded.ProofFormatID {
		t.Errorf("ProofFormatID: %v != %v", orig.ProofFormatID, decoded.ProofFormatID)
	}
	if orig.VerifierID != decoded.VerifierID {
		t.Errorf("VerifierID: %v != %v", orig.VerifierID, decoded.VerifierID)
	}
	if orig.HashSuiteID != decoded.HashSuiteID {
		t.Errorf("HashSuiteID: %v != %v", orig.HashSuiteID, decoded.HashSuiteID)
	}
	if orig.IdentitySchemeID != decoded.IdentitySchemeID {
		t.Errorf("IdentitySchemeID: %v != %v", orig.IdentitySchemeID, decoded.IdentitySchemeID)
	}
	if orig.FinalitySchemeID != decoded.FinalitySchemeID {
		t.Errorf("FinalitySchemeID: %v != %v", orig.FinalitySchemeID, decoded.FinalitySchemeID)
	}
	if orig.PublicInputsHash != decoded.PublicInputsHash {
		t.Errorf("PublicInputsHash mismatch")
	}
	if orig.VerifierKeyHash != decoded.VerifierKeyHash {
		t.Errorf("VerifierKeyHash mismatch")
	}
	if orig.ProgramOrAirID != decoded.ProgramOrAirID {
		t.Errorf("ProgramOrAirID mismatch")
	}
	if orig.ProgramOrAirHash != decoded.ProgramOrAirHash {
		t.Errorf("ProgramOrAirHash mismatch")
	}
	if orig.SoundnessBitsClaimed != decoded.SoundnessBitsClaimed {
		t.Errorf("SoundnessBitsClaimed: %d != %d", orig.SoundnessBitsClaimed, decoded.SoundnessBitsClaimed)
	}
	if orig.HashOutputBits != decoded.HashOutputBits {
		t.Errorf("HashOutputBits: %d != %d", orig.HashOutputBits, decoded.HashOutputBits)
	}
	if orig.TransparentSetup != decoded.TransparentSetup {
		t.Errorf("TransparentSetup: %v != %v", orig.TransparentSetup, decoded.TransparentSetup)
	}
	if orig.UsesPairings != decoded.UsesPairings {
		t.Errorf("UsesPairings: %v != %v", orig.UsesPairings, decoded.UsesPairings)
	}
	if orig.UsesKZG != decoded.UsesKZG {
		t.Errorf("UsesKZG: %v != %v", orig.UsesKZG, decoded.UsesKZG)
	}
	if orig.UsesTrustedSetup != decoded.UsesTrustedSetup {
		t.Errorf("UsesTrustedSetup: %v != %v", orig.UsesTrustedSetup, decoded.UsesTrustedSetup)
	}
	if orig.UsesClassicalSNARKWrapper != decoded.UsesClassicalSNARKWrapper {
		t.Errorf("UsesClassicalSNARKWrapper: %v != %v", orig.UsesClassicalSNARKWrapper, decoded.UsesClassicalSNARKWrapper)
	}
	if !bytes.Equal(orig.ProofBytes, decoded.ProofBytes) {
		t.Errorf("ProofBytes: %v != %v", orig.ProofBytes, decoded.ProofBytes)
	}
}

// TestZProofEnvelope_Unmarshal_Truncated proves the decoder refuses a
// short buffer with a typed error rather than panicking.
func TestZProofEnvelope_Unmarshal_Truncated(t *testing.T) {
	orig := fixtureEnvelope().Marshal()
	for cut := 0; cut < len(orig); cut++ {
		_, err := UnmarshalZProofEnvelope(orig[:cut])
		if err == nil {
			t.Errorf("UnmarshalZProofEnvelope accepted %d-byte truncated input; want error", cut)
		}
	}
}

// TestZProofEnvelope_TranscriptHash_Determinism proves equal envelopes
// produce equal transcript hashes.
func TestZProofEnvelope_TranscriptHash_Determinism(t *testing.T) {
	a := fixtureEnvelope().TranscriptHash()
	b := fixtureEnvelope().TranscriptHash()
	if a != b {
		t.Errorf("TranscriptHash is non-deterministic across equal envelopes")
	}
}

// TestZProofEnvelope_TranscriptHash_FieldMutationChangesHash proves every
// envelope field is hash-bound. Mutating any field MUST change the hash.
// This is the property that makes a post-sign envelope mutation break
// signature verification at the threshold-sig layer.
func TestZProofEnvelope_TranscriptHash_FieldMutationChangesHash(t *testing.T) {
	base := fixtureEnvelope().TranscriptHash()

	cases := []struct {
		name   string
		mutate func(e *ZProofEnvelope)
	}{
		{"Version", func(e *ZProofEnvelope) { e.Version++ }},
		{"ProfileID", func(e *ZProofEnvelope) { e.ProfileID++ }},
		{"ChainID", func(e *ZProofEnvelope) { e.ChainID++ }},
		{"NetworkID", func(e *ZProofEnvelope) { e.NetworkID++ }},
		{"Epoch", func(e *ZProofEnvelope) { e.Epoch++ }},
		{"ProofPolicyID", func(e *ZProofEnvelope) { e.ProofPolicyID = config.ProofPolicySTARKFRIKeccak }},
		{"ProofBackendID", func(e *ZProofEnvelope) { e.ProofBackendID = config.ProofBackendSP1CompressedSTARK }},
		{"ProofFormatID", func(e *ZProofEnvelope) { e.ProofFormatID = config.ProofFormatSP1BinaryV1 }},
		{"VerifierID", func(e *ZProofEnvelope) { e.VerifierID = config.VerifierSP1CompressedSTARKPQ }},
		{"HashSuiteID", func(e *ZProofEnvelope) { e.HashSuiteID = config.HashSuiteBLAKE3Legacy }},
		{"IdentitySchemeID", func(e *ZProofEnvelope) { e.IdentitySchemeID = config.SigSchemeMLDSA87 }},
		{"FinalitySchemeID", func(e *ZProofEnvelope) { e.FinalitySchemeID = config.SigSchemePulsarM87 }},
		{"PublicInputsHash", func(e *ZProofEnvelope) { e.PublicInputsHash[0] ^= 0xFF }},
		{"VerifierKeyHash", func(e *ZProofEnvelope) { e.VerifierKeyHash[0] ^= 0xFF }},
		{"ProgramOrAirID", func(e *ZProofEnvelope) { e.ProgramOrAirID[0] ^= 0xFF }},
		{"ProgramOrAirHash", func(e *ZProofEnvelope) { e.ProgramOrAirHash[0] ^= 0xFF }},
		{"SoundnessBitsClaimed", func(e *ZProofEnvelope) { e.SoundnessBitsClaimed++ }},
		{"HashOutputBits", func(e *ZProofEnvelope) { e.HashOutputBits++ }},
		{"TransparentSetup", func(e *ZProofEnvelope) { e.TransparentSetup = !e.TransparentSetup }},
		{"UsesPairings", func(e *ZProofEnvelope) { e.UsesPairings = !e.UsesPairings }},
		{"UsesKZG", func(e *ZProofEnvelope) { e.UsesKZG = !e.UsesKZG }},
		{"UsesTrustedSetup", func(e *ZProofEnvelope) { e.UsesTrustedSetup = !e.UsesTrustedSetup }},
		{"UsesClassicalSNARKWrapper", func(e *ZProofEnvelope) { e.UsesClassicalSNARKWrapper = !e.UsesClassicalSNARKWrapper }},
		{"ProofBytes", func(e *ZProofEnvelope) { e.ProofBytes = []byte("different-proof-bytes") }},
	}
	for _, c := range cases {
		mutated := fixtureEnvelope()
		c.mutate(mutated)
		if mutated.TranscriptHash() == base {
			t.Errorf("mutating %s did not change TranscriptHash", c.name)
		}
	}
}

// TestHashZPublicInputs_Determinism proves equal inputs hash to equal
// bytes across calls.
func TestHashZPublicInputs_Determinism(t *testing.T) {
	in := fixturePublicInputs()
	a := HashZPublicInputs(in)
	b := HashZPublicInputs(in)
	if a != b {
		t.Errorf("HashZPublicInputs is non-deterministic across equal inputs")
	}
}

// TestHashZPublicInputs_FieldMutationChangesHash proves every public-input
// field is hash-bound. This is the test the parent task names by spec.
func TestHashZPublicInputs_FieldMutationChangesHash(t *testing.T) {
	base := HashZPublicInputs(fixturePublicInputs())

	cases := []struct {
		name   string
		mutate func(in *ZPublicInputs)
	}{
		{"Version", func(in *ZPublicInputs) { in.Version++ }},
		{"ProfileID", func(in *ZPublicInputs) { in.ProfileID++ }},
		{"NetworkID", func(in *ZPublicInputs) { in.NetworkID++ }},
		{"ChainID", func(in *ZPublicInputs) { in.ChainID++ }},
		{"Epoch", func(in *ZPublicInputs) { in.Epoch++ }},
		{"PreviousZStateRoot", func(in *ZPublicInputs) { in.PreviousZStateRoot[0] ^= 0xFF }},
		{"NewZStateRoot", func(in *ZPublicInputs) { in.NewZStateRoot[0] ^= 0xFF }},
		{"TxBatchHash", func(in *ZPublicInputs) { in.TxBatchHash[0] ^= 0xFF }},
		{"IdentityRoot", func(in *ZPublicInputs) { in.IdentityRoot[0] ^= 0xFF }},
		{"ValidatorRegistryRoot", func(in *ZPublicInputs) { in.ValidatorRegistryRoot[0] ^= 0xFF }},
		{"RevocationRoot", func(in *ZPublicInputs) { in.RevocationRoot[0] ^= 0xFF }},
		{"StakeWeightRoot", func(in *ZPublicInputs) { in.StakeWeightRoot[0] ^= 0xFF }},
		{"CommitteeRoot", func(in *ZPublicInputs) { in.CommitteeRoot[0] ^= 0xFF }},
		{"DKGTranscriptRoot", func(in *ZPublicInputs) { in.DKGTranscriptRoot[0] ^= 0xFF }},
		{"GroupPublicKeyHash", func(in *ZPublicInputs) { in.GroupPublicKeyHash[0] ^= 0xFF }},
		{"QChainTipHash", func(in *ZPublicInputs) { in.QChainTipHash[0] ^= 0xFF }},
		{"EpochCommitmentHash", func(in *ZPublicInputs) { in.EpochCommitmentHash[0] ^= 0xFF }},
		{"HashSuiteID", func(in *ZPublicInputs) { in.HashSuiteID = config.HashSuiteBLAKE3Legacy }},
		{"IdentitySchemeID", func(in *ZPublicInputs) { in.IdentitySchemeID = config.SigSchemeMLDSA87 }},
		{"FinalitySchemeID", func(in *ZPublicInputs) { in.FinalitySchemeID = config.SigSchemePulsarM87 }},
		{"ProofPolicyID", func(in *ZPublicInputs) { in.ProofPolicyID = config.ProofPolicySTARKFRIKeccak }},
	}
	for _, c := range cases {
		mutated := fixturePublicInputs()
		c.mutate(mutated)
		if HashZPublicInputs(mutated) == base {
			t.Errorf("mutating %s did not change HashZPublicInputs output", c.name)
		}
	}
}

// TestHashZPublicInputs_NilPanics proves the helper panics on nil. A
// nil input is a programmer error; returning a well-defined constant
// opens a constant-collision attack class (see F84).
func TestHashZPublicInputs_NilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("HashZPublicInputs(nil) did not panic")
		}
	}()
	_ = HashZPublicInputs(nil)
}

// fixturePublicInputs returns a fully-populated ZPublicInputs.
func fixturePublicInputs() *ZPublicInputs {
	return &ZPublicInputs{
		Version:               1,
		ProfileID:             uint32(config.ProfileLuxStrictPQ),
		NetworkID:             1,
		ChainID:               42,
		Epoch:                 7,
		PreviousZStateRoot:    pad48(0xA1),
		NewZStateRoot:         pad48(0xA2),
		TxBatchHash:           pad48(0xA3),
		IdentityRoot:          pad48(0xA4),
		ValidatorRegistryRoot: pad48(0xA5),
		RevocationRoot:        pad48(0xA6),
		StakeWeightRoot:       pad48(0xA7),
		CommitteeRoot:         pad48(0xA8),
		DKGTranscriptRoot:     pad48(0xA9),
		GroupPublicKeyHash:    pad48(0xAA),
		QChainTipHash:         pad48(0xAB),
		EpochCommitmentHash:   pad48(0xAC),
		HashSuiteID:           config.HashSuiteSHA3NIST,
		IdentitySchemeID:      config.SigSchemeMLDSA65,
		FinalitySchemeID:      config.SigSchemePulsarM65,
		ProofPolicyID:         config.ProofPolicySTARKFRISHA3PQ,
	}
}
