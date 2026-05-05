// Copyright (C) 2025, Lux Industries Inc. All rights reserved.

package quasar

import (
	"encoding/binary"
	"fmt"

	"github.com/luxfi/lattice/v7/ring"
	"github.com/luxfi/lattice/v7/utils/structs"
	coronaThreshold "github.com/luxfi/pulsar/threshold"
)

// coronaGobEncode serializes a Pulsar threshold signature using the
// native ring.Poly / structs.Vector binary encoders. The legacy name
// coronaGobEncode is preserved for caller compatibility — actual
// encoding is no longer gob-based. gob added ~10x overhead via reflection
// type metadata; the native path is the raw NTT-coefficient stream
// matching the on-disk Pulsar KAT format.
//
// Wire layout (length-prefixed concatenation):
//
//	[4-byte BE C_len ][C bytes ]
//	[4-byte BE Z_len ][Z bytes ]
//	[4-byte BE Delta_len][Delta bytes]
//
// Returns nil on encoder error; callers treat nil as "no signature
// available" and the round driver falls back to the next-lower witness
// level.
func coronaGobEncode(sig *coronaThreshold.Signature) []byte {
	if sig == nil {
		return nil
	}
	cb, err := sig.C.MarshalBinary()
	if err != nil {
		return nil
	}
	zb, err := sig.Z.MarshalBinary()
	if err != nil {
		return nil
	}
	db, err := sig.Delta.MarshalBinary()
	if err != nil {
		return nil
	}
	out := make([]byte, 0, 12+len(cb)+len(zb)+len(db))
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(cb)))
	out = append(out, hdr[:]...)
	out = append(out, cb...)
	binary.BigEndian.PutUint32(hdr[:], uint32(len(zb)))
	out = append(out, hdr[:]...)
	out = append(out, zb...)
	binary.BigEndian.PutUint32(hdr[:], uint32(len(db)))
	out = append(out, hdr[:]...)
	out = append(out, db...)
	return out
}

// coronaGobDecode is the inverse of coronaGobEncode.
func coronaGobDecode(data []byte) (*coronaThreshold.Signature, error) {
	if len(data) < 12 {
		return nil, ErrCertCorrupt
	}
	out := &coronaThreshold.Signature{}

	off := 0
	readField := func(unmarshal func([]byte) error) error {
		if off+4 > len(data) {
			return ErrCertCorrupt
		}
		n := binary.BigEndian.Uint32(data[off : off+4])
		off += 4
		if off+int(n) > len(data) {
			return ErrCertCorrupt
		}
		if err := unmarshal(data[off : off+int(n)]); err != nil {
			return fmt.Errorf("%w: %v", ErrCertCorrupt, err)
		}
		off += int(n)
		return nil
	}

	if err := readField(func(b []byte) error {
		out.C = ring.Poly{}
		return out.C.UnmarshalBinary(b)
	}); err != nil {
		return nil, err
	}
	if err := readField(func(b []byte) error {
		out.Z = structs.Vector[ring.Poly]{}
		return out.Z.UnmarshalBinary(b)
	}); err != nil {
		return nil, err
	}
	if err := readField(func(b []byte) error {
		out.Delta = structs.Vector[ring.Poly]{}
		return out.Delta.UnmarshalBinary(b)
	}); err != nil {
		return nil, err
	}
	return out, nil
}
