// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"context"
	"time"

	"github.com/luxfi/ids"
)

// ConsensusParams defines consensus parameters
type ConsensusParams struct {
	K                     int
	AlphaPreference       int
	AlphaConfidence       int
	Beta                  int
	ConcurrentPolls       int
	OptimalProcessing     int
	MaxOutstandingItems   int
	MaxItemProcessingTime time.Duration
}

// Block interface for consensus
type Block interface {
	ID() ids.ID
	ParentID() ids.ID
	Height() uint64
	Timestamp() int64
	Bytes() []byte
	Verify(context.Context) error
	Accept(context.Context) error
	Reject(context.Context) error
}

// Consensus interface
type Consensus interface {
	Add(Block) error
	RecordPoll(ids.ID, bool) error
	IsAccepted(ids.ID) bool
	GetPreference() ids.ID
	Finalized() bool
	Parameters() ConsensusParams
	HealthCheck() error
}

// Stats represents consensus statistics
type Stats struct {
	BlocksAccepted        uint64
	BlocksRejected        uint64
	VotesProcessed        uint64
	PollsCompleted        uint64
	AverageDecisionTimeMs float64
}

// BlockStatus represents block status
type BlockStatus uint8

const (
	StatusUnknown BlockStatus = iota
	StatusProcessing
	StatusAccepted
	StatusRejected
)
