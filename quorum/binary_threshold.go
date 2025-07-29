// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"fmt"

	"github.com/luxfi/consensus/poll"
)

// binaryThreshold is the implementation of a binary threshold instance
// that can be embedded by confidence
type binaryThreshold struct {
	// wrap the binary sampler logic
	poll.BinarySampler

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
	
	// Focus state - tracks cumulative support
	focusStrength [2]int // strength for choice 0 and 1
	
	// Track confidence per choice for Focus
	choiceConfidence [2][]int
	
	// Track last choice polled for consecutive poll tracking
	lastChoice int
	lastChoiceSet bool
}

func newBinaryThreshold(alphaPreference int, terminationConditions []terminationCondition, choice int) binaryThreshold {
	bt := binaryThreshold{
		BinarySampler:         poll.NewBinarySampler(choice),
		alphaPreference:       alphaPreference,
		terminationConditions: terminationConditions,
		confidence:            make([]int, len(terminationConditions)),
		// focusStrength starts at [0, 0] - no initialization needed
	}
	// Initialize confidence arrays for both choices
	bt.choiceConfidence[0] = make([]int, len(terminationConditions))
	bt.choiceConfidence[1] = make([]int, len(terminationConditions))
	return bt
}

func (bt *binaryThreshold) RecordPoll(count, choice int) {
	if bt.finalized {
		return // This instance is already decided.
	}

	if count < bt.alphaPreference {
		bt.RecordUnsuccessfulPoll()
		return
	}

	// Focus: Update preference strength
	bt.focusStrength[choice]++
	
	// Reset confidence if not consecutive poll for same choice
	if !bt.lastChoiceSet || choice != bt.lastChoice {
		// Clear confidence for the other choice
		clear(bt.choiceConfidence[1-choice])
	}
	bt.lastChoice = choice
	bt.lastChoiceSet = true
	
	// Build confidence for this choice with alphaConfidence support
	for i, terminationCondition := range bt.terminationConditions {
		// If I did not reach this alpha threshold, I did not
		// reach any more alpha thresholds.
		// Clear the remaining confidence counters.
		if count < terminationCondition.alphaConfidence {
			clear(bt.choiceConfidence[choice][i:])
			if choice == bt.Preference() {
				clear(bt.confidence[i:])
			}
			break
		}

		// Build confidence for this choice
		bt.choiceConfidence[choice][i]++
	}
	
	// Check if we should switch preference based on accumulated strength
	currentPreference := bt.Preference()
	if bt.focusStrength[choice] > bt.focusStrength[currentPreference] {
		// Switch preference
		bt.BinarySampler.RecordSuccessfulPoll(choice)
		// Copy choice's confidence to main confidence tracker
		copy(bt.confidence, bt.choiceConfidence[choice])
		currentPreference = choice
	}
	
	// Check finalization if this is our preference
	if choice == currentPreference {
		// Update main confidence tracker
		copy(bt.confidence, bt.choiceConfidence[choice])
		
		// Check if we should finalize
		for i, terminationCondition := range bt.terminationConditions {
			if bt.confidence[i] >= terminationCondition.beta {
				bt.finalized = true
				return
			}
		}
	}
}

func (bt *binaryThreshold) RecordUnsuccessfulPoll() {
	clear(bt.confidence)
	// Clear all choice confidence on unsuccessful poll
	clear(bt.choiceConfidence[0])
	clear(bt.choiceConfidence[1])
	bt.lastChoiceSet = false
}

func (bt *binaryThreshold) Finalized() bool {
	return bt.finalized
}

func (bt *binaryThreshold) String() string {
	return fmt.Sprintf("BT(Confidence = %v, Finalized = %v, %+v)",
		bt.confidence,
		bt.finalized,
		bt.BinarySampler)
}
