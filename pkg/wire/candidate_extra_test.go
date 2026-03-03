// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wire

import (
	"testing"
)

func TestVoterIDFromPublicKey(t *testing.T) {
	pk := []byte("test-public-key-data")
	id := VoterIDFromPublicKey(pk)

	// Should be deterministic
	id2 := VoterIDFromPublicKey(pk)
	if id != id2 {
		t.Error("VoterIDFromPublicKey should be deterministic")
	}

	// Should equal DeriveVoterID with NodeIDDomain
	expected := DeriveVoterID(NodeIDDomain, pk)
	if id != expected {
		t.Error("VoterIDFromPublicKey should use NodeIDDomain")
	}

	// Different key -> different ID
	id3 := VoterIDFromPublicKey([]byte("other-key"))
	if id == id3 {
		t.Error("different keys should produce different IDs")
	}
}

func TestCandidateIDBytes(t *testing.T) {
	data := []byte("test")
	id := DeriveItemID(data)
	b := id.Bytes()
	if len(b) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(b))
	}
	// Verify bytes match the array
	for i := 0; i < 32; i++ {
		if b[i] != id[i] {
			t.Errorf("byte mismatch at index %d", i)
		}
	}
}

func TestVoterIDBytes(t *testing.T) {
	id := DeriveVoterID("test", []byte("data"))
	b := id.Bytes()
	if len(b) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(b))
	}
	for i := 0; i < 32; i++ {
		if b[i] != id[i] {
			t.Errorf("byte mismatch at index %d", i)
		}
	}
}

func TestUint64ToBytesAndBack(t *testing.T) {
	tests := []uint64{0, 1, 255, 256, 65535, 1<<32 - 1, 1<<64 - 1}
	for _, n := range tests {
		b := Uint64ToBytes(n)
		if len(b) != 8 {
			t.Errorf("expected 8 bytes, got %d", len(b))
		}
		got := BytesToUint64(b)
		if got != n {
			t.Errorf("roundtrip failed: %d != %d", got, n)
		}
	}
}

func TestBytesToUint64Short(t *testing.T) {
	// Less than 8 bytes should return 0
	if BytesToUint64(nil) != 0 {
		t.Error("nil should return 0")
	}
	if BytesToUint64([]byte{1, 2, 3}) != 0 {
		t.Error("short slice should return 0")
	}
	if BytesToUint64([]byte{0, 0, 0, 0, 0, 0, 0}) != 0 {
		t.Error("7 bytes should return 0")
	}
}

func TestSignatureSchemeNoSignature(t *testing.T) {
	vote := &Vote{}
	if vote.SignatureScheme() != SigNone {
		t.Errorf("empty signature should return SigNone, got %d", vote.SignatureScheme())
	}
}

func TestCandidateVerifyTampered(t *testing.T) {
	c := NewCandidate([]byte("domain"), []byte("payload"), EmptyCandidateID, 1)
	if !c.Verify() {
		t.Error("fresh candidate should verify")
	}

	// Tamper with payload
	c.Payload = []byte("tampered")
	if c.Verify() {
		t.Error("tampered candidate should not verify")
	}
}

func TestResultSerialization(t *testing.T) {
	r := &Result{
		ItemID:     DeriveItemID([]byte("test")),
		Finalized:  true,
		Accepted:   true,
		Confidence: 95,
		Signatures: [][]byte{{1, 2, 3}},
		Synthesis:  "agreed",
	}

	data, err := MarshalResult(r)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	r2, err := UnmarshalResult(data)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if r2.ItemID != r.ItemID {
		t.Error("ItemID mismatch")
	}
	if !r2.Finalized {
		t.Error("Finalized mismatch")
	}
	if !r2.Accepted {
		t.Error("Accepted mismatch")
	}
	if r2.Confidence != 95 {
		t.Errorf("Confidence mismatch: %d", r2.Confidence)
	}
	if r2.Synthesis != "agreed" {
		t.Error("Synthesis mismatch")
	}

	// Invalid JSON
	_, err = UnmarshalResult([]byte("{bad"))
	if err == nil {
		t.Error("should fail on invalid JSON")
	}
}

func TestValidatorSetSerialization(t *testing.T) {
	vs := &ValidatorSet{
		Epoch: 5,
		Validators: []Validator{
			{ID: DeriveVoterID("node", []byte("a")), Weight: 100, PublicKey: []byte("pk1")},
			{ID: DeriveVoterID("node", []byte("b")), Weight: 200, PublicKey: []byte("pk2")},
		},
		TotalWeight: 300,
	}

	data, err := MarshalValidatorSet(vs)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	vs2, err := UnmarshalValidatorSet(data)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if vs2.Epoch != 5 {
		t.Error("Epoch mismatch")
	}
	if len(vs2.Validators) != 2 {
		t.Errorf("expected 2 validators, got %d", len(vs2.Validators))
	}
	if vs2.TotalWeight != 300 {
		t.Error("TotalWeight mismatch")
	}

	// Invalid JSON
	_, err = UnmarshalValidatorSet([]byte("bad"))
	if err == nil {
		t.Error("should fail on invalid JSON")
	}
}

