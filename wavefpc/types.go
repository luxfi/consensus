// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package wavefpc implements Fast-Path Consensus that rides on top of existing consensus
// with zero extra messages - votes piggyback in blocks, no QCs, no protocol changes
package wavefpc

import (
	"time"
	
	"github.com/luxfi/ids"
)

// TxRef is a transaction reference (32 bytes)
type TxRef [32]byte

// ObjectID represents an owned object identifier
type ObjectID [32]byte

// Status represents the FPC state of a transaction
type Status uint8

const (
	Pending    Status = iota // Not enough votes yet
	Executable               // ≥2f+1 votes, can execute owned-only tx
	Final                    // Anchored in accepted block or has certificate in DAG
	Mixed                    // Mixed owned+shared, needs Final before execution
)

// Config holds WaveFPC configuration
type Config struct {
	N                 int           // Committee size (3F+1)
	F                 int           // Byzantine fault tolerance
	Epoch             uint64        // Current epoch
	VoteLimitPerBlock int           // Max votes per block (e.g., 256)
	VotePrefix        []byte        // Domain separator for proofs
	Clock             func() time.Time
}

// Block represents the minimal block interface we need
type Block struct {
	ID       ids.ID
	Author   ids.NodeID
	Round    uint64
	Payload  BlockPayload
}

// BlockPayload contains FPC-specific fields added to existing blocks
type BlockPayload struct {
	// Your existing fields...
	
	// FPC additions (minimal!)
	FPCVotes [][]byte // Array of txRefs (32 bytes each), owned-only, no dupes
	EpochBit bool     // True when starting epoch-close; pauses new fast finality
}

// Proof contains finality proof information
type Proof struct {
	Status       Status
	VoterCount   int
	VoterBitmap  []byte     // Bitset of voters
	BLSProof     *BLSBundle // Optional BLS aggregated signature
	RingtailProof *PQBundle  // Optional Ringtail PQ proof
}

// BLSBundle contains aggregated BLS signature proof
type BLSBundle struct {
	AggSignature []byte
	VoterBitmap  []byte
	Message      []byte
}

// PQBundle contains Ringtail post-quantum proof
type PQBundle struct {
	Proof       []byte
	VoterBitmap []byte
}

// Classifier identifies owned vs shared inputs
type Classifier interface {
	// OwnedInputs returns owned object IDs; empty if none (shared/mixed)
	OwnedInputs(tx TxRef) []ObjectID
	
	// Conflicts returns true if two txs conflict based on owned inputs
	Conflicts(a, b TxRef) bool
}

// DAGTap provides fast ancestry checks
type DAGTap interface {
	// InAncestry checks if needleTx is in ancestry of blockID
	InAncestry(blockID ids.ID, needleTx TxRef) bool
	
	// Optional: resolve (author,round) -> blockID if you track rounds
	GetBlockByAuthorRound(author ids.NodeID, round uint64) (ids.ID, bool)
}

// PQEngine handles optional Ringtail PQ signatures
type PQEngine interface {
	// Submit starts PQ signature collection once tx has ≥2f+1 votes
	Submit(tx TxRef, voters []ids.NodeID)
	
	// HasPQ returns true when PQ proof is ready
	HasPQ(tx TxRef) bool
	
	// GetPQ returns the PQ bundle if ready
	GetPQ(tx TxRef) (*PQBundle, bool)
}

// WaveFPC is the main FPC interface
type WaveFPC interface {
	// Wire-in points for consensus engine
	OnBlockObserved(b *Block)    // Called for every seen block (pre-accept ok)
	OnBlockAccepted(b *Block)     // Called when block becomes accepted/committed
	OnEpochCloseStart()           // Flip EpochBit on next blocks; pause fast finality
	OnEpochClosed()               // Cleanup transient locks
	
	// Proposer hook when building a block
	NextVotes(budget int) []TxRef
	
	// Introspection
	Status(tx TxRef) (Status, Proof)
	
	// For mixed txs: gate execution until anchor accepted
	MarkMixed(tx TxRef)
	
	// Metrics
	GetMetrics() Metrics
}

// Metrics tracks FPC performance
type Metrics struct {
	TotalVotes      uint64
	ExecutableTxs   uint64
	FinalTxs        uint64
	ConflictCount   uint64
	EpochChanges    uint64
	VoteLatency     time.Duration
	FinalityLatency time.Duration
}

// ValidatorIndex returns the index of a validator in the committee
func ValidatorIndex(nodeID ids.NodeID, validators []ids.NodeID) int {
	for i, v := range validators {
		if v == nodeID {
			return i
		}
	}
	return -1
}