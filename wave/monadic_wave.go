// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
	"fmt"
	"slices"

	"github.com/luxfi/consensus/photon"
)

var _ Monadic = (*monadicWave)(nil)

func NewMonadicWave(alphaPreference int, TerminationConditions []TerminationCondition) Monadic {
	return &monadicWave{
		alphaPreference:       alphaPreference,
		TerminationConditions: TerminationConditions,
		confidence:            make([]int, len(TerminationConditions)),
	}
}

// monadicWave is the implementation of a monadic wave instance
// Invariant:
// len(TerminationConditions) == len(confidence)
// TerminationConditions[i].AlphaConfidence < TerminationConditions[i+1].AlphaConfidence
// TerminationConditions[i].Beta >= TerminationConditions[i+1].beta
// confidence[i] >= confidence[i+1] (except after finalizing due to early termination)
type monadicWave struct {
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

func (mw *monadicWave) RecordPoll(count int) {
	for i, TerminationCondition := range mw.TerminationConditions {
		// If I did not reach this alpha threshold, I did not
		// reach any more alpha thresholds.
		// Clear the remaining confidence counters.
		if count < TerminationCondition.AlphaConfidence {
			clear(mw.confidence[i:])
			return
		}

		// I reached this alpha threshold, increment the confidence counter
		// and check if I can finalize.
		mw.confidence[i]++
		if mw.confidence[i] >= TerminationCondition.Beta {
			mw.finalized = true
			return
		}
	}
}

func (mw *monadicWave) RecordUnsuccessfulPoll() {
	clear(mw.confidence)
}

func (mw *monadicWave) Finalized() bool {
	return mw.finalized
}

func (mw *monadicWave) Extend(choice int) Dyadic {
	return &dyadicWave{
		DyadicPhoton:          photon.NewDyadicPhoton(choice),
		confidence:            slices.Clone(mw.confidence),
		alphaPreference:       mw.alphaPreference,
		TerminationConditions: mw.TerminationConditions,
		finalized:             mw.finalized,
	}
}

func (mw *monadicWave) Clone() Monadic {
	newWave := *mw
	newWave.confidence = slices.Clone(mw.confidence)
	return &newWave
}

func (mw *monadicWave) String() string {
	return fmt.Sprintf("MonadicWave(Confidence = %v, Finalized = %v)",
		mw.confidence,
		mw.finalized)
}