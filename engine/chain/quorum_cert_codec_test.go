// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"testing"

	"github.com/luxfi/ids"
)

// TestQuorumCertCodec_RoundTrip proves the gossip wire codec is exact:
// marshal→unmarshal yields an equal cert, and a verified cert stays verified.
func TestQuorumCertCodec_RoundTrip(t *testing.T) {
	vs := newTestValidatorSet(4)
	pos := VotePosition{
		ChainID:  ids.GenerateTestID(),
		Height:   12345,
		Round:    7,
		BlockID:  ids.GenerateTestID(),
		ParentID: ids.GenerateTestID(),
	}
	votes := make([]SignedVote, 0, 4)
	for i := 0; i < 4; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	cert, err := AssembleQuorumCert(pos, 3, votes)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if err := cert.Verify(vs); err != nil {
		t.Fatalf("original cert must verify: %v", err)
	}

	enc, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalQuorumCert(enc)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !cert.Equal(got) {
		t.Fatal("round-trip cert must equal the original")
	}
	if err := got.Verify(vs); err != nil {
		t.Fatalf("round-trip cert must still verify: %v", err)
	}
}

// TestQuorumCertCodec_RejectsCorrupt proves the decoder is fail-closed:
// trailing bytes, short reads, and an oversized vote_count are all rejected
// without panic or unbounded allocation.
func TestQuorumCertCodec_RejectsCorrupt(t *testing.T) {
	vs := newTestValidatorSet(3)
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()}
	votes := []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
	}
	cert, _ := AssembleQuorumCert(pos, 3, votes)
	enc, _ := cert.MarshalBinary()

	// Trailing byte → reject.
	if _, err := UnmarshalQuorumCert(append(enc, 0x00)); err == nil {
		t.Fatal("trailing byte must be rejected")
	}
	// Truncated → reject.
	if _, err := UnmarshalQuorumCert(enc[:len(enc)-1]); err == nil {
		t.Fatal("truncated input must be rejected")
	}
	// Empty → reject.
	if _, err := UnmarshalQuorumCert(nil); err == nil {
		t.Fatal("empty input must be rejected")
	}
	// Oversized vote_count (header claims many votes, no body) → reject in O(1).
	// Build a header with a huge vote_count and no records.
	hdr := make([]byte, qcHeaderSize)
	// version=1
	hdr[0], hdr[1] = 0x00, 0x01
	hdr[2] = byte(QCFinality)
	// set vote_count (last 4 bytes of header) to 0xFFFFFFFF
	hdr[qcHeaderSize-4] = 0xFF
	hdr[qcHeaderSize-3] = 0xFF
	hdr[qcHeaderSize-2] = 0xFF
	hdr[qcHeaderSize-1] = 0xFF
	if _, err := UnmarshalQuorumCert(hdr); err == nil {
		t.Fatal("oversized vote_count must be rejected without allocation")
	}
}

// TestQuorumCert_DuplicateVoterRejected proves the distinctness clause:
// assembly drops a structurally-impossible duplicate, and a hand-crafted cert
// with non-increasing voters fails Verify.
func TestQuorumCert_DuplicateVoterRejected(t *testing.T) {
	vs := newTestValidatorSet(2)
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()}

	// Assembly rejects a duplicate NodeID.
	_, err := AssembleQuorumCert(pos, 2, []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
	})
	if err == nil {
		t.Fatal("assembly must reject a duplicate voter")
	}

	// Hand-craft a cert whose votes are NOT strictly increasing → Verify fails.
	bad := &QuorumCert{
		Version: QuorumCertVersion, Type: QCFinality, Position: pos, Threshold: 1,
		Votes: []SignedVote{
			{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
			{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)}, // dup
		},
	}
	if err := bad.Verify(vs); err == nil {
		t.Fatal("Verify must reject non-strictly-increasing voters")
	}
}

// TestQuorumCert_NilVerifierFailsClosed proves a cert can never be trusted
// without a verifier.
func TestQuorumCert_NilVerifierFailsClosed(t *testing.T) {
	vs := newTestValidatorSet(1)
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()}
	cert, _ := AssembleQuorumCert(pos, 1, []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
	})
	if err := cert.Verify(nil); err == nil {
		t.Fatal("Verify(nil) must fail closed")
	}
}
