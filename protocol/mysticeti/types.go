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

// Vote represents a fast-path vote reference
type Vote struct {
	TxRef [32]byte // Transaction reference
	Index uint32   // Vote index
}

// Block represents a MYSTICETI DAG block (single message, no separate certificates)
type Block struct {
	ID        ids.ID      // Block hash
	Author    ids.NodeID  // Validator ID
	Round     uint64      // Threshold clock round
	Parents   []ids.ID    // ≥2f+1 from r-1, first is self-chain
	Votes     []Vote      // Fast-path votes embedded
	EpochBit  bool        // Pauses fast-path near epoch end
	Signature []byte      // Author's signature (BLS or Ed25519)
	
	// Optional fields for compatibility
	Transactions []Transaction // Actual transactions (optional)
	Timestamp    int64         // Unix timestamp
	hash         ids.ID        // Cached hash
}

// Transaction represents a transaction in the system
type Transaction struct {
	ID        ids.ID
	Ref       [32]byte      // Transaction reference for voting
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

// FPVote represents a fast-path vote for an owned-object transaction (legacy)
// Use Vote struct in Block.Votes instead
type FPVote struct {
	TxID      ids.ID   // Hash of the owned-object transaction
	InputKeys [][]byte // Object IDs proving ownership domain
}

// Slot uniquely identifies a proposer slot
type Slot struct {
	Round     uint64
	SlotIndex uint16      // 0=primary, 1=backup, etc.
	Proposer  ids.NodeID
}

// SlotState represents the consensus state of a slot
type SlotState uint8

const (
	Undecided SlotState = iota
	ToCommit
	ToSkip
)

// Decision represents the consensus decision for a slot (legacy)
// Use SlotState instead
type Decision = SlotState

const (
	Commit = ToCommit
	Skip   = ToSkip
)

// Parameters holds consensus configuration
type Parameters struct {
	WaveLength         uint32        // Decision horizon (default 3)
	NumProposers       uint16        // Proposer slots per round (start with 2)
	TimeoutDelta       uint32        // Liveness timeout in ms (200-400ms)
	MaxParents         uint32        // Cap number of parents beyond 2f+1
	MinParents         uint32        // Minimum parent requirement (2f+1)
	GossipBatchBytes   uint32        // Maximum gossip batch size
	SigScheme          string        // Signature scheme (ed25519, bls)
	EnableFastPath     bool          // Enable fast-path for owned objects
	
	// Dual finality parameters
	EnableBLS          bool          // Enable BLS aggregation
	EnableRingtail     bool          // Enable Ringtail PQ finality
	AlphaClassical     uint32        // Classical threshold (2f+1)
	AlphaPQ            uint32        // PQ threshold (2f+1)
	QRounds            uint32        // PQ rounds (default 2)
}

// DefaultParameters returns default MYSTICETI parameters
func DefaultParameters() Parameters {
	return Parameters{
		WaveLength:       3,        // 3-round decision horizon
		NumProposers:     2,        // Primary + backup
		TimeoutDelta:     300,      // 300ms timeout
		MaxParents:       64,       // Maximum 64 parents
		MinParents:       0,        // Will be set to 2f+1
		GossipBatchBytes: 1000000,  // 1MB gossip batches
		SigScheme:        "bls",    // BLS for aggregation
		EnableFastPath:   true,
		EnableBLS:        true,
		EnableRingtail:   true,
		AlphaClassical:   0,        // Will be set to 2f+1
		AlphaPQ:          0,        // Will be set to 2f+1
		QRounds:          2,        // 2-phase PQ
	}
}

// ComputeID computes and caches the block's hash
func (b *Block) ComputeID() ids.ID {
	if b.ID == ids.Empty {
		// Compute hash from canonical encoding
		hashArray := hashing.ComputeHash256Array(b.Bytes())
		b.ID = ids.ID(hashArray)
		b.hash = b.ID
	}
	return b.ID
}

// Bytes returns the canonical byte representation
func (b *Block) Bytes() []byte {
	// TODO: Implement canonical encoding
	return nil
}

// Height returns the block's round (for compatibility)
func (b *Block) Height() uint64 {
	return b.Round
}

// SelfParent returns the first parent (proposer's previous block)
func (b *Block) SelfParent() ids.ID {
	if len(b.Parents) > 0 {
		return b.Parents[0]
	}
	return ids.Empty
}

// GetParents returns all parent IDs
func (b *Block) GetParents() []ids.ID {
	return b.Parents
}

// GetTimestamp returns the block's timestamp
func (b *Block) GetTimestamp() time.Time {
	return time.Unix(b.Timestamp, 0)
}

// Verify performs basic validation
func (b *Block) Verify() error {
	// TODO: Implement validation
	return nil
}