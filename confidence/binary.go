// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package confidence

// Binary is a snow instance deciding between two values.
type Binary interface {
	// RecordPoll records the results of a network poll. If the poll was
	// successful, the preference of the instance may be updated. Assumes that
	// the received votes have been filtered for conflicting IDs.
	RecordPoll(count int, choice int)

	// RecordUnsuccessfulPoll resets the confidence of this instance
	RecordUnsuccessfulPoll()

	// Preference returns the choice that this instance has preferred
	Preference() int

	// Finalized returns true if this instance has accepted a choice
	Finalized() bool
}