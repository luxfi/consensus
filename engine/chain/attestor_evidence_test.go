// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// attestor_evidence_test.go — the export sidecar's two jobs, driven at their edges.
//
// The attestor builds the ⅔-by-stake artifact bridges and settlement consume, and it
// is the only place that records which distinct executions stake has signed at a
// height. The existing suite drives both on the SIMULTANEOUS split, where neither
// side certifies. This drives the SEQUENTIAL one, which is the ordering an
// equivocator would actually choose, and the fail-closed edges the file documents.
package chain

import (
	"testing"

	"github.com/luxfi/ids"
)

// TestAttestor_EvidenceOutlivesTheCert. Attesting two executions at one height is
// the signature that proves a validator equivocated, and it is evidence whenever it
// happens — before the cert forms or after.
//
// An equivocator has no reason to race the honest cert. It signs the honest block,
// lets the height certify, and signs the fork afterwards. Both signatures exist and
// both are attributable; only the recording stops.
func TestAttestor_EvidenceOutlivesTheCert(t *testing.T) {
	vs := newTestValidatorSet(5)
	q := NewQuasarAttestor(vs, vs)
	chainID := ids.GenerateTestID()
	honest, fork := ids.GenerateTestID(), ids.GenerateTestID()
	posHonest := attestPos(chainID, 500, honest)
	posFork := attestPos(chainID, 500, fork)

	if cert, _ := attestAll(t, q, vs, posHonest, 0, 4); cert == nil {
		t.Fatal("control broke: 4 of 5 must certify the honest block")
	}
	if evid, _ := q.EquivocationEvidenceAt(500); len(evid) != 1 {
		t.Fatalf("control broke: one execution attested, got %d", len(evid))
	}

	// Validator 0 already signed the honest block. It now signs a second execution
	// at the same height — a provable double-sign, with its own signature on both.
	att, err := q.Attest(posFork, vs.signerFor(0), vs.nodeID(0))
	if err != nil {
		t.Fatalf("Attest: %v", err)
	}
	if _, emitted, _ := q.Ingest(posFork, posFork.Height, att); emitted {
		t.Fatal("SAFETY: a second cert emitted at an already-certified height")
	}

	evid, isEquiv := q.EquivocationEvidenceAt(500)
	if !isEquiv || len(evid) != 2 {
		t.Fatalf("a validator that signs a SECOND execution at a certified height left no "+
			"evidence: EquivocationEvidenceAt reports %d execution(s), equivocation=%v. Ingest "+
			"returns on the already-certified check BEFORE it records the subject, so the "+
			"double-sign that arrives after the cert — the ordering an equivocator would pick, "+
			"since racing the honest cert gains it nothing — is dropped along with the vote. "+
			"The signature verified; only the record of it did not happen.",
			len(evid), isEquiv)
	}
}

// TestAttestor_NilStakeIsRejectedNotFatal. The constructor's own contract: "A nil
// stake source is rejected at emit time (fail-closed — a cert must be ⅔-stake
// checkable)." Rejected means an error returned to the caller. The count floor and
// the ⅔ gate both read the source, and Ingest reaches the floor as soon as one
// verified attestation lands.
func TestAttestor_NilStakeIsRejectedNotFatal(t *testing.T) {
	vs := newTestValidatorSet(5)
	q := NewQuasarAttestor(vs, nil)
	pos := attestPos(ids.GenerateTestID(), 7, ids.GenerateTestID())

	att, err := q.Attest(pos, vs.signerFor(0), vs.nodeID(0))
	if err != nil {
		t.Fatalf("Attest: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Ingest PANICKED on a nil stake source: %v. The file documents this as "+
				"rejected at emit time; a nil source instead reaches "+
				"q.stake.ValidatorCount(epoch) on the first verified attestation. Every "+
				"exported entry point here takes peer data, so a fail-closed path that is "+
				"actually a nil dereference is a remote crash on any node wired without stake.", r)
		}
	}()
	if cert, emitted, err := q.Ingest(pos, pos.Height, att); err == nil && emitted {
		t.Fatalf("a cert emitted with no stake source to check ⅔ against: %v", cert)
	}
}

