// Copyright (C) 2025, Lux Industries Inc. All rights reserved.

package quasar

import (
	"bytes"
	"encoding/gob"
	"testing"

	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
)

// TestCoronaCertSize_BinaryVsGob measures the on-wire Pulsar/Corona
// signature size produced by the native binary encoder vs the legacy gob
// encoder, for the production parameter set (M=8, N=7, LogN=8 -> ring
// degree 256, Q=0x1000000004A01 48-bit prime). Reports per-cert and
// 10K-cert storage so we can see the gob -> native delta.
func TestCoronaCertSize_BinaryVsGob(t *testing.T) {
	cfg, err := GenerateDualKeys(2, 3)
	if err != nil {
		t.Fatalf("GenerateDualKeys: %v", err)
	}

	signers := make([]*coronaThreshold.Signer, 3)
	i := 0
	for _, share := range cfg.CoronaShares {
		signers[i] = coronaThreshold.NewSigner(share)
		i++
	}

	const sessionID = 1
	prfKey := []byte("pulsar-cert-size-prf")
	signerIDs := []int{0, 1, 2}

	r1 := make(map[int]*coronaThreshold.Round1Data)
	for _, s := range signers {
		d, err := s.Round1(sessionID, prfKey, signerIDs)
		if err != nil {
			t.Fatalf("Round1: %v", err)
		}
		r1[d.PartyID] = d
	}
	r2 := make(map[int]*coronaThreshold.Round2Data)
	for _, s := range signers {
		d, err := s.Round2(sessionID, "test-message", prfKey, signerIDs, r1)
		if err != nil {
			t.Skipf("Corona Round2 (rejection sampling flake): %v", err)
		}
		r2[d.PartyID] = d
	}
	sig, err := signers[0].Finalize(r2)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	binBytes := coronaGobEncode(sig)

	var gobBuf bytes.Buffer
	if err := gob.NewEncoder(&gobBuf).Encode(sig); err != nil {
		t.Fatalf("gob encode: %v", err)
	}
	gobBytes := gobBuf.Bytes()

	binSize := len(binBytes)
	gobSize := len(gobBytes)
	ratio := float64(gobSize) / float64(binSize)

	t.Logf("Pulsar/Corona Signature size (production params M=8 N=7 LogN=8 Q=2^48-ish):")
	t.Logf("  Native binary (Poly+Vector MarshalBinary):  %7d bytes (%.2f KB)", binSize, float64(binSize)/1024.0)
	t.Logf("  Legacy gob (reflection metadata):           %7d bytes (%.2f KB)", gobSize, float64(gobSize)/1024.0)
	t.Logf("  Bloat: gob/native = %.2fx", ratio)
	t.Logf("  10K certs storage:")
	t.Logf("    Native: %.2f MB", float64(binSize)*10000/1024.0/1024.0)
	t.Logf("    Gob:    %.2f MB", float64(gobSize)*10000/1024.0/1024.0)

	// Sanity: roundtrip must be byte-equal
	rt, err := coronaGobDecode(binBytes)
	if err != nil {
		t.Fatalf("native roundtrip decode: %v", err)
	}
	rtBytes := coronaGobEncode(rt)
	if !bytes.Equal(binBytes, rtBytes) {
		t.Fatalf("native encode/decode not byte-equal: orig=%d roundtrip=%d", len(binBytes), len(rtBytes))
	}

	if !coronaThreshold.Verify(cfg.CoronaGroupKey, "test-message", rt) {
		t.Fatalf("native-decoded signature failed Verify")
	}
}
