// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/poll"
)

// polyThreshold is the implementation of a poly threshold instance
// that can be embedded by confidence
type polyThreshold struct {
	// wrap the poly sampler logic
	poll.PolySampler

	// alphaPreference is the threshold required to update the preference
	alphaPreference int

	// terminationConditions gives the ascending ordered list of alphaConfidence values
	// required to increment the corresponding confidence counter.
	// The corresponding beta values give the threshold required to finalize this instance.
	terminationConditions []terminationCondition

	// confidence is the number of consecutive successful polls for a given
	// alphaConfidence threshold.
	// This instance finalizes when confidence[i] >= terminationConditions[i].beta for any i
	confidence []int

	// finalized prevents the state from changing after the required number of
	// consecutive polls has been reached
	finalized bool
}

func newPolyThreshold(alphaPreference int, terminationConditions []terminationCondition, choice ids.ID) polyThreshold {
	return polyThreshold{
		PolySampler:           poll.NewPolySampler(choice),
		alphaPreference:       alphaPreference,
		terminationConditions: terminationConditions,
		confidence:            make([]int, len(terminationConditions)),
	}
}

func (*polyThreshold) Add(_ ids.ID) {}

func (pt *polyThreshold) RecordPoll(count int, choice ids.ID) {
	if pt.finalized {
		return // This instance is already decided.
	}

	if count < pt.alphaPreference {
		pt.RecordUnsuccessfulPoll()
		return
	}

	// If I am changing my preference, reset confidence counters
	// before recording a successful poll on the sampler instance.
	if choice != pt.Preference() {
		clear(pt.confidence)
	}
	pt.PolySampler.RecordSuccessfulPoll(choice)

	for i, terminationCondition := range pt.terminationConditions {
		// If I did not reach this alpha threshold, I did not
		// reach any more alpha thresholds.
		// Clear the remaining confidence counters.
		if count < terminationCondition.alphaConfidence {
			clear(pt.confidence[i:])
			return
		}

		// I reached this alpha threshold, increment the confidence counter
		// and check if I can finalize.
		pt.confidence[i]++
		if pt.confidence[i] >= terminationCondition.beta {
			pt.finalized = true
			return
		}
	}
}

func (pt *polyThreshold) RecordUnsuccessfulPoll() {
	clear(pt.confidence)
}

func (pt *polyThreshold) Finalized() bool {
	return pt.finalized
}

func (pt *polyThreshold) String() string {
	return fmt.Sprintf("PT(Confidence = %v, Finalized = %v, %v)",
		pt.confidence,
		pt.finalized,
		pt.PolySampler)
}
