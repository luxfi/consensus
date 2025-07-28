// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/bag"
)


// Status represents the current state of a decidable element
type Status uint8

const (
	Unknown Status = iota
	Processing
	Rejected  
	Accepted
)

// Consensus represents a consensus instance
type Consensus interface {
	// Add a new item to track
	Add(context.Context, Decidable) error
	
	// RecordPoll records the results of a poll
	RecordPoll(context.Context, bag.Bag[ids.ID]) error
	
	// Finalized returns the finalized items
	Finalized() []Decidable
	
	// HealthCheck returns whether consensus is healthy
	HealthCheck(context.Context) (interface{}, error)
	
	// Parameters returns the consensus parameters
	Parameters() Parameters
}

// Parameters represents consensus parameters
type Parameters struct {
	K                     int
	AlphaPreference       int
	AlphaConfidence       int
	Beta                  int
	ConcurrentRepolls     int
}

// Validator represents a validator in the network
type Validator interface {
	// NodeID returns the validator's node ID
	NodeID() ids.NodeID
	
	// Weight returns the validator's stake weight
	Weight() uint64
}

// ValidatorSet represents a set of validators
type ValidatorSet interface {
	// Sample returns a sample of k validators
	Sample(k int) ([]Validator, error)
	
	// Weight returns the total weight
	Weight() uint64
	
	// Len returns the number of validators
	Len() int
	
	// Get returns a validator by node ID
	Get(ids.NodeID) (Validator, bool)
}

// Poll represents a poll for consensus
type Poll interface {
	// Vote records a vote
	Vote(vdr ids.NodeID, vote ids.ID) 
	
	// Drop removes a vote
	Drop(vdr ids.NodeID)
	
	// Finished returns whether the poll has reached a decision
	Finished() bool
	
	// Result returns the result if finished
	Result() bag.Bag[ids.ID]
}