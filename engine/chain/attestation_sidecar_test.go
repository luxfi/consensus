// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// attestation_sidecar_test.go — the attestor's edges: what it refuses to sign,
// what it remembers, and what it forgets.
//
// The attestor is an artifact builder and never a decider, which means every one
// of its failures is best-effort by design — a node that cannot sign simply
// contributes no attestation and the chain does not care. That is exactly why
// the refusals need stating: a builder whose failures are all harmless is a
// builder whose failures are never noticed, and "returns an error" and "returns
// a vote with no signature in it" look identical to a caller that ignores the
// error.
package chain

import (
	"errors"
	"testing"

	"github.com/luxfi/ids"
)

// errNoSigner is what a node with no usable signing key answers with.
var errNoSigner = errors.New("this node holds no signing key at this epoch")

// TestAttestRefusesRatherThanEmitAnUnsignedVote holds the two ways signing can
// fail. Both must answer with an error and the ZERO vote — an attestation with
// an empty signature would be gossiped, fail verification at every peer, and
// look to this node like it had contributed.
func TestAttestRefusesRatherThanEmitAnUnsignedVote(t *testing.T) {
	f := newCertFixture(t, 4)
	a := NewQuasarAttestor(f.vs, f.stake)

	for _, row := range []struct {
		holds  string
		signer VoteSigner
	}{
		{"no signer at all", nil},
		{"a signer that cannot sign", voteSignerFunc(func([]byte) ([]byte, error) { return nil, errNoSigner })},
	} {
		t.Run(row.holds, func(t *testing.T) {
			vote, err := a.Attest(f.pos, row.signer, f.vs.nodeID(0))
			if err == nil {
				t.Fatal("a node that could not sign reported an attestation")
			}
			if vote.Signature != nil || vote.NodeID != (ids.NodeID{}) || vote.Accept {
				t.Fatalf("a refused attestation carried a vote: %+v", vote)
			}
		})
	}

	// And the control: a working signer produces an attestation the cert path
	// accepts, so the refusals above are about signing and not about the fixture.
	vote, err := a.Attest(f.pos, f.vs.signerFor(0), f.vs.nodeID(0))
	if err != nil {
		t.Fatalf("Attest: %v", err)
	}
	if !vote.Accept || vote.NodeID != f.vs.nodeID(0) {
		t.Fatalf("the attestation is not this node's accept: %+v", vote)
	}
	if !f.vs.VerifyVote(vote.NodeID, CanonicalVoteMessage(f.pos), vote.Signature, f.pos.Height) {
		t.Fatal("the attestation does not verify over the position it attests")
	}
}

// TestEquivocationEvidenceIsAbsentUntilThereIsAny separates the two answers a
// height can give. Nothing attested and one thing attested are both "no
// evidence", and only the second bool distinguishes a quiet height from a
// healthy one — a caller that read the slice alone would see the same emptiness
// for a height nobody has voted at and a height everybody agrees on.
func TestEquivocationEvidenceIsAbsentUntilThereIsAny(t *testing.T) {
	f := newCertFixture(t, 4)
	a := NewQuasarAttestor(f.vs, f.stake)

	if ids, ok := a.EquivocationEvidenceAt(f.pos.Height); ids != nil || ok {
		t.Fatalf("a height nobody attested reports evidence: %v %v", ids, ok)
	}

	for i := 0; i < 2; i++ {
		if _, _, err := a.Ingest(f.pos, f.pos.Height, f.votes[i]); err != nil {
			t.Fatalf("Ingest: %v", err)
		}
	}
	subjects, contested := a.EquivocationEvidenceAt(f.pos.Height)
	if len(subjects) != 1 || contested {
		t.Fatalf("one attested canonical is not evidence: %v %v", subjects, contested)
	}

	// A second canonical at the same height IS evidence: stake signed two
	// different blocks at one height, which the ⅔ threshold stops turning into
	// two certs but does not stop being slashable.
	fork := f.pos
	fork.CanonicalID = ids.GenerateTestID()
	forkVote := SignedVote{NodeID: f.vs.nodeID(2), Accept: true, Signature: f.vs.sign(2, fork)}
	if _, _, err := a.Ingest(fork, f.pos.Height, forkVote); err != nil {
		t.Fatalf("Ingest fork: %v", err)
	}
	subjects, contested = a.EquivocationEvidenceAt(f.pos.Height)
	if len(subjects) != 2 || !contested {
		t.Fatalf("two attested canonicals at one height must be evidence: %v %v", subjects, contested)
	}
}

