// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
	"fmt"
	"sync"

	"github.com/luxfi/consensus/photon"
)

var _ Dyadic = (*dyadicWave)(nil)

// NewDyadicWave creates a new dyadic wave instance
func NewDyadicWave(alphaPreference int, TerminationConditions []TerminationCondition, choice int) Dyadic {
	return &dyadicWave{
		DyadicPhoton:          photon.NewDyadicPhoton(choice),
		alphaPreference:       alphaPreference,
		TerminationConditions: TerminationConditions,
		confidence:            make([]int, len(TerminationConditions)),
	}
}

// dyadicWave is the implementation of a dyadic wave instance
// Invariant:
// len(TerminationConditions) == len(confidence)
// TerminationConditions[i].AlphaConfidence < TerminationConditions[i+1].AlphaConfidence
// TerminationConditions[i].Beta >= TerminationConditions[i+1].beta
// confidence[i] >= confidence[i+1] (except after finalizing due to early termination)
type dyadicWave struct {
	// wrap the dyadic photon logic
	photon.DyadicPhoton

	// mu protects all fields below
	mu sync.RWMutex

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

func (dw *dyadicWave) Preference() int {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	return dw.DyadicPhoton.Preference()
}

func (dw *dyadicWave) RecordPoll(count, choice int) {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	if dw.finalized {
		return // This instance is already decided.
	}

	if count < dw.alphaPreference {
		clear(dw.confidence)
		return
	}

	// If I am changing my preference, reset confidence counters
	// before recording a successful poll on the slush instance.
	if choice != dw.DyadicPhoton.Preference() {
		clear(dw.confidence)
	}
	dw.DyadicPhoton.RecordSuccessfulPoll(choice)

	for i, TerminationCondition := range dw.TerminationConditions {
		// If I did not reach this alpha threshold, I did not
		// reach any more alpha thresholds.
		// Clear the remaining confidence counters.
		if count < TerminationCondition.AlphaConfidence {
			clear(dw.confidence[i:])
			return
		}

		// I reached this alpha threshold, increment the confidence counter
		// and check if I can finalize.
		dw.confidence[i]++
		if dw.confidence[i] >= TerminationCondition.Beta {
			dw.finalized = true
			return
		}
	}
}

func (dw *dyadicWave) RecordUnsuccessfulPoll() {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	clear(dw.confidence)
}

func (dw *dyadicWave) Finalized() bool {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	return dw.finalized
}

func (dw *dyadicWave) String() string {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	return fmt.Sprintf("DyadicWave(Confidence = %v, Finalized = %v, %s)",
		dw.confidence,
		dw.finalized,
		&dw.DyadicPhoton)
}
