// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package fpc

import (
	"crypto/sha256"
	"sync"
	"sync/atomic"

	"github.com/luxfi/ids"
)

// simpleLogger wraps log functions for Ringtail
type simpleLogger struct{}

func (s *simpleLogger) Debug(msg string, args ...interface{}) {}
func (s *simpleLogger) Info(msg string, args ...interface{})  {}
func (s *simpleLogger) Warn(msg string, args ...interface{})  {}
func (s *simpleLogger) Error(msg string, args ...interface{}) {}

// Implementation of WaveFPC - the core FPC engine
type waveFPC struct {
	cfg      Config
	cls      Classifier
	dag      DAGTap
	pq       PQEngine
	// ringtail would go here if needed

	mu          sync.RWMutex
	epochPaused atomic.Bool

	// Core state (sharded for performance)
	votes     *ShardedMap[TxRef, *Bitset]    // Voters for tx
	votedOn   *ShardedMap[[64]byte, TxRef]   // key = hash(validator||object)
	state     *ShardedMap[TxRef, Status]     // Transaction state
	mixedTxs  *ShardedMap[TxRef, bool]       // Tracks mixed txs
	conflicts *ShardedMap[ObjectID, []TxRef] // Object -> conflicting txs

	// Epoch tracking
	epochBitAuthors map[ids.NodeID]bool
	epochMu         sync.Mutex

	// Metrics
	metrics Metrics

	// My node identity
	myNodeID   ids.NodeID
	validators []ids.NodeID
}

// New creates a new WaveFPC instance
func New(cfg Config, cls Classifier, dag DAGTap, pq PQEngine, myNodeID ids.NodeID, validators []ids.NodeID) WaveFPC {

	return &waveFPC{
		cfg:             cfg,
		cls:             cls,
		dag:             dag,
		pq:              pq,
		myNodeID:        myNodeID,
		validators:      validators,
		votes:           NewShardedMap[TxRef, *Bitset](16),
		votedOn:         NewShardedMap[[64]byte, TxRef](16),
		state:           NewShardedMap[TxRef, Status](16),
		mixedTxs:        NewShardedMap[TxRef, bool](16),
		conflicts:       NewShardedMap[ObjectID, []TxRef](16),
		epochBitAuthors: make(map[ids.NodeID]bool),
	}
}

// NextVotes returns votes to include in the next block
func (w *waveFPC) NextVotes(budget int) []TxRef {
	if w.epochPaused.Load() {
		return nil // No new votes during epoch close
	}

	picks := make([]TxRef, 0, budget)
	processed := 0

	// TODO: Get candidates from mempool/DAG frontier
	// For now, return empty
	candidates := w.getCandidates()

	for _, tx := range candidates {
		if processed >= budget {
			break
		}

		// Check if it's owned-only
		owned := w.cls.OwnedInputs(tx)
		if len(owned) == 0 {
			continue // Skip shared/mixed txs
		}

		// Check if we've already voted on any of these objects
		canVote := true
		for _, obj := range owned {
			key := w.makeVoteKey(w.myNodeID, obj)
			if existing, ok := w.votedOn.Get(key); ok {
				if existing != tx {
					canVote = false // Already voted for different tx on this object
					break
				}
			}
		}

		if !canVote {
			continue
		}

		// Reserve our vote locally
		for _, obj := range owned {
			key := w.makeVoteKey(w.myNodeID, obj)
			w.votedOn.Set(key, tx)
		}

		picks = append(picks, tx)
		processed++
	}

	return picks
}

