// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_cert_codec.go — deterministic wire codec for the engine-level
// QuorumCert. A finality cert is gossiped so followers can verify α-of-K
// finality without re-collecting votes themselves; this is its on-wire form.
//
// All multi-byte integers are big-endian. Decoding is fail-closed on every
// short read and rejects trailing bytes, and caps the attacker-controlled vote
// count against the remaining buffer BEFORE any allocation (decode-DoS guard).
package chain

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/luxfi/ids"
)

// ErrQCWireCorrupt is returned by the decoder on any structural defect.
var ErrQCWireCorrupt = errors.New("chain: quorum cert wire corrupt")

// Wire layout (big-endian):
//
//	version:2
//	type:1
//	chain_id:32
//	height:8
//	round:4
//	block_id:32
//	parent_id:32
//	threshold:4
//	vote_count:4
//	then vote_count records, each:
//	  node_id:20          (ids.NodeID is a 20-byte value)
//	  accept:1
//	  sig_len:4  sig:sig_len

// qcVoteFixed is the fixed-size prefix of one encoded vote (node_id + accept +
// sig_len). Used to cap vote_count against the buffer before allocation.
const qcVoteFixed = ids.NodeIDLen + 1 + 4

// qcHeaderSize is the fixed header byte length preceding the vote records.
const qcHeaderSize = 2 + 1 + 32 + 8 + 4 + 32 + 32 + 4 + 4

// MarshalBinary encodes the cert deterministically. Equal certs encode to equal
// bytes (votes are kept in the strictly-increasing order Assemble produced).
func (c *QuorumCert) MarshalBinary() ([]byte, error) {
	if c == nil {
		return nil, ErrQCNil
	}
	buf := make([]byte, 0, qcHeaderSize)
	var u16 [2]byte
	binary.BigEndian.PutUint16(u16[:], c.Version)
	buf = append(buf, u16[:]...)
	buf = append(buf, byte(c.Type))
	buf = append(buf, c.Position.ChainID[:]...)
	var u64 [8]byte
	binary.BigEndian.PutUint64(u64[:], c.Position.Height)
	buf = append(buf, u64[:]...)
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], c.Position.Round)
	buf = append(buf, u32[:]...)
	buf = append(buf, c.Position.BlockID[:]...)
	buf = append(buf, c.Position.ParentID[:]...)
	binary.BigEndian.PutUint32(u32[:], c.Threshold)
	buf = append(buf, u32[:]...)
	binary.BigEndian.PutUint32(u32[:], uint32(len(c.Votes)))
	buf = append(buf, u32[:]...)

	for i := range c.Votes {
		v := &c.Votes[i]
		buf = append(buf, v.NodeID[:]...)
		if v.Accept {
			buf = append(buf, 0x01)
		} else {
			buf = append(buf, 0x00)
		}
		binary.BigEndian.PutUint32(u32[:], uint32(len(v.Signature)))
		buf = append(buf, u32[:]...)
		buf = append(buf, v.Signature...)
	}
	return buf, nil
}

// UnmarshalQuorumCert is the inverse of MarshalBinary. Strict trailing-bytes
// policy and fail-closed on every short read.
func UnmarshalQuorumCert(data []byte) (*QuorumCert, error) {
	r := &qcWireReader{buf: data}
	c := &QuorumCert{}

	v16, err := r.u16()
	if err != nil {
		return nil, ErrQCWireCorrupt
	}
	c.Version = v16
	t8, err := r.u8()
	if err != nil {
		return nil, ErrQCWireCorrupt
	}
	c.Type = QCType(t8)
	if err = r.readIDInto(&c.Position.ChainID); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if c.Position.Height, err = r.u64(); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if c.Position.Round, err = r.u32(); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if err = r.readIDInto(&c.Position.BlockID); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if err = r.readIDInto(&c.Position.ParentID); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if c.Threshold, err = r.u32(); err != nil {
		return nil, ErrQCWireCorrupt
	}
	voteCount, err := r.u32()
	if err != nil {
		return nil, ErrQCWireCorrupt
	}

	// Cap vote_count against the remaining buffer BEFORE reserving capacity:
	// each vote occupies at least qcVoteFixed bytes, so a count whose minimum
	// footprint exceeds the bytes that remain is structurally impossible.
	// Rejects an adversarial header (vote_count = 0xFFFFFFFF) in O(1).
	if uint64(voteCount)*uint64(qcVoteFixed) > uint64(len(r.buf)) {
		return nil, fmt.Errorf("%w: vote_count %d exceeds remaining buffer (%d bytes)",
			ErrQCWireCorrupt, voteCount, len(r.buf))
	}
	c.Votes = make([]SignedVote, 0, voteCount)
	for i := uint32(0); i < voteCount; i++ {
		var sv SignedVote
		if err = r.readNodeIDInto(&sv.NodeID); err != nil {
			return nil, ErrQCWireCorrupt
		}
		ab, err := r.u8()
		if err != nil {
			return nil, ErrQCWireCorrupt
		}
		sv.Accept = ab != 0
		if sv.Signature, err = r.lenPrefixed(); err != nil {
			return nil, ErrQCWireCorrupt
		}
		c.Votes = append(c.Votes, sv)
	}

	if len(r.buf) != 0 {
		return nil, fmt.Errorf("%w: %d trailing bytes", ErrQCWireCorrupt, len(r.buf))
	}
	return c, nil
}

// qcWireReader is a bounds-checked sequential reader for the cert codec.
type qcWireReader struct{ buf []byte }

func (r *qcWireReader) need(n int) bool { return len(r.buf) >= n }

func (r *qcWireReader) u8() (uint8, error) {
	if !r.need(1) {
		return 0, ErrQCWireCorrupt
	}
	v := r.buf[0]
	r.buf = r.buf[1:]
	return v, nil
}

func (r *qcWireReader) u16() (uint16, error) {
	if !r.need(2) {
		return 0, ErrQCWireCorrupt
	}
	v := binary.BigEndian.Uint16(r.buf[:2])
	r.buf = r.buf[2:]
	return v, nil
}

func (r *qcWireReader) u32() (uint32, error) {
	if !r.need(4) {
		return 0, ErrQCWireCorrupt
	}
	v := binary.BigEndian.Uint32(r.buf[:4])
	r.buf = r.buf[4:]
	return v, nil
}

func (r *qcWireReader) u64() (uint64, error) {
	if !r.need(8) {
		return 0, ErrQCWireCorrupt
	}
	v := binary.BigEndian.Uint64(r.buf[:8])
	r.buf = r.buf[8:]
	return v, nil
}

func (r *qcWireReader) readIDInto(dst *ids.ID) error {
	if !r.need(ids.IDLen) {
		return ErrQCWireCorrupt
	}
	copy(dst[:], r.buf[:ids.IDLen])
	r.buf = r.buf[ids.IDLen:]
	return nil
}

func (r *qcWireReader) readNodeIDInto(dst *ids.NodeID) error {
	if !r.need(ids.NodeIDLen) {
		return ErrQCWireCorrupt
	}
	copy(dst[:], r.buf[:ids.NodeIDLen])
	r.buf = r.buf[ids.NodeIDLen:]
	return nil
}

func (r *qcWireReader) lenPrefixed() ([]byte, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if uint64(n) > uint64(len(r.buf)) {
		return nil, ErrQCWireCorrupt
	}
	out := make([]byte, n)
	copy(out, r.buf[:n])
	r.buf = r.buf[n:]
	return out, nil
}
