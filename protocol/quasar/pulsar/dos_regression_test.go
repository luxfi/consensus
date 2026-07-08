// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// dos_regression_test.go -- Item7(a) regression lock (shipped v1.35.30
// surface): Finalize must reject a Partial naming a PartyID outside
// ValidatorSetSize BEFORE any PartyID-sized bitmap allocation, closing the
// unbounded-allocation DoS in the pulsarlib CanonicalSignerSet /
// singletonBitmap `make([]byte, PartyID/8+1)` sizing.

package pulsar

import (
	"bytes"
	"math"
	"testing"

	pulsarlib "github.com/luxfi/pulsar/pkg/pulsar"
)

// TestFinalize_RejectsOversizedPartyID drives the real Round1 -> Round2 ->
// Finalize path with one legitimate signer and one hostile Round2 message
// naming PartyID == math.MaxUint32, well outside ValidatorSetSize. Round2
// itself imposes no PartyID bound (that is Finalize's responsibility), so
// this is exactly what an attacker-controlled peer partial looks like on the
// wire. Finalize must return ErrPartyIDOutOfRange and must NOT allocate a
// ~512MiB bitmap along the way.
func TestFinalize_RejectsOversizedPartyID(t *testing.T) {
	pool, nonceID := makePool()
	sess := fullSession(pool.Root())
	sid := sess.SessionID()

	s := &PulsarRoundSigner{
		Session:          sess,
		Pool:             pool,
		Threshold:        2,
		ValidatorSetSize: 8,
		L:                5,
		Core:             nil,
	}

	canonical, ok := pool.At(pulsarlib.CanonicalNonceIndex(sid, pool.Root(), pool.Size()))
	if !ok {
		t.Fatal("pool.At canonical index failed")
	}
	r1, err := s.Round1(sid, nonceID, canonical)
	if err != nil {
		t.Fatalf("Round1: %v", err)
	}

	var partials []pulsarlib.Partial
	for _, party := range []uint32{2, math.MaxUint32} {
		in := pulsarlib.PartialInput{
			PartyID: party,
			ZShare:  bytes.Repeat([]byte{byte(party)}, 5*256*4),
		}
		p, err := s.Round2(r1, in)
		if err != nil {
			t.Fatalf("Round2 party %d: %v", party, err)
		}
		partials = append(partials, p)
	}

	if _, _, err := s.Finalize(r1, partials); err != ErrPartyIDOutOfRange {
		t.Fatalf("Finalize err = %v, want ErrPartyIDOutOfRange", err)
	}
}

// TestFinalize_RejectsOversizedPartyID_ExcludedFromChosen confirms the bound
// covers the WHOLE partials slice, not just the threshold-chosen subset: a
// hostile oversized PartyID that sorts last (and would be excluded by the
// first-threshold cut) is still rejected.
func TestFinalize_RejectsOversizedPartyID_ExcludedFromChosen(t *testing.T) {
	pool, nonceID := makePool()
	sess := fullSession(pool.Root())
	sid := sess.SessionID()

	s := &PulsarRoundSigner{
		Session:          sess,
		Pool:             pool,
		Threshold:        2, // chooses the two SMALLEST PartyIDs (1,2)
		ValidatorSetSize: 8,
		L:                5,
	}

	canonical, _ := pool.At(pulsarlib.CanonicalNonceIndex(sid, pool.Root(), pool.Size()))
	r1, err := s.Round1(sid, nonceID, canonical)
	if err != nil {
		t.Fatalf("Round1: %v", err)
	}

	var partials []pulsarlib.Partial
	for _, party := range []uint32{1, 2, 3, math.MaxUint32} {
		p, err := s.Round2(r1, pulsarlib.PartialInput{
			PartyID: party,
			ZShare:  bytes.Repeat([]byte{byte(party)}, 5*256*4),
		})
		if err != nil {
			t.Fatalf("Round2 party %d: %v", party, err)
		}
		partials = append(partials, p)
	}

	if _, _, err := s.Finalize(r1, partials); err != ErrPartyIDOutOfRange {
		t.Fatalf("Finalize err = %v, want ErrPartyIDOutOfRange (even when oversized PartyID is excluded from chosen)", err)
	}
}

// TestFinalize_InRangePartyIDsSucceed is the happy-path guard: with every
// PartyID inside ValidatorSetSize the bound is transparent and Finalize
// still fails closed only on the (expected) missing signing core.
func TestFinalize_InRangePartyIDsSucceed(t *testing.T) {
	pool, nonceID := makePool()
	sess := fullSession(pool.Root())
	sid := sess.SessionID()

	s := &PulsarRoundSigner{
		Session:          sess,
		Pool:             pool,
		Threshold:        2,
		ValidatorSetSize: 8,
		L:                5,
	}

	canonical, _ := pool.At(pulsarlib.CanonicalNonceIndex(sid, pool.Root(), pool.Size()))
	r1, err := s.Round1(sid, nonceID, canonical)
	if err != nil {
		t.Fatalf("Round1: %v", err)
	}

	var partials []pulsarlib.Partial
	for _, party := range []uint32{2, 5} {
		p, err := s.Round2(r1, pulsarlib.PartialInput{
			PartyID: party,
			ZShare:  bytes.Repeat([]byte{byte(party)}, 5*256*4),
		})
		if err != nil {
			t.Fatalf("Round2 party %d: %v", party, err)
		}
		partials = append(partials, p)
	}

	agg, cert, err := s.Finalize(r1, partials)
	if err != ErrProfileNotReady {
		t.Fatalf("Finalize err = %v, want ErrProfileNotReady (no core), NOT a bound rejection", err)
	}
	if len(agg.ZSum) == 0 {
		t.Fatal("aggregate z-sum is empty on the in-range happy path")
	}
	// Bitmap must be bounded by ValidatorSetSize, never by a raw PartyID.
	if len(cert.SignerBitmap) > (s.ValidatorSetSize/8 + 1) {
		t.Fatalf("bitmap length %d exceeds ValidatorSetSize-bounded maximum", len(cert.SignerBitmap))
	}
}
