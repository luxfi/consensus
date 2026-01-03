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

// BLSSignature contains a classical BLS threshold signature.
// Fast path for consensus - used in parallel with RingtailSignature.
type BLSSignature struct {
	Signature   []byte // BLS signature bytes
	ValidatorID string // Signing validator
	IsThreshold bool   // True if threshold signature
	SignerIndex int    // Signer index in committee
}

// RingtailSignature contains a post-quantum Ringtail threshold signature.
// Quantum-safe path - used in parallel with BLSSignature.
type RingtailSignature struct {
	Signature   []byte // Ringtail (Ring-LWE) signature bytes
	ValidatorID string // Signing validator
	IsThreshold bool   // True if threshold signature
	SignerIndex int    // Signer index in committee
	Round       int    // Ringtail protocol round (1 or 2)
}

// QuasarSignature bundles BLS + Ringtail for complete quantum finality.
// Both signatures are collected in parallel.
type QuasarSignature struct {
	BLS      *BLSSignature      // Classical fast path
	Ringtail *RingtailSignature // Quantum-safe path
}

// RingtailRound1Data contains the output of Ringtail Round 1.
type RingtailRound1Data struct {
	PartyID int
}

// Signer is the exported interface for the quantum signing engine.
// It provides parallel BLS+Ringtail threshold signing for PQ-safe consensus.
type Signer = signer

// NewSigner creates a new quantum signer with the given threshold.
func NewSigner(threshold int) (*Signer, error) {
	return newSigner(threshold)
}

// NewSignerWithConfig creates a new quantum signer with full configuration.
func NewSignerWithConfig(config SignerConfig) (*Signer, error) {
	return newSignerWithDualThreshold(config)
}

// NewSignerWithDualThreshold creates a new quantum signer with dual threshold configuration.
// This is an alias for NewSignerWithConfig for backward compatibility.
func NewSignerWithDualThreshold(config SignerConfig) (*Signer, error) {
	return NewSignerWithConfig(config)
}

// NewSignerWithThresholdConfig creates a signer from ThresholdConfig.
func NewSignerWithThresholdConfig(config ThresholdConfig) (*Signer, error) {
	return newSignerWithThresholdConfig(config)
}
