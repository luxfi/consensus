// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

import (
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils"
)

// Consensus represents a general consensus instance that can be used directly to
// process the results of network queries.
type Consensus interface {
	fmt.Stringer

	// Adds a new choice to vote on
	Add(newChoice ids.ID)

	// Returns the currently preferred choice to be finalized
	Preference() ids.ID

	// RecordPoll records the results of a network poll. Assumes all choices
	// have been previously added.
	//
	// If the consensus instance was not previously finalized, this function
	// will return true if the poll was successful and false if the poll was
	// unsuccessful.
	//
	// If the consensus instance was previously finalized, the function may
	// return true or false.
	RecordPoll(votes *utils.Bag) bool

	// RecordUnsuccessfulPoll resets the wave counters of this consensus
	// instance
	RecordUnsuccessfulPoll()

	// Return whether a choice has been finalized
	Finalized() bool
}

// Factory produces Polyadic and Monadic decision instances
type Factory interface {
	NewPolyadic(params Parameters, choice ids.ID) Polyadic
	NewMonadic(params Parameters) Monadic
}

// Polyadic is a photon instance deciding between an unbounded number of values.
// The caller samples k nodes and calls RecordPoll with the result.
// RecordUnsuccessfulPoll resets the confidence counters when one or
// more consecutive polls fail to reach alphaPreference votes.
type Polyadic interface {
	fmt.Stringer

	// Adds a new possible choice
	Add(newChoice ids.ID)

	// Returns the currently preferred choice to be finalized
	Preference() ids.ID

	// RecordPoll records the results of a network poll
	RecordPoll(count int, choice ids.ID)

	// RecordUnsuccessfulPoll resets the wave counter of this instance
	RecordUnsuccessfulPoll()

	// Return whether a choice has been finalized
	Finalized() bool
}

// Dyadic is a photon instance deciding between two values.
// The caller samples k nodes and calls RecordPoll with the result.
// RecordUnsuccessfulPoll resets the confidence counters when one or
// more consecutive polls fail to reach alphaPreference votes.
type Dyadic interface {
	fmt.Stringer

	// Returns the currently preferred choice to be finalized
	Preference() int

	// RecordPoll records the results of a network poll
	RecordPoll(count, choice int)

	// RecordUnsuccessfulPoll resets the wave counter of this instance
	RecordUnsuccessfulPoll()

	// Return whether a choice has been finalized
	Finalized() bool
}

// Monadic is a photon instance deciding on one value.
// The caller samples k nodes and calls RecordPoll with the result.
// RecordUnsuccessfulPoll resets the confidence counters when one or
// more consecutive polls fail to reach alphaPreference votes.
type Monadic interface {
	fmt.Stringer

	// RecordPoll records the results of a network poll
	RecordPoll(count int)

	// RecordUnsuccessfulPoll resets the wave counter of this instance
	RecordUnsuccessfulPoll()

	// Return whether a choice has been finalized
	Finalized() bool

	// Returns a new dyadic instance with the original choice.
	Extend(originalPreference int) Dyadic

	// Returns a new monadic instance with the same state
	Clone() Monadic
}