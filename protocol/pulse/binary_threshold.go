// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pulse

import (
	"fmt"

	"github.com/luxfi/consensus/protocol/prism"
	quorum "github.com/luxfi/consensus/threshold"
)

// binaryThreshold is the implementation of a binary threshold instance
// that can be embedded by confidence
type binaryThreshold struct {
	// wrap the binary sampler logic  
	prism.BinarySampler

	// alphaPreference is the threshold required to update the preference
	alphaPreference int

	// terminationConditions gives the ascending ordered list of alphaConfidence values
	// required to increment the corresponding confidence counter.
	// The corresponding beta values give the threshold required to finalize this instance.
	terminationConditions []quorum.TerminationCondition

	// confidence is the number of consecutive successful prisms for a given
	// alphaConfidence threshold.
	// This instance finalizes when confidence[i] >= terminationConditions[i].beta for any i
	confidence []int

	// finalized prevents the state from changing after the required number of
	// consecutive prisms has been reached
	finalized bool

	// preference is the choice with the largest number of prisms which preferred
	// the color. Ties are broken by switching choice lazily
	preference int

	// preferenceStrength tracks the total number of network prisms which
	// preferred each choice
	preferenceStrength [2]int
}

func newBinaryThreshold(alphaPreference int, terminationConditions []quorum.TerminationCondition, choice int) binaryThreshold {
	return binaryThreshold{
		BinarySampler:         prism.NewBinarySampler(choice),
		alphaPreference:       alphaPreference,
		terminationConditions: terminationConditions,
		confidence:            make([]int, len(terminationConditions)),
		preference:            choice,
	}
}

func (bt *binaryThreshold) Preference() int {
	// It is possible, with low probability, that the pulse preference is
	// not equal to the photon preference when pulse finalizes. However,
	// this case is handled for completion. Therefore, if pulse is
	// finalized, then our finalized pulse choice should be preferred.
	if bt.Finalized() {
		return bt.BinarySampler.Preference()
	}
	
	// If alphaPreference is 1, use flake behavior (immediate switching)
	if bt.alphaPreference == 1 {
		return bt.BinarySampler.Preference()
	}
	
	return bt.preference
}

func (bt *binaryThreshold) RecordPrism(count, choice int) {
	if bt.finalized {
		return // This instance is already decided.
	}

	if count < bt.alphaPreference {
		bt.RecordUnsuccessfulPoll()
		return
	}

	// Track preference strength for ball behavior
	bt.preferenceStrength[choice]++
	
	// Update preference based on strength
	if bt.preferenceStrength[choice] > bt.preferenceStrength[1-choice] {
		bt.preference = choice
	}

	// If I am changing my preference, reset confidence counters
	// before recording a successful prism on the sampler instance.
	if choice != bt.BinarySampler.Preference() {
		clear(bt.confidence)
	}
	bt.BinarySampler.RecordSuccessfulPoll(choice)

	for i, terminationCondition := range bt.terminationConditions {
		// If I did not reach this alpha threshold, I did not
		// reach any more alpha thresholds.
		// Clear the remaining confidence counters.
		if count < terminationCondition.AlphaConfidence {
			clear(bt.confidence[i:])
			return
		}

		// I reached this alpha threshold, increment the confidence counter
		// and check if I can finalize.
		bt.confidence[i]++
		if bt.confidence[i] >= terminationCondition.Beta {
			bt.finalized = true
			return
		}
	}
}

func (bt *binaryThreshold) RecordUnsuccessfulPoll() {
	clear(bt.confidence)
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
