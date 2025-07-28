// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package ringtail implements post-quantum threshold signatures for
// Lux Quantum Consensus - providing quantum-resistant finality on top
// of the fast metastable BLS consensus.
package ringtail

import (
	"time"

	"github.com/luxfi/ids"
)

// ThresholdKey represents a t-of-n threshold signing key
type ThresholdKey interface {
	// Sign creates a signature share for the given message
	Sign(message []byte) ([]byte, error)
	
	// Aggregate combines signature shares into a final signature
	Aggregate(shares [][]byte) ([]byte, error)
	
	// Verify checks if a signature is valid
	Verify(message []byte, signature []byte) bool
	
	// GetThreshold returns the threshold parameters (t, n)
	GetThreshold() (t, n int)
}

// QuantumFinalizer provides post-quantum finality for blocks
type QuantumFinalizer interface {
	// OnMetastableFinality is called when a block achieves BLS finality
	OnMetastableFinality(height uint64, blockHash ids.ID)
	
	// GetQuantumCert returns the quantum certificate for a height
	GetQuantumCert(height uint64) ([]byte, bool)
	
	// IsQuantumFinal checks if a block has quantum finality
	IsQuantumFinal(height uint64) bool
	
	// SetQuantumInterval sets the Q parameter (seconds between quantum certs)
	SetQuantumInterval(q time.Duration)
}

// RingtailConfig contains configuration for the Ringtail post-quantum layer
type RingtailConfig struct {
	// Q is the quantum finality interval in seconds
	Q time.Duration
	
	// Threshold is the number of validators needed for a quantum cert (2f+1)
	Threshold int
	
	// ValidatorCount is the total number of validators
	ValidatorCount int
	
	// MergeBlocks determines how many blocks to include in one quantum cert
	MergeBlocks int
	
	// DelayAfterBLS adds extra delay after BLS finality before quantum cert
	DelayAfterBLS time.Duration
	
	// PrecomputedRounds is the number of pre-computed offline rounds
	PrecomputedRounds int
}