// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// cert_wire_vector_test.go — the two wire formats written down, by hand, from the
// SPEC rather than from the encoder.
//
// A round-trip test proves the encoder agrees with itself. Swap two same-width
// fields in both halves and it still passes, while every peer on the old build
// reads a different message under the same signature. These vectors are assembled
// byte by byte from the layouts documented in canonicalVoteMessageFor and
// cert_codec.go, so a field reordering, a width change, a domain-tag edit or a
// dropped binding fails here.
//
// The vote message is the one that matters most: it is what a validator's key
// actually signs, so it is the statement every quorum argument is about.
package chain

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/luxfi/ids"
)

// fill returns a 32-byte id whose every byte is b — recognisable at a glance in a
// hexdump, so a misplaced field is visible rather than merely unequal.
func fill(b byte) ids.ID {
	var id ids.ID
	for i := range id {
		id[i] = b
	}
	return id
}

func fillNode(b byte) ids.NodeID {
	var n ids.NodeID
	for i := range n {
		n[i] = b
	}
	return n
}

// vectorPosition is the position both vectors below are written against.
func vectorPosition() VotePosition {
	return VotePosition{
		ChainID:            fill(0x11),
		Height:             0x0102030405060708,
		Round:              0x0A0B0C0D,
		BlockID:            fill(0x22),
		ParentID:           fill(0x33),
		CanonicalID:        fill(0x44),
		ParentCanonicalID:  fill(0x55),
		ExecutionStateRoot: fill(0x66),
		PayloadRoot:        fill(0x77),
		ValidatorSetRoot:   fill(0x88),
	}
}

// TestVoteMessage_WireVector pins the SIGNED bytes.
//
//	"LUX/chain/vote/v2\x00"  version:2  qc_type:1
//	chain_id:32  height:8  round:4
//	canonical_block_id:32  parent_canonical_id:32
//	execution_state_root:32  payload_root:32  validator_set_root:32
//	accept:1
//
// The outer envelope ids are deliberately ABSENT: two validators that executed
// one inner block sign identical bytes whatever wrapper they received.
func TestVoteMessage_WireVector(t *testing.T) {
	var want []byte
	want = append(want, "LUX/chain/vote/v2\x00"...)
	want = append(want, 0x00, 0x03) // version 3, big-endian
	want = append(want, 0x01)       // qc_type = QCFinality
	want = append(want, bytes.Repeat([]byte{0x11}, 32)...)
	want = append(want, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08) // height
	want = append(want, 0x0A, 0x0B, 0x0C, 0x0D)                         // round
	want = append(want, bytes.Repeat([]byte{0x44}, 32)...)              // canonical
	want = append(want, bytes.Repeat([]byte{0x55}, 32)...)              // parent canonical
	want = append(want, bytes.Repeat([]byte{0x66}, 32)...)              // execution state root
	want = append(want, bytes.Repeat([]byte{0x77}, 32)...)              // payload root
	want = append(want, bytes.Repeat([]byte{0x88}, 32)...)              // validator set root
	want = append(want, 0x01)                                           // accept

	if got := CanonicalVoteMessage(vectorPosition()); !bytes.Equal(got, want) {
		t.Fatalf("the signed vote message does not match the documented layout.\n got %x\nwant %x", got, want)
	}
	if uint16(3) != QuorumCertVersion {
		t.Fatalf("the vector pins version 3; QuorumCertVersion is now %d — the wire moved, so the "+
			"vector must be rewritten from the new spec, not regenerated from the encoder",
			QuorumCertVersion)
	}
}

// TestVoteMessage_BindsEveryConsensusField: each field the layout names must
// change the message. A field present in the struct but absent from the bytes is
// a value a signature does not cover — free for an attacker to restate.
func TestVoteMessage_BindsEveryConsensusField(t *testing.T) {
	base := CanonicalVoteMessage(vectorPosition())

	bound := map[string]func(*VotePosition){
		"ChainID":            func(p *VotePosition) { p.ChainID = fill(0xF1) },
		"Height":             func(p *VotePosition) { p.Height++ },
		"Round":              func(p *VotePosition) { p.Round++ },
		"CanonicalID":        func(p *VotePosition) { p.CanonicalID = fill(0xF2) },
		"ParentCanonicalID":  func(p *VotePosition) { p.ParentCanonicalID = fill(0xF3) },
		"ExecutionStateRoot": func(p *VotePosition) { p.ExecutionStateRoot = fill(0xF4) },
		"PayloadRoot":        func(p *VotePosition) { p.PayloadRoot = fill(0xF5) },
		"ValidatorSetRoot":   func(p *VotePosition) { p.ValidatorSetRoot = fill(0xF6) },
	}
	for name, mutate := range bound {
		pos := vectorPosition()
		mutate(&pos)
		if bytes.Equal(CanonicalVoteMessage(pos), base) {
			t.Fatalf("%s does not reach the signed message: a signature over this position also "+
				"covers every other value of %s", name, name)
		}
	}

	// The outer envelope ids must NOT reach the message — that is what lets two
	// wrappers of one execution interoperate.
	for name, mutate := range map[string]func(*VotePosition){
		"BlockID":  func(p *VotePosition) { p.BlockID = fill(0xE1) },
		"ParentID": func(p *VotePosition) { p.ParentID = fill(0xE2) },
	} {
		pos := vectorPosition()
		mutate(&pos)
		if !bytes.Equal(CanonicalVoteMessage(pos), base) {
			t.Fatalf("the outer %s reached the signed message; a wrapper id in the signature splits "+
				"the quorum across envelopes of one execution", name)
		}
	}
}

