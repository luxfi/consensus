// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package poll

import (
	"github.com/luxfi/ids"
)

// Unary represents a unary consensus poll
type Unary interface {
	// RecordPoll records a poll with votes
	RecordPoll(votes []ids.ID)
	
	// Preference returns the current preference
	Preference() ids.ID
	
	// Finalized returns true if consensus is reached
	Finalized() bool
	
	// Extend creates a binary poll from this unary poll
	Extend(choice int) Binary
	
	// Clone creates a copy of this poll
	Clone() Unary
}

// Binary represents a binary consensus poll
type Binary interface {
	// RecordPoll records a poll with votes
	RecordPoll(votes []ids.ID)
	
	// Preference returns the current preference
	Preference() ids.ID
	
	// Finalized returns true if consensus is reached
	Finalized() bool
}

// Nnary represents an n-ary consensus poll
type Nnary interface {
	// RecordPoll records a poll with votes
	RecordPoll(votes []ids.ID)
	
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

// RecordSuccessfulPoll records a successful poll for the given choice
func (b *BinarySampler) RecordSuccessfulPoll(choice int) {
	b.preference = choice
}

// UnarySampler is used for unary sampling
type UnarySampler struct {
	count int
}

// RecordSuccessfulPoll records a successful poll
func (u *UnarySampler) RecordSuccessfulPoll() {
	u.count++
}