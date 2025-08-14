// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package mysticeti implements the MYSTICETI uncertified-DAG consensus protocol
// with fast-path support for owned-object transactions.
package mysticeti

import (
	"time"

	"github.com/luxfi/crypto/hashing"
	"github.com/luxfi/ids"
)

// BlockHeader represents the header of a MYSTICETI block
type BlockHeader struct {
	Version      uint16      // Protocol version
	Epoch        uint64      // Committee configuration epoch
	Round        uint64      // Threshold clock round
	SlotIndex    uint16      // Proposer slot within the round (0..NumSlots-1)
	Proposer     ids.NodeID  // Validator ID
	ParentIDs    []ids.ID    // Hashes of parents (≥ 2f+1, first = proposer's prev block)
	TxRoot       ids.ID      // Merkle root of Transactions
	FPVoteRoot   ids.ID      // Merkle root of FastPathVotes
	Flags        BlockFlags  // Control flags
	WeightCommit uint64      // Stake weight snapshot
	Timestamp    int64       // Unix timestamp
}

// BlockFlags contains control flags for the block
type BlockFlags struct {
	EpochChangeBit bool // If true: votes in/under this block don't finalize fast-path
}

// Block represents a complete MYSTICETI block
type Block struct {
	Header        BlockHeader
	Transactions  []Transaction  // Shared + mixed + owned transactions
	FastPathVotes []FPVote       // Votes over owned transactions
	Signature     []byte         // Proposer's signature
	hash          ids.ID         // Cached hash
}

// Transaction represents a transaction in the system
type Transaction struct {
	ID        ids.ID
	Type      TxType        // Owned, Shared, or Mixed
	Payload   []byte
	InputKeys [][]byte      // Object IDs for ownership verification
	Nonce     uint64
}

// TxType defines the transaction type for fast-path eligibility
type TxType uint8

const (
	TxTypeOwned  TxType = iota // Owned-object transaction (fast-path eligible)
	TxTypeShared                // Shared-object transaction (consensus path only)
	TxTypeMixed                 // Mixed owned+shared (consensus path only)
)

// FPVote represents a fast-path vote for an owned-object transaction
type FPVote struct {
	TxID      ids.ID   // Hash of the owned-object transaction
	InputKeys [][]byte // Object IDs proving ownership domain
}

// SlotID uniquely identifies a proposer slot
type SlotID struct {
	Round     uint64
	SlotIndex uint16
	Proposer  ids.NodeID
}

// Decision represents the consensus decision for a slot
type Decision uint8

const (
	Undecided Decision = iota
	Commit
	Skip
)

// Parameters holds consensus configuration
type Parameters struct {
	WaveLength         uint32        // Min secure distance between proposer & decision rounds
	NumSlotsPerRound   uint16        // Number of proposer slots per round
	PrimaryTimeoutMS   uint32        // Wait for primary slot's proposal
	DecisionWaitMS     uint32        // Optional wait to accumulate 2f+1 certs
	MaxParents         uint32        // Cap number of parents beyond 2f+1
	MinParents         uint32        // Minimum parent requirement (2f+1)
	GossipBatchBytes   uint32        // Maximum gossip batch size
	SigScheme          string        // Signature scheme (ed25519, dilithium)
	EnableFastPath     bool          // Enable fast-path for owned objects
}

// DefaultParameters returns default MYSTICETI parameters
func DefaultParameters() Parameters {
	return Parameters{
		WaveLength:       3,      // 3-round commit
		NumSlotsPerRound: 2,      // Start with 2 slots for stability
		PrimaryTimeoutMS: 800,    // 800ms primary timeout
		DecisionWaitMS:   250,    // 250ms decision wait
		MaxParents:       64,     // Maximum 64 parents
		MinParents:       0,      // Will be set to 2f+1 based on validator count
		GossipBatchBytes: 1000000, // 1MB gossip batches
		SigScheme:        "ed25519",
		EnableFastPath:   true,
	}
}

// ID returns the block's hash
func (b *Block) ID() ids.ID {
	if b.hash == ids.Empty {
		// Compute hash from canonical encoding
		// Compute hash using SHA256
		hashArray := hashing.ComputeHash256Array(b.Bytes())
		b.hash = ids.ID(hashArray)
	}
	return b.hash
}

// Bytes returns the canonical byte representation
func (b *Block) Bytes() []byte {
	// TODO: Implement canonical encoding
	return nil
}

// Height returns the block's round (for compatibility)
func (b *Block) Height() uint64 {
	return b.Header.Round
}

// Parent returns the first parent (proposer's previous block)
func (b *Block) Parent() ids.ID {
	if len(b.Header.ParentIDs) > 0 {
		return b.Header.ParentIDs[0]
	}
	return ids.Empty
}

// Parents returns all parent IDs
func (b *Block) Parents() []ids.ID {
	return b.Header.ParentIDs
}

// GetTimestamp returns the block's timestamp
func (b *Block) Timestamp() time.Time {
	return time.Unix(b.Header.Timestamp, 0)
}

// Verify performs basic validation
func (b *Block) Verify() error {
	// TODO: Implement validation
	return nil
}