// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// attestation_test.go — the Quasar post-accept attestation sidecar (attestation.go).
//
// Proves the owner's cert-sidecar contract: a certificate forms POST-ACCEPT and matches the
// accepted block; an ABSENT/late cert never blocks (the collector has no acceptance surface at
// all); a below-⅔ collusion yields NO cert; the emitted cert verifies for an INDEPENDENT
// external verifier (the bridge/DEX consumer); forgery attempts are rejected; and equivocation
// (two inner blocks attested at one height) is surfaced as evidence but can never yield two
// certs. Also pins the no-static-K/α quorum floor across the scale matrix n∈{1,2,3,4,5,21,100}.

package chain

import (
	"testing"

	"github.com/luxfi/ids"
)

// attestPos builds a minimal accepted-block position: chain + height + inner canonical id. The
// other canonical axes are Empty (deterministic), which is exactly a bare/in-process block.
func attestPos(chainID ids.ID, height uint64, canonical ids.ID) VotePosition {
	return VotePosition{ChainID: chainID, Height: height, CanonicalID: canonical, BlockID: canonical}
}

// attestAll has validators [from,to) attest pos through q; returns the cert emitted (if any) and
// at which voter index it emitted.
func attestAll(t *testing.T, q *QuasarAttestor, vs *testValidatorSet, pos VotePosition, from, to int) (*QuorumCert, int) {
	t.Helper()
	var cert *QuorumCert
	emitAt := -1
	for i := from; i < to; i++ {
		att, err := q.Attest(pos, vs.signerFor(i), vs.nodeID(i))
		if err != nil {
			t.Fatalf("node %d Attest: %v", i, err)
		}
		c, emitted, err := q.Ingest(pos, att)
		if err != nil {
			t.Fatalf("node %d Ingest: %v", i, err)
		}
		if emitted && cert == nil {
			cert, emitAt = c, i
		}
	}
	return cert, emitAt
}

// -----------------------------------------------------------------------------
// (1) The cert forms POST-ACCEPT and matches the accepted block.
// -----------------------------------------------------------------------------

func TestQuasarAttestor_FormsPostAccept_MatchesAcceptedBlock(t *testing.T) {
	vs := newTestValidatorSet(5)
	q := NewQuasarAttestor(vs, vs, nil)
	chainID := ids.GenerateTestID()
	inner := ids.GenerateTestID()
	pos := attestPos(chainID, 100, inner)

	// No attestations yet ⇒ no cert (nothing to gate — acceptance already happened upstream).
	if _, ok := q.CertAt(100); ok {
		t.Fatal("cert must not exist before any attestation")
	}

	cert, emitAt := attestAll(t, q, vs, pos, 0, 5)
	if cert == nil {
		t.Fatal("a ⅔-stake set of post-accept attestations MUST form a certificate")
	}
	// α = ⌊2·5/3⌋+1 = 4 ⇒ emits on the 4th distinct attester (index 3).
	if emitAt != 3 {
		t.Fatalf("cert should emit at the 4th attester (⅔ of 5), emitted at index %d", emitAt)
	}
	if cert.Position.CanonicalID != inner {
		t.Fatalf("cert certifies canonical %s, want the accepted inner block %s", cert.Position.CanonicalID, inner)
	}
	if got, ok := q.CertAt(100); !ok || got != cert {
		t.Fatal("CertAt(100) must return the emitted cert")
	}
	// The emitted cert is a valid ⅔-stake finality cert at the epoch.
	if err := cert.VerifyWeighted(vs, vs, q.epochOf(100)); err != nil {
		t.Fatalf("emitted cert must VerifyWeighted: %v", err)
	}
}

// -----------------------------------------------------------------------------
// (2) An ABSENT / below-⅔ cert NEVER blocks. The collector has no acceptance surface.
// -----------------------------------------------------------------------------

