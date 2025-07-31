// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/bag"
)

// Unary represents a unary consensus poll
type Unary interface {
	// RecordPrism records a prism with votes
	RecordPrism(votes []ids.ID)
	
	// Preference returns the current preference
	Preference() ids.ID
	
	// Finalized returns true if consensus is reached
	Finalized() bool
	
	// Extend creates a binary prism from this unary poll
	Extend(choice int) Binary
	
	// Clone creates a copy of this poll
	Clone() Unary
}

// Binary represents a binary consensus poll
type Binary interface {
	// RecordPrism records a prism with votes
	RecordPrism(votes []ids.ID)
	
	// Preference returns the current preference
	Preference() ids.ID
	
	// Finalized returns true if consensus is reached
	Finalized() bool
}

// MultiChoice represents a multi-choice consensus poll
type MultiChoice interface {
	// RecordPrism records a prism with votes
	RecordPrism(votes []ids.ID)
	
	// Preference returns the current preference
	Preference() ids.ID
	
	// Finalized returns true if consensus is reached
	Finalized() bool
}

// BinarySampler is used for binary sampling in consensus
type BinarySampler struct {
	preference int
}

// NewBinarySampler creates a new binary sampler
func NewBinarySampler(choice int) BinarySampler {
	return BinarySampler{preference: choice}
}

// Preference returns the current preference
func (b BinarySampler) Preference() int {
	return b.preference
}

// RecordSuccessfulPrism records a successful prism for the given choice
func (b *BinarySampler) RecordSuccessfulPoll(choice int) {
	b.preference = choice
}

// UnarySampler is used for unary sampling
type UnarySampler struct {
	count int
}

// RecordSuccessfulPrism records a successful poll
func (u *UnarySampler) RecordSuccessfulPoll() {
	u.count++
}

// Decision represents a consensus decision
type Decision interface {
	ID() ids.ID
	Accept() error
	Reject() error
}

// DependencyGraph represents a graph of decisions and their dependencies
type DependencyGraph interface {
	GetDependencies(decisionID ids.ID) []ids.ID
	GetDecisions() []ids.ID
}

// Splitter samples validators from a set
type Splitter interface {
	Sample(validators bag.Bag[ids.NodeID], numToSample int) ([]ids.NodeID, error)
}

// Cutter determines when consensus is reached
type Cutter interface {
	// IsAlphaPreferred returns true if the decision has enough votes to be preferred
	IsAlphaPreferred(votes int) bool
	
	// IsAlphaConfident returns true if the decision has enough votes to be confident
	IsAlphaConfident(votes int) bool
	
	// IsBetaVirtuous returns true if the decision meets beta threshold
	IsBetaVirtuous(successfulPolls int) bool
}

// ValidatorSet provides access to validator information
type ValidatorSet interface {
	GetWeight(nodeID ids.NodeID) uint64
	Sample(size int) ([]ids.NodeID, error)
}