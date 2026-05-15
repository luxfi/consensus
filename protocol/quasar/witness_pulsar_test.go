// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"crypto/rand"
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/crypto/mldsa"
	"github.com/luxfi/pulsar/ref/go/pkg/pulsar"
)

// TestVerifyPQFinality_PulsarM_RealVerifyAccepts proves that
// verifyPQFinality dispatches to luxfi/pulsar-m.Verify and accepts a
// real Pulsar-M-65 signature produced over the canonical
// SigningDigest. Closes F109: the prior implementation accepted by
// bit-counting; this test passes only when an actual lattice
// signature math verification succeeds.
func TestVerifyPQFinality_PulsarM_RealVerifyAccepts(t *testing.T) {
	params := pulsar.ParamsP65
	priv, err := pulsar.GenerateKey(params, rand.Reader)
	if err != nil {
		t.Fatalf("pulsar.GenerateKey: %v", err)
	}

	w := NewVerkleWitness(1)
	w.BindPQGroupKey(priv.Pub.Bytes)

	// Build a structurally complete witness (the bit-count pre-filter
	// requires at least 1 set bit; threshold=1).
	witness := &WitnessProof{
		Commitment:   make([]byte, 32),
		Path:         make([]byte, 16),
		OpeningProof: make([]byte, 48),
		CoronaBits:   []byte{0x01},
		BlockHeight:  100,
		StateRoot:    make([]byte, 32),
		Timestamp:    1700000000,
	}
	for i := range witness.StateRoot {
		witness.StateRoot[i] = byte(i)
	}

	digest := witness.SigningDigest()
	sig, err := pulsar.Sign(params, priv, digest[:], []byte(signingDigestCustomization), true, rand.Reader)
	if err != nil {
		t.Fatalf("pulsar.Sign: %v", err)
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
	params := pulsar.ParamsP65
	priv, err := pulsar.GenerateKey(params, rand.Reader)
	if err != nil {
		t.Fatalf("pulsar.GenerateKey: %v", err)
	}

	w := NewVerkleWitness(1)
	w.BindPQGroupKey(priv.Pub.Bytes)

	witness := &WitnessProof{
		Commitment:   make([]byte, 32),
		Path:         make([]byte, 16),
		OpeningProof: make([]byte, 48),
		CoronaBits:   []byte{0x01},
		BlockHeight:  100,
		StateRoot:    make([]byte, 32),
		Timestamp:    1700000000,
	}
	digest := witness.SigningDigest()
	sig, err := pulsar.Sign(params, priv, digest[:], []byte(signingDigestCustomization), true, rand.Reader)
	if err != nil {
		t.Fatalf("pulsar.Sign: %v", err)
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

// TestVerifyPQFinality_PulsarM_BitCountAlone_DoesNotPass is the
// canonical F109 anti-regression. The earlier form of this test wired
// an all-0xFF "signature" into the witness, but those bytes failed at
// the FIPS 204 decoder *before* the lattice verify equation ran — so a
// regression that broke only the verify equation could slip past. The
// stronger form below builds a VALID Pulsar-M signature over message M1
// and then mutates the witness to M2 (a different StateRoot) so the
// signed digest no longer matches the witness's SigningDigest(). A
// real lattice verifier MUST refuse; a bit-count tautology accepts.
func TestVerifyPQFinality_PulsarM_BitCountAlone_DoesNotPass(t *testing.T) {
	params := pulsar.ParamsP65
	priv, err := pulsar.GenerateKey(params, rand.Reader)
	if err != nil {
		t.Fatalf("pulsar.GenerateKey: %v", err)
	}

	w := NewVerkleWitness(1)
	w.BindPQGroupKey(priv.Pub.Bytes)

	// M1 — the witness we actually sign.
	witness := &WitnessProof{
		Commitment:   make([]byte, 32),
		Path:         make([]byte, 16),
		OpeningProof: make([]byte, 48),
		CoronaBits:   []byte{0xFF}, // 8 bits set — bit-count would pass at any threshold ≤ 8
		BlockHeight:  100,
		StateRoot:    make([]byte, 32),
		Timestamp:    1700000000,
	}
	for i := range witness.StateRoot {
		witness.StateRoot[i] = byte(i)
	}

	digestM1 := witness.SigningDigest()
	sig, err := pulsar.Sign(params, priv, digestM1[:], []byte(signingDigestCustomization), true, rand.Reader)
	if err != nil {
		t.Fatalf("pulsar.Sign: %v", err)
	}
	witness.PQSignature = sig.Bytes

	// Swap the message: mutate StateRoot so SigningDigest now yields
	// M2 ≠ M1. The signature still decodes (it's a genuine Pulsar-M
	// signature), the bit-count pre-filter still passes (CoronaBits
	// untouched), but the lattice verify equation MUST reject.
	witness.StateRoot[0] ^= 0x01
	digestM2 := witness.SigningDigest()
	if digestM1 == digestM2 {
		t.Fatal("test bug: StateRoot mutation did not change SigningDigest")
	}

	err = w.verifyPQFinality(witness, priv.Pub.Bytes, mldsa.MLDSA65)
	if err == nil {
		t.Fatal("verifyPQFinality accepted a valid signature over a different message: F109 regressed")
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
	w.SetProfile(config.StrictPQ())

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
