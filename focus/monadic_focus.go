// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package focus

import (
	"fmt"

	"github.com/luxfi/consensus/wave"
)

var _ Monadic = (*monadicFocus)(nil)

func newMonadicFocus(alphaPreference int, terminationConditions []terminationCondition) monadicFocus {
	// Convert terminationConditions to wave format
	waveConditions := make([]wave.TerminationCondition, len(terminationConditions))
	for i, tc := range terminationConditions {
		waveConditions[i] = wave.TerminationCondition{
			AlphaConfidence: tc.alphaConfidence,
			Beta:            tc.beta,
		}
	}
	
	confidence := make([]int, len(terminationConditions))
	return monadicFocus{
		Monadic:         wave.NewMonadicWave(alphaPreference, waveConditions),
		alphaPreference: alphaPreference,
		confidence:      confidence,
	}
}

// monadicFocus is the implementation of a monadic focus instance
type monadicFocus struct {
	// wrap the monadic wave logic
	wave.Monadic

	// alphaPreference is the threshold required to update the preference
	alphaPreference int

	// preferenceStrength tracks the total number of polls with a preference
	preferenceStrength int
	
	// confidence tracks the confidence values for finalization
	confidence []int
}

func (mf *monadicFocus) RecordPoll(count int) {
	if count >= mf.alphaPreference {
		mf.preferenceStrength++
		// Update confidence
		for i := range mf.confidence {
			mf.confidence[i]++
		}
	} else {
		// Reset confidence
		for i := range mf.confidence {
			mf.confidence[i] = 0
		}
	}
	mf.Monadic.RecordPoll(count)
}

func (mf *monadicFocus) RecordUnsuccessfulPoll() {
	// Reset confidence
	for i := range mf.confidence {
		mf.confidence[i] = 0
	}
	mf.Monadic.RecordUnsuccessfulPoll()
}

func (mf *monadicFocus) Extend(choice int) Dyadic {
	df := &dyadicFocus{
		Dyadic:             mf.Monadic.Extend(choice),
		alphaPreference:    mf.alphaPreference,
		preference:         choice,
		preferenceStrength: [2]int{},
	}
	df.preferenceStrength[choice] = mf.preferenceStrength
	return df
}

func (mf *monadicFocus) Clone() Monadic {
	confidence := make([]int, len(mf.confidence))
	copy(confidence, mf.confidence)
	return &monadicFocus{
		Monadic:            mf.Monadic.Clone(),
		alphaPreference:    mf.alphaPreference,
		preferenceStrength: mf.preferenceStrength,
		confidence:         confidence,
	}
}

func (mf *monadicFocus) String() string {
	return fmt.Sprintf("MonadicFocus(PreferenceStrength = %d, Confidence = %v, Finalized = %v)",
		mf.preferenceStrength,
		mf.confidence,
		mf.Finalized())
}