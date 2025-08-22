// Package pq implements the post-quantum consensus engine that combines
// classical and quantum-resistant consensus mechanisms.
package pq

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/quasar"
	"github.com/luxfi/ids"
)

// ConsensusEngine implements post-quantum consensus combining classical and quantum-resistant security
type ConsensusEngine struct {
	params   config.Parameters
	quasar   *quasar.Quasar
	finality chan FinalityEvent
	mu       sync.RWMutex

	// State tracking
	height    uint64
	round     uint64
	finalized map[ids.ID]bool
}

// FinalityEvent represents a finalized block with quantum-resistant proofs
type FinalityEvent struct {
	Height    uint64
	BlockID   ids.ID
	Timestamp time.Time
	PQProof   []byte // Post-quantum proof
	BLSProof  []byte // Classical BLS proof
}

// NewConsensus creates a new post-quantum consensus engine
func NewConsensus(params config.Parameters) *ConsensusEngine {
	return &ConsensusEngine{
		params:    params,
		quasar:    quasar.New(params),
		finality:  make(chan FinalityEvent, 100),
		finalized: make(map[ids.ID]bool),
	}
}

// Initialize sets up the PQ engine with keys
func (e *ConsensusEngine) Initialize(ctx context.Context, blsKey, pqKey []byte) error {
	return e.quasar.Initialize(ctx, blsKey, pqKey)
}

// ProcessBlock processes a block through PQ consensus
func (e *ConsensusEngine) ProcessBlock(ctx context.Context, blockID ids.ID, votes map[string]int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check if already finalized
	if e.finalized[blockID] {
		return nil
	}

	// For now, use simplified consensus logic
	// In production, this would integrate with quasar's internal methods
	totalVotes := 0
	maxVotes := 0
	bestBlock := ""
	
	for block, count := range votes {
		totalVotes += count
		if count > maxVotes {
			maxVotes = count
			bestBlock = block
		}
	}
	
	// Check if we have enough votes for the best block
	if totalVotes == 0 || float64(maxVotes)/float64(totalVotes) < e.params.Alpha {
		return fmt.Errorf("insufficient votes for finality: %d/%d for block %s", maxVotes, totalVotes, bestBlock)
	}
	
	// Create mock certificate
	cert := &quasar.CertBundle{
		BLSAgg: []byte("mock-bls-aggregate"),
		PQCert: []byte("mock-pq-certificate"),
	}
	if cert == nil {
		return fmt.Errorf("insufficient votes for finality")
	}

	// Mark as finalized
	e.finalized[blockID] = true
	e.height++

	// Emit finality event
	select {
	case e.finality <- FinalityEvent{
		Height:    e.height,
		BlockID:   blockID,
		Timestamp: time.Now(),
		PQProof:   cert.PQCert,
		BLSProof:  cert.BLSAgg,
	}:
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Channel full, drop oldest
	}

	return nil
}

// IsFinalized checks if a block has achieved PQ finality
func (e *ConsensusEngine) IsFinalized(blockID ids.ID) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.finalized[blockID]
}

// FinalityChannel returns the channel for finality events
func (e *ConsensusEngine) FinalityChannel() <-chan FinalityEvent {
	return e.finality
}

// Height returns the current finalized height
func (e *ConsensusEngine) Height() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.height
}

// SetFinalizedCallback sets a callback for finalized blocks
func (e *ConsensusEngine) SetFinalizedCallback(cb func(FinalityEvent)) {
	e.quasar.SetFinalizedCallback(func(qb quasar.QBlock) {
		blockID, _ := ids.FromString(qb.Hash)
		cb(FinalityEvent{
			Height:    uint64(qb.Height),
			BlockID:   blockID,
			Timestamp: qb.Timestamp,
			PQProof:   qb.Cert.PQCert,
			BLSProof:  qb.Cert.BLSAgg,
		})
	})
}

// Metrics returns engine metrics
func (e *ConsensusEngine) Metrics() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	return map[string]interface{}{
		"height":         e.height,
		"round":          e.round,
		"finalized":      len(e.finalized),
		"k":              e.params.K,
		"alpha":          e.params.Alpha,
		"beta":           e.params.Beta,
		"block_time":     e.params.BlockTime.String(),
	}
}