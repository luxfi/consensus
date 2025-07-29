// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

import (
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/bag"
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
	RecordPoll(votes bag.Bag[ids.ID]) bool

	// RecordUnsuccessfulPoll resets the counters of this consensus instance
	RecordUnsuccessfulPoll()

	// Return whether a choice has been finalized
	Finalized() bool
}

// Polyadic is a consensus instance deciding between an unbounded number of values.
type Polyadic interface {
	fmt.Stringer

	// Adds a new possible choice
	Add(newChoice ids.ID)

	// Returns the currently preferred choice to be finalized
	Preference() ids.ID

	// RecordPoll records the results of a network poll
	RecordPoll(count int, choice ids.ID)

	// RecordUnsuccessfulPoll resets the counter of this instance
	RecordUnsuccessfulPoll()

	// Return whether a choice has been finalized
	Finalized() bool
}

// Dyadic is a consensus instance deciding between two values.
type Dyadic interface {
	fmt.Stringer

	// Returns the currently preferred choice to be finalized
	Preference() int

	// RecordPoll records the results of a network poll
	RecordPoll(count, choice int)

	// RecordUnsuccessfulPoll resets the counter of this instance
	RecordUnsuccessfulPoll()

	// Return whether a choice has been finalized
	Finalized() bool
}

// Monadic is a consensus instance deciding on one value.
type Monadic interface {
	fmt.Stringer

	// RecordPoll records the results of a network poll
	RecordPoll(count int)

	// RecordUnsuccessfulPoll resets the counter of this instance
	RecordUnsuccessfulPoll()

	// Return whether a choice has been finalized
	Finalized() bool

	// Returns a new dyadic instance with the original choice
	Extend(originalPreference int) Dyadic

	// Returns a new monadic instance with the same state
	Clone() Monadic
}