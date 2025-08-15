// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

// NodeID represents a node identifier
type NodeID [20]byte

// TxID represents a transaction identifier
type TxID [32]byte

// BlockID represents a block identifier
type BlockID [32]byte

// VertexID represents a DAG vertex identifier
type VertexID [32]byte

// Height represents blockchain height
type Height uint64

// Round represents consensus round
type Round uint64

// CertBundle contains dual certificates for PQ-security
type CertBundle struct {
	BLSAgg []byte // compact BLS aggregate signature
	RTCert []byte // Ringtail aggregate certificate
}

// BlockPayload contains linear block consensus fields
type BlockPayload struct {
	FPCVotes [][]byte    // TxID refs; no dupes; owned-only
	EpochBit bool        // epoch fence
	Cert     CertBundle  // if quasar enabled
}

// VertexPayload contains DAG vertex consensus fields
type VertexPayload struct {
	FPCVotes [][]byte
	Cert     CertBundle
}

// Topic represents a consensus topic for sampling
type Topic string

// Probe represents a probe result for peer health
type Probe int

const (
	ProbeGood    Probe = iota
	ProbeTimeout
	ProbeBadSig
)

// Status represents transaction status
type Status int

const (
	StatusUnknown Status = iota
	StatusPending
	StatusAccepted
	StatusRejected
)
