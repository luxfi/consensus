// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package fpc implements Fast-Path Consensus for wave certificates embedded in blocks
package fpc

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/luxfi/consensus/types"
)

// Status represents the status of a transaction in the fast path
type Status uint8

const (
	// StatusPending means the transaction is pending
	StatusPending Status = iota
	// StatusExecutable means the transaction has 2f+1 votes and can be executed (owned objects)
	StatusExecutable
	// StatusFinal means the transaction is anchored by a committed block or has 2f+1 certs in history
	StatusFinal
)

// Quorum represents the quorum configuration
type Quorum struct {
	N int // Total validators
	F int // Maximum byzantine failures tolerated
}

// Config represents the FPC configuration
type Config struct {
	Quorum            Quorum
	Epoch             uint64
	VoteLimitPerBlock int
	VotePrefix        []byte
}

// BlockRef represents a block reference with embedded FPC votes
type BlockRef struct {
	ID       types.BlockID
	Round    uint64
	Author   types.NodeID
	Final    bool     // true if consensus-committed
	EpochBit bool     // epoch fence bit
	FPCVotes [][]byte // embedded fast-path vote references
}

// Classifier determines transaction types for execution policy
type Classifier interface {
	IsOwned(tx types.TxRef) bool // Owned objects can execute at Executable status
	IsMixed(tx types.TxRef) bool // Mixed objects must wait for Final status
}

// DAGTap provides DAG ancestry queries
type DAGTap interface {
	InAncestry(block types.BlockID, tx types.TxRef) bool
}

// Engine manages fast path consensus with embedded wave certificates
type Engine interface {
	// NextVotes returns transactions to include as FPC votes in next block
	NextVotes(budget int) []types.TxRef
	// OnBlockObserved processes FPC votes from an observed block
	OnBlockObserved(ctx context.Context, b *BlockRef)
	// OnBlockAccepted anchors transactions when block is consensus-committed
	OnBlockAccepted(ctx context.Context, b *BlockRef)
	// Status returns current fast-path status of a transaction
	Status(tx types.TxRef) (Status, Proof)
	// ExecutableOwned returns owned transactions ready for execution
	ExecutableOwned() []types.TxRef
	// OnEpochCloseStart marks epoch fence
	OnEpochCloseStart()
	// OnEpochClosed clears epoch fence
	OnEpochClosed()
}

// Proof contains optional voting proof
type Proof struct {
	VoterBitmap []byte
}

// engine implements the FPC engine
type engine struct {
	mu     sync.RWMutex
	cfg    Config
	clf    Classifier
	votes  map[TxRef]map[ids.ID]bool // tx -> set of voter block IDs
	status map[TxRef]Status
	queue  []TxRef
	epoch  atomic.Uint64
}

// New creates a new FPC engine for fast-path wave certificates
func New(cfg Config, clf Classifier) Engine {
	e := &engine{
		cfg:    cfg,
		clf:    clf,
		votes:  make(map[TxRef]map[ids.ID]bool),
		status: make(map[TxRef]Status),
	}
	e.epoch.Store(cfg.Epoch)
	return e
}

// NextVotes returns the next batch of transactions to embed as FPC votes
func (e *engine) NextVotes(limit int) []TxRef {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	if limit <= 0 || limit > e.cfg.VoteLimitPerBlock {
		limit = e.cfg.VoteLimitPerBlock
	}
	
	// Take from queue head (FIFO for fairness)
	if len(e.queue) < limit {
		limit = len(e.queue)
	}
	
	out := make([]TxRef, limit)
	copy(out, e.queue[:limit])
	e.queue = e.queue[limit:]
	
	return out
}

// OnBlockObserved processes embedded FPC votes from an observed block
func (e *engine) OnBlockObserved(b BlockRef) {
	// Epoch fence: do not advance fast-path during epoch transition
	if b.EpochBit {
		return
	}
	
	e.mu.Lock()
	defer e.mu.Unlock()
	
	for _, raw := range b.FPCVotes {
		if len(raw) != 32 {
			continue // Invalid vote reference
		}
		
		var tx TxRef
		copy(tx[:], raw)
		
		// Initialize vote map if needed
		if e.votes[tx] == nil {
			e.votes[tx] = make(map[ids.ID]bool)
		}
		
		// Record vote from this block
		e.votes[tx][b.ID] = true
		
		// Check if we reached quorum (2f+1) for wave certificate
		if len(e.votes[tx]) >= 2*e.cfg.Quorum.F+1 {
			// Upgrade status if this is an owned object
			if e.status[tx] < StatusExecutable && e.clf.IsOwned(tx) {
				e.status[tx] = StatusExecutable
			}
		}
	}
}

// OnBlockAccepted anchors transactions to final status when block commits
func (e *engine) OnBlockAccepted(b BlockRef) {
	// Only process if block is consensus-committed
	if !b.Final {
		return
	}
	
	e.mu.Lock()
	defer e.mu.Unlock()
	
	for _, raw := range b.FPCVotes {
		if len(raw) != 32 {
			continue
		}
		
		var tx TxRef
		copy(tx[:], raw)
		
		// If transaction has votes and is executable, upgrade to final
		// This anchors the fast-path decision in the committed chain
		if e.status[tx] >= StatusExecutable {
			e.status[tx] = StatusFinal
		}
	}
}

// Status returns the current fast-path status of a transaction
func (e *engine) Status(tx TxRef) Status {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.status[tx]
}

// ExecutableOwned returns all owned transactions that reached Executable status
func (e *engine) ExecutableOwned() []TxRef {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	var out []TxRef
	for tx, st := range e.status {
		if st == StatusExecutable && e.clf.IsOwned(tx) {
			out = append(out, tx)
		}
	}
	return out
}

// Enqueue adds a transaction to the FPC voting queue
func (e *engine) Enqueue(tx TxRef) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	// Check if already in queue
	for _, existing := range e.queue {
		if existing == tx {
			return
		}
	}
	
	// Add to queue and initialize status
	e.queue = append(e.queue, tx)
	if _, exists := e.status[tx]; !exists {
		e.status[tx] = StatusPending
	}
}

// SimpleClassifier provides a basic transaction classifier
// In production, this would analyze transaction inputs/outputs
type SimpleClassifier struct{}

// IsOwned returns true if transaction operates on owned objects
func (s SimpleClassifier) IsOwned(tx TxRef) bool {
	// Simple heuristic: first byte determines ownership
	// In production, this would check actual object ownership
	return tx[0]&0x01 == 0
}

// IsMixed returns true if transaction operates on mixed/shared objects
func (s SimpleClassifier) IsMixed(tx TxRef) bool {
	return !s.IsOwned(tx)
}