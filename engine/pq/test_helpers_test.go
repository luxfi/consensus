// Copyright (C) 2025, Lux Industries Inc. All rights reserved.

package pq

import (
	"testing"

	"github.com/luxfi/consensus/protocol/quasar"
	"github.com/luxfi/crypto/bls"
)

// newTestSigner returns a Quasar signer with a single configured validator
// covering BLS + Ringtail + ML-DSA. The first return is the validator ID
// chosen, the second is the signer ready for TripleSignRound1.
func newTestSigner(t *testing.T) (string, *quasar.Signer) {
	t.Helper()

	cfg, err := quasar.GenerateDualKeys(1, 3)
	if err != nil {
		t.Fatalf("GenerateDualKeys: %v", err)
	}
	s, err := quasar.NewSignerWithDualThreshold(*cfg)
	if err != nil {
		t.Fatalf("NewSignerWithDualThreshold: %v", err)
	}
	for _, id := range []string{"v0", "v1", "v2"} {
		if err := s.AddValidator(id, 100); err != nil {
			t.Fatalf("AddValidator(%s): %v", id, err)
		}
	}
	return "v0", s
}

// testBLSAggKey returns a freshly generated BLS public key for tests that
// need any non-nil verify key to feed AttachVerifyKeys.
func testBLSAggKey(t *testing.T) *bls.PublicKey {
	t.Helper()
	sk, err := bls.NewSecretKey()
	if err != nil {
		t.Fatalf("bls.NewSecretKey: %v", err)
	}
	return sk.PublicKey()
}
