// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package witness implements Verkle tree witnesses for consensus state proofs.
// This module provides hooks for generating and verifying Verkle witnesses
// that prove consensus state transitions without requiring full state.
package witness

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/luxfi/ids"
	"github.com/luxfi/go-verkle"
)

// Provider manages Verkle witnesses for consensus operations
type Provider struct {
	mu       sync.RWMutex
	tree     verkle.VerkleNode
	cache    map[string]*Witness
	maxCache int
	
	// Hooks for consensus integration
	preStateHook  func(context.Context, ids.ID) error
	postStateHook func(context.Context, ids.ID, *Witness) error
}

// Witness contains Verkle proof data
type Witness struct {
	// State root before transition
	PreRoot []byte
	
	// State root after transition
	PostRoot []byte
	
	// Verkle proof for state transition
	Proof *verkle.Proof
	
	// Keys accessed during transition
	AccessedKeys [][]byte
	
	// Values at accessed keys
	AccessedValues map[string][]byte
	
	// Metadata
	Height    uint64
	Timestamp int64
	GasUsed   uint64
}

// Config contains witness provider configuration
type Config struct {
	// Maximum cached witnesses
	MaxCache int
	
	// Enable compression
	Compress bool
	
	// Verkle tree depth
	TreeDepth int
	
	// Key length in bytes
	KeyLength int
}

// DefaultConfig returns default witness configuration
func DefaultConfig() Config {
	return Config{
		MaxCache:  1000,
		Compress:  true,
		TreeDepth: 256,
		KeyLength: 32,
	}
}

// NewProvider creates a new witness provider
func NewProvider(cfg Config) *Provider {
	return &Provider{
		tree:     verkle.New(),
		cache:    make(map[string]*Witness),
		maxCache: cfg.MaxCache,
	}
}

// GenerateWitness creates a Verkle witness for a state transition
func (p *Provider) GenerateWitness(ctx context.Context, keys [][]byte, preState, postState map[string][]byte) (*Witness, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Create witness structure
	w := &Witness{
		AccessedKeys:   keys,
		AccessedValues: make(map[string][]byte),
	}
	
	// Build pre-state tree and get root
	preTree := verkle.New()
	for k, v := range preState {
		key, _ := hex.DecodeString(k)
		if err := preTree.Insert(key, v, nil); err != nil {
			return nil, fmt.Errorf("failed to insert pre-state: %w", err)
		}
		w.AccessedValues[k] = v
	}
	w.PreRoot = preTree.Commit().Bytes()
	
	// Build post-state tree and get root
	postTree := verkle.New()
	for k, v := range postState {
		key, _ := hex.DecodeString(k)
		if err := postTree.Insert(key, v, nil); err != nil {
			return nil, fmt.Errorf("failed to insert post-state: %w", err)
		}
	}
	w.PostRoot = postTree.Commit().Bytes()
	
	// Generate Verkle proof
	proof, err := verkle.MakeVerkleMultiProof(preTree, postTree, keys, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate verkle proof: %w", err)
	}
	w.Proof = proof
	
	// Cache witness
	cacheKey := hex.EncodeToString(w.PostRoot)
	p.cache[cacheKey] = w
	
	// Evict old entries if cache is full
	if len(p.cache) > p.maxCache {
		p.evictOldest()
	}
	
	return w, nil
}

// VerifyWitness verifies a Verkle witness
func (p *Provider) VerifyWitness(ctx context.Context, w *Witness) error {
	if w == nil || w.Proof == nil {
		return fmt.Errorf("invalid witness")
	}
	
	// Verify the Verkle proof
	pe, err := verkle.PreStateTreeFromProof(w.Proof, w.PreRoot)
	if err != nil {
		return fmt.Errorf("failed to reconstruct pre-state: %w", err)
	}
	
	// Verify accessed values match
	for k, expectedValue := range w.AccessedValues {
		key, _ := hex.DecodeString(k)
		actualValue, err := pe.Get(key, nil)
		if err != nil {
			return fmt.Errorf("failed to get value for key %s: %w", k, err)
		}
		if string(actualValue) != string(expectedValue) {
			return fmt.Errorf("value mismatch for key %s", k)
		}
	}
	
	return nil
}

// GetCachedWitness retrieves a cached witness by root hash
func (p *Provider) GetCachedWitness(root []byte) (*Witness, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	key := hex.EncodeToString(root)
	w, ok := p.cache[key]
	return w, ok
}

// SetPreStateHook sets the pre-state transition hook
func (p *Provider) SetPreStateHook(hook func(context.Context, ids.ID) error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.preStateHook = hook
}

// SetPostStateHook sets the post-state transition hook
func (p *Provider) SetPostStateHook(hook func(context.Context, ids.ID, *Witness) error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.postStateHook = hook
}

// evictOldest removes the oldest cached witness
func (p *Provider) evictOldest() {
	// Simple eviction - just remove first found
	// TODO: Implement LRU or other eviction policy
	for k := range p.cache {
		delete(p.cache, k)
		break
	}
}

// CompressWitness compresses a witness for network transmission
func CompressWitness(w *Witness) ([]byte, error) {
	// TODO: Implement compression
	// This would use snappy or zstd compression
	return nil, fmt.Errorf("compression not yet implemented")
}

// DecompressWitness decompresses a witness received from network
func DecompressWitness(data []byte) (*Witness, error) {
	// TODO: Implement decompression
	return nil, fmt.Errorf("decompression not yet implemented")
}

// Hook integrations for consensus engine

// BeforeTransition is called before a consensus state transition
func (p *Provider) BeforeTransition(ctx context.Context, txID ids.ID) error {
	if p.preStateHook != nil {
		return p.preStateHook(ctx, txID)
	}
	return nil
}

// AfterTransition is called after a consensus state transition
func (p *Provider) AfterTransition(ctx context.Context, txID ids.ID, w *Witness) error {
	if p.postStateHook != nil {
		return p.postStateHook(ctx, txID, w)
	}
	return nil
}

// Stats returns witness provider statistics
type Stats struct {
	CachedWitnesses int
	TreeDepth       int
	TotalProofs     uint64
	VerifiedProofs  uint64
	FailedProofs    uint64
}

// GetStats returns current statistics
func (p *Provider) GetStats() Stats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	return Stats{
		CachedWitnesses: len(p.cache),
		TreeDepth:       256, // Fixed for now
		// TODO: Track proof statistics
	}
}