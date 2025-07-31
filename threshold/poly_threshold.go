// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"fmt"
	"slices"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/protocol/prism"
)

// polyThreshold is the implementation of a poly threshold instance
// that can be embedded by confidence
type polyThreshold struct {
	// wrap the poly sampler logic
	prism.PolySampler

	// alphaPreference is the threshold required to update the preference
	alphaPreference int

	// terminationConditions gives the ascending ordered list of alphaConfidence values
	// required to increment the corresponding confidence counter.
	// The corresponding beta values give the threshold required to finalize this instance.
	terminationConditions []terminationCondition

	// confidence is the number of consecutive successful prisms for a given
	// alphaConfidence threshold.
	// This instance finalizes when confidence[i] >= terminationConditions[i].beta for any i
	confidence []int

	// finalized prevents the state from changing after the required number of
	// consecutive prisms has been reached
	finalized bool
	
	// Focus state - tracks cumulative support
	focusStrength map[ids.ID]int
	
	// Track confidence per choice for Focus
	choiceConfidence map[ids.ID][]int
	
	// Track last choice polled for consecutive prism tracking
	lastChoice ids.ID
}

func newPolyThreshold(alphaPreference int, terminationConditions []terminationCondition, choice ids.ID) polyThreshold {
	focusStrength := make(map[ids.ID]int)
	// Initialize focusStrength for the initial choice
	focusStrength[choice] = 0
	
	return polyThreshold{
		PolySampler:           prism.NewPolySampler(choice),
		alphaPreference:       alphaPreference,
		terminationConditions: terminationConditions,
		confidence:            make([]int, len(terminationConditions)),
		focusStrength:         focusStrength,
		choiceConfidence:      make(map[ids.ID][]int),
	}
}

func (*polyThreshold) Add(_ ids.ID) {}

func (pt *polyThreshold) Preference() ids.ID {
	// If finalized, return the PolySampler preference
	if pt.Finalized() {
		return pt.PolySampler.Preference()
	}
	
	// Always use Focus behavior - return the choice with highest focusStrength
	var bestChoice ids.ID
	var bestStrength int = -1  // Start at -1 to handle zero strength
	currentPref := pt.PolySampler.Preference()
	
	// Make sure to check the current preference even if not in map
	if currentPref != ids.Empty {
		bestChoice = currentPref
		bestStrength = pt.focusStrength[currentPref]
	}
	
	for choice, strength := range pt.focusStrength {
		if strength > bestStrength || (strength == bestStrength && choice == currentPref) {
			bestStrength = strength
			bestChoice = choice
		}
	}
	
	if bestChoice != ids.Empty {
		return bestChoice
	}
	return currentPref
}

func (pt *polyThreshold) RecordPrism(count int, choice ids.ID) {
	if pt.finalized {
		return // This instance is already decided.
	}

	if count < pt.alphaPreference {
		pt.RecordUnsuccessfulPoll()
		return
	}

	// Focus: Update preference strength
	pt.focusStrength[choice]++
	
	// Initialize confidence array for this choice if needed
	if pt.choiceConfidence[choice] == nil {
		pt.choiceConfidence[choice] = make([]int, len(pt.terminationConditions))
	}
	
	// Reset confidence if not consecutive prism for same choice
	if choice != pt.lastChoice {
		// Clear confidence for all choices except current
		for c := range pt.choiceConfidence {
			if c != choice {
				clear(pt.choiceConfidence[c])
			}
		}
	}
	pt.lastChoice = choice
	
	// Check if we should switch preference based on accumulated strength
	// Find the choice with highest strength
	var bestChoice ids.ID
	var bestStrength int = -1
	for c, s := range pt.focusStrength {
		if s > bestStrength {
			bestStrength = s
			bestChoice = c
		}
	}
	
	// If the best choice is different from current PolySampler preference, update it
	if bestChoice != ids.Empty && bestChoice != pt.PolySampler.Preference() {
		pt.PolySampler.RecordSuccessfulPoll(bestChoice)
		// Copy choice's confidence to main confidence tracker
		pt.confidence = slices.Clone(pt.choiceConfidence[bestChoice])
	}

	// Build confidence for this choice with alphaConfidence support
	for i, terminationCondition := range pt.terminationConditions {
		// If I did not reach this alpha threshold, I did not
		// reach any more alpha thresholds.
		// Clear the remaining confidence counters.
		if count < terminationCondition.AlphaConfidence {
			clear(pt.choiceConfidence[choice][i:])
			if choice == pt.Preference() {
				clear(pt.confidence[i:])
			}
			return
		}

		// Build confidence for this choice
		pt.choiceConfidence[choice][i]++
		
		// If this is our preference, update main confidence and check finalization
		if choice == pt.Preference() {
			pt.confidence[i] = pt.choiceConfidence[choice][i]
			if pt.confidence[i] >= terminationCondition.Beta {
				pt.finalized = true
				return
			}
		}
	}
}

func (pt *polyThreshold) RecordUnsuccessfulPoll() {
	clear(pt.confidence)
	// Clear all choice confidence on unsuccessful poll
	for c := range pt.choiceConfidence {
		clear(pt.choiceConfidence[c])
	}
	pt.lastChoice = ids.Empty
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
