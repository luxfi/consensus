// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package types

import (
	"time"

	"github.com/luxfi/consensus/core/choices"
	coretypes "github.com/luxfi/consensus/core/types"
	"github.com/luxfi/ids"
)

// Core type aliases for convenience
type (
	// Basic identifiers
	ID     = ids.ID
	NodeID = ids.NodeID
	Hash   = ids.ID

	// Cryptographic types
	Signature  []byte
	PublicKey  []byte
	PrivateKey []byte
)

// Decision re-exports from core/types for consistency
type Decision = coretypes.Decision

// Decision constants re-exported from core/types
const (
	DecideUndecided = coretypes.DecideUndecided
	DecideAccept    = coretypes.DecideAccept
	DecideReject    = coretypes.DecideReject
)

// VoteType represents the type of vote
type VoteType int

const (
	VotePreference VoteType = iota
	VoteCommit
	VoteCancel
)

// GenesisID is the ID of the genesis block
var GenesisID = ids.Empty

// Block represents a block in the blockchain
type Block struct {
	ID       ID        `json:"id"`
	ParentID ID        `json:"parent_id"`
	Height   uint64    `json:"height"`
	Payload  []byte    `json:"payload"`
	Time     time.Time `json:"time"`
}

// Vote represents a vote on a block
type Vote struct {
	BlockID   ID       `json:"block_id"`
	VoteType  VoteType `json:"vote_type"`
	Voter     NodeID   `json:"voter"`
	Signature []byte   `json:"signature"`
}

// Certificate represents a consensus certificate
type Certificate struct {
	BlockID    ID        `json:"block_id"`
	Height     uint64    `json:"height"`
	Votes      []Vote    `json:"votes"`
	Timestamp  time.Time `json:"timestamp"`
	Signatures [][]byte  `json:"signatures"`
}

// Status re-exports from choices for consistency
type Status = choices.Status

// Status constants re-exported from choices
const (
	StatusUnknown    = choices.Unknown
	StatusProcessing = choices.Processing
	StatusRejected   = choices.Rejected
	StatusAccepted   = choices.Accepted
)

// Config represents consensus configuration
type Config struct {
	// Consensus parameters
	Alpha          int           `json:"alpha"`           // Quorum size
	K              int           `json:"k"`               // Sample size
	MaxOutstanding int           `json:"max_outstanding"` // Max outstanding polls
	MaxPollDelay   time.Duration `json:"max_poll_delay"`  // Max delay between polls

	// Network parameters
	NetworkTimeout time.Duration `json:"network_timeout"`
	MaxMessageSize int           `json:"max_message_size"`

	// Security parameters
	SecurityLevel    int  `json:"security_level"`    // NIST security level
	QuantumResistant bool `json:"quantum_resistant"` // Use PQ crypto
	GPUAcceleration  bool `json:"gpu_acceleration"`  // Use GPU acceleration
}

// DefaultConfig returns default consensus configuration
func DefaultConfig() Config {
	return Config{
		Alpha:          20,
		K:              20,
		MaxOutstanding: 10,
		MaxPollDelay:   time.Second,

		NetworkTimeout: 5 * time.Second,
		MaxMessageSize: 2 * 1024 * 1024, // 2MB

		SecurityLevel:    5, // NIST Level 5
		QuantumResistant: true,
		GPUAcceleration:  true,
	}
}
