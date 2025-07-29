// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/photon"
)

var _ Polyadic = (*polyadicWave)(nil)

func NewPolyadicWave(alphaPreference int, TerminationConditions []TerminationCondition, choice ids.ID) Polyadic {
	return &polyadicWave{
		PolyadicPhoton:        photon.NewPolyadicPhoton(choice),
		alphaPreference:       alphaPreference,
		TerminationConditions: TerminationConditions,
		confidence:            make([]int, len(TerminationConditions)),
	}
}

// polyadicWave is the implementation of a wave instance with an
// unbounded number of choices
// Invariant:
// len(TerminationConditions) == len(confidence)
// TerminationConditions[i].AlphaConfidence < TerminationConditions[i+1].AlphaConfidence
// TerminationConditions[i].Beta >= TerminationConditions[i+1].beta
// confidence[i] >= confidence[i+1] (except after finalizing due to early termination)
type polyadicWave struct {
	// wrap the polyadic photon logic
	photon.PolyadicPhoton

	// alphaPreference is the threshold required to update the preference
	alphaPreference int

	// TerminationConditions gives the ascending ordered list of AlphaConfidence values
	// required to increment the corresponding confidence counter.
	// The corresponding Beta values give the threshold required to finalize this instance.
	TerminationConditions []TerminationCondition

	// confidence is the number of consecutive successful polls for a given
	// AlphaConfidence threshold.
	// This instance finalizes when confidence[i] >= TerminationConditions[i].Beta for any i
	confidence []int

	// finalized prevents the state from changing after the required number of
	// consecutive polls has been reached
	finalized bool
}

func (*polyadicWave) Add(_ ids.ID) {}

func (pw *polyadicWave) RecordPoll(count int, choice ids.ID) {
	if pw.finalized {
		return // This instance is already decided.
	}

	if count < pw.alphaPreference {
		pw.RecordUnsuccessfulPoll()
		return
	}

	// If I am changing my preference, reset confidence counters
	// before recording a successful poll on the slush instance.
	if choice != pw.Preference() {
		clear(pw.confidence)
	}
	pw.PolyadicPhoton.RecordSuccessfulPoll(choice)

	for i, TerminationCondition := range pw.TerminationConditions {
		// If I did not reach this alpha threshold, I did not
		// reach any more alpha thresholds.
		// Clear the remaining confidence counters.
		if count < TerminationCondition.AlphaConfidence {
			clear(pw.confidence[i:])
			return
		}

		// I reached this alpha threshold, increment the confidence counter
		// and check if I can finalize.
		pw.confidence[i]++
		if pw.confidence[i] >= TerminationCondition.Beta {
			pw.finalized = true
			return
		}
	}
}

func (pw *polyadicWave) RecordUnsuccessfulPoll() {
	clear(pw.confidence)
}

func (pw *polyadicWave) Finalized() bool {
	return pw.finalized
}

func (pw *polyadicWave) String() string {
	return fmt.Sprintf("PolyadicWave(Confidence = %v, Finalized = %v, %s)",
		pw.confidence,
		pw.finalized,
		&pw.PolyadicPhoton)
}