// TestVoteMessage_AcceptAndRejectAreDistinct: a reject signature must never be
// presentable as an accept. They differ in the final byte and nowhere else.
func TestVoteMessage_AcceptAndRejectAreDistinct(t *testing.T) {
	pos := vectorPosition()
	accept := canonicalVoteMessageFor(pos, true)
	reject := canonicalVoteMessageFor(pos, false)
	if bytes.Equal(accept, reject) {
		t.Fatal("accept and reject sign identical bytes — a reject signature is an accept signature")
	}
	if len(accept) != len(reject) || !bytes.Equal(accept[:len(accept)-1], reject[:len(reject)-1]) {
		t.Fatal("accept and reject differ before the decision byte; the decision must be the only difference")
	}
	if accept[len(accept)-1] != 0x01 || reject[len(reject)-1] != 0x00 {
		t.Fatalf("decision byte is %#x/%#x, want 0x01/0x00", accept[len(accept)-1], reject[len(reject)-1])
	}
}

// TestVoteMessage_DegradesUnsetCanonicalToTheOuterID pins the one place the
// non-wrapped degrade lives. A bare chain must sign its outer id under the
// canonical slot, and it must produce the SAME bytes a wrapped position with
// those ids set explicitly produces — otherwise the two disagree about one block.
func TestVoteMessage_DegradesUnsetCanonicalToTheOuterID(t *testing.T) {
	bare := VotePosition{ChainID: fill(0x11), Height: 9, Round: 1, BlockID: fill(0x22), ParentID: fill(0x33)}
	explicit := bare
	explicit.CanonicalID, explicit.ParentCanonicalID = fill(0x22), fill(0x33)

	if !bytes.Equal(CanonicalVoteMessage(bare), CanonicalVoteMessage(explicit)) {
		t.Fatal("a bare position and the same position with canonical == outer sign different bytes")
	}
}

// --- the cert frame -----------------------------------------------------------

// certVector assembles the documented cert frame by hand, with one vote.
func certVector() []byte {
	var b []byte
	b = append(b, 0x00, 0x03) // version:2
	b = append(b, 0x01)       // type:1 = QCFinality
	b = append(b, 0x02)       // tier:1 = Nova
	b = append(b, bytes.Repeat([]byte{0x11}, 32)...)
	b = append(b, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08) // height:8
	b = append(b, 0x0A, 0x0B, 0x0C, 0x0D)                         // round:4
	b = append(b, bytes.Repeat([]byte{0x22}, 32)...)              // block_id
	b = append(b, bytes.Repeat([]byte{0x33}, 32)...)              // parent_id
	b = append(b, bytes.Repeat([]byte{0x44}, 32)...)              // canonical_block_id
	b = append(b, bytes.Repeat([]byte{0x55}, 32)...)              // parent_canonical_id
	b = append(b, bytes.Repeat([]byte{0x66}, 32)...)              // execution_state_root
	b = append(b, bytes.Repeat([]byte{0x77}, 32)...)              // payload_root
	b = append(b, bytes.Repeat([]byte{0x88}, 32)...)              // validator_set_root
	b = append(b, 0x00, 0x00, 0x00, 0x07)                         // threshold:4
	b = append(b, 0x00, 0x00, 0x00, 0x01)                         // vote_count:4
	b = append(b, bytes.Repeat([]byte{0x99}, ids.NodeIDLen)...)   // node_id:20
	b = append(b, 0x01)                                           // accept:1
	b = append(b, 0x00, 0x00, 0x00, 0x03)                         // sig_len:4
	b = append(b, 0xAB, 0xCD, 0xEF)                               // sig
	return b
}