// TestPruneBelowDropsEveryHeightUnderTheWindow holds the memory bound across all
// three maps the attestor keeps. Pruning two of them and forgetting the third is
// the shape of leak that shows up as unbounded growth on a long-running node and
// nowhere else, so the test asserts on all three and on the boundary itself:
// strictly below goes, the height named stays.
func TestPruneBelowDropsEveryHeightUnderTheWindow(t *testing.T) {
	f := newCertFixture(t, 4)
	a := NewQuasarAttestor(f.vs, f.stake)

	// Two heights: one that emits a cert (so certs and subjects are populated)
	// and one left in flight below the threshold (so a bucket survives).
	emitted := f.pos
	emitted.Height = 10
	for i := range f.votes {
		v := SignedVote{NodeID: f.vs.nodeID(i), Accept: true, Signature: f.vs.sign(i, emitted)}
		if _, _, err := a.Ingest(emitted, emitted.Height, v); err != nil {
			t.Fatalf("Ingest: %v", err)
		}
	}
	if _, ok := a.CertAt(emitted.Height); !ok {
		t.Fatal("a unanimous set of attestations did not emit a cert — the fixture proves nothing")
	}

	// A height left in flight BELOW the prune point: its bucket is the one the
	// prune has to reclaim, and it is a different map from the two above.
	stale := f.pos
	stale.Height = 9
	stale.CanonicalID = ids.GenerateTestID()
	staleVote := SignedVote{NodeID: f.vs.nodeID(0), Accept: true, Signature: f.vs.sign(0, stale)}
	if _, _, err := a.Ingest(stale, stale.Height, staleVote); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	inFlight := f.pos
	inFlight.Height = 11
	inFlight.CanonicalID = ids.GenerateTestID()
	lone := SignedVote{NodeID: f.vs.nodeID(0), Accept: true, Signature: f.vs.sign(0, inFlight)}
	if _, emittedNow, err := a.Ingest(inFlight, inFlight.Height, lone); err != nil || emittedNow {
		t.Fatalf("one attestation must not emit a cert: %v %v", emittedNow, err)
	}

	// Prune at the in-flight height: everything strictly below it goes.
	a.PruneBelow(inFlight.Height)

	if _, ok := a.CertAt(emitted.Height); ok {
		t.Fatal("a pruned height still answers with its cert")
	}
	if subjects, _ := a.EquivocationEvidenceAt(emitted.Height); subjects != nil {
		t.Fatalf("a pruned height still answers with its subjects: %v", subjects)
	}
	if subjects, _ := a.EquivocationEvidenceAt(inFlight.Height); len(subjects) != 1 {
		t.Fatalf("the height named by the prune must survive it, got %v", subjects)
	}
	// The stale in-flight bucket is gone: its remaining attestations can no
	// longer arrive, so holding it is holding memory nothing will ever use.
	// Re-ingesting at that height starts a fresh bucket rather than joining one.
	if _, _, err := a.Ingest(stale, stale.Height, staleVote); err != nil {
		t.Fatalf("Ingest at a pruned height: %v", err)
	}
	if subjects, _ := a.EquivocationEvidenceAt(stale.Height); len(subjects) != 1 {
		t.Fatalf("a pruned height did not start fresh: %v", subjects)
	}

	// The in-flight bucket survived, which is what lets the height still reach a
	// quorum: the remaining attestations arrive and the cert emits.
	for i := 1; i < len(f.votes); i++ {
		v := SignedVote{NodeID: f.vs.nodeID(i), Accept: true, Signature: f.vs.sign(i, inFlight)}
		if _, _, err := a.Ingest(inFlight, inFlight.Height, v); err != nil {
			t.Fatalf("Ingest: %v", err)
		}
	}
	if _, ok := a.CertAt(inFlight.Height); !ok {
		t.Fatal("the surviving bucket lost its earlier attestation — the prune took too much")
	}
}

// TestBuildVerifiedQuorumCertFailsClosed holds the sole multi-validator producer
// of the authority token to its own contract: the token exists only where the
// full predicate passed, and every way of not passing gives the same
// ErrNoVerifiedQC to the control flow with the precise cause joined on for
// diagnosis. That pairing is the point — a caller branches on one error, an
// operator reads the other.
func TestBuildVerifiedQuorumCertFailsClosed(t *testing.T) {
	f := newCertFixture(t, 4)
	alpha := uint32(NovaSignerFloor(4))

	// The control: a verified token, and its cert is the one that was checked.
	tok, err := BuildVerifiedQuorumCert(f.vs, f.stake, Nova, alpha, f.pos.Height, f.pos, f.votes[:3])
	if err != nil {
		t.Fatalf("BuildVerifiedQuorumCert: %v", err)
	}
	if tok.Cert() == nil || tok.Cert().VoterCount() != 3 {
		t.Fatalf("the token does not carry the cert that was verified: %+v", tok.Cert())
	}

	for _, row := range []struct {
		holds string
		build func() (VerifiedQuorumCert, error)
		cause error
	}{
		{
			holds: "no verifier is a refusal, never a token nothing checked",
			build: func() (VerifiedQuorumCert, error) {
				return BuildVerifiedQuorumCert(nil, f.stake, Nova, alpha, f.pos.Height, f.pos, f.votes[:3])
			},
		},
		{
			holds: "votes short of the count floor do not assemble",
			build: func() (VerifiedQuorumCert, error) {
				return BuildVerifiedQuorumCert(f.vs, f.stake, Nova, alpha, f.pos.Height, f.pos, f.votes[:1])
			},
			cause: ErrQCBelowThreshold,
		},
		{
			holds: "an assembled cert that fails the weighted predicate is still not a token",
			build: func() (VerifiedQuorumCert, error) {
				short := f.stake
				short.signerStake = 0
				return BuildVerifiedQuorumCert(f.vs, short, Nova, alpha, f.pos.Height, f.pos, f.votes[:3])
			},
			cause: ErrQCStakeBelowMajority,
		},
		{
			holds: "with no stake source the count-only predicate still runs, and still refuses",
			build: func() (VerifiedQuorumCert, error) {
				bad := make([]SignedVote, 3)
				copy(bad, f.votes[:3])
				bad[0].Signature = append([]byte(nil), bad[0].Signature...)
				bad[0].Signature[0] ^= 0xFF
				return BuildVerifiedQuorumCert(f.vs, nil, Nova, alpha, f.pos.Height, f.pos, bad)
			},
			cause: ErrQCSigInvalid,
		},
	} {
		t.Run(row.holds, func(t *testing.T) {
			tok, err := row.build()
			if !errors.Is(err, ErrNoVerifiedQC) {
				t.Fatalf("the caller's error must be ErrNoVerifiedQC, got %v", err)
			}
			if row.cause != nil && !errors.Is(err, row.cause) {
				t.Fatalf("the precise cause must ride along for diagnosis, want %v got %v", row.cause, err)
			}
			if tok.Cert() != nil {
				t.Fatal("a refused build returned a cert")
			}
		})
	}
}
