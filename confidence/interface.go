// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package confidence

// Confidence tracks confidence in consensus decisions
type Confidence interface {
	// RecordPoll records a successful poll result
	RecordPoll(count int)
	
	// RecordUnsuccessfulPoll records an unsuccessful poll
	RecordUnsuccessfulPoll()
	
	// Finalized returns whether confidence threshold has been reached
	Finalized() bool
}

