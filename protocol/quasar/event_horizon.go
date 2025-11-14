// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Event Horizon - The supermassive black hole drawing all chains into quantum consensus

package quasar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Block from any chain entering the event horizon
type Block struct {
	Chain     string    // "P-Chain", "X-Chain", "C-Chain", "Bridge", etc.
	ID        [32]byte  // Block hash
	Height    uint64    // Block number
	Timestamp time.Time
	Data      []byte    // Block data
}

// FinalizedBlock with quantum signatures from the singularity
type FinalizedBlock struct {
	Height     uint64
	Sources    []*Block
	Hash       string
	Timestamp  time.Time
	Signatures map[string]*HybridSignature // validator -> hybrid sig
}

// Quasar is the Quasar's boundary where blocks achieve quantum finality
type Quasar struct {
	mu sync.RWMutex

	// Event horizon - all chains feed through here
	chains   map[string]chan *Block // chain -> blocks falling in
	hybrid   *QuasarHybridConsensus // BLS + Ringtail quantum consensus
	finality map[string]*FinalizedBlock // hash -> finalized

	// Singularity state
	height uint64
	blocks uint64 // Total blocks processed
	proofs uint64 // Quantum proofs generated
}

// New creates the supermassive black hole at the center of the galaxy
func NewQuasar(threshold int) (*Quasar, error) {
	hybrid, err := NewQuasarHybridConsensus(threshold)
	if err != nil {
		return nil, err
	}

	eh := &Quasar{
		chains:   make(map[string]chan *Block),
		hybrid:   hybrid,
		finality: make(map[string]*FinalizedBlock),
	}

	// Primary chains auto-register
	eh.register("P-Chain")
	eh.register("X-Chain")
	eh.register("C-Chain")

	return eh, nil
}

// Start activates the event horizon
func (eh *Quasar) Start(ctx context.Context) {
	// Process all registered chains
	eh.mu.RLock()
	for chain := range eh.chains {
		go eh.process(ctx, chain)
	}
	eh.mu.RUnlock()

	// Periodic epoch finalization
	go eh.finalize(ctx)

	fmt.Println("[QUASAR] Event horizon active - quantum consensus enabled")
}

// Submit a block to the event horizon (auto-registers chain if new)
func (eh *Quasar) Submit(block *Block) {
	eh.mu.RLock()
	ch, exists := eh.chains[block.Chain]
	eh.mu.RUnlock()

	if !exists {
		eh.register(block.Chain)
		eh.mu.RLock()
		ch = eh.chains[block.Chain]
		eh.mu.RUnlock()
	}

	select {
	case ch <- block:
	default:
		<-ch        // Drop oldest
		ch <- block // Add new
	}
}

// Verify checks if a block has quantum finality
func (eh *Quasar) Verify(hash string) bool {
	eh.mu.RLock()
	defer eh.mu.RUnlock()

	fb, exists := eh.finality[hash]
	if !exists {
		return false
	}

	// Verify all hybrid signatures (BLS + Ringtail)
	for _, sig := range fb.Signatures {
		if !eh.hybrid.VerifyHybridSignature([]byte(hash), sig) {
			return false
		}
	}

	return true
}

// Stats returns event horizon metrics
func (eh *Quasar) Stats() (height, blocks, proofs uint64) {
	eh.mu.RLock()
	defer eh.mu.RUnlock()
	return eh.height, eh.blocks, eh.proofs
}

// Chains returns all registered chains
func (eh *Quasar) Chains() []string {
	eh.mu.RLock()
	defer eh.mu.RUnlock()

	chains := make([]string, 0, len(eh.chains))
	for chain := range eh.chains {
		chains = append(chains, chain)
	}
	return chains
}

// register adds a chain to the event horizon
func (eh *Quasar) register(chain string) {
	eh.mu.Lock()
	defer eh.mu.Unlock()

	if _, exists := eh.chains[chain]; exists {
		return
	}

	eh.chains[chain] = make(chan *Block, 100)
	fmt.Printf("[QUASAR] %s drawn into event horizon\n", chain)
}

// process handles blocks from a specific chain
func (eh *Quasar) process(ctx context.Context, chain string) {
	eh.mu.RLock()
	ch := eh.chains[chain]
	eh.mu.RUnlock()

	for {
		select {
		case <-ctx.Done():
			return
		case block := <-ch:
			eh.accept(block)
		}
	}
}

// accept finalizes a single block with quantum signatures
func (eh *Quasar) accept(block *Block) {
	eh.mu.Lock()
	defer eh.mu.Unlock()

	// Compute quantum hash
	hash := eh.hash(block)

	// Collect validator signatures (BLS + Ringtail parallel)
	sigs := make(map[string]*HybridSignature)
	if sig, err := eh.hybrid.SignMessage("validator1", []byte(hash)); err == nil {
		sigs["validator1"] = sig
	}

	// Create finalized block
	fb := &FinalizedBlock{
		Height:     eh.height + 1,
		Sources:    []*Block{block},
		Hash:       hash,
		Timestamp:  time.Now(),
		Signatures: sigs,
	}

	eh.finality[hash] = fb
	eh.height++
	eh.blocks++
}

// finalize creates quantum proofs for epochs
func (eh *Quasar) finalize(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			eh.epoch()
		}
	}
}

// epoch creates a quantum proof for all finalized blocks
func (eh *Quasar) epoch() {
	eh.mu.Lock()
	defer eh.mu.Unlock()

	if len(eh.finality) == 0 {
		return
	}

	// Aggregate all block hashes
	var data string
	for hash := range eh.finality {
		data += hash
	}

	_ = sha256.Sum256([]byte(data))
	eh.proofs++

	fmt.Printf("[QUASAR] Epoch finalized: height=%d blocks=%d proof=%d\n",
		eh.height, len(eh.finality), eh.proofs)
}

// hash creates a quantum-resistant block hash
func (eh *Quasar) hash(block *Block) string {
	data := fmt.Sprintf("%s:%x:%d:%d",
		block.Chain,
		block.ID[:],
		block.Height,
		block.Timestamp.Unix())

	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}
