// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package ringtail

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultPrecomputeTarget is the default number of shares to precompute.
	DefaultPrecomputeTarget = 50

	// MinPrecomputeShares is the minimum shares to maintain.
	MinPrecomputeShares = 20
)

// PrecomputedShare represents a precomputed Ringtail share.
type PrecomputedShare struct {
	Index     uint32
	Data      []byte
	Timestamp time.Time
}

// Precomputer manages precomputation of Ringtail shares.
// It maintains a pool of ready-to-use shares to hide lattice computation latency.
type Precomputer struct {
	mu sync.RWMutex

	// Configuration
	keyPair *KeyPair
	workers int
	target  int

	// Share pool
	shares chan *PrecomputedShare
	
	// State
	running atomic.Bool
	wg      sync.WaitGroup
	
	// Metrics
	generated atomic.Uint64
	consumed  atomic.Uint64
}

// NewPrecomputer creates a new precomputer.
func NewPrecomputer(keyPair *KeyPair, workers int) *Precomputer {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	return &Precomputer{
		keyPair: keyPair,
		workers: workers,
		target:  DefaultPrecomputeTarget,
		shares:  make(chan *PrecomputedShare, DefaultPrecomputeTarget),
	}
}

// Start starts the precomputation workers.
func (p *Precomputer) Start(ctx context.Context) {
	if !p.running.CompareAndSwap(false, true) {
		return
	}

	// Start workers
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}

	// Start monitor
	p.wg.Add(1)
	go p.monitor(ctx)
}

// Stop stops all precomputation.
func (p *Precomputer) Stop() {
	if !p.running.CompareAndSwap(true, false) {
		return
	}

	p.wg.Wait()
	close(p.shares)
}

// GetShare retrieves a precomputed share.
func (p *Precomputer) GetShare() *PrecomputedShare {
	select {
	case share := <-p.shares:
		p.consumed.Add(1)
		return share
	default:
		// No shares available
		return nil
	}
}

// Available returns the number of available precomputed shares.
func (p *Precomputer) Available() int {
	return len(p.shares)
}

// SetTarget sets the target number of shares to maintain.
func (p *Precomputer) SetTarget(target int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if target < MinPrecomputeShares {
		target = MinPrecomputeShares
	}
	p.target = target
}

// worker is a precomputation worker goroutine.
func (p *Precomputer) worker(ctx context.Context, id int) {
	defer p.wg.Done()

	// Pin to CPU for better cache locality
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for {
		// Check if we should stop
		if !p.running.Load() {
			return
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		// Check if we need more shares
		if len(p.shares) >= p.target {
			// Sleep to avoid busy waiting
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Generate a share
		share := p.generateShare(id)
		if share == nil {
			continue
		}

		// Try to add to pool
		select {
		case p.shares <- share:
			p.generated.Add(1)
		case <-ctx.Done():
			return
		default:
			// Pool is full, discard
		}
	}
}

// generateShare generates a single precomputed share.
func (p *Precomputer) generateShare(workerID int) *PrecomputedShare {
	// Use actual lattice computation from luxfi/ringtail
	// This is where the heavy cryptographic work happens
	
	// Create a partial signature that can be bound to a message later
	scheme := NewScheme()
	
	// Unmarshal private key
	sk := scheme.NewPrivateKey()
	if err := sk.UnmarshalBinary(p.keyPair.PrivateKey); err != nil {
		return nil
	}
	
	// Generate precomputed lattice data
	// This creates the expensive part of the signature that doesn't depend on the message
	precomp, err := scheme.Precompute(sk)
	if err != nil {
		return nil
	}
	
	// Serialize precomputed data
	precompBytes, err := precomp.MarshalBinary()
	if err != nil {
		return nil
	}
	
	return &PrecomputedShare{
		Index:     uint32(workerID),
		Data:      precompBytes,
		Timestamp: time.Now(),
	}
}

// monitor monitors the share pool and adjusts workers if needed.
func (p *Precomputer) monitor(ctx context.Context) {
	defer p.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.checkHealth()
		}
	}
}

// checkHealth checks the health of the precompute pool.
func (p *Precomputer) checkHealth() {
	available := p.Available()
	generated := p.generated.Load()
	consumed := p.consumed.Load()

	// Calculate generation rate
	rate := float64(generated) / time.Since(time.Now()).Seconds()
	
	// Log metrics
	_ = map[string]interface{}{
		"available":       available,
		"generated_total": generated,
		"consumed_total":  consumed,
		"generation_rate": rate,
		"workers":         p.workers,
	}

	// TODO: Adjust workers dynamically based on consumption rate
}

// Stats returns precomputer statistics.
type Stats struct {
	Available      int
	Generated      uint64
	Consumed       uint64
	Workers        int
	Target         int
	GenerationRate float64
}

// GetStats returns current statistics.
func (p *Precomputer) GetStats() Stats {
	return Stats{
		Available: p.Available(),
		Generated: p.generated.Load(),
		Consumed:  p.consumed.Load(),
		Workers:   p.workers,
		Target:    p.target,
	}
}

// PrecomputeManager manages precomputation across multiple validators.
type PrecomputeManager struct {
	mu          sync.RWMutex
	precomputers map[string]*Precomputer
}

// NewPrecomputeManager creates a new precompute manager.
func NewPrecomputeManager() *PrecomputeManager {
	return &PrecomputeManager{
		precomputers: make(map[string]*Precomputer),
	}
}

// AddValidator adds a validator's precomputer.
func (pm *PrecomputeManager) AddValidator(validatorID string, keyPair *KeyPair, workers int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pc := NewPrecomputer(keyPair, workers)
	pm.precomputers[validatorID] = pc
}

// GetPrecomputer gets a validator's precomputer.
func (pm *PrecomputeManager) GetPrecomputer(validatorID string) (*Precomputer, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	pc, ok := pm.precomputers[validatorID]
	return pc, ok
}

// StartAll starts all precomputers.
func (pm *PrecomputeManager) StartAll(ctx context.Context) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, pc := range pm.precomputers {
		pc.Start(ctx)
	}
}

// StopAll stops all precomputers.
func (pm *PrecomputeManager) StopAll() {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, pc := range pm.precomputers {
		pc.Stop()
	}
}

// GetAllStats returns statistics for all precomputers.
func (pm *PrecomputeManager) GetAllStats() map[string]Stats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	stats := make(map[string]Stats)
	for id, pc := range pm.precomputers {
		stats[id] = pc.GetStats()
	}
	return stats
}