// OnBlockObserved processes votes from an observed block
func (w *waveFPC) OnBlockObserved(b *Block) {
	if len(b.Payload.FPCVotes) == 0 {
		return
	}

	voterIdx := ValidatorIndex(b.Author, w.validators)
	if voterIdx < 0 {
		return // Not in validator set
	}

	for _, raw := range b.Payload.FPCVotes {
		var tx TxRef
		copy(tx[:], raw)

		// Get owned inputs
		owned := w.cls.OwnedInputs(tx)
		if len(owned) == 0 {
			continue // Not an owned tx
		}

		// Check for equivocation on each object
		validVote := true
		for _, obj := range owned {
			key := w.makeVoteKey(b.Author, obj)
			if prev, ok := w.votedOn.Get(key); ok && prev != tx {
				validVote = false // Equivocation detected
				break
			}
		}

		if !validVote {
			continue
		}

		// Record the vote for each object
		for _, obj := range owned {
			key := w.makeVoteKey(b.Author, obj)
			w.votedOn.Set(key, tx)

			// Track conflicts
			w.addConflict(obj, tx)
		}

		// Update vote bitset
		bs := w.getOrCreateBitset(tx)
		bs.mu.Lock()
		newVote := bs.Set(voterIdx)
		count := bs.Count()
		bs.mu.Unlock()

		if newVote && count >= 2*w.cfg.F+1 {
			// Transaction is now executable!
			w.state.Set(tx, Executable)
			atomic.AddUint64(&w.metrics.ExecutableTxs, 1)

			// Submit to PQ engine if available
			if w.pq != nil {
				bs.mu.Lock()
				voters := bs.GetVoters(w.validators)
				bs.mu.Unlock()
				w.pq.Submit(tx, voters)
			}
		}

		atomic.AddUint64(&w.metrics.TotalVotes, 1)
	}
}

// OnBlockAccepted checks for anchoring when a block is accepted
func (w *waveFPC) OnBlockAccepted(b *Block) {
	// Check if any executable txs can now be marked final
	for _, raw := range b.Payload.FPCVotes {
		var tx TxRef
		copy(tx[:], raw)

		if st, _ := w.state.Get(tx); st == Executable {
			if w.anchorCovers(tx, b) {
				w.state.Set(tx, Final)
				atomic.AddUint64(&w.metrics.FinalTxs, 1)
			}
		}
	}

	// Track epoch bit authors
	if b.Payload.EpochBit {
		w.registerEpochBitAuthor(b.Author)
	}
}

// OnEpochCloseStart begins epoch closing
func (w *waveFPC) OnEpochCloseStart() {
	w.epochPaused.Store(true)
	atomic.AddUint64(&w.metrics.EpochChanges, 1)
}

// OnEpochClosed completes epoch transition
func (w *waveFPC) OnEpochClosed() {
	w.epochPaused.Store(false)

	// Clear epoch bit authors
	w.epochMu.Lock()
	w.epochBitAuthors = make(map[ids.NodeID]bool)
	w.epochMu.Unlock()

	// TODO: Clear old vote state
}

// Status returns the current status and proof for a transaction
func (w *waveFPC) Status(tx TxRef) (Status, Proof) {
	st, _ := w.state.Get(tx)

	proof := Proof{
		Status: st,
	}

	// Get vote count
	if bs := w.getBitset(tx); bs != nil {
		bs.mu.Lock()
		proof.VoterCount = bs.Count()
		proof.VoterBitmap = bs.Bytes()
		bs.mu.Unlock()
	}

	// Get PQ proof if available
	if w.pq != nil && w.pq.HasPQ(tx) {
		if pqBundle, ok := w.pq.GetPQ(tx); ok {
			proof.RingtailProof = pqBundle
		}
	}

	// Check if both BLS and Ringtail are ready for dual finality
	if w.cfg.EnableBLS && w.cfg.EnableRingtail {
		// In production, BLS would be aggregated from block signatures
		// For now, simulate BLS availability when we have quorum
		if proof.VoterCount >= 2*w.cfg.F+1 {
			proof.BLSProof = &BLSBundle{
				VoterBitmap: proof.VoterBitmap,
				Message:     tx[:],
			}
		}
	}

	return st, proof
}

// MarkMixed marks a transaction as mixed (owned+shared)
func (w *waveFPC) MarkMixed(tx TxRef) {
	w.mixedTxs.Set(tx, true)
	w.state.Set(tx, Mixed)
}

