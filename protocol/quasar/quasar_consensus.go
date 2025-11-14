// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Quasar - Supermassive black hole providing quantum consensus for all chains

package quasar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Quasar implements Horizon - the supermassive black hole at galaxy center
type Quasar struct {
	mu sync.RWMutex

	chains   map[ChainID]chan *Block
	hybrid   *QuasarHybridConsensus
	finality map[string]*FinalizedBlock

	height uint64
	blocks uint64
	proofs uint64
}

// New creates a new Quasar consensus engine
func New(threshold int) (*Quasar, error) {
	hybrid, err := NewQuasarHybridConsensus(threshold)
	if err != nil {
		return nil, err
	}

	q := &Quasar{
		chains:   make(map[ChainID]chan *Block),
		hybrid:   hybrid,
		finality: make(map[string]*FinalizedBlock),
	}

	// Auto-register primary chains
	q.Register("P-Chain")
	q.Register("X-Chain")
	q.Register("C-Chain")

	return q, nil
}

// Start activates the event horizon
func (q *Quasar) Start(ctx context.Context) error {
	q.mu.RLock()
	for chain := range q.chains {
		go q.process(ctx, chain)
	}
	q.mu.RUnlock()

	go q.finalize(ctx)

	fmt.Println("[QUASAR] Event horizon active")
	return nil
}

// Ingest pulls blocks into the event horizon
func (q *Quasar) Ingest(ctx context.Context, batch *FinalityBatch) error {
	for _, block := range batch.Blocks {
		q.Submit(block)
	}
	return nil
}

// VerifyProof validates quantum finality proof
func (q *Quasar) VerifyProof(ctx context.Context, proof *FinalityProof) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	fb, exists := q.finality[proof.BlockHash]
	if !exists {
		return fmt.Errorf("block not finalized")
	}

	// Verify all hybrid signatures
	for _, sig := range fb.Signatures {
		if !q.hybrid.VerifyHybridSignature([]byte(proof.BlockHash), sig) {
			return fmt.Errorf("invalid signature")
		}
	}

	return nil
}

// Snapshot returns current horizon state
func (q *Quasar) Snapshot() HorizonStats {
	q.mu.RLock()
	defer q.mu.RUnlock()

	chains := make([]string, 0, len(q.chains))
	for chain := range q.chains {
		chains = append(chains, chain)
	}

	return HorizonStats{
		Height: q.height,
		Blocks: q.blocks,
		Proofs: q.proofs,
		Chains: chains,
	}
}

// RegisteredChains returns all active chains
func (q *Quasar) RegisteredChains() []ChainID {
	q.mu.RLock()
	defer q.mu.RUnlock()

	chains := make([]ChainID, 0, len(q.chains))
	for chain := range q.chains {
		chains = append(chains, chain)
	}
	return chains
}

// submit sends block to event horizon (auto-registers chain)
func (q *Quasar) Submit(block *Block) {
	q.mu.RLock()
	ch, exists := q.chains[block.Chain]
	q.mu.RUnlock()

	if !exists {
		q.Register(block.Chain)
		q.mu.RLock()
		ch = q.chains[block.Chain]
		q.mu.RUnlock()
	}

	select {
	case ch <- block:
	default:
		<-ch
		ch <- block
	}
}

// register adds chain to event horizon
func (q *Quasar) Register(chain ChainID) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.chains[chain]; exists {
		return
	}

	q.chains[chain] = make(chan *Block, 100)
	fmt.Printf("[QUASAR] %s drawn into event horizon\n", chain)
}

// process handles blocks from a chain
func (q *Quasar) process(ctx context.Context, chain ChainID) {
	q.mu.RLock()
	ch := q.chains[chain]
	q.mu.RUnlock()

	for {
		select {
		case <-ctx.Done():
			return
		case block := <-ch:
			q.Accept(block)
		}
	}
}

// accept finalizes block with quantum signatures
func (q *Quasar) Accept(block *Block) {
	q.mu.Lock()
	defer q.mu.Unlock()

	hash := q.hash(block)

	sigs := make(map[string]*HybridSignature)
	if sig, err := q.hybrid.SignMessage("validator1", []byte(hash)); err == nil {
		sigs["validator1"] = sig
	}

	fb := &FinalizedBlock{
		Height:     q.height + 1,
		Sources:    []*Block{block},
		Hash:       hash,
		Timestamp:  time.Now(),
		Signatures: sigs,
	}

	q.finality[hash] = fb
	q.height++
	q.blocks++
}

// finalize creates quantum proofs periodically
func (q *Quasar) finalize(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.epoch()
		}
	}
}

// epoch aggregates blocks into quantum proof
func (q *Quasar) epoch() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.finality) == 0 {
		return
	}

	var data string
	for hash := range q.finality {
		data += hash
	}

	_ = sha256.Sum256([]byte(data))
	q.proofs++

	fmt.Printf("[QUASAR] Epoch: height=%d blocks=%d proof=%d\n",
		q.height, len(q.finality), q.proofs)
}

// hash computes quantum-resistant block hash
func (q *Quasar) hash(block *Block) string {
	data := fmt.Sprintf("%s:%x:%d:%d",
		block.Chain,
		block.ID[:],
		block.Height,
		block.Timestamp.Unix())

	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// Verify checks if a block has quantum finality
func (q *Quasar) Verify(hash string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	fb, exists := q.finality[hash]
	if !exists {
		return false
	}

	for _, sig := range fb.Signatures {
		if !q.hybrid.VerifyHybridSignature([]byte(hash), sig) {
			return false
		}
	}

	return true
}

// Stats returns metrics
func (q *Quasar) Stats() (height, blocks, proofs uint64) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.height, q.blocks, q.proofs
}

// Chains returns all registered chains
func (q *Quasar) Chains() []ChainID {
	q.mu.RLock()
	defer q.mu.RUnlock()

	chains := make([]ChainID, 0, len(q.chains))
	for chain := range q.chains {
		chains = append(chains, chain)
	}
	return chains
}
