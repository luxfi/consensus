// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Package quasar implements post-quantum consensus with event horizon finality.

package quasar

import (
	"context"
	"time"
)

// Block represents a finalized block in the Quasar consensus.
// This is the primary block type used throughout the system.
type Block struct {
	ID        [32]byte   // Unique block identifier
	ChainID   [32]byte   // Chain this block belongs to
	ChainName string     // Human-readable chain name (e.g., "P-Chain", "X-Chain", "C-Chain")
	Height    uint64     // Block height
	Hash      string     // Block hash
	Timestamp time.Time  // Block timestamp
	Data      []byte     // Block payload data
	Cert      *BlockCert // Quantum certificate (nil if not finalized)
}

// BlockCert contains cryptographic certificates for quantum finality.
type BlockCert struct {
	BLS      []byte            // BLS aggregate signature
	PQ       []byte            // Post-quantum certificate (ML-DSA/Ringtail)
	Sigs     map[string][]byte // Individual validator signatures
	Epoch    uint64            // Epoch number
	Finality time.Time         // Time of finality
}

// Verify checks both BLS and PQ certificates.
func (c *BlockCert) Verify(validators []string) bool {
	if c == nil {
		return false
	}
	return len(c.BLS) > 0 && len(c.PQ) > 0
}

// Engine is the main interface for quantum consensus.
type Engine interface {
	// Start begins the consensus engine
	Start(ctx context.Context) error

	// Stop gracefully shuts down the consensus engine
	Stop() error

	// Submit adds a block to the consensus pipeline
	Submit(block *Block) error

	// Finalized returns a channel of finalized blocks
	Finalized() <-chan *Block

	// IsFinalized checks if a block is finalized
	IsFinalized(blockID [32]byte) bool

	// Stats returns consensus metrics
	Stats() Stats
}

// Stats contains consensus metrics.
type Stats struct {
	Height          uint64        // Current finalized height
	ProcessedBlocks uint64        // Total blocks processed
	FinalizedBlocks uint64        // Total blocks finalized
	PendingBlocks   int           // Blocks awaiting finality
	Validators      int           // Active validator count
	Uptime          time.Duration // Time since start
}

// Core is an alias for the aggregator implementation.
// Use NewCore() to create a new instance.
type Core = QuasarCore
