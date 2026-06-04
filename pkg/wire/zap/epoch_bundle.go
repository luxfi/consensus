// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"github.com/luxfi/zap"
)

// EpochBundle is the LP-182 schema 0x07 wire view of a Quantum bundle
// (protocol/quasar/epoch.go::QuantumBundle). Bundles N BLS-signed blocks
// into one Corona-signed quantum-safe anchor.
//
// The BlockHashes field carries the concatenation of 32-byte block
// hashes (BlockCount * 32 bytes). The wire layer is shape-agnostic;
// caller slices BlockHashes() at 32-byte strides.
type EpochBundle struct {
	msg *zap.Message
	obj zap.Object
}

const (
	OffsetEpochBundle_Epoch        = 0  // uint64
	OffsetEpochBundle_Sequence     = 8  // uint64
	OffsetEpochBundle_StartHeight  = 16 // uint64
	OffsetEpochBundle_EndHeight    = 24 // uint64
	OffsetEpochBundle_BlockCount   = 32 // uint32
	OffsetEpochBundle_Timestamp    = 40 // int64
	OffsetEpochBundle_MerkleRoot   = 48 // bytes (32B)
	OffsetEpochBundle_PreviousHash = 56 // bytes (32B)
	OffsetEpochBundle_BlockHashes  = 64 // bytes (N * 32B)
	OffsetEpochBundle_Signature    = 72 // bytes (Corona-threshold encoded)
	SizeEpochBundle                = 80
)

// EpochBundleFields carries the per-field input set.
type EpochBundleFields struct {
	Epoch        uint64
	Sequence     uint64
	StartHeight  uint64
	EndHeight    uint64
	BlockCount   uint32
	Timestamp    int64
	MerkleRoot   [32]byte
	PreviousHash [32]byte
	BlockHashes  []byte // BlockCount * 32 bytes
	Signature    []byte // Corona threshold signature (already encoded)
}

// WrapEpochBundle parses b and returns the typed view.
func WrapEpochBundle(b []byte) (EpochBundle, error) {
	msg, err := zap.Parse(b)
	if err != nil {
		return EpochBundle{}, err
	}
	obj := msg.Root()
	if obj.IsNull() {
		return EpochBundle{}, ErrInvalidQuasarCert
	}
	return EpochBundle{msg: msg, obj: obj}, nil
}

// NewEpochBundle builds an epoch-bundle wire buffer.
func NewEpochBundle(f EpochBundleFields) EpochBundle {
	b := zap.NewBuilder(512)
	ob := b.StartObject(SizeEpochBundle)
	ob.SetUint64(OffsetEpochBundle_Epoch, f.Epoch)
	ob.SetUint64(OffsetEpochBundle_Sequence, f.Sequence)
	ob.SetUint64(OffsetEpochBundle_StartHeight, f.StartHeight)
	ob.SetUint64(OffsetEpochBundle_EndHeight, f.EndHeight)
	ob.SetUint32(OffsetEpochBundle_BlockCount, f.BlockCount)
	ob.SetInt64(OffsetEpochBundle_Timestamp, f.Timestamp)
	ob.SetBytes(OffsetEpochBundle_MerkleRoot, f.MerkleRoot[:])
	ob.SetBytes(OffsetEpochBundle_PreviousHash, f.PreviousHash[:])
	ob.SetBytes(OffsetEpochBundle_BlockHashes, f.BlockHashes)
	ob.SetBytes(OffsetEpochBundle_Signature, f.Signature)
	ob.FinishAsRoot()
	buf := b.Finish()
	msg, err := zap.Parse(buf)
	if err != nil {
		panic("zap: NewEpochBundle produced unparseable bytes: " + err.Error())
	}
	return EpochBundle{msg: msg, obj: msg.Root()}
}

func (e EpochBundle) Epoch() uint64       { return e.obj.Uint64(OffsetEpochBundle_Epoch) }
func (e EpochBundle) Sequence() uint64    { return e.obj.Uint64(OffsetEpochBundle_Sequence) }
func (e EpochBundle) StartHeight() uint64 { return e.obj.Uint64(OffsetEpochBundle_StartHeight) }
func (e EpochBundle) EndHeight() uint64   { return e.obj.Uint64(OffsetEpochBundle_EndHeight) }
func (e EpochBundle) BlockCount() uint32  { return e.obj.Uint32(OffsetEpochBundle_BlockCount) }
func (e EpochBundle) Timestamp() int64    { return e.obj.Int64(OffsetEpochBundle_Timestamp) }

func (e EpochBundle) MerkleRoot() [32]byte {
	var out [32]byte
	copy(out[:], e.obj.Bytes(OffsetEpochBundle_MerkleRoot))
	return out
}

func (e EpochBundle) PreviousHash() [32]byte {
	var out [32]byte
	copy(out[:], e.obj.Bytes(OffsetEpochBundle_PreviousHash))
	return out
}

func (e EpochBundle) BlockHashes() []byte {
	return e.obj.Bytes(OffsetEpochBundle_BlockHashes)
}

func (e EpochBundle) Signature() []byte {
	return e.obj.Bytes(OffsetEpochBundle_Signature)
}

// Bytes returns the underlying ZAP buffer.
func (e EpochBundle) Bytes() []byte {
	if e.msg == nil {
		return nil
	}
	return e.msg.Bytes()
}

// IsZero reports whether the bundle is the zero value.
func (e EpochBundle) IsZero() bool { return e.msg == nil }
