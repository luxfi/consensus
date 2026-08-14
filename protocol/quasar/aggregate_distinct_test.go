// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// aggregate_distinct_test.go — an aggregate's signer list is a SET.
//
// Aggregation sums the named validators' public keys, and BLS is linear, so a repeated
// id adds the same key again. t copies of one validator yield t*pk, and that validator
// can produce t*sigma by itself: e(t*sigma, g) = e(H(m), t*pk). Without a distinctness
// check, one validator forges a t-of-n aggregate — no key compromise, no collusion,
// nothing but its own honest signature scaled.

package quasar

import (
	"context"
	"testing"

	"github.com/luxfi/crypto/bls"
	"github.com/stretchr/testify/require"
)

// TestAggregateRejectsRepeatedSigner builds the forgery and requires it to fail.
func TestAggregateRejectsRepeatedSigner(t *testing.T) {
	const threshold = 3

	s, err := newSigner(threshold)
	require.NoError(t, err)

	// Three real validators exist, so a genuine 3-of-3 is possible. The attacker is
	// one of them and signs only for itself.
	for _, id := range []string{"attacker", "honest-1", "honest-2"} {
		require.NoError(t, s.AddValidator(id, 100))
	}

	message := []byte("block-at-height-42")

	sk := s.blsKeys["attacker"]
	require.NotNil(t, sk, "attacker must hold its own key")
	own, err := sk.Sign(message)
	require.NoError(t, err)

	// Scale the attacker's single signature to t copies by aggregating it with itself.
	// This is the exact value that satisfies the t-fold aggregate of its own key.
	scaled, err := bls.AggregateSignatures([]*bls.Signature{own, own, own})
	require.NoError(t, err)

	forged := &AggregatedSignature{
		BLSAggregated: bls.SignatureToBytes(scaled),
		ValidatorIDs:  []string{"attacker", "attacker", "attacker"},
		SignerCount:   threshold, // self-declared, and a lie
	}

	if s.VerifyAggregatedSignatureWithContext(context.Background(), message, forged) {
		t.Fatal("a single validator's own signature, repeated, verified as a 3-of-3 aggregate — one validator can finalize alone")
	}
}

// TestAggregateCountsDistinctSignersNotTheDeclaredOne: the count that decides the
// threshold is the one we derive, never the one the sender wrote down. Two real
// signers claiming to be three must fail.
func TestAggregateCountsDistinctSignersNotTheDeclaredOne(t *testing.T) {
	const threshold = 3

	s, err := newSigner(threshold)
	require.NoError(t, err)
	for _, id := range []string{"v1", "v2", "v3"} {
		require.NoError(t, s.AddValidator(id, 100))
	}

	message := []byte("block-at-height-43")

	var sigs []*bls.Signature
	for _, id := range []string{"v1", "v2"} {
		sig, err := s.blsKeys[id].Sign(message)
		require.NoError(t, err)
		sigs = append(sigs, sig)
	}
	agg, err := bls.AggregateSignatures(sigs)
	require.NoError(t, err)

	under := &AggregatedSignature{
		BLSAggregated: bls.SignatureToBytes(agg),
		ValidatorIDs:  []string{"v1", "v2"},
		SignerCount:   threshold, // claims one more signer than it has
	}

	if s.VerifyAggregatedSignatureWithContext(context.Background(), message, under) {
		t.Fatal("2 signers verified against a threshold of 3 because the message said so")
	}
}

// TestGenuineAggregateStillVerifies is the control on the control: the distinctness
// rule must not break an honest quorum.
func TestGenuineAggregateStillVerifies(t *testing.T) {
	const threshold = 3

	s, err := newSigner(threshold)
	require.NoError(t, err)
	ids := []string{"v1", "v2", "v3"}
	for _, id := range ids {
		require.NoError(t, s.AddValidator(id, 100))
	}

	message := []byte("block-at-height-44")

	var sigs []*bls.Signature
	for _, id := range ids {
		sig, err := s.blsKeys[id].Sign(message)
		require.NoError(t, err)
		sigs = append(sigs, sig)
	}
	agg, err := bls.AggregateSignatures(sigs)
	require.NoError(t, err)

	honest := &AggregatedSignature{
		BLSAggregated: bls.SignatureToBytes(agg),
		ValidatorIDs:  ids,
		SignerCount:   threshold,
	}

	if !s.VerifyAggregatedSignatureWithContext(context.Background(), message, honest) {
		t.Fatal("a genuine 3-of-3 aggregate was refused — the distinctness rule is too strict")
	}
}
