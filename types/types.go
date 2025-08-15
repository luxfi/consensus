// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

import "time"

type Topic string

type NodeID string

type Probe int

const (
	ProbeGood Probe = iota
	ProbeTimeout
	ProbeBadSig
)

// Decision is already defined in decision.go

type Round struct {
	Height uint64
	Time   time.Time
}

type Slot struct {
	Round uint64
	Index uint16 // multi-proposer if needed
}

type Digest [32]byte

// BlockID represents a block identifier
type BlockID [32]byte

// VertexID represents a DAG vertex identifier
type VertexID [32]byte

// TxRef represents a transaction reference
type TxRef [32]byte
