// Copyright (C) 2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.
//
// Fixture dumper for the C++ witness-verify port (luxcpp/consensus).
//
// Run with:
//   cd /Users/z/work/lux/consensus/protocol/quasar
//   QUASAR_DUMP=/Users/z/work/luxcpp/consensus/testdata \
//     GOWORK=off go test -count=1 -run TestDumpWitnessFixtures -v .
//
// Each fixture is one file named witness_<idx>_<label>.bin with layout:
//   magic[4]              = "QWFX"  (Quasar Witness FiXture)
//   abi_version[4 LE u32] = 1
//   flag[1]               = 0x01 if expected-valid, 0x00 if expected-invalid
//   _pad[3]               = zeros
//   group_key[48]         = compressed BLS12-381 G1
//   msg_len[8 LE u64]
//   msg[msg_len]
//   sig[96]               = compressed BLS12-381 G2
//
// C++ tests read these byte-for-byte; the Go side asserts each fixture
// verifies (or fails to verify) on the Go reference path, so any drift
// is caught on whichever side regenerates first.

package quasar

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/threshold"
)

const (
	fixtureMagic = "QWFX"
	fixtureABI   = uint32(1)
)

func writeFixture(t *testing.T, dir, name string, valid bool, groupKey, msg, sig []byte) {
	t.Helper()
	if len(groupKey) != 48 {
		t.Fatalf("fixture %s: group key length %d, want 48", name, len(groupKey))
	}
	if len(sig) != 96 {
		t.Fatalf("fixture %s: sig length %d, want 96", name, len(sig))
	}

	buf := make([]byte, 0, 4+4+4+48+8+len(msg)+96)
	buf = append(buf, []byte(fixtureMagic)...)
	var u4 [4]byte
	binary.LittleEndian.PutUint32(u4[:], fixtureABI)
	buf = append(buf, u4[:]...)
	if valid {
		buf = append(buf, 0x01, 0, 0, 0)
	} else {
		buf = append(buf, 0x00, 0, 0, 0)
	}
	buf = append(buf, groupKey...)
	var u8 [8]byte
	binary.LittleEndian.PutUint64(u8[:], uint64(len(msg)))
	buf = append(buf, u8[:]...)
	buf = append(buf, msg...)
	buf = append(buf, sig...)

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("wrote %s (%d bytes)", path, len(buf))
}

// TestDumpWitnessFixtures dumps a covering set of (group_key, msg, sig)
// triples for the C++ verifier. Skipped unless QUASAR_DUMP is set so
// the regular `go test` run stays read-only on the filesystem.
func TestDumpWitnessFixtures(t *testing.T) {
	dir := os.Getenv("QUASAR_DUMP")
	if dir == "" {
		t.Skip("QUASAR_DUMP not set; skipping fixture generation")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}

	// ---- Fixture 1: plain BLS Sign/Verify (single signer). The
	// threshold path collapses to this when n=1 and Lagrange weight is
	// 1, and it's also the byte-equal target for any C++ verifier of
	// the bls.Verify primitive.
	sk1, err := bls.NewSecretKey()
	if err != nil {
		t.Fatalf("NewSecretKey: %v", err)
	}
	pk1 := sk1.PublicKey()
	msg1 := []byte("quasar witness fixture 1 — single signer")
	sig1, err := sk1.Sign(msg1)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	// Sanity: the Go side must verify before we ask C++ to verify.
	if !bls.Verify(pk1, sig1, msg1) {
		t.Fatalf("fixture 1 self-verify failed")
	}
	writeFixture(t, dir, "witness_01_single_signer.bin", true,
		bls.PublicKeyToCompressedBytes(pk1), msg1, bls.SignatureToBytes(sig1))

	// ---- Fixture 2: same group key, wrong message → must NOT verify.
	wrongMsg := []byte("not the message that was signed")
	if bls.Verify(pk1, sig1, wrongMsg) {
		t.Fatalf("fixture 2 unexpectedly verified")
	}
	writeFixture(t, dir, "witness_02_wrong_message.bin", false,
		bls.PublicKeyToCompressedBytes(pk1), wrongMsg, bls.SignatureToBytes(sig1))

	// ---- Fixture 3: empty message — boundary case the verify hot path
	// must handle without UB on a NULL-but-zero-length input.
	msg3 := []byte{}
	sig3, err := sk1.Sign(msg3)
	if err != nil {
		t.Fatalf("Sign empty: %v", err)
	}
	if !bls.Verify(pk1, sig3, msg3) {
		t.Fatalf("fixture 3 self-verify failed")
	}
	writeFixture(t, dir, "witness_03_empty_message.bin", true,
		bls.PublicKeyToCompressedBytes(pk1), msg3, bls.SignatureToBytes(sig3))

	// ---- Fixture 4: BLS threshold aggregate (n-of-n) — exercises the
	// path AggregateThresholdSignatures → blsVerifier.VerifyBytes that
	// is the actual Quasar witness hot path. n=3, t=2, 3 of 3 signers.
	shares, groupKey, err := GenerateThresholdKeys(threshold.SchemeBLS, 2, 3)
	if err != nil {
		t.Fatalf("GenerateThresholdKeys: %v", err)
	}
	keyShares := map[string]threshold.KeyShare{
		"v0": shares[0], "v1": shares[1], "v2": shares[2],
	}
	signer, err := newSignerWithThresholdConfig(ThresholdConfig{
		SchemeID:     threshold.SchemeBLS,
		Threshold:    2,
		TotalParties: 3,
		KeyShares:    keyShares,
		GroupKey:     groupKey,
	})
	if err != nil {
		t.Fatalf("newSignerWithThresholdConfig: %v", err)
	}

	ctx := t.Context()
	msg4 := []byte("quasar threshold-signature fixture — 3-of-3")
	share0, err := signer.SignMessageThreshold(ctx, "v0", msg4)
	if err != nil {
		t.Fatalf("SignMessageThreshold v0: %v", err)
	}
	share1, err := signer.SignMessageThreshold(ctx, "v1", msg4)
	if err != nil {
		t.Fatalf("SignMessageThreshold v1: %v", err)
	}
	share2, err := signer.SignMessageThreshold(ctx, "v2", msg4)
	if err != nil {
		t.Fatalf("SignMessageThreshold v2: %v", err)
	}
	aggSig, err := signer.AggregateThresholdSignatures(ctx, msg4,
		[]threshold.SignatureShare{share0, share1, share2})
	if err != nil {
		t.Fatalf("AggregateThresholdSignatures: %v", err)
	}
	if !signer.VerifyThresholdSignatureBytes(msg4, aggSig.Bytes()) {
		t.Fatalf("fixture 4 self-verify failed")
	}
	writeFixture(t, dir, "witness_04_threshold_3of3.bin", true,
		groupKey.Bytes(), msg4, aggSig.Bytes())

	// ---- Fixture 5: same threshold cert as #4 but a tampered byte
	// (flip the last byte of the signature) → must NOT verify.
	tampered := append([]byte(nil), aggSig.Bytes()...)
	tampered[len(tampered)-1] ^= 0x01
	// The tampered bytes may fail subgroup check (likely) rather than
	// the pairing equation; either way Go's VerifyThresholdSignatureBytes
	// returns false.
	if signer.VerifyThresholdSignatureBytes(msg4, tampered) {
		t.Fatalf("fixture 5 unexpectedly verified after tamper")
	}
	writeFixture(t, dir, "witness_05_threshold_tampered.bin", false,
		groupKey.Bytes(), msg4, tampered)
}
