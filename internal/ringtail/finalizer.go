// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package ringtail

import (
	"sync"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// PQFinalizer implements parallel Ringtail post-quantum finality
// This runs asynchronously after metastable consensus achieves finality
type PQFinalizer struct {
	mu sync.RWMutex
	
	// Configuration
	config      RingtailConfig
	ringtail    ThresholdKey
	validators  []ids.NodeID
	
	// State
	certCache   map[uint64][]byte      // height -> quantum certificate
	pending     map[uint64]pendingCert // blocks awaiting quantum finality
	shareCache  map[uint64]map[ids.NodeID][]byte // height -> validator -> share
	
	// Scheduling
	timer       *time.Timer
	lastCertHeight uint64
	
	logger      log.Logger
}

type pendingCert struct {
	height    uint64
	blockHash ids.ID
	startTime time.Time
	shares    map[ids.NodeID][]byte
}

// NewPQFinalizer creates a new post-quantum finalizer
func NewPQFinalizer(config RingtailConfig, ringtail ThresholdKey, validators []ids.NodeID) *PQFinalizer {
	return &PQFinalizer{
		config:      config,
		ringtail:    ringtail,
		validators:  validators,
		certCache:   make(map[uint64][]byte),
		pending:     make(map[uint64]pendingCert),
		shareCache:  make(map[uint64]map[ids.NodeID][]byte),
		logger:      log.NewLogger("ringtail"),
	}
}

// OnMetastableFinality is called when a block achieves fast BLS finality
// This triggers the parallel quantum finality process
func (pq *PQFinalizer) OnMetastableFinality(height uint64, blockHash ids.ID) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	// Check if we should create a quantum cert for this height
	if !pq.shouldCertify(height) {
		return
	}
	
	// Start the quantum finality process in parallel
	pq.pending[height] = pendingCert{
		height:    height,
		blockHash: blockHash,
		startTime: time.Now(),
		shares:    make(map[ids.NodeID][]byte),
	}
	
	// Launch parallel Ringtail signing
	go pq.collectQuantumSignatures(height, blockHash)
}

// shouldCertify determines if this height needs a quantum certificate
func (pq *PQFinalizer) shouldCertify(height uint64) bool {
	// Certificate every Q seconds worth of blocks
	if pq.config.MergeBlocks > 1 {
		return height%uint64(pq.config.MergeBlocks) == 0
	}
	
	// Or based on time elapsed since last cert
	if pq.lastCertHeight == 0 {
		return true
	}
	
	// Simple Q-second interval
	return height >= pq.lastCertHeight + uint64(pq.config.Q.Seconds())
}

// collectQuantumSignatures gathers threshold signatures from validators
func (pq *PQFinalizer) collectQuantumSignatures(height uint64, blockHash ids.ID) {
	// Optional delay to ensure metastable finality is truly irreversible
	if pq.config.DelayAfterBLS > 0 {
		time.Sleep(pq.config.DelayAfterBLS)
	}
	
	// Create message to sign (block hash or Merkle root of block range)
	message := pq.createQuantumMessage(height, blockHash)
	
	// Sign with our share
	share, err := pq.ringtail.Sign(message)
	if err != nil {
		pq.logger.Error("failed to create signature share", "error", err)
		return
	}
	
	pq.mu.Lock()
	if pending, ok := pq.pending[height]; ok {
		pending.shares[pq.validators[0]] = share // Assume we're first validator for now
		pq.pending[height] = pending
	}
	pq.mu.Unlock()
	
	// In real implementation, gossip our share to other validators
	pq.gossipShare(height, share)
	
	// Wait for threshold shares
	pq.waitForThreshold(height)
}

// waitForThreshold waits for enough shares to create the quantum certificate
func (pq *PQFinalizer) waitForThreshold(height uint64) {
	threshold, _ := pq.ringtail.GetThreshold()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	timeout := time.After(30 * time.Second) // Reasonable timeout
	
	for {
		select {
		case <-ticker.C:
			pq.mu.RLock()
			pending, ok := pq.pending[height]
			shareCount := len(pending.shares)
			pq.mu.RUnlock()
			
			if !ok {
				return // Cert was already created or cancelled
			}
			
			if shareCount >= threshold {
				// We have enough shares, create the certificate
				pq.createQuantumCertificate(height)
				return
			}
			
		case <-timeout:
			pq.logger.Warn("timeout waiting for quantum shares", "height", height)
			pq.mu.Lock()
			delete(pq.pending, height)
			pq.mu.Unlock()
			return
		}
	}
}

// createQuantumCertificate aggregates shares into the final certificate
func (pq *PQFinalizer) createQuantumCertificate(height uint64) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	pending, ok := pq.pending[height]
	if !ok {
		return
	}
	
	// Collect shares in deterministic order
	shares := make([][]byte, 0, len(pending.shares))
	for _, validator := range pq.validators {
		if share, ok := pending.shares[validator]; ok {
			shares = append(shares, share)
		}
		if len(shares) >= pq.config.Threshold {
			break
		}
	}
	
	// Aggregate into quantum certificate
	cert, err := pq.ringtail.Aggregate(shares)
	if err != nil {
		pq.logger.Error("failed to aggregate quantum certificate", "error", err)
		return
	}
	
	// Store certificate
	pq.certCache[height] = cert
	pq.lastCertHeight = height
	delete(pq.pending, height)
	
	// Log achievement of quantum finality
	elapsed := time.Since(pending.startTime)
	pq.logger.Info("Quantum finality achieved",
		"height", height,
		"elapsed", elapsed,
		"certSize", len(cert),
	)
	
	// Gossip the certificate
	pq.gossipCertificate(height, cert)
}

// createQuantumMessage creates the message to be signed for quantum finality
func (pq *PQFinalizer) createQuantumMessage(height uint64, blockHash ids.ID) []byte {
	// For merged blocks, create Merkle root of block range
	if pq.config.MergeBlocks > 1 {
		// In real implementation, compute Merkle root of blocks
		// For now, just use the latest block hash
	}
	
	// Format: HEIGHT || BLOCK_HASH
	message := make([]byte, 8+32)
	// Write height as big-endian
	for i := 0; i < 8; i++ {
		message[i] = byte(height >> (56 - 8*i))
	}
	copy(message[8:], blockHash[:])
	
	return message
}

// GetQuantumCert returns the quantum certificate for a height
func (pq *PQFinalizer) GetQuantumCert(height uint64) ([]byte, bool) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	
	cert, ok := pq.certCache[height]
	return cert, ok
}

// IsQuantumFinal checks if a block has achieved quantum finality
func (pq *PQFinalizer) IsQuantumFinal(height uint64) bool {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	
	_, ok := pq.certCache[height]
	return ok
}

// SetQuantumInterval updates the Q parameter
func (pq *PQFinalizer) SetQuantumInterval(q time.Duration) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	pq.config.Q = q
}

// RecordShare records a signature share from another validator
func (pq *PQFinalizer) RecordShare(height uint64, validator ids.NodeID, share []byte) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	if pending, ok := pq.pending[height]; ok {
		pending.shares[validator] = share
		pq.pending[height] = pending
	}
}

// gossipShare broadcasts our signature share to other validators
func (pq *PQFinalizer) gossipShare(height uint64, share []byte) {
	// In real implementation, use P2P gossip
	// This is where ZMQ transport would be used for high-performance sharing
}

// gossipCertificate broadcasts the completed quantum certificate
func (pq *PQFinalizer) gossipCertificate(height uint64, cert []byte) {
	// Broadcast the certificate so all nodes can store it
	// Certificate is only 2-3KB, so network overhead is minimal
}