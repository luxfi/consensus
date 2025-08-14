// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nebula_test

import (
	"context"

	"github.com/luxfi/consensus/core/interfaces"
)

// Vertex interface for tests
type Vertex interface {
	interfaces.Decidable
	Parents() ([]Vertex, error)
	Height() (uint64, error)
	Txs(context.Context) ([]Tx, error)
	Bytes() []byte
}

// Tx interface for tests
type Tx interface {
	interfaces.Decidable
	Bytes() []byte
}

var _ Vertex = (*TestVertex)(nil)

// TestVertex is a useful test vertex
type TestVertex struct {
	interfaces.TestDecidable

	ParentsV    []Vertex
	ParentsErrV error
	HeightV     uint64
	HeightErrV  error
	TxsV        []Tx
	TxsErrV     error
	BytesV      []byte
}

func (v *TestVertex) Parents() ([]Vertex, error) {
	return v.ParentsV, v.ParentsErrV
}

func (v *TestVertex) Height() (uint64, error) {
	return v.HeightV, v.HeightErrV
}

func (v *TestVertex) Txs(context.Context) ([]Tx, error) {
	return v.TxsV, v.TxsErrV
}

func (v *TestVertex) Bytes() []byte {
	return v.BytesV
}