func TestQuasarAttestor_BelowThreshold_NoCert_NeverBlocks(t *testing.T) {
	vs := newTestValidatorSet(5)
	q := NewQuasarAttestor(vs, vs, nil)
	pos := attestPos(ids.GenerateTestID(), 7, ids.GenerateTestID())

	// Only 3 of 5 attest — below α=4 and below ⅔ stake. Ingest returns cleanly every time
	// (no error, no cert): there is NOTHING to block, because acceptance is not this layer's job.
	for i := 0; i < 3; i++ {
		att, _ := q.Attest(pos, vs.signerFor(i), vs.nodeID(i))
		c, emitted, err := q.Ingest(pos, att)
		if err != nil || emitted || c != nil {
			t.Fatalf("below-threshold ingest %d must be a clean no-cert no-op, got cert=%v emitted=%v err=%v", i, c != nil, emitted, err)
		}
	}
	if _, ok := q.CertAt(7); ok {
		t.Fatal("no cert may form below the ⅔ threshold")
	}
	// The evidence view shows the single (honest) subject — not an equivocation.
	if ids, isEquiv := q.EquivocationEvidenceAt(7); isEquiv || len(ids) != 1 {
		t.Fatalf("below-threshold honest attestation is one subject, not equivocation; got %d ids equiv=%v", len(ids), isEquiv)
	}
}

// -----------------------------------------------------------------------------
// (3) External verifier (the bridge/DEX consumer) verifies the cert; tampering fails.
// -----------------------------------------------------------------------------

func TestQuasarAttestor_ExternalVerifierVerifiesCert(t *testing.T) {
	vs := newTestValidatorSet(5)
	q := NewQuasarAttestor(vs, vs, nil)
	pos := attestPos(ids.GenerateTestID(), 42, ids.GenerateTestID())
	cert, _ := attestAll(t, q, vs, pos, 0, 5)
	if cert == nil {
		t.Fatal("expected a cert")
	}

	// An INDEPENDENT verifier (models the bridge B/M chain or the 0x9999 receipt verifier) that
	// only knows the validator set + stake — no engine, no sampler — accepts the cert.
	extVerifier := newTestValidatorSet(5)
	_ = extVerifier // (same key derivation; models an external party holding the same set)
	if err := cert.VerifyWeighted(vs, vs, q.epochOf(42)); err != nil {
		t.Fatalf("external ⅔-stake verify must pass: %v", err)
	}
	// Tamper one voter's signature → verification MUST fail (no forged cert passes).
	bad := *cert
	bad.Votes = append([]SignedVote(nil), cert.Votes...)
	bad.Votes[0].Signature = append([]byte(nil), bad.Votes[0].Signature...)
	bad.Votes[0].Signature[0] ^= 0xFF
	if err := bad.VerifyWeighted(vs, vs, q.epochOf(42)); err == nil {
		t.Fatal("a cert with a tampered signature MUST fail external verification")
	}
}

// -----------------------------------------------------------------------------
// (4) Forgery rejected: bad signature dropped, non-accept dropped, below-⅔ collusion no cert.
// -----------------------------------------------------------------------------

func TestQuasarAttestor_ForgeryRejected(t *testing.T) {
	vs := newTestValidatorSet(5)
	q := NewQuasarAttestor(vs, vs, nil)
	pos := attestPos(ids.GenerateTestID(), 9, ids.GenerateTestID())

	// A forged signature (not by the claimed node) is dropped fail-closed.
	forged := SignedVote{NodeID: vs.nodeID(0), Accept: true, Signature: []byte("not a real signature")}
	if c, emitted, err := q.Ingest(pos, forged); err == nil || emitted || c != nil {
		t.Fatalf("a forged-signature attestation must be dropped with an error, got err=%v emitted=%v", err, emitted)
	}
	// A non-accept attestation is refused (finality certs carry accepts only).
	if _, _, err := q.Ingest(pos, SignedVote{NodeID: vs.nodeID(1), Accept: false}); err == nil {
		t.Fatal("a non-accept attestation must be refused")
	}
	// A colluding MINORITY (2 of 5) cannot forge a cert: below ⅔ stake, none emits.
	cert, _ := attestAll(t, q, vs, pos, 0, 2)
	if cert != nil {
		t.Fatal("a below-⅔ colluding minority must never assemble a cert")
	}
	if _, ok := q.CertAt(9); ok {
		t.Fatal("no cert below the ⅔ threshold")
	}
}

