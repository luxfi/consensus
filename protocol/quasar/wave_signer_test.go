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
	"github.com/luxfi/threshold/protocols/pulsar"
)

// TestRoundSigner_RunRound_ProducesFIPS204Verifiable runs a
// minimal Lux round at (K, T) = (3, 2): three sampled validators
// produce one FIPS 204 ML-DSA signature on a round item, which
// verifies under unmodified pulsar.VerifyCtx (i.e. FIPS 204
// ML-DSA.Verify with the precompile ctx).
func TestRoundSigner_RunRound_ProducesFIPS204Verifiable(t *testing.T) {
	params := pulsar.MustParamsFor(pulsar.ModeP65)

	// Build a (3, 2) DKG using pulsar directly to set up the
	// per-validator KeyShares. This mirrors what a real Lux
	// validator-set DKG ceremony would produce at the start of an
	// epoch.
	committee := []pulsar.NodeID{
		{0x01, 0x00, 'V'}, {0x02, 0x00, 'V'}, {0x03, 0x00, 'V'},
	}
	const n, threshold = 3, 2

	// Per-party long-term identity keys (v1.0.7 mandatory CR-6/8 closure).
	identities := make(map[pulsar.NodeID]*pulsar.IdentityKey, n)
	dirEntries := make(map[pulsar.NodeID]*pulsar.IdentityPublicKey, n)
	for i := 0; i < n; i++ {
		ik, err := pulsar.GenerateIdentity(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		identities[committee[i]] = ik
		dirEntries[committee[i]] = ik.PublicKey()
	}
	directory, err := pulsar.NewIdentityDirectory(dirEntries)
	if err != nil {
		t.Fatal(err)
	}

	dkg := make([]*pulsar.DKGSession, n)
	for i := 0; i < n; i++ {
		rng := bytes.NewReader(append([]byte{byte(i)}, bytes.Repeat([]byte{0xDE, 0xAD}, 32)...))
		s, err := pulsar.NewDKGSession(params, committee, threshold, committee[i], identities[committee[i]], directory, rng)
		if err != nil {
			t.Fatal(err)
		}
		dkg[i] = s
	}
	r1 := make([]*pulsar.DKGRound1Msg, n)
	for i, s := range dkg {
		m, err := s.Round1()
		if err != nil {
			t.Fatal(err)
		}
		r1[i] = m
	}
	r2 := make([]*pulsar.DKGRound2Msg, n)
	for i, s := range dkg {
		m, err := s.Round2(r1)
		if err != nil {
			t.Fatal(err)
		}
		r2[i] = m
	}
	outs := make([]*pulsar.DKGOutput, n)
	for i, s := range dkg {
		out, err := s.Round3(r1, r2)
		if err != nil {
			t.Fatal(err)
		}
		outs[i] = out
	}
	groupPK := outs[0].GroupPubkey

	// Build the RoundSigner's share map keyed by ids.NodeID.
	shares := make(map[ids.NodeID]*pulsar.KeyShare, n)
	pool := make([]ids.NodeID, n)
	for i := 0; i < n; i++ {
		var id ids.NodeID
		copy(id[:], committee[i][:])
		pool[i] = id
		shares[id] = outs[i].SecretShare
	}

	ctxBytes := []byte("lux-evm-precompile-pulsar-v1")
	signer := &RoundSigner{
		Params:        params,
		Cut:           prism.NewUniformCut(pool),
		K:             3,
		Threshold:     2,
		Shares:        shares,
		PrecompileCtx: ctxBytes,
	}

	// Drive one Lux round.
	item := []byte("lux-round smoke test item")
	res, err := signer.RunRound(context.Background(), item)
	if err != nil {
		t.Fatalf("RunRound: %v", err)
	}

	// The result must verify under unmodified FIPS 204 ML-DSA.Verify
	// with the precompile ctx -- this is the Class N1 manifesto in
	// effect end-to-end through the Wave-driver adapter.
	if err := pulsar.VerifyCtx(params, groupPK, item, ctxBytes, res.Signature); err != nil {
		t.Fatalf("FIPS 204 verify (with precompile ctx) on RoundSigner output: %v", err)
	}

	// Signature must NOT verify under a different ctx -- proves ctx
	// is load-bearing end-to-end.
	if err := pulsar.VerifyCtx(params, groupPK, item, nil, res.Signature); err == nil {
		t.Fatalf("expected verify failure under ctx=nil; got nil error")
	}

	// Suppress the unused rand import in case the test ever reverts.
	_ = rand.Reader
}