// TestQuorumCertCodec_WireVector decodes the hand-written frame and checks every
// field landed where the layout says it does.
func TestQuorumCertCodec_WireVector(t *testing.T) {
	c, err := UnmarshalQuorumCert(certVector())
	if err != nil {
		t.Fatalf("the documented frame did not decode: %v", err)
	}
	pos := c.Position
	switch {
	case c.Version != 3:
		t.Fatalf("version = %d", c.Version)
	case c.Type != QCFinality:
		t.Fatalf("type = %d", c.Type)
	case c.Tier != Nova:
		t.Fatalf("tier = %s", c.Tier)
	case pos.ChainID != fill(0x11):
		t.Fatalf("chain_id = %s", pos.ChainID)
	case pos.Height != 0x0102030405060708:
		t.Fatalf("height = %#x", pos.Height)
	case pos.Round != 0x0A0B0C0D:
		t.Fatalf("round = %#x", pos.Round)
	case pos.BlockID != fill(0x22):
		t.Fatalf("block_id = %s", pos.BlockID)
	case pos.ParentID != fill(0x33):
		t.Fatalf("parent_id = %s", pos.ParentID)
	case pos.CanonicalID != fill(0x44):
		t.Fatalf("canonical_block_id = %s", pos.CanonicalID)
	case pos.ParentCanonicalID != fill(0x55):
		t.Fatalf("parent_canonical_id = %s", pos.ParentCanonicalID)
	case pos.ExecutionStateRoot != fill(0x66):
		t.Fatalf("execution_state_root = %s", pos.ExecutionStateRoot)
	case pos.PayloadRoot != fill(0x77):
		t.Fatalf("payload_root = %s", pos.PayloadRoot)
	case pos.ValidatorSetRoot != fill(0x88):
		t.Fatalf("validator_set_root = %s", pos.ValidatorSetRoot)
	case c.Threshold != 7:
		t.Fatalf("threshold = %d", c.Threshold)
	case len(c.Votes) != 1:
		t.Fatalf("vote_count = %d", len(c.Votes))
	case c.Votes[0].NodeID != fillNode(0x99):
		t.Fatalf("node_id = %s", c.Votes[0].NodeID)
	case !c.Votes[0].Accept:
		t.Fatal("accept byte 0x01 did not decode as accept")
	case !bytes.Equal(c.Votes[0].Signature, []byte{0xAB, 0xCD, 0xEF}):
		t.Fatalf("signature = %x", c.Votes[0].Signature)
	}

	// The encoder must emit exactly what the spec says it emits.
	got, err := c.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(got, certVector()) {
		t.Fatalf("encoder output differs from the documented frame.\n got %x\nwant %x", got, certVector())
	}
}

// TestQuorumCertCodec_NoTruncatedPrefixDecodes: no proper prefix of a valid frame
// may decode. A decoder that accepts a short read has silently substituted a
// zero for whatever the truncated field was.
func TestQuorumCertCodec_NoTruncatedPrefixDecodes(t *testing.T) {
	full := certVector()
	for n := 0; n < len(full); n++ {
		if _, err := UnmarshalQuorumCert(full[:n]); err == nil {
			t.Fatalf("a %d-byte prefix of a %d-byte cert decoded", n, len(full))
		}
	}
	if _, err := UnmarshalQuorumCert(append(full, 0x00)); err == nil {
		t.Fatal("a trailing byte after a complete frame was accepted")
	}
}

// TestQuorumCertCodec_LengthCannotExceedTheFrame: a sig_len larger than what the
// frame can hold must be refused, and so must a vote_count whose minimum
// footprint exceeds the remaining bytes.
func TestQuorumCertCodec_LengthCannotExceedTheFrame(t *testing.T) {
	full := certVector()
	sigLenAt := len(full) - 3 - 4

	over := append([]byte(nil), full...)
	binary.BigEndian.PutUint32(over[sigLenAt:sigLenAt+4], 0xFFFFFFFF)
	if _, err := UnmarshalQuorumCert(over); err == nil {
		t.Fatal("a sig_len of 0xFFFFFFFF was accepted")
	}

	countAt := 2 + 1 + 1 + 32 + 8 + 4 + 32*7 + 4
	huge := append([]byte(nil), full...)
	binary.BigEndian.PutUint32(huge[countAt:countAt+4], 0xFFFFFFFF)
	if _, err := UnmarshalQuorumCert(huge); err == nil {
		t.Fatal("a vote_count of 0xFFFFFFFF was accepted")
	}
}

// TestQuorumCertCodec_IsCanonical: exactly one byte string per cert. The fuzzer
// states this ("re-encoding must reproduce the SAME bytes the decoder accepted")
// and then only re-decodes, so nothing checks it.
//
// It matters wherever cert bytes are the unit of identity — served to peers from
// a per-block cache, compared for de-duplication, or recorded as evidence. A
// second spelling of one cert is a second entry.
func TestQuorumCertCodec_IsCanonical(t *testing.T) {
	full := certVector()
	acceptAt := len(full) - 3 - 4 - 1

	variant := append([]byte(nil), full...)
	variant[acceptAt] = 0x02 // any non-zero decodes as accept

	c, err := UnmarshalQuorumCert(variant)
	if err != nil {
		return // refusing a non-canonical decision byte is the other valid answer
	}
	got, err := c.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(got, variant) {
		t.Fatalf("the decoder accepted a frame it cannot reproduce: accept byte %#x decoded and "+
			"re-encoded as %#x. Two distinct byte strings name one cert, so anything that treats "+
			"cert bytes as identity — the per-block serving cache, a dedup, a digest — can be "+
			"split in two by flipping a bit no field reads.",
			variant[acceptAt], got[acceptAt])
	}
}
