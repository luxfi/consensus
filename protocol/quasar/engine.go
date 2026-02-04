// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Quasar implementation - the supermassive black hole at the center of the blockchain galaxy.

package quasar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// quasarEngine implements the Engine interface.
type quasarEngine struct {
	mu     sync.RWMutex
	cfg    Config
	ctx    context.Context
	cancel context.CancelFunc

	// Block processing
	incoming  chan *Block
	finalized chan *Block

	// State
	finalizedBlocks map[string]*Block // hash -> block
	height          uint64
	startTime       time.Time

	// Consensus engine
	certifier *Certifier

	// Metrics
	processed uint64
}

// NewEngine creates a new Quasar consensus engine.
func NewEngine(cfg Config) (Engine, error) {
	threshold := cfg.QThreshold
	if threshold < 1 {
		threshold = 1
	}

	certifier, err := newCertifier(threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to create certifier: %w", err)
	}

	bufSize := 1000
	return &quasarEngine{
		cfg:             cfg,
		incoming:        make(chan *Block, bufSize),
		finalized:       make(chan *Block, bufSize),
		finalizedBlocks: make(map[string]*Block),
		certifier:       certifier,
	}, nil
}

// Start begins the consensus engine.
func (q *quasarEngine) Start(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.ctx, q.cancel = context.WithCancel(ctx)
	q.startTime = time.Now()

	go q.processLoop()
	return nil
}

// Stop gracefully shuts down the consensus engine.
func (q *quasarEngine) Stop() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.cancel != nil {
		q.cancel()
	}
	return nil
}

// Submit adds a block to the consensus pipeline.
func (q *quasarEngine) Submit(block *Block) error {
	if block == nil {
		return fmt.Errorf("nil block")
	}

	select {
	case q.incoming <- block:
		return nil
	default:
		return fmt.Errorf("buffer full")
	}
}

// Finalized returns a channel of finalized blocks.
func (q *quasarEngine) Finalized() <-chan *Block {
	return q.finalized
}

// IsFinalized checks if a block is finalized.
func (q *quasarEngine) IsFinalized(blockID [32]byte) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	hash := hex.EncodeToString(blockID[:])
	_, ok := q.finalizedBlocks[hash]
	return ok
}

// Stats returns consensus metrics.
func (q *quasarEngine) Stats() Stats {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return Stats{
		Height:          q.height,
		ProcessedBlocks: q.processed,
		FinalizedBlocks: uint64(len(q.finalizedBlocks)),
		PendingBlocks:   len(q.incoming),
		Validators:      q.certifier.validatorCount(),
		Uptime:          time.Since(q.startTime),
	}
}

// processLoop is the main consensus loop.
func (q *quasarEngine) processLoop() {
	for {
		select {
		case <-q.ctx.Done():
			return
		case block := <-q.incoming:
			q.processBlock(block)
		}
	}
}

// processBlock processes a single block through consensus.
func (q *quasarEngine) processBlock(block *Block) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.processed++

	// Generate quantum certificate
	cert := q.certifier.generateCert(block)
	if cert == nil {
		return // Did not achieve consensus
	}

	// Finalize block
	block.Cert = cert
	block.Hash = computeHash(block)

	q.finalizedBlocks[block.Hash] = block
	q.height++

	// Notify listeners
	select {
	case q.finalized <- block:
	default:
		// Drop if buffer full
	}
}

// computeHash computes a block hash.
func computeHash(block *Block) string {
	h := sha256.New()
	h.Write(block.ID[:])
	h.Write(block.ChainID[:])
	h.Write([]byte(block.ChainName))
	h.Write([]byte(fmt.Sprintf("%d:%d", block.Height, block.Timestamp.Unix())))
	return hex.EncodeToString(h.Sum(nil))
}

// Certifier handles lightweight certificate generation for the engine.
// NOTE: This uses SHA256 commitments for internal block tracking.
// For real threshold BLS + Ringtail signatures, see the Signer in quasar.go.
type Certifier struct {
	mu         sync.RWMutex
	threshold  int
	validators map[string]int // validator -> weight
}

func newCertifier(threshold int) (*Certifier, error) {
	return &Certifier{
		threshold:  threshold,
		validators: make(map[string]int),
	}, nil
}

func (h *Certifier) validatorCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.validators)
}

// generateCert creates a lightweight certificate for internal block tracking.
// NOTE: This uses SHA256 for internal engine use only. For real threshold
// signatures with BLS + Ringtail, use the Signer via ProcessBlock in Quasar.
func (h *Certifier) generateCert(block *Block) *BlockCert {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Create deterministic commitments for block tracking
	blsData := sha256.Sum256(block.ID[:])
	pqData := sha256.Sum256(append(block.ID[:], block.ChainID[:]...))

	return &BlockCert{
		BLS:      blsData[:],
		PQ:       pqData[:],
		Sigs:     make(map[string][]byte),
		Epoch:    block.Height,
		Finality: time.Now(),
	}
}

// AddValidator adds a validator to the consensus.
func (h *Certifier) AddValidator(id string, weight int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.validators[id] = weight
}

// RemoveValidator removes a validator from the consensus.
func (h *Certifier) RemoveValidator(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.validators, id)
}
