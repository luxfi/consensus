// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package confidence

import (
	"fmt"
	"slices"
)

var _ Unary = (*unaryQuantum)(nil)

func newUnaryQuantum(alphaPreference int, terminationConditions []terminationCondition) unaryQuantum {
	return unaryQuantum{
		alphaPreference:       alphaPreference,
		terminationConditions: terminationConditions,
		confidence:            make([]int, len(terminationConditions)),
	}
}

// unaryQuantum is the implementation of a unary quantum consensus instance
// Invariant:
// len(terminationConditions) == len(confidence)
// terminationConditions[i].alphaConfidence < terminationConditions[i+1].alphaConfidence
// terminationConditions[i].beta >= terminationConditions[i+1].beta
// confidence[i] >= confidence[i+1] (except after finalizing due to early termination)
type unaryQuantum struct {
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

func (sf *unaryQuantum) RecordPoll(count int) {
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

func (sf *unaryQuantum) RecordUnsuccessfulPoll() {
	clear(sf.confidence)
}

func (sf *unaryQuantum) Finalized() bool {
	return sf.finalized
}

func (sf *unaryQuantum) Extend(choice int) Binary {
	return &binaryQuantum{
		binarySlush:           binarySlush{preference: choice},
		confidence:            slices.Clone(sf.confidence),
		alphaPreference:       sf.alphaPreference,
		terminationConditions: sf.terminationConditions,
		finalized:             sf.finalized,
	}
}

func (sf *unaryQuantum) Clone() Unary {
	newQuantum := *sf
	newQuantum.confidence = slices.Clone(sf.confidence)
	return &newQuantum
}

func (sf *unaryQuantum) String() string {
	return fmt.Sprintf("QC(Confidence = %v, Finalized = %v)",
		sf.confidence,
		sf.finalized)
}