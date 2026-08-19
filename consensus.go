// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

import (
	"errors"

	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/consensus/types"
)

// The names a caller needs to write down the signatures below. The protocol
// packages under protocol/ and the production engines under engine/chain/ are
// imported directly; this root package is the small block-and-vote surface,
// not a mirror of the module.
type (
	Chain    = engine.Chain
	Config   = types.Config
	Block    = types.Block
	Vote     = types.Vote
	VoteType = types.VoteType
	ID       = types.ID
	NodeID   = types.NodeID
)

const (
	VotePreference = types.VotePreference
	VoteCommit     = types.VoteCommit
)

// GenesisID is the ID of the genesis block.
var GenesisID = types.GenesisID

var (
	// ErrTimeout reports that an operation did not settle in its window.
	ErrTimeout = types.ErrTimeout

	// ErrUnknownState reports a state value outside the defined set.
	ErrUnknownState = errors.New("unknown state")
)

// DefaultConfig returns the default consensus configuration.
func DefaultConfig() Config {
	return types.DefaultConfig()
}

// NewChain creates a chain consensus engine. A block reaches accepted once
// cfg.Alpha votes name it.
func NewChain(cfg Config) *Chain {
	return engine.NewChain(cfg)
}

// NewBlock creates a block at height, descending from parentID.
func NewBlock(id ID, parentID ID, height uint64, payload []byte) *Block {
	return &Block{
		ID:       id,
		ParentID: parentID,
		Height:   height,
		Payload:  payload,
	}
}

// NewVote creates a vote by voter on blockID.
func NewVote(blockID ID, voteType VoteType, voter NodeID) *Vote {
	return &Vote{
		BlockID:  blockID,
		VoteType: voteType,
		Voter:    voter,
	}
}
