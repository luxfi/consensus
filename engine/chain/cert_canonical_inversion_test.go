// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// cert_canonical_inversion_test.go — adversarial inversions of the QCv2
// canonical-commitment cert. Each test asserts a forged / replayed / malformed /
// sub-quorum / wrong-set / cross-context cert is REJECTED, and — the heart of the
// canonical/transport split — that the OUTER envelope id is NON-AUTHORITATIVE
// (swapping it never changes the signed identity) while the CANONICAL commitment
// IS cryptographically bound (tampering it breaks every signature).
package chain

import (
	"bytes"
	"context"
	"testing"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// fivePos builds a canonical position for a block at height/round.
func canonPos(chainID, outer, parentOuter, inner, parentInner ids.ID, height uint64, round uint32) VotePosition {
	return VotePosition{
		ChainID:           chainID,
		Height:            height,
		Round:             round,
		BlockID:           outer,
		ParentID:          parentOuter,
		CanonicalID:       inner,
		ParentCanonicalID: parentInner,
	}
}

// mkValidCert assembles an n-of-N Ed25519 cert over pos.
func mkValidCert(t *testing.T, vs *testValidatorSet, pos VotePosition, n int) *QuorumCert {
	t.Helper()
	votes := make([]SignedVote, 0, n)
	for i := 0; i < n; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	cert, err := AssembleQuorumCert(pos, uint32(n), votes)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	return cert
}

// TestCanonMsg_ExcludesEnvelope_BindsCanonical is the cryptographic statement of
// the fix: CanonicalVoteMessage depends on the CANONICAL identity and NOT on the
// outer envelope. Two positions that differ ONLY in the outer ids produce the SAME
// signed bytes (duplicates interoperate); positions that differ in ANY canonical
// axis produce DIFFERENT bytes (a fork is a different signature).
func TestCanonMsg_ExcludesEnvelope_BindsCanonical(t *testing.T) {
	chainID := ids.GenerateTestID()
	inner := ids.GenerateTestID()
	parentInner := ids.GenerateTestID()

	base := canonPos(chainID, ids.GenerateTestID(), ids.GenerateTestID(), inner, parentInner, 7, 3)

	// Same canonical identity, DIFFERENT outer envelope ⇒ identical signed message.
	aliased := base
	aliased.BlockID = ids.GenerateTestID()
	aliased.ParentID = ids.GenerateTestID()
	if !bytes.Equal(CanonicalVoteMessage(base), CanonicalVoteMessage(aliased)) {
		t.Fatal("envelope swap changed the signed message — the outer id is NOT supposed to be signed")
	}

	// Any CANONICAL change ⇒ different signed message.
	for name, mutate := range map[string]func(p *VotePosition){
		"canonicalID":        func(p *VotePosition) { p.CanonicalID = ids.GenerateTestID() },
		"parentCanonicalID":  func(p *VotePosition) { p.ParentCanonicalID = ids.GenerateTestID() },
		"executionStateRoot": func(p *VotePosition) { p.ExecutionStateRoot = ids.GenerateTestID() },
		"payloadRoot":        func(p *VotePosition) { p.PayloadRoot = ids.GenerateTestID() },
		"validatorSetRoot":   func(p *VotePosition) { p.ValidatorSetRoot = ids.GenerateTestID() },
		"height":             func(p *VotePosition) { p.Height++ },
		"round":              func(p *VotePosition) { p.Round++ },
		"chainID":            func(p *VotePosition) { p.ChainID = ids.GenerateTestID() },
	} {
		m := base
		mutate(&m)
		if bytes.Equal(CanonicalVoteMessage(base), CanonicalVoteMessage(m)) {
			t.Fatalf("mutating %s did NOT change the signed message (it must be bound)", name)
		}
	}
}

// TestInversion_EnvelopeSwapVerifiesButCanonicalTamperFails: a cert whose OUTER id
// is swapped STILL verifies (the outer id is not signed — it is a transport hint);
// a cert whose CANONICAL id is tampered FAILS (every signature was over the
// original canonical identity). This is the malleability boundary: an attacker may
// relabel the envelope (harmless) but cannot retarget the finalized execution block.
func TestInversion_EnvelopeSwapVerifiesButCanonicalTamperFails(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	// A realistic proposervm block: BOTH canonical ids are populated (non-Empty), so
	// the message binds the canonical identity and never falls back to the outer ids.
	pos := canonPos(chainID, ids.GenerateTestID(), ids.GenerateTestID(), ids.GenerateTestID(), ids.GenerateTestID(), 9, 0)
	cert := mkValidCert(t, vs, pos, 4)

	if err := cert.Verify(vs, 9); err != nil {
		t.Fatalf("baseline cert must verify: %v", err)
	}

	// Swap the OUTER envelope id (transport) — STILL verifies (non-authoritative).
	swapped := *cert
	swapped.Position.BlockID = ids.GenerateTestID()
	swapped.Position.ParentID = ids.GenerateTestID()
	if err := swapped.Verify(vs, 9); err != nil {
		t.Fatalf("envelope swap must NOT break verification (outer id is not signed): %v", err)
	}

	// Tamper the CANONICAL id — every signature was over the old one ⇒ FAILS.
	tampered := *cert
	tampered.Position.CanonicalID = ids.GenerateTestID()
	if err := tampered.Verify(vs, 9); err == nil {
		t.Fatal("tampering the canonical id must break verification (it is cryptographically bound)")
	}
}

// TestInversion_ForgedSignatureRejected: a cert carrying a forged signature (a
// random blob, or another node's key) fails verification.
func TestInversion_ForgedSignatureRejected(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	pos := canonPos(chainID, ids.GenerateTestID(), ids.Empty, ids.GenerateTestID(), ids.Empty, 1, 0)

	good := mkValidCert(t, vs, pos, 4)
	// Replace one signature with garbage.
	forged := *good
	forged.Votes = append([]SignedVote(nil), good.Votes...)
	forged.Votes[2].Signature = bytes.Repeat([]byte{0xAB}, len(forged.Votes[2].Signature))
	if err := forged.Verify(vs, 1); err == nil {
		t.Fatal("a forged signature must be rejected")
	}
}

// TestInversion_WrongSetSignerRejected: a voter that is NOT in the validator set
// contributes an unverifiable signature → rejected (unknown node ⇒ false, never an
// error, never a count).
func TestInversion_WrongSetSignerRejected(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	pos := canonPos(chainID, ids.GenerateTestID(), ids.Empty, ids.GenerateTestID(), ids.Empty, 1, 0)

	outsider := ids.GenerateTestNodeID() // not in vs
	votes := []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
		{NodeID: outsider, Accept: true, Signature: bytes.Repeat([]byte{0x01}, 64)},
	}
	cert, err := AssembleQuorumCert(pos, 4, votes)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if err := cert.Verify(vs, 1); err == nil {
		t.Fatal("a cert padded with an out-of-set signer must not reach quorum")
	}
}

