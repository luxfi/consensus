// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package focus

import (
	"fmt"
	"sync"

	"github.com/luxfi/consensus/wave"
)

var _ Monadic = (*monadicFocus)(nil)

func newMonadicFocus(alphaPreference int, terminationConditions []terminationCondition) *monadicFocus {
	// Convert terminationConditions to wave format
	waveConditions := make([]wave.TerminationCondition, len(terminationConditions))
	for i, tc := range terminationConditions {
		waveConditions[i] = wave.TerminationCondition{
			AlphaConfidence: tc.alphaConfidence,
			Beta:            tc.beta,
		}
	}
	
	confidence := make([]int, len(terminationConditions))
	return &monadicFocus{
		Monadic:         wave.NewMonadicWave(alphaPreference, waveConditions),
		alphaPreference: alphaPreference,
		confidence:      confidence,
	}
}

// monadicFocus is the implementation of a monadic focus instance
type monadicFocus struct {
	// wrap the monadic wave logic
	wave.Monadic

	// mu protects the fields below
	mu sync.RWMutex

	// alphaPreference is the threshold required to update the preference
	alphaPreference int

	// preferenceStrength tracks the total number of polls with a preference
	preferenceStrength int
	
	// confidence tracks the confidence values for finalization
	confidence []int
}

func (mf *monadicFocus) RecordPoll(count int) {
	// Update focus state first
	mf.mu.Lock()
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
	mf.mu.Unlock()
	
	// Then delegate to wave
	mf.Monadic.RecordPoll(count)
}

func (mf *monadicFocus) RecordUnsuccessfulPoll() {
	// Update focus state first
	mf.mu.Lock()
	// Reset confidence
	for i := range mf.confidence {
		mf.confidence[i] = 0
	}
	mf.mu.Unlock()
	
	// Then delegate to wave
	mf.Monadic.RecordUnsuccessfulPoll()
}

func (mf *monadicFocus) Extend(choice int) Dyadic {
	mf.mu.RLock()
	strength := mf.preferenceStrength
	alpha := mf.alphaPreference
	mf.mu.RUnlock()
	
	df := &dyadicFocus{
		Dyadic:             mf.Monadic.Extend(choice),
		alphaPreference:    alpha,
		preference:         choice,
		preferenceStrength: [2]int{},
	}
	df.preferenceStrength[choice] = strength
	return df
}

func (mf *monadicFocus) Clone() Monadic {
	mf.mu.RLock()
	defer mf.mu.RUnlock()
	
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
	mf.mu.RLock()
	defer mf.mu.RUnlock()
	
	return fmt.Sprintf("MonadicFocus(PreferenceStrength = %d, Confidence = %v, Finalized = %v)",
		mf.preferenceStrength,
		mf.confidence,
		mf.Finalized())
}