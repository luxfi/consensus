// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package ai

// Vote represents a consensus vote for batch processing.
type Vote struct {
	VoterID      [32]byte
	BlockID      [32]byte
	IsPreference bool
}

// ValidatorInfo contains validator information for quorum calculations.
type ValidatorInfo struct {
	ValidatorID [32]byte
	Weight      uint64
}

// QuorumResult contains the result of a quorum check.
type QuorumResult struct {
	HasQuorum    bool
	TotalWeight  uint64
	VotedWeight  uint64
	QuorumWeight uint64
}
