// Copyright (C) 2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package focus

import (
	"fmt"
	
	"github.com/luxfi/ids"
)

// Dyadic is a focus instance deciding between two values
type Dyadic interface {
	fmt.Stringer
	
	// Returns the currently preferred choice
	Preference() int
	
	// RecordPoll records the results of a network poll
	RecordPoll(count, choice int)
	
	// RecordUnsuccessfulPoll resets the confidence counter
	RecordUnsuccessfulPoll()
	
	// Return whether a choice has been finalized
	Finalized() bool
}

// Monadic is a focus instance deciding on one value
type Monadic interface {
	fmt.Stringer
	
	// RecordPoll records the results of a network poll
	RecordPoll(count int)
	
	// RecordUnsuccessfulPoll resets the confidence counter
	RecordUnsuccessfulPoll()
	
	// Return whether a choice has been finalized
	Finalized() bool
	
	// Returns a new dyadic instance with the original choice
	Extend(originalPreference int) Dyadic
	
	// Returns a new monadic instance with the same state
	Clone() Monadic
}

// Polyadic is a focus instance deciding between multiple values
type Polyadic interface {
	fmt.Stringer
	
	// Adds a new possible choice
	Add(newChoice ids.ID)
	
	// Returns the currently preferred choice
	Preference() ids.ID
	
	// RecordPoll records the results of a network poll
	RecordPoll(count int, choice ids.ID)
	
	// RecordUnsuccessfulPoll resets the confidence counter
	RecordUnsuccessfulPoll()
	
	// Return whether a choice has been finalized
	Finalized() bool
}

// Parameters for focus consensus
type Parameters struct {
	K               int // Sample size
	AlphaPreference int // Preference threshold
	AlphaConfidence int // Confidence threshold  
	Beta            int // Finalization threshold
}

// terminationCondition defines when focus finalizes
type terminationCondition struct {
	alphaConfidence int
	beta            int
}

// Factory creates focus instances
type Factory interface {
	NewDyadic(params Parameters, choice int) Dyadic
	NewMonadic(params Parameters) Monadic
	NewPolyadic(params Parameters, choice ids.ID) Polyadic
}

// newSingleTerminationCondition creates a single termination condition
func newSingleTerminationCondition(alphaConfidence int, beta int) []terminationCondition {
	return []terminationCondition{
		{
			alphaConfidence: alphaConfidence,
			beta:            beta,
		},
	}
}