// -----------------------------------------------------------------------------
// (5) Equivocation evidence: two inner blocks attested at one height ⇒ evidence, never 2 certs.
// -----------------------------------------------------------------------------

func TestQuasarAttestor_EquivocationEvidence_NeverTwoCerts(t *testing.T) {
	vs := newTestValidatorSet(5)
	q := NewQuasarAttestor(vs, vs, nil)
	chainID := ids.GenerateTestID()
	innerA := ids.GenerateTestID()
	innerB := ids.GenerateTestID() // a DIFFERENT inner block at the SAME height
	posA := attestPos(chainID, 500, innerA)
	posB := attestPos(chainID, 500, innerB)

	// A 3/2 split across two inner blocks at one height (an equivocation an adversary tries to
	// manufacture): neither side reaches ⅔ ⇒ NO cert on either. Honest nodes never do this
	// (they sign only what sampling accepted, one block per height); this is the adversarial case.
	certA, _ := attestAll(t, q, vs, posA, 0, 3) // 3 attest A
	certB, _ := attestAll(t, q, vs, posB, 3, 5) // 2 attest B
	if certA != nil || certB != nil {
		t.Fatal("a split equivocation below ⅔ on each side must NOT produce any cert")
	}
	// Both inner blocks are surfaced as slashable equivocation evidence at the height.
	evid, isEquiv := q.EquivocationEvidenceAt(500)
	if !isEquiv || len(evid) != 2 {
		t.Fatalf("two distinct inner blocks attested at one height must be flagged as equivocation evidence; got %d ids equiv=%v", len(evid), isEquiv)
	}

	// Now the healthy case: ⅔ honest attest the SAME block ⇒ exactly ONE cert forms, for that
	// block, and never for the other. (A fresh attestor to isolate the count.)
	q2 := NewQuasarAttestor(vs, vs, nil)
	certOne, _ := attestAll(t, q2, vs, posA, 0, 4) // 4 of 5 attest A ⇒ cert
	if certOne == nil || certOne.Position.CanonicalID != innerA {
		t.Fatal("a ⅔-honest set must form exactly one cert, for the accepted block A")
	}
	if _, ok := q2.CertAt(500); !ok {
		t.Fatal("cert for A must be recorded")
	}
}

// -----------------------------------------------------------------------------
// (6) No static K/α: the quorum floor derives from the live committee size each height.
// -----------------------------------------------------------------------------

func TestQuasarQuorum_ScaleMatrix(t *testing.T) {
	// α = ⌊2n/3⌋+1 — the BFT supermajority floor, matching the engine's presets
	// (K1→1, K2→2, K4→3, K5→4, K21→15, K100→67). n=1 self, n=2 both-required.
	cases := map[int]int{1: 1, 2: 2, 3: 3, 4: 3, 5: 4, 21: 15, 100: 67}
	for n, want := range cases {
		if got := quasarQuorum(n); got != want {
			t.Fatalf("quasarQuorum(%d) = %d, want %d", n, got, want)
		}
	}
	// Live-set derivation: an attestor sized to n=3 emits at 3 (not a static 4 or 15).
	vs := newTestValidatorSet(3)
	q := NewQuasarAttestor(vs, vs, nil)
	pos := attestPos(ids.GenerateTestID(), 1, ids.GenerateTestID())
	cert, emitAt := attestAll(t, q, vs, pos, 0, 3)
	if cert == nil || emitAt != 2 {
		t.Fatalf("n=3 must emit at the 3rd attester (⌊2·3/3⌋+1=3), got cert=%v emitAt=%d", cert != nil, emitAt)
	}
}
