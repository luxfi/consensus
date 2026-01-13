// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package field

import (
	"context"
	"sync"
	"time"
)

// Service coordinates epoch transitions and cross-chain checkpoint bundling.
// In DAG consensus, epochs demarcate finality boundaries where all vertices
// within an epoch are guaranteed to be ordered before the next epoch starts.
type Service struct {
	mu           sync.RWMutex
	currentEpoch uint64
	checkpoints  map[uint64]Checkpoint
	bundleSize   int
}

// Checkpoint represents a finality checkpoint at an epoch boundary.
type Checkpoint struct {
	Epoch     uint64
	Timestamp time.Time
	Root      [32]byte // Merkle root of finalized vertices
	Signature []byte   // Aggregate signature from validators
}

// New creates a new Nebula service for epoch/checkpoint coordination.
func New() *Service {
	return &Service{
		currentEpoch: 0,
		checkpoints:  make(map[uint64]Checkpoint),
		bundleSize:   100, // Default bundle size
	}
}

// NewWithConfig creates a Nebula service with custom configuration.
func NewWithConfig(bundleSize int) *Service {
	return &Service{
		currentEpoch: 0,
		checkpoints:  make(map[uint64]Checkpoint),
		bundleSize:   bundleSize,
	}
}

// CurrentEpoch returns the current epoch number.
func (s *Service) CurrentEpoch() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentEpoch
}

// AdvanceEpoch moves to the next epoch after finality is achieved.
func (s *Service) AdvanceEpoch(ctx context.Context, root [32]byte, signature []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.currentEpoch++
	s.checkpoints[s.currentEpoch] = Checkpoint{
		Epoch:     s.currentEpoch,
		Timestamp: time.Now(),
		Root:      root,
		Signature: signature,
	}
	return nil
}

// GetCheckpoint retrieves a checkpoint by epoch number.
func (s *Service) GetCheckpoint(epoch uint64) (Checkpoint, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp, exists := s.checkpoints[epoch]
	return cp, exists
}

// LatestCheckpoint returns the most recent checkpoint.
func (s *Service) LatestCheckpoint() (Checkpoint, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.currentEpoch == 0 {
		return Checkpoint{}, false
	}
	cp, exists := s.checkpoints[s.currentEpoch]
	return cp, exists
}

// BundleSize returns the configured bundle size for checkpointing.
func (s *Service) BundleSize() int {
	return s.bundleSize
}