// TestInversion_SubQuorumCertRejectedAtFloor: a cert that asserts a threshold below
// the chain's α floor is rejected at the receive boundary even if its internal
// signatures verify (the MinThreshold floor — sub-quorum finality forgery defence).
func TestInversion_SubQuorumCertRejectedAtFloor(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	rt := newCanonRuntime(t, vs, chainID, 4) // α floor = AlphaPreference = 3 (params5)

	inner := ids.GenerateTestID()
	outer := ids.GenerateTestID()
	trackEnvelope(rt, outer, ids.Empty, inner, ids.Empty, 1, 0)

	// A 2-of-5 cert (threshold 2 < α floor 3) — internally valid signatures but the
	// asserted threshold is below the chain's quorum floor.
	pos := canonPos(chainID, outer, ids.Empty, inner, ids.Empty, 1, 0)
	cert := mkValidCert(t, vs, pos, 2)
	b, _ := cert.MarshalBinary()
	if rt.HandleIncomingCert(b) {
		t.Fatal("a cert below the α floor must be rejected (sub-quorum forgery defence)")
	}
}

// TestInversion_CrossHeightReplayRejected: a cert assembled for height H cannot be
// replayed as height H' — height is in the signed message, so re-stamping it breaks
// every signature.
func TestInversion_CrossHeightReplayRejected(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	pos := canonPos(chainID, ids.GenerateTestID(), ids.Empty, ids.GenerateTestID(), ids.Empty, 10, 0)
	cert := mkValidCert(t, vs, pos, 4)

	replay := *cert
	replay.Position.Height = 11 // re-stamp to a different height
	if err := replay.Verify(vs, 10); err == nil {
		t.Fatal("a cross-height replay must be rejected (height is signed)")
	}
}

