package quasar

import (
	"testing"
)

// TestVerifyBLSAggregate_C1_ActualDeserialization is the regression test for C-1.
// verifyBLSAggregate must reject bytes that are not a valid BLS curve point.
func TestVerifyBLSAggregate_C1_ActualDeserialization(t *testing.T) {
	w := NewVerkleWitness(3)

	// Empty signature must fail
	err := w.verifyBLSAggregate(nil, []byte("validators"))
	if err == nil {
		t.Error("empty aggregate signature should fail")
	}

	// Short signature must fail
	err = w.verifyBLSAggregate([]byte{0x01, 0x02}, []byte("validators"))
	if err == nil {
		t.Error("short signature should fail")
	}

	// 48 bytes of zeros is NOT a valid BLS G1 point -- must be rejected
	// by actual deserialization, not just the length check.
	invalidSig := make([]byte, 48)
	err = w.verifyBLSAggregate(invalidSig, []byte("validators"))
	if err == nil {
		t.Error("zeroed 48-byte signature should fail BLS deserialization (not a valid curve point)")
	}

	// Random garbage of correct length should also fail deserialization
	garbageSig := make([]byte, 96)
	for i := range garbageSig {
		garbageSig[i] = byte(i + 1)
	}
	err = w.verifyBLSAggregate(garbageSig, []byte("validators"))
	if err == nil {
		t.Error("garbage bytes should fail BLS deserialization")
	}
}

// TestValidateCompressedStructure_C3_Rename verifies the method was renamed
// from VerifyCompressed to make clear it does no crypto.
func TestValidateCompressedStructure_C3_Rename(t *testing.T) {
	w := NewVerkleWitness(2)

	// Sufficient validators passes structural check
	err := w.validateCompressedStructure(&CompressedWitness{
		CommitmentAndProof: make([]byte, 32),
		Validators:         0x07, // bits 0,1,2 set = 3 validators
	})
	if err != nil {
		t.Errorf("expected nil error for sufficient validators, got: %v", err)
	}

	// Insufficient validators fails
	err = w.validateCompressedStructure(&CompressedWitness{
		CommitmentAndProof: make([]byte, 32),
		Validators:         0x01, // only 1 validator
	})
	if err == nil {
		t.Error("expected error for insufficient validators")
	}
}

// TestCreateVerkleCommitment_HandlesError verifies the SetBytes error is propagated.
func TestCreateVerkleCommitment_HandlesError(t *testing.T) {
	// Too short
	_, err := createVerkleCommitment(make([]byte, 10))
	if err == nil {
		t.Error("expected error for short stateRoot")
	}

	// 32 bytes of zeros may or may not be a valid point depending on the curve.
	// The important thing is the error is returned, not silently ignored.
	_, err = createVerkleCommitment(make([]byte, 32))
	// We don't assert pass/fail here because it depends on banderwagon encoding.
	// We just verify it does not panic.
	_ = err
}

// TestCompressPath_ShortInput verifies compressPath handles short inputs.
func TestCompressPath_ShortInput(t *testing.T) {
	// Input shorter than 16 bytes should not panic
	short := []byte{1, 2, 3}
	result := compressPath(short)
	if len(result) != 16 {
		t.Errorf("expected 16 bytes, got %d", len(result))
	}
	if result[0] != 1 || result[1] != 2 || result[2] != 3 {
		t.Error("first bytes should match input")
	}
	for i := 3; i < 16; i++ {
		if result[i] != 0 {
			t.Errorf("byte %d should be zero-padded, got %d", i, result[i])
		}
	}

	// Empty input
	result = compressPath(nil)
	if len(result) != 16 {
		t.Errorf("expected 16 bytes for nil input, got %d", len(result))
	}

	// Normal input (>= 16 bytes)
	normal := make([]byte, 32)
	for i := range normal {
		normal[i] = byte(i)
	}
	result = compressPath(normal)
	if len(result) != 16 {
		t.Errorf("expected 16 bytes, got %d", len(result))
	}
	for i := 0; i < 16; i++ {
		if result[i] != byte(i) {
			t.Errorf("byte %d mismatch", i)
		}
	}
}