// TestAttestor_SiblingStatementDoesNotAggregate. Two attestations naming the same
// (height, execution) but signing DIFFERENT statements — here a different validator
// set-root, i.e. a different epoch's committee — must not be pooled. Aggregating
// them would build a cert whose signatures cover two different messages, and it
// would count both toward one threshold.
func TestAttestor_SiblingStatementDoesNotAggregate(t *testing.T) {
	vs := newTestValidatorSet(5)
	q := NewQuasarAttestor(vs, vs)
	chainID := ids.GenerateTestID()
	inner := ids.GenerateTestID()

	pos := attestPos(chainID, 900, inner)
	sibling := pos
	sibling.ValidatorSetRoot = ids.GenerateTestID() // same subject, different committee

	first, err := q.Attest(pos, vs.signerFor(0), vs.nodeID(0))
	if err != nil {
		t.Fatalf("Attest: %v", err)
	}
	if _, _, err := q.Ingest(pos, pos.Height, first); err != nil {
		t.Fatalf("control broke: the first attestation must be accepted: %v", err)
	}

	second, err := q.Attest(sibling, vs.signerFor(1), vs.nodeID(1))
	if err != nil {
		t.Fatalf("Attest: %v", err)
	}
	if _, _, err := q.Ingest(sibling, sibling.Height, second); err == nil {
		t.Fatal("an attestation over a DIFFERENT statement for the same (height, execution) was " +
			"pooled with the first; a cert built from both would carry signatures over two messages")
	}
}

// TestAttestor_MutilatedSignatureRejected: truncating or extending a valid
// signature must fail verification, not merely change it. Both are the cheapest
// wire mutations there are.
func TestAttestor_MutilatedSignatureRejected(t *testing.T) {
	vs := newTestValidatorSet(5)
	q := NewQuasarAttestor(vs, vs)
	pos := attestPos(ids.GenerateTestID(), 42, ids.GenerateTestID())

	good, err := q.Attest(pos, vs.signerFor(0), vs.nodeID(0))
	if err != nil {
		t.Fatalf("Attest: %v", err)
	}
	if _, _, err := q.Ingest(pos, pos.Height, good); err != nil {
		t.Fatalf("control broke: a well-formed attestation must ingest: %v", err)
	}

	for name, sig := range map[string][]byte{
		"truncated": good.Signature[:len(good.Signature)-1],
		"extended":  append(append([]byte(nil), good.Signature...), 0x00),
		"empty":     {},
		"flipped":   flipFirstBit(good.Signature),
	} {
		bad := SignedVote{NodeID: vs.nodeID(1), Accept: true, Signature: sig}
		if _, emitted, err := q.Ingest(pos, pos.Height, bad); err == nil || emitted {
			t.Fatalf("a %s signature was accepted", name)
		}
	}

	// A signature by the WRONG key over the right message, presented under another
	// validator's identity.
	stolen := SignedVote{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(3, pos)}
	if _, _, err := q.Ingest(pos, pos.Height, stolen); err == nil {
		t.Fatal("validator 3's signature was accepted as validator 2's attestation")
	}

	// An ACCEPT slot carrying a non-accept vote.
	reject := good
	reject.Accept = false
	if _, _, err := q.Ingest(pos, pos.Height, reject); err == nil {
		t.Fatal("a non-accept vote was ingested as an attestation")
	}
}

func flipFirstBit(in []byte) []byte {
	out := append([]byte(nil), in...)
	if len(out) > 0 {
		out[0] ^= 0x01
	}
	return out
}

// TestAttestor_CertThresholdTracksTheLiveSet: the emitted artifact must declare the
// floor derived from the committee at the block's epoch, so an external verifier
// re-deriving it from the same set agrees. A cert that under-declares its own floor
// is one an external consumer can be talked into accepting on fewer signatures.
func TestAttestor_CertThresholdTracksTheLiveSet(t *testing.T) {
	for _, n := range []int{1, 2, 4, 5, 7} {
		vs := newTestValidatorSet(n)
		q := NewQuasarAttestor(vs, vs)
		pos := attestPos(ids.GenerateTestID(), 3, ids.GenerateTestID())

		cert, _ := attestAll(t, q, vs, pos, 0, n)
		if cert == nil {
			t.Fatalf("n=%d: the whole set attesting must certify", n)
		}
		if want := uint32(quasarQuorum(n)); cert.Threshold != want {
			t.Fatalf("n=%d: cert declares threshold %d, live floor is %d", n, cert.Threshold, want)
		}
		if err := cert.VerifyWeighted(vs, vs, pos.Height); err != nil {
			t.Fatalf("n=%d: the emitted artifact does not clear its own ⅔ gate: %v", n, err)
		}
		if !cert.AuthorizesExport() {
			t.Fatalf("n=%d: an export artifact must be export-grade", n)
		}
	}
}