// TestInversion_CrossChainReplayRejected: a cert for chain X cannot be replayed on
// chain Y — chainID is in the signed message, and the receive path also drops a
// cert whose position chain != ours.
func TestInversion_CrossChainReplayRejected(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainX := ids.GenerateTestID()
	pos := canonPos(chainX, ids.GenerateTestID(), ids.Empty, ids.GenerateTestID(), ids.Empty, 1, 0)
	cert := mkValidCert(t, vs, pos, 4)

	// Crypto: re-stamping the chain id breaks the signatures.
	replay := *cert
	replay.Position.ChainID = ids.GenerateTestID()
	if err := replay.Verify(vs, 1); err == nil {
		t.Fatal("a cross-chain replay must be rejected (chainID is signed)")
	}

	// Receive path: even a self-consistent cert for ANOTHER chain is dropped.
	chainY := ids.GenerateTestID()
	rt := newCanonRuntime(t, vs, chainY, 4)
	b, _ := cert.MarshalBinary() // cert is for chainX
	if rt.HandleIncomingCert(b) {
		t.Fatal("a cert for a different chain must be dropped by the receive path")
	}
}

// TestInversion_DuplicateVoterRejected: a cert with a repeated NodeID never
// double-counts — assembly rejects it, and a hand-built one fails the
// strictly-increasing clause.
func TestInversion_DuplicateVoterRejected(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	pos := canonPos(chainID, ids.GenerateTestID(), ids.Empty, ids.GenerateTestID(), ids.Empty, 1, 0)

	dup := []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)}, // same node twice
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
	}
	if _, err := AssembleQuorumCert(pos, 4, dup); err == nil {
		t.Fatal("assembling a cert with a duplicate voter must fail")
	}

	// Hand-build a cert that bypasses assembly's dedup — Verify must still reject it
	// on the strictly-increasing clause (no double counting).
	v := vs.signedVote(0, pos)
	raw := &QuorumCert{
		Version:   QuorumCertVersion,
		Type:      QCFinality,
		Position:  pos,
		Threshold: 2,
		Votes:     []SignedVote{{NodeID: v.NodeID, Accept: true, Signature: v.Signature}, {NodeID: v.NodeID, Accept: true, Signature: v.Signature}},
	}
	if err := raw.Verify(vs, 1); err == nil {
		t.Fatal("a cert with non-increasing (duplicate) voters must fail verification")
	}
}

// TestInversion_StaleEpochSetRootMismatchDropped: a cert whose ValidatorSetRoot does
// not match the set-root WE recompute at the block's epoch is dropped by the
// receive-side epoch cross-check — a cert laundered from a different validator-set
// epoch cannot finalize against our set.
func TestInversion_StaleEpochSetRootMismatchDropped(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()

	// Our node computes set-root = a fixed value R for every epoch.
	R := ids.GenerateTestID()
	rt := newCanonRuntimeOpts(t, vs, chainID, 4, WithValidatorSetRoot(ValidatorSetRootFunc(func(uint64) ids.ID { return R })))

	inner := ids.GenerateTestID()
	outer := ids.GenerateTestID()
	trackEnvelope(rt, outer, ids.Empty, inner, ids.Empty, 1, 0)

	// The cert is signed under a DIFFERENT (stale) set-root R'.
	stale := ids.GenerateTestID()
	pos := canonPos(chainID, outer, ids.Empty, inner, ids.Empty, 1, 0)
	pos.ValidatorSetRoot = stale
	cert := mkValidCert(t, vs, pos, 4)
	b, _ := cert.MarshalBinary()
	if rt.HandleIncomingCert(b) {
		t.Fatal("a cert under a stale/foreign validator-set root must be dropped by the epoch cross-check")
	}
}

// newCanonRuntime builds a started follower with the test set as verifier and an α
// floor from params, returning the receive Runtime (Noop logger so the equivocation
// Crit path never exits the test process).
func newCanonRuntime(t *testing.T, vs *testValidatorSet, chainID ids.ID, _ int) *Runtime {
	return newCanonRuntimeOpts(t, vs, chainID, 0)
}

func newCanonRuntimeOpts(t *testing.T, vs *testValidatorSet, chainID ids.ID, _ int, extra ...Option) *Runtime {
	t.Helper()
	opts := append([]Option{WithQuorumCert(chainID, vs.nodeID(4), vs, &recordingGossiper{}, vs.signerFor(4))}, extra...)
	follower := NewWithConfig(Config{Params: params5()}, opts...)
	if err := follower.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = follower.Stop(context.Background()) })
	return &Runtime{Transitive: follower, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}
}
