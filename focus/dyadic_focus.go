// Copyright (C) 2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package focus

import (
	"fmt"
	"sync"

	"github.com/luxfi/consensus/wave"
)

var _ Dyadic = (*dyadicFocus)(nil)

func newDyadicFocus(alphaPreference int, terminationConditions []terminationCondition, choice int) *dyadicFocus {
	// Convert terminationConditions to wave format
	waveConditions := make([]wave.TerminationCondition, len(terminationConditions))
	for i, tc := range terminationConditions {
		waveConditions[i] = wave.TerminationCondition{
			AlphaConfidence: tc.alphaConfidence,
			Beta:            tc.beta,
		}
	}
	
	return &dyadicFocus{
		Dyadic:             wave.NewDyadicWave(alphaPreference, waveConditions, choice),
		alphaPreference:    alphaPreference,
		preference:         choice,
		preferenceStrength: [2]int{},
	}
}

// dyadicFocus is the implementation of a dyadic focus instance  
type dyadicFocus struct {
	// wrap the dyadic wave logic
	wave.Dyadic
	
	// mu protects the fields below
	mu sync.RWMutex
	
	// alphaPreference is the threshold required to update the preference
	alphaPreference int

	// preference is the choice with the largest number of polls which preferred
	// the color. Ties are broken by switching choice lazily
	preference int

	// preferenceStrength tracks the total number of network polls which
	// preferred each choice
	preferenceStrength [2]int
}

func (df *dyadicFocus) Preference() int {
	// It is possible, with low probability, that the wave preference is
	// not equal to the focus preference when wave finalizes. However,
	// this case is handled for completion. Therefore, if wave is
	// finalized, then our finalized wave choice should be preferred.
	if df.Finalized() {
		return df.Dyadic.Preference()
	}
	
	df.mu.RLock()
	defer df.mu.RUnlock()
	return df.preference
}

func (df *dyadicFocus) RecordPoll(count, choice int) {
	// Update focus state first
	df.mu.Lock()
	if count >= df.alphaPreference {
		df.preferenceStrength[choice]++
		if df.preferenceStrength[choice] > df.preferenceStrength[1-choice] {
			df.preference = choice
		}
	}
	df.mu.Unlock()
	
	// Then delegate to wave
	df.Dyadic.RecordPoll(count, choice)
}

func (df *dyadicFocus) String() string {
	df.mu.RLock()
	defer df.mu.RUnlock()
	
	return fmt.Sprintf(
		"DyadicFocus(Preference = %d, PreferenceStrength[0] = %d, PreferenceStrength[1] = %d, %s)",
		df.preference,
		df.preferenceStrength[0],
		df.preferenceStrength[1],
		df.Dyadic)
}

// getPreferenceStrength is a test helper to access preference strength
func (df *dyadicFocus) getPreferenceStrength() [2]int {
	df.mu.RLock()
	defer df.mu.RUnlock()
	return df.preferenceStrength
}