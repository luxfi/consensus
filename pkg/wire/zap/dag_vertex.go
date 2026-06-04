// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"github.com/luxfi/zap"
)

// DAGVertex is the LP-182 schema 0x0C wire view of a DAG vertex
// (engine/dag/vertex.go::Vertex). Carries the vertex ID + parents +
// height + timestamp + payload + UTXO inputs/outputs.
//
// Variable-length concatenated payloads:
//
//	ParentIDs    N * 32B parent vertex IDs
//	Inputs       N * 36B (32 TxID + 4 OutputIndex)
//	Outputs      N * 36B
//	Data         opaque payload
type DAGVertex struct {
	msg *zap.Message
	obj zap.Object
}

const (
	OffsetDAGVertex_ID        = 0  // bytes (32B)
	OffsetDAGVertex_Height    = 8  // uint64
	OffsetDAGVertex_Timestamp = 16 // int64
	OffsetDAGVertex_ParentIDs = 24 // bytes (N * 32B)
	OffsetDAGVertex_Inputs    = 32 // bytes (N * 36B)
	OffsetDAGVertex_Outputs   = 40 // bytes (N * 36B)
	OffsetDAGVertex_Data      = 48 // bytes
	SizeDAGVertex             = 56
)

// DAGVertexFields carries the per-field input set.
type DAGVertexFields struct {
	ID        [32]byte
	Height    uint64
	Timestamp int64
	ParentIDs []byte // N * 32B
	Inputs    []byte // N * (32 TxID + 4 BE OutputIndex)
	Outputs   []byte // N * 36B
	Data      []byte
}

// WrapDAGVertex parses b and returns the typed view.
func WrapDAGVertex(b []byte) (DAGVertex, error) {
	msg, err := zap.Parse(b)
	if err != nil {
		return DAGVertex{}, err
	}
	obj := msg.Root()
	if obj.IsNull() {
		return DAGVertex{}, ErrInvalidQuasarCert
	}
	return DAGVertex{msg: msg, obj: obj}, nil
}

// NewDAGVertex builds a DAG-vertex wire buffer.
func NewDAGVertex(f DAGVertexFields) DAGVertex {
	b := zap.NewBuilder(512)
	ob := b.StartObject(SizeDAGVertex)
	ob.SetBytes(OffsetDAGVertex_ID, f.ID[:])
	ob.SetUint64(OffsetDAGVertex_Height, f.Height)
	ob.SetInt64(OffsetDAGVertex_Timestamp, f.Timestamp)
	ob.SetBytes(OffsetDAGVertex_ParentIDs, f.ParentIDs)
	ob.SetBytes(OffsetDAGVertex_Inputs, f.Inputs)
	ob.SetBytes(OffsetDAGVertex_Outputs, f.Outputs)
	ob.SetBytes(OffsetDAGVertex_Data, f.Data)
	ob.FinishAsRoot()
	buf := b.Finish()
	msg, err := zap.Parse(buf)
	if err != nil {
		panic("zap: NewDAGVertex produced unparseable bytes: " + err.Error())
	}
	return DAGVertex{msg: msg, obj: msg.Root()}
}

func (v DAGVertex) ID() [32]byte {
	var out [32]byte
	copy(out[:], v.obj.Bytes(OffsetDAGVertex_ID))
	return out
}
func (v DAGVertex) Height() uint64    { return v.obj.Uint64(OffsetDAGVertex_Height) }
func (v DAGVertex) Timestamp() int64  { return v.obj.Int64(OffsetDAGVertex_Timestamp) }
func (v DAGVertex) ParentIDs() []byte { return v.obj.Bytes(OffsetDAGVertex_ParentIDs) }
func (v DAGVertex) Inputs() []byte    { return v.obj.Bytes(OffsetDAGVertex_Inputs) }
func (v DAGVertex) Outputs() []byte   { return v.obj.Bytes(OffsetDAGVertex_Outputs) }
func (v DAGVertex) Data() []byte      { return v.obj.Bytes(OffsetDAGVertex_Data) }

// Bytes returns the underlying ZAP buffer.
func (v DAGVertex) Bytes() []byte {
	if v.msg == nil {
		return nil
	}
	return v.msg.Bytes()
}

// IsZero reports whether the vertex is the zero value.
func (v DAGVertex) IsZero() bool { return v.msg == nil }
