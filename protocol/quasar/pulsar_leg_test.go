// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"crypto/rand"
	"testing"

	"github.com/luxfi/threshold/protocols/pulsar"
	"golang.org/x/crypto/sha3"
)

// signPulsarThresholdLeg drives one t-of-n Pulsar threshold-sign ceremony over
// a fixed committee and its DKG outputs, returning the FIPS 204 ML-DSA
// signature that verifies under dkgOuts[0].GroupPubkey through unmodified
// pulsar.VerifyCtx.
//
// This is the test-only ceremony driver. It replaces the per-round wrapper
// that the deleted wave_signer.RoundSigner provided: committee selection (the
// RoundSigner's Prism.Cut sampling) is irrelevant to cert composition, so it is
// dropped and the caller passes the fixed committee directly. The signing
// ceremony itself is unchanged and real — a genuine t-of-n run over the
// canonical Pulsar reconstruction-aggregator path — so every cert-composition
// test keeps a Pulsar leg whose bytes come from the real signing primitive
// (no stub, no single-party shortcut).
func signPulsarThresholdLeg(
	t *testing.T,
	params *pulsar.Params,
	committee []pulsar.NodeID,
	threshold int,
	identities map[pulsar.NodeID]*pulsar.IdentityKey,
	dkgOuts []*pulsar.DKGOutput,
	item, ctx []byte,
) *pulsar.Signature {
	t.Helper()
	if threshold < 1 || len(committee) < threshold || len(dkgOuts) < len(committee) {
		t.Fatalf("signPulsarThresholdLeg: invalid (committee=%d, threshold=%d, dkgOuts=%d)",
			len(committee), threshold, len(dkgOuts))
	}
	quorum := committee[:threshold]
	groupPK := dkgOuts[0].GroupPubkey

	// Per-round session id binds the ceremony PRNG to this item so the
	// signature is deterministic across replays of the same round.
	var sessionID [16]byte
	h := sha3.NewLegacyKeccak256()
	h.Write(item)
	h.Write(quorum[0][:])
	copy(sessionID[:], h.Sum(nil))

	// Pairwise symmetric session keys. The witness pattern runs all signers
	// in-process; we derive the identity-stage keys exactly as a distributed
	// run would, transcript-bound so keys differ per round.
	transcript := make([]byte, 0, len(item)+len(groupPK.Bytes)+len(quorum[0]))
	transcript = append(transcript, item...)
	transcript = append(transcript, groupPK.Bytes...)
	transcript = append(transcript, quorum[0][:]...)
	sessionKeys := make(map[pulsar.NodeID]map[pulsar.NodeID][32]byte, len(quorum))
	for _, id := range quorum {
		sessionKeys[id] = make(map[pulsar.NodeID][32]byte, len(quorum)-1)
	}
	for i := 0; i < len(quorum); i++ {
		for j := i + 1; j < len(quorum); j++ {
			a, b := quorum[i], quorum[j]
			key, err := pulsar.SymmetricSession(a, identities[a], b, identities[b], sessionID, transcript)
			if err != nil {
				t.Fatalf("pulsar.SymmetricSession: %v", err)
			}
			sessionKeys[a][b] = key
			sessionKeys[b][a] = key
		}
	}

	// All committee shares are needed for committee-root reconstruction.
	allShares := make([]*pulsar.KeyShare, len(committee))
	for i := range committee {
		allShares[i] = dkgOuts[i].SecretShare
	}

	// One ThresholdSigner per quorum member, then Round1 / Round2 / Combine.
	signers := make([]*pulsar.ThresholdSigner, threshold)
	for i := 0; i < threshold; i++ {
		ts, err := pulsar.NewThresholdSigner(
			params, sessionID, 0, quorum,
			dkgOuts[i].SecretShare, sessionKeys[quorum[i]], item, rand.Reader,
		)
		if err != nil {
			t.Fatalf("pulsar.NewThresholdSigner[%d]: %v", i, err)
		}
		signers[i] = ts
	}
	round1 := make([]*pulsar.Round1Message, threshold)
	for i, ts := range signers {
		m, err := ts.Round1(item)
		if err != nil {
			t.Fatalf("pulsar Round1[%d]: %v", i, err)
		}
		round1[i] = m
	}
	round2 := make([]*pulsar.Round2Message, threshold)
	for i, ts := range signers {
		m, _, err := ts.Round2(round1)
		if err != nil {
			t.Fatalf("pulsar Round2[%d]: %v", i, err)
		}
		round2[i] = m
	}
	sig, err := pulsar.Combine(
		params, groupPK, item, ctx, false,
		sessionID, 0, quorum, threshold, round1, round2, allShares,
	)
	if err != nil {
		t.Fatalf("pulsar.Combine: %v", err)
	}
	return sig
}