func TestUnmarshalCandidateInvalid(t *testing.T) {
	_, err := UnmarshalCandidate([]byte("{invalid"))
	if err == nil {
		t.Error("should fail on invalid JSON")
	}
}

func TestUnmarshalVoteInvalid(t *testing.T) {
	_, err := UnmarshalVote([]byte("bad"))
	if err == nil {
		t.Error("should fail on invalid JSON")
	}
}

func TestUnmarshalCertificateInvalid(t *testing.T) {
	_, err := UnmarshalCertificate([]byte("bad"))
	if err == nil {
		t.Error("should fail on invalid JSON")
	}
}

func TestCredentialDeserializeTruncatedSigLen(t *testing.T) {
	// Type + numSigs=1, but no sig length bytes
	data := []byte{CredentialTypeMLDSA65, 0, 1}
	_, err := DeserializeCredential(data)
	if err == nil {
		t.Error("should fail on truncated sig length")
	}
}

func TestCredentialDeserializeTruncatedSigData(t *testing.T) {
	// Type + numSigs=1 + sigLen=100, but only 2 bytes of data
	data := []byte{CredentialTypeMLDSA65, 0, 1, 0, 100, 1, 2}
	_, err := DeserializeCredential(data)
	if err == nil {
		t.Error("should fail on truncated sig data")
	}
}

func TestCredentialDeserializeTooShort(t *testing.T) {
	_, err := DeserializeCredential([]byte{1})
	if err != ErrCredentialTooShort {
		t.Errorf("expected ErrCredentialTooShort, got %v", err)
	}
}

func TestCredentialValidateNonMLDSA(t *testing.T) {
	cred := NewCredential(CredentialTypeSecp256k1)
	cred.AddSignature([]byte{1, 2, 3}) // arbitrary size
	err := cred.ValidateSignatureSizes()
	if err != nil {
		t.Errorf("non-MLDSA type should accept any size: %v", err)
	}
}

func TestCredentialValidateMLDSA44(t *testing.T) {
	cred := NewCredential(CredentialTypeMLDSA44)
	cred.AddSignature(make([]byte, MLDSA44SignatureSize))
	if err := cred.ValidateSignatureSizes(); err != nil {
		t.Errorf("valid MLDSA44 sig should pass: %v", err)
	}

	cred.AddSignature(make([]byte, 10))
	if err := cred.ValidateSignatureSizes(); err == nil {
		t.Error("invalid MLDSA44 sig size should fail")
	}
}

func TestCredentialValidateMLDSA87(t *testing.T) {
	cred := NewCredential(CredentialTypeMLDSA87)
	cred.AddSignature(make([]byte, MLDSA87SignatureSize))
	if err := cred.ValidateSignatureSizes(); err != nil {
		t.Errorf("valid MLDSA87 sig should pass: %v", err)
	}
}

func TestSignatureSizeForTypeUnknown(t *testing.T) {
	if SignatureSizeForType(0xFF) != 0 {
		t.Error("unknown type should return 0")
	}
}

func TestPublicKeySizeForTypeUnknown(t *testing.T) {
	if PublicKeySizeForType(0xFF) != 0 {
		t.Error("unknown type should return 0")
	}
}

func TestIsPostQuantumBLS(t *testing.T) {
	cred := NewCredential(CredentialTypeBLS)
	if cred.IsPostQuantum() {
		t.Error("BLS should not be post-quantum")
	}
}

func TestIsPostQuantumEd25519(t *testing.T) {
	cred := NewCredential(CredentialTypeEd25519)
	if cred.IsPostQuantum() {
		t.Error("Ed25519 should not be post-quantum")
	}
}

func TestIsPostQuantumMLDSA44(t *testing.T) {
	cred := NewCredential(CredentialTypeMLDSA44)
	if !cred.IsPostQuantum() {
		t.Error("MLDSA44 should be post-quantum")
	}
}

func TestIsPostQuantumMLDSA87(t *testing.T) {
	cred := NewCredential(CredentialTypeMLDSA87)
	if !cred.IsPostQuantum() {
		t.Error("MLDSA87 should be post-quantum")
	}
}

func TestOutputOwnersIsPostQuantumFalse(t *testing.T) {
	o := &OutputOwners{AddressType: CredentialTypeSecp256k1}
	if o.IsPostQuantum() {
		t.Error("secp256k1 output owners should not be PQ")
	}
}

func TestOutputOwnersIsPostQuantumMLDSA44(t *testing.T) {
	o := &OutputOwners{AddressType: CredentialTypeMLDSA44}
	if !o.IsPostQuantum() {
		t.Error("MLDSA44 output owners should be PQ")
	}
}

func TestOutputOwnersIsPostQuantumMLDSA87(t *testing.T) {
	o := &OutputOwners{AddressType: CredentialTypeMLDSA87}
	if !o.IsPostQuantum() {
		t.Error("MLDSA87 output owners should be PQ")
	}
}
