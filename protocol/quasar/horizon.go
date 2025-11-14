// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Horizon - The event horizon interface for quantum consensus

package quasar

import (
	"context"
	"time"
)

// Horizon is the event horizon boundary where blocks achieve quantum finality
type Horizon interface {
	// Start activates the event horizon
	Start(ctx context.Context) error

	// Ingest pulls a batch of blocks into the event horizon
	Ingest(ctx context.Context, batch *FinalityBatch) error

	// VerifyProof validates a quantum finality proof
	VerifyProof(ctx context.Context, proof *FinalityProof) error

	// Snapshot returns current state of the horizon
	Snapshot() HorizonStats

	// RegisteredChains returns all chains in the event horizon
	RegisteredChains() []ChainID
}

// Block from any chain entering the event horizon
type Block struct {
	Chain     ChainID
	ID        [32]byte
	Height    uint64
	Timestamp time.Time
	Data      []byte
}

// FinalizedBlock with quantum signatures from the singularity
type FinalizedBlock struct {
	Height     uint64
	Sources    []*Block
	Hash       string
	Timestamp  time.Time
	Signatures map[string]*HybridSignature
}

// FinalityBatch is a batch of blocks entering the event horizon
type FinalityBatch struct {
	Chain  ChainID
	Blocks []*Block
}

// FinalityProof is cryptographic proof of quantum finality
type FinalityProof struct {
	BlockHash string
	Height    uint64
	BLS       []byte // Classical signature
	Ringtail  []byte // Post-quantum signature (ML-DSA)
	Timestamp int64
}

// HorizonStats provides metrics from the event horizon
type HorizonStats struct {
	Height uint64   // Quantum height
	Blocks uint64   // Total blocks processed
	Proofs uint64   // Quantum proofs generated
	Chains []string // Active chains
}

// ChainID identifies a blockchain
type ChainID = string
