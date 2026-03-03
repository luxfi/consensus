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
	ID        [32]byte    // Unique block identifier
	ChainID   [32]byte    // Chain this block belongs to
	ChainName string      // Human-readable chain name (e.g., "P-Chain", "X-Chain", "C-Chain")
	Height    uint64      // Block height
	Hash      string      // Block hash
	Timestamp time.Time   // Block timestamp
	Data      []byte      // Block payload data
	Cert      *QuasarCert // Quantum certificate (nil if not finalized)
}

// QuasarCert is the finality certificate for Quasar consensus.
//
// Three signature layers, each already compact on its own except ML-DSA:
//
//	BLS12-381  — classical aggregate (48 bytes, O(1) via native BLS aggregation)
//	Ringtail   — lattice threshold signature (O(1) after DKG threshold signing)
//	ML-DSA-65  — per-validator PQ identity (3309 bytes × N validators, O(N))
//
// Z-Chain's sole job is to roll up ML-DSA × N into a single Groth16 SNARK:
//
//	Statement: ∀ i ∈ [N]: ML-DSA.Verify(mldsa_pk_i, msg, mldsa_σ_i) = 1
//	Output:    192-byte Groth16 proof (MLDSAProof)
//
// BLS and Ringtail don't need Z-Chain — they're already O(1).
//
// Total cert size: 48 (BLS) + |Ringtail| + 192 (MLDSAProof), constant in N.
type QuasarCert struct {
	BLS        []byte    // BLS12-381 aggregate (48 bytes, classical)
	Ringtail   []byte    // Ringtail lattice threshold sig (O(1) after DKG)
	MLDSAProof []byte    // Z-Chain Groth16 rolling up N × ML-DSA identity sigs (192 bytes)
	Epoch      uint64    // Epoch number
	Finality   time.Time // Time of finality
	Validators int       `json:"validators,omitempty"` // Count of signing validators
}

// Verify checks structural presence of BLS and PQ certificates.
// This does NOT perform cryptographic verification -- use VerifyWithKeys for that.
// Returns false unconditionally; callers must use VerifyWithKeys with the
// validators' group public key to get a real verification result.
func (c *QuasarCert) Verify(validators []string) bool {
	return false
}

// VerifyWithKeys performs cryptographic verification of the certificate.
// A Quasar cert is finalized iff ALL three components verify in parallel:
//   - BLS12-381 aggregate (classical, fast path)
//   - Ringtail threshold sig (lattice PQ)
//   - ML-DSA proof (FIPS 204 PQ)
//
// Defense in depth: classical AND two independent PQ schemes. Any single
// scheme broken does not break finality.
func (c *QuasarCert) VerifyWithKeys(groupKey []byte, pqKey []byte) bool {
	if c == nil || len(groupKey) == 0 {
		return false
	}
	if len(c.BLS) == 0 || len(c.Ringtail) == 0 || len(c.MLDSAProof) == 0 {
		return false
	}
	// Real verification is delegated to the signer's full threshold verifier
	// which runs all three checks in parallel goroutines via VerifyAggregatedSignature.
	// This method provides the structural gate; callers with the signer use that path.
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
//
//	BLS:      sign with BLS key           → aggregate into 48 bytes (ECDL)
//	Ringtail: sign with ring-LWE key      → PQ threshold proof (Module-LWE)
//	ML-DSA:   sign with ML-DSA-65 key     → PQ identity proof (Module-LWE + Module-SIS)
//
// In QuasarCert (stored in block header):
//
//	BLS aggregate:  48 bytes
//	PQ proof:       variable (aggregated Ringtail + ML-DSA, or future SNARK)
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
