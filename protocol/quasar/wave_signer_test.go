// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"

	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/ids"
	"github.com/luxfi/pulsar-m/ref/go/pkg/pulsarm"
)

// TestLuxRoundSigner_RunLuxRound_ProducesFIPS204Verifiable runs a
// minimal Lux round at (K, T) = (3, 2): three sampled validators
// produce one FIPS 204 ML-DSA signature on a round item, which
// verifies under unmodified pulsarm.VerifyCtx (i.e. FIPS 204
// ML-DSA.Verify with the precompile ctx).
func TestLuxRoundSigner_RunLuxRound_ProducesFIPS204Verifiable(t *testing.T) {
	params := pulsarm.MustParamsFor(pulsarm.ModeP65)

	// Build a (3, 2) DKG using pulsarm directly to set up the
	// per-validator KeyShares. This mirrors what a real Lux
	// validator-set DKG ceremony would produce at the start of an
	// epoch.
	committee := []pulsarm.NodeID{
		{0x01, 0x00, 'V'}, {0x02, 0x00, 'V'}, {0x03, 0x00, 'V'},
	}
	const n, threshold = 3, 2
	dkg := make([]*pulsarm.DKGSession, n)
	for i := 0; i < n; i++ {
		rng := bytes.NewReader(append([]byte{byte(i)}, bytes.Repeat([]byte{0xDE, 0xAD}, 32)...))
		s, err := pulsarm.NewDKGSession(params, committee, threshold, committee[i], rng)
		if err != nil {
			t.Fatal(err)
		}
		dkg[i] = s
	}
	r1 := make([]*pulsarm.DKGRound1Msg, n)
	for i, s := range dkg {
		m, err := s.Round1()
		if err != nil {
			t.Fatal(err)
		}
		r1[i] = m
	}
	r2 := make([]*pulsarm.DKGRound2Msg, n)
	for i, s := range dkg {
		m, err := s.Round2(r1)
		if err != nil {
			t.Fatal(err)
		}
		r2[i] = m
	}
	outs := make([]*pulsarm.DKGOutput, n)
	for i, s := range dkg {
		out, err := s.Round3(r1, r2)
		if err != nil {
			t.Fatal(err)
		}
		outs[i] = out
	}
	groupPK := outs[0].GroupPubkey

	// Build the LuxRoundSigner's share map keyed by ids.NodeID.
	shares := make(map[ids.NodeID]*pulsarm.KeyShare, n)
	pool := make([]ids.NodeID, n)
	for i := 0; i < n; i++ {
		var id ids.NodeID
		copy(id[:], committee[i][:])
		pool[i] = id
		shares[id] = outs[i].SecretShare
	}

	ctxBytes := []byte("lux-evm-precompile-pulsar-v1")
	signer := &LuxRoundSigner{
		Params:        params,
		Cut:           prism.NewUniformCut(pool),
		K:             3,
		Threshold:     2,
		Shares:        shares,
		PrecompileCtx: ctxBytes,
	}

	// Drive one Lux round.
	item := []byte("lux-round smoke test item")
	res, err := signer.RunLuxRound(context.Background(), item)
	if err != nil {
		t.Fatalf("RunLuxRound: %v", err)
	}

	// The result must verify under unmodified FIPS 204 ML-DSA.Verify
	// with the precompile ctx -- this is the Class N1 manifesto in
	// effect end-to-end through the Wave-driver adapter.
	if err := pulsarm.VerifyCtx(params, groupPK, item, ctxBytes, res.Signature); err != nil {
		t.Fatalf("FIPS 204 verify (with precompile ctx) on LuxRoundSigner output: %v", err)
	}

	// Signature must NOT verify under a different ctx -- proves ctx
	// is load-bearing end-to-end.
	if err := pulsarm.VerifyCtx(params, groupPK, item, nil, res.Signature); err == nil {
		t.Fatalf("expected verify failure under ctx=nil; got nil error")
	}

	// Suppress the unused rand import in case the test ever reverts.
	_ = rand.Reader
}
