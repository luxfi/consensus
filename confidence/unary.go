// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package confidence

// Unary is a consensus instance deciding on a single value.
type Unary interface {
	// RecordPoll records the results of a network poll. If the poll was
	// successful, the confidence of the instance may be updated.
	RecordPoll(count int)

	// RecordUnsuccessfulPoll resets the confidence of this instance
	RecordUnsuccessfulPoll()

	// Finalized returns true if this instance has accepted
	Finalized() bool

	// Extend returns a binary instance with the provided choice
	Extend(choice int) Binary

	// Clone returns a copy of this unary instance
	Clone() Unary
}