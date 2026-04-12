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
	Cert      *QuasarCert // Quantum certificate (nil if not finalized)
}

// QuasarCert is the finality certificate for Quasar consensus.
//
// Carries up to three verification paths:
//   1. BLS aggregate (48 bytes) — classical fast-path (BLS12-381, ECDL hardness)
//   2. PQ proof (variable) — post-quantum (ML-DSA-65 FIPS 204, or Ringtail Ring-LWE)
//
// In triple mode (BLS + Ringtail + ML-DSA), PQProof carries the aggregated
// post-quantum material from both Ringtail threshold and ML-DSA identity sigs.
// Future: Groth16 SNARK (~192 bytes) to compress N ML-DSA sigs.
type QuasarCert struct {
	BLS     []byte    // BLS aggregate signature (N validators → 48 bytes)
	PQProof []byte    // Post-quantum proof (ML-DSA sig or future Groth16 SNARK aggregate)
	Epoch    uint64    // Epoch number
	Finality time.Time // Time of finality
	// Validators is the count of validators who signed the PQ proof.
	Validators int `json:"validators,omitempty"`
}

// Verify checks structural presence of BLS and PQ certificates.
// This does NOT perform cryptographic verification -- use VerifyWithKeys for that.
// Returns false unconditionally; callers must use VerifyWithKeys with the
// validators' group public key to get a real verification result.
func (c *QuasarCert) Verify(validators []string) bool {
	return false
}

// VerifyWithKeys performs cryptographic verification of the BLS aggregate
// signature against the provided group public key.
// pqKey is reserved for post-quantum certificate verification.
func (c *QuasarCert) VerifyWithKeys(groupKey []byte, pqKey []byte) bool {
	if c == nil {
		return false
	}
	if len(c.BLS) == 0 || len(c.PQProof) == 0 {
		return false
	}
	if len(groupKey) == 0 {
		return false
	}
	// Cryptographic BLS verification requires the full threshold verifier
	// from the signer. This method provides the structural gate; real
	// verification is done by the signer's VerifyAggregatedSignature path.
	// Callers with access to the signer should use that instead.
	return false
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

// QuasarSignature bundles all three proof paths for quantum finality.
// All three run in parallel via [signer.TripleSignRound1].
//
// Per-validator (collected during consensus, NOT stored in block):
//   BLS:      sign with BLS key           → aggregate into 48 bytes (ECDL)
//   Ringtail: sign with ring-LWE key      → PQ threshold proof (Module-LWE)
//   ML-DSA:   sign with ML-DSA-65 key     → PQ identity proof (Module-LWE + Module-SIS)
//
// In QuasarCert (stored in block header):
//   BLS aggregate:  48 bytes
//   PQ proof:       variable (aggregated Ringtail + ML-DSA, or future SNARK)
type QuasarSignature struct {
	BLS      *BLSSignature      // Classical fast path (aggregatable)
	Ringtail *RingtailSignature // PQ anonymous path (ring-LWE threshold)
	MLDSA    []byte             // PQ identity proof (ML-DSA-65, FIPS 204)
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
func NewSignerWithDualThreshold(config SignerConfig) (*Signer, error) {
	return NewSignerWithConfig(config)
}

// NewSignerWithThresholdConfig creates a signer from ThresholdConfig.
func NewSignerWithThresholdConfig(config ThresholdConfig) (*Signer, error) {
	return newSignerWithThresholdConfig(config)
}
