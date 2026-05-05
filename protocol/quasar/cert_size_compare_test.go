// Copyright (C) 2025, Lux Industries Inc. All rights reserved.

package quasar

import (
	"bytes"
	"encoding/gob"
	"testing"

	ringtailThreshold "github.com/luxfi/pulsar/threshold"
)

// TestRingtailCertSize_BinaryVsGob measures the on-wire Pulsar/Ringtail
// signature size produced by the native binary encoder vs the legacy gob
// encoder, for the production parameter set (M=8, N=7, LogN=8 -> ring
// degree 256, Q=0x1000000004A01 48-bit prime). Reports per-cert and
// 10K-cert storage so we can see the gob -> native delta.
func TestRingtailCertSize_BinaryVsGob(t *testing.T) {
	cfg, err := GenerateDualKeys(2, 3)
	if err != nil {
		t.Fatalf("GenerateDualKeys: %v", err)
	}

	signers := make([]*ringtailThreshold.Signer, 3)
	i := 0
	for _, share := range cfg.RingtailShares {
		signers[i] = ringtailThreshold.NewSigner(share)
		i++
	}

	const sessionID = 1
	prfKey := []byte("pulsar-cert-size-prf")
	signerIDs := []int{0, 1, 2}

	r1 := make(map[int]*ringtailThreshold.Round1Data)
	for _, s := range signers {
		d := s.Round1(sessionID, prfKey, signerIDs)
		r1[d.PartyID] = d
	}
	r2 := make(map[int]*ringtailThreshold.Round2Data)
	for _, s := range signers {
		d, err := s.Round2(sessionID, "test-message", prfKey, signerIDs, r1)
		if err != nil {
			t.Skipf("Ringtail Round2 (rejection sampling flake): %v", err)
		}
		r2[d.PartyID] = d
	}
	sig, err := signers[0].Finalize(r2)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	binBytes := ringtailGobEncode(sig)

	var gobBuf bytes.Buffer
	if err := gob.NewEncoder(&gobBuf).Encode(sig); err != nil {
		t.Fatalf("gob encode: %v", err)
	}
	gobBytes := gobBuf.Bytes()

	binSize := len(binBytes)
	gobSize := len(gobBytes)
	ratio := float64(gobSize) / float64(binSize)

	t.Logf("Pulsar/Ringtail Signature size (production params M=8 N=7 LogN=8 Q=2^48-ish):")
	t.Logf("  Native binary (Poly+Vector MarshalBinary):  %7d bytes (%.2f KB)", binSize, float64(binSize)/1024.0)
	t.Logf("  Legacy gob (reflection metadata):           %7d bytes (%.2f KB)", gobSize, float64(gobSize)/1024.0)
	t.Logf("  Bloat: gob/native = %.2fx", ratio)
	t.Logf("  10K certs storage:")
	t.Logf("    Native: %.2f MB", float64(binSize)*10000/1024.0/1024.0)
	t.Logf("    Gob:    %.2f MB", float64(gobSize)*10000/1024.0/1024.0)

	// Sanity: roundtrip must be byte-equal
	rt, err := ringtailGobDecode(binBytes)
	if err != nil {
		t.Fatalf("native roundtrip decode: %v", err)
	}
	rtBytes := ringtailGobEncode(rt)
	if !bytes.Equal(binBytes, rtBytes) {
		t.Fatalf("native encode/decode not byte-equal: orig=%d roundtrip=%d", len(binBytes), len(rtBytes))
	}

	if !ringtailThreshold.Verify(cfg.RingtailGroupKey, "test-message", rt) {
		t.Fatalf("native-decoded signature failed Verify")
	}
}
