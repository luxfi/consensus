// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"crypto/rand"
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/crypto/mldsa"
	"github.com/luxfi/pulsar-m/ref/go/pkg/pulsarm"
)

// TestVerifyPQFinality_PulsarM_RealVerifyAccepts proves that
// verifyPQFinality dispatches to luxfi/pulsar-m.Verify and accepts a
// real Pulsar-M-65 signature produced over the canonical
// SigningDigest. Closes F109: the prior implementation accepted by
// bit-counting; this test passes only when an actual lattice
// signature math verification succeeds.
func TestVerifyPQFinality_PulsarM_RealVerifyAccepts(t *testing.T) {
	params := pulsarm.ParamsP65
	priv, err := pulsarm.GenerateKey(params, rand.Reader)
	if err != nil {
		t.Fatalf("pulsarm.GenerateKey: %v", err)
	}

	w := NewVerkleWitness(1)
	w.BindPQGroupKey(priv.Pub.Bytes)

	// Build a structurally complete witness (the bit-count pre-filter
	// requires at least 1 set bit; threshold=1).
	witness := &WitnessProof{
		Commitment:   make([]byte, 32),
		Path:         make([]byte, 16),
		OpeningProof: make([]byte, 48),
		RingtailBits: []byte{0x01},
		BlockHeight:  100,
		StateRoot:    make([]byte, 32),
		Timestamp:    1700000000,
	}
	for i := range witness.StateRoot {
		witness.StateRoot[i] = byte(i)
	}

	digest := witness.SigningDigest()
	sig, err := pulsarm.Sign(params, priv, digest[:], []byte(signingDigestCustomization), true, rand.Reader)
	if err != nil {
		t.Fatalf("pulsarm.Sign: %v", err)
	}
	witness.PQSignature = sig.Bytes

	if err := w.verifyPQFinality(witness, priv.Pub.Bytes, mldsa.MLDSA65); err != nil {
		t.Fatalf("verifyPQFinality rejected a real Pulsar-M signature: %v", err)
	}
}

// TestVerifyPQFinality_PulsarM_TamperedSignatureRejected proves a
// tampered signature is refused by the real lattice verifier — the
// load-bearing property that closes the F109 attack chain.
func TestVerifyPQFinality_PulsarM_TamperedSignatureRejected(t *testing.T) {
	params := pulsarm.ParamsP65
	priv, err := pulsarm.GenerateKey(params, rand.Reader)
	if err != nil {
		t.Fatalf("pulsarm.GenerateKey: %v", err)
	}

	w := NewVerkleWitness(1)
	w.BindPQGroupKey(priv.Pub.Bytes)

	witness := &WitnessProof{
		Commitment:   make([]byte, 32),
		Path:         make([]byte, 16),
		OpeningProof: make([]byte, 48),
		RingtailBits: []byte{0x01},
		BlockHeight:  100,
		StateRoot:    make([]byte, 32),
		Timestamp:    1700000000,
	}
	digest := witness.SigningDigest()
	sig, err := pulsarm.Sign(params, priv, digest[:], []byte(signingDigestCustomization), true, rand.Reader)
	if err != nil {
		t.Fatalf("pulsarm.Sign: %v", err)
	}

	// Flip a byte mid-signature. A real lattice verifier MUST refuse;
	// a bit-count tautology would happily accept.
	tampered := append([]byte(nil), sig.Bytes...)
	tampered[len(tampered)/2] ^= 0xFF
	witness.PQSignature = tampered

	err = w.verifyPQFinality(witness, priv.Pub.Bytes, mldsa.MLDSA65)
	if err == nil {
		t.Fatal("verifyPQFinality accepted a tampered signature")
	}
	if !errors.Is(err, ErrPulsarMVerifyFail) {
		t.Fatalf("expected ErrPulsarMVerifyFail, got: %v", err)
	}
}

// TestVerifyPQFinality_BitCountAlone_DoesNotPass is the canonical
// F109 anti-regression: a "signature" of all 0xFF bytes that satisfies
// the bit-count pre-filter MUST be refused by the real verifier. If
// this test ever passes by accepting the dummy, F109 has re-opened.
func TestVerifyPQFinality_BitCountAlone_DoesNotPass(t *testing.T) {
	params := pulsarm.ParamsP65
	priv, err := pulsarm.GenerateKey(params, rand.Reader)
	if err != nil {
		t.Fatalf("pulsarm.GenerateKey: %v", err)
	}

	w := NewVerkleWitness(1)
	w.BindPQGroupKey(priv.Pub.Bytes)

	witness := &WitnessProof{
		Commitment:   make([]byte, 32),
		Path:         make([]byte, 16),
		OpeningProof: make([]byte, 48),
		RingtailBits: []byte{0xFF}, // 8 bits set — bit-count would pass at any threshold ≤ 8
		PQSignature:  make([]byte, params.SignatureSize),
		BlockHeight:  100,
		StateRoot:    make([]byte, 32),
		Timestamp:    1700000000,
	}
	for i := range witness.PQSignature {
		witness.PQSignature[i] = 0xFF
	}

	err = w.verifyPQFinality(witness, priv.Pub.Bytes, mldsa.MLDSA65)
	if err == nil {
		t.Fatal("verifyPQFinality accepted an all-0xFF non-signature with high bit-count: F109 regressed")
	}
	if !errors.Is(err, ErrPulsarMVerifyFail) {
		t.Fatalf("expected ErrPulsarMVerifyFail, got: %v", err)
	}
}

// TestVerifyBLSAggregate_RefusedUnderStrictPQ proves verifyBLSAggregate
// hard-refuses any call made under a strict-PQ profile, regardless of
// signature validity. Closes F107.
func TestVerifyBLSAggregate_RefusedUnderStrictPQ(t *testing.T) {
	w := NewVerkleWitness(1)
	w.SetProfile(config.LuxStrictPQ())

	// 48 bytes of zeros — would fail curve-point check anyway, but the
	// strict-PQ refusal must fire FIRST (typed sentinel).
	err := w.verifyBLSAggregate(make([]byte, 48), []byte("validators"))
	if !errors.Is(err, ErrBLSForbiddenUnderStrictPQ) {
		t.Fatalf("expected ErrBLSForbiddenUnderStrictPQ, got: %v", err)
	}
}

// TestVerifyBLSAggregate_AllowedUnderLegacyNilProfile proves the BLS
// aggregate path remains callable on the pre-locked-profile legacy
// caller path (nil profile). The function still refuses garbage curve
// points; it just doesn't refuse on profile grounds.
//
// ForkClassicalCompatUnsafe is NOT in this test: even the fork profile
// pins ForbidPairings=true on the proof axis, which transitively forbids
// BLS aggregate verification. The only path that admits a BLS aggregate
// today is the truly-legacy "no profile bound" caller path retained for
// pre-locked-profile compatibility — and even there only as a stepping
// stone toward Pulsar-M everywhere.
func TestVerifyBLSAggregate_AllowedUnderLegacyNilProfile(t *testing.T) {
	w := NewVerkleWitness(1) // profile remains nil

	// Garbage bytes — must still fail the curve-point check, but with
	// a different error than the strict-PQ sentinel.
	err := w.verifyBLSAggregate(make([]byte, 48), []byte("validators"))
	if err == nil {
		t.Fatal("verifyBLSAggregate should reject zero-point bytes even on legacy path")
	}
	if errors.Is(err, ErrBLSForbiddenUnderStrictPQ) {
		t.Fatalf("nil-profile legacy path must NOT raise the strict-PQ refusal; got: %v", err)
	}
}
