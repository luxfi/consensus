// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package confidence

import "fmt"

var _ Binary = (*binaryQuantum)(nil)

func newBinaryQuantum(alphaPreference int, terminationConditions []terminationCondition, choice int) binaryQuantum {
	return binaryQuantum{
		binarySlush:           newBinarySlush(choice),
		alphaPreference:       alphaPreference,
		terminationConditions: terminationConditions,
		confidence:            make([]int, len(terminationConditions)),
	}
}

// binaryQuantum is the implementation of a binary quantum consensus instance
// Invariant:
// len(terminationConditions) == len(confidence)
// terminationConditions[i].alphaConfidence < terminationConditions[i+1].alphaConfidence
// terminationConditions[i].beta >= terminationConditions[i+1].beta
// confidence[i] >= confidence[i+1] (except after finalizing due to early termination)
type binaryQuantum struct {
	// wrap the binary slush logic
	binarySlush

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

func (sf *binaryQuantum) RecordPoll(count, choice int) {
	if sf.finalized {
		return // This instance is already decided.
	}

	if count < sf.alphaPreference {
		sf.RecordUnsuccessfulPoll()
		return
	}

	// If I am changing my preference, reset confidence counters
	// before recording a successful poll on the slush instance.
	if choice != sf.Preference() {
		clear(sf.confidence)
	}
	sf.binarySlush.RecordSuccessfulPoll(choice)

	for i, terminationCondition := range sf.terminationConditions {
		// If I did not reach this alpha threshold, I did not
		// reach any more alpha thresholds.
		// Clear the remaining confidence counters.
		if count < terminationCondition.AlphaConfidence {
			clear(sf.confidence[i:])
			return
		}

		// I reached this alpha threshold, increment the confidence counter
		// and check if I can finalize.
		sf.confidence[i]++
		if sf.confidence[i] >= terminationCondition.Beta {
			sf.finalized = true
			return
		}
	}
}

func (sf *binaryQuantum) RecordUnsuccessfulPoll() {
	clear(sf.confidence)
}

func (sf *binaryQuantum) Finalized() bool {
	return sf.finalized
}

func (sf *binaryQuantum) String() string {
	return fmt.Sprintf("QC(Confidence = %v, Finalized = %v, %s)",
		sf.confidence,
		sf.finalized,
		&sf.binarySlush)
}