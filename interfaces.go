// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package consensus implements the Lux consensus protocol.
//
// The consensus package provides a modular, composable framework for
// building consensus algorithms. It is designed to be a drop-in replacement
// for Lux Nodes's consensus while providing cleaner interfaces and
// better separation of concerns.
package consensus

import (
	"context"
	"fmt"

	"github.com/luxfi/ids"
)

// Consensus represents a consensus instance that processes the results
// of network queries to reach agreement on a value.
type Consensus interface {
	fmt.Stringer

	// Add introduces a new choice to the consensus instance.
	Add(choice ids.ID) error

	// Preference returns the currently preferred choice.
	Preference() ids.ID

	// RecordPoll records the results of a network poll.
	// Returns true if the poll was successful, false otherwise.
	RecordPoll(votes Bag[ids.ID]) bool

	// RecordUnsuccessfulPoll resets internal counters after an unsuccessful poll.
	RecordUnsuccessfulPoll()

	// Finalized returns true if consensus has been reached.
	Finalized() bool
}

// Binary represents binary consensus between two choices.
type Binary interface {
	fmt.Stringer

	// Preference returns the currently preferred choice (0 or 1).
	Preference() int

	// RecordPoll records a poll result for a binary choice.
	RecordPoll(count int, choice int) bool

	// RecordUnsuccessfulPoll resets internal counters.
	RecordUnsuccessfulPoll()

	// Finalized returns true if consensus has been reached.
	Finalized() bool
}

// Unary represents unary consensus (accept/reject a single value).
type Unary interface {
	fmt.Stringer

	// RecordPoll records a poll result.
	RecordPoll(count int) bool

	// RecordUnsuccessfulPoll resets internal counters.
	RecordUnsuccessfulPoll()

	// Finalized returns true if consensus has been reached.
	Finalized() bool

	// Extend creates a binary consensus instance from this unary instance.
	Extend(choice int) Binary

	// Clone creates a copy of this unary instance.
	Clone() Unary
}

// Factory creates consensus instances with specific parameters.
type Factory interface {
	// NewConsensus creates a new n-ary consensus instance.
	NewConsensus(params Parameters, choice ids.ID) Consensus

	// NewBinary creates a new binary consensus instance.
	NewBinary(params Parameters, choice int) Binary

	// NewUnary creates a new unary consensus instance.
	NewUnary(params Parameters) Unary
}

// Parameters defines consensus algorithm parameters.
type Parameters interface {
	// K returns the sample size.
	K() int

	// AlphaPreference returns the preference threshold.
	AlphaPreference() int

	// AlphaConfidence returns the confidence threshold.
	AlphaConfidence() int

	// Beta returns the finalization threshold.
	Beta() int

	// Valid returns an error if the parameters are invalid.
	Valid() error
}

// Health represents the health of a consensus instance.
type Health interface {
	// Healthy returns true if the consensus instance is healthy.
	Healthy(context.Context) (bool, error)

	// HealthReport returns a detailed health report.
	HealthReport(context.Context) (HealthReport, error)
}

// HealthReport contains detailed health information.
type HealthReport struct {
	// ConsensusType identifies the consensus algorithm.
	ConsensusType string

	// Healthy indicates overall health status.
	Healthy bool

	// Details contains algorithm-specific health information.
	Details map[string]interface{}

	// Metrics contains performance metrics.
	Metrics Metrics
}

// Metrics tracks consensus performance.
type Metrics struct {
	// PollsRecorded is the total number of polls recorded.
	PollsRecorded uint64

	// SuccessfulPolls is the number of successful polls.
	SuccessfulPolls uint64

	// UnsuccessfulPolls is the number of unsuccessful polls.
	UnsuccessfulPolls uint64

	// TimeToFinalization is the time taken to reach consensus in nanoseconds.
	TimeToFinalization int64
}

// Pollable represents an object that can be polled in consensus.
type Pollable interface {
	// PollID returns the ID used for polling.
	PollID() ids.ID
}

// Dependencies represents an object with dependencies on other objects.
type Dependencies interface {
	// Dependencies returns the IDs this object depends on.
	Dependencies() []ids.ID
}

// Conflicts represents an object that may conflict with other objects.
type Conflicts interface {
	// Conflicts returns the IDs this object conflicts with.
	Conflicts() []ids.ID
}