// GetMetrics returns current metrics
func (w *waveFPC) GetMetrics() Metrics {
	return w.metrics
}

// Helper methods

func (w *waveFPC) makeVoteKey(validator ids.NodeID, obj ObjectID) [64]byte {
	h := sha256.New()
	h.Write(validator[:])
	h.Write(obj[:])
	var key [64]byte
	copy(key[:32], h.Sum(nil))
	return key
}

func (w *waveFPC) getOrCreateBitset(tx TxRef) *Bitset {
	bs, _ := w.votes.GetOrCreate(tx, func() *Bitset {
		return NewBitset(w.cfg.N)
	})
	return bs
}

func (w *waveFPC) getBitset(tx TxRef) *Bitset {
	bs, _ := w.votes.Get(tx)
	return bs
}

func (w *waveFPC) anchorCovers(tx TxRef, anchor *Block) bool {
	// Check if anchor's ancestry contains enough votes for tx
	bs := w.getBitset(tx)
	if bs == nil {
		return false
	}

	bs.mu.Lock()
	voteCount := bs.Count()
	bs.mu.Unlock()

	// Need ≥2f+1 votes and anchor must contain them in ancestry
	if voteCount < 2*w.cfg.F+1 {
		return false
	}

	// For simplicity, assume anchor covers if it or its ancestry contains tx
	// In production, check actual vote inclusion in ancestor blocks
	return w.dag.InAncestry(anchor.ID, tx)
}

func (w *waveFPC) registerEpochBitAuthor(author ids.NodeID) {
	w.epochMu.Lock()
	defer w.epochMu.Unlock()

	w.epochBitAuthors[author] = true

	// Check if we have enough authors for epoch close
	if len(w.epochBitAuthors) >= 2*w.cfg.F+1 {
		// Epoch can close
		// TODO: Trigger epoch close completion
	}
}

func (w *waveFPC) addConflict(obj ObjectID, tx TxRef) {
	existing, _ := w.conflicts.Get(obj)

	// Check if already tracked
	for _, e := range existing {
		if e == tx {
			return
		}
	}

	// Add to conflicts
	existing = append(existing, tx)
	w.conflicts.Set(obj, existing)

	if len(existing) > 1 {
		atomic.AddUint64(&w.metrics.ConflictCount, 1)
	}
}

func (w *waveFPC) getCandidates() []TxRef {
	// TODO: Get from mempool/DAG frontier
	// This would integrate with your existing transaction pool
	return nil
}

// Bitset for tracking voters (thread-safe)
type Bitset struct {
	mu   sync.Mutex
	bits []uint64
	size int
}

func NewBitset(size int) *Bitset {
	numWords := (size + 63) / 64
	return &Bitset{
		bits: make([]uint64, numWords),
		size: size,
	}
}

func (b *Bitset) Set(idx int) bool {
	if idx < 0 || idx >= b.size {
		return false
	}

	word := idx / 64
	bit := uint64(1) << (idx % 64)

	if b.bits[word]&bit != 0 {
		return false // Already set
	}

	b.bits[word] |= bit
	return true
}

func (b *Bitset) Count() int {
	count := 0
	for _, word := range b.bits {
		// Brian Kernighan's algorithm
		w := word
		for w != 0 {
			w &= w - 1
			count++
		}
	}
	return count
}

func (b *Bitset) Bytes() []byte {
	bytes := make([]byte, len(b.bits)*8)
	for i, word := range b.bits {
		for j := 0; j < 8; j++ {
			bytes[i*8+j] = byte(word >> (j * 8))
		}
	}
	return bytes
}

func (b *Bitset) GetVoters(validators []ids.NodeID) []ids.NodeID {
	var voters []ids.NodeID
	for i := 0; i < b.size && i < len(validators); i++ {
		word := i / 64
		bit := uint64(1) << (i % 64)
		if b.bits[word]&bit != 0 {
			voters = append(voters, validators[i])
		}
	}
	return voters
}
