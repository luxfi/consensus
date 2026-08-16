// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package types

import (
	"time"

	"github.com/luxfi/consensus/core/choices"
	coretypes "github.com/luxfi/consensus/core/types"
	"github.com/luxfi/ids"
)

// Identifiers, named here so the block-and-vote surface reads in one vocabulary.
type (
	ID     = ids.ID
	NodeID = ids.NodeID
)

// Decision is the outcome a block has been assigned.
type Decision = coretypes.Decision

const (
	DecideUndecided = coretypes.DecideUndecided
	DecideAccept    = coretypes.DecideAccept
	DecideReject    = coretypes.DecideReject
)

// VoteType distinguishes a soft preference from a commitment.
type VoteType int

const (
	VotePreference VoteType = iota
	VoteCommit
)

// GenesisID is the ID of the genesis block.
var GenesisID = ids.Empty

// Block is a block in the chain.
type Block struct {
	ID       ID        `json:"id"`
	ParentID ID        `json:"parent_id"`
	Height   uint64    `json:"height"`
	Payload  []byte    `json:"payload"`
	Time     time.Time `json:"time"`
}

// Vote is one validator's vote on one block.
type Vote struct {
	BlockID   ID       `json:"block_id"`
	VoteType  VoteType `json:"vote_type"`
	Voter     NodeID   `json:"voter"`
	Signature []byte   `json:"signature"`
}

// Status is where a block stands.
type Status = choices.Status

const (
	StatusUnknown    = choices.Unknown
	StatusProcessing = choices.Processing
	StatusAccepted   = choices.Accepted
)

// Config parameterizes a chain engine.
type Config struct {
	// Alpha is the quorum: how many DISTINCT voters must name a block before it
	// is accepted. Counting votes rather than voters lets one node reach quorum
	// by itself. At one or below, the first vote to arrive is final.
	Alpha int `json:"alpha"`
}

// DefaultConfig returns the default consensus configuration.
func DefaultConfig() Config {
	return Config{Alpha: 20}